# ----------------------------------------------------------------------
# STEP 1: ビルドステージ (Goバイナリのコンパイル)
# ----------------------------------------------------------------------
FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
# アプリケーションのソースコード全体をコピー (main.go を含む)
COPY . .
# 実行ファイルが ルート直下の main.go を起点としているため、ビルド対象をルート (.) に指定
# 実行ファイルは ./app/bin/llm_cleaner に出力されます
RUN CGO_ENABLED=0 go build -o bin/llm_cleaner .

# ----------------------------------------------------------------------
# STEP 2: 実行ステージ (実行専用の超軽量・セキュアなイメージ)
# ----------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12
LABEL org.opencontainers.image.source=https://github.com/shouni/action-perfect-get-on-go

# 実行可能なバイナリの配置場所を /usr/local/bin に設定
WORKDIR /usr/local/bin

# 修正: ビルドステージの相対パス (/app/bin/llm_cleaner) からコピー
COPY --from=builder /app/bin/llm_cleaner /usr/local/bin/llm_cleaner

# エントリポイントを定義
ENTRYPOINT ["/usr/local/bin/llm_cleaner"]
