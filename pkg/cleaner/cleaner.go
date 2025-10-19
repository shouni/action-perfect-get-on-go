package cleaner

import (
	"context"

	"action-perfect-get-on-go/pkg/types" // ⭐ 修正点: pkg/scraper への依存を pkg/types に変更
)

// CombineContents は抽出結果から本文を結合します。（現在はスタブ）
func CombineContents(results []types.URLResult) string { // ⭐ 修正点: types.URLResult を使用
	// 実際の結合処理はここで実装されますが、ビルドを通すために空文字列を返します。
	return ""
}

// CleanAndStructureText は結合されたテキストをLLMで処理します。（現在はスタブ）
func CleanAndStructureText(ctx context.Context, combinedText string) (string, error) {
	// 実際のLLM処理はここで実装されますが、ビルドを通すために空文字列とnilエラーを返します。
	// TODO: go-ai-client を利用したロジックを実装
	return "Placeholder: LLM output will go here.", nil
}
