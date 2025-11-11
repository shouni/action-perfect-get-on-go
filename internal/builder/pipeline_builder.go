package builder

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"

	"github.com/shouni/action-perfect-get-on-go/internal/cleaner"
	"github.com/shouni/action-perfect-get-on-go/internal/pipeline"
	"github.com/shouni/action-perfect-get-on-go/prompts"
	"github.com/shouni/web-text-pipe-go/pkg/builder"
)

// BuildPipeline は、必要なすべての依存関係を構築し、DIされた Pipeline インスタンスと
// GCSクライアントのクリーンアップ関数 (Close) を返します。
func BuildPipeline(ctx context.Context, opts pipeline.CmdOptions) (*pipeline.Pipeline, func(), error) {

	// ----------------------------------------------------------------
	// 1. GCS クライアントの初期化とクリーンアップ設定
	// ----------------------------------------------------------------

	// GCS クライアントの初期化
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("GCSクライアントの初期化に失敗しました: %w", err)
	}

	// クリーンアップ関数を定義
	gcsClientCloser := func() {
		gcsClient.Close()
	}

	// ----------------------------------------------------------------
	// 2. Webコンテンツ取得のための依存関係の具体化
	// ----------------------------------------------------------------

	// BuildReliableScraperExecutor を呼び出し、リトライ実行者を取得
	scraperExecutor, err := builder.BuildReliableScraperExecutor(opts.ScraperTimeout, opts.MaxScraperParallel)
	if err != nil {
		// 失敗時はGCSクライアントを閉じる
		return nil, gcsClientCloser, fmt.Errorf("ReliableScraperExecutorの初期化に失敗しました: %w", err)
	}

	// ----------------------------------------------------------------
	// 3. ContentCleaner (LLMクリーンアップロジック) の構築
	// ----------------------------------------------------------------

	// プロンプトビルダーの初期化
	mapBuilder := prompts.NewMapPromptBuilder()
	if err := mapBuilder.Err(); err != nil {
		return nil, gcsClientCloser, fmt.Errorf("Map Prompt Builderの初期化に失敗しました: %w", err)
	}
	reduceBuilder := prompts.NewReducePromptBuilder()
	if err := reduceBuilder.Err(); err != nil {
		return nil, gcsClientCloser, fmt.Errorf("Reduce Prompt Builderの初期化に失敗しました: %w", err)
	}

	// PromptBuilders を構造体にまとめる
	builders := cleaner.PromptBuilders{
		MapBuilder:    mapBuilder,
		ReduceBuilder: reduceBuilder,
	}

	// LLMExecutor の構築
	executor, err := cleaner.NewLLMConcurrentExecutor(ctx, opts.LLMAPIKey, cleaner.DefaultMaxMapConcurrency)
	if err != nil {
		return nil, gcsClientCloser, fmt.Errorf("LLM Executorの初期化に失敗しました: %w", err)
	}

	// Cleaner の構築
	contentCleaner, err := cleaner.NewCleaner(builders, executor)
	if err != nil {
		return nil, gcsClientCloser, fmt.Errorf("Cleanerの初期化に失敗しました: %w", err)
	}

	// ----------------------------------------------------------------
	// 4. パイプラインステージの実装とPipelineの構築 (DIの実行)
	// ----------------------------------------------------------------

	// 4.1 URLGenerator の構築
	// NewLocalGCSInputReader を使用して InputReader の具象実装を作成し、
	// それを NewDefaultURLGeneratorImpl に注入するように修正。
	urlReader := pipeline.NewLocalGCSInputReader(gcsClient)
	urlGen := pipeline.NewDefaultURLGeneratorImpl(urlReader)

	// 4.2 ContentFetcher の構築
	fetcher := pipeline.NewWebContentFetcherImpl(scraperExecutor)

	// 4.3 OutputGenerator の構築 (ContentCleanerを注入)
	outputGen := pipeline.NewLLMOutputGeneratorImpl(contentCleaner)

	// 全てのステージとオプションをPipelineに注入し、クリーンアップ関数も一緒に返す
	return pipeline.NewPipeline(opts, urlGen, fetcher, outputGen), gcsClientCloser, nil
}
