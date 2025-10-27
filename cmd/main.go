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

	"github.com/spf13/cobra"
)

// ----------------------------------------------------------------
// グローバル変数と初期設定 (変更なし)
// ----------------------------------------------------------------

// コマンドラインオプションのグローバル変数
var llmAPIKey string
var llmTimeout time.Duration
var scraperTimeout time.Duration
var urlFile string

func init() {
	rootCmd.PersistentFlags().DurationVarP(&llmTimeout, "llm-timeout", "t", 5*time.Minute, "LLM処理のタイムアウト時間")
	rootCmd.PersistentFlags().DurationVarP(&scraperTimeout, "scraper-timeout", "s", 15*time.Second, "WebスクレイピングのHTTPタイムアウト時間")
	rootCmd.PersistentFlags().StringVarP(&llmAPIKey, "api-key", "k", "", "Gemini APIキー (環境変数 GEMINI_API_KEY が優先)")
	rootCmd.PersistentFlags().StringVarP(&urlFile, "url-file", "f", "", "処理対象のURLリストを記載したファイルパス")
}

// プログラムのエントリーポイント
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

// ----------------------------------------------------------------
// メインオーケストレーター
// ----------------------------------------------------------------

// runMain は CLIのメインロジックを実行します。実行ステップを管理するオーケストレーターです。
func runMain(cmd *cobra.Command, args []string) error {
	// LLM処理のコンテキストタイムアウトをフラグ値で設定
	ctx, cancel := context.WithTimeout(cmd.Context(), llmTimeout)
	defer cancel()

	// 1. URLの読み込みとバリデーション
	urls, err := generateURLs(urlFile)
	if err != nil {
		return err
	}
	log.Printf("🚀 Action Perfect Get On: %d個のURLの処理を開始します。", len(urls))

	// 2. Webコンテンツの取得とリトライ
	successfulResults, err := generateContents(ctx, urls, scraperTimeout)
	if err != nil {
		return err // 処理可能なコンテンツがゼロの場合のエラー
	}

	// 3. AIクリーンアップと出力
	if err := generateCleanedOutput(ctx, successfulResults, llmAPIKey); err != nil {
		return err
	}

	return nil
}

// ----------------------------------------------------------------
// 抽出されたステップ関数 (ジェネレーター的な役割)
// ----------------------------------------------------------------

// generateURLs はファイルからURLを読み込み、バリデーションします。
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

// generateContents はURLのリストを受け取り、並列スクレイピングとリトライを実行し、成功した結果のみを返します。
func generateContents(ctx context.Context, urls []string, timeout time.Duration) ([]types.URLResult, error) {
	log.Println("--- 1. Webコンテンツの並列抽出を開始 ---")
	initialURLCount := len(urls)

	// ParallelScraperの初期化
	s, err := scraper.NewParallelScraper(timeout)
	if err != nil {
		return nil, fmt.Errorf("スクレイパーの初期化に失敗しました: %w", err)
	}

	// 並列実行
	results := s.ScrapeInParallel(ctx, urls)

	// 無条件遅延 (2秒)
	log.Println("並列抽出が完了しました。サーバー負荷を考慮し、次の処理に進む前に2秒待機します。")
	time.Sleep(2 * time.Second)

	// 結果の分類
	successfulResults, failedURLs := classifyResults(results)
	initialSuccessfulCount := len(successfulResults)

	// 失敗URLのリトライ処理
	if len(failedURLs) > 0 {
		retriedSuccessfulResults, retryErr := processFailedURLs(ctx, failedURLs, timeout)
		if retryErr != nil {
			log.Printf("WARNING: 失敗URLのリトライ処理中にエラーが発生しました: %v", retryErr)
		}
		// リトライで成功した結果をメインのリストに追加
		successfulResults = append(successfulResults, retriedSuccessfulResults...)
	}

	// 最終成功数のチェック
	if len(successfulResults) == 0 {
		return nil, fmt.Errorf("処理可能なWebコンテンツを一件も取得できませんでした。URLを確認してください。")
	}

	// ログ出力
	log.Printf("最終成功数: %d/%d URL (初期成功: %d, リトライ成功: %d)",
		len(successfulResults), initialURLCount, initialSuccessfulCount, len(successfulResults)-initialSuccessfulCount)

	return successfulResults, nil
}

