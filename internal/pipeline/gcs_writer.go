package pipeline

import (
	"context"
	"fmt"

	"cloud.google.com/go/storage"
)

// GCSFileWriter は GCSOutputWriter インターフェースの具象実装です。
type GCSFileWriter struct {
	client *storage.Client
}

// NewGCSFileWriter は新しい GCSFileWriter インスタンスを作成します。
func NewGCSFileWriter(client *storage.Client) *GCSFileWriter {
	return &GCSFileWriter{client: client}
}

// WriteToGCS は指定されたバケットとパスにコンテンツを書き込みます。
// これは GCSOutputWriter インターフェースを満たします。
func (w *GCSFileWriter) WriteToGCS(ctx context.Context, bucketName, objectPath string, content string) error {
	// バケットとオブジェクトの参照を取得
	bucket := w.client.Bucket(bucketName)
	obj := bucket.Object(objectPath)

	// Writerを取得し、コンテキストを使用してタイムアウトやキャンセルを処理可能にする
	wc := obj.NewWriter(ctx)

	// オブジェクトのメタデータやACLを設定する必要がある場合は、ここでwcに設定します
	wc.ContentType = "text/markdown"

	// 書き込み
	if _, err := wc.Write([]byte(content)); err != nil {
		wc.Close() // 書き込みエラー時は必ず閉じる
		return fmt.Errorf("GCSへのコンテンツ書き込みに失敗しました: %w", err)
	}

	// Writerを閉じる (これが実際のアップロードをトリガーします)
	if err := wc.Close(); err != nil {
		return fmt.Errorf("GCS Writerのクローズに失敗しました (アップロード失敗): %w", err)
	}

	return nil
}

// 型アサーションチェック
var _ GCSOutputWriter = (*GCSFileWriter)(nil)
