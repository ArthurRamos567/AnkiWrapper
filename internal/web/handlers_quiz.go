package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"anki-wrapper/internal/domain"
	"anki-wrapper/internal/quiz"
)

func (s *Server) handleQuizPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	history, _ := s.quizRepo.GetRecentQuizzes(ctx, 10)
	stats, _ := s.quizRepo.GetQuizStats(ctx)
	vocab := s.getVocabSnapshot()

	var sessions []domain.ChatSession
	if s.chatRepo != nil {
		sessions, _ = s.chatRepo.GetSessions(ctx)
	}

	topicPrefill := r.URL.Query().Get("topic")
	attachSessionID := r.URL.Query().Get("attach_session")
	if attachSessionID != "" && topicPrefill == "" {
		for _, sess := range sessions {
			if sess.ID == attachSessionID {
				topicPrefill = sess.Title
				break
			}
		}
	}

	data := map[string]any{
		"ActiveNav":         "quiz",
		"VocabCount":        len(vocab),
		"SelectedMode":      "multiple_choice",
		"SelectedTopic":     topicPrefill,
		"ActiveQuestion":    nil,
		"RecentQuizzes":     history,
		"Stats":             stats,
		"ChatSessions":      sessions,
		"AttachedSessionID": attachSessionID,
	}

	if err := s.tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleQuizGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	_ = r.ParseForm()
	modes := r.Form["modes"]
	if len(modes) == 0 {
		if m := r.FormValue("mode"); m != "" {
			modes = []string{m}
		} else {
			modes = []string{"multiple_choice", "cloze", "free"}
		}
	}

	topic := r.FormValue("topic")
	chatSessionID := r.FormValue("chat_session_id")
	var chatContext string
	if chatSessionID != "" && s.chatRepo != nil {
		msgs, err := s.chatRepo.GetChatMessages(r.Context(), chatSessionID)
		if err == nil && len(msgs) > 0 {
			var sb strings.Builder
			start := 0
			if len(msgs) > 10 {
				start = len(msgs) - 10
			}
			for _, m := range msgs[start:] {
				role := "Estudante"
				if m.Role == "assistant" {
					role = "Tutor"
				}
				sb.WriteString(fmt.Sprintf("%s: %s\n", role, m.Content))
			}
			chatContext = sb.String()
		}
	}

	if strings.TrimSpace(topic) == "" && chatContext != "" {
		topic = "Conversa Anexada do Chat"
	}

	count := 1
	if cStr := r.FormValue("count"); cStr != "" {
		if c, err := strconv.Atoi(cStr); err == nil && c > 0 {
			count = c
		}
	}
	if count > 50 {
		count = 50
	}

	qs, err := s.quizEngine.GenerateBatch(r.Context(), modes, topic, count, chatContext)
	if err != nil {
		http.Error(w, fmt.Sprintf("Erro ao gerar questão: %v", err), http.StatusInternalServerError)
		return
	}

	s.mu.Lock()
	for _, q := range qs {
		s.activeQuestions[q.ID] = q
	}
	s.mu.Unlock()

	if err := s.tmpl.ExecuteTemplate(w, "question_list", qs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleQuizAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	qID := r.FormValue("question_id")
	userAnswer := r.FormValue("user_answer")

	s.mu.RLock()
	q, ok := s.activeQuestions[qID]
	s.mu.RUnlock()

	if !ok {
		q = &quiz.Question{
			ID:              qID,
			Mode:            r.FormValue("mode"),
			Topic:           r.FormValue("topic"),
			PromptText:      r.FormValue("prompt_text"),
			CorrectAnswer:   r.FormValue("correct_answer"),
			ReferenceAnswer: r.FormValue("reference_answer"),
			Explanation:     r.FormValue("explanation"),
		}
	}

	if q.GrammarRef == nil && r.FormValue("grammar_title") != "" {
		q.GrammarRef = &domain.GrammarReference{
			ID:         r.FormValue("grammar_id"),
			Title:      r.FormValue("grammar_title"),
			SectionKey: r.FormValue("grammar_section_key"),
			Chapter:    r.FormValue("grammar_chapter"),
			URL:        r.FormValue("grammar_url"),
			WebURL:     r.FormValue("grammar_web_url"),
		}
	}

	eval, err := s.quizEngine.EvaluateAnswer(r.Context(), q, userAnswer)
	if err != nil {
		http.Error(w, fmt.Sprintf("Erro ao avaliar resposta: %v", err), http.StatusInternalServerError)
		return
	}

	// Persist in repository
	if s.quizRepo != nil {
		_ = s.quizRepo.SaveQuizRecord(r.Context(), domain.QuizRecord{
			Mode:          q.Mode,
			Topic:         q.Topic,
			Question:      q.PromptText,
			UserAnswer:    userAnswer,
			CorrectAnswer: eval.CorrectAnswer,
			IsCorrect:     eval.IsCorrect,
			Explanation:   eval.Explanation,
		})
	}

	if err := s.tmpl.ExecuteTemplate(w, "answer_result", eval); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
