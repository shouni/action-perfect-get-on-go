package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"action-perfect-get-on-go/pkg/cleaner"
	"action-perfect-get-on-go/pkg/scraper"
	"action-perfect-get-on-go/pkg/types" // ⭐ 修正点: 共有型をインポート

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
	// ⭐ 修正点: 柔軟性を向上させるため、少なくとも1つ以上のURLを必須とする
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

	// ⭐ 修正点: Scraperの初期化時に、フラグ値のタイムアウトを渡す
	s := scraper.NewParallelScraper(scraperTimeout)

	results := s.ScrapeInParallel(ctx, urls)

	// 処理結果の確認と成功結果のフィルタリング
	var successResults []types.URLResult // ⭐ 修正点: 成功した結果のみを格納
	var successCount int

	for _, res := range results {
		if res.Error != nil {
			log.Printf("❌ ERROR: %s の抽出に失敗しました: %v", res.URL, res.Error)
		} else {
			successResults = append(successResults, res)
			successCount++
		}
	}

	if successCount == 0 {
		return fmt.Errorf("致命的エラー: すべてのURLの抽出に失敗しました。処理を中断します")
	}

	// --- 2. データ結合フェーズ ---
	log.Println("--- 2. 抽出結果の結合 ---")

	combinedText := cleaner.CombineContents(successResults)

	log.Printf("結合されたテキストの長さ: %dバイト (成功: %d/%d URL)",
		len(combinedText), successCount, len(urls))

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
