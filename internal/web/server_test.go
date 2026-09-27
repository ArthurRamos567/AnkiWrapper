package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"anki-wrapper/internal/chat"
	"anki-wrapper/internal/domain"
	"anki-wrapper/internal/quiz"
)

// MockLLM for web tests
type MockWebLLM struct{}

func (m *MockWebLLM) ExecutePrompt(ctx context.Context, prompt string) (string, error) {
	return "ok", nil
}

func (m *MockWebLLM) ExecuteStructured(ctx context.Context, prompt string, schema string, target any) error {
	if strings.Contains(prompt, "Gere exatamente") {
		raw := map[string]any{
			"questions": []map[string]any{
				{
					"mode":           "multiple_choice",
					"prompt_text":    "Qual partícula usar?",
					"sentence":       "日本[にほん]____行[い]く。",
					"options":        []string{"に", "で", "を", "が"},
					"correct_answer": "に",
					"hint":           "Destino",
					"explanation":    "Partícula に indica destino.",
				},
				{
					"mode":           "cloze",
					"prompt_text":    "Complete a lacuna:",
					"sentence":       "ご飯[はん]を____。",
					"correct_answer": "食[た]べる",
					"hint":           "Verbo comer",
					"explanation":    "Taberu significa comer.",
				},
			},
		}
		b, _ := json.Marshal(raw)
		return json.Unmarshal(b, target)
	} else if strings.Contains(prompt, "Modo do Quiz:") {
		raw := map[string]any{
			"prompt_text":    "Qual partícula usar?",
			"sentence":       "日本[にほん]____行[い]く。",
			"options":        []string{"に", "で", "を", "が"},
			"correct_option": "に",
			"hint":           "Destino",
			"explanation":    "Partícula に indica destino.",
		}
		b, _ := json.Marshal(raw)
		return json.Unmarshal(b, target)
	} else if strings.Contains(prompt, "Você é um avaliador pedagógico") {
		raw := map[string]any{
			"is_correct":         true,
			"score":              100,
			"corrected_sentence": "日本[にほん]に行[い]く。",
			"explanation":        "Correto!",
			"feedback":           "Muito bem!",
		}
		b, _ := json.Marshal(raw)
		return json.Unmarshal(b, target)
	}
	return nil
}

// MockPromptRepo for web tests
type MockWebPromptRepo struct {
	settings map[string]string
	prompts  map[string]string
}

func (m *MockWebPromptRepo) GetAllPrompts(ctx context.Context) ([]domain.PromptItem, error) {
	return []domain.PromptItem{
		{Key: "chat_system", Title: "Chat", Content: "Você é tutor."},
	}, nil
}
func (m *MockWebPromptRepo) GetPrompt(ctx context.Context, key string) (string, error) {
	return m.prompts[key], nil
}
func (m *MockWebPromptRepo) UpdatePrompt(ctx context.Context, key, content string) error {
	if m.prompts == nil {
		m.prompts = make(map[string]string)
	}
	m.prompts[key] = content
	return nil
}
func (m *MockWebPromptRepo) GetSetting(ctx context.Context, key, def string) string {
	if v, ok := m.settings[key]; ok {
		return v
	}
	return def
}
func (m *MockWebPromptRepo) SetSetting(ctx context.Context, key, val string) error {
	if m.settings == nil {
		m.settings = make(map[string]string)
	}
	m.settings[key] = val
	return nil
}

// MockQuizRepo for web tests
type MockWebQuizRepo struct{}

func (m *MockWebQuizRepo) SaveQuizRecord(ctx context.Context, record domain.QuizRecord) error {
	return nil
}
func (m *MockWebQuizRepo) GetQuizStats(ctx context.Context) (*domain.QuizStats, error) {
	return &domain.QuizStats{Total: 10, Correct: 8, Incorrect: 2, Accuracy: 80.0}, nil
}
func (m *MockWebQuizRepo) GetRecentQuizzes(ctx context.Context, limit int) ([]domain.QuizRecord, error) {
	return nil, nil
}

// MockChatRepo for web tests
type MockWebChatRepo struct {
	sessions []domain.ChatSession
	messages map[string][]domain.ChatMessage
}

