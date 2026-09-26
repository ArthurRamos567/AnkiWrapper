package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"anki-wrapper/internal/domain"
)

func (s *Server) handleDecksPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vocab := s.getVocabSnapshot()

	if s.vectorRepo == nil {
		http.Error(w, "Repositório de vocabulário não disponível", http.StatusServiceUnavailable)
		return
	}

	decks := s.vectorRepo.GetDecks(ctx)

	selectedDeck := strings.TrimSpace(r.URL.Query().Get("deck"))
	if selectedDeck == "" && len(decks) > 0 {
		selectedDeck = decks[0].Name
	}

	filter := strings.TrimSpace(r.URL.Query().Get("filter"))
	if filter == "" {
		filter = "all"
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	lowerQ := strings.ToLower(q)

	allDeckItems := s.vectorRepo.GetVocabByDeck(ctx, selectedDeck)

	// Deck-specific counters
	deckTotal := len(allDeckItems)
	deckRead := 0
	for _, item := range allDeckItems {
		if item.IsRead {
			deckRead++
		}
	}
	deckUnread := deckTotal - deckRead
	deckPercentage := 0
	if deckTotal > 0 {
		deckPercentage = int(float64(deckRead) / float64(deckTotal) * 100)
	}

	// Filter items by status and search query
	var displayedItems []domain.VocabItem
	for _, item := range allDeckItems {
		if filter == "read" && !item.IsRead {
			continue
		}
		if filter == "unread" && item.IsRead {
			continue
		}

		if lowerQ != "" {
			match := strings.Contains(strings.ToLower(item.Word), lowerQ) ||
				strings.Contains(strings.ToLower(item.Reading), lowerQ) ||
				strings.Contains(strings.ToLower(item.Meaning), lowerQ) ||
				strings.Contains(strings.ToLower(item.Sentence), lowerQ) ||
				strings.Contains(strings.ToLower(item.Tags), lowerQ)
			if !match {
				continue
			}
		}

		displayedItems = append(displayedItems, item)
	}

	// Global counters
	globalTotal := s.vectorRepo.Count(ctx)
	globalRead := s.vectorRepo.CountRead(ctx)
	globalUnread := globalTotal - globalRead
	globalPercentage := 0
	if globalTotal > 0 {
		globalPercentage = int(float64(globalRead) / float64(globalTotal) * 100)
	}

	data := map[string]any{
		"ActiveNav":        "decks",
		"VocabCount":       len(vocab),
		"Decks":            decks,
		"SelectedDeck":     selectedDeck,
		"CurrentFilter":    filter,
		"SearchQuery":      q,
		"Items":            displayedItems,
		"DisplayedCount":   len(displayedItems),
		"DeckTotal":        deckTotal,
		"DeckRead":         deckRead,
		"DeckUnread":       deckUnread,
		"DeckPercentage":   deckPercentage,
		"GlobalTotal":      globalTotal,
		"GlobalRead":       globalRead,
		"GlobalUnread":     globalUnread,
		"GlobalPercentage": globalPercentage,
	}

	if err := s.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleVocabToggleRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimSpace(r.FormValue("id"))
	if idStr == "" {
		idStr = strings.TrimSpace(r.URL.Query().Get("id"))
	}
	if idStr == "" {
		http.Error(w, "ID do vocabulário obrigatório", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID inválido", http.StatusBadRequest)
		return
	}

	if s.vectorRepo == nil {
		http.Error(w, "Repositório não disponível", http.StatusServiceUnavailable)
		return
	}

	// Find current state
	allVocab := s.vectorRepo.GetAllVocab(r.Context())
	var currentItem *domain.VocabItem
	for _, it := range allVocab {
		if it.ID == id {
			currentItem = &it
			break
		}
	}

	var newReadState bool
	explicitVal := r.FormValue("read")
	if explicitVal != "" {
		newReadState = (explicitVal == "1" || explicitVal == "true")
	} else if currentItem != nil {
		newReadState = !currentItem.IsRead
	} else {
		newReadState = true
	}

	if err := s.vectorRepo.MarkVocabRead(r.Context(), id, newReadState); err != nil {
		http.Error(w, fmt.Sprintf("Erro ao atualizar status: %v", err), http.StatusInternalServerError)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if newReadState {
			fmt.Fprintf(w, `<button type="button" hx-post="/vocab/toggle-read?id=%d" hx-target="this" hx-swap="outerHTML" style="cursor: pointer; font-size: 0.8rem; padding: 2px 6px; border: 1px solid #008800; background: #eafbe8; color: #006600; border-radius: 3px;" title="Marcar como não lido">✓ Lido</button>`, id)
		} else {
			fmt.Fprintf(w, `<button type="button" hx-post="/vocab/toggle-read?id=%d" hx-target="this" hx-swap="outerHTML" style="cursor: pointer; font-size: 0.8rem; padding: 2px 6px; border: 1px solid #ccc; background: #fafafa; color: #666; border-radius: 3px;" title="Marcar como lido">○ Não lido</button>`, id)
		}
		return
	}

	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = "/decks"
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}

func (s *Server) handleDeckToggleRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	deck := strings.TrimSpace(r.FormValue("deck"))
	if deck == "" {
		deck = strings.TrimSpace(r.URL.Query().Get("deck"))
	}

	readVal := r.FormValue("read")
	if readVal == "" {
		readVal = r.URL.Query().Get("read")
	}
	isRead := (readVal == "1" || readVal == "true")

	if s.vectorRepo != nil {
		_ = s.vectorRepo.MarkDeckRead(r.Context(), deck, isRead)
	}

	ref := r.Header.Get("Referer")
	if ref == "" {
		ref = fmt.Sprintf("/decks?deck=%s", deck)
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}
