package scraper

import (
	"context"
	"fmt"
	"sync"
	"time"

	"action-perfect-get-on-go/pkg/types"
	"github.com/shouni/go-web-exact/pkg/httpclient"
	webextractor "github.com/shouni/go-web-exact/pkg/web"
)

// Scraper はWebコンテンツの抽出機能を提供するインターフェースです。
type Scraper interface {
	ScrapeInParallel(ctx context.Context, urls []string) []types.URLResult
}

// ParallelScraper は Scraper インターフェースを実装する具体的な構造体です。
type ParallelScraper struct {
	// 抽出処理を webextractor.Extractor に委譲
	extractor *webextractor.Extractor
}

// NewParallelScraper は ParallelScraper を初期化します。
// timeout は HTTPクライアントのタイムアウト時間です。
func NewParallelScraper(timeout time.Duration) *ParallelScraper {
	// 1. 堅牢な HTTP クライアントを初期化 (リトライ、エラーハンドリング内蔵)
	httpClient := httpclient.New(timeout)

	// 2. 抽出ロジックを管理する Extractor を初期化 (httpclient が Fetcher インターフェースを満たすと仮定)
	extractor := webextractor.NewExtractor(httpClient)

	return &ParallelScraper{
		extractor: extractor,
	}
}

// ScrapeInParallel は Scraper インターフェースのメソッドを実装します。
// Goルーチンとチャネルを用いて、複数のURLから並列にコンテンツを抽出します。
func (s *ParallelScraper) ScrapeInParallel(ctx context.Context, urls []string) []types.URLResult {
	var wg sync.WaitGroup

	// 処理結果を収集するためのバッファ付きチャネル
	resultsChan := make(chan types.URLResult, len(urls))

	for _, url := range urls {
		wg.Add(1)

		// Goルーチンを起動し、並列に処理を実行
		go func(u string) {
			defer wg.Done()

			// Context がキャンセルされていないか確認 (全体の LLM タイムアウト等)
			select {
			case <-ctx.Done():
				resultsChan <- types.URLResult{
					URL:   u,
					Error: ctx.Err(),
				}
				return
			default:
				// 処理を続行
			}

			// 実際の抽出処理を実行 (webextractor に委譲)
			content, err := s.extractContent(u, ctx)

			resultsChan <- types.URLResult{
				URL:     u,
				Content: content,
				Error:   err,
			}
		}(url)
	}

	// すべてのGoルーチンが完了するのを待つ
	wg.Wait()
	close(resultsChan)

	// チャネルからすべての結果を収集
	var finalResults []types.URLResult
	for res := range resultsChan {
		finalResults = append(finalResults, res)
	}

	return finalResults
}

// ----------------------------------------------------------------
// メインコンテンツ抽出のヘルパーメソッド (Extractorへの委譲)
// ----------------------------------------------------------------

// extractContent は単一のURLからメインコンテンツを抽出します。
func (s *ParallelScraper) extractContent(url string, ctx context.Context) (string, error) {
	// Extractor の FetchAndExtractText を呼び出し、抽出・整形ロジックをすべて委譲します。
	content, hasBodyFound, err := s.extractor.FetchAndExtractText(url, ctx)
	if err != nil {
		// httpclient/extractor 内部でリトライ、エラーハンドリングが行われています。
		return "", fmt.Errorf("コンテンツの抽出に失敗しました: %w", err)
	}

	// 抽出ロジックの判定結果 (本文が見つからなかった場合)
	if content == "" || !hasBodyFound {
		// Extractor のロジックによってはタイトルのみ取得の場合があるため、厳密にチェック
		return "", fmt.Errorf("URL %s から有効な本文を抽出できませんでした", url)
	}

	return content, nil
}
