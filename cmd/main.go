package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shouni/action-perfect-get-on-go/pkg/cleaner"
	"github.com/shouni/action-perfect-get-on-go/pkg/scraper"
	"github.com/shouni/action-perfect-get-on-go/pkg/types"
	"github.com/shouni/go-web-exact/pkg/httpclient"
	webextractor "github.com/shouni/go-web-exact/pkg/web"
	"github.com/spf13/cobra"
)

// ----------------------------------------------------------------
// 定数定義 (マジックナンバーの排除)
// ----------------------------------------------------------------

const (
	// initialScrapeDelayは並列スクレイピング後の無条件待機時間です。
	initialScrapeDelay = 2 * time.Second
	retryScrapeDelay   = 5 * time.Second // ★ 修正: コメント削除 ★

	// CLI/Appの実行ステップを表現するための定数
	phaseURLs    = "URL生成フェーズ"
	phaseContent = "コンテンツ取得フェーズ"
	phaseCleanUp = "AIクリーンアップフェーズ"
)

// グローバル変数群 (CLIオプションの値を一時的に保持)
// これらの変数はinit()でcobraフラグにバインドされ、runMainの開始時にcmdOptions構造体に集約されます。
var llmAPIKey string
var llmTimeout time.Duration
var scraperTimeout time.Duration
var urlFile string
var maxScraperParallel int

// cmdOptionsはCLIオプションの値を集約するための構造体です。
// これを関数に渡すことで依存性を明示的にし、テスト容易性を高めます。
type cmdOptions struct {
	LLMAPIKey          string
	LLMTimeout         time.Duration
	ScraperTimeout     time.Duration
	URLFile            string
	MaxScraperParallel int
}

// ----------------------------------------------------------------
// App構造体の導入 (ビジネスロジックの格納)
// ----------------------------------------------------------------

// App はアプリケーションの実行に必要なすべてのロジックをカプセル化します。
type App struct {
	Options cmdOptions
}

// NewApp は cmdOptions を使って App の新しいインスタンスを作成します。
func NewApp(opts cmdOptions) *App {
	return &App{Options: opts}
}

// Execute はアプリケーションの主要な処理フローを実行します。
func (a *App) Execute(ctx context.Context) error {
	// 1. URLの読み込みとバリデーション
	urls, err := generateURLs(a.Options.URLFile)
	if err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", phaseURLs, err)
	}
	log.Printf("INFO: Perfect Get On 処理を開始します。対象URL数: %d個", len(urls))

	// 2. Webコンテンツの取得とリトライ
	successfulResults, err := a.generateContents(ctx, urls)
	if err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", phaseContent, err)
	}

	// 3. AIクリーンアップと出力
	// ★ 修正: generateCleanedOutputをAppメソッドとして呼び出し、APIキーの引数を削除 ★
	if err := a.generateCleanedOutput(ctx, successfulResults); err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", phaseCleanUp, err)
	}

	return nil
}

// ----------------------------------------------------------------
// CLI初期化
// ----------------------------------------------------------------

func init() {
	rootCmd.PersistentFlags().DurationVarP(&llmTimeout, "llm-timeout", "t", 5*time.Minute, "LLM処理のタイムアウト時間")
	rootCmd.PersistentFlags().DurationVarP(&scraperTimeout, "scraper-timeout", "s", 15*time.Second, "WebスクレイピングのHTTPタイムアウト時間")
	rootCmd.PersistentFlags().StringVarP(&llmAPIKey, "api-key", "k", "", "Gemini APIキー (環境変数 GEMINI_API_KEY が優先)")
	rootCmd.PersistentFlags().StringVarP(&urlFile, "url-file", "f", "", "処理対象のURLリストを記載したファイルパス")
	rootCmd.PersistentFlags().IntVarP(&maxScraperParallel, "parallel", "p", scraper.DefaultMaxConcurrency, "Webスクレイピングの最大同時並列リクエスト数")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "action-perfect-get-on-go",
	Short: "複数のURLを並列で取得し、LLMでクリーンアップします。",
	Long: `
実行には、-fまたは--url-fileオプションでURLリストファイルを指定してください。
`,
	RunE: runMain,
}

// runMainはCLIのメインロジックを実行します。Appを初期化し、実行を委譲します。
func runMain(cmd *cobra.Command, args []string) error {
	// CLIオプションを構造体に集約
	opts := cmdOptions{
		LLMAPIKey:          llmAPIKey,
		LLMTimeout:         llmTimeout,
		ScraperTimeout:     scraperTimeout,
		URLFile:            urlFile,
		MaxScraperParallel: maxScraperParallel,
	}

	// LLM処理のコンテキストタイムアウトをフラグ値で設定
	ctx, cancel := context.WithTimeout(cmd.Context(), opts.LLMTimeout)
	defer cancel()

	// App構造体を初期化し、実行ロジックを委譲する
	app := NewApp(opts)
	return app.Execute(ctx)
}

// ----------------------------------------------------------------
// Appのメソッド (コンテンツのスクレイピングとリトライロジック)
// ----------------------------------------------------------------

// generateContentsは、URLリストに対して並列スクレイピングと、失敗したURLに対するリトライを実行します。
func (a *App) generateContents(ctx context.Context, urls []string) ([]types.URLResult, error) {
	log.Println("INFO: フェーズ1 - Webコンテンツの並列抽出を開始します。")

	// 1. 依存性の初期化 (Optionsから設定値を取得)
	httpClient := httpclient.New(a.Options.ScraperTimeout)
	extractor := webextractor.NewExtractor(httpClient)
	s := scraper.NewParallelScraper(extractor, a.Options.MaxScraperParallel)

	// 2. 並列実行
	results := s.ScrapeInParallel(ctx, urls)

	// 3. 無条件遅延
	log.Printf("INFO: 並列抽出が完了しました。次の処理に進む前に %s 待機します。", initialScrapeDelay)
	time.Sleep(initialScrapeDelay) // NOTE: サーバー負荷を考慮した固定遅延。将来的に動的/ランダム遅延へ改善を検討。

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

// AIによるクリーンアップと出力

// generateCleanedOutputは、取得したコンテンツを結合し、LLMでクリーンアップ・構造化して出力します。
// ★ 修正: Appのメソッドにし、LLMAPIKeyをApp.Optionsから取得するように変更 ★
func (a *App) generateCleanedOutput(ctx context.Context, successfulResults []types.URLResult) error {
	// Cleanerの初期化
	// PromptBuilderのコスト削減のため、ここで一度だけ初期化し再利用します。
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

	// APIキーは App.Options から取得
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

// generateURLsはファイルからURLを読み込み、基本的なバリデーションを実行します。
func generateURLs(filePath string) ([]string, error) {
	if filePath == "" {
		return nil, fmt.Errorf("処理対象のURLを指定してください。-f/--url-file オプションでURLリストファイルを指定してください。")
	}

	urls, err := readURLsFromFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("URLファイルの読み込みに失敗しました: %w", err)
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("URLリストファイルに有効なURLが一件も含まれていませんでした。")
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
func processFailedURLs(ctx context.Context, failedURLs []string, extractor *webextractor.Extractor, retryDelay time.Duration) ([]types.URLResult, error) {
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
