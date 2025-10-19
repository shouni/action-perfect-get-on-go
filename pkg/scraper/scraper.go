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

// Client は単一のURLからコンテンツを抽出する基本的なクライアント構造体です。
type Client struct {
	extractor *webextractor.Extractor
}

// ParallelScraper は Scraper インターフェースを実装する並列処理構造体です。
// Client へのポインタを保持することで、抽出ロジックを共有します。
type ParallelScraper struct {
	client *Client
}

// NewClient は単一リクエスト用のクライアントを初期化します。
func NewClient(timeout time.Duration) (*Client, error) {
	// 1. 堅牢な HTTP クライアントを初期化 (リトライ、エラーハンドリング内蔵)
	httpClient := httpclient.New(timeout)

	// 2. 抽出ロジックを管理する Extractor を初期化
	extractor := webextractor.NewExtractor(httpClient)

	// エラーは発生しない想定だが、統一的なインターフェースとして返す
	return &Client{
		extractor: extractor,
	}, nil // nil エラーを返す
}

// NewParallelScraper は ParallelScraper を初期化します。
// エラーハンドリングのために、戻り値に error を追加します。
func NewParallelScraper(timeout time.Duration) (*ParallelScraper, error) {
	// 内部の基本クライアントを初期化
	client, err := NewClient(timeout)
	if err != nil {
		return nil, err
	}

	return &ParallelScraper{
		client: client,
	}, nil
}

// ScrapeInParallel は Scraper インターフェースのメソッドを実装します。（変更なし）
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

			// 並列処理では、ParallelScraperが持つ内部Clientのメソッドを呼び出す
			res, err := s.client.ExtractContent(u, ctx)

			resultsChan <- types.URLResult{
				URL:     u,
				Content: res, // resはstring
				Error:   err,
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

// ExtractContent は単一のURLからメインコンテンツを抽出します。
// これは ParallelScraper の内部、および cmd/main.go の順次リトライで使用されます。
// 引数の順序を合わせるため、Clientのメソッドとして定義します。
func (c *Client) ExtractContent(url string, ctx context.Context) (string, error) {
	// Extractor の FetchAndExtractText を呼び出し、抽出・整形ロジックをすべて委譲します。
	content, hasBodyFound, err := c.extractor.FetchAndExtractText(url, ctx)
	if err != nil {
		return "", fmt.Errorf("コンテンツの抽出に失敗しました: %w", err)
	}

	// 抽出ロジックの判定結果 (本文が見つからなかった場合)
	if content == "" || !hasBodyFound {
		return "", fmt.Errorf("URL %s から有効な本文を抽出できませんでした", url)
	}

	return content, nil
}

// // 既存の extractContent ヘルパーを Client のメソッドとして再定義したため、元のものは削除または修正が必要です
// func (s *ParallelScraper) extractContent(url string, ctx context.Context) (string, error) {
//     // (この関数は不要になるか、上記の Client.ExtractContent を呼び出すように修正されます)
//     return s.client.ExtractContent(url, ctx)
// }
