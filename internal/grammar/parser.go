package grammar

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"anki-wrapper/internal/domain"
)

var officialSlugMap = map[string]string{
	"noun-related-particles":              "nounparticles",
	"particles-used-with-verbs":           "verbparticles",
	"intro-to-particles":                  "particles",
	"expressing-state-of-being":           "stateofbeing",
	"adjectives":                          "adjectives",
	"verb-basics":                         "verbs",
	"negative-verbs":                      "negativeverbs",
	"past-tense":                          "pastverbs",
	"transitive-and-intransitive-verbs":   "transitive",
	"relative-clauses-and-sentence-order": "clauses",
	"adverbs-and-sentence-ending-particles": "adverbs",
	"polite-form-and-verb-stems":          "polite",
	"people":                              "people",
	"defining-and-describing":             "defining",
	"compound-sentences":                  "compound",
	"te-form":                             "teform",
	"potential-form":                      "potential",
	"suru-naru":                           "surunaru",
	"conditionals":                        "conditionals",
	"must":                                "must",
	"desire-and-suggestions":              "desire",
	"acting-on-relative-clauses":          "relative",
	"trying":                              "try",
	"giving-and-receiving":                "giving",
	"making-requests":                     "requests",
	"numbers-and-counting":                "numbers",
	"formal-expressions":                  "formal",
	"causative-and-passive-verbs":         "causepass",
	"honorifics":                          "honorific",
	"unintended-actions":                  "unintended",
	"generic-nouns":                       "genericnouns",
	"degrees-of-certainty":                "certainty",
	"difficulty":                          "difficult",
	"time-specific-actions":               "timeactions",
	"lack-of-change":                      "nochange",
	"hypotheses-and-conclusions":          "conclusions",
	"more-negative-verbs":                 "negativeverbs2",
	"similarity":                          "similarity",
	"comparisons":                         "comparison",
	"amounts":                             "amount",
	"hiragana":                            "hiragana",
	"katakana":                            "katakana",
	"kanji":                               "kanji",
}

// ParseDir recursively scans a directory of Tae Kim markdown files and parses them.
func ParseDir(baseDir string) ([]domain.GrammarLesson, error) {
	var lessons []domain.GrammarLesson

	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		lesson, err := ParseFile(baseDir, path)
		if err != nil {
			return err
		}
		lessons = append(lessons, *lesson)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("falha ao processar diretório de gramática: %w", err)
	}

	return lessons, nil
}

// ParseFile parses a single markdown file with YAML frontmatter.
func ParseFile(baseDir, filePath string) (*domain.GrammarLesson, error) {
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler arquivo %s: %w", filePath, err)
	}

	relPath, err := filepath.Rel(baseDir, filePath)
	if err != nil {
		relPath = filePath
	}

	id := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
	slug := strings.TrimSuffix(filepath.Base(filePath), ".md")

	scanner := bufio.NewScanner(strings.NewReader(string(contentBytes)))
	inFrontmatter := false
	frontmatterEnded := false

	var title string
	var chapterNumber int
	var chapter string
	var part int
	var bodyBuilder strings.Builder

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !frontmatterEnded {
			if trimmed == "---" {
				if !inFrontmatter {
					inFrontmatter = true
					continue
				} else {
					inFrontmatter = false
					frontmatterEnded = true
					continue
				}
			}

			if inFrontmatter {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					val = strings.Trim(val, `"'`)

					switch key {
					case "title":
						title = val
					case "chapterNumber":
						chapterNumber, _ = strconv.Atoi(val)
					case "chapter":
						chapter = val
					case "part":
						part, _ = strconv.Atoi(val)
					}
				}
				continue
			}
		}

		bodyBuilder.WriteString(line)
		bodyBuilder.WriteString("\n")
	}

	if title == "" {
		title = strings.Title(strings.ReplaceAll(slug, "-", " "))
	}
	if chapter == "" {
		chapter = "Gramática Geral"
	}

	sectionKey := fmt.Sprintf("%d.%d", chapterNumber, part)
	if chapterNumber == 0 && part == 0 {
		sectionKey = slug
	}

	officialSlug := slug
	if mapped, ok := officialSlugMap[slug]; ok {
		officialSlug = mapped
	}
	officialURL := fmt.Sprintf("https://guidetojapanese.org/learn/grammar/%s", officialSlug)
	webURL := fmt.Sprintf("https://jp.saeris.gg/learn/grammar/%s", id)

	ref := &domain.GrammarReference{
		ID:         id,
		Title:      title,
		SectionKey: sectionKey,
		Chapter:    chapter,
		URL:        officialURL,
		WebURL:     webURL,
	}

	return &domain.GrammarLesson{
		ID:            id,
		Title:         title,
		ChapterNumber: chapterNumber,
		Chapter:       chapter,
		Part:          part,
		SectionKey:    sectionKey,
		Slug:          slug,
		URL:           officialURL,
		WebURL:        webURL,
		FilePath:      relPath,
		Content:       strings.TrimSpace(bodyBuilder.String()),
		Reference:     ref,
	}, nil
}
