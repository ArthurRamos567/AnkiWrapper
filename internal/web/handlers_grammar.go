package web

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"anki-wrapper/internal/domain"
	"anki-wrapper/internal/grammar"
)

// ChapterGroup groups lessons belonging to the same Tae Kim chapter.
type ChapterGroup struct {
	ChapterNumber int
	ChapterName   string
	Lessons       []domain.GrammarLesson
}

// UserNote represents a custom user grammar markdown note in data/knowledge/notes.
type UserNote struct {
	Filename string
	Title    string
	Content  string
	HTML     template.HTML
}

const userNotesDir = "data/knowledge/notes"

func (s *Server) handleGrammarPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vocab := s.getVocabSnapshot()

	tab := strings.TrimSpace(r.URL.Query().Get("tab"))
	if tab == "" {
		tab = "taekim"
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))

	data := map[string]any{
		"ActiveNav":   "grammar",
		"CurrentTab":  tab,
		"VocabCount":  len(vocab),
		"SearchQuery": q,
	}

	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if filter == "" {
		filter = "all"
	}

	// 1. Tae Kim Tab
	var allLessons []domain.GrammarLesson
	if s.grammarRepo != nil {
		if q != "" {
			allLessons, _ = s.grammarRepo.SearchLessons(ctx, q, 60)
		} else {
			allLessons, _ = s.grammarRepo.GetAllLessons(ctx)
		}
	}

	readCount := 0
	for _, l := range allLessons {
		if l.IsRead {
			readCount++
		}
	}
	totalCount := len(allLessons)
	unreadCount := totalCount - readCount
	readPercentage := 0
	if totalCount > 0 {
		readPercentage = int(float64(readCount) / float64(totalCount) * 100)
	}

	var displayedLessons []domain.GrammarLesson
	for _, l := range allLessons {
		if filter == "read" && !l.IsRead {
			continue
		}
		if filter == "unread" && l.IsRead {
			continue
		}
		displayedLessons = append(displayedLessons, l)
	}

	var groups []ChapterGroup
	groupMap := make(map[string]int)
	for _, l := range displayedLessons {
		chap := l.Chapter
		if chap == "" {
			chap = "Gramática Geral"
		}

		idx, exists := groupMap[chap]
		if !exists {
			idx = len(groups)
			groups = append(groups, ChapterGroup{
				ChapterNumber: l.ChapterNumber,
				ChapterName:   chap,
				Lessons:       nil,
			})
			groupMap[chap] = idx
		}
		groups[idx].Lessons = append(groups[idx].Lessons, l)
	}

	data["Groups"] = groups
	data["Lessons"] = displayedLessons
	data["TotalLessons"] = totalCount
	data["ReadCount"] = readCount
	data["UnreadCount"] = unreadCount
	data["ReadPercentage"] = readPercentage
	data["CurrentFilter"] = filter

	// 2. Notes Tab
	notes := listUserNotes(userNotesDir)
	data["Notes"] = notes

	selectedFile := strings.TrimSpace(r.URL.Query().Get("file"))
	var activeNote *UserNote
	if selectedFile != "" {
		for _, n := range notes {
			if n.Filename == selectedFile {
				activeNote = &n
				break
			}
		}
	}
	data["ActiveNote"] = activeNote

	if err := s.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleGrammarLesson(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vocab := s.getVocabSnapshot()

	id := strings.TrimSpace(r.URL.Query().Get("id"))
	section := strings.TrimSpace(r.URL.Query().Get("section"))

	if s.grammarRepo == nil {
		http.Error(w, "Repositório de gramática não disponível", http.StatusServiceUnavailable)
		return
	}

	var lesson *domain.GrammarLesson
	var err error

	if id != "" {
		lesson, err = s.grammarRepo.GetLessonByID(ctx, id)
	} else if section != "" {
		lesson, err = s.grammarRepo.GetLessonBySection(ctx, section)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Erro ao carregar lição: %v", err), http.StatusInternalServerError)
		return
	}

	if lesson == nil {
		http.NotFound(w, r)
		return
	}

	data := map[string]any{
		"ActiveNav":   "grammar",
		"CurrentTab":  "taekim",
		"IsLesson":    true,
		"VocabCount":  len(vocab),
		"Lesson":      lesson,
		"ContentHTML": grammar.RenderMarkdown(lesson.Content),
	}

	if err := s.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleGrammarLessonToggleRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		id = strings.TrimSpace(r.URL.Query().Get("id"))
	}
	if id == "" {
		http.Error(w, "ID da lição obrigatório", http.StatusBadRequest)
		return
	}

	if s.grammarRepo == nil {
		http.Error(w, "Repositório não disponível", http.StatusServiceUnavailable)
		return
	}

	lesson, err := s.grammarRepo.GetLessonByID(r.Context(), id)
	if err != nil || lesson == nil {
		http.Error(w, "Lição não encontrada", http.StatusNotFound)
		return
	}

	var newReadState bool
	explicitVal := r.FormValue("read")
	if explicitVal != "" {
		newReadState = (explicitVal == "1" || explicitVal == "true")
	} else {
		newReadState = !lesson.IsRead
	}

	if err := s.grammarRepo.MarkLessonRead(r.Context(), id, newReadState); err != nil {
		http.Error(w, fmt.Sprintf("Erro ao atualizar status: %v", err), http.StatusInternalServerError)
		return
	}

	lesson.IsRead = newReadState

	format := r.URL.Query().Get("format")
	if format == "button" || strings.HasPrefix(r.Header.Get("HX-Target"), "read-btn") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if lesson.IsRead {
			fmt.Fprintf(w, `<button type="button" hx-post="/grammar/lesson/toggle-read?id=%s&format=button" hx-target="this" hx-swap="outerHTML" style="cursor: pointer; padding: 4px 10px; background: #eafbe8; color: #006600; border: 1px solid #008800; border-radius: 3px; font-size: 0.85rem; font-weight: bold;" title="Clique para desmarcar">✓ Lido (Concluído)</button>`, lesson.ID)
		} else {
			fmt.Fprintf(w, `<button type="button" hx-post="/grammar/lesson/toggle-read?id=%s&format=button" hx-target="this" hx-swap="outerHTML" style="cursor: pointer; padding: 4px 10px; background: #f8f9fa; color: #444; border: 1px solid #ccc; border-radius: 3px; font-size: 0.85rem;" title="Clique para marcar como lido">○ Marcar como Lido</button>`, lesson.ID)
		}
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if lesson.IsRead {
			fmt.Fprintf(w, `<button type="button" hx-post="/grammar/lesson/toggle-read?id=%s" hx-target="this" hx-swap="outerHTML" style="cursor: pointer; font-size: 0.8rem; padding: 2px 6px; border: 1px solid #008800; background: #eafbe8; color: #006600; border-radius: 3px;" title="Marcar como não lido">✓ Lido</button>`, lesson.ID)
		} else {
			fmt.Fprintf(w, `<button type="button" hx-post="/grammar/lesson/toggle-read?id=%s" hx-target="this" hx-swap="outerHTML" style="cursor: pointer; font-size: 0.8rem; padding: 2px 6px; border: 1px solid #ccc; background: #fafafa; color: #666; border-radius: 3px;" title="Marcar como lido">○ Não lido</button>`, lesson.ID)
		}
		return
	}

	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = "/grammar?tab=taekim"
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}

