package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/shouni/action-perfect-get-on-go/internal/cleaner"
	"github.com/shouni/go-utils/iohandler"
	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
)

const (
	previewLines     = 10
	gcsPrefix        = "gs://" // GCS URIのプレフィックス
	gcsPathSeparator = "/"
)

// ----------------------------------------------------------------
// 依存関係インターフェースの定義 (DIのため)
// ----------------------------------------------------------------

// ContentCleaner はLLMによるクリーンアップ処理の抽象化です。
type ContentCleaner interface {
	CleanAndStructureText(ctx context.Context, results []extTypes.URLResult) (string, error)
}

// GCSOutputWriter はGCSへの出力処理の抽象化です。
// WriteToGCS は指定されたバケットとパスにコンテンツを書き込みます。
type GCSOutputWriter interface {
	WriteToGCS(ctx context.Context, bucket, path string, content string) error
}

// ----------------------------------------------------------------
// 具象実装
// ----------------------------------------------------------------

// LLMOutputGeneratorImpl は OutputGenerator インターフェースの具象実装です。
// 依存関係はコンストラクタで注入されます。
type LLMOutputGeneratorImpl struct {
	contentCleaner ContentCleaner
	gcsWriter      GCSOutputWriter
}

// NewLLMOutputGeneratorImpl は LLMOutputGeneratorImpl の新しいインスタンスを作成します。
func NewLLMOutputGeneratorImpl(contentCleaner ContentCleaner, gcsWriter GCSOutputWriter) *LLMOutputGeneratorImpl {
	return &LLMOutputGeneratorImpl{
		contentCleaner: contentCleaner,
		gcsWriter:      gcsWriter,
	}
}

// Generate は、取得したコンテンツをLLMでクリーンアップ・構造化し、ファイルに出力します。
// GCS出力が指定された場合、ローカル出力はスキップされます。
func (l *LLMOutputGeneratorImpl) Generate(ctx context.Context, opts CmdOptions, successfulResults []extTypes.URLResult) error {
	slog.Info("フェーズ2 - 抽出結果を基に、AIクリーンアップと構造化を開始します。", slog.Int("count", len(successfulResults)))

	// AIクリーンアップフェーズ (LLM) (注入されたcontentCleanerを使用)
	slog.Info("フェーズ3 - LLMによるテキストのクリーンアップと構造化を開始します (Go-AI-Client利用)。")

	cleanedText, err := l.contentCleaner.CleanAndStructureText(ctx, successfulResults)
	if err != nil {
		return fmt.Errorf("LLMクリーンアップ処理に失敗しました: %w", err)
	}

	// ----------------------------------------------------------------
	// 最終結果の出力処理
	// ----------------------------------------------------------------

	outputFilePath := opts.OutputFilePath
	hasGCSOutput := strings.HasPrefix(outputFilePath, gcsPrefix)

	if hasGCSOutput {
		// GCSへの出力パス
		bucket, path, err := l.parseGCSURI(outputFilePath)
		if err != nil {
			return fmt.Errorf("GCS URIのパースに失敗しました: %w", err)
		}

		// GCSへの出力実行
		if err := l.writeToGCS(ctx, bucket, path, cleanedText); err != nil {
			return fmt.Errorf("GCSへの最終結果の出力に失敗しました: %w", err)
		}

		// GCSへの書き込みが完了したら、ローカル出力/標準出力の処理をスキップして終了
		slog.Info("LLMによる構造化とGCSへの出力が完了しました。", slog.String("uri", outputFilePath))
		return nil
	}

	// GCSへの出力ではない場合（ローカルファイルまたは標準出力）

	// 2. ローカルファイルへの出力 (パスが空でない場合)
	if outputFilePath != "" {
		if err := l.writeOutputString(outputFilePath, cleanedText); err != nil {
			return fmt.Errorf("ローカルファイルへの最終結果の出力に失敗しました: %w", err)
		}
	} else {
		// 3. 標準出力への出力（outputFilePath == "" の場合）
		if err := l.outputPreview(cleanedText); err != nil {
			return err
		}
	}

	slog.Info("LLMによる構造化と出力が完了しました。")
	return nil
}

// parseGCSURI は "gs://bucket/path/to/object" 形式のURIをバケット名とパスに分解します。
func (l *LLMOutputGeneratorImpl) parseGCSURI(uri string) (bucket, path string, err error) {
	if !strings.HasPrefix(uri, gcsPrefix) {
		return "", "", fmt.Errorf("URIは '%s' で始まっていません: %s", gcsPrefix, uri)
	}

	// "gs://" を取り除く
	trimmedURI := strings.TrimPrefix(uri, gcsPrefix)

	// 最初の '/' でバケットとパスを分割
	parts := strings.SplitN(trimmedURI, gcsPathSeparator, 2)

	bucket = parts[0]
	if bucket == "" {
		return "", "", fmt.Errorf("GCS URIにバケット名が指定されていません: %s", uri)
	}

	path = ""
	if len(parts) > 1 {
		path = parts[1]
	} else {
		// gs://bucket/ の形式でパスがない場合はエラーとする
		return "", "", fmt.Errorf("GCS URIにオブジェクトパスが指定されていません: %s", uri)
	}

	return bucket, path, nil
}

// writeToGCS は、注入されたGCSOutputWriterを使ってGCSへ内容を書き出します。
func (l *LLMOutputGeneratorImpl) writeToGCS(ctx context.Context, bucket, path string, content string) error {
	slog.Info("最終生成結果をGCSに書き込みます", slog.String("bucket", bucket), slog.String("path", path))

	if err := l.gcsWriter.WriteToGCS(ctx, bucket, path, content); err != nil {
		return fmt.Errorf("GCSバケット '%s' パス '%s' への書き込みに失敗しました: %w", bucket, path, err)
	}

	slog.Info("最終生成完了 - GCSに書き込みました", slog.String("uri", fmt.Sprintf("gs://%s/%s", bucket, path)))
	return nil
}

// WriteOutputString は、ファイルまたは標準出力に内容を書き出します。
// ファイル名が指定された場合、ディレクトリが存在しなければ作成し、ファイルに書き込みます。
func (l *LLMOutputGeneratorImpl) writeOutputString(filename string, content string) error {
	if filename != "" {
		// 1. ディレクトリの作成 (存在しない場合は再帰的に作成)
		dir := filepath.Dir(filename)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("ディレクトリの作成に失敗しました (%s): %w", dir, err)
		}

		// 2. ファイルへの書き込み
		if err := iohandler.WriteOutputString(filename, content); err != nil {
			return fmt.Errorf("ファイルへの書き込みに失敗しました: %w", err)
		}
		slog.Info("最終生成完了 - ファイルに書き込みました", slog.String("file", filename))

		return nil
	}

	return fmt.Errorf("writeOutputStringが空のファイル名で呼び出されました。")
}

// outputPreview は、標準出力にプレビューを書き出します。
func (l *LLMOutputGeneratorImpl) outputPreview(content string) error {
	// 標準出力にファイルの冒頭10行を表示
	lines := strings.Split(content, "\n")
	previewContent := ""
	if len(lines) > 0 {
		// 最初の10行（または行数全て）を抽出
		end := previewLines
		if len(lines) < previewLines {
			end = len(lines)
		}
		previewContent = strings.Join(lines[:end], "\n")
	}

	slog.Info("最終生成結果を標準出力にプレビュー表示します (冒頭10行)。")
	return iohandler.WriteOutputString("", previewContent)
}

// 型アサーションチェック
var _ ContentCleaner = (*cleaner.Cleaner)(nil)
