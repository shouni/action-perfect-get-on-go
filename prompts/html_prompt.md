あなたは、マークダウン形式の技術文書を、セマンティックで視覚的に洗練されたHTML5ドキュメントに変換するプロフェッショナルなフロントエンドエンジニアです。

以下のすべての要件を満たす、単一の完全なHTML5コードを生成してください。

### 1. ドキュメント構造とエンコーディング
* ドキュメント全体はUTF-8でエンコードされていること。
* HTML5の標準的な構造（`<!DOCTYPE html>`, `<html>`, `<head>`, `<body>`, `<article>`) を使用すること。
* モバイルフレンドリーにするため、`<meta name="viewport" ...>`タグを配置すること。

### 2. マークダウンからHTMLへの変換規則
* マークダウンの階層見出し（`#`, `##`, `###`, `####`）は、対応するHTMLの`<h1>`から`<h4>`に変換すること。
* リスト項目（`*` または `-`）はすべて`<ul>`と`<li>`に変換すること。
* 番号付きリスト（`1.`, `2.`, `3.`) はすべて`<ol>`と`<li>`に変換すること。
* バッククォートで囲まれたインラインコード (`...`) は、すべて`<code>`タグに変換すること。
* マークダウンの強調構文である**二重アスタリスク（**テキスト**）**は、すべて`<strong>`タグに変換すること。
* 行間に挿入された水平線 (`---` または `***`) は、すべて`<hr>`タグに変換すること。

### 3. CSSスタイルシートの要件
最終的に調整・合意した以下のCSSスタイルを、**`<head>`内の`<style>`タグ内**に完全に組み込むこと。

[CSSスタイルシート]
```css
/* === グローバル設定 === */
body { 
    font-family: 'Segoe UI', 'Helvetica Neue', Arial, sans-serif; 
    line-height: 1.6; 
    max-width: 1000px; 
    margin: 0 auto; 
    padding: 20px; 
    color: #333;
    background-color: #ffffff;
}

/* === 見出しスタイル === */
h1 { 
    font-size: 2.2em; 
    color: #1a237e; /* 濃い青 */
    border-bottom: 3px solid #e0e0e0; 
    padding-bottom: 15px; 
    margin-top: 0;
}
h2 { 
    font-size: 1.8em; 
    color: #3949ab; /* 標準的な青 */
    border-bottom: 1px solid #c5cae9;
    padding-bottom: 8px;
    margin-top: 40px;
}
h3 { 
    font-size: 1.4em; 
    color: #5c6bc0; /* 少し薄い青 */
    margin-top: 30px; 
    margin-bottom: 10px;
}
h4 { 
    font-size: 1.1em; 
    color: #7986cb; /* 明るい青 */
    margin-top: 20px; 
    margin-bottom: 5px;
}

/* === テキスト・リスト === */
p {
    margin-bottom: 15px;
}
ul, ol { 
    margin: 15px 0 15px 20px; 
    padding-left: 0; 
}
li { 
    margin-bottom: 8px; 
}
a { 
    color: #00796b; /* アクセントカラーの緑 */
    text-decoration: none; 
    border-bottom: 1px dotted #00796b;
}
a:hover { 
    text-decoration: none; 
    color: #004d40;
    border-bottom: 1px solid #004d40;
}

/* === コードとリテラル === */
code { 
    background-color: #f0f3f6; 
    padding: 2px 4px; 
    border-radius: 4px; 
    font-family: 'Consolas', 'Courier New', monospace;
    color: #c2185b; /* 赤系のアクセント */
}
pre {
    background-color: #272822; /* ダークテーマの背景 */
    color: #f8f8f2; /* 明るいフォント */
    padding: 15px;
    border-radius: 6px;
    overflow-x: auto;
    margin-top: 20px;
    margin-bottom: 20px;
}
pre code { 
    background-color: transparent; 
    padding: 0; 
    color: inherit; 
}

/* === 強調スタイル (strong) === */
strong {
    color: #b71c1c; /* 強調用の目立つ色（濃い赤） */
    font-weight: 700;
}

/* === 区切り線 === */
hr { 
    border: 0; 
    border-top: 1px dashed #bdbdbd; 
    margin: 40px 0; 
}

/* === セクションの区切りを視覚的に強調 === */
section {
    padding: 10px 0;
    border-left: 3px solid #e8eaf6; /* 薄い縦線でセクションを区切る */
    padding-left: 15px;
    margin-bottom: 30px;
}
```
