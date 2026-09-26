package grammar

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	md := `
# The Main Title

## The Section

Here is a paragraph with ""と"" particle and ==highlighted== text.
Also Japanese with furigana ++{ナイフ|na i fu}++ and {フォーク|fo o ku}.

### Vocabulary
++{食べる|た・べる}++
: to eat

1. {友達|とも・だち}==と==話した。
   Talked with friend.

| Term | Meaning |
| --- | --- |
| 本 | book |
| 雑誌 | magazine |

> This is an important rule to remember.
`
	html := string(RenderMarkdown(md))

	checks := []string{
		"The Main Title</h1>",
		"The Section</h2>",
		"“と”",
		`highlighted</mark>`,
		`<ruby class="furigana-ruby">ナイフ<rt>na i fu</rt></ruby>`,
		`<ruby class="furigana-ruby">フォーク<rt>fo o ku</rt></ruby>`,
		`to eat</p>`,
		`<ol style="margin:0.5rem 0 0.5rem 1.5rem;padding:0;">`,
		`Term</th>`,
		`magazine</td>`,
		`<blockquote style=`,
	}

	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("Expected output to contain %q, but got:\n%s", check, html)
		}
	}
}
