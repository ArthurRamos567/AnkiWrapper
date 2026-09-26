package chat

import (
	"html"
	"html/template"
	"regexp"
	"strings"
	"sync"
	"unicode"

	"anki-wrapper/internal/domain"
)

// furiganaRegex matches Kanji, Katakana, digits or latin characters immediately followed by [reading] in brackets.
// Hiragana (phonetic by definition) is excluded so preceding particles (like は, を, に) remain untouched.
var furiganaRegex = regexp.MustCompile(`([一-龯々〆ヵヶァ-ヴー0-9０-９a-zA-Z]+)\[([ぁ-んァ-ヶー・]+)\]`)

var (
	vocabMu       sync.RWMutex
	knownWords    = make(map[string]struct{})
	knownReadings = make(map[string]struct{})
	knownStems    = make(map[string]struct{})
)

// SetKnownVocab updates the in-memory dictionary of words that exist in the user's collection.
func SetKnownVocab(items []domain.VocabItem) {
	vocabMu.Lock()
	defer vocabMu.Unlock()

	knownWords = make(map[string]struct{}, len(items)*2)
	knownReadings = make(map[string]struct{}, len(items)*2)
	knownStems = make(map[string]struct{}, len(items)*2)

	for _, item := range items {
		w := strings.TrimSpace(item.Word)
		if w != "" {
			knownWords[w] = struct{}{}
			stem := stripTrailingHiragana(w)
			if stem != "" {
				knownStems[stem] = struct{}{}
			}
		}
		r := strings.TrimSpace(item.Reading)
		if r != "" {
			knownReadings[r] = struct{}{}
		}
	}
}

// ClearKnownVocab resets the known vocabulary cache.
func ClearKnownVocab() {
	vocabMu.Lock()
	defer vocabMu.Unlock()
	knownWords = make(map[string]struct{})
	knownReadings = make(map[string]struct{})
	knownStems = make(map[string]struct{})
}

func stripTrailingHiragana(s string) string {
	return strings.TrimRightFunc(s, func(r rune) bool {
		return unicode.In(r, unicode.Hiragana)
	})
}

// IsWordKnown checks if a given word or reading exists in the user's Anki collection.
func IsWordKnown(word, reading string) bool {
	vocabMu.RLock()
	defer vocabMu.RUnlock()

	// If no vocabulary has been registered, do not highlight anything as new.
	if len(knownWords) == 0 {
		return true
	}

	w := strings.TrimSpace(word)
	r := strings.TrimSpace(reading)
	rClean := strings.ReplaceAll(r, "・", "")

	if _, ok := knownWords[w]; ok {
		return true
	}
	if _, ok := knownReadings[r]; ok {
		return true
	}
	if _, ok := knownReadings[rClean]; ok {
		return true
	}
	if _, ok := knownStems[w]; ok {
		return true
	}

	// Check stem
	stem := stripTrailingHiragana(w)
	if stem != "" {
		if _, ok := knownStems[stem]; ok {
			return true
		}
		if _, ok := knownWords[stem]; ok {
			return true
		}
	}

	// Check polite prefix お / ご
	trimmedPrefix := strings.TrimPrefix(strings.TrimPrefix(w, "お"), "ご")
	if trimmedPrefix != w {
		if _, ok := knownWords[trimmedPrefix]; ok {
			return true
		}
		if _, ok := knownStems[trimmedPrefix]; ok {
			return true
		}
	}

	return false
}

// FuriganaToHTML converts Anki-style furigana to HTML <ruby> tags.
// Safe text escaping is performed on non-ruby characters.
// Words not found in the user's Anki collection receive the 'vocab-new' class.
func FuriganaToHTML(text string) string {
	if text == "" {
		return ""
	}

	safeText := html.EscapeString(text)

	result := furiganaRegex.ReplaceAllStringFunc(safeText, func(match string) string {
		sub := furiganaRegex.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		word := sub[1]
		reading := sub[2]

		if IsWordKnown(word, reading) {
			return `<ruby class="furigana-ruby">` + word + `<rt>` + reading + `</rt></ruby>`
		}
		return `<ruby class="furigana-ruby vocab-new" title="Palavra nova (não cadastrada no Anki)">` + word + `<rt>` + reading + `</rt></ruby>`
	})

	return result
}

// FuriganaToSafeHTML returns template.HTML directly to avoid double escaping in Go templates.
func FuriganaToSafeHTML(text string) template.HTML {
	return template.HTML(FuriganaToHTML(text))
}
