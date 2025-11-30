package cleaner

import (
	"time"

	"github.com/shouni/action-perfect-get-on-go/internal/prompts"
)

// DefaultSeparator は、一般的な段落区切りに使用される標準的な区切り文字です。
const DefaultSeparator = "\n\n"

// MaxSegmentChars は、MapフェーズでLLMに一度に渡す安全な最大文字数。
const MaxSegmentChars = 400000

// DefaultMaxMapConcurrency は、Mapフェーズでデフォルトで許可する同時実行数です。
const DefaultMaxMapConcurrency = 2

// DefaultLLMRateLimit は、2000msごとに1リクエストを許可するレートリミットです。
const DefaultLLMRateLimit = 2000 * time.Millisecond

// Segment は、LLMに渡すテキストと、それが由来する元のURLを保持します。
type Segment struct {
	Text string
	URL  string
}

// PromptBuilders は Cleaner が依存する PromptBuilder をまとめています。
type PromptBuilders struct {
	MapBuilder    *prompts.PromptBuilder
	ReduceBuilder *prompts.PromptBuilder
}
