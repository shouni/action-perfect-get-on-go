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

const previewLines = 10

// ----------------------------------------------------------------
// 依存関係インターフェースの定義 (DIのため)
// ----------------------------------------------------------------

// ContentCleaner はLLMによるクリーンアップ処理の抽象化です。
type ContentCleaner interface {
	CleanAndStructureText(ctx context.Context, results []extTypes.URLResult) (string, error)
}

// ----------------------------------------------------------------
// 具象実装
// ----------------------------------------------------------------

// LLMOutputGeneratorImpl は OutputGenerator インターフェースの具象実装です。
// 依存関係はコンストラクタで注入されます。
type LLMOutputGeneratorImpl struct {
	contentCleaner ContentCleaner
}

// NewLLMOutputGeneratorImpl は LLMOutputGeneratorImpl の新しいインスタンスを作成します。
func NewLLMOutputGeneratorImpl(contentCleaner ContentCleaner) *LLMOutputGeneratorImpl {
	return &LLMOutputGeneratorImpl{
		contentCleaner: contentCleaner,
	}
}

// Generate は、取得したコンテンツをLLMでクリーンアップ・構造化し、ファイルに出力します。
func (l *LLMOutputGeneratorImpl) Generate(ctx context.Context, opts CmdOptions, successfulResults []extTypes.URLResult) error {
	slog.Info("フェーズ2 - 抽出結果を基に、AIクリーンアップと構造化を開始します。", slog.Int("count", len(successfulResults)))

	// AIクリーンアップフェーズ (LLM) (注入されたcontentCleanerを使用)
	slog.Info("フェーズ3 - LLMによるテキストのクリーンアップと構造化を開始します (Go-AI-Client利用)。")

	// 修正: llmAPIKey を渡す引数を削除しました。
	// ContentCleaner インターフェースの定義が変更されたため、呼び出し側も修正します。
	cleanedText, err := l.contentCleaner.CleanAndStructureText(ctx, successfulResults)
	if err != nil {
		return fmt.Errorf("LLMクリーンアップ処理に失敗しました: %w", err)
	}

	// 最終結果の出力 (iohandlerパッケージを使用)
	if err := l.writeOutputString(opts.OutputFilePath, cleanedText); err != nil {
		return fmt.Errorf("最終結果の出力に失敗しました: %w", err)
	}

	slog.Info("LLMによる構造化が完了し、ファイルに出力されました。")
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

	err := l.outputPreview(content)
	if err != nil {
		return err
	}

	return nil
}

// outputPreview は、ファイルまたは標準出力にプレビューを書き出します。
func (l *LLMOutputGeneratorImpl) outputPreview(content string) error {
	// 3. 標準出力にファイルの冒頭10行を表示
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

	return iohandler.WriteOutputString("", previewContent)
}

// 型アサーションチェック
var _ ContentCleaner = (*cleaner.Cleaner)(nil)
