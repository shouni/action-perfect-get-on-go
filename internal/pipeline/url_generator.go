package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"cloud.google.com/go/storage"
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
	var reader io.Reader
	ctx := context.Background()

	if strings.HasPrefix(filePath, "gs://") {
		// GCS URI の処理
		client, err := storage.NewClient(ctx)
		if err != nil {
			return nil, fmt.Errorf("GCSクライアントの初期化に失敗しました: %w", err)
		}
		defer client.Close()

		// gs://bucket-name/object-name からバケット名とオブジェクト名を取得
		// filePath[5:] は "gs://" を除いた部分
		parts := strings.SplitN(filePath[5:], "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("無効なGCS URI形式です: %s", filePath)
		}
		bucketName := parts[0]
		objectName := parts[1]

		// GCS オブジェクトリーダーを作成
		rc, err := client.Bucket(bucketName).Object(objectName).NewReader(ctx)
		if err != nil {
			// NewReader がエラーを返す場合、ファイルが存在しないか、権限がない
			return nil, fmt.Errorf("GCSファイルの読み込みに失敗しました (URI: %s): %w", filePath, err)
		}
		defer rc.Close()
		reader = rc

	} else {
		// ローカルファイルパスの処理 (既存のロジック)
		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("ローカルファイルのオープンに失敗しました: %w", err)
		}
		defer file.Close()
		reader = file
	}

	var urls []string
	scanner := bufio.NewScanner(reader)

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
