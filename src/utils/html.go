package utils

import (
	"html"
	"regexp"
	"strings"
)

// EscapeHTML 转义 HTML 特殊字符
func EscapeHTML(s string) string {
	return html.EscapeString(s)
}

// FoldText2Html 生成折叠 HTML 文本（纯文本内容，自动转义）
func FoldText2Html(title, content string) string {
	escaped := html.EscapeString(content)
	return "<b>" + html.EscapeString(title) + "</b>\n<blockquote expandable>" + escaped + "</blockquote>"
}

var (
	// 匹配 ```lang\n...\n``` 代码块
	reCodeBlock = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	// 匹配 `inline code`
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	// 匹配 **bold**
	reBold = regexp.MustCompile(`\*\*(.+?)\*\*`)
	// 匹配 *italic* (不匹配 ** 开头的)
	reItalic = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
)

// MarkdownToFoldedHTML 将 Markdown 转为 Telegram HTML 并包裹折叠标签
func MarkdownToFoldedHTML(title, mdContent string) string {
	body := markdownToTelegramHTML(mdContent)
	return "<b>" + html.EscapeString(title) + "</b>\n<blockquote expandable>" + body + "</blockquote>"
}

// markdownToTelegramHTML 将常见 markdown 语法转为 Telegram 支持的 HTML
func markdownToTelegramHTML(md string) string {
	// 1. 提取代码块，用占位符替换，避免内部被转义
	type codeBlock struct {
		lang string
		code string
	}
	var blocks []codeBlock
	placeholder := "\x00CODEBLOCK%d\x00"

	result := reCodeBlock.ReplaceAllStringFunc(md, func(match string) string {
		subs := reCodeBlock.FindStringSubmatch(match)
		lang := subs[1]
		code := subs[2]
		idx := len(blocks)
		blocks = append(blocks, codeBlock{lang: lang, code: code})
		return strings.Replace(placeholder, "%d", strings.Repeat("0", 1)+string(rune('0'+idx)), 1)
	})

	// 简化：用索引占位
	blocks = nil
	result = reCodeBlock.ReplaceAllStringFunc(md, func(match string) string {
		subs := reCodeBlock.FindStringSubmatch(match)
		idx := len(blocks)
		blocks = append(blocks, codeBlock{lang: subs[1], code: subs[2]})
		return "\x00CB" + string(rune(idx)) + "\x00"
	})

	// 2. 提取行内代码
	type inlineCode struct {
		code string
	}
	var inlines []inlineCode
	result = reInlineCode.ReplaceAllStringFunc(result, func(match string) string {
		subs := reInlineCode.FindStringSubmatch(match)
		idx := len(inlines)
		inlines = append(inlines, inlineCode{code: subs[1]})
		return "\x00IC" + string(rune(idx)) + "\x00"
	})

	// 3. 转义剩余 HTML
	result = html.EscapeString(result)

	// 4. 转换 markdown 格式
	result = reBold.ReplaceAllString(result, "<b>$1</b>")

	// 5. 还原行内代码
	for i, ic := range inlines {
		escaped := html.EscapeString(ic.code)
		result = strings.Replace(result, html.EscapeString("\x00IC"+string(rune(i))+"\x00"), "<code>"+escaped+"</code>", 1)
	}

	// 6. 还原代码块
	for i, cb := range blocks {
		escaped := html.EscapeString(cb.code)
		// 去掉末尾多余换行
		escaped = strings.TrimRight(escaped, "\n")
		var replacement string
		if cb.lang != "" {
			replacement = "<pre><code class=\"language-" + html.EscapeString(cb.lang) + "\">" + escaped + "</code></pre>"
		} else {
			replacement = "<pre><code>" + escaped + "</code></pre>"
		}
		result = strings.Replace(result, html.EscapeString("\x00CB"+string(rune(i))+"\x00"), replacement, 1)
	}

	return result
}
