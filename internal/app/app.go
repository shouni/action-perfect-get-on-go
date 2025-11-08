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

	"github.com/shouni/go-http-kit/pkg/httpkit"
	"github.com/shouni/go-web-exact/v2/pkg/extract"
	extScraper "github.com/shouni/go-web-exact/v2/pkg/scraper"
	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
)

// ----------------------------------------------------------------
// 定数定義
// ----------------------------------------------------------------

const (
	phaseURLs    = "URL生成フェーズ"
	phaseContent = "コンテンツ取得フェーズ"
	phaseCleanUp = "AIクリーンアップフェーズ"
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

	// generateCleanedOutputを呼び出す
	if err := a.generateCleanedOutput(ctx, successfulResults); err != nil {
		return fmt.Errorf("%sでエラーが発生しました: %w", phaseCleanUp, err)
	}

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
	// 1. HTTP クライアント (Fetcher)
	clientOptions := []httpkit.ClientOption{
		httpkit.WithMaxRetries(2),
	}
	fetcher := httpkit.New(a.Options.ScraperTimeout, clientOptions...)
	// 2. ScraperExecutor の具体的な実装
	extractor, err := extract.NewExtractor(fetcher)
	if err != nil {
		return nil, fmt.Errorf("Extractorの初期化エラー: %w", err)
	}
	s := extScraper.NewParallelScraper(extractor, a.Options.MaxScraperParallel)
	results := s.ScrapeInParallel(ctx, urls)

	return results, nil
}

// generateCleanedOutputは、取得したコンテンツを結合し、LLMでクリーンアップ・構造化して出力します。
// LLM依存のロジックは cleaner.Cleaner に移譲します。
func (a *App) generateCleanedOutput(ctx context.Context, successfulResults []extTypes.URLResult) error {
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

	// cleaner.Cleanerのメソッドを呼び出す
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
// ヘルパー関数
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
