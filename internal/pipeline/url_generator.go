package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// DefaultURLGeneratorImpl は URLGenerator インターフェースの具象実装です。
// (元の app.generateURLs のロジックを保持)
type DefaultURLGeneratorImpl struct{}

// NewDefaultURLGeneratorImpl は DefaultURLGeneratorImpl の新しいインスタンスを作成します。
func NewDefaultURLGeneratorImpl() *DefaultURLGeneratorImpl {
	return &DefaultURLGeneratorImpl{}
}

// Generate はファイルからURLを読み込み、基本的なバリデーションを実行します。
func (d *DefaultURLGeneratorImpl) Generate(ctx context.Context, opts CmdOptions) ([]string, error) {
	if opts.URLFile == "" {
		return nil, fmt.Errorf("処理対象のURLを指定してください。-f/--url-file オプションでURLリストファイルを指定してください。")
	}

	urls, err := d.readURLsFromFile(opts.URLFile)
	if err != nil {
		return nil, fmt.Errorf("URLファイルの読み込みに失敗しました: %w", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("URLリストファイルに有効なURLが一件も含まれていませんでした。")
	}
	return urls, nil
}

// readURLsFromFileは指定されたファイルからURLを読み込みます。(元のヘルパー関数から移動)
func (*DefaultURLGeneratorImpl) readURLsFromFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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