func (m *MockWebChatRepo) CreateSession(ctx context.Context, id, title string) error {
	m.sessions = append(m.sessions, domain.ChatSession{ID: id, Title: title})
	return nil
}
func (m *MockWebChatRepo) GetSessions(ctx context.Context) ([]domain.ChatSession, error) {
	return m.sessions, nil
}
func (m *MockWebChatRepo) UpdateSessionTitle(ctx context.Context, id, title string) error {
	return nil
}
func (m *MockWebChatRepo) DeleteSession(ctx context.Context, id string) error {
	return nil
}
func (m *MockWebChatRepo) SaveChatMessage(ctx context.Context, msg domain.ChatMessage) error {
	return nil
}
func (m *MockWebChatRepo) GetChatMessages(ctx context.Context, sessionID string) ([]domain.ChatMessage, error) {
	if m.messages != nil {
		return m.messages[sessionID], nil
	}
	return nil, nil
}
func (m *MockWebChatRepo) ClearSessionMessages(ctx context.Context, sessionID string) error {
	return nil
}

// MockTelemetryRepo for web tests
type MockWebTelemetryRepo struct {
	cleared bool
}

func (m *MockWebTelemetryRepo) RecordUsage(ctx context.Context, rec domain.LLMUsageRecord) error {
	return nil
}

func (m *MockWebTelemetryRepo) GetUsageStats(ctx context.Context) (*domain.LLMUsageStats, error) {
	return &domain.LLMUsageStats{
		TotalCalls:        5,
		TotalTokens:       1200,
		AvgDurationMs:     450,
		CallsByProvider:   map[string]int{"gemini": 5},
		CallsByOperation:  map[string]int{"chat_reply": 5},
		TokensByOperation: map[string]int64{"chat_reply": 1200},
	}, nil
}

func (m *MockWebTelemetryRepo) GetRecentUsage(ctx context.Context, limit int) ([]domain.LLMUsageRecord, error) {
	return []domain.LLMUsageRecord{
		{
			Provider:    "gemini",
			Model:       "gemini-2.5-flash",
			Operation:   "chat_reply",
			TotalTokens: 240,
			DurationMs:  400,
			Status:      "SUCCESS",
		},
	}, nil
}

func (m *MockWebTelemetryRepo) ClearUsage(ctx context.Context) error {
	m.cleared = true
	return nil
}

// MockGrammarRepo for web tests
type MockWebGrammarRepo struct {
	readMap map[string]bool
}

func (m *MockWebGrammarRepo) SyncLessons(ctx context.Context, lessons []domain.GrammarLesson) (int, error) {
	return len(lessons), nil
}
func (m *MockWebGrammarRepo) SearchLessons(ctx context.Context, query string, limit int) ([]domain.GrammarLesson, error) {
	return m.GetAllLessons(ctx)
}
func (m *MockWebGrammarRepo) GetLessonByID(ctx context.Context, id string) (*domain.GrammarLesson, error) {
	lessons, _ := m.GetAllLessons(ctx)
	for _, l := range lessons {
		if l.ID == id {
			return &l, nil
		}
	}
	return nil, nil
}
func (m *MockWebGrammarRepo) GetLessonBySection(ctx context.Context, sectionKey string) (*domain.GrammarLesson, error) {
	lessons, _ := m.GetAllLessons(ctx)
	for _, l := range lessons {
		if l.SectionKey == sectionKey {
			return &l, nil
		}
	}
	return nil, nil
}
func (m *MockWebGrammarRepo) GetAllLessons(ctx context.Context) ([]domain.GrammarLesson, error) {
	ref := &domain.GrammarReference{
		ID:         "basics/noun-particles",
		Title:      "Noun Particles",
		SectionKey: "3.11",
		Chapter:    "Basic Grammar",
		URL:        "https://guidetojapanese.org/learn/grammar/nounparticles",
		WebURL:     "https://jp.saeris.gg/learn/grammar/basics/noun-particles",
	}
	isRead := false
	if m.readMap != nil {
		isRead = m.readMap["basics/noun-particles"]
	}
	return []domain.GrammarLesson{
		{
			ID:            "basics/noun-particles",
			Title:         "Noun Particles",
			ChapterNumber: 3,
			Chapter:       "Basic Grammar",
			Part:          11,
			SectionKey:    "3.11",
			Slug:          "noun-particles",
			URL:           "https://guidetojapanese.org/learn/grammar/nounparticles",
			WebURL:        "https://jp.saeris.gg/learn/grammar/basics/noun-particles",
			Content:       "## The Particles\n\nExplanation of と particle.",
			IsRead:        isRead,
			Reference:     ref,
		},
	}, nil
}
func (m *MockWebGrammarRepo) Count(ctx context.Context) int {
	return 1
}
func (m *MockWebGrammarRepo) MarkLessonRead(ctx context.Context, id string, isRead bool) error {
	if m.readMap == nil {
		m.readMap = make(map[string]bool)
	}
	m.readMap[id] = isRead
	return nil
}
func (m *MockWebGrammarRepo) GetReadLessons(ctx context.Context) ([]domain.GrammarLesson, error) {
	all, _ := m.GetAllLessons(ctx)
	var res []domain.GrammarLesson
	for _, l := range all {
		if l.IsRead {
			res = append(res, l)
		}
	}
	return res, nil
}
func (m *MockWebGrammarRepo) SearchReadLessons(ctx context.Context, query string, topK int) ([]domain.GrammarLesson, error) {
	return m.GetReadLessons(ctx)
}
func (m *MockWebGrammarRepo) CountRead(ctx context.Context) int {
	read, _ := m.GetReadLessons(ctx)
	return len(read)
}

