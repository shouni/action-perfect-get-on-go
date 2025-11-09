package pipeline

import (
	"context"
	"time"

	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
)

// ----------------------------------------------------------------
// 共通構造体
// ----------------------------------------------------------------

// CmdOptions は CLI オプションの値を集約するための構造体です。
// (元の app.CmdOptions から移動)
type CmdOptions struct {
	LLMAPIKey          string
	LLMTimeout         time.Duration
	ScraperTimeout     time.Duration
	URLFile            string
	OutputFilePath     string
	MaxScraperParallel int
}

// ----------------------------------------------------------------
// パイプラインステージのインターフェース (DIの契約)
// ----------------------------------------------------------------

// URLGenerator は、処理対象のURLリストを生成するステージの契約です。
type URLGenerator interface {
	// Generate はファイルなどからURLリストを読み込みます。
	Generate(ctx context.Context, opts CmdOptions) ([]string, error)
}

// ContentFetcher は、URLからWebコンテンツを取得し、結果を分類するステージの契約です。
type ContentFetcher interface {
	// Fetch はURLリストからコンテンツを並列/リトライで取得します。
	Fetch(ctx context.Context, opts CmdOptions, urls []string) ([]extTypes.URLResult, error)
}

// OutputGenerator は、取得したコンテンツをクリーンアップし、ファイルに出力するステージの契約です。
type OutputGenerator interface {
	// Generate はコンテンツを結合し、LLMで構造化し、最終結果をファイルに出力します。
	Generate(ctx context.Context, opts CmdOptions, results []extTypes.URLResult) error
}

// ----------------------------------------------------------------
// Pipeline コア構造
// ----------------------------------------------------------------

// Pipeline はアプリケーションの実行パイプラインを定義し、DIされた依存関係を保持します。
type Pipeline struct {
	// Options はパイプライン実行全体で必要な設定値を保持します。
	Options CmdOptions
	// DIされるステージ実装
	URLGen    URLGenerator
	Fetcher   ContentFetcher
	OutputGen OutputGenerator
}

// NewPipeline は CmdOptions とステージの具象実装を受け取り、Pipelineインスタンスを構築します。
func NewPipeline(
	opts CmdOptions,
	urlGen URLGenerator,
	fetcher ContentFetcher,
	outputGen OutputGenerator,
) *Pipeline {
	return &Pipeline{
		Options:   opts,
		URLGen:    urlGen,
		Fetcher:   fetcher,
		OutputGen: outputGen,
	}
}
