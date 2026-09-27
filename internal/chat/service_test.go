package chat

import (
	"context"
	"encoding/json"
	"testing"

	"anki-wrapper/internal/domain"
)

// MockLLMProvider implements domain.LLMProvider for testing
type MockLLMProvider struct {
	replyJapanese   string
	translation     string
	pedagogicalNote string
}

func (m *MockLLMProvider) ExecutePrompt(ctx context.Context, prompt string) (string, error) {
	return m.replyJapanese, nil
}

func (m *MockLLMProvider) ExecuteStructured(ctx context.Context, prompt string, schema string, target any) error {
	resp := map[string]string{
		"reply_japanese":   m.replyJapanese,
		"translation":     m.translation,
		"pedagogical_note": m.pedagogicalNote,
	}
	bytes, _ := json.Marshal(resp)
	return json.Unmarshal(bytes, target)
}

// MockChatRepository implements domain.ChatRepository for testing
type MockChatRepository struct {
	sessions []domain.ChatSession
	messages []domain.ChatMessage
}

func (m *MockChatRepository) CreateSession(ctx context.Context, id, title string) error {
	m.sessions = append(m.sessions, domain.ChatSession{ID: id, Title: title})
	return nil
}

func (m *MockChatRepository) GetSessions(ctx context.Context) ([]domain.ChatSession, error) {
	return m.sessions, nil
}

func (m *MockChatRepository) UpdateSessionTitle(ctx context.Context, id, title string) error {
	for i := range m.sessions {
		if m.sessions[i].ID == id {
			m.sessions[i].Title = title
			break
		}
	}
	return nil
}

func (m *MockChatRepository) DeleteSession(ctx context.Context, id string) error {
	var filtered []domain.ChatSession
	for _, s := range m.sessions {
		if s.ID != id {
			filtered = append(filtered, s)
		}
	}
	m.sessions = filtered
	return nil
}

func (m *MockChatRepository) SaveChatMessage(ctx context.Context, msg domain.ChatMessage) error {
	m.messages = append(m.messages, msg)
	return nil
}

func (m *MockChatRepository) GetChatMessages(ctx context.Context, sessionID string) ([]domain.ChatMessage, error) {
	var res []domain.ChatMessage
	for _, msg := range m.messages {
		if msg.SessionID == sessionID {
			res = append(res, msg)
		}
	}
	return res, nil
}

func (m *MockChatRepository) ClearSessionMessages(ctx context.Context, sessionID string) error {
	var res []domain.ChatMessage
	for _, msg := range m.messages {
		if msg.SessionID != sessionID {
			res = append(res, msg)
		}
	}
	m.messages = res
	return nil
}

// MockPromptRepository implements domain.PromptRepository
type MockPromptRepository struct {
	prompts  map[string]string
	settings map[string]string
}

func (m *MockPromptRepository) GetAllPrompts(ctx context.Context) ([]domain.PromptItem, error) {
	var res []domain.PromptItem
	for k, v := range m.prompts {
		res = append(res, domain.PromptItem{Key: k, Content: v})
	}
	return res, nil
}

func (m *MockPromptRepository) GetPrompt(ctx context.Context, key string) (string, error) {
	return m.prompts[key], nil
}

func (m *MockPromptRepository) UpdatePrompt(ctx context.Context, key, content string) error {
	m.prompts[key] = content
	return nil
}

func (m *MockPromptRepository) GetSetting(ctx context.Context, key, defaultValue string) string {
	if val, ok := m.settings[key]; ok {
		return val
	}
	return defaultValue
}

func (m *MockPromptRepository) SetSetting(ctx context.Context, key, value string) error {
	m.settings[key] = value
	return nil
}

func TestChatServiceReply(t *testing.T) {
	mockLLM := &MockLLMProvider{
		replyJapanese:   "こんにちは！元気[げんき]ですか？",
		translation:     "Olá! Como vai?",
		pedagogicalNote: "Lembre-se da entonação interrogativa com か.",
	}

	mockPrompts := &MockPromptRepository{
		prompts: map[string]string{
			"chat_system":      "Você é um tutor de japonês.",
			"soft_n_plus_one": "Use vocabulário natural.",
		},
		settings: map[string]string{
			"support_lang": "pt",
		},
	}

	mockChatRepo := &MockChatRepository{}

	svc := NewService(mockLLM, mockPrompts, nil, mockChatRepo, nil)

	resp, err := svc.Reply(context.Background(), "session_1", "Olá, tudo bem?")
	if err != nil {
		t.Fatalf("erro inesperado no Reply: %v", err)
	}

	if resp.Content != "こんにちは！元気[げんき]ですか？" {
		t.Errorf("conteúdo inesperado: %s", resp.Content)
	}

	// Verify furigana HTML conversion
	if resp.FuriganaHTML == "" || resp.FuriganaHTML == resp.Content {
		t.Errorf("furigana HTML não gerado corretamente: %s", resp.FuriganaHTML)
	}

	// Verify messages saved
	msgs, _ := mockChatRepo.GetChatMessages(context.Background(), "session_1")
	if len(msgs) != 2 {
		t.Fatalf("esperado 2 mensagens salvas (user e assistant), obtido %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Errorf("ordem das mensagens incorreta: %s, %s", msgs[0].Role, msgs[1].Role)
	}
}

func TestChatServiceReplyModes(t *testing.T) {
	mockLLM := &MockLLMProvider{
		replyJapanese: "A formação dos kanjis possui 4 padrões principais.",
	}
	mockPrompts := &MockPromptRepository{
		prompts: map[string]string{
			"chat_system": "Você é tutor.",
		},
		settings: map[string]string{
			"support_lang": "pt",
		},
	}
	mockChatRepo := &MockChatRepository{}
	svc := NewService(mockLLM, mockPrompts, nil, mockChatRepo, nil)

	// Mode PT
	resp, err := svc.Reply(context.Background(), "sess_pt", "explique os kanjis", "pt")
	if err != nil {
		t.Fatalf("erro no Reply pt: %v", err)
	}
	if resp.Content != "A formação dos kanjis possui 4 padrões principais." {
		t.Errorf("conteúdo inesperado no modo PT: %s", resp.Content)
	}
}
