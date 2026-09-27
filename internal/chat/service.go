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
    "reply": {"type": "string"},
    "translation": {"type": "string"},
    "pedagogical_note": {"type": "string"}
  },
  "required": ["reply"]
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

// Reply generates a chat response for the given user message and session, adapting to the selected chatMode (pt, ja, auto).
func (s *Service) Reply(ctx context.Context, sessionID, userMessage string, mode ...string) (*domain.ChatMessage, error) {
	chatMode := "auto"
	if len(mode) > 0 && mode[0] != "" {
		chatMode = mode[0]
	}

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

	var modeGuideline string
	switch chatMode {
	case "pt":
		modeGuideline = fmt.Sprintf(`DIRETRIZ DE IDIOMA E PAPEL OBRIGATÓRIA:
- O estudante selecionou o modo "Português (Tutor / Dúvidas)".
- Responda EXCLUSIVAMENTE em %s no campo "reply".
- Explique conceitos, regras gramaticais, etimologia, radicais e tire dúvidas com clareza pedagógica em %s.
- Ao citar termos, frases ou kanjis em japonês como exemplos, anote-os com furigana no formato Anki: 漢字[かんじ].
- O campo "translation" pode ficar vazio (pois a resposta principal já está em %s).`, langName, langName, langName)
	case "ja":
		modeGuideline = fmt.Sprintf(`DIRETRIZ DE IDIOMA E PAPEL OBRIGATÓRIA:
- O estudante selecionou o modo "Japonês (Conversação)".
- Responda em Japonês no campo "reply", mantendo uma conversa natural e fluente no nível do estudante.
- Anote todos os kanjis em japonês com furigana no formato Anki: 漢字[かんじ].
- Forneça a tradução completa em %s no campo "translation".`, langName)
	default: // "auto"
		modeGuideline = fmt.Sprintf(`DIRETRIZ DE IDIOMA E ADAPTAÇÃO:
- O estudante selecionou o modo "Automático (Adaptar)".
- Se a mensagem do estudante for uma pergunta, dúvida ou pedido de explicação em %s, responda em %s no campo "reply" (anotando termos ou exemplos em japonês com furigana 漢字[かんじ]).
- Se a mensagem do estudante for uma prática de conversa em Japonês, converse e responda em Japonês no campo "reply" (com furigana Anki 漢字[かんじ]) e forneça a tradução de apoio em %s no campo "translation".`, langName, langName, langName)
	}

	fullPrompt := fmt.Sprintf(`%s

%s

Idioma de suporte configurado: %s.

%s

%s
%s

Histórico recente da conversa:
%s

Nova mensagem do estudante: "%s"

INSTRUÇÕES DE FORMATO:
- Responda EXCLUSIVAMENTE em JSON estruturado de acordo com o schema.
- Preencha o campo "reply" com o texto da sua resposta.
- Sempre anote termos ou kanjis em japonês com furigana Anki: 漢字[かんじ].
`, chatSystemPrompt, softN1Rule, langName, modeGuideline, vocabContext, grammarContext, histBuilder.String(), userMessage)

	var raw struct {
		Reply           string `json:"reply"`
		ReplyJapanese   string `json:"reply_japanese"`
		Translation     string `json:"translation"`
		PedagogicalNote string `json:"pedagogical_note"`
	}

	if err := s.llm.ExecuteStructured(ctx, fullPrompt, ChatResponseSchema, &raw); err != nil {
		return nil, fmt.Errorf("falha ao gerar resposta do chat: %w", err)
	}

	replyText := raw.Reply
	if replyText == "" && raw.ReplyJapanese != "" {
		replyText = raw.ReplyJapanese
	}

	assistantMsg := domain.ChatMessage{
		SessionID:       sessionID,
		Role:            "assistant",
		Content:         replyText,
		FuriganaHTML:    FuriganaToHTML(replyText),
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
