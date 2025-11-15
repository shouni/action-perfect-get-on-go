package pipeline

import (
	"context"
	"io"
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

// ScraperRunner は並列スクレイピングを実行する外部依存の抽象化です。
// ContentFetcher の具象実装が内部で使用するサブ依存として定義されます。
type ScraperRunner interface {
	ScrapeInParallel(ctx context.Context, urls []string) []extTypes.URLResult
}

// InputReader は、抽象化された入力ストリームを開くための契約です。
// これにより、GCSやローカルファイルへのアクセスロジックを分離します。
type InputReader interface {
	// Open は、指定されたパス（またはURI）から読み取り可能なストリームを開き、
	// io.ReadCloser を返します。
	Open(ctx context.Context, path string) (io.ReadCloser, error)
}

type Writer interface {
	WriteToGCS(ctx context.Context, bucket, path string, content io.Reader, contentType string) error
	WriteToLocal(ctx context.Context, path string, content io.Reader) error
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
