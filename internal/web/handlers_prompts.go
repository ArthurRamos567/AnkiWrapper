package web

import (
	"fmt"
	"net/http"

	"anki-wrapper/internal/anki"
	"anki-wrapper/internal/chat"
)

func (s *Server) handlePromptsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vocab := s.getVocabSnapshot()
	promptsList, _ := s.prompts.GetAllPrompts(ctx)
	lang := s.prompts.GetSetting(ctx, "support_lang", "pt")

	s.mu.RLock()
	currentAnkiPath := s.ankiPath
	s.mu.RUnlock()

	data := map[string]any{
		"ActiveNav":   "prompts",
		"VocabCount":  len(vocab),
		"Prompts":     promptsList,
		"SupportLang": lang,
		"AnkiPath":    currentAnkiPath,
	}

	if err := s.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handlePromptUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	key := r.FormValue("key")
	content := r.FormValue("content")

	if err := s.prompts.UpdatePrompt(r.Context(), key, content); err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<span class="text-rose-400">❌ Erro: %v</span>`, err)))
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<span class="text-emerald-400 animate-fade">✅ Salvo com sucesso!</span>`))
}

func (s *Server) handleSettingUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	key := r.FormValue("key")
	val := r.FormValue("value")

	_ = s.prompts.SetSetting(r.Context(), key, val)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<span class="text-emerald-400">Idioma atualizado!</span>`))
}

func (s *Server) handleAnkiReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	path := s.ankiPath
	s.mu.RUnlock()

	items, err := anki.ReadAnkiVocab(path)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`<span class="text-rose-400">Falha ao recarregar: %v</span>`, err)))
		return
	}

	s.mu.Lock()
	s.vocab = items
	s.mu.Unlock()

	chat.SetKnownVocab(items)

	// Incremental RAG Vector Sync via repository interface
	if s.vectorRepo != nil {
		_, _ = s.vectorRepo.SyncVocab(r.Context(), items)
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(fmt.Sprintf(`<span class="text-emerald-400">Recarregado: %d palavras ativas no RAG!</span>`, len(items))))
}
