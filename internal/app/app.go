package app

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shouni/action-perfect-get-on-go/pkg/cleaner"
	"github.com/shouni/action-perfect-get-on-go/pkg/iohandler"

	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
	extScraper "github.com/shouni/go-web-exact/v2/pkg/scraper"
	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
)

// ----------------------------------------------------------------
// 定数定義
// ----------------------------------------------------------------

const (
	// initialScrapeDelayは並列スクレイピング後の無条件待機時間です。
	initialScrapeDelay = 2 * time.Second
	retryScrapeDelay   = 5 * time.Second

	phaseURLs    = "URL生成フェーズ"
	phaseContent = "コンテンツ取得フェーズ"
	phaseCleanUp = "AIクリーンアップフェーズ"

	defaultHTTPMaxRetries = 2
)

// ----------------------------------------------------------------
// CLIオプションの構造体 (cmdパッケージから利用するためExport)
// ----------------------------------------------------------------

// CmdOptionsはCLIオプションの値を集約するための構造体です。
type CmdOptions struct {
	LLMAPIKey          string
	LLMTimeout         time.Duration
	ScraperTimeout     time.Duration
	URLFile            string
	OutputFilePath     string
	MaxScraperParallel int
}

// ----------------------------------------------------------------
// App構造体の導入
// ----------------------------------------------------------------

// App はアプリケーションの実行に必要なすべてのロジックをカプセル化します。
type App struct {
	Options CmdOptions // Appが自身のオプションを持つ
}

// NewApp は CmdOptions を使って App の新しいインスタンスを作成します。
func NewApp(opts CmdOptions) *App {
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

	// generateCleanedOutputを呼び出し、クリーンアップされたテキストを取得
	cleanedText, err := a.generateCleanedOutput(ctx, successfulResults)
	if err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", phaseCleanUp, err)
	}

	if err := iohandler.WriteOutputString(a.Options.OutputFilePath, cleanedText); err != nil {
		return fmt.Errorf("最終結果の出力に失敗しました: %w", err)
	}
	log.Println("INFO: 処理が正常に完了しました。")

	return nil
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
func (a *App) generateContents(ctx context.Context, urls []string) ([]extTypes.URLResult, error) {
	log.Println("INFO: フェーズ1 - Webコンテンツの並列抽出を開始します。")

	// 1. 依存性の初期化
	clientOptions := []httpkit.ClientOption{
		httpkit.WithMaxRetries(defaultHTTPMaxRetries), // httpkitレベルのリトライ
	}
	webClient := httpkit.New(a.Options.ScraperTimeout, clientOptions...)

	extractor, err := extract.NewExtractor(webClient)
	if err != nil {
		return nil, fmt.Errorf("Extractorの初期化に失敗しました: %w", err)
	}

	s := extScraper.NewParallelScraper(extractor, a.Options.MaxScraperParallel)

	// 2. 並列実行
	results := s.ScrapeInParallel(ctx, urls)

	// 3. 無条件遅延 (負荷軽減) を復活
	log.Printf("INFO: 並列抽出が完了しました。次の処理に進む前に %s 待機します。", initialScrapeDelay)
	time.Sleep(initialScrapeDelay)

	// 4. 結果の分類
	successfulResults, failedURLs := classifyResults(results)
	initialSuccessfulCount := len(successfulResults)

	// 5. 失敗URLの上位レベルリトライ処理を復活
	if len(failedURLs) > 0 {
		// リトライ処理では、extractor（内部にfetcher/httpkitクライアントを持つ）を利用
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

// generateCleanedOutputは、取得したコンテンツを結合し、LLMでクリーンアップ・構造化します。
func (a *App) generateCleanedOutput(ctx context.Context, successfulResults []extTypes.URLResult) (string, error) {
	// Cleanerの初期化
	c, err := cleaner.NewCleaner()
	if err != nil {
		return "", fmt.Errorf("Cleanerの初期化に失敗しました: %w", err)
	}

	// データ結合とURL抽出の処理は、cleanerパッケージ内部に移動しました。
	log.Printf("INFO: フェーズ2 - 抽出結果 (%d件) を基に、AIクリーンアップと構造化を開始します。", len(successfulResults))

	// AIクリーンアップフェーズ (LLM)
	log.Println("INFO: フェーズ3 - LLMによるテキストのクリーンアップと構造化を開始します (Go-AI-Client利用)。")

	// 引数を変更: combinedTextとsourceURLsを削除し、successfulResultsを直接渡す
	cleanedText, err := c.CleanAndStructureText(ctx, successfulResults, a.Options.LLMAPIKey)
	if err != nil {
		return "", fmt.Errorf("LLMクリーンアップ処理に失敗しました: %w", err)
	}

	return cleanedText, nil
}

// ----------------------------------------------------------------
// ヘルパー関数 (iohandlerと競合するため、ReadInput/WriteOutputは削除)
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
func classifyResults(results []extTypes.URLResult) (successfulResults []extTypes.URLResult, failedURLs []string) {
	for _, res := range results {
		// extTypes.URLResult に合わせて修正
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
func processFailedURLs(ctx context.Context, failedURLs []string, extractor *extract.Extractor, retryDelay time.Duration) ([]extTypes.URLResult, error) {
	log.Printf("WARNING: 抽出に失敗したURLが %d 件ありました。%s待機後、順次リトライを開始します。", len(failedURLs), retryDelay)
	time.Sleep(retryDelay)

	var retriedSuccessfulResults []extTypes.URLResult
	log.Println("INFO: 失敗URLの順次リトライを開始します。")

	for _, url := range failedURLs {
		log.Printf("INFO: リトライ中: %s", url)

		// FetchAndExtractText は go-web-exact/v2 の extract.Extractor のメソッド
		content, hasBodyFound, err := extractor.FetchAndExtractText(ctx, url)

		var extractErr error
		if err != nil {
			extractErr = fmt.Errorf("コンテンツの抽出に失敗しました: %w", err)
		} else if content == "" || !hasBodyFound {
			extractErr = fmt.Errorf("URL %s から有効な本文を抽出できませんでした", url)
		}

		if extractErr != nil {
			formattedErr := formatErrorLog(extractErr) // formatErrorLogで整形
			log.Printf("ERROR: リトライでも %s の抽出に失敗しました: %s", url, formattedErr)
		} else {
			log.Printf("INFO: SUCCESS: %s の抽出がリトライで成功しました。", url)
			retriedSuccessfulResults = append(retriedSuccessfulResults, extTypes.URLResult{
				URL:     url,
				Content: content,
				Error:   nil,
			})
		}
	}
	return retriedSuccessfulResults, nil
}
