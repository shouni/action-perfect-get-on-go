package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/shouni/action-perfect-get-on-go/pkg/cleaner"
	"github.com/shouni/action-perfect-get-on-go/pkg/scraper"
	"github.com/shouni/action-perfect-get-on-go/pkg/types"
	"github.com/shouni/go-cli-base"
	"github.com/shouni/go-web-exact/v2/pkg/client"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
)

// ----------------------------------------------------------------
// 定数定義
// ----------------------------------------------------------------

const (
	// initialScrapeDelayは並列スクレイピング後の無条件待機時間です。
	// サーバー負荷を考慮した固定遅延。将来的に動的/ランダム遅延へ改善を検討。
	initialScrapeDelay = 2 * time.Second
	retryScrapeDelay   = 5 * time.Second

	phaseURLs             = "URL生成フェーズ"
	phaseContent          = "コンテンツ取得フェーズ"
	phaseCleanUp          = "AIクリーンアップフェーズ"
	defaultHTTPMaxRetries = 2
)

// ----------------------------------------------------------------
// CLIオプションの構造体
// ----------------------------------------------------------------

// cmdOptionsはCLIオプションの値を集約するための構造体です。
type cmdOptions struct {
	LLMAPIKey          string
	LLMTimeout         time.Duration
	ScraperTimeout     time.Duration
	URLFile            string
	MaxScraperParallel int
}

// ----------------------------------------------------------------
// App構造体の導入
// ----------------------------------------------------------------

// App はアプリケーションの実行に必要なすべてのロジックをカプセル化します。
type App struct {
	Options cmdOptions // Appが自身のオプションを持つ
}

// NewApp は cmdOptions を使って App の新しいインスタンスを作成します。
func NewApp(opts cmdOptions) *App {
	return &App{Options: opts}
}

// Execute はアプリケーションの主要な処理フローを実行します。
func (a *App) Execute(ctx context.Context) error {
	urls, err := a.generateURLs()
	if err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", phaseURLs, err)
	}
	log.Printf("INFO: Perfect Get On 処理を開始します。対象URL数: %d個", len(urls))

	successfulResults, err := a.generateContents(ctx, urls)
	if err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", phaseContent, err)
	}

	if err := a.generateCleanedOutput(ctx, successfulResults); err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", phaseCleanUp, err)
	}

	return nil
}

// ----------------------------------------------------------------
// CLIエントリポイント (go-cli-baseを流用)
// ----------------------------------------------------------------

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

func init() {
	// フラグを cobra.Command に直接定義
	runCmd.Flags().DurationP("llm-timeout", "t", 5*time.Minute, "LLM処理のタイムアウト時間")
	runCmd.Flags().DurationP("scraper-timeout", "s", 15*time.Second, "WebスクレイピングのHTTPタイムアウト時間")
	runCmd.Flags().StringP("api-key", "k", "", "Gemini APIキー (環境変数 GEMINI_API_KEY が優先)")
	runCmd.Flags().StringP("url-file", "f", "", "処理対象のURLリストを記載したファイルパス")
	runCmd.Flags().IntP("parallel", "p", scraper.DefaultMaxConcurrency, "Webスクレイピングの最大同時並列リクエスト数")

	runCmd.MarkFlagRequired("url-file")
}

func main() {
	clibase.Execute("action-perfect-get-on-go", runCmd)
}

