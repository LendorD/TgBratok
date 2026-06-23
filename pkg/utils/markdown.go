// Package utils содержит вспомогательные функции, не зависящие от слоёв приложения.
package utils

import (
	"fmt"
	"regexp"
	"strings"
)

// Регулярки для разметки, которую обычно выдаёт модель.
var (
	reCodeBlock  = regexp.MustCompile("(?s)```([a-zA-Z0-9_+-]*)\\n?(.*?)```")
	reInlineCode = regexp.MustCompile("`([^`\\n]+)`")
	reBold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reBoldUnder  = regexp.MustCompile(`__([^_]+)__`)
	reHeader     = regexp.MustCompile(`(?m)^\s{0,3}#{1,6}\s+(.+?)\s*$`)
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
)

// MarkdownToHTML конвертирует Markdown ответов модели в ограниченный HTML,
// который понимает Telegram (b, i, code, pre, a). Блоки кода и инлайн-код
// прячутся за плейсхолдеры, чтобы их содержимое не трогали остальные замены.
func MarkdownToHTML(s string) string {
	var blocks []string
	s = reCodeBlock.ReplaceAllStringFunc(s, func(m string) string {
		sub := reCodeBlock.FindStringSubmatch(m)
		lang, code := sub[1], strings.Trim(sub[2], "\n")
		var html string
		if lang != "" {
			html = fmt.Sprintf("<pre><code class=\"language-%s\">%s</code></pre>", lang, htmlEscape(code))
		} else {
			html = "<pre>" + htmlEscape(code) + "</pre>"
		}
		blocks = append(blocks, html)
		return fmt.Sprintf("\x00B%d\x00", len(blocks)-1)
	})

	var inlines []string
	s = reInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		sub := reInlineCode.FindStringSubmatch(m)
		inlines = append(inlines, "<code>"+htmlEscape(sub[1])+"</code>")
		return fmt.Sprintf("\x00I%d\x00", len(inlines)-1)
	})

	// Экранируем обычный текст, затем накладываем разметку.
	s = htmlEscape(s)
	s = reHeader.ReplaceAllString(s, "<b>$1</b>")
	s = reBold.ReplaceAllString(s, "<b>$1</b>")
	s = reBoldUnder.ReplaceAllString(s, "<b>$1</b>")
	s = reLink.ReplaceAllString(s, `<a href="$2">$1</a>`)

	// Возвращаем код на место.
	for i, html := range inlines {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00I%d\x00", i), html)
	}
	for i, html := range blocks {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00B%d\x00", i), html)
	}
	return s
}

// htmlEscape экранирует символы, специальные для HTML Telegram.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
