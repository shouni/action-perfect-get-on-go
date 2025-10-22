package prompts

import (
	_ "embed"
	"fmt"
	"log"
	"strings"
	"text/template"
)

//go:embed map_segment_prompt.md
var MapSegmentPromptTemplate string

//go:embed reduce_final_prompt.md
var ReduceFinalPromptTemplate string

// ----------------------------------------------------------------
// テンプレート構造体
// ----------------------------------------------------------------

// MapTemplateData は、Mapフェーズのプロンプトに埋め込むデータ構造体です。
type MapTemplateData struct {
	SegmentText string
}

// ReduceTemplateData は、Reduceフェーズのプロンプトに埋め込むデータ構造体です。
type ReduceTemplateData struct {
	CombinedText string
}

// ----------------------------------------------------------------
// ビルダー実装
// ----------------------------------------------------------------

// PromptBuilder はプロンプトの構成とテンプレート実行を管理します。
// text/template を使用するため、fmt.Sprintf の代わりに Execute メソッドを使用します。
type PromptBuilder struct {
	// 埋め込み済みのテンプレート文字列からパースされた Goテンプレートを保持します
	tmpl *template.Template
}

// NewMapPromptBuilder は Mapフェーズ用の PromptBuilder を初期化します。
func NewMapPromptBuilder() *PromptBuilder {
	// テンプレート変数の名前（SegmentText）をテンプレートファイル内のマーカーと一致させる必要があります。
	tmpl, err := template.New("map_segment").Parse(MapSegmentPromptTemplate)
	if err != nil {
		// プログラム起動時のパース失敗は致命的なので log.Fatal で落とします。
		log.Fatalf("MapSegmentPromptTemplateのパースに失敗しました: %v", err)
	}
	return &PromptBuilder{tmpl: tmpl}
}

// NewReducePromptBuilder は Reduceフェーズ用の PromptBuilder を初期化します。
func NewReducePromptBuilder() *PromptBuilder {
	tmpl, err := template.New("reduce_final").Parse(ReduceFinalPromptTemplate)
	if err != nil {
		log.Fatalf("ReduceFinalPromptTemplateのパースに失敗しました: %v", err)
	}
	return &PromptBuilder{tmpl: tmpl}
}

// BuildMap は MapTemplateData を埋め込み、Geminiへ送るための最終的なプロンプト文字列を完成させます。
func (b *PromptBuilder) BuildMap(data MapTemplateData) (string, error) {
	if b.tmpl == nil {
		return "", fmt.Errorf("prompt template is not initialized (Map)")
	}

	var sb strings.Builder
	// テンプレートを実行し、結果を strings.Builder に書き込む
	if err := b.tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("Mapプロンプトの実行に失敗しました: %w", err)
	}

	// 必須項目チェック: SegmentText が空だとMap処理の意図が失われるため
	if data.SegmentText == "" {
		return "", fmt.Errorf("Mapプロンプト実行失敗: SegmentTextが空です (template: %s)", b.tmpl.Name())
	}

	return sb.String(), nil
}

// BuildReduce は ReduceTemplateData を埋め込み、Geminiへ送るための最終的なプロンプト文字列を完成させます。
func (b *PromptBuilder) BuildReduce(data ReduceTemplateData) (string, error) {
	if b.tmpl == nil {
		return "", fmt.Errorf("prompt template is not initialized (Reduce)")
	}

	var sb strings.Builder
	// テンプレートを実行し、結果を strings.Builder に書き込む
	if err := b.tmpl.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("Reduceプロンプトの実行に失敗しました: %w", err)
	}

	// 必須項目チェック: CombinedText が空だとReduce処理の意図が失われるため
	if data.CombinedText == "" {
		return "", fmt.Errorf("Reduceプロンプト実行失敗: CombinedTextが空です (template: %s)", b.tmpl.Name())
	}

	return sb.String(), nil
}
