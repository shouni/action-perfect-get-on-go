package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/shouni/go-cli-base"
)

// Execute は、CLIアプリケーションのルートエントリポイントです。
// 全てのサブコマンドをルートコマンドにアタッチし、実行を開始します。
func Execute() {
	// CustomFlagFunc: アプリ固有の永続フラグを追加する関数 (今回はなし)
	// CustomPreRunEFunc: PersistentPreRunEに追加するアプリ固有のロジック (今回はなし)
	// cmds: ルートに追加するサブコマンド (runCmd)

	// clibase.Execute を使用して、アプリケーション名、カスタマイズ関数、サブコマンドを渡して実行します。
	// Executeは内部でNewRootCmdを呼び出し、PersistentPreRunEなどを設定します。
	clibase.Execute("action-perfect-get-on-go", nil, nil, runCmd)
}

// init関数でサブコマンドの定義とフラグの設定を行う
func init() {
	// runCmd は cmd/run.go で定義されています
	// フラグ定義は cmd/run.go の init() に移動しました。
}

// createPreRunE は、clibase共通のPersistentPreRunEロジックとアプリケーション固有のロジックを結合した関数を作成します。
// clibaseパッケージで定義された関数ですが、もしここに追加の共通ロジックが必要な場合は再定義します。
func createPreRunE(preRunE func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// clibase 共通の PersistentPreRun 処理
		if clibase.Flags.Verbose {
			log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
			log.Println("INFO: Verbose mode enabled.")
		} else {
			// Verboseでない場合は、標準のログフラグを設定
			log.SetFlags(log.Ldate | log.Ltime)
		}

		// アプリケーション固有の PersistentPreRunE 処理を実行
		if preRunE != nil {
			return preRunE(cmd, args)
		}
		return nil
	}
}

// fatalIfError はエラーが発生した場合にログを出力して終了します。
func fatalIfError(err error) {
	if err != nil {
		log.Printf("FATAL: %v", err)
		os.Exit(1)
	}
}
