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
// GCSクライアントを保持し、再利用することでパフォーマンスを向上させます。
type DefaultURLGeneratorImpl struct {
	// GCSクライアントを保持
	gcsClient *storage.Client
}

// NewDefaultURLGeneratorImpl は DefaultURLGeneratorImpl の新しいインスタンスを作成し、
// GCSクライアントを注入します。
// GCSを使用しない場合は nil を渡すことができますが、その場合は readURLsFromFile でエラーになります。
func NewDefaultURLGeneratorImpl(gcsClient *storage.Client) *DefaultURLGeneratorImpl {
	// gcsClient は、呼び出し元 (例: cmd パッケージや builder パッケージ) で一度だけ初期化され、
	// ここに渡されることを想定しています。
	return &DefaultURLGeneratorImpl{
		gcsClient: gcsClient,
	}
}

// Generate はファイルからURLを読み込み、基本的なバリデーションを実行します。
func (d *DefaultURLGeneratorImpl) Generate(ctx context.Context, opts CmdOptions) ([]string, error) {
	if opts.URLFile == "" {
		return nil, fmt.Errorf("処理対象のURLを指定してください。-f/--url-file オプションでURLリストファイルを指定してください。")
	}

	urls, err := d.readURLsFromFile(ctx, opts.URLFile)
	if err != nil {
		return nil, fmt.Errorf("URLファイルの読み込みに失敗しました: %w", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("URLリストファイルに有効なURLが一件も含まれていませんでした。")
	}
	return urls, nil
}

// readURLsFromFileは指定されたファイルからURLを読み込みます。
func (d *DefaultURLGeneratorImpl) readURLsFromFile(ctx context.Context, filePath string) ([]string, error) {
	var reader io.Reader

	if strings.HasPrefix(filePath, "gs://") {
		// GCS URI の処理
		if d.gcsClient == nil {
			return nil, fmt.Errorf("GCS URIが指定されましたが、GCSクライアントが初期化されていません。")
		}

		// GCSクライアントは NewDefaultURLGeneratorImpl で一度初期化されているため、
		// ここで client.Close() は呼び出しません。

		// gs://bucket-name/object-name からバケット名とオブジェクト名を取得
		// filePath[5:] は "gs://" を除いた部分
		parts := strings.SplitN(filePath[5:], "/", 2)

		// 修正：バケット名とオブジェクト名が両方存在し、かつ空でないことをチェック
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("無効なGCS URI形式です: %s (gs://bucket-name/object-name の形式で指定してください)", filePath)
		}
		bucketName := parts[0]
		objectName := parts[1]

		// GCS オブジェクトリーダーを作成
		// d.gcsClient を再利用
		rc, err := d.gcsClient.Bucket(bucketName).Object(objectName).NewReader(ctx)
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