// MockVectorRepo for web tests
type MockWebVectorRepo struct {
	items   []domain.VocabItem
	readMap map[int64]bool
}

func (m *MockWebVectorRepo) SyncVocab(ctx context.Context, items []domain.VocabItem) (int, error) {
	m.items = items
	return len(items), nil
}
func (m *MockWebVectorRepo) SearchTopK(ctx context.Context, query string, topK int, threshold float32) []domain.ScoredVocab {
	return nil
}
func (m *MockWebVectorRepo) SearchReadTopK(ctx context.Context, query string, topK int, threshold float32) []domain.ScoredVocab {
	return nil
}
func (m *MockWebVectorRepo) Count(ctx context.Context) int {
	return len(m.items)
}
func (m *MockWebVectorRepo) CountRead(ctx context.Context) int {
	read := m.GetReadVocab(ctx)
	return len(read)
}
func (m *MockWebVectorRepo) MarkVocabRead(ctx context.Context, id int64, isRead bool) error {
	if m.readMap == nil {
		m.readMap = make(map[int64]bool)
	}
	m.readMap[id] = isRead
	for i := range m.items {
		if m.items[i].ID == id {
			m.items[i].IsRead = isRead
		}
	}
	return nil
}
func (m *MockWebVectorRepo) MarkDeckRead(ctx context.Context, deckName string, isRead bool) error {
	if m.readMap == nil {
		m.readMap = make(map[int64]bool)
	}
	for i := range m.items {
		if deckName == "" || deckName == "all" || m.items[i].DeckName == deckName {
			m.items[i].IsRead = isRead
			m.readMap[m.items[i].ID] = isRead
		}
	}
	return nil
}
func (m *MockWebVectorRepo) GetDecks(ctx context.Context) []domain.DeckInfo {
	total := len(m.items)
	read := m.CountRead(ctx)
	pct := 0
	if total > 0 {
		pct = int(float64(read) / float64(total) * 100)
	}
	return []domain.DeckInfo{
		{
			ID:          1,
			Name:        "Japanese words",
			TotalVocab:  total,
			ReadVocab:   read,
			UnreadVocab: total - read,
			Percentage:  pct,
		},
	}
}
func (m *MockWebVectorRepo) GetAllVocab(ctx context.Context) []domain.VocabItem {
	return m.items
}
func (m *MockWebVectorRepo) GetVocabByDeck(ctx context.Context, deckName string) []domain.VocabItem {
	var res []domain.VocabItem
	for _, it := range m.items {
		if deckName == "" || deckName == "all" || it.DeckName == deckName {
			res = append(res, it)
		}
	}
	return res
}
func (m *MockWebVectorRepo) GetReadVocab(ctx context.Context) []domain.VocabItem {
	var res []domain.VocabItem
	for _, it := range m.items {
		if it.IsRead {
			res = append(res, it)
		}
	}
	return res
}

