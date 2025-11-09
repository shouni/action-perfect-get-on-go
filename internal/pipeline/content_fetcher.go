package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/shouni/go-web-exact/v2/pkg/extract"
	extScraper "github.com/shouni/go-web-exact/v2/pkg/scraper"
	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
)

// ----------------------------------------------------------------
// 依存関係インターフェースの定義 (DIのため)
// ----------------------------------------------------------------

// ScraperExecutor は並列スクレイピングを実行する外部依存の抽象化です。
// (extScraper.ParallelScraperがこれを実装すると想定)
type ScraperExecutor interface {
	ScrapeInParallel(ctx context.Context, urls []string) []extTypes.URLResult
}

// Extractor はコンテンツ抽出ロジックの抽象化です。
// (extract.Extractorがこれを実装すると想定)
type Extractor interface {
	FetchAndExtractText(ctx context.Context, url string) (string, bool, error)
}

// ----------------------------------------------------------------
// 具象実装
// ----------------------------------------------------------------

// WebContentFetcherImpl は ContentFetcher インターフェースの具象実装です。
// 依存関係はコンストラクタで注入されます。
type WebContentFetcherImpl struct {
	scraperExecutor ScraperExecutor
	extractor       Extractor // リトライ処理で使用
}

// NewWebContentFetcherImpl は WebContentFetcherImpl の新しいインスタンスを作成します。
func NewWebContentFetcherImpl(scraperExecutor ScraperExecutor, extractor Extractor) *WebContentFetcherImpl {
	return &WebContentFetcherImpl{
		scraperExecutor: scraperExecutor,
		extractor:       extractor,
	}
}

// Fetch は、URLリストに対して並列スクレイピングと、失敗したURLに対するリトライを実行します。
// (元の app.generateContents のロジックを保持)
func (w *WebContentFetcherImpl) Fetch(ctx context.Context, opts CmdOptions, urls []string) ([]extTypes.URLResult, error) {
	// [行番号: 29 修正] log.Println -> slog.Info
	slog.Info("フェーズ1 - Webコンテンツの並列抽出を開始します。")

	// 1. 並列実行 (注入されたscraperExecutorを使用)
	results := w.scraperExecutor.ScrapeInParallel(ctx, urls)

	// 2. 無条件遅延 (負荷軽減)
	// [行番号: 32 修正] log.Printf -> slog.Info (構造化)
	slog.Info("並列抽出が完了しました。次の処理に進む前に待機します。", slog.Duration("delay", InitialScrapeDelay))
	time.Sleep(InitialScrapeDelay)

	// 3. 結果の分類
	successfulResults, failedURLs := classifyResults(results)
	initialSuccessfulCount := len(successfulResults)

	// 4. 失敗URLの上位レベルリトライ処理 (注入されたextractorを使用)
	if len(failedURLs) > 0 {
		retriedSuccessfulResults, retryErr := w.processFailedURLs(ctx, failedURLs, RetryScrapeDelay)
		if retryErr != nil {
			// [行番号: 42 修正] log.Printf -> slog.Warn (構造化)
			slog.Warn("失敗URLのリトライ処理中にエラーが発生しました", slog.Any("error", retryErr))
		}
		successfulResults = append(successfulResults, retriedSuccessfulResults...)
	}

	// 5. 最終チェックとログ
	if len(successfulResults) == 0 {
		return nil, fmt.Errorf("処理可能なWebコンテンツを一件も取得できませんでした。URLを確認してください。")
	}

	// [行番号: 54 修正] log.Printf -> slog.Info (構造化)
	slog.Info("最終成功数",
		slog.Int("successful", len(successfulResults)),
		slog.Int("total", len(urls)),
		slog.Int("initial_successful", initialSuccessfulCount),
		slog.Int("retry_successful", len(successfulResults)-initialSuccessfulCount),
	)

	return successfulResults, nil
}

// processFailedURLsは、失敗したURLに対し、指定された遅延時間後に順次リトライを実行します。
// (元のヘルパー関数から移動し、Extractorへの依存を変更)
func (w *WebContentFetcherImpl) processFailedURLs(ctx context.Context, failedURLs []string, retryDelay time.Duration) ([]extTypes.URLResult, error) {
	// [行番号: 84 修正] log.Printf -> slog.Warn (構造化)
	slog.Warn("抽出に失敗したURLがありました。待機後、順次リトライを開始します。", slog.Int("count", len(failedURLs)), slog.Duration("delay", retryDelay))
	time.Sleep(retryDelay)

	var retriedSuccessfulResults []extTypes.URLResult
	// [行番号: 87 修正] log.Println -> slog.Info
	slog.Info("失敗URLの順次リトライを開始します。")

	for _, url := range failedURLs {
		// [行番号: 90 修正] log.Printf -> slog.Info (構造化)
		slog.Info("リトライ中", slog.String("url", url))

		// 注入されたExtractorを使用
		content, hasBodyFound, err := w.extractor.FetchAndExtractText(ctx, url)

		var extractErr error
		if err != nil {
			extractErr = fmt.Errorf("コンテンツの抽出に失敗しました: %w", err)
		} else if content == "" || !hasBodyFound {
			extractErr = fmt.Errorf("URL %s から有効な本文を抽出できませんでした", url)
		}

		if extractErr != nil {
			formattedErr := formatErrorLog(extractErr)
			// [行番号: 110 修正] log.Printf -> slog.Error (構造化)
			slog.Error("リトライでもURLの抽出に失敗しました", slog.String("url", url), slog.String("error", formattedErr))
		} else {
			// [行番号: 113 修正] log.Printf -> slog.Info (構造化)
			slog.Info("URLの抽出がリトライで成功しました", slog.String("url", url))
			retriedSuccessfulResults = append(retriedSuccessfulResults, extTypes.URLResult{
				URL:     url,
				Content: content,
				Error:   nil,
			})
		}
	}
	return retriedSuccessfulResults, nil
}

// classifyResultsは並列抽出の結果を成功と失敗に分類します。(元のヘルパー関数から移動)
func classifyResults(results []extTypes.URLResult) (successfulResults []extTypes.URLResult, failedURLs []string) {
	for _, res := range results {
		if res.Error != nil || res.Content == "" {
			failedURLs = append(failedURLs, res.URL)
		} else {
			successfulResults = append(successfulResults, res)
		}
	}
	return successfulResults, failedURLs
}

// formatErrorLogは、冗長なエラーメッセージを短縮します。(元のヘルパー関数から移動)
func formatErrorLog(err error) string {
	errMsg := err.Error()
	if idx := strings.Index(errMsg, ", ボディ: <!"); idx != -1 {
		errMsg = errMsg[:idx]
	}
	if idx := strings.LastIndex(errMsg, "最終エラー:"); idx != -1 {
		return strings.TrimSpace(errMsg[idx:])
	}
	return errMsg
}

// 型アサーションチェック (インターフェースの定義が外部パッケージの実装に合致することを確認)
var _ ScraperExecutor = (*extScraper.ParallelScraper)(nil)
var _ Extractor = (*extract.Extractor)(nil)