// runMainLogicはCLIのメインロジックを実行し、フラグをAppに渡します。
func runMainLogic(cmd *cobra.Command, args []string) error {
	// ⬇️ 修正: フラグ値取得時のエラーハンドリングを追加し、堅牢化
	llmTimeout, err := cmd.Flags().GetDuration("llm-timeout")
	if err != nil {
		return fmt.Errorf("failed to get llm-timeout flag: %w", err)
	}
	scraperTimeout, err := cmd.Flags().GetDuration("scraper-timeout")
	if err != nil {
		return fmt.Errorf("failed to get scraper-timeout flag: %w", err)
	}
	llmAPIKey, err := cmd.Flags().GetString("api-key")
	if err != nil {
		return fmt.Errorf("failed to get api-key flag: %w", err)
	}
	urlFile, err := cmd.Flags().GetString("url-file")
	if err != nil {
		return fmt.Errorf("failed to get url-file flag: %w", err)
	}
	maxScraperParallel, err := cmd.Flags().GetInt("parallel")
	if err != nil {
		return fmt.Errorf("failed to get parallel flag: %w", err)
	}

	opts := cmdOptions{
		LLMAPIKey:          llmAPIKey,
		LLMTimeout:         llmTimeout,
		ScraperTimeout:     scraperTimeout,
		URLFile:            urlFile,
		MaxScraperParallel: maxScraperParallel,
	}

	// グローバルタイムアウト設定
	ctx, cancel := context.WithTimeout(cmd.Context(), opts.LLMTimeout)
	defer cancel()

	app := NewApp(opts)
	return app.Execute(ctx)
}

// ----------------------------------------------------------------
// Appのメソッド (a.Optionsを参照)
// ----------------------------------------------------------------

