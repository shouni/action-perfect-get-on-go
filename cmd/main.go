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

// コマンドラインオプションのグローバル変数
var llmAPIKey string
var llmTimeout time.Duration
var scraperTimeout time.Duration
var urlFile string

func init() {
	rootCmd.PersistentFlags().DurationVarP(&llmTimeout, "llm-timeout", "t", 5*time.Minute, "LLM処理のタイムアウト時間")
	rootCmd.PersistentFlags().DurationVarP(&scraperTimeout, "scraper-timeout", "s", 15*time.Second, "WebスクレイピングのHTTPタイムアウト時間")
	rootCmd.PersistentFlags().StringVarP(&llmAPIKey, "api-key", "k", "", "Gemini APIキー (環境変数 GEMINI_API_KEY が優先)")
	// ⭐ 修正点: urlFile フラグの登録はこれでOK
	rootCmd.PersistentFlags().StringVarP(&urlFile, "url-file", "f", "", "処理対象のURLリストを記載したファイルパス")
}

// プログラムのエントリーポイント
func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	// ⭐ 修正点: Useの記述から [URL...] を削除。Argsのチェックを削除するため。
	Use:   "action-perfect-get-on-go",
	Short: "複数のURLを並列で取得し、LLMでクリーンアップします。",
	Long: `
Action Perfect Get On Ready to Go
銀河の果てまで 追いかけてゆく 魂の血潮で アクセル踏み込み

複数のURLを並列でスクレイピングし、取得した本文をLLMで重複排除・構造化するツールです。
実行には、-fまたは--url-fileオプションでURLリストファイルを指定してください。
`,
	// ⭐ 修正点: ファイル入力に切り替えるため、引数の最小個数チェックを削除
	// Args: cobra.MinimumNArgs(1),
	RunE: runMain,
}

// runMain は CLIのメインロジックを実行します。
func runMain(cmd *cobra.Command, args []string) error {
	var urls []string
	var err error

	// ⭐ 修正点: Cleanerの初期化をここ（runMainの冒頭）に移動し、cを定義する
	// PromptBuilderのコスト削減のため、ここで一度だけ初期化し再利用します。
	c, err := cleaner.NewCleaner()
	if err != nil {
		// NewCleanerが失敗した場合（主にPrompt Builderのテンプレートパースエラー）、ここで終了
		return fmt.Errorf("Cleanerの初期化に失敗しました: %w", err)
	}

	// URL入力ロジック
	if urlFile != "" {
		urls, err = readURLsFromFile(urlFile)
		if err != nil {
			return fmt.Errorf("URLファイルの読み込みに失敗しました: %w", err)
		}
	} else if len(args) > 0 {
		// 互換性のために、ファイルフラグがない場合はコマンド引数もチェックする（推奨はしないが、一時的な対応として残す）
		urls = args
		log.Println("⚠️ WARNING: URLが引数として渡されました。将来的に -f/--url-file フラグの使用が必須になります。")
	} else {
		// ファイルも引数も提供されていない場合はエラー
		return fmt.Errorf("処理対象のURLを指定してください。-f/--url-file オプションでURLリストファイルを指定するか、URLを引数に渡してください。")
	}

	if len(urls) == 0 {
		return fmt.Errorf("URLリストファイルに有効なURLが一件も含まれていませんでした。")
	}

	// LLM処理のコンテキストタイムアウトをフラグ値で設定
	ctx, cancel := context.WithTimeout(cmd.Context(), llmTimeout)
	defer cancel()

	log.Printf("🚀 Action Perfect Get On: %d個のURLの処理を開始します。", len(urls))

	// --- 1. 並列抽出フェーズ (Scraping) ---
	log.Println("--- 1. Webコンテンツの並列抽出を開始 ---")

	// ParallelScraperの初期化 (エラーをチェック)
	s, err := scraper.NewParallelScraper(scraperTimeout)
	if err != nil {
		// 初期化失敗時にログに出力
		log.Printf("ERROR: スクライパーの初期化に失敗しました: %v", err)
		return fmt.Errorf("スクレイパーの初期化に失敗しました: %w", err)
	}

	// 並列実行
	results := s.ScrapeInParallel(ctx, urls)

	// -----------------------------------------------------------
	// 2秒無条件遅延と結果の分類
	// -----------------------------------------------------------

	// 1. 無条件遅延 (2秒)
	log.Println("並列抽出が完了しました。サーバー負荷を考慮し、次の処理に進む前に2秒待機します。")
	time.Sleep(2 * time.Second)

	// 2. 結果の分類
	successfulResults, failedURLs := classifyResults(results)

	// 初期成功数を保持
	initialSuccessfulCount := len(successfulResults)

	// 3. 失敗URLのリトライ処理
	if len(failedURLs) > 0 {
		retriedSuccessfulResults, retryErr := processFailedURLs(ctx, failedURLs, scraperTimeout)
		if retryErr != nil {
			// 初期化エラーが発生した場合の警告 (処理は続行)
			log.Printf("WARNING: 失敗URLのリトライ処理中にエラーが発生しました: %v", retryErr)
		}
		// リトライで成功した結果をメインのリストに追加
		successfulResults = append(successfulResults, retriedSuccessfulResults...)
	}

	// 成功URLがゼロの場合は終了
	if len(successfulResults) == 0 {
		return fmt.Errorf("処理可能なWebコンテンツを一件も取得できませんでした。URLを確認してください。")
	}

	// --- 2. データ結合フェーズ (リトライ成功結果も含む) ---
	log.Println("--- 2. 抽出結果の結合 ---")

	combinedText := cleaner.CombineContents(successfulResults)

	// ログ出力に初期成功数と最終成功数を明記
	log.Printf("結合されたテキストの長さ: %dバイト (初期成功: %d/%d URL, 最終成功: %d/%d URL)",
		len(combinedText), initialSuccessfulCount, len(urls), len(successfulResults), len(urls))

	// --- 3. AIクリーンアップフェーズ (LLM) ---
	log.Println("--- 3. LLMによるテキストのクリーンアップと構造化を開始 (Go-AI-Client利用) ---")

	// ⭐ 修正済み: c.CleanAndStructureText(ctx, combinedText, llmAPIKey) が c のスコープ内で実行される
	cleanedText, err := c.CleanAndStructureText(ctx, combinedText, llmAPIKey)
	if err != nil {
		return fmt.Errorf("LLMクリーンアップ処理に失敗しました: %w", err)
	}

	// --- 4. 最終結果の出力 ---
	fmt.Println("\n===============================================")
	fmt.Println("✅ PERFECT GET ON: LLMクリーンアップ後の最終出力データ:")
	fmt.Println("===============================================")
	fmt.Println(cleanedText)
	fmt.Println("===============================================")

	return nil
}

// ----------------------------------------------------------------
// ヘルパー関数
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
