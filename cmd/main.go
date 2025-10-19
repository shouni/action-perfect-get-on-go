package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"action-perfect-get-on-go/pkg/cleaner"
	"action-perfect-get-on-go/pkg/scraper"

	"github.com/spf13/cobra"
)

// URLResult は個々のURLの抽出結果を格納する構造体
type URLResult struct {
	URL     string
	Content string // 抽出された本文
	Error   error
}

// プログラムのエントリーポイント
func main() {
	if err := rootCmd.Execute(); err != nil {
		// Cobraのエラーは既に表示されていることが多いが、確実に終了コードを返す
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
	// 少なくとも2つ以上のURLを必須とする
	Args: cobra.MinimumNArgs(2),
	RunE: runMain,
}

// runMain は CLIのメインロジックを実行します。
func runMain(cmd *cobra.Command, args []string) error {
	urls := args

	// LLM処理は時間がかかる可能性があるため、長めのコンテキストを設定
	ctx, cancel := context.WithTimeout(cmd.Context(), time.Minute*5)
	defer cancel()

	log.Printf("🚀 Action Perfect Get On: %d個のURLの処理を開始します。", len(urls))

	// --- 1. 並列抽出フェーズ (Scraping) ---
	log.Println("--- 1. Webコンテンツの並列抽出を開始 ---")

	s := scraper.NewParallelScraper()
	results := s.ScrapeInParallel(ctx, urls)

	// 処理結果の確認
	var successCount int
	for _, res := range results {
		if res.Error != nil {
			log.Printf("❌ ERROR: %s の抽出に失敗しました: %v", res.URL, res.Error)
		} else {
			successCount++
		}
	}

	if successCount == 0 {
		return fmt.Errorf("致命的エラー: すべてのURLの抽出に失敗しました。処理を中断します")
	}

	// --- 2. データ結合フェーズ ---
	log.Println("--- 2. 抽出結果の結合 ---")

	// cleanerパッケージの関数を呼び出す
	combinedText := cleaner.CombineContents(results)

	log.Printf("結合されたテキストの長さ: %dバイト (成功: %d/%d URL)",
		len(combinedText), successCount, len(urls))

	// --- 3. AIクリーンアップフェーズ (LLM) ---
	log.Println("--- 3. LLMによるテキストのクリーンアップと構造化を開始 (Go-AI-Client利用) ---")

	// cleanerパッケージの関数を呼び出す
	cleanedText, err := cleaner.CleanAndStructureText(ctx, combinedText)
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
