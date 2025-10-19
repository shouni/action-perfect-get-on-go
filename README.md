# 🤖 Action Perfect Get On Go

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## 🌟 概要: 完璧な情報取得とAI構造化

**action-perfect-get-on-go** は、複数のウェブページから本文を**並列で高速に取得**し、その結合されたテキストを **LLM（大規模言語モデル）** によって**重複排除**および**論理的に構造化**する、堅牢なコマンドラインツールです。

### 🚨 サブ概要：Action Perfect Get On Ready to Go
> **銀河の果てまで 追いかけてゆく 魂の血潮で アクセル踏み込み**

### 🛠️ 主な機能

1.  **並列Webスクレイピング**: 複数のURLからの本文抽出をGoルーチンで同時に実行し、時間を大幅に短縮します。（内部で `go-web-exact` を利用）
2.  **AI駆動のデータクリーンアップ**: 結合されたテキストから重複コンテンツやノイズ（フッター、ナビゲーションなど）を排除し、情報構造を再構築します。（内部で `go-ai-client` を利用）
3.  **堅牢なエラーハンドリング**: ネットワークエラーやAPI制限に対応するため、各フェーズでタイムアウトとリトライ機構を備えています。

## ✨ 技術スタック

| 要素 | 技術 / ライブラリ | 役割 |
| :--- | :--- | :--- |
| **言語** | **Go (Golang)** | ツールの開発言語。並列処理と堅牢な実行環境を提供します。 |
| **CLI** | **Cobra** | コマンドライン引数（URLリスト）の解析に使用します。 |
| **Web抽出** | **`github.com/shouni/go-web-exact`** | 任意のウェブページからメインの本文コンテンツを正確に抽出します。 |
| **AI通信** | **`github.com/shouni/go-ai-client`** | LLM（Geminiなど）への通信を管理し、自動リトライ機能を提供します。 |
| **並列処理** | **`sync.WaitGroup` / Goルーチン** | 複数のURLへのアクセスを同時に高速で実行します。 |

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
````

実行ファイルは `./bin/llm_cleaner` に生成されます。

### 2\. 環境変数の設定 (必須)

LLMを利用するために、APIキーを環境変数に設定する必要があります。

```bash
# LLM API キー (go-ai-client が参照します)
export GEMINI_API_KEY="YOUR_GEMINI_API_KEY" 
# または、go-ai-client が対応するその他のモデルのキー
```

## 🚀 使い方 (Usage)

本ツールは、処理対象のURLを\*\*コマンドライン引数（可変長）\*\*として受け取ります。これにより、柔軟に複数のURLを指定できます。

### 実行コマンド形式

```bash
./bin/llm_cleaner [https://www.youtube.com/watch?v=KsZ6tROaVOQ](https://www.youtube.com/watch?v=KsZ6tROaVOQ) [https://www.youtube.com/watch?v=-s7TCuCpB5c](https://www.youtube.com/watch?v=-s7TCuCpB5c) [https://www.youtube.com/watch?v=ep9zgmN9BNA](https://www.youtube.com/watch?v=ep9zgmN9BNA) [https://en.wikipedia.org/wiki/4](https://en.wikipedia.org/wiki/4) ... [https://en.wikipedia.org/wiki/N](https://en.wikipedia.org/wiki/N)
```

**注意:** 処理を実行するには、少なくとも2つ以上のURLを指定する必要があります。

### 実行例

```bash
./bin/llm_cleaner \
    [https://blog.go-lang.org/intro-to-go](https://blog.go-lang.org/intro-to-go) \
    [https://go.dev/doc/effective_go](https://go.dev/doc/effective_go) \
    [https://go.dev/doc/code](https://go.dev/doc/code) \
    [https://blog.go-lang.org/concurrency-is-not-parallelism](https://blog.go-lang.org/concurrency-is-not-parallelism)
```

### 🗃️ 処理の流れ

1.  コマンドライン引数で渡された複数のURLへのアクセスが**同時に**開始されます。
2.  `go-web-exact`により各ページの本文が抽出されます。
3.  抽出された本文が一つに結合されます。
4.  結合テキストが専用のプロンプトと共にLLMに送信されます。
5.  LLMが重複を排除し、構造化した最終的なテキストが標準出力に出力されます。

## 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。

