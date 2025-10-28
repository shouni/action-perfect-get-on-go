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
type ParallelScraper struct {
	extractor *webextractor.Extractor
	// 最大並列数を保持するフィールド
	maxConcurrency int
}

// NewParallelScraper は ParallelScraper を初期化します。
// 依存性として Extractor と、最大同時実行数を受け取ります。
func NewParallelScraper(extractor *webextractor.Extractor, maxConcurrency int) *ParallelScraper {
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // 安全なデフォルト値
	}
	return &ParallelScraper{
		extractor:      extractor,
		maxConcurrency: maxConcurrency,
	}
}

// ScrapeInParallel は Scraper インターフェースのメソッドを実装します。
func (s *ParallelScraper) ScrapeInParallel(ctx context.Context, urls []string) []types.URLResult {
	var wg sync.WaitGroup
	resultsChan := make(chan types.URLResult, len(urls))

	// バッファ付きチャネルをセマフォとして使用し、同時実行数を制限する
	semaphore := make(chan struct{}, s.maxConcurrency)

	for _, url := range urls {
		wg.Add(1)

		// リソース（スロット）の確保。10件実行中の場合はここでブロックして待機。
		semaphore <- struct{}{}

		go func(u string) {
			defer wg.Done()

			// 処理完了後にリソース（スロット）を解放。他の待機中のGoroutineが実行可能になる。
			defer func() { <-semaphore }()

			select {
			case <-ctx.Done():
				resultsChan <- types.URLResult{
					URL:   u,
					Error: ctx.Err(),
				}
				return
			default:
			}

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
