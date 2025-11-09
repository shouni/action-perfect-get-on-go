package builder

import (
	"fmt"

	"github.com/shouni/action-perfect-get-on-go/internal/cleaner"
	"github.com/shouni/action-perfect-get-on-go/internal/pipeline"
	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
	extScraper "github.com/shouni/go-web-exact/v2/pkg/scraper"
)

// BuildPipeline は、必要なすべての依存関係を構築し、DIされた Pipeline インスタンスを返します。
// (元の app.go の初期化ロジックと NewApp の役割を担う)
func BuildPipeline(opts pipeline.CmdOptions) (*pipeline.Pipeline, error) {
	// ----------------------------------------------------------------
	// 1. 依存関係の具体化 (外部パッケージの依存関係構築)
	// ----------------------------------------------------------------

	// 1.1 HTTP クライアント (Fetcher) の構築
	clientOptions := []httpkit.ClientOption{
		httpkit.WithMaxRetries(pipeline.DefaultHTTPMaxRetries), // httpkitレベルのリトライ
	}
	httpClient := httpkit.New(opts.ScraperTimeout, clientOptions...)

	// 1.2 Extractor (Webコンテンツ抽出ロジック) の構築
	extractor, err := extract.NewExtractor(httpClient)
	if err != nil {
		return nil, fmt.Errorf("Extractorの初期化に失敗しました: %w", err)
	}

	// 1.3 ScraperExecutor (並列実行ロジック) の構築
	scraperExecutor := extScraper.NewParallelScraper(extractor, opts.MaxScraperParallel)

	// 1.4 ContentCleaner (LLMクリーンアップロジック) の構築
	contentCleaner, err := cleaner.NewCleaner(cleaner.DefaultMaxMapConcurrency)
	if err != nil {
		return nil, fmt.Errorf("Cleanerの初期化に失敗しました: %w", err)
	}

	// ----------------------------------------------------------------
	// 2. パイプラインステージの実装
	// ----------------------------------------------------------------

	// 2.1 URLGenerator の構築
	urlGen := pipeline.NewDefaultURLGeneratorImpl()

	// 2.2 ContentFetcher の構築 (ScraperExecutorとExtractorを注入)
	fetcher := pipeline.NewWebContentFetcherImpl(scraperExecutor, extractor)

	// 2.3 OutputGenerator の構築 (ContentCleanerを注入)
	outputGen := pipeline.NewLLMOutputGeneratorImpl(contentCleaner)

	// ----------------------------------------------------------------
	// 3. Pipeline の構築 (DIの実行)
	// ----------------------------------------------------------------

	// 全てのステージとオプションをPipelineに注入
	return pipeline.NewPipeline(opts, urlGen, fetcher, outputGen), nil
}
