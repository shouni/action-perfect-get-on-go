package scraper

import (
	"context"
	"time"

	httpclient "github.com/shouni/go-web-exact/pkg/httpclient"
)

// URLResult は個々のURLの抽出結果を格納する構造体
type URLResult struct {
	URL     string
	Content string // 抽出された本文
	Error   error
}

// Scraper はWebコンテンツの抽出機能を提供するインターフェースです。
type Scraper interface {
	ScrapeInParallel(ctx context.Context, urls []string) []URLResult
}

// ParallelScraper は Scraper インターフェースを実装する具体的な構造体です。
type ParallelScraper struct {
	webClient *httpclient.Client
}

// NewParallelScraper は ParallelScraper を初期化します。
func NewParallelScraper() *ParallelScraper {
	// 堅牢性を確保するため、タイムアウトを設定 (15秒は仮)
	return &ParallelScraper{
		webClient: httpclient.New(time.Second * 15),
	}
}

// ScrapeInParallel は Scraper インターフェースのメソッドを実装します。
func (s *ParallelScraper) ScrapeInParallel(ctx context.Context, urls []string) []URLResult {
	// TODO: 実装ロジック

	// ビルドを通すためのスタブ処理
	return []URLResult{}
}
