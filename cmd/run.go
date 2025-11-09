package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/shouni/action-perfect-get-on-go/internal/builder"
	"github.com/shouni/action-perfect-get-on-go/internal/pipeline"
	"github.com/spf13/cobra"
)

// runCmd は、メインのCLIコマンド定義です。
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Webコンテンツの取得とAIクリーンアップを実行します。",
	Long: `
Webコンテンツの取得とAIクリーンアップを実行します。
実行には、-fまたは--url-fileオプションでURLリストファイルを指定してください。

-oまたは--outputオプションで出力ファイルパスを指定すると、ファイルに書き込まれ、
標準出力には冒頭のプレビューが表示されます。指定しない場合は標準出力に出力されます。
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
	runCmd.Flags().StringP("output", "o", "./output/output_reduce_final.md", "最終的な構造化Markdownを出力するファイルパス (省略時は標準出力)")
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
	outputFilePath, err := cmd.Flags().GetString("output")
	if err != nil {
		return fmt.Errorf("outputフラグの取得に失敗しました: %w", err)
	}
	maxScraperParallel, err := cmd.Flags().GetInt("parallel")
	if err != nil {
		return fmt.Errorf("parallelフラグの取得に失敗しました: %w", err)
	}

	opts := pipeline.CmdOptions{
		LLMAPIKey:          llmAPIKey,
		LLMTimeout:         llmTimeout,
		ScraperTimeout:     scraperTimeout,
		URLFile:            urlFile,
		OutputFilePath:     outputFilePath,
		MaxScraperParallel: maxScraperParallel,
	}
	// 1. パイプラインの構築
	p, err := builder.BuildPipeline(opts)
	if err != nil {
		// パイプライン構築が失敗した場合（例: Extractor初期化失敗など）
		return fmt.Errorf("パイプラインの構築に失敗しました: %w", err)
	}

	// 2. パイプラインの実行
	// Execute は、内部のすべての処理（URL生成、コンテンツ取得、クリーンアップ、ファイル出力）からのエラーをラップして返します。
	if err := p.Execute(context.Background()); err != nil {
		// パイプライン実行中の任意のステージでエラーが発生した場合
		return fmt.Errorf("パイプラインの実行中にエラーが発生しました: %w", err)
	}

	return nil
}
