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
	// CustomFlagFunc: アプリ固有の永続フラグを追加する関数
	// CustomPreRunEFunc: PersistentPreRunEに追加するアプリ固有のロジック

	clibase.Execute("action-perfect-get-on-go", nil, createPreRunE(nil), runCmd)
}

// init関数でサブコマンドの定義とフラグの設定を行う
func init() {

}

// createPreRunE は、clibase共通のPersistentPreRunEロジックとアプリケーション固有のロジックを結合した関数を作成します。
// clibaseパッケージで定義された関数ですが、もしここに追加の共通ロジックが必要な場合は再定義します。
func createPreRunE(preRunE func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// clibase 共通の PersistentPreRun 処理
		if clibase.Flags.Verbose {
			// Verboseモードではファイル名と行番号を含む詳細なログを出力
			log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
			log.Println("INFO: Verbose mode enabled.")
		} else {
			// 通常モードでは日付と時刻のみを出力
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
