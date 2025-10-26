package scraper

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shouni/action-perfect-get-on-go/pkg/types"

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
type ParallelScraper struct {
	client *Client
}

// NewClient は単一リクエスト用のクライアントを初期化します。
func NewClient(timeout time.Duration) (*Client, error) {
	// 1. 堅牢な HTTP クライアントを初期化 (リトライ、エラーハンドリング内蔵)
	httpClient := httpclient.New(timeout)

	// 2. 抽出ロジックを管理する Extractor を初期化
	extractor := webextractor.NewExtractor(httpClient)

	// 行番号 44 の修正: エラーを返す意図をコメントで明確化
	// 現在のhttpclientとwebextractorの実装ではエラーは発生しない想定だが、
	// 将来的な変更に備え、統一的なインターフェースとしてerrorを返すようにしている。
	return &Client{
		extractor: extractor,
	}, nil
}

// NewParallelScraper は ParallelScraper を初期化します。
func NewParallelScraper(timeout time.Duration) (*ParallelScraper, error) {
	client, err := NewClient(timeout)
	if err != nil {
		return nil, err
	}

	return &ParallelScraper{
		client: client,
	}, nil
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

			res, err := s.client.ExtractContent(u, ctx)

			resultsChan <- types.URLResult{
				URL:     u,
				Content: res,
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
