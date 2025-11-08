package cleaner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/shouni/action-perfect-get-on-go/prompts"
	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
)

// ContentSeparator は、結合された複数の文書間を区切るための明確な区切り文字です。
const ContentSeparator = "\n\n--- DOCUMENT END ---\n\n"

// DefaultSeparator は、一般的な段落区切りに使用される標準的な区切り文字です。
const DefaultSeparator = "\n\n"

// MaxSegmentChars は、MapフェーズでLLMに一度に渡す安全な最大文字数。
// トークン制限に十分なマージンを持たせた値です。
const MaxSegmentChars = 400000

// ----------------------------------------------------------------
// LLM応答マーカーの定数
// ----------------------------------------------------------------
const (
	// FinalStartMarker は Reduce プロンプトで定義された最終出力開始マーカーです。
	FinalStartMarker = "<FINAL_START>"
	FinalEndMarker   = "<FINAL_END>"
)

// ----------------------------------------------------------------
// Cleaner 構造体とコンストラクタの導入
// ----------------------------------------------------------------

// Cleaner はコンテンツのクリーンアップと要約を担当します。
type Cleaner struct {
	mapBuilder    *prompts.PromptBuilder
	reduceBuilder *prompts.PromptBuilder
}

// NewCleaner は新しい Cleaner インスタンスを作成し、PromptBuilderを一度だけ初期化します。
// 外部との整合性を保つため、引数なしに戻します。
func NewCleaner() (*Cleaner, error) {
	// テンプレートパースはここで一度だけ行い、失敗した場合はエラーを返す
	mapBuilder := prompts.NewMapPromptBuilder()
	if err := mapBuilder.Err(); err != nil {
		return nil, fmt.Errorf("failed to initialize map prompt builder: %w", err)
	}
	reduceBuilder := prompts.NewReducePromptBuilder()
	if err := reduceBuilder.Err(); err != nil {
		return nil, fmt.Errorf("failed to initialize reduce prompt builder: %w", err)
	}

	return &Cleaner{
		mapBuilder:    mapBuilder,
		reduceBuilder: reduceBuilder,
	}, nil
}

// ----------------------------------------------------------------
// 新規追加: URLリストをMarkdown形式に整形するヘルパー関数
// ----------------------------------------------------------------

// formatURLsForTemplate は、URLの文字列スライスをMarkdownリスト形式に変換します。
// * [URL] + 改行 の形式で結合し、テンプレートに渡せる単一の文字列を生成します。
func formatURLsForTemplate(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	var b strings.Builder
	for _, url := range urls {
		// * [URL] 形式で整形し、改行を追加
		b.WriteString(fmt.Sprintf("* %s\n", url))
	}
	return b.String()
}

// ----------------------------------------------------------------
// 既存関数のリファクタリング
// ----------------------------------------------------------------

// CombineContents は、成功した抽出結果の本文を効率的に結合します。
// 各コンテンツの前には、ソースURL情報が付加され、LLMが識別できるようにします。
// 最後の文書でなければ明確な区切り文字を追加します。
func CombineContents(results []extTypes.URLResult) string {
	var builder strings.Builder

	for i, res := range results {
		// URLを追記することで、LLMがどのソースのテキストであるかを識別できるようにする
		builder.WriteString(fmt.Sprintf("--- SOURCE URL %d: %s ---\n", i+1, res.URL))
		builder.WriteString(res.Content)

		// 最後の文書でなければ明確な区切り文字を追加
		if i < len(results)-1 {
			builder.WriteString(ContentSeparator)
		}
	}

	return builder.String()
}

// CleanAndStructureText は、MapReduce処理を実行し、最終的なクリーンアップと構造化を行います。
// sourceURLs が Reduce プロンプトに直接渡され、最終文書のメタ情報として使用されます。
func (c *Cleaner) CleanAndStructureText(ctx context.Context, combinedText string, apiKeyOverride string, sourceURLs []string) (string, error) {

	// 1. LLMクライアントの初期化
	var client *gemini.Client
	var err error

	if apiKeyOverride != "" {
		client, err = gemini.NewClient(ctx, gemini.Config{APIKey: apiKeyOverride})
	} else {
		client, err = gemini.NewClientFromEnv(ctx)
	}

	if err != nil {
		return "", fmt.Errorf("LLMクライアントの初期化に失敗しました。APIキー（--api-keyオプションまたは環境変数）が設定されているか確認してください: %w", err)
	}

	// 2. Mapフェーズのためのテキスト分割
	segments := segmentText(combinedText, MaxSegmentChars)
	log.Printf("テキストを %d 個のセグメントに分割しました。中間要約を開始します。", len(segments))

	// 3. Mapフェーズの実行（各セグメントの並列処理）
	intermediateSummaries, err := c.processSegmentsInParallel(ctx, client, segments)
	if err != nil {
		return "", fmt.Errorf("セグメント処理（Mapフェーズ）に失敗しました: %w", err)
	}

	// 4. Reduceフェーズの準備：中間要約の結合
	finalCombinedText := strings.Join(intermediateSummaries, "\n\n--- INTERMEDIATE SUMMARY END ---\n\n")

	// URLリストをMarkdown文字列に整形
	formattedURLs := formatURLsForTemplate(sourceURLs)

	// 5. Reduceフェーズ：最終的な統合と構造化のためのLLM呼び出し
	log.Println("中間要約の結合が完了しました。最終的な構造化（Reduceフェーズ）を開始します。")

	// ReduceTemplateData に整形済み文字列を含める
	reduceData := prompts.ReduceTemplateData{
		CombinedText: finalCombinedText,
		SourceURLs:   formattedURLs,
	}

	finalPrompt, err := c.reduceBuilder.BuildReduce(reduceData)
	if err != nil {
		return "", fmt.Errorf("最終 Reduce プロンプトの生成に失敗しました: %w", err)
	}

	finalResponse, err := client.GenerateContent(ctx, finalPrompt, "gemini-2.5-flash")
	if err != nil {
		return "", fmt.Errorf("LLM最終構造化処理（Reduceフェーズ）に失敗しました: %w", err)
	}

	// 6. 最終出力のクリーンアップ（マーカー削除）
	cleanedText := cleanFinalOutput(finalResponse.Text)

	return cleanedText, nil
}

