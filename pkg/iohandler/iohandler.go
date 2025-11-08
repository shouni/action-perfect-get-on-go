package iohandler

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const previewLines = 20

// ReadInput は、ファイルまたは標準入力から内容を読み込みます。
func ReadInput(filename string) ([]byte, error) {
	if filename != "" {
		fmt.Fprintf(os.Stderr, "ファイルから読み込み中: %s\n", filename)
		return os.ReadFile(filename)
	}

	fmt.Fprintln(os.Stderr, "標準入力 (stdin) から読み込み中...")
	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("標準入力からの読み込みに失敗しました: %w", err)
	}
	return content, nil
}

// WriteOutputString は、ファイルまたは標準出力に内容を書き出します。
// ファイル名が指定された場合、ディレクトリが存在しなければ作成し、ファイルに書き込みます。
// その後、ファイルの冒頭10行を標準出力に出力します。
func WriteOutputString(filename string, content string) error { // ★ 関数名を WriteOutputString に変更
	if filename != "" {
		// 1. ディレクトリの作成 (存在しない場合は再帰的に作成)
		dir := filepath.Dir(filename)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("ディレクトリの作成に失敗しました (%s): %w", dir, err)
		}

		// 2. ファイルへの書き込み
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return fmt.Errorf("ファイルへの書き込みに失敗しました: %w", err)
		}
		fmt.Fprintf(os.Stderr, "\n--- 最終生成完了 ---\nファイルに書き込みました: %s\n", filename)

		// 3. 標準出力にファイルの冒頭10行を表示
		lines := strings.Split(content, "\n")

		fmt.Fprintln(os.Stdout, "\n--- ファイル出力プレビュー (標準出力) ---")

		previewContent := ""
		if len(lines) > 0 {
			// 最初の10行（または行数全て）を抽出
			end := previewLines
			if len(lines) < previewLines {
				end = len(lines)
			}
			previewContent = strings.Join(lines[:end], "\n")
		}

		fmt.Fprintln(os.Stdout, previewContent)
		if len(lines) > previewLines {
			fmt.Fprintln(os.Stdout, "...")
		}
		fmt.Fprintln(os.Stdout, "------------------------------------------")

		return nil
	}

	fmt.Fprintln(os.Stderr, "\n--- スクリプト生成結果 ---")
	// スクリプト本体は標準出力に出力 (パイプ処理を考慮)
	fmt.Fprintln(os.Stdout, content)
	return nil
}
