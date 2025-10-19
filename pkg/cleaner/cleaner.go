package cleaner

import (
	"context"
	"fmt"
	"log"
	"strings"

	"action-perfect-get-on-go/pkg/types"
	gemini "github.com/shouni/go-ai-client/pkg/ai/gemini"
)

// ContentSeparator は、結合された複数の文書間を区切るための明確な区切り文字です。
const ContentSeparator = "\n\n--- DOCUMENT END ---\n\n"

// MaxInputChars は、LLMに渡すテキストの最大許容文字数（バイト数ではありません）。
// これは、APIのトークン制限（gemini-2.5-flashは200万トークン）に安全なマージンを設けた推定値です。
// 安全のため、約10万トークン（40万文字）を上限とします。
const MaxInputChars = 400000

// CombineContents は、成功した抽出結果の本文を効率的に結合します。
// 各コンテンツの前には、ソースURL情報が付加されます。
func CombineContents(results []types.URLResult) string {
	var builder strings.Builder

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
// api_key_override は、コマンドライン引数で渡されたAPIキーです。
// 環境変数よりもこちらが優先されます。
func CleanAndStructureText(ctx context.Context, combinedText string, apiKeyOverride string) (string, error) {

	// 1. LLM入力テキストのサイズ制限チェック (クリティカルな修正)
	// rune (文字) ベースで長さを取得し、トークン超過によるAPIエラーを防ぐ
	inputRunes := []rune(combinedText)
	if len(inputRunes) > MaxInputChars {
		log.Printf("⚠️ WARNING: 結合されたテキストが大きすぎます（%d文字 > 制限 %d文字）。安全のため、後方の情報を切り詰めます。", len(inputRunes), MaxInputChars)

		// 警告メッセージを冒頭に追加
		warningMsg := "【WARNING: 入力テキストが大きすぎたため、後方の情報を切り捨てました。】\n\n"

		// 安全な長さで切り詰め
		safeText := warningMsg + string(inputRunes[:MaxInputChars])
		combinedText = safeText
	}

	// 2. LLMクライアントの初期化 (APIキーの柔軟性向上)
	var client *gemini.Client
	var err error

	if apiKeyOverride != "" {
		// CLIオプションでキーが渡された場合、それを優先して使用
		client, err = gemini.NewClient(ctx, gemini.Config{APIKey: apiKeyOverride})
	} else {
		// CLIオプションがない場合、環境変数から読み込みを試みる
		client, err = gemini.NewClientFromEnv(ctx)
	}

	if err != nil {
		// APIキーがない、またはクライアント作成に失敗した場合
		return "", fmt.Errorf("LLMクライアントの初期化に失敗しました。APIキー（--api-keyまたは環境変数）が設定されているか確認してください: %w", err)
	}

	// 3. LLMに渡すためのプロンプトを構築
	prompt := buildCleaningPrompt(combinedText)

	// 4. LLM APIを呼び出し
	// モデル名は構造化処理に適したものを指定
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
