package cleaner

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	extTypes "github.com/shouni/go-web-exact/v2/pkg/types"
)

// Cleaner はコンテンツのクリーンアップと要約を担当します。
type Cleaner struct {
	builders PromptBuilders
	executor LLMExecutor // LLMExecutor インターフェースに依存
}

// NewCleaner は新しい Cleaner インスタンスを作成し、PromptBuilderを一度だけ初期化します。
func NewCleaner(builders PromptBuilders, executor LLMExecutor) (*Cleaner, error) {
	if executor == nil {
		return nil, fmt.Errorf("LLM Executor は nil にできません")
	}

	return &Cleaner{
		builders: builders,
		executor: executor,
	}, nil
}

// CleanAndStructureText は、MapReduce処理を実行し、最終的なクリーンアップと構造化を行います。
// LLMExecutor に依存することで、APIキーの処理や並列実行の詳細から解放されています。
func (c *Cleaner) CleanAndStructureText(ctx context.Context, results []extTypes.URLResult) (string, error) {
	// 1. MapフェーズのためのURL単位のテキスト分割
	var allSegments []Segment
	for _, res := range results {
		// URLResultのContentを個別にセグメント分割
		segments := segmentText(res.Content, MaxSegmentChars)
		for _, segText := range segments {
			allSegments = append(allSegments, Segment{Text: segText, URL: res.URL})
		}
	}

	slog.Info("コンテンツをURL単位でセグメントに分割しました。中間要約を開始します。",
		slog.Int("total_segments", len(allSegments)))

	// 2. Mapフェーズの実行（Executorに委譲）
	intermediateSummaries, err := c.executor.ExecuteMap(ctx, allSegments, c.builders.MapBuilder)
	if err != nil {
		return "", fmt.Errorf("セグメント処理（Mapフェーズ）に失敗しました: %w", err)
	}

	// 3. Reduceフェーズの準備：中間要約の結合
	finalCombinedText := strings.Join(intermediateSummaries, "\n\n--- INTERMEDIATE SUMMARY END ---\n\n")

	// 4. Reduceフェーズ：最終的な統合と構造化のためのLLM呼び出し（Executorに委譲）
	slog.Info("中間要約の結合が完了しました。最終的な構造化（Reduceフェーズ）を開始します。")

	finalResponseText, err := c.executor.ExecuteReduce(ctx, finalCombinedText, c.builders.ReduceBuilder)
	if err != nil {
		return "", fmt.Errorf("LLM最終構造化処理（Reduceフェーズ）に失敗しました: %w", err)
	}

	// 5. 最終出力のクリーンアップ（マーカー削除）
	cleanedText := cleanFinalOutput(finalResponseText)

	return cleanedText, nil
}
