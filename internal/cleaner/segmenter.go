package cleaner

import (
	"log/slog"
	"strings"
)

// segmentText は、結合されたテキストを、安全な最大文字数を超えないように分割します。
// これは純粋な関数であり、外部の状態に依存しません。
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
			// 修正: log.Printf -> slog.Warn (構造化ロギング)
			slog.Warn("⚠️ 分割点で適切な区切りが見つかりませんでした。強制的に分割します。",
				slog.Int("forced_chars", maxChars))
			splitIndex = maxChars
		}

		segments = append(segments, string(current[:splitIndex]))
		current = current[splitIndex:]
	}

	return segments
}

// cleanFinalOutput は、LLMの応答から <FINAL_START> と <FINAL_END> マーカーを削除し、
// マーカー間のクリーンなテキストを抽出します。
func cleanFinalOutput(llmResponse string) string {
	startIdx := strings.Index(llmResponse, FinalStartMarker)
	endIdx := strings.Index(llmResponse, FinalEndMarker)

	// マーカーが見つからない場合は、そのまま返す
	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		slog.Warn("⚠️ LLM応答で最終出力マーカーが見つかりませんでした。そのまま応答を返します。",
			slog.String("marker_start", FinalStartMarker),
			slog.String("marker_end", FinalEndMarker))
		return strings.TrimSpace(llmResponse)
	}

	// <FINAL_START> の直後から <FINAL_END> の直前までを抽出
	extracted := llmResponse[startIdx+len(FinalStartMarker) : endIdx]

	// 前後の空白文字を削除して返す
	return strings.TrimSpace(extracted)
}
