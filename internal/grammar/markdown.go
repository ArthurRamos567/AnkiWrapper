package grammar

import (
	"html/template"
	"regexp"
	"strings"

	"anki-wrapper/internal/chat"
)

var (
	doublePlusFuriganaRe = regexp.MustCompile(`\+\+\{([^}|]+)\|([^}]+)\}\+\+`)
	bracketFuriganaRe    = regexp.MustCompile(`\{([^}|]+)\|([^}]+)\}`)
	highlightRe          = regexp.MustCompile(`==([^=]+)==`)
	doubleQuoteRe        = regexp.MustCompile(`""([^""]+)""`)
	inlineCodeRe         = regexp.MustCompile("`([^`]+)`")
	boldRe               = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe             = regexp.MustCompile(`\*([^*]+)\*`)
	linkRe               = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
)

// RenderMarkdown converts Tae Kim's specialized Markdown format into clean, semantic HTML.
func RenderMarkdown(content string) template.HTML {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var htmlBuilder strings.Builder

	inTable := false
	tableHeaderDone := false
	inList := false
	inNumberedList := false

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Close lists if non-list line
		if !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "* ") && inList {
			htmlBuilder.WriteString("</ul>\n")
			inList = false
		}
		isNumbered := len(trimmed) > 2 && (trimmed[0] >= '0' && trimmed[0] <= '9') && strings.HasPrefix(trimmed[1:], ". ") ||
			(len(trimmed) > 3 && (trimmed[0] >= '0' && trimmed[0] <= '9') && (trimmed[1] >= '0' && trimmed[1] <= '9') && strings.HasPrefix(trimmed[2:], ". "))
		if !isNumbered && inNumberedList {
			htmlBuilder.WriteString("</ol>\n")
			inNumberedList = false
		}

		// Close table if non-table line
		if (!strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|")) && inTable {
			htmlBuilder.WriteString("</tbody></table>\n")
			inTable = false
			tableHeaderDone = false
		}

		if trimmed == "" {
			continue
		}

		// Horizontal rule
		if trimmed == "---" || trimmed == "***" {
			htmlBuilder.WriteString("<hr style=\"border:0;border-top:1px solid #ccc;margin:1.5rem 0;\">\n")
			continue
		}

		// Headers
		if strings.HasPrefix(trimmed, "#### ") {
			text := strings.TrimPrefix(trimmed, "#### ")
			htmlBuilder.WriteString("<h4 style=\"margin-top:1rem;margin-bottom:0.35rem;font-size:1rem;\">" + formatInline(text) + "</h4>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			text := strings.TrimPrefix(trimmed, "### ")
			htmlBuilder.WriteString("<h3 style=\"margin-top:1.25rem;margin-bottom:0.5rem;font-size:1.15rem;\">" + formatInline(text) + "</h3>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			text := strings.TrimPrefix(trimmed, "## ")
			htmlBuilder.WriteString("<h2 style=\"margin-top:1.5rem;margin-bottom:0.5rem;font-size:1.3rem;border-bottom:1px solid #eee;padding-bottom:0.25rem;\">" + formatInline(text) + "</h2>\n")
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			text := strings.TrimPrefix(trimmed, "# ")
			htmlBuilder.WriteString("<h1 style=\"margin-top:1.75rem;margin-bottom:0.75rem;font-size:1.5rem;\">" + formatInline(text) + "</h1>\n")
			continue
		}

		// Tables
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			cells := splitTableRow(trimmed)
			// Check if separator line
			isSeparator := true
			for _, cell := range cells {
				c := strings.TrimSpace(cell)
				if c != "" && !strings.Contains(c, "-") {
					isSeparator = false
					break
				}
			}

			if isSeparator {
				tableHeaderDone = true
				continue
			}

			if !inTable {
				inTable = true
				htmlBuilder.WriteString("<table style=\"width:100%;margin:1rem 0;border-collapse:collapse;font-size:0.9rem;\">\n")
				htmlBuilder.WriteString("<thead><tr style=\"background:#f8f9fa;\">")
				for _, cell := range cells {
					htmlBuilder.WriteString("<th style=\"border:1px solid #ccc;padding:6px 10px;text-align:left;\">" + formatInline(cell) + "</th>")
				}
				htmlBuilder.WriteString("</tr></thead><tbody>\n")
				continue
			}

			htmlBuilder.WriteString("<tr>")
			for _, cell := range cells {
				tag := "td"
				if !tableHeaderDone {
					tag = "th"
				}
				htmlBuilder.WriteString("<" + tag + " style=\"border:1px solid #ccc;padding:6px 10px;\">" + formatInline(cell) + "</" + tag + ">")
			}
			htmlBuilder.WriteString("</tr>\n")
			continue
		}

		// Unordered list
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				inList = true
				htmlBuilder.WriteString("<ul style=\"margin:0.5rem 0 0.5rem 1.5rem;padding:0;\">\n")
			}
			text := strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* ")
			htmlBuilder.WriteString("<li style=\"margin-bottom:0.25rem;\">" + formatInline(text) + "</li>\n")
			continue
		}

		// Numbered list
		if isNumbered {
			if !inNumberedList {
				inNumberedList = true
				htmlBuilder.WriteString("<ol style=\"margin:0.5rem 0 0.5rem 1.5rem;padding:0;\">\n")
			}
			dotIdx := strings.Index(trimmed, ". ")
			text := trimmed[dotIdx+2:]
			htmlBuilder.WriteString("<li style=\"margin-bottom:0.5rem;\">" + formatInline(text) + "</li>\n")
			continue
		}

		// Definition list line (: meaning)
		if strings.HasPrefix(trimmed, ": ") {
			text := strings.TrimPrefix(trimmed, ": ")
			htmlBuilder.WriteString("<p style=\"margin:0 0 0.5rem 1.5rem;color:#555;font-size:0.9rem;\">" + formatInline(text) + "</p>\n")
			continue
		}

		// Blockquote
		if strings.HasPrefix(trimmed, "> ") {
			text := strings.TrimPrefix(trimmed, "> ")
			htmlBuilder.WriteString("<blockquote style=\"border-left:3px solid #0055aa;margin:0.75rem 0;padding:0.25rem 0.75rem;background:#f8f9fa;color:#333;\">" + formatInline(text) + "</blockquote>\n")
			continue
		}

		// Regular paragraph
		htmlBuilder.WriteString("<p style=\"margin:0.65rem 0;line-height:1.6;\">" + formatInline(line) + "</p>\n")
	}

	if inList {
		htmlBuilder.WriteString("</ul>\n")
	}
	if inNumberedList {
		htmlBuilder.WriteString("</ol>\n")
	}
	if inTable {
		htmlBuilder.WriteString("</tbody></table>\n")
	}

	return template.HTML(htmlBuilder.String())
}

