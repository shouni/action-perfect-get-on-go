package cleaner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"action-perfect-get-on-go/pkg/types"

	gemini "github.com/shouni/go-ai-client/pkg/ai/gemini"
)

// ContentSeparator は、結合された複数の文書間を区切るための明確な区切り文字です。
const ContentSeparator = "\n\n--- DOCUMENT END ---\n\n"

// DefaultSeparator は、一般的な段落区切りに使用される標準的な区切り文字です。
const DefaultSeparator = "\n\n"

// MaxSegmentChars は、MapフェーズでLLMに一度に渡す安全な最大文字数。
// トークン制限に十分なマージンを持たせた値です。
const MaxSegmentChars = 400000

// CombineContents は、成功した抽出結果の本文を効率的に結合します。
func CombineContents(results []types.URLResult) string {
	var builder strings.Builder

	for i, res := range results {
		builder.WriteString(fmt.Sprintf("--- SOURCE URL %d: %s ---\n", i+1, res.URL))
		builder.WriteString(res.Content)

		if i < len(results)-1 {
			builder.WriteString(ContentSeparator)
		}
	}

	return builder.String()
}

// CleanAndStructureText は、結合された巨大なテキストをセグメントに分割し、
// 並列処理（MapReduceパターン）で最終的な構造化テキストを生成します。
func CleanAndStructureText(ctx context.Context, combinedText string, apiKeyOverride string) (string, error) {

	// 1. LLMクライアントの初期化
	var client *gemini.Client
	var err error

	if apiKeyOverride != "" {
		client, err = gemini.NewClient(ctx, gemini.Config{APIKey: apiKeyOverride})
	} else {
		client, err = gemini.NewClientFromEnv(ctx)
	}

	if err != nil {
		// ⭐ 修正: エラーメッセージを明確化
		return "", fmt.Errorf("LLMクライアントの初期化に失敗しました。APIキー（--api-keyオプションまたは環境変数）が設定されているか確認してください: %w", err)
	}

	// 2. Mapフェーズのためのテキスト分割
	segments := segmentText(combinedText, MaxSegmentChars)
	log.Printf("テキストを %d 個のセグメントに分割しました。中間要約を開始します。", len(segments))

	// 3. Mapフェーズの実行（各セグメントの並列処理）
	intermediateSummaries, err := processSegmentsInParallel(ctx, client, segments)
	if err != nil {
		return "", fmt.Errorf("セグメント処理（Mapフェーズ）に失敗しました: %w", err)
	}

	// 4. Reduceフェーズの準備：中間要約の結合
	finalCombinedText := strings.Join(intermediateSummaries, "\n\n--- INTERMEDIATE SUMMARY END ---\n\n")

	// 5. Reduceフェーズ：最終的な統合と構造化のためのLLM呼び出し
	// ⭐ 修正: ログメッセージを結合完了後に移動
	log.Println("中間要約の結合が完了しました。最終的な構造化（Reduceフェーズ）を開始します。")

	finalPrompt := buildFinalReducePrompt(finalCombinedText)
	finalResponse, err := client.GenerateContent(ctx, finalPrompt, "gemini-2.5-flash")
	if err != nil {
		return "", fmt.Errorf("LLM最終構造化処理（Reduceフェーズ）に失敗しました: %w", err)
	}

	return finalResponse.Text, nil
}

// ----------------------------------------------------------------
// ヘルパー関数群
// ----------------------------------------------------------------

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

		// 最大サイズに近い位置で最後の区切り（ContentSeparatorなど）を探す
		splitIndex := maxChars
		segmentCandidate := string(current[:maxChars]) // Runeからstringに変換
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

		// ⭐ 修正: splitIndex の計算ロジックを修正
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

// processSegmentsInParallel は、各セグメントをGoルーチンで並列にLLM処理にかけます（Mapフェーズ）。
func processSegmentsInParallel(ctx context.Context, client *gemini.Client, segments []string) ([]string, error) {
	var wg sync.WaitGroup
	// 結果とエラーを収集するためのチャネル
	resultsChan := make(chan struct {
		summary string
		err     error
	}, len(segments))

	for i, segment := range segments {
		wg.Add(1)

		go func(index int, seg string) {
			defer wg.Done()

			prompt := buildSegmentMapPrompt(seg)
			response, err := client.GenerateContent(ctx, prompt, "gemini-2.5-flash")

			if err != nil {
				log.Printf("❌ ERROR: セグメント %d の処理に失敗しました: %v", index+1, err)
				resultsChan <- struct {
					summary string
					err     error
				}{summary: "", err: fmt.Errorf("segment %d processing failed: %w", index+1, err)}
				return
			}

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

// buildSegmentMapPrompt は、個々のセグメントを要約するためのプロンプトを生成します。
func buildSegmentMapPrompt(segmentText string) string {
	var sb strings.Builder
	sb.WriteString("Summarize and clean up the information in the following text segment in Markdown format, eliminating redundant expressions.\n")
	sb.WriteString("Remove any duplicate information within this segment.\n")
	sb.WriteString("【NOTE】This is an intermediate process. Ensure all information is preserved, and provide logical headers to facilitate overall structuring in subsequent processes.\n\n")
	sb.WriteString("--- Input Segment ---\n")
	sb.WriteString(segmentText)
	sb.WriteString("\n------------------------\n\n")
	sb.WriteString("✅ Output the cleaned Markdown text:")

	return sb.String()
}

// buildFinalReducePrompt は、すべての中間要約を統合するためのプロンプトを生成します。
func buildFinalReducePrompt(finalCombinedText string) string {
	var sb strings.Builder
	sb.WriteString("The following text is a collection of intermediate summaries processed segment by segment from information extracted from multiple web pages.\n")
	sb.WriteString("Your task is to **perfectly integrate** this information and create a **final structured document**.\n\n")
	sb.WriteString("--- Final Processing Instructions ---\n")
	sb.WriteString("1. **Final Deduplication**: Identify any remaining duplicate information between intermediate summaries and integrate them completely, retaining only the most complete information.\n")
	sb.WriteString("2. **Logical Structuring**: Reconstruct the entire content as a single topic, applying the most logical and easy-to-understand hierarchical structure (Markdown headers).\n")
	sb.WriteString("3. **Noise Removal**: Remove any unnecessary messages or noise remaining from the intermediate processing.\n")
	sb.WriteString("4. **Output Format**: The output must be clean Markdown text only, without any additional explanations or comments.\n")
	sb.WriteString("----------------\n\n")
	sb.WriteString("--- Combined Intermediate Summaries ---\n")
	sb.WriteString(finalCombinedText)
	sb.WriteString("\n------------------------\n\n")
	sb.WriteString("✅ Output the final structured document:")

	return sb.String()
}
