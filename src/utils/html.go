package utils

import (
	"fmt"
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
	reCodeBlock  = regexp.MustCompile("(?s)```(\\w*)\\n(.*?)```")
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reHeading    = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	reHR         = regexp.MustCompile(`(?m)^---+$`)
	reListItem   = regexp.MustCompile(`(?m)^(\s*)[-*]\s+(.+)$`)
)

// MarkdownToFoldedHTML 将 Markdown 转为 Telegram HTML 并包裹折叠标签
func MarkdownToFoldedHTML(title, mdContent string) string {
	body := markdownToTelegramHTML(mdContent)
	return "<b>" + html.EscapeString(title) + "</b>\n<blockquote expandable>" + body + "</blockquote>"
}

func markdownToTelegramHTML(md string) string {
	// 用唯一占位符保护代码块和行内代码，避免被 HTML 转义破坏
	type placeholder struct {
		key     string
		replace string
	}
	var holders []placeholder
	idx := 0

	nextKey := func() string {
		k := fmt.Sprintf("\x00PH%d\x00", idx)
		idx++
		return k
	}

	// 1. 提取代码块
	md = reCodeBlock.ReplaceAllStringFunc(md, func(match string) string {
		subs := reCodeBlock.FindStringSubmatch(match)
		lang, code := subs[1], strings.TrimRight(subs[2], "\n")
		escaped := html.EscapeString(code)
		var rep string
		if lang != "" {
			rep = "<pre><code class=\"language-" + html.EscapeString(lang) + "\">" + escaped + "</code></pre>"
		} else {
			rep = "<pre><code>" + escaped + "</code></pre>"
		}
		key := nextKey()
		holders = append(holders, placeholder{key: key, replace: rep})
		return key
	})

	// 2. 提取行内代码
	md = reInlineCode.ReplaceAllStringFunc(md, func(match string) string {
		subs := reInlineCode.FindStringSubmatch(match)
		escaped := html.EscapeString(subs[1])
		rep := "<code>" + escaped + "</code>"
		key := nextKey()
		holders = append(holders, placeholder{key: key, replace: rep})
		return key
	})

	// 3. 转换 heading → bold
	md = reHeading.ReplaceAllString(md, "\x01B$1\x01b")

	// 3.1 转换 --- 水平线
	md = reHR.ReplaceAllString(md, "————————")

	// 3.2 转换列表项 - xxx → • xxx
	md = reListItem.ReplaceAllString(md, "$1• $2")

	// 3.3 转换 bold **text**
	md = reBold.ReplaceAllString(md, "\x01B$1\x01b")

	// 4. 转义剩余内容
	md = html.EscapeString(md)

	// 5. 还原 bold 标记（转义后 \x01 变成 html 实体？不会，\x01 不是 HTML 特殊字符）
	md = strings.ReplaceAll(md, "\x01B", "<b>")
	md = strings.ReplaceAll(md, "\x01b", "</b>")

	// 6. 还原占位符（占位符中的 \x00 也不会被 html.EscapeString 改变）
	for _, h := range holders {
		escaped := html.EscapeString(h.key)
		md = strings.Replace(md, escaped, h.replace, 1)
	}

	return md
}
