package cleaner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync" // ⭐ 追加: 並列処理のため

	"action-perfect-get-on-go/pkg/types"

	gemini "github.com/shouni/go-ai-client/pkg/ai/gemini"
)

// ContentSeparator は、結合された複数の文書間を区切るための明確な区切り文字です。
const ContentSeparator = "\n\n--- DOCUMENT END ---\n\n"

// MaxSegmentChars は、MapフェーズでLLMに一度に渡す安全な最大文字数。
// この値は、最終的な構造化プロセスで情報が欠落しないよう、トークン制限（200万）に十分なマージンを持たせた値です。
const MaxSegmentChars = 400000

// CombineContents は、成功した抽出結果の本文を効率的に結合します。
func CombineContents(results []types.URLResult) string {
	var builder strings.Builder

	for i, res := range results {
		// URLを追記することで、LLMがどのソースのテキストであるかを識別できるようにする
		builder.WriteString(fmt.Sprintf("--- SOURCE URL %d: %s ---\n", i+1, res.URL))
		builder.WriteString(res.Content)

		// 最後の文書でなければ区切り文字を追加
		if i < len(results)-1 {
			builder.WriteString(ContentSeparator)
		}
	}

	return builder.String()
}

// CleanAndStructureText は、結合された巨大なテキストをセグメントに分割し、
// 並列処理（MapReduceパターン）で最終的な構造化テキストを生成します。
func CleanAndStructureText(ctx context.Context, combinedText string, apiKeyOverride string) (string, error) {

	// 1. LLMクライアントの初期化 (APIキーの柔軟性向上)
	var client *gemini.Client
	var err error

	if apiKeyOverride != "" {
		client, err = gemini.NewClient(ctx, gemini.Config{APIKey: apiKeyOverride})
	} else {
		client, err = gemini.NewClientFromEnv(ctx)
	}

	if err != nil {
		return "", fmt.Errorf("LLMクライアントの初期化に失敗しました。APIキーを確認してください: %w", err)
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
	log.Println("中間要約を結合し、最終的な構造化（Reduceフェーズ）を開始します。")

	finalPrompt := buildFinalReducePrompt(finalCombinedText)
	// モデルは構造化に適したものを指定
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

		// 最大サイズに近い位置で最後の改行（段落区切り）を探す
		splitIndex := maxChars // デフォルトは最大文字数で強制分割
		segmentCandidate := string(current[:maxChars])

		// 最後の大きな区切り文字（ContentSeparatorの区切りなど）を探す
		lastSeparatorLen := 0
		lastSeparatorIndex := strings.LastIndex(segmentCandidate, ContentSeparator)
		if lastSeparatorIndex != -1 {
			lastSeparatorLen = len(ContentSeparator)
		} else {
			// ContentSeparator が見つからない場合、一般的な改行(\n\n)を探す
			lastSeparatorIndex = strings.LastIndex(segmentCandidate, "\n\n")
			if lastSeparatorIndex != -1 {
				lastSeparatorLen = len("\n\n")
			}
		}

		if lastSeparatorIndex != -1 && lastSeparatorIndex > maxChars/2 {
			// 分割位置を安全な区切り文字の直後に設定
			splitIndex = lastSeparatorIndex + lastSeparatorLen
		} else {
			// 安全な区切りが見つからない場合は、そのまま最大文字数で切り、警告を出す
			log.Printf("⚠️ WARNING: 分割点で適切な区切りが見つかりませんでした。強制的に %d 文字で分割します。", maxChars)
			// splitIndex は maxChars のまま
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

			// セグメント処理用のプロンプトを生成
			prompt := buildSegmentMapPrompt(seg)

			// LLM APIを呼び出し
			response, err := client.GenerateContent(ctx, prompt, "gemini-2.5-flash")

			if err != nil {
				log.Printf("❌ ERROR: セグメント %d の処理に失敗しました: %v", index+1, err)
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
		}(i, segment)
	}

	wg.Wait()
	close(resultsChan)

	var summaries []string
	for res := range resultsChan {
		if res.err != nil {
			// 一つでもセグメント処理で失敗したら、全体を失敗とする
			return nil, res.err
		}
		summaries = append(summaries, res.summary)
	}

	return summaries, nil
}

// buildSegmentMapPrompt は、個々のセグメントを要約するためのプロンプトを生成します。
func buildSegmentMapPrompt(segmentText string) string {
	var sb strings.Builder
	sb.WriteString("以下のテキストセグメントに含まれる情報を、冗長な表現を排除してMarkdown形式でクリーンアップし、構造化してください。\n")
	sb.WriteString("このセグメント内の重複情報は排除してください。\n")
	sb.WriteString("【注意】これは中間処理です。情報を見落とさず、後続の処理で全体の構造化ができるよう、論理的なヘッダーを付けて出力してください。\n\n")
	sb.WriteString("--- 入力セグメント ---\n")
	sb.WriteString(segmentText)
	sb.WriteString("\n------------------------\n\n")
	sb.WriteString("✅ クリーンアップされたMarkdownテキストを出力してください:")

	return sb.String()
}

// buildFinalReducePrompt は、すべての中間要約を統合するためのプロンプトを生成します。
func buildFinalReducePrompt(finalCombinedText string) string {
	var sb strings.Builder
	sb.WriteString("以下のテキストは、複数のウェブページから抽出された情報をセグメントごとに処理した中間要約の集合体です。\n")
	sb.WriteString("あなたのタスクは、これらの情報を**完璧に統合**し、**最終的な構造化文書**を作成することです。\n\n")
	sb.WriteString("--- 最終処理指示 ---\n")
	sb.WriteString("1. **最終的な重複排除**: 中間要約間に残っている重複情報を識別し、最も完全な情報のみを残して、完全に統合してください。\n")
	sb.WriteString("2. **論理的な構造化**: 全体を一つのトピックとして再構成し、最も論理的で分かりやすい階層構造（Markdownヘッダー）にしてください。\n")
	sb.WriteString("3. **ノイズ除去**: 中間処理時に残った不要なメッセージやノイズはすべて削除してください。\n")
	sb.WriteString("4. **出力形式**: 出力は、Markdown形式のクリーンなテキストのみとし、追加の説明や感想は一切含めないでください。\n")
	sb.WriteString("----------------\n\n")
	sb.WriteString("--- 中間要約結合テキスト ---\n")
	sb.WriteString(finalCombinedText)
	sb.WriteString("\n------------------------\n\n")
	sb.WriteString("✅ 最終的な構造化文書を出力してください:")

	return sb.String()
}