func (s *Server) handleGrammarNoteSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	content := strings.TrimSpace(r.FormValue("content"))
	filename := strings.TrimSpace(r.FormValue("filename"))

	if title == "" {
		title = "Nova Anotação"
	}

	if filename == "" {
		slug := slugify(title)
		if slug == "" {
			slug = "nota"
		}
		filename = slug + ".md"
	} else {
		filename = filepath.Base(filename)
	}

	_ = os.MkdirAll(userNotesDir, 0o755)
	fullPath := filepath.Join(userNotesDir, filename)

	fileBody := fmt.Sprintf("# %s\n\n%s\n", title, content)
	if err := os.WriteFile(fullPath, []byte(fileBody), 0o644); err != nil {
		http.Error(w, fmt.Sprintf("Erro ao salvar nota: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/grammar?tab=notes&file=%s", filename), http.StatusSeeOther)
}

func (s *Server) handleGrammarNoteDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	filename := filepath.Base(strings.TrimSpace(r.FormValue("file")))
	if filename != "" && filename != "." && filename != ".." {
		_ = os.Remove(filepath.Join(userNotesDir, filename))
	}

	http.Redirect(w, r, "/grammar?tab=notes", http.StatusSeeOther)
}

func listUserNotes(dir string) []UserNote {
	var notes []UserNote
	entries, err := os.ReadDir(dir)
	if err != nil {
		return notes
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		content := string(data)
		title := strings.TrimSuffix(e.Name(), ".md")
		lines := strings.Split(content, "\n")
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if strings.HasPrefix(trimmed, "# ") {
				title = strings.TrimPrefix(trimmed, "# ")
				break
			}
		}

		notes = append(notes, UserNote{
			Filename: e.Name(),
			Title:    strings.TrimSpace(title),
			Content:  content,
			HTML:     grammar.RenderMarkdown(content),
		})
	}
	return notes
}

var nonAlphaNumRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slugify(s string) string {
	lower := strings.ToLower(s)
	clean := nonAlphaNumRegex.ReplaceAllString(lower, "-")
	return strings.Trim(clean, "-")
}
