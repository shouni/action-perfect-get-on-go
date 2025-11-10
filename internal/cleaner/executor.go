package cleaner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shouni/action-perfect-get-on-go/prompts"
	"github.com/shouni/go-ai-client/v2/pkg/ai/gemini"
)

// LLMExecutor は、LLMの実行能力を抽象化するインターフェースです。
// これにより、Cleanerのコアロジックから API通信と並列実行の詳細を分離します。
type LLMExecutor interface {
	ExecuteMap(ctx context.Context, segments []Segment, builder *prompts.PromptBuilder) ([]string, error)
	ExecuteReduce(ctx context.Context, combinedText string, builder *prompts.PromptBuilder) (string, error)
}

// LLMConcurrentExecutor は LLMExecutor の具体的な実装で、
// Goroutine、セマフォ、レートリミッターを使用して並列実行を行います。
type LLMConcurrentExecutor struct {
	client      *gemini.Client
	concurrency int
}

// NewLLMConcurrentExecutor は新しい LLMConcurrentExecutor インスタンスを作成します。
func NewLLMConcurrentExecutor(ctx context.Context, apiKeyOverride string, concurrency int) (*LLMConcurrentExecutor, error) {
	var client *gemini.Client
	var err error

	if apiKeyOverride != "" {
		client, err = gemini.NewClient(ctx, gemini.Config{APIKey: apiKeyOverride})
	} else {
		client, err = gemini.NewClientFromEnv(ctx)
	}

	if err != nil {
		return nil, fmt.Errorf("LLMクライアントの初期化に失敗しました。APIキーを確認してください: %w", err)
	}

	if concurrency < 1 {
		concurrency = 1
	}

	return &LLMConcurrentExecutor{
		client:      client,
		concurrency: concurrency,
	}, nil
}

// MapResult はセグメント処理の結果を保持します。
type MapResult struct {
	Summary string
	Err     error
}

// ExecuteMap は Mapフェーズの並列処理を実行します。
func (e *LLMConcurrentExecutor) ExecuteMap(ctx context.Context, allSegments []Segment, mapBuilder *prompts.PromptBuilder) ([]string, error) {
	var wg sync.WaitGroup
	resultsChan := make(chan MapResult, len(allSegments))

	// 並列処理セマフォ
	sem := make(chan struct{}, e.concurrency)

	// LLM APIのコール間隔を制御するレートリミッター
	ticker := time.NewTicker(DefaultLLMRateLimit)
	defer ticker.Stop()
	rateLimiter := ticker.C

	// 修正: log.Printf -> slog.Info (構造化)
	slog.Info("セグメントの並列処理を開始します",
		slog.Int("total_segments", len(allSegments)),
		slog.Int("max_parallel", e.concurrency),
		slog.Duration("rate_limit", DefaultLLMRateLimit))

	for i, seg := range allSegments {
		sem <- struct{}{} // セマフォ取得
		wg.Add(1)

		go func(index int, s Segment) {
			defer func() { <-sem }() // セマフォ解放
			defer wg.Done()

			// レートリミットとコンテキストキャンセルを select で同時に監視
			select {
			case <-rateLimiter:
				// 続行
			case <-ctx.Done():
				// エラーログを出す必要がないため、エラーメッセージは簡略化
				resultsChan <- MapResult{Err: fmt.Errorf("セグメント %d 処理がコンテキストキャンセルにより中断されました: %w", index+1, ctx.Err())}
				return
			}

			mapData := prompts.MapTemplateData{
				SegmentText: s.Text,
				SourceURL:   s.URL,
			}
			prompt, err := mapBuilder.BuildMap(mapData)
			if err != nil {
				// エラー処理は resultsChan に集約
				resultsChan <- MapResult{Err: fmt.Errorf("セグメント %d プロンプト生成失敗 (URL: %s): %w", index+1, s.URL, err)}
				return
			}

			response, err := e.client.GenerateContent(ctx, prompt, "gemini-2.5-flash")
			if err != nil {
				// エラー処理は resultsChan に集約
				resultsChan <- MapResult{Err: fmt.Errorf("セグメント %d 処理失敗 (URL: %s): %w", index+1, s.URL, err)}
				return
			}

			resultsChan <- MapResult{Summary: response.Text, Err: nil}
		}(i, seg)
	}

	wg.Wait()
	close(resultsChan)

	var summaries []string
	for res := range resultsChan {
		if res.Err != nil {
			return nil, res.Err
		}
		summaries = append(summaries, res.Summary)
	}

	return summaries, nil
}

// ExecuteReduce は ReduceフェーズのAPI呼び出しを実行します。
func (e *LLMConcurrentExecutor) ExecuteReduce(ctx context.Context, combinedText string, reduceBuilder *prompts.PromptBuilder) (string, error) {
	// 修正: log.Println -> slog.Info
	slog.Info("最終的な構造化（Reduceフェーズ）を開始します。")

	reduceData := prompts.ReduceTemplateData{
		CombinedText: combinedText,
	}

	finalPrompt, err := reduceBuilder.BuildReduce(reduceData)
	if err != nil {
		return "", fmt.Errorf("最終 Reduce プロンプトの生成に失敗しました: %w", err)
	}

	finalResponse, err := e.client.GenerateContent(ctx, finalPrompt, "gemini-2.5-flash")
	if err != nil {
		return "", fmt.Errorf("LLM最終構造化処理（Reduceフェーズ）に失敗しました: %w", err)
	}

	return finalResponse.Text, nil
}
