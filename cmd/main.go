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
		return fmt.Errorf("スクレイパーの初期化に失敗しました: %w", err)
	}

	// 並列実行
	results := s.ScrapeInParallel(ctx, urls)

	// 1. 無条件遅延
	log.Println("並列抽出が完了しました。サーバー負荷を考慮し、次の処理に進む前に1秒待機します。")
	time.Sleep(1 * time.Second)

	// 2. 結果の分類
	var successfulResults []types.URLResult
	var failedURLs []string

	for _, res := range results {
		// エラーが発生した、またはコンテンツが空の場合は失敗と見なす
		if res.Error != nil || res.Content == "" {
			log.Printf("❌ ERROR: %s の抽出に失敗しました: %v", res.URL, res.Error)
			failedURLs = append(failedURLs, res.URL)
		} else {
			successfulResults = append(successfulResults, res)
		}
	}

	// 3. 失敗URLがある場合、追加で5秒待機後に順次リトライ
	if len(failedURLs) > 0 {
		log.Printf("⚠️ WARNING: 抽出に失敗したURLが %d 件あります。5秒待機後、順次リトライを開始します。", len(failedURLs))
		time.Sleep(5 * time.Second) // リトライ前の追加遅延

		// リトライ用の非並列クライアントを初期化
		retryScraperClient, err := scraper.NewClient(scraperTimeout)
		if err != nil {
			log.Printf("ERROR: リトライ用スクレイパーの初期化に失敗しました: %v", err)
		} else {
			log.Println("--- 1b. 失敗URLの順次リトライを開始 ---")

			for _, url := range failedURLs {
				log.Printf("リトライ中: %s", url)

				// 順次再試行 (非並列)
				content, err := retryScraperClient.ExtractContent(url, ctx)

				if err != nil || content == "" {
					log.Printf("❌ ERROR: リトライでも %s の抽出に失敗しました: %v", url, err)
				} else {
					log.Printf("✅ SUCCESS: %s の抽出がリトライで成功しました。", url)
					// リトライで成功したものを成功リストに追加
					successfulResults = append(successfulResults, types.URLResult{
						URL:     url,
						Content: content,
						Error:   nil,
					})
				}
			}
		}
	}

	// 成功URLがゼロの場合は終了
	if len(successfulResults) == 0 {
		return fmt.Errorf("致命的エラー: 最終的に処理可能なWebコンテンツがありませんでした。")
	}

	// --- 2. データ結合フェーズ (リトライ成功結果も含む) ---
	log.Println("--- 2. 抽出結果の結合 ---")

	combinedText := cleaner.CombineContents(successfulResults)

	log.Printf("結合されたテキストの長さ: %dバイト (成功: %d/%d URL)",
		len(combinedText), len(successfulResults), len(urls))

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
