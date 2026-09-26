package chat

import (
	"strings"
	"testing"

	"anki-wrapper/internal/domain"
)

func TestFuriganaToHTML(t *testing.T) {
	ClearKnownVocab()

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "私[わたし]は日本語[にほんご]を勉強[べんきょう]しています。",
			expected: `<ruby class="furigana-ruby">私<rt>わたし</rt></ruby>は<ruby class="furigana-ruby">日本語<rt>にほんご</rt></ruby>を<ruby class="furigana-ruby">勉強<rt>べんきょう</rt></ruby>しています。`,
		},
		{
			input:    "1[いち]日[にち]に珈琲[コーヒー]を飲[の]みます。",
			expected: `<ruby class="furigana-ruby">1<rt>いち</rt></ruby><ruby class="furigana-ruby">日<rt>にち</rt></ruby>に<ruby class="furigana-ruby">珈琲<rt>コーヒー</rt></ruby>を<ruby class="furigana-ruby">飲<rt>の</rt></ruby>みます。`,
		},
	}

	for _, tc := range tests {
		actual := FuriganaToHTML(tc.input)
		if actual != tc.expected {
			t.Errorf("For input %s\nExpected:\n%s\nGot:\n%s", tc.input, tc.expected, actual)
		}
	}
}

func TestFuriganaVocabHighlighting(t *testing.T) {
	SetKnownVocab([]domain.VocabItem{
		{Word: "日本語", Reading: "にほんご"},
		{Word: "食べる", Reading: "たべる"},
	})
	defer ClearKnownVocab()

	input := "日本語[にほんご]と飛行機[ひこうき]と食べ[た]る。"
	html := FuriganaToHTML(input)

	// 日本語 should NOT have vocab-new
	if strings.Contains(html, `日本語<rt>にほんご</rt></ruby>`) && strings.Contains(html, `class="furigana-ruby vocab-new">日本語`) {
		t.Errorf("Esperado que 日本語 não tivesse a classe vocab-new, mas obteve:\n%s", html)
	}

	// 食べ should NOT have vocab-new because stem matches 食べる
	if strings.Contains(html, `class="furigana-ruby vocab-new">食べ`) {
		t.Errorf("Esperado que 食べ não tivesse a classe vocab-new por causa do radical 食べる, mas obteve:\n%s", html)
	}

	// 飛行機 SHOULD have vocab-new because it is NOT in the vocab
	if !strings.Contains(html, `class="furigana-ruby vocab-new" title="Palavra nova (não cadastrada no Anki)">飛行機<rt>ひこうき</rt></ruby>`) {
		t.Errorf("Esperado que 飛行機 tivesse a classe vocab-new, mas obteve:\n%s", html)
	}
}
