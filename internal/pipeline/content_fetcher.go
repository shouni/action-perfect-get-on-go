package pipeline

import (
	"context"
	"fmt"
	"log/slog"

	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
	"github.com/shouni/web-text-pipe-go/pkg/runner"
)

// ----------------------------------------------------------------
// 具象実装
// ----------------------------------------------------------------

// WebContentFetcherImpl は ContentFetcher インターフェースの具象実装です。
// その唯一の責務は、スクレイピング実行者に処理を委譲することです。
type WebContentFetcherImpl struct {
	scraperRunner ScraperRunner
}

// NewWebContentFetcherImpl は WebContentFetcherImpl の新しいインスタンスを作成します。
// ここで注入されるのは、リトライ機能を持つ runner.ReliableScraper です。
func NewWebContentFetcherImpl(scraperRunner ScraperRunner) *WebContentFetcherImpl {
	return &WebContentFetcherImpl{
		scraperRunner: scraperRunner,
	}
}

// Fetch は、URLリストに対してスクレイピング処理を実行者に委譲します。
// リトライ、遅延、分類のロジックは ScraperRunner (ReliableScraper) 側で完結します。
func (w *WebContentFetcherImpl) Fetch(ctx context.Context, opts CmdOptions, urls []string) ([]extTypes.URLResult, error) {
	// ログメッセージの参照名を修正
	slog.Info("Webコンテンツの抽出処理を ScraperRunner に委譲します。", slog.Int("total_urls", len(urls)))

	// 注入された ScraperRunner (ReliableScraper) が、並列実行とリトライの両方を処理します。
	successfulResults := w.scraperRunner.ScrapeInParallel(ctx, urls)

	if len(successfulResults) == 0 {
		return nil, fmt.Errorf("処理可能なWebコンテンツを一件も取得できませんでした。URLを確認してください。")
	}

	return successfulResults, nil
}

// ----------------------------------------------------------------
// 型アサーションチェック
// ----------------------------------------------------------------

// runner.ReliableScraper がこのパッケージで定義された ScraperRunner インターフェースを満たしているか確認します。
var _ ScraperRunner = (*runner.ReliableScraper)(nil)
