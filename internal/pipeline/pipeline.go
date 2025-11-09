package pipeline

import (
	"context"
	"fmt"
	"log/slog" // log/slog に変更
	"time"
)

// ----------------------------------------------------------------
// 定数定義 (Appから移動)
// ----------------------------------------------------------------

const (
	// InitialScrapeDelay は並列スクレイピング後の無条件待機時間です。
	InitialScrapeDelay = 2 * time.Second
	RetryScrapeDelay   = 5 * time.Second

	PhaseURLs    = "URL生成フェーズ"
	PhaseContent = "コンテンツ取得フェーズ"
	PhaseCleanUp = "AIクリーンアップと出力フェーズ"

	DefaultHTTPMaxRetries = 2
)

// Execute はアプリケーションの主要な処理フローを、注入されたステージを通じて実行します。
// (元の App.Execute のロジックを再構成)
func (p *Pipeline) Execute(ctx context.Context) error {
	// 1. URL生成ステージ
	urls, err := p.URLGen.Generate(ctx, p.Options)
	if err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", PhaseURLs, err)
	}
	// [行番号: 30 修正] log.Printf -> slog.Info (構造化)
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

	// [行番号: 42 修正] log.Println -> slog.Info
	slog.Info("処理が正常に完了しました。")
	return nil
}
