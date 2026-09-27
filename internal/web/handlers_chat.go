package web

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"anki-wrapper/internal/chat"
	"anki-wrapper/internal/domain"
)

func (s *Server) handleChatPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vocab := s.getVocabSnapshot()
	sessions, _ := s.chatRepo.GetSessions(ctx)

	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		if len(sessions) > 0 {
			sessionID = sessions[0].ID
		} else {
			sessionID = fmt.Sprintf("sess_%d", time.Now().Unix())
			_ = s.chatRepo.CreateSession(ctx, sessionID, "Nova Conversa")
			sessions, _ = s.chatRepo.GetSessions(ctx)
		}
	}

	sessionTitle := "Conversa"
	for _, sess := range sessions {
		if sess.ID == sessionID {
			sessionTitle = sess.Title
			break
		}
	}

	messages, _ := s.chatRepo.GetChatMessages(ctx, sessionID)
	for i := range messages {
		if messages[i].Role == "assistant" && messages[i].Content != "" {
			messages[i].FuriganaHTML = chat.FuriganaToHTML(messages[i].Content)
		}
	}

	data := map[string]any{
		"ActiveNav":           "chat",
		"VocabCount":          len(vocab),
		"Messages":            messages,
		"Sessions":            sessions,
		"CurrentSessionID":    sessionID,
		"CurrentSessionTitle": sessionTitle,
	}

	if err := s.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleChatSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	msg := r.FormValue("message")
	sessionID := r.FormValue("session_id")
	chatMode := r.FormValue("chat_mode")
	if sessionID == "" {
		sessionID = "default"
	}
	if msg == "" {
		return
	}

	userMsg := domain.ChatMessage{Role: "user", Content: msg}

	// Render user message bubble immediately
	_ = s.tmpl.ExecuteTemplate(w, "chat_bubble_user", userMsg)

	// Call AI
	assistantMsg, err := s.chatService.Reply(r.Context(), sessionID, msg, chatMode)
	if err != nil {
		errBubble := domain.ChatMessage{
			Role:         "assistant",
			Content:      fmt.Sprintf("Ocorreu um erro ao processar: %v", err),
			FuriganaHTML: template.HTMLEscapeString(fmt.Sprintf("Erro: %v", err)),
		}
		_ = s.tmpl.ExecuteTemplate(w, "chat_bubble_assistant", errBubble)
		return
	}

	// Render assistant response bubble
	_ = s.tmpl.ExecuteTemplate(w, "chat_bubble_assistant", assistantMsg)
}

func (s *Server) handleChatNewSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	newID := fmt.Sprintf("sess_%d", time.Now().Unix())
	_ = s.chatRepo.CreateSession(r.Context(), newID, "Nova Conversa")
	http.Redirect(w, r, fmt.Sprintf("/chat?session_id=%s", newID), http.StatusSeeOther)
}

func (s *Server) handleChatDeleteSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.FormValue("session_id")
	if sessionID != "" {
		_ = s.chatRepo.DeleteSession(r.Context(), sessionID)
	}
	http.Redirect(w, r, "/chat", http.StatusSeeOther)
}

func (s *Server) handleChatClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.FormValue("session_id")
	if sessionID == "" {
		sessionID = "default"
	}

	_ = s.chatRepo.ClearSessionMessages(r.Context(), sessionID)
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`
		<div id="chat-empty" class="text-center py-16 text-slate-500 space-y-2">
			<span class="text-4xl block">🇯🇵</span>
			<p class="text-sm font-medium text-slate-400">Conversa reiniciada. Diga "こんにちは！" para começar!</p>
		</div>
	`))
}
