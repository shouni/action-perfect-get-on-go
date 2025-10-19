package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"action-perfect-get-on-go/pkg/cleaner"
	"action-perfect-get-on-go/pkg/scraper"
	"action-perfect-get-on-go/pkg/types"

	"github.com/spf13/cobra"
)

// コマンドラインオプションのグローバル変数
var llmAPIKey string
var llmTimeout time.Duration
var scraperTimeout time.Duration

func init() {
	rootCmd.PersistentFlags().DurationVarP(&llmTimeout, "llm-timeout", "t", 5*time.Minute, "LLM処理のタイムアウト時間")
	rootCmd.PersistentFlags().DurationVarP(&scraperTimeout, "scraper-timeout", "s", 15*time.Second, "WebスクレイピングのHTTPタイムアウト時間")
	rootCmd.PersistentFlags().StringVarP(&llmAPIKey, "api-key", "k", "", "Gemini APIキー (環境変数 GEMINI_API_KEY が優先)")
}

// プログラムのエントリーポイント
func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "action-perfect-get-on-go [URL...]",
	Short: "複数のURLを並列で取得し、LLMでクリーンアップします。",
	Long: `
Action Perfect Get On Ready to Go
銀河の果てまで 追いかけてゆく 魂の血潮で アクセル踏み込み

複数のURLを並列でスクレイピングし、取得した本文をLLMで重複排除・構造化するツールです。
[URL...]としてスペース区切りで複数のURLを引数に指定してください。
`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMain,
}

// runMain は CLIのメインロジックを実行します。
func runMain(cmd *cobra.Command, args []string) error {
	urls := args

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
	// 1秒無条件遅延と結果の分類
	// -----------------------------------------------------------

	// 1. 無条件遅延 (1秒)
	log.Println("並列抽出が完了しました。サーバー負荷を考慮し、次の処理に進む前に1秒待機します。")
	time.Sleep(1 * time.Second)

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

	cleanedText, err := cleaner.CleanAndStructureText(ctx, combinedText, llmAPIKey)
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

// processFailedURLs は失敗したURLに対して5秒待機後、1回だけ順次リトライを実行します。
func processFailedURLs(ctx context.Context, failedURLs []string, scraperTimeout time.Duration) ([]types.URLResult, error) {
	log.Printf("⚠️ WARNING: 抽出に失敗したURLが %d 件ありました。5秒待機後、順次リトライを開始します。", len(failedURLs))
	time.Sleep(5 * time.Second) // リトライ前の追加遅延

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
			log.Printf("❌ ERROR: リトライでも %s の抽出に失敗しました: %v", url, err)
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
