package cleaner

import (
	"context"
	"fmt"
	"strings"

	"action-perfect-get-on-go/pkg/types"

	gemini "github.com/shouni/go-ai-client/pkg/ai/gemini"
)

// ContentSeparator は、結合された複数の文書間を区切るための明確な区切り文字です。
const ContentSeparator = "\n\n--- DOCUMENT END ---\n\n"

// CombineContents は、成功した抽出結果の本文を効率的に結合します。
// 各コンテンツの前には、ソースURL情報が付加されます。
func CombineContents(results []types.URLResult) string {
	var builder strings.Builder

	// 各コンテンツを結合し、明確な区切り文字を入れる
	for i, res := range results {
		// URLを追記することで、LLMがどのソースのテキストであるかを識別できるようにする
		builder.WriteString(fmt.Sprintf("--- SOURCE URL %d: %s ---\n", i+1, res.URL))
		builder.WriteString(res.Content)

		// 最後の文書でなければ区切り文字を追加
		if i < len(results)-1 {
			builder.WriteString(ContentSeparator)
		}
	}

	return builder.String()
}

// CleanAndStructureText は結合されたテキストをLLMで処理し、
// 重複排除と論理的な構造化を実行したクリーンなテキストを返します。
func CleanAndStructureText(ctx context.Context, combinedText string) (string, error) {
	// 1. LLMクライアントの初期化 (APIキーを環境変数から読み込むヘルパーを使用)
	client, err := gemini.NewClientFromEnv(ctx)
	if err != nil {
		return "", fmt.Errorf("LLMクライアントの初期化に失敗しました: %w", err)
	}

	// 2. LLMに渡すためのプロンプトを構築
	prompt := buildCleaningPrompt(combinedText)

	// 3. LLM APIを呼び出し（モデル名は構造化処理に適したものを指定）
	response, err := client.GenerateContent(ctx, prompt, "gemini-2.5-flash")
	if err != nil {
		return "", fmt.Errorf("LLM APIの呼び出しに失敗しました: %w", err)
	}

	return response.Text, nil
}

// buildCleaningPrompt はLLMに明確な指示を与えるためのプロンプトを生成します。
func buildCleaningPrompt(combinedText string) string {
	var sb strings.Builder
	sb.WriteString("以下のテキストは、複数のウェブページから抽出されたものです。")
	sb.WriteString("あなたのタスクは、これらの情報を完璧に、迅速に、論理的に構造化することです。\n\n")
	sb.WriteString("--- 処理指示 ---\n")
	sb.WriteString("1. **重複排除**: 複数のソースで繰り返されている情報を識別し、最も完全で正確な情報を残し、重複を徹底的に排除してください。\n")
	sb.WriteString("2. **論理的な構造化**: 情報をトピックやセクションごとに整理し、ヘッダー（Markdown形式: #, ##, ...）を使って読みやすい構造にしてください。\n")
	sb.WriteString("3. **ノイズ除去**: 不要なフッター、ナビゲーション、広告、または空のセクションはすべて削除してください。\n")
	sb.WriteString("4. **出力形式**: 出力は、Markdown形式のクリーンなテキストのみとし、追加の説明や感想は一切含めないでください。\n")
	sb.WriteString("5. **情報源の引用**: 各情報の最後に、どのソースURL（--- SOURCE URL ... ---）から引用したかを明示的に示す必要はありません。完全に統合されたテキストとしてください。\n")
	sb.WriteString("----------------\n\n")
	sb.WriteString("--- 入力結合テキスト ---\n")
	sb.WriteString(combinedText)
	sb.WriteString("\n------------------------\n\n")
	sb.WriteString("✅ 処理結果を出力してください:")

	return sb.String()
}
