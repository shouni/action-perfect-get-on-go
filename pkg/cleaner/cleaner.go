package cleaner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/shouni/action-perfect-get-on-go/prompts"
	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
)

// DefaultSeparator は、一般的な段落区切りに使用される標準的な区切り文字です。
const DefaultSeparator = "\n\n"

// MaxSegmentChars は、MapフェーズでLLMに一度に渡す安全な最大文字数。
// トークン制限に十分なマージンを持たせた値です。
const MaxSegmentChars = 400000

const DefaultMaxMapConcurrency = 5

// DefaultLLMRateLimit 200msごとに1リクエストを許可 = 1秒あたり最大5リクエスト
const DefaultLLMRateLimit = 200 * time.Millisecond

// ----------------------------------------------------------------
// LLM応答マーカーの定数
// ----------------------------------------------------------------
const (
	// FinalStartMarker は Reduce プロンプトで定義された最終出力開始マーカーです。
	FinalStartMarker = "<FINAL_START>"
	FinalEndMarker   = "<FINAL_END>"
)

// ----------------------------------------------------------------
// 内部ヘルパ構造体: セグメントとURLを紐づける
// ----------------------------------------------------------------

// Segment は、LLMに渡すテキストと、それが由来する元のURLを保持します。
type Segment struct {
	Text string
	URL  string
}

// ----------------------------------------------------------------
// Cleaner 構造体とコンストラクタの導入
// ----------------------------------------------------------------

// Cleaner はコンテンツのクリーンアップと要約を担当します。
type Cleaner struct {
	mapBuilder    *prompts.PromptBuilder
	reduceBuilder *prompts.PromptBuilder
	concurrency   int
}

// NewCleaner は新しい Cleaner インスタンスを作成し、PromptBuilderを一度だけ初期化します。
// concurrency: Mapフェーズで同時に実行するLLMリクエストの最大数
func NewCleaner(concurrency int) (*Cleaner, error) {
	// テンプレートパースはここで一度だけ行い、失敗した場合はエラーを返す
	mapBuilder := prompts.NewMapPromptBuilder()
	if err := mapBuilder.Err(); err != nil {
		return nil, fmt.Errorf("failed to initialize map prompt builder: %w", err)
	}
	reduceBuilder := prompts.NewReducePromptBuilder()
	if err := reduceBuilder.Err(); err != nil {
		return nil, fmt.Errorf("failed to initialize reduce prompt builder: %w", err)
	}

	// 最小1並列は保証
	if concurrency < 1 {
		log.Printf("⚠️ WARNING: concurrencyが1未満に設定されています (%d)。1に設定します。", concurrency)
		concurrency = 1
	}

	return &Cleaner{
		mapBuilder:    mapBuilder,
		reduceBuilder: reduceBuilder,
		concurrency:   concurrency,
	}, nil
}

// ----------------------------------------------------------------
// 既存関数の大幅な変更と削除
// ----------------------------------------------------------------

// CleanAndStructureText は、MapReduce処理を実行し、最終的なクリーンアップと構造化を行います。
func (c *Cleaner) CleanAndStructureText(ctx context.Context, results []extTypes.URLResult, apiKeyOverride string) (string, error) {
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

	// 2. MapフェーズのためのURL単位のテキスト分割
	var allSegments []Segment
	for _, res := range results {
		// URLResultのContentを個別にセグメント分割
		segments := segmentText(res.Content, MaxSegmentChars)
		for _, segText := range segments {
			// segmentTextが強制分割した場合の検出ロジックはsegmentText内に移動済み
			allSegments = append(allSegments, Segment{Text: segText, URL: res.URL})
		}
	}

	log.Printf("合計 %d 個のセグメント（URL単位で分割）に分割しました。中間要約を開始します。", len(allSegments))

	// 3. Mapフェーズの実行（各セグメントの並列処理）
	intermediateSummaries, err := c.processSegmentsInParallel(ctx, client, allSegments)
	if err != nil {
		return "", fmt.Errorf("セグメント処理（Mapフェーズ）に失敗しました: %w", err)
	}

	// 4. Reduceフェーズの準備：中間要約の結合
	finalCombinedText := strings.Join(intermediateSummaries, "\n\n--- INTERMEDIATE SUMMARY END ---\n\n")

	// 5. Reduceフェーズ：最終的な統合と構造化のためのLLM呼び出し
	log.Println("中間要約の結合が完了しました。最終的な構造化（Reduceフェーズ）を開始します。")

	reduceData := prompts.ReduceTemplateData{
		CombinedText: finalCombinedText,
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

		// 1. 一般的な改行(\n\n)を探す
		if lastSepIdx := strings.LastIndex(segmentCandidate, DefaultSeparator); lastSepIdx != -1 && lastSepIdx > maxChars/2 {
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
func (c *Cleaner) processSegmentsInParallel(ctx context.Context, client *gemini.Client, allSegments []Segment) ([]string, error) {
	var wg sync.WaitGroup
	resultsChan := make(chan struct {
		summary string
		err     error
	}, len(allSegments))

	sem := make(chan struct{}, c.concurrency) // 並列処理セマフォ

	// time.NewTicker を使用し、deferで確実に停止する
	// LLM APIのコール間隔を制御するレートリミッター
	ticker := time.NewTicker(DefaultLLMRateLimit)
	defer ticker.Stop() // 非常に重要: 関数終了時にタイマーのGoroutineリークを防ぐ
	rateLimiter := ticker.C

	for i, seg := range allSegments {
		sem <- struct{}{}

		wg.Add(1)

		go func(index int, s Segment) {
			defer func() { <-sem }()
			defer wg.Done()

			// レートリミットとコンテキストキャンセルを select で同時に監視
			select {
			case <-rateLimiter:
				// レートリミット間隔が経過し、リクエストが許可された
			case <-ctx.Done():
				// コンテキストがキャンセルされた場合、このGoroutineを終了
				log.Printf("INFO: セグメント %d の処理がコンテキストキャンセルにより中断されました (URL: %s)", index+1, s.URL)
				return
			}

			mapData := prompts.MapTemplateData{
				SegmentText: s.Text,
				SourceURL:   s.URL,
			}
			prompt, err := c.mapBuilder.BuildMap(mapData)
			if err != nil {
				log.Printf("❌ ERROR: セグメント %d のプロンプト生成に失敗しました: %v (URL: %s)", index+1, err, s.URL)
				resultsChan <- struct {
					summary string
					err     error
				}{summary: "", err: fmt.Errorf("セグメント %d プロンプト生成失敗: %w", index+1, err)}
				return
			}

			response, err := client.GenerateContent(ctx, prompt, "gemini-2.5-flash")

			if err != nil {
				// エラーログのコメントアウトを解除
				log.Printf("❌ ERROR: セグメント %d の処理に失敗しました: %v (URL: %s)", index+1, err, s.URL)
				resultsChan <- struct {
					summary string
					err     error
				}{summary: "", err: fmt.Errorf("セグメント %d 処理失敗: %w", index+1, err)}
				return
			}

			resultsChan <- struct {
				summary string
				err     error
			}{summary: response.Text, err: nil}
		}(i, seg)
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
