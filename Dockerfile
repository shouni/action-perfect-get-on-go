# ----------------------------------------------------------------------
# STEP 1: ビルドステージ (Goバイナリのコンパイル)
# ----------------------------------------------------------------------
FROM golang:1.25 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 実行ファイルが ./cmd ディレクトリにあるため、ビルドパスを指定
RUN CGO_ENABLED=0 go build -o bin/llm_cleaner

# ----------------------------------------------------------------------
# STEP 2: 実行ステージ (実行専用の超軽量・セキュアなイメージ)
# ----------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12
WORKDIR /bin
COPY --from=builder /bin/llm_cleaner /bin/llm_cleaner
ENTRYPOINT ["/bin/llm_cleaner"]