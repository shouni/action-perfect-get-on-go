package cleaner

import (
	"context"

	"action-perfect-get-on-go/pkg/scraper"
)

// CombineContents は抽出結果から本文を結合します。（現在はスタブ）
// scraper.URLResult を正しく参照できるように修正
func CombineContents(results []scraper.URLResult) string {
	// 実際の結合処理はここで実装されますが、ビルドを通すために空文字列を返します。
	return ""
}

// CleanAndStructureText は結合されたテキストをLLMで処理します。（現在はスタブ）
func CleanAndStructureText(ctx context.Context, combinedText string) (string, error) {
	// 実際のLLM処理はここで実装されますが、ビルドを通すために空文字列とnilエラーを返します。
	return "Placeholder: LLM output will go here.", nil
}
