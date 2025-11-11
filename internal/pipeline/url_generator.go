package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

// URLReader は InputReader インターフェースの別名であり、URLGenerator が依存すべき抽象化です。
type URLReader InputReader

// DefaultURLGeneratorImpl は URLGenerator インターフェースの具象実装です。
// InputReader に依存し、入力ソース（GCS/ローカル）のロジックから分離されます。
type DefaultURLGeneratorImpl struct {
	// 抽象化されたリーダーに依存
	reader URLReader
}

// NewDefaultURLGeneratorImpl は DefaultURLGeneratorImpl の新しいインスタンスを作成し、
// 抽象化されたリーダーを注入します。
func NewDefaultURLGeneratorImpl(reader URLReader) *DefaultURLGeneratorImpl {
	return &DefaultURLGeneratorImpl{
		reader: reader,
	}
}

// Generate はファイルからURLを読み込み、基本的なバリデーションを実行します。
func (d *DefaultURLGeneratorImpl) Generate(ctx context.Context, opts CmdOptions) ([]string, error) {
	if opts.URLFile == "" {
		return nil, fmt.Errorf("処理対象のURLを指定してください。-f/--url-file オプションでURLリストファイルを指定してください。")
	}

	// 1. InputReader を使ってストリームを開く（ローカル/GCSのロジックは委譲）
	rc, err := d.reader.Open(ctx, opts.URLFile)
	if err != nil {
		return nil, fmt.Errorf("URLファイルのオープンに失敗しました: %w", err)
	}
	defer rc.Close()

	// 2. ストリームからURLをパースする（責務分離）
	urls, err := parseURLs(rc)
	if err != nil {
		return nil, fmt.Errorf("URLファイルの読み込み・パースに失敗しました: %w", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("URLリストファイルに有効なURLが一件も含まれていませんでした。")
	}
	return urls, nil
}

// parseURLs は、io.Reader から URL を抽出し、コメントと空行をスキップする独立したヘルパー関数です。
func parseURLs(r io.Reader) ([]string, error) {
	var urls []string
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// 空行またはコメント行 (#で始まる行) をスキップ
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		urls = append(urls, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("ファイルの読み取り中にエラーが発生しました: %w", err)
	}
	return urls, nil
}