// generateURLsはファイルからURLを読み込み、基本的なバリデーションを実行します。
func (a *App) generateURLs() ([]string, error) {
	if a.Options.URLFile == "" {
		return nil, fmt.Errorf("処理対象のURLを指定してください。-f/--url-file オプションでURLリストファイルを指定してください。")
	}

	urls, err := readURLsFromFile(a.Options.URLFile)
	if err != nil {
		return nil, fmt.Errorf("URLファイルの読み込みに失敗しました: %w", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("URLリストファイルに有効なURLが一件も含まれていませんでした。")
	}
	return urls, nil
}

// generateContentsは、URLリストに対して並列スクレイピングと、失敗したURLに対するリトライを実行します。
func (a *App) generateContents(ctx context.Context, urls []string) ([]types.URLResult, error) {
	log.Println("INFO: フェーズ1 - Webコンテンツの並列抽出を開始します。")

	// 1. 依存性の初期化 (Optionsから設定値を取得)
	clientOptions := []client.ClientOption{
		client.WithMaxRetries(defaultHTTPMaxRetries),
	}
	webClient := client.New(a.Options.ScraperTimeout, clientOptions...)

	// 新しいクライアント (Fetcher) を Extractor に注入
	extractor, err := extract.NewExtractor(webClient)
	if err != nil {
		// NOTE: webClient は client.New() から常に有効なインスタンスが返されるため、
		// ここでエラーが発生する場合は extract パッケージ内部の初期化失敗が考えられます。
		return nil, fmt.Errorf("Extractorの初期化に失敗しました: %w", err)
	}

	s := scraper.NewParallelScraper(extractor, a.Options.MaxScraperParallel)

	// 2. 並列実行
	results := s.ScrapeInParallel(ctx, urls)

	// 3. 無条件遅延
	log.Printf("INFO: 並列抽出が完了しました。次の処理に進む前に %s 待機します。", initialScrapeDelay)
	time.Sleep(initialScrapeDelay)

	// 4. 結果の分類
	successfulResults, failedURLs := classifyResults(results)
	initialSuccessfulCount := len(successfulResults)

	// 5. 失敗URLのリトライ処理
	if len(failedURLs) > 0 {
		retriedSuccessfulResults, retryErr := processFailedURLs(ctx, failedURLs, extractor, retryScrapeDelay)
		if retryErr != nil {
			log.Printf("WARNING: 失敗URLのリトライ処理中にエラーが発生しました: %v", retryErr)
		}
		successfulResults = append(successfulResults, retriedSuccessfulResults...)
	}

	// 6. 最終チェックとログ
	if len(successfulResults) == 0 {
		return nil, fmt.Errorf("処理可能なWebコンテンツを一件も取得できませんでした。URLを確認してください。")
	}

	log.Printf("INFO: 最終成功数: %d/%d URL (初期成功: %d, リトライ成功: %d)",
		len(successfulResults), len(urls), initialSuccessfulCount, len(successfulResults)-initialSuccessfulCount)

	return successfulResults, nil
}

// generateCleanedOutputは、取得したコンテンツを結合し、LLMでクリーンアップ・構造化して出力します。
func (a *App) generateCleanedOutput(ctx context.Context, successfulResults []types.URLResult) error {
	// Cleanerの初期化
	c, err := cleaner.NewCleaner()
	if err != nil {
		return fmt.Errorf("Cleanerの初期化に失敗しました: %w", err)
	}

	// データ結合フェーズ
	log.Println("INFO: フェーズ2 - 抽出結果の結合を開始します。")
	combinedText := cleaner.CombineContents(successfulResults)
	log.Printf("INFO: 結合されたテキストの長さ: %dバイト", len(combinedText))

	// AIクリーンアップフェーズ (LLM)
	log.Println("INFO: フェーズ3 - LLMによるテキストのクリーンアップと構造化を開始します (Go-AI-Client利用)。")

	cleanedText, err := c.CleanAndStructureText(ctx, combinedText, a.Options.LLMAPIKey)
	if err != nil {
		return fmt.Errorf("LLMクリーンアップ処理に失敗しました: %w", err)
	}

	// 最終結果の出力
	fmt.Println("\n===============================================")
	fmt.Println("✅ PERFECT GET ON: LLMクリーンアップ後の最終出力データ:")
	fmt.Println("===============================================")
	fmt.Println(cleanedText)
	fmt.Println("===============================================")

	return nil
}

// ----------------------------------------------------------------
// ヘルパー関数 (変更なし)
// ----------------------------------------------------------------

// readURLsFromFileは指定されたファイルからURLを読み込みます。
func readURLsFromFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
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

// classifyResultsは並列抽出の結果を成功と失敗に分類します。
func classifyResults(results []types.URLResult) (successfulResults []types.URLResult, failedURLs []string) {
	for _, res := range results {
		if res.Error != nil || res.Content == "" {
			failedURLs = append(failedURLs, res.URL)
		} else {
			successfulResults = append(successfulResults, res)
		}
	}
	return successfulResults, failedURLs
}

// formatErrorLogは、冗長なエラーメッセージ（HTMLボディなどを含むもの）をステータスコード情報のみに短縮します。
func formatErrorLog(err error) string {
	errMsg := err.Error()
	if idx := strings.Index(errMsg, ", ボディ: <!"); idx != -1 {
		errMsg = errMsg[:idx]
	}

	if idx := strings.LastIndex(errMsg, "最終エラー:"); idx != -1 {
		return strings.TrimSpace(errMsg[idx:])
	}

	return errMsg
}

// processFailedURLsは、失敗したURLに対し、指定された遅延時間後に順次リトライを実行します。
func processFailedURLs(ctx context.Context, failedURLs []string, extractor *extract.Extractor, retryDelay time.Duration) ([]types.URLResult, error) {
	log.Printf("WARNING: 抽出に失敗したURLが %d 件ありました。%s待機後、順次リトライを開始します。", len(failedURLs), retryDelay)
	time.Sleep(retryDelay)

	var retriedSuccessfulResults []types.URLResult
	log.Println("INFO: 失敗URLの順次リトライを開始します。")

	for _, url := range failedURLs {
		log.Printf("INFO: リトライ中: %s", url)

		content, hasBodyFound, err := extractor.FetchAndExtractText(url, ctx)

		var extractErr error
		if err != nil {
			extractErr = fmt.Errorf("コンテンツの抽出に失敗しました: %w", err)
		} else if content == "" || !hasBodyFound {
			extractErr = fmt.Errorf("URL %s から有効な本文を抽出できませんでした", url)
		}

		if extractErr != nil {
			formattedErr := formatErrorLog(extractErr)
			log.Printf("ERROR: リトライでも %s の抽出に失敗しました: %s", url, formattedErr)
		} else {
			log.Printf("INFO: SUCCESS: %s の抽出がリトライで成功しました。", url)
			retriedSuccessfulResults = append(retriedSuccessfulResults, types.URLResult{
				URL:     url,
				Content: content,
				Error:   nil,
			})
		}
	}
	return retriedSuccessfulResults, nil
}
