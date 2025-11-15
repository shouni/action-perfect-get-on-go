package builder

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shouni/action-perfect-get-on-go/internal/cleaner"
	"github.com/shouni/action-perfect-get-on-go/internal/pipeline"
	"github.com/shouni/action-perfect-get-on-go/prompts"
	gcsClient "github.com/shouni/go-remote-io/pkg/factory"
	"github.com/shouni/web-text-pipe-go/pkg/builder"
)

// BuildPipeline は、必要なすべての依存関係を構築し、DIされた Pipeline インスタンスと
// GCSクライアントのクリーンアップ関数 (Close) を返します。
func BuildPipeline(ctx context.Context, opts pipeline.CmdOptions) (*pipeline.Pipeline, func(), error) {

	// ----------------------------------------------------------------
	// 1. GCS クライアントの初期化とクリーンアップ設定 (Factoryに委譲)
	// ----------------------------------------------------------------

	// Factoryを初期化し、GCSクライアントの初期化と管理を委譲する
	gcsClient, err := gcsClient.NewClientFactory(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("Factoryの初期化に失敗しました: %w", err)
	}

	// FactoryのCloseは func() error 型なので、戻り値の func() に合わせるためのラッパーを定義
	closer := func() {
		if closeErr := gcsClient.Close(); closeErr != nil {
			slog.Error("Factoryのクローズ中にエラーが発生しました", slog.Any("error", closeErr))
		}
	}

	// ----------------------------------------------------------------
	// 2. Webコンテンツ取得のための依存関係の具体化
	// ----------------------------------------------------------------

	// BuildReliableScraperExecutor を呼び出し、リトライ実行者を取得
	scraperExecutor, err := builder.BuildReliableScraperExecutor(opts.ScraperTimeout, opts.MaxScraperParallel)
	if err != nil {
		// 失敗時はFactoryを閉じる
		return nil, closer, fmt.Errorf("ReliableScraperExecutorの初期化に失敗しました: %w", err)
	}

	// ----------------------------------------------------------------
	// 3. ContentCleaner (LLMクリーンアップロジック) の構築
	// ----------------------------------------------------------------

	// プロンプトビルダーの初期化
	mapBuilder := prompts.NewMapPromptBuilder()
	if err := mapBuilder.Err(); err != nil {
		return nil, closer, fmt.Errorf("Map Prompt Builderの初期化に失敗しました: %w", err)
	}
	reduceBuilder := prompts.NewReducePromptBuilder()
	if err := reduceBuilder.Err(); err != nil {
		return nil, closer, fmt.Errorf("Reduce Prompt Builderの初期化に失敗しました: %w", err)
	}

	// PromptBuilders を構造体にまとめる
	builders := cleaner.PromptBuilders{
		MapBuilder:    mapBuilder,
		ReduceBuilder: reduceBuilder,
	}

	// LLMExecutor の構築
	executor, err := cleaner.NewLLMConcurrentExecutor(ctx, opts.LLMAPIKey, cleaner.DefaultMaxMapConcurrency)
	if err != nil {
		return nil, closer, fmt.Errorf("LLM Executorの初期化に失敗しました: %w", err)
	}

	// Cleaner の構築
	contentCleaner, err := cleaner.NewCleaner(builders, executor)
	if err != nil {
		return nil, closer, fmt.Errorf("Cleanerの初期化に失敗しました: %w", err)
	}

	// ----------------------------------------------------------------
	// 4. パイプラインステージの実装とPipelineの構築 (DIの実行)
	// ----------------------------------------------------------------

	// 4.1 URLGenerator の構築
	urlReader, err := gcsClient.NewInputReader()
	if err != nil {
		return nil, closer, fmt.Errorf("InputReaderの生成に失敗しました: %w", err)
	}
	// urlReader (remoteio.InputReader) を NewDefaultURLGeneratorImpl に注入
	urlGen := pipeline.NewDefaultURLGeneratorImpl(urlReader)

	// 4.2 ContentFetcher の構築
	fetcher := pipeline.NewWebContentFetcherImpl(scraperExecutor)

	// 4.3 OutputGenerator の構築 (ContentCleanerとWriterを注入)
	rawOutputWriter, err := gcsClient.NewOutputWriter()
	if err != nil {
		return nil, closer, fmt.Errorf("OutputWriterの生成に失敗しました: %w", err)
	}

	// 具象型 (UniversalIOWriter) は pipeline.Writer (GCSとLocalの両機能を結合したもの) を満たす。
	outputWriter, ok := rawOutputWriter.(pipeline.Writer)
	if !ok {
		// Factoryが予期せぬ型を返した場合のガード
		return nil, closer, fmt.Errorf("生成されたWriterが pipeline.Writer インターフェース (GCS/Localの両機能) を満たしていません")
	}

	// ContentCleanerとOutputWriterを注入 (修正されたoutputWriterを使用)
	outputGen := pipeline.NewLLMOutputGeneratorImpl(contentCleaner, outputWriter)

	// 全てのステージとオプションをPipelineに注入し、クリーンアップ関数も一緒に返す
	return pipeline.NewPipeline(opts, urlGen, fetcher, outputGen), closer, nil
}
