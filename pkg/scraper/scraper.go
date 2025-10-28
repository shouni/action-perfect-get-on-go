package scraper

import (
	"context"
	"fmt"
	"sync"

	"github.com/shouni/action-perfect-get-on-go/pkg/types"
	webextractor "github.com/shouni/go-web-exact/pkg/web"
)

// Scraper はWebコンテンツの抽出機能を提供するインターフェースです。
type Scraper interface {
	ScrapeInParallel(ctx context.Context, urls []string) []types.URLResult
}

// ParallelScraper は Scraper インターフェースを実装する並列処理構造体です。
// httpclient を直接知る必要はなく、webextractor.Extractor に依存します。
type ParallelScraper struct {
	extractor *webextractor.Extractor
}

// NewParallelScraper は ParallelScraper を初期化します。
// 依存性として、既に初期化された *webextractor.Extractor を受け取ります（DI）。
// これにより、テスト時にモックの Extractor を注入できるようになります。
// time.Duration の timeout は、クライアントの初期化時に外部で設定される想定です。
func NewParallelScraper(extractor *webextractor.Extractor) *ParallelScraper {
	// NewClient は削除され、ここでは依存性の注入のみを行う
	return &ParallelScraper{
		extractor: extractor,
	}
}

// ScrapeInParallel は Scraper インターフェースのメソッドを実装します。
func (s *ParallelScraper) ScrapeInParallel(ctx context.Context, urls []string) []types.URLResult {
	var wg sync.WaitGroup
	resultsChan := make(chan types.URLResult, len(urls))

	for _, url := range urls {
		wg.Add(1)

		go func(u string) {
			defer wg.Done()
			select {
			case <-ctx.Done():
				resultsChan <- types.URLResult{
					URL:   u,
					Error: ctx.Err(),
				}
				return
			default:
			}

			// 修正: s.client.ExtractContent を呼び出す代わりに、
			// 内部の Extractor の FetchAndExtractText を直接呼び出します。
			content, hasBodyFound, err := s.extractor.FetchAndExtractText(u, ctx)

			var extractErr error
			if err != nil {
				extractErr = fmt.Errorf("コンテンツの抽出に失敗しました: %w", err)
			} else if content == "" || !hasBodyFound {
				// 抽出ロジックの判定結果 (本文が見つからなかった場合)
				extractErr = fmt.Errorf("URL %s から有効な本文を抽出できませんでした", u)
			}

			resultsChan <- types.URLResult{
				URL:     u,
				Content: content,
				Error:   extractErr,
			}
		}(url)
	}

	wg.Wait()
	close(resultsChan)

	var finalResults []types.URLResult
	for res := range resultsChan {
		finalResults = append(finalResults, res)
	}

	return finalResults
}
