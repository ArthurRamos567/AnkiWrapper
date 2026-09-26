package anki

import (
	"html"
	"regexp"
	"strings"
)

var (
	styleRegex       = regexp.MustCompile(`(?is)<style.*?</style>`)
	scriptRegex      = regexp.MustCompile(`(?is)<script.*?</script>`)
	metaHeaderRegex  = regexp.MustCompile(`(?is)<i>.*?</i>`)
	extraBoxRegex    = regexp.MustCompile(`(?is)<div[^>]*data-sc-(class|content)=["'](extra-box|example-sentence|forms)["'][^>]*>.*?</div>`)
	formsTableRegex  = regexp.MustCompile(`(?is)<div[^>]*data-sc-content=["']forms["'].*?</div>`)
	tagRegex         = regexp.MustCompile(`<[^>]*>`)
	spaceRegex       = regexp.MustCompile(`\s+`)
	jmdictFooter     = regexp.MustCompile(`(?i)(JMdict|Tatoeba).*`)
	freqTags         = regexp.MustCompile(`\([★▲△▽▼priority\s,special\sreading]+\)`)
)

// SanitizeMeaning removes HTML tags, Yomitan styles, metadata headers, and extra glossary clutter.
func SanitizeMeaning(rawHTML string) string {
	if rawHTML == "" {
		return ""
	}

	// 1. Remove style and script blocks
	clean := styleRegex.ReplaceAllString(rawHTML, "")
	clean = scriptRegex.ReplaceAllString(clean, "")

	// 2. Remove Yomitan metadata headers: <i>(priority form, ★, Jitendex...)</i>
	clean = metaHeaderRegex.ReplaceAllString(clean, "")

	// 3. Remove extra boxes (example sentences, forms tables)
	clean = extraBoxRegex.ReplaceAllString(clean, "")
	clean = formsTableRegex.ReplaceAllString(clean, "")

	// 4. Replace breaks and list items with readable punctuation
	clean = strings.ReplaceAll(clean, "<br>", "; ")
	clean = strings.ReplaceAll(clean, "<br/>", "; ")
	clean = strings.ReplaceAll(clean, "<br />", "; ")
	clean = strings.ReplaceAll(clean, "</li>", "; ")

	// 5. Remove remaining HTML tags
	clean = tagRegex.ReplaceAllString(clean, " ")

	// 6. Decode HTML entities
	clean = html.UnescapeString(clean)

	// 7. Strip leftover dictionary footer references and frequency stars
	clean = jmdictFooter.ReplaceAllString(clean, "")
	clean = freqTags.ReplaceAllString(clean, "")

	// 8. Normalize whitespace
	clean = spaceRegex.ReplaceAllString(clean, " ")
	clean = strings.TrimSpace(clean)
	clean = strings.Trim(clean, ";, -")

	return clean
}
