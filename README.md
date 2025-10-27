# 🤖 Action Perfect Get On Go

[![Language](https://img.shields.io/badge/Language-Go-blue)](https://golang.org/)
[![Go Version](https://img.shields.io/github/go-mod/go-version/shouni/prototypus-ai-doc-go)](https://golang.org/)
[![GitHub tag (latest by date)](https://img.shields.io/github/v/tag/shouni/action-perfect-get-on-go)](https://github.com/shouni/action-perfect-get-on-go/tags)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## 🌟 概要: 完璧な情報取得とAI構造化

**Action Perfect Get On Go** は、複数のウェブページから本文を**並列で高速に取得**し、その結合されたテキストを **LLM（大規模言語モデル）** の**マルチステップ処理**によって**情報欠落なく重複排除**および**論理的に構造化**する、堅牢なコマンドラインツールです。

このツールは、初期の並列処理での負荷や一時的なネットワークエラーに耐えるための**堅牢なリトライ・遅延メカニズム**を備えています。

-----

## 🛠️ 主な機能と堅牢性

1.  **堅牢なWebスクレイピングとリトライ**:
    * 複数のURLからの本文抽出をGoルーチンで同時に実行します。（並列数は**最大15件**に制限）
    * **並列処理後の固定遅延**（`initialScrapeDelay`）と、抽出失敗時の**固定遅延**（`retryScrapeDelay`）後の順次リトライ（1回）ロジックにより、サーバー負荷を軽減しつつ取得成功率を最大化します。
2.  **LLMマルチステップ処理 (MapReduce型)**:
    * 巨大な結合テキストをセグメントに**分割**。
    * 各セグメントを並列でLLM処理し、**中間要約**（Map）を作成。
    * 中間要約を統合し、**最終的な重複排除と論理構造化**（Reduce）を実行することで、大規模な情報でも情報の欠落を防ぎ、高品質な結果を保証します。
3.  **AI駆動のデータクリーンアップと構造化**: 結合されたテキストから重複コンテンツやノイズ（フッター、ナビゲーションなど）を排除し、情報構造を再構築します。処理指示は**日本語**で行われます。（内部で `go-ai-client` を利用）
4.  **柔軟な設定**: 各フェーズでタイムアウトを設定可能にし、LLM APIキーを環境変数またはCLIオプションで柔軟に設定できます。
5.  **内部設計の最適化**:
    * プロンプト定義を外部ファイル（`.md`）に分離し、Goの`embed`パッケージでバイナリに組み込むことで、デプロイの堅牢性を確保。
    * LLMプロンプトの生成に**ビルダーパターン**を採用し、テンプレートのパースコストを削減するため**再利用可能なインスタンス**として管理しています。
    * **CLIオプションを構造体に集約**することで、グローバル変数への依存を減らし、テスト容易性と長期的な保守性を向上させています。

-----

## ✨ 技術スタック

| 要素 | 技術 / ライブラリ | 役割 |
| :--- | :--- | :--- |
| **言語** | **Go (Golang)** | ツールの開発言語。並列処理と堅牢な実行環境を提供します。 |
| **CLI** | **Cobra** | コマンドライン引数とオプションの解析に使用します。 |
| **Web抽出** | **[`github.com/shouni/go-web-exact`](https://github.com/shouni/go-web-exact)** | 任意のウェブページからメインの本文コンテンツを正確に抽出します。 |
| **AI通信** | **[`github.com/shouni/go-ai-client`](https://github.com/shouni/go-ai-client)** | LLM（Gemini）への通信を管理し、自動リトライ機能を提供します。 |
| **プロンプト** | **`text/template`, `embed`** | プロンプトを外部ファイル化し、**テンプレートパースのコストを抑えた**効率的なプロンプト生成ロジックを実現します。 |
| **並列処理** | **`sync.WaitGroup` / Goルーチン** | 複数のURLへのアクセスを同時に高速で実行します。 |
| **設計パターン** | **DI準備（構造体集約）** | グローバル変数への依存を減らし、CLIオプションを構造体に集約することで、テスト容易性を向上させます。 |

-----

## 🛠️ 事前準備と設定

### 1\. ビルド

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

本ツールは、処理対象のURLを記載した**ファイル**を読み込む形式のみをサポートします。

### 実行コマンド形式とオプション

| オプション | フラグ | 説明 | デフォルト値 |
| :--- | :--- | :--- | :--- |
| `--api-key` | `-k` | **Gemini APIキー**を直接指定します（推奨）。 | なし |
| `--url-file` | `-f` | **処理対象のURLリストを記載したファイルパス**を指定します。ファイル内ではURLを改行区切りで記述します。 **(必須)** | なし |
| `--llm-timeout` | `-t` | LLM処理全体のタイムアウト時間。 | 5m0s (5分) |
| `--scraper-timeout` | `-s` | Webスクレイピング（HTTPアクセス）のタイムアウト時間。 | 15s (15秒) |

### 1\. URLファイル (`urls.txt` の例) の作成

ファイル内に、1行に1つずつ処理したいURLを記述します。

```text
# ファイル名: urls.txt
https://example.com/page-a
https://example.com/page-b
https://example.com/page-c/specification
```

### 2\. 実行コマンド形式

処理を実行するには、**必ず** `--url-file` または `-f` フラグを使用して有効なファイルパスを指定する必要があります。URLを直接引数として渡す形式はサポートされていません。

```bash
# 最小実行形式 (環境変数にAPIキーが設定されている場合)
./bin/llm_cleaner -f ./urls.txt

# 推奨実行形式 (APIキーとカスタムタイムアウトを指定)
./bin/llm_cleaner -k "YOUR_API_KEY" -f ./urls.txt -s 30s -t 3m
```

### 🗃️ 処理の流れ

1.  **並列抽出**: コマンドライン引数で渡された複数のURLへのアクセスが**同時に**開始されます。（最大15並列）
2.  **遅延**: 並列抽出完了後、サーバー負荷軽減のため**固定時間**（`initialScrapeDelay`）待機します。
3.  **リトライ**: 抽出に失敗したURLがある場合、**固定時間**（`retryScrapeDelay`）待機した後、順次**1回だけリトライ**されます。
4.  **結合と分割**: 最終的に成功したすべての本文が結合され、LLMのトークン制限に応じて複数のチャンクに分割されます。
5.  **Mapフェーズ**: 各チャンクが並列でLLMに送られ、中間要約が作成されます。
6.  **Reduceフェーズ**: すべての中間要約が統合され、最終的な重複排除と構造化が実行されます。
7.  **出力**: LLMが構造化した最終的なテキスト（Markdown形式）が標準出力に出力されます。

-----

## 📜 ライセンス (License)

このプロジェクトは [MIT License](https://opensource.org/licenses/MIT) の下で公開されています。
