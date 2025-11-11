package pipeline

import (
	"context"
	"fmt"
	"log/slog" // log/slog に変更
)

// ----------------------------------------------------------------
// 定数定義 (Appから移動)
// ----------------------------------------------------------------

const (
	PhaseURLs    = "URL生成フェーズ"
	PhaseContent = "コンテンツ取得フェーズ"
	PhaseCleanUp = "AIクリーンアップと出力フェーズ"
)

// Execute はアプリケーションの主要な処理フローを、注入されたステージを通じて実行します。
// (元の App.Execute のロジックを再構成)
func (p *Pipeline) Execute(ctx context.Context) error {
	// 1. URL生成ステージ
	urls, err := p.URLGen.Generate(ctx, p.Options)
	if err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", PhaseURLs, err)
	}
	slog.Info("Perfect Get On 処理を開始します。", slog.Int("target_urls", len(urls)))

	// 2. コンテンツ取得ステージ
	successfulResults, err := p.Fetcher.Fetch(ctx, p.Options, urls)
	if err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", PhaseContent, err)
	}

	// 3. AIクリーンアップと出力ステージ
	if err := p.OutputGen.Generate(ctx, p.Options, successfulResults); err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", PhaseCleanUp, err)
	}
	slog.Info("処理が正常に完了しました。")
	return nil
}