// ----------------------------------------------------------------
// ヘルパー関数群
// ----------------------------------------------------------------

// cleanFinalOutput は、LLMの応答から <FINAL_START> と <FINAL_END> マーカーを削除し、
// マーカー間のクリーンなテキストを抽出します。
func cleanFinalOutput(llmResponse string) string {
	startIdx := strings.Index(llmResponse, FinalStartMarker)
	endIdx := strings.Index(llmResponse, FinalEndMarker)

	// マーカーが見つからない場合は、そのまま返す
	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		log.Println("⚠️ WARNING: LLM応答で <FINAL_START> または <FINAL_END> マーカーが見つかりませんでした。そのまま応答を返します。")
		return strings.TrimSpace(llmResponse)
	}

	// <FINAL_START> の直後から <FINAL_END> の直前までを抽出
	extracted := llmResponse[startIdx+len(FinalStartMarker) : endIdx]

	// 前後の空白文字を削除して返す
	return strings.TrimSpace(extracted)
}

// segmentText は、結合されたテキストを、安全な最大文字数を超えないように分割します。
// 段落の区切りを優先して分割し、文脈の欠落を最小限に抑えます。
func segmentText(text string, maxChars int) []string {
	var segments []string
	current := []rune(text)

	for len(current) > 0 {
		if len(current) <= maxChars {
			segments = append(segments, string(current))
			break
		}

		splitIndex := maxChars
		segmentCandidate := string(current[:maxChars])
		separatorFound := false
		separatorLen := 0

		// 1. ContentSeparator (最高優先度) を探す
		if lastSepIdx := strings.LastIndex(segmentCandidate, ContentSeparator); lastSepIdx != -1 && lastSepIdx > maxChars/2 {
			splitIndex = lastSepIdx
			separatorLen = len(ContentSeparator)
			separatorFound = true
		} else if lastSepIdx := strings.LastIndex(segmentCandidate, DefaultSeparator); lastSepIdx != -1 && lastSepIdx > maxChars/2 {
			// 2. ContentSeparator が見つからない場合、一般的な改行(\n\n)を探す
			splitIndex = lastSepIdx
			separatorLen = len(DefaultSeparator)
			separatorFound = true
		}

		// 区切り文字の種類に応じて、加算する長さを適切に選択
		if separatorFound {
			// 区切り文字の直後までを分割位置とする
			splitIndex += separatorLen
		} else {
			// 安全な区切りが見つからない場合は、そのまま最大文字数で切り、警告を出す
			log.Printf("⚠️ WARNING: 分割点で適切な区切りが見つかりませんでした。強制的に %d 文字で分割します。", maxChars)
			splitIndex = maxChars
		}

		segments = append(segments, string(current[:splitIndex]))
		current = current[splitIndex:]
	}

	return segments
}

// processSegmentsInParallel は Mapフェーズのログ出力を最小化
func (c *Cleaner) processSegmentsInParallel(ctx context.Context, client *gemini.Client, segments []string) ([]string, error) {
	var wg sync.WaitGroup
	resultsChan := make(chan struct {
		summary string
		err     error
	}, len(segments))

	for i, segment := range segments {
		wg.Add(1)

		go func(index int, seg string) {
			defer wg.Done()

			mapData := prompts.MapTemplateData{SegmentText: seg}
			prompt, err := c.mapBuilder.BuildMap(mapData)
			if err != nil {
				log.Printf("❌ ERROR: セグメント %d のプロンプト生成に失敗しました: %v", index+1, err)
				resultsChan <- struct {
					summary string
					err     error
				}{summary: "", err: fmt.Errorf("セグメント %d プロンプト生成失敗: %w", index+1, err)}
				return
			}

			// Mapプロンプトのプレビューは省略（冗長なため）

			response, err := client.GenerateContent(ctx, prompt, "gemini-2.5-flash")

			if err != nil {
				log.Printf("❌ ERROR: セグメント %d の処理に失敗しました: %v", index+1, err)
				resultsChan <- struct {
					summary string
					err     error
				}{summary: "", err: fmt.Errorf("セグメント %d 処理失敗: %w", index+1, err)}
				return
			}

			// Map Intermediate Summary の全内容は省略（冗長なため）

			resultsChan <- struct {
				summary string
				err     error
			}{summary: response.Text, err: nil}
		}(i, segment)
	}

	wg.Wait()
	close(resultsChan)

	var summaries []string
	for res := range resultsChan {
		if res.err != nil {
			return nil, res.err
		}
		summaries = append(summaries, res.summary)
	}

	return summaries, nil
}

// ExtractURLs は、成功した結果からURLのリストのみを抽出します。
func ExtractURLs(results []extTypes.URLResult) []string {
	urls := make([]string, len(results))
	for i, res := range results {
		urls[i] = res.URL
	}
	return urls
}