// generateCleanedOutput は取得したコンテンツを結合し、LLMでクリーンアップ・構造化して出力します。
func generateCleanedOutput(ctx context.Context, successfulResults []types.URLResult, apiKey string) error {
	// Cleanerの初期化
	// PromptBuilderのコスト削減のため、ここで一度だけ初期化し再利用します。
	c, err := cleaner.NewCleaner()
	if err != nil {
		return fmt.Errorf("Cleanerの初期化に失敗しました: %w", err)
	}

	// データ結合フェーズ
	log.Println("--- 2. 抽出結果の結合 ---")
	combinedText := cleaner.CombineContents(successfulResults)
	log.Printf("結合されたテキストの長さ: %dバイト", len(combinedText))

	// AIクリーンアップフェーズ (LLM)
	log.Println("--- 3. LLMによるテキストのクリーンアップと構造化を開始 (Go-AI-Client利用) ---")
	cleanedText, err := c.CleanAndStructureText(ctx, combinedText, apiKey)
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
// ヘルパー関数 (ロジックは元のコードからそのまま維持)
// ----------------------------------------------------------------

// readURLsFromFile は指定されたファイルからURLを読み込み、スライスとして返します。
// 空行とコメント行（#から始まる）はスキップします。
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
		// 空行とコメント行（#で始まる）をスキップ
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

// classifyResults は並列抽出の結果を成功と失敗に分類します。
func classifyResults(results []types.URLResult) (successfulResults []types.URLResult, failedURLs []string) {
	for _, res := range results {
		// エラーが発生した、またはコンテンツが空の場合は失敗と見なす
		if res.Error != nil || res.Content == "" {
			failedURLs = append(failedURLs, res.URL)
		} else {
			successfulResults = append(successfulResults, res)
		}
	}
	return successfulResults, failedURLs
}

// formatErrorLog は、冗長なHTMLボディを含むエラーメッセージを、ステータスコード情報のみに短縮します。
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

// processFailedURLs は失敗したURLに対して5秒待機後、1回だけ順次リトライを実行します。
func processFailedURLs(ctx context.Context, failedURLs []string, scraperTimeout time.Duration) ([]types.URLResult, error) {
	log.Printf("⚠️ WARNING: 抽出に失敗したURLが %d 件ありました。5秒待機後、順次リトライを開始します。", len(failedURLs))
	time.Sleep(5 * time.Second) // リトライ前の追加遅延 (ここは変更なしで5秒維持)

	// リトライ用の非並列クライアントを初期化
	retryScraperClient, err := scraper.NewClient(scraperTimeout)
	if err != nil {
		log.Printf("WARNING: リトライ用スクレイパーの初期化に失敗しました: %v。リトライ処理は実行されません。", err)
		return nil, err // 初期化エラーは呼び出し元に通知
	}

	var retriedSuccessfulResults []types.URLResult
	log.Println("--- 1b. 失敗URLの順次リトライを開始 ---")

	for _, url := range failedURLs {
		log.Printf("リトライ中: %s", url)

		// 順次再試行 (非並列)
		content, err := retryScraperClient.ExtractContent(url, ctx)

		if err != nil || content == "" {
			formattedErr := formatErrorLog(err)
			log.Printf("❌ ERROR: リトライでも %s の抽出に失敗しました: %s", url, formattedErr)
		} else {
			log.Printf("✅ SUCCESS: %s の抽出がリトライで成功しました。", url)
			retriedSuccessfulResults = append(retriedSuccessfulResults, types.URLResult{
				URL:     url,
				Content: content,
				Error:   nil,
			})
		}
	}
	return retriedSuccessfulResults, nil
}
