package utils

import (
	"html"
)

// EscapeHTML 转义 HTML 特殊字符
func EscapeHTML(s string) string {
	return html.EscapeString(s)
}

// FoldText2Html 生成折叠 HTML 文本
func FoldText2Html(title, content string) string {
	escaped := html.EscapeString(content)
	return "<b>" + html.EscapeString(title) + "</b>\n<blockquote expandable>" + escaped + "</blockquote>"
}

// MarkdownToFoldedHTML 将 Markdown 文本包裹成折叠 HTML
func MarkdownToFoldedHTML(title, mdContent string) string {
	escaped := html.EscapeString(mdContent)
	return "<b>" + html.EscapeString(title) + "</b>\n<blockquote expandable>" + escaped + "</blockquote>"
}