func setupTestServer(t *testing.T) (*Server, *MockWebTelemetryRepo) {
	mockLLM := &MockWebLLM{}
	mockPrompts := &MockWebPromptRepo{
		settings: map[string]string{"support_lang": "pt"},
		prompts:  map[string]string{"chat_system": "Prompt"},
	}
	mockQuizRepo := &MockWebQuizRepo{}
	mockTelemetry := &MockWebTelemetryRepo{}
	mockChatRepo := &MockWebChatRepo{
		sessions: []domain.ChatSession{
			{ID: "sess_1", Title: "Conversa de Partículas"},
		},
		messages: map[string][]domain.ChatMessage{
			"sess_1": {
				{ID: 1, SessionID: "sess_1", Role: "user", Content: "Como usar a partícula に?"},
				{ID: 2, SessionID: "sess_1", Role: "assistant", Content: "A partícula に indica destino."},
			},
		},
	}
	mockGrammar := &MockWebGrammarRepo{}
	mockVector := &MockWebVectorRepo{
		items: []domain.VocabItem{
			{ID: 1, DeckID: 1, DeckName: "Japanese words", Word: "日本", Reading: "にほん", Meaning: "Japão"},
		},
	}

	quizEng := quiz.NewEngine(mockLLM, mockPrompts, mockVector, mockQuizRepo, mockGrammar)
	chatSvc := chat.NewService(mockLLM, mockPrompts, mockVector, mockChatRepo, mockGrammar)

	srv, err := NewServer(Config{
		QuizEngine:  quizEng,
		ChatService: chatSvc,
		Prompts:     mockPrompts,
		QuizRepo:    mockQuizRepo,
		ChatRepo:    mockChatRepo,
		Telemetry:   mockTelemetry,
		GrammarRepo: mockGrammar,
		VectorRepo:  mockVector,
		Vocab:       []domain.VocabItem{{ID: 1, DeckID: 1, DeckName: "Japanese words", Word: "日本", Reading: "にほん", Meaning: "Japão"}},
		AnkiPath:    "dummy/path",
	})
	if err != nil {
		t.Fatalf("falha ao inicializar Web Server: %v", err)
	}

	return srv, mockTelemetry
}

