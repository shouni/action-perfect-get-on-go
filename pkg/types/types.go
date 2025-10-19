package types

// URLResult は個々のURLの抽出結果を格納する構造体
// 複数のパッケージで共有されます。
type URLResult struct {
	URL     string
	Content string // 抽出された本文
	Error   error
}
