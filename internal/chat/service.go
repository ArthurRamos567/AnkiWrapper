package chat

import (
	"context"
	"fmt"
	"strings"

	"anki-wrapper/internal/domain"
)

const ChatResponseSchema = `{
  "type": "object",
  "properties": {
    "reply_japanese": {"type": "string"},
    "translation": {"type": "string"},
    "pedagogical_note": {"type": "string"}
  },
  "required": ["reply_japanese", "translation", "pedagogical_note"]
}`

// Service manages conversational sessions and pedagogical AI responses with injected dependencies.
type Service struct {
	llm         domain.LLMProvider
	prompts     domain.PromptRepository
	vectorStore domain.VectorRepository
	chatRepo    domain.ChatRepository
	grammarRepo domain.GrammarRepository
}

// NewService creates a new Chat Service with pure dependency injection.
func NewService(
	llm domain.LLMProvider,
	prompts domain.PromptRepository,
	vectorStore domain.VectorRepository,
	chatRepo domain.ChatRepository,
	grammarRepo domain.GrammarRepository,
) *Service {
	return &Service{
		llm:         llm,
		prompts:     prompts,
		vectorStore: vectorStore,
		chatRepo:    chatRepo,
		grammarRepo: grammarRepo,
	}
}

// Reply generates a chat response for the given user message and session.
func (s *Service) Reply(ctx context.Context, sessionID, userMessage string) (*domain.ChatMessage, error) {
	ctx = domain.WithOperation(ctx, "chat_reply")
	// Ensure session exists
	if s.chatRepo != nil {
		_ = s.chatRepo.CreateSession(ctx, sessionID, "Nova Conversa")
	}

	// 1. Save user message
	userMsg := domain.ChatMessage{
		SessionID: sessionID,
		Role:      "user",
		Content:   userMessage,
	}
	if s.chatRepo != nil {
		if err := s.chatRepo.SaveChatMessage(ctx, userMsg); err != nil {
			return nil, fmt.Errorf("falha ao salvar mensagem do usuário: %w", err)
		}
	}

	// 2. Fetch recent conversation history
	var history []domain.ChatMessage
	if s.chatRepo != nil {
		history, _ = s.chatRepo.GetChatMessages(ctx, sessionID)
		if len(history) <= 1 {
			title := userMessage
			if len([]rune(title)) > 25 {
				title = string([]rune(title)[:25]) + "..."
			}
			_ = s.chatRepo.UpdateSessionTitle(ctx, sessionID, title)
		}
	}

	// Build history string
	var histBuilder strings.Builder
	startIdx := 0
	if len(history) > 8 {
		startIdx = len(history) - 8
	}
	for _, m := range history[startIdx:] {
		histBuilder.WriteString(fmt.Sprintf("%s: %s\n", strings.ToUpper(m.Role), m.Content))
	}

	// 3. Prepare prompts
	chatSystemPrompt, _ := s.prompts.GetPrompt(ctx, "chat_system")
	softN1Rule, _ := s.prompts.GetPrompt(ctx, "soft_n_plus_one")
	supportLang := s.prompts.GetSetting(ctx, "support_lang", "pt")

	langName := "Português"
	if supportLang == "en" {
		langName = "Inglês"
	}

	// 4. RAG search for vocabulary relevant to the user's message (ONLY if marked as read)
	var matchedVocab []domain.VocabItem
	if s.vectorStore != nil {
		scored := s.vectorStore.SearchReadTopK(ctx, userMessage, 10, 0.10)
		for _, sc := range scored {
			matchedVocab = append(matchedVocab, sc.Item)
		}
	}

	vocabContext := ""
	if len(matchedVocab) > 0 {
		vocabContext = fmt.Sprintf("Vocabulário do Anki lido/estudado relevante para o contexto atual:\n%s", domain.VocabListToString(matchedVocab))
	}

	// 5. Grammar Guide RAG: Retrieve matching Tae Kim grammar lesson ONLY if read
	var matchedGrammar *domain.GrammarReference
	var grammarContext string
	if s.grammarRepo != nil {
		lessons, _ := s.grammarRepo.SearchReadLessons(ctx, userMessage, 1)
		if len(lessons) > 0 {
			l := lessons[0]
			matchedGrammar = l.Reference
			grammarContext = fmt.Sprintf("\n[REGRA GRAMATICAL DO TAE KIM'S GUIDE (ESTUDADA/LIDA PELO USUÁRIO)]:\nLição %s: %s (Capítulo: %s)\nFonte Oficial: %s\nTrecho da regra:\n%s\nSe você explicar ou corrigir gramática com base nesta lição, cite a fonte na nota pedagógica ou resposta.\n", l.SectionKey, l.Title, l.Chapter, l.URL, l.Snippet(400))
		}
	}

	fullPrompt := fmt.Sprintf(`%s

%s

Idioma de suporte para tradução e notas pedagógicas: %s.

%s
%s

Histórico recente da conversa:
%s

Nova mensagem do estudante: "%s"

Responda em JSON seguindo o schema. Lembre-se de anotar kanjis na resposta em japonês com furigana: 漢字[かんじ].
`, chatSystemPrompt, softN1Rule, langName, vocabContext, grammarContext, histBuilder.String(), userMessage)

	var raw struct {
		ReplyJapanese   string `json:"reply_japanese"`
		Translation     string `json:"translation"`
		PedagogicalNote string `json:"pedagogical_note"`
	}

	if err := s.llm.ExecuteStructured(ctx, fullPrompt, ChatResponseSchema, &raw); err != nil {
		return nil, fmt.Errorf("falha ao gerar resposta do chat: %w", err)
	}

	assistantMsg := domain.ChatMessage{
		SessionID:       sessionID,
		Role:            "assistant",
		Content:         raw.ReplyJapanese,
		FuriganaHTML:    FuriganaToHTML(raw.ReplyJapanese),
		Translation:     raw.Translation,
		PedagogicalNote: raw.PedagogicalNote,
		GrammarRef:      matchedGrammar,
	}

	if s.chatRepo != nil {
		if err := s.chatRepo.SaveChatMessage(ctx, assistantMsg); err != nil {
			return nil, fmt.Errorf("falha ao salvar mensagem do assistente: %w", err)
		}
	}

	return &assistantMsg, nil
}