func splitTableRow(row string) []string {
	trimmed := strings.Trim(row, "|")
	parts := strings.Split(trimmed, "|")
	var result []string
	for _, p := range parts {
		result = append(result, strings.TrimSpace(p))
	}
	return result
}

func formatInline(s string) string {
	// 1. Double quotes
	s = doubleQuoteRe.ReplaceAllString(s, "“$1”")

	// 2. Highlighting
	s = highlightRe.ReplaceAllString(s, `<mark style="background:#e8f0fe;color:#0055aa;padding:0 2px;border-radius:2px;font-weight:bold;">$1</mark>`)

	// 3. Furigana: ++{kanji|reading}++
	s = doublePlusFuriganaRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := doublePlusFuriganaRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		word, reading := sub[1], sub[2]
		if chat.IsWordKnown(word, reading) {
			return `<ruby class="furigana-ruby">` + word + `<rt>` + reading + `</rt></ruby>`
		}
		return `<ruby class="furigana-ruby vocab-new" title="Palavra nova (não cadastrada no Anki)">` + word + `<rt>` + reading + `</rt></ruby>`
	})

	// 4. Furigana: {kanji|reading}
	s = bracketFuriganaRe.ReplaceAllStringFunc(s, func(match string) string {
		sub := bracketFuriganaRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		word, reading := sub[1], sub[2]
		if chat.IsWordKnown(word, reading) {
			return `<ruby class="furigana-ruby">` + word + `<rt>` + reading + `</rt></ruby>`
		}
		return `<ruby class="furigana-ruby vocab-new" title="Palavra nova (não cadastrada no Anki)">` + word + `<rt>` + reading + `</rt></ruby>`
	})

	// 5. Bold & Italic
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")

	// 6. Inline code
	s = inlineCodeRe.ReplaceAllString(s, "<code style=\"background:#f0f0f0;padding:1px 4px;font-size:0.9em;\">$1</code>")

	// 7. Links
	s = linkRe.ReplaceAllString(s, `<a href="$2" style="color:#0055aa;">$1</a>`)

	return s
}
