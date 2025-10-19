# 🤖 Action Perfect Get On Go

## 🌟 概要: 完璧な情報取得とAI構造化

**Action Perfect Get On Go** は、複数のウェブページから本文を**並列で高速に取得**し、その結合されたテキストを **LLM（大規模言語モデル）** の**マルチステップ処理**によって**情報欠落なく重複排除**および**論理的に構造化**する、堅牢なコマンドラインツールです。

### サブ概要：Action Perfect Get On Ready to Go
**銀河の果てまで 追いかけてゆく 魂の血潮で アクセル踏み込み**

-----

### 🛠️ 主な機能

1.  **並列Webスクレイピング**: 複数のURLからの本文抽出をGoルーチンで同時に実行し、時間を大幅に短縮します。（内部で `go-web-exact` を利用）
2.  **LLMマルチステップ処理 (MapReduce型)**:
    * 巨大な結合テキストをセグメントに**分割**。
    * 各セグメントを並列でLLM処理し、**中間要約**を作成。
    * 中間要約を統合し、**最終的な重複排除と論理構造化**を実行することで、大規模な情報でも情報の欠落を防ぎ、高品質な結果を保証します。
3.  **AI駆動のデータクリーンアップと構造化**: 結合されたテキストから重複コンテンツやノイズ（フッター、ナビゲーションなど）を排除し、情報構造を再構築します。（内部で `go-ai-client` を利用）
4.  **柔軟な設定と堅牢なエラーハンドリング**: LLM APIキーを**CLIオプション**または環境変数で設定可能にし、各フェーズでタイムアウトとリトライ機構を備えています。

-----

## ✨ 技術スタック

| 要素 | 技術 / ライブラリ | 役割 |
| :--- | :--- | :--- |
| **言語** | **Go (Golang)** | ツールの開発言語。並列処理と堅牢な実行環境を提供します。 |
| **CLI** | **Cobra** | コマンドライン引数とオプションの解析に使用します。 |
| **Web抽出** | **`github.com/shouni/go-web-exact`** | 任意のウェブページからメインの本文コンテンツを正確に抽出します。 |
| **AI通信** | **`github.com/shouni/go-ai-client`** | LLM（Gemini）への通信を管理し、自動リトライ機能を提供します。 |
| **並列処理** | **`sync.WaitGroup` / Goルーチン** | 複数のURLへのアクセスを同時に高速で実行します。 |

-----

## 🛠️ 事前準備と設定

### 1. ビルド

```bash
# リポジトリをクローン
git clone git@github.com:your-repo-path/action-perfect-get-on-go.git
cd action-perfect-get-on-go

# 依存関係をダウンロード
go mod tidy

# 実行ファイルを bin/ ディレクトリに生成
go build -o bin/llm_cleaner ./cmd
```

実行ファイルは `./bin/llm_cleaner` に生成されます。

### 2\. LLM API キーの設定 (必須)

LLM（Gemini）を利用するためには、APIキーが必要です。設定は以下の**どちらか**の方法で行います。

* **推奨**: コマンド実行時に `-k` または `--api-key` フラグで直接指定する。
* **代替**: 環境変数 `GEMINI_API_KEY` を設定する。

**注意**: コマンドラインフラグでキーを指定した場合、環境変数の設定よりも**常に優先されます**。

```bash
# 例: 環境変数に設定する場合
export GEMINI_API_KEY="YOUR_GEMINI_API_KEY" 
```

-----

## 🚀 使い方 (Usage)

本ツールは、処理対象のURLを**コマンドライン引数**として受け取ります。

### 実行コマンド形式とオプション

| オプション | フラグ | 説明 | デフォルト値 |
| :--- | :--- | :--- | :--- |
| `--api-key` | `-k` | **Gemini APIキー**を直接指定します（推奨）。 | なし |
| `--llm-timeout` | `-t` | LLM処理全体のタイムアウト時間。 | 5m0s (5分) |
| `--scraper-timeout` | `-s` | Webスクレイピング（HTTPアクセス）のタイムアウト時間。 | 15s (15秒) |

```bash
# 最小実行形式 (環境変数にAPIキーが設定されている場合)
./bin/llm_cleaner [https://www.youtube.com/watch?v=KsZ6tROaVOQ](https://www.youtube.com/watch?v=KsZ6tROaVOQ) [https://www.youtube.com/watch?v=-s7TCuCpB5c](https://www.youtube.com/watch?v=-s7TCuCpB5c) ...

# 推奨実行形式 (APIキーとカスタムタイムアウトを指定)
./bin/llm_cleaner -k "YOUR_API_KEY" -s 30s -t 3m \
    [https://example.com/page-a](https://example.com/page-a) \
    [https://example.com/page-b](https://example.com/page-b) \
    [https://example.com/page-c](https://example.com/page-c)
```

**注意**: 処理を実行するには、**少なくとも1つ以上のURL**を指定する必要があります。

### 🗃️ 処理の流れ

1.  コマンドライン引数で渡された複数のURLへのアクセスが**同時に**開始されます。
2.  `go-web-exact`により各ページの本文が抽出されます。
3.  抽出された本文が一つに結合され、**LLMのセグメントサイズに応じて複数のチャンクに分割**されます。
4.  **Mapフェーズ**: 各チャンクが並列でLLMに送られ、中間要約が作成されます。
5.  **Reduceフェーズ**: すべての中間要約が統合され、最終的な重複排除と構造化が実行されます。
6.  LLMが構造化した最終的なテキスト（Markdown形式）が標準出力に出力されます。

-----

## 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。