func TestWebRoutes(t *testing.T) {
	srv, mockTelem := setupTestServer(t)
	handler := srv.Routes()

	// 1. Root redirect to /quiz
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("esperado status 303 no root, obtido %d", rec.Code)
	}

	// 2. Quiz Page
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/quiz", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /quiz, obtido %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="modes"`) || !strings.Contains(body, `type="checkbox"`) {
		t.Errorf("página do quiz deve conter os checkboxes de categorias/modos com name=modes")
	}
	if !strings.Contains(body, `name="count"`) || !strings.Contains(body, `id="count-input"`) {
		t.Errorf("página do quiz deve conter o input de quantidade com name=count e id=count-input")
	}
	if !strings.Contains(body, `max="50"`) {
		t.Errorf("input de quantidade deve ter max=50")
	}
	if !strings.Contains(body, "global-error-banner") {
		t.Errorf("página deve conter o banner global de erros global-error-banner")
	}
	if !strings.Contains(body, "htmx:responseError") {
		t.Errorf("página deve registrar o listener de htmx:responseError para visibilidade de falhas na UI")
	}
	if !strings.Contains(body, "new-vocab-toggle-btn") {
		t.Errorf("página do quiz deve conter o botão de alternância de novas palavras")
	}
	if !strings.Contains(body, "btn-toggle-attach") {
		t.Errorf("página do quiz deve conter o botão de anexar conversa btn-toggle-attach")
	}
	if !strings.Contains(body, "attach-chat-box") {
		t.Errorf("página do quiz deve conter a caixa attach-chat-box")
	}
	if !strings.Contains(body, "Conversa de Partículas") {
		t.Errorf("página do quiz deve listar sessões de chat disponíveis para anexar")
	}

	// 2b. Quiz Page with attach_session query param
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/quiz?attach_session=sess_1", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /quiz?attach_session=sess_1, obtido %d", rec.Code)
	}
	attachedBody := rec.Body.String()
	if !strings.Contains(attachedBody, `value="Conversa de Partículas"`) {
		t.Errorf("tópico deve vir pré-preenchido com o título da sessão anexada")
	}
	if !strings.Contains(attachedBody, `value="sess_1" selected`) {
		t.Errorf("sessão de chat deve vir selecionada no dropdown de anexo")
	}

	// 3. Chat Page
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/chat", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /chat, obtido %d", rec.Code)
	}
	chatBody := rec.Body.String()
	if !strings.Contains(chatBody, "chat-mode-select") {
		t.Errorf("página do chat deve conter o seletor de modo de resposta chat-mode-select")
	}
	if !strings.Contains(chatBody, `name="chat_mode"`) {
		t.Errorf("formulário do chat deve conter o input hidden com name=chat_mode")
	}
	if !strings.Contains(chatBody, `<textarea id="chat-input"`) {
		t.Errorf("página do chat deve usar textarea expandível id=chat-input em vez de input comum")
	}
	if strings.Contains(chatBody, `<textarea id="chat-input" name="message" placeholder=`) {
		t.Errorf("campo de texto do chat não deve conter preview/placeholder")
	}
	if !strings.Contains(chatBody, "/quiz?attach_session=sess_1") {
		t.Errorf("página do chat deve conter link para gerar quiz a partir da sessão ativa")
	}

	// 4. Prompts Page
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/prompts", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /prompts, obtido %d", rec.Code)
	}

	// 5. Update prompt setting
	formData := url.Values{
		"key":   {"support_lang"},
		"value": {"en"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/prompts/setting", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no update de setting, obtido %d", rec.Code)
	}

	// 6. Telemetry Page
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/telemetry", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /telemetry, obtido %d", rec.Code)
	}
	telemBody := rec.Body.String()
	if !strings.Contains(telemBody, "Telemetria de Tokens &amp; Uso de LLM") && !strings.Contains(telemBody, "Telemetria") {
		t.Errorf("página de telemetria deve conter o título ou menção de telemetria")
	}
	if !strings.Contains(telemBody, "1200") {
		t.Errorf("página de telemetria deve exibir contagem de tokens (1200)")
	}

	// 7. Clear Telemetry
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/telemetry/clear", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("esperado status 303 no /telemetry/clear, obtido %d", rec.Code)
	}
	if !mockTelem.cleared {
		t.Errorf("esperado que mockTelem.ClearUsage tenha sido chamado")
	}

	// 8. Grammar Guide Index Page (defaults to Tae Kim tab)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/grammar", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /grammar, obtido %d", rec.Code)
	}
	grammarBody := rec.Body.String()
	if !strings.Contains(grammarBody, "tab=taekim") || !strings.Contains(grammarBody, "tab=quickref") || !strings.Contains(grammarBody, "tab=notes") {
		t.Errorf("página de gramática deve conter as abas taekim, quickref e notes")
	}
	if !strings.Contains(grammarBody, "Tae Kim's Guide to Japanese Grammar") {
		t.Errorf("aba do Tae Kim deve conter o título do guia")
	}
	if !strings.Contains(grammarBody, "Noun Particles") {
		t.Errorf("aba do Tae Kim deve listar as lições disponíveis")
	}

	// 9. Grammar Quick Reference Tab
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/grammar?tab=quickref", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /grammar?tab=quickref, obtido %d", rec.Code)
	}
	quickrefBody := rec.Body.String()
	if !strings.Contains(quickrefBody, "Referência Rápida de Gramática Japonesa") {
		t.Errorf("aba de referência rápida deve conter o título esperado")
	}
	if !strings.Contains(quickrefBody, "Partículas Mais Utilizadas") {
		t.Errorf("aba de referência rápida deve conter a tabela de partículas")
	}

	// 10. Grammar Notes Tab
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/grammar?tab=notes", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /grammar?tab=notes, obtido %d", rec.Code)
	}
	notesBody := rec.Body.String()
	if !strings.Contains(notesBody, "Anotações Locais") {
		t.Errorf("aba de notas deve conter o título de anotações locais")
	}

	// 11. Grammar Lesson Reader Page
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/grammar/lesson?id=basics/noun-particles", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /grammar/lesson, obtido %d", rec.Code)
	}
	lessonBody := rec.Body.String()
	if !strings.Contains(lessonBody, "Noun Particles") {
		t.Errorf("leitor de lição deve conter o título da lição")
	}
	if !strings.Contains(lessonBody, "guidetojapanese.org") {
		t.Errorf("leitor de lição deve conter link para a fonte oficial")
	}

	// 12. Grammar Lesson Toggle Read Endpoint
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/grammar/lesson/toggle-read?id=basics/noun-particles", nil)
	req.Header.Set("HX-Request", "true")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /grammar/lesson/toggle-read, obtido %d", rec.Code)
	}
	toggleBody := rec.Body.String()
	if !strings.Contains(toggleBody, "Lido") {
		t.Errorf("resposta do toggle deve indicar status lido, obtido: %s", toggleBody)
	}

	// 13. Grammar Filter by Read
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/grammar?tab=taekim&filter=read", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /grammar?filter=read, obtido %d", rec.Code)
	}
	filterReadBody := rec.Body.String()
	if !strings.Contains(filterReadBody, "Noun Particles") {
		t.Errorf("lição lida deve aparecer no filtro read")
	}

	// 14. Grammar Filter by Unread (should NOT contain the read lesson)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/grammar?tab=taekim&filter=unread", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /grammar?filter=unread, obtido %d", rec.Code)
	}
	filterUnreadBody := rec.Body.String()
	if strings.Contains(filterUnreadBody, "Noun Particles") {
		t.Errorf("lição lida NÃO deve aparecer no filtro unread")
	}

	// 15. Decks Page
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/decks", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /decks, obtido %d", rec.Code)
	}
	decksBody := rec.Body.String()
	if !strings.Contains(decksBody, "Decks do Anki") || !strings.Contains(decksBody, "Japanese words") {
		t.Errorf("página de decks deve listar decks e títulos")
	}

	// 16. Toggle Vocab Read Endpoint
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/vocab/toggle-read?id=1", nil)
	req.Header.Set("HX-Request", "true")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /vocab/toggle-read, obtido %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Lido") {
		t.Errorf("resposta deve indicar status lido, obtido %s", rec.Body.String())
	}

	// 17. Decks Filter by Read
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/decks?filter=read", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /decks?filter=read, obtido %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "日本") {
		t.Errorf("palavra lida deve aparecer no filtro read")
	}

	// 18. Toggle Entire Deck Unread
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/decks/toggle-read?deck=Japanese+words&read=0", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Errorf("esperado status 303 no /decks/toggle-read, obtido %d", rec.Code)
	}

	// 19. Verify Deck Filter Read is now empty
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/decks?filter=read", nil)
	handler.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "日本") {
		t.Errorf("palavra desmarcada NÃO deve aparecer no filtro read")
	}

	// 20. Quiz Batch Generate Endpoint
	generateForm := url.Values{
		"count": {"2"},
		"modes": {"multiple_choice", "cloze"},
		"topic": {"Comida"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/quiz/generate", strings.NewReader(generateForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /quiz/generate com lote, obtido %d", rec.Code)
	}
	genBody := rec.Body.String()
	if !strings.Contains(genBody, "#1 de 2") || !strings.Contains(genBody, "#2 de 2") {
		t.Errorf("resposta do /quiz/generate deve conter os marcadores de lote #1 de 2 e #2 de 2, obtido: %s", genBody)
	}
	if !strings.Contains(genBody, "Qual partícula usar?") || !strings.Contains(genBody, "Complete a lacuna:") {
		t.Errorf("resposta do /quiz/generate deve conter as questões geradas, obtido: %s", genBody)
	}

	// 20b. Quiz Generate with attached chat session
	attachForm := url.Values{
		"count":           {"1"},
		"modes":           {"multiple_choice"},
		"chat_session_id": {"sess_1"},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/quiz/generate", strings.NewReader(attachForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /quiz/generate com chat anexado, obtido %d", rec.Code)
	}
	attachGenBody := rec.Body.String()
	if !strings.Contains(attachGenBody, "Qual partícula usar?") {
		t.Errorf("resposta do /quiz/generate com anexo deve gerar questão normalmente, obtido: %s", attachGenBody)
	}

	// 21. Quiz Answer Endpoint
	ansForm := url.Values{
		"question_id":    {"q_sample"},
		"user_answer":    {"に"},
		"mode":           {"multiple_choice"},
		"correct_answer": {"に"},
		"explanation":    {"Partícula に indica destino."},
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/quiz/answer", strings.NewReader(ansForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("esperado status 200 no /quiz/answer, obtido %d", rec.Code)
	}
	ansBody := rec.Body.String()
	if !strings.Contains(ansBody, "Correto") {
		t.Errorf("resposta do /quiz/answer deve conter resultado Correto, obtido: %s", ansBody)
	}
}


