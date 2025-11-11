package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"cloud.google.com/go/storage"
)

// LocalGCSInputReader は InputReader の具象実装であり、
// ローカルファイルと GCS オブジェクトの読み込みを処理します。
type LocalGCSInputReader struct {
	gcsClient *storage.Client
}

// NewLocalGCSInputReader は LocalGCSInputReader の新しいインスタンスを作成します。
func NewLocalGCSInputReader(gcsClient *storage.Client) *LocalGCSInputReader {
	return &LocalGCSInputReader{
		gcsClient: gcsClient,
	}
}

// Open は、ファイルパスを検査し、ローカルファイルまたはGCSからストリームを開きます。
func (r *LocalGCSInputReader) Open(ctx context.Context, filePath string) (io.ReadCloser, error) {
	if strings.HasPrefix(filePath, "gs://") {
		return r.openGCSObject(ctx, filePath)
	}
	// ローカルファイルパスの処理
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("ローカルファイルのオープンに失敗しました: %w", err)
	}
	return file, nil
}

// openGCSObject は、GCS URI からオブジェクトを読み込み、io.ReadCloser を返します。
func (r *LocalGCSInputReader) openGCSObject(ctx context.Context, gcsURI string) (io.ReadCloser, error) {
	if r.gcsClient == nil {
		return nil, fmt.Errorf("GCS URIが指定されましたが、GCSクライアントが初期化されていません。")
	}

	// URIのパースロジック
	path := gcsURI[5:]
	parts := strings.SplitN(path, "/", 2)

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("無効なGCS URI形式です: %s (gs://bucket-name/object-name の形式で指定してください)", gcsURI)
	}
	bucketName := parts[0]
	objectName := parts[1]

	// GCS オブジェクトリーダーを作成
	rc, err := r.gcsClient.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("GCSファイルの読み込みに失敗しました (URI: %s): %w", gcsURI, err)
	}
	return rc, nil
}
