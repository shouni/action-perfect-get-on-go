package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"action-perfect-get-on-go/internal/cleaner"

	"github.com/shouni/go-remote-io/pkg/remoteio"
	"github.com/shouni/go-utils/iohandler"
	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
)

const (
	previewLines = 10
)

// ----------------------------------------------------------------
// 依存関係インターフェースの定義 (DIのため)
// ----------------------------------------------------------------

// ContentCleaner はLLMによるクリーンアップ処理の抽象化です。
type ContentCleaner interface {
	CleanAndStructureText(ctx context.Context, results []extTypes.URLResult) (string, error)
}

// MdToHtmlRunner は、github.com/shouni/go-text-format/pkg/runner.MarkdownToHtmlRunner インターフェースと一致するよう定義します。
type MdToHtmlRunner interface {
	ConvertMarkdownToHtml(ctx context.Context, title string, markdown []byte) (*bytes.Buffer, error)
}

// ----------------------------------------------------------------
// 具象実装
// ----------------------------------------------------------------

// LLMOutputGeneratorImpl は OutputGenerator インターフェースの具象実装です。
// 依存関係はコンストラクタで注入されます。
type LLMOutputGeneratorImpl struct {
	contentCleaner  ContentCleaner
	universalWriter Writer
	htmlRunner      MdToHtmlRunner
}

// NewLLMOutputGeneratorImpl は LLMOutputGeneratorImpl の新しいインスタンスを作成します。
func NewLLMOutputGeneratorImpl(contentCleaner ContentCleaner, writer Writer, htmlRunner MdToHtmlRunner) *LLMOutputGeneratorImpl {
	return &LLMOutputGeneratorImpl{
		contentCleaner:  contentCleaner,
		universalWriter: writer,
		htmlRunner:      htmlRunner,
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
	// コンテンツを io.Reader に変換
	contentBytes := []byte(cleanedText)
	contentReader := bytes.NewReader(contentBytes)

	if remoteio.IsGCSURI(outputFilePath) {
		slog.Info("LLMによって生成されたMarkdownをHTMLドキュメントに変換します。")
		htmlBuffer, err := l.htmlRunner.ConvertMarkdownToHtml(ctx, "", contentBytes)
		if err != nil {
			return fmt.Errorf("MarkdownからHTMLへの変換に失敗しました: %w", err)
		}
		contentReader = bytes.NewReader(htmlBuffer.Bytes())

		// GCSへの出力パス
		bucket, path, err := remoteio.ParseGCSURI(outputFilePath)
		if err != nil {
			return fmt.Errorf("GCS URIのパースに失敗しました: %w", err)
		}

		if err := l.writeToGCS(ctx, bucket, path, contentReader); err != nil {
			return fmt.Errorf("GCSへの最終結果の出力に失敗しました: %w", err)
		}

		// GCSへの書き込みが完了したら、ローカル出力/標準出力の処理をスキップして終了
		slog.Info("LLMによる構造化とGCSへの出力が完了しました。", slog.String("uri", outputFilePath))
		return nil
	}

	// 2. ローカルファイルへの出力 (パスが空でない場合)
	if outputFilePath != "" {
		if err := l.writeToLocal(ctx, outputFilePath, contentReader); err != nil {
			return fmt.Errorf("ローカルファイルへの最終結果の出力に失敗しました: %w", err)
		}
		slog.Info("LLMによる構造化とローカルファイルへの出力が完了しました。", slog.String("file", outputFilePath))
	} else {
		// 3. 標準出力への出力（outputFilePath == "" の場合）
		if err := l.outputPreview(cleanedText); err != nil {
			return err
		}
		slog.Info("LLMによる構造化と標準出力へのプレビューが完了しました。")
	}

	return nil
}

// writeToGCS は、注入されたWriterを使ってGCSへ内容を書き出します。
func (l *LLMOutputGeneratorImpl) writeToGCS(ctx context.Context, bucket, path string, contentReader io.Reader) error {
	slog.Info("最終生成結果をGCSに書き込みます", slog.String("bucket", bucket), slog.String("path", path))

	// 注入された Writer が remoteio.GCSOutputWriter を満たすことを確認
	gcsWriter, ok := l.universalWriter.(remoteio.GCSOutputWriter)
	if !ok {
		return fmt.Errorf("内部エラー: 注入された Writer は GCSOutputWriter インターフェースを満たしていません")
	}

	if err := gcsWriter.WriteToGCS(ctx, bucket, path, contentReader, "text/html; charset=utf-8"); err != nil {
		return fmt.Errorf("GCSバケット '%s' パス '%s' への書き込みに失敗しました: %w", bucket, path, err)
	}

	slog.Info("最終生成完了 - GCSに書き込みました", slog.String("uri", fmt.Sprintf("gs://%s/%s", bucket, path)))
	return nil
}

// writeToLocal ローカルファイルへの書き込み
func (l *LLMOutputGeneratorImpl) writeToLocal(ctx context.Context, path string, contentReader io.Reader) error {
	slog.Info("最終生成結果をローカルファイルに書き込みます", slog.String("path", path))

	// 注入された Writer が remoteio.LocalOutputWriter を満たすことを確認
	localWriter, ok := l.universalWriter.(remoteio.LocalOutputWriter)
	if !ok {
		return fmt.Errorf("内部エラー: 注入された Writer は LocalOutputWriter インターフェースを満たしていません")
	}

	if err := localWriter.WriteToLocal(ctx, path, contentReader); err != nil {
		return fmt.Errorf("ローカルファイル '%s' への書き込みに失敗しました: %w", path, err)
	}

	slog.Info("最終生成完了 - ローカルファイルに書き込みました", slog.String("file", path))
	return nil
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
	// iohandler.WriteOutputString は string を受け取るため、ここではそのまま利用
	return iohandler.WriteOutputString("", previewContent)
}

// 型アサーションチェック
var _ ContentCleaner = (*cleaner.Cleaner)(nil)
