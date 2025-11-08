package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/shouni/action-perfect-get-on-go/internal/app"
	"github.com/spf13/cobra"
)

// runCmd は、メインのCLIコマンド定義です。
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Webコンテンツの取得とAIクリーンアップを実行します。",
	Long: `
Webコンテンツの取得とAIクリーンアップを実行します。
実行には、-fまたは--url-fileオプションでURLリストファイルを指定してください。
`,
	RunE: runMainLogic,
}

// init関数でサブコマンド固有のフラグを定義します。
func init() {
	// フラグを cobra.Command に直接定義
	runCmd.Flags().DurationP("llm-timeout", "t", 5*time.Minute, "LLM処理のタイムアウト時間")
	runCmd.Flags().DurationP("scraper-timeout", "s", 15*time.Second, "WebスクレイピングのHTTPタイムアウト時間")
	runCmd.Flags().StringP("api-key", "k", "", "Gemini APIキー (環境変数 GEMINI_API_KEY が優先)")
	runCmd.Flags().StringP("url-file", "f", "", "処理対象のURLリストを記載したファイルパス")
	runCmd.Flags().IntP("parallel", "p", 5, "Webスクレイピングの最大同時並列リクエスト数")
	runCmd.MarkFlagRequired("url-file")
}

// runMainLogicはCLIのメインロジックを実行し、フラグをAppに渡します。
func runMainLogic(cmd *cobra.Command, args []string) error {
	// フラグから値を取得し、エラーチェック
	llmTimeout, err := cmd.Flags().GetDuration("llm-timeout")
	if err != nil {
		return fmt.Errorf("llm-timeoutフラグの取得に失敗しました: %w", err)
	}
	scraperTimeout, err := cmd.Flags().GetDuration("scraper-timeout")
	if err != nil {
		return fmt.Errorf("scraper-timeoutフラグの取得に失敗しました: %w", err)
	}
	llmAPIKey, err := cmd.Flags().GetString("api-key")
	if err != nil {
		return fmt.Errorf("api-keyフラグの取得に失敗しました: %w", err)
	}
	urlFile, err := cmd.Flags().GetString("url-file")
	if err != nil {
		return fmt.Errorf("url-fileフラグの取得に失敗しました: %w", err)
	}
	maxScraperParallel, err := cmd.Flags().GetInt("parallel")
	if err != nil {
		return fmt.Errorf("parallelフラグの取得に失敗しました: %w", err)
	}

	opts := app.CmdOptions{
		LLMAPIKey:          llmAPIKey,
		LLMTimeout:         llmTimeout,
		ScraperTimeout:     scraperTimeout,
		URLFile:            urlFile,
		MaxScraperParallel: maxScraperParallel,
	}

	// グローバルタイムアウト設定
	// runCmdのContext()は既にキャンセル可能なContextを持っている可能性がありますが、
	// ここでLLMTimeoutに基づくタイムアウトを設定し直します。
	ctx, cancel := context.WithTimeout(cmd.Context(), opts.LLMTimeout)
	defer cancel()

	application := app.NewApp(opts)
	return application.Execute(ctx)
}
