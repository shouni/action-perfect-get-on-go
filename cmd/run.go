package cmd

import (
	"context"
	"fmt"
	"time"

	"action-perfect-get-on-go/internal/builder"
	"action-perfect-get-on-go/internal/pipeline"

	"github.com/spf13/cobra"
)

// パイプライン全体の最大実行時間。個別のLLM/スクレイピングタイムアウトとは別に、全体の上限を設ける。
const defaultContextTimeout = 30 * time.Minute

// Mapフェーズ (中間要約) のデフォルトモデル: 速度とコストを優先
const defaultMapModelName = "gemini-2.5-flash"

// Reduceフェーズ (最終構造化) のデフォルトモデル: 品質と論理性を優先
const defaultReduceModelName = "gemini-2.5-pro"

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
	runCmd.Flags().String("map-model", defaultMapModelName, "Mapフェーズ に使用するAIモデル名")
	runCmd.Flags().String("reduce-model", defaultReduceModelName, "Reduceフェーズ に使用するAIモデル名")

	runCmd.MarkFlagRequired("url-file")
}

// newCmdOptionsFromFlags は cobra.Command のフラグから CmdOptions 構造体を生成します。
// これにより、runMainLogic のフラグ取得ロジックが簡潔になります。
func newCmdOptionsFromFlags(cmd *cobra.Command) (pipeline.CmdOptions, error) {
	llmTimeout, err := cmd.Flags().GetDuration("llm-timeout")
	if err != nil {
		return pipeline.CmdOptions{}, fmt.Errorf("llm-timeoutフラグの取得に失敗しました: %w", err)
	}
	scraperTimeout, err := cmd.Flags().GetDuration("scraper-timeout")
	if err != nil {
		return pipeline.CmdOptions{}, fmt.Errorf("scraper-timeoutフラグの取得に失敗しました: %w", err)
	}
	llmAPIKey, err := cmd.Flags().GetString("api-key")
	if err != nil {
		return pipeline.CmdOptions{}, fmt.Errorf("api-keyフラグの取得に失敗しました: %w", err)
	}
	urlFile, err := cmd.Flags().GetString("url-file")
	if err != nil {
		return pipeline.CmdOptions{}, fmt.Errorf("url-fileフラグの取得に失敗しました: %w", err)
	}
	outputFilePath, err := cmd.Flags().GetString("output")
	if err != nil {
		return pipeline.CmdOptions{}, fmt.Errorf("outputフラグの取得に失敗しました: %w", err)
	}
	maxScraperParallel, err := cmd.Flags().GetInt("parallel")
	if err != nil {
		return pipeline.CmdOptions{}, fmt.Errorf("parallelフラグの取得に失敗しました: %w", err)
	}

	if maxScraperParallel < 1 {
		return pipeline.CmdOptions{}, fmt.Errorf("--parallel には1以上の値を指定する必要があります")
	}

	mapModel, err := cmd.Flags().GetString("map-model")
	if err != nil {
		return pipeline.CmdOptions{}, fmt.Errorf("map-modelフラグの取得に失敗しました: %w", err)
	}
	reduceModel, err := cmd.Flags().GetString("reduce-model")
	if err != nil {
		return pipeline.CmdOptions{}, fmt.Errorf("reduce-modelフラグの取得に失敗しました: %w", err)
	}

	if mapModel == "" {
		return pipeline.CmdOptions{}, fmt.Errorf("--map-model には空でないAIモデル名を指定する必要があります")
	}
	if reduceModel == "" {
		return pipeline.CmdOptions{}, fmt.Errorf("--reduce-model には空でないAIモデル名を指定する必要があります")
	}
	// 構造体の初期化
	opts := pipeline.CmdOptions{
		LLMAPIKey:          llmAPIKey,
		LLMTimeout:         llmTimeout,
		ScraperTimeout:     scraperTimeout,
		URLFile:            urlFile,
		OutputFilePath:     outputFilePath,
		MaxScraperParallel: maxScraperParallel,
		MapModel:           mapModel,
		ReduceModel:        reduceModel,
	}

	return opts, nil
}

// runMainLogicはCLIのメインロジックを実行し、フラグをAppに渡します。
// フラグ取得処理は newCmdOptionsFromFlags に抽出されています。
func runMainLogic(cmd *cobra.Command, args []string) error {
	// 1. フラグからオプション構造体を生成する処理をヘルパー関数に委譲
	opts, err := newCmdOptionsFromFlags(cmd)
	if err != nil {
		return err // フラグ取得エラーを直接返す
	}

	// LLMTimeout を含む、パイプライン全体の実行コンテキストを作成
	ctx, cancel := context.WithTimeout(cmd.Context(), defaultContextTimeout)
	defer cancel()

	// 2. パイプラインの構築
	p, closer, err := builder.BuildPipeline(ctx, opts)
	if err != nil {
		// パイプライン構築が失敗した場合（例: Extractor初期化失敗など）
		return fmt.Errorf("パイプラインの構築に失敗しました: %w", err)
	}

	// GCSクライアントを含むすべてのリソースを確実にクローズする
	defer closer()

	// 3. パイプラインの実行
	if err := p.Execute(ctx); err != nil {
		return fmt.Errorf("パイプラインの実行中にエラーが発生しました: %w", err)
	}

	return nil
}
