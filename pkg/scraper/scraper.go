// pkg/scraper/scraper.go

package scraper

import (
	"context"
	"time"

	"action-perfect-get-on-go/pkg/types"

	httpclient "github.com/shouni/go-web-exact/pkg/httpclient"
)

// Scraper はWebコンテンツの抽出機能を提供するインターフェースです。
type Scraper interface {
	ScrapeInParallel(ctx context.Context, urls []string) []types.URLResult
}

// ParallelScraper は Scraper インターフェースを実装する具体的な構造体です。
type ParallelScraper struct {
	webClient *httpclient.Client
}

// NewParallelScraper は ParallelScraper を初期化します。
func NewParallelScraper(timeout time.Duration) *ParallelScraper {
	return &ParallelScraper{
		webClient: httpclient.New(timeout),
	}
}

// ScrapeInParallel は Scraper インターフェースのメソッドを実装します。
func (s *ParallelScraper) ScrapeInParallel(ctx context.Context, urls []string) []types.URLResult {
	// TODO: 実装ロジック

	// ビルドを通すためのスタブ処理
	return []types.URLResult{}
}
