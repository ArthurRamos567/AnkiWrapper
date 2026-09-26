package quiz

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"anki-wrapper/internal/chat"
	"anki-wrapper/internal/domain"
)

// Engine orchestrates question generation and evaluation using injected interfaces.
type Engine struct {
	llm         domain.LLMProvider
	prompts     domain.PromptRepository
	vectorStore domain.VectorRepository
	quizRepo    domain.QuizRepository
	grammarRepo domain.GrammarRepository
}

// NewEngine creates a new QuizEngine with pure dependency injection.
func NewEngine(
	llm domain.LLMProvider,
	prompts domain.PromptRepository,
	vectorStore domain.VectorRepository,
	quizRepo domain.QuizRepository,
	grammarRepo domain.GrammarRepository,
) *Engine {
	return &Engine{
		llm:         llm,
		prompts:     prompts,
		vectorStore: vectorStore,
		quizRepo:    quizRepo,
		grammarRepo: grammarRepo,
	}
}

// GenerateQuestion creates a quiz question given a mode, topic, and vector-searched vocabulary.
func (e *Engine) GenerateQuestion(ctx context.Context, mode, topic string) (*Question, error) {
	ctx = domain.WithOperation(ctx, "quiz_generate")
	if mode == "" {
		mode = "multiple_choice"
	}
	if topic == "" {
		topic = "Gramática Geral e Vocabulário Conhecido"
	}

	generatorPrompt, _ := e.prompts.GetPrompt(ctx, "quiz_generator")
	softN1Rule, _ := e.prompts.GetPrompt(ctx, "soft_n_plus_one")
	supportLang := e.prompts.GetSetting(ctx, "support_lang", "pt")

	langName := "Português"
	if supportLang == "en" {
		langName = "Inglês"
	}

	// 1. RAG Search: Retrieve top 5 most compatible words for the topic (ONLY if marked as read)
	var matchedVocab []domain.VocabItem
	if e.vectorStore != nil {
		scored := e.vectorStore.SearchReadTopK(ctx, topic, 5, 0.10)
		for _, s := range scored {
			matchedVocab = append(matchedVocab, s.Item)
		}

		if len(matchedVocab) == 0 && (topic == "" || topic == "Gramática Geral e Vocabulário Conhecido") {
			readVocab := e.vectorStore.GetReadVocab(ctx)
			if len(readVocab) > 0 {
				rand.Shuffle(len(readVocab), func(i, j int) {
					readVocab[i], readVocab[j] = readVocab[j], readVocab[i]
				})
				limit := 5
				if len(readVocab) < limit {
					limit = len(readVocab)
				}
				matchedVocab = readVocab[:limit]
			}
		}
	}

	var vocabContext strings.Builder
	if len(matchedVocab) > 0 {
		vocabContext.WriteString(fmt.Sprintf("[Vocabulário do Anki lido/estudado compatível com o tópico \"%s\"]:\n", topic))
		vocabContext.WriteString(domain.VocabListToString(matchedVocab))
	} else {
		vocabContext.WriteString("[Aviso]: Nenhuma palavra correspondente foi marcada como lida no Anki pelo usuário. Utilize apenas vocabulário elementar e fundamental.\n")
	}

	if len(matchedVocab) < 2 {
		vocabContext.WriteString("\n[Diretriz Soft N+1]: Poucas palavras cadastradas para este tema. Introduza termos novos com furigana para garantir naturalidade.\n")
	}

	// 2. Grammar Guide RAG: Retrieve matching Tae Kim grammar lesson ONLY if read by the user
	var matchedGrammar *domain.GrammarReference
	var grammarContext strings.Builder
	if e.grammarRepo != nil {
		var lessons []domain.GrammarLesson
		if topic != "" && topic != "Gramática Geral e Vocabulário Conhecido" {
			lessons, _ = e.grammarRepo.SearchReadLessons(ctx, topic, 1)
		} else {
			// If topic is generic and user has read lessons, pick one of the read lessons to practice!
			readLessons, _ := e.grammarRepo.GetReadLessons(ctx)
			if len(readLessons) > 0 {
				picked := readLessons[rand.IntN(len(readLessons))]
				lessons = []domain.GrammarLesson{picked}
			}
		}

		if len(lessons) > 0 {
			l := lessons[0]
			matchedGrammar = l.Reference
			grammarContext.WriteString(fmt.Sprintf("\n[REGRA GRAMATICAL DO TAE KIM'S GUIDE (ESTUDADA/LIDA PELO USUÁRIO)]:\nLição %s: %s (Capítulo: %s)\nTrecho da regra:\n%s\n", l.SectionKey, l.Title, l.Chapter, l.Snippet(200)))
		} else {
			grammarContext.WriteString("\n[Aviso]: Crie a questão baseando-se estritamente no vocabulário fornecido e em estruturas gramaticais elementares.\n")
		}
	}

	promptBuilder := fmt.Sprintf(`%s

%s

Idioma de suporte para explicações e enunciados: %s.
Tópico solicitado: "%s".
Modo do Quiz: "%s".

%s
%s

Gere 1 questão no modo solicitado com kanji contendo furigana Anki (漢字[かんじ]).

DIRETRIZES DE FORMATO E ECONOMIA DE TOKENS:
- Responda EXCLUSIVAMENTE em JSON estruturado de acordo com o schema.
- NUNCA inclua marcações HTML (<div>, <form>, etc). O HTML da página é renderizado pelo servidor.
- Mantenha explicação, enunciado e dicas breves e diretos (máximo 2 a 3 frases) para economizar tokens.

DIRETRIZES DE ENUNCIADO:
1. No modo "free" (Tradução/Composição Livre):
   - O campo "prompt_text" deve conter EXCLUSIVAMENTE a frase na língua de suporte (%s) para ser traduzida (ex: "Traduza para o japonês: '...'").
   - NUNCA inclua a resposta em japonês no "prompt_text" ou "hint".
   - A resposta correta com furigana pertence estritamente ao campo "reference_answer".
2. No modo "multiple_choice":
   - "sentence": frase em japonês com a lacuna "____".
   - "options": 4 opções plausíveis.
   - "correct_option": a opção correta.
3. No modo "cloze":
   - "sentence": frase em japonês com a lacuna "____".
   - "correct_answer": a resposta que preenche a lacuna.
4. "hint" (opcional): Dica sutil e curta, sem revelar a resposta.
`, generatorPrompt, softN1Rule, langName, topic, mode, vocabContext.String(), grammarContext.String(), langName)

	q := &Question{
		ID:                fmt.Sprintf("q_%d", time.Now().UnixNano()),
		Index:             1,
		Total:             1,
		Mode:              mode,
		Topic:             topic,
		MatchedVocabCount: len(matchedVocab),
		GrammarRef:        matchedGrammar,
	}

	switch mode {
	case "multiple_choice":
		var raw struct {
			PromptText    string   `json:"prompt_text"`
			Sentence      string   `json:"sentence"`
			Options       []string `json:"options"`
			CorrectOption string   `json:"correct_option"`
			Hint          string   `json:"hint"`
			Explanation   string   `json:"explanation"`
		}
		if err := e.llm.ExecuteStructured(ctx, promptBuilder, MultipleChoiceSchema, &raw); err != nil {
			return nil, fmt.Errorf("falha ao gerar questão de múltipla escolha: %w", err)
		}
		q.PromptText = raw.PromptText
		q.Sentence = raw.Sentence
		q.SentenceHTML = chat.FuriganaToHTML(raw.Sentence)
		q.Options = raw.Options
		q.CorrectAnswer = raw.CorrectOption
		q.Hint = raw.Hint
		q.Explanation = raw.Explanation

	case "cloze":
		var raw struct {
			PromptText    string `json:"prompt_text"`
			Sentence      string `json:"sentence"`
			CorrectAnswer string `json:"correct_answer"`
			Hint          string `json:"hint"`
			Explanation   string `json:"explanation"`
		}
		if err := e.llm.ExecuteStructured(ctx, promptBuilder, ClozeSchema, &raw); err != nil {
			return nil, fmt.Errorf("falha ao gerar questão cloze: %w", err)
		}
		q.PromptText = raw.PromptText
		q.Sentence = raw.Sentence
		q.SentenceHTML = chat.FuriganaToHTML(raw.Sentence)
		q.CorrectAnswer = raw.CorrectAnswer
		q.Hint = raw.Hint
		q.Explanation = raw.Explanation

	case "free":
		var raw struct {
			PromptText      string `json:"prompt_text"`
			ReferenceAnswer string `json:"reference_answer"`
			Hint            string `json:"hint"`
			Explanation     string `json:"explanation"`
		}
		if err := e.llm.ExecuteStructured(ctx, promptBuilder, FreeSchema, &raw); err != nil {
			return nil, fmt.Errorf("falha ao gerar questão livre: %w", err)
		}
		q.PromptText = raw.PromptText
		q.ReferenceAnswer = raw.ReferenceAnswer
		q.Sentence = ""
		q.SentenceHTML = ""
		q.CorrectAnswer = raw.ReferenceAnswer
		q.Hint = raw.Hint
		q.Explanation = raw.Explanation
	}

	return q, nil
}

// GenerateBatch creates count quiz questions for the given modes and topic in an efficient single LLM call.
func (e *Engine) GenerateBatch(ctx context.Context, modes []string, topic string, count int) ([]*Question, error) {
	if count <= 1 {
		selectedMode := "multiple_choice"
		if len(modes) > 0 {
			selectedMode = modes[rand.IntN(len(modes))]
		}
		q, err := e.GenerateQuestion(ctx, selectedMode, topic)
		if err != nil {
			return nil, err
		}
		q.Index = 1
		q.Total = 1
		return []*Question{q}, nil
	}

	if count > 10 {
		count = 10
	}

	ctx = domain.WithOperation(ctx, "quiz_generate")
	if len(modes) == 0 {
		modes = []string{"multiple_choice", "cloze", "free"}
	}
	if topic == "" {
		topic = "Gramática Geral e Vocabulário Conhecido"
	}

	generatorPrompt, _ := e.prompts.GetPrompt(ctx, "quiz_generator")
	softN1Rule, _ := e.prompts.GetPrompt(ctx, "soft_n_plus_one")
	supportLang := e.prompts.GetSetting(ctx, "support_lang", "pt")

	langName := "Português"
	if supportLang == "en" {
		langName = "Inglês"
	}

	// 1. RAG Search: Retrieve top compatible words for the topic (ONLY if marked as read)
	vocabLimit := 5 * count
	if vocabLimit > 20 {
		vocabLimit = 20
	}
	var matchedVocab []domain.VocabItem
	if e.vectorStore != nil {
		scored := e.vectorStore.SearchReadTopK(ctx, topic, vocabLimit, 0.10)
		for _, s := range scored {
			matchedVocab = append(matchedVocab, s.Item)
		}

		if len(matchedVocab) == 0 && (topic == "" || topic == "Gramática Geral e Vocabulário Conhecido") {
			readVocab := e.vectorStore.GetReadVocab(ctx)
			if len(readVocab) > 0 {
				rand.Shuffle(len(readVocab), func(i, j int) {
					readVocab[i], readVocab[j] = readVocab[j], readVocab[i]
				})
				limit := vocabLimit
				if len(readVocab) < limit {
					limit = len(readVocab)
				}
				matchedVocab = readVocab[:limit]
			}
		}
	}

	var vocabContext strings.Builder
	if len(matchedVocab) > 0 {
		vocabContext.WriteString(fmt.Sprintf("[Vocabulário do Anki lido/estudado compatível com o tópico \"%s\"]:\n", topic))
		vocabContext.WriteString(domain.VocabListToString(matchedVocab))
	} else {
		vocabContext.WriteString("[Aviso]: Nenhuma palavra correspondente foi marcada como lida no Anki pelo usuário. Utilize apenas vocabulário elementar e fundamental.\n")
	}

	if len(matchedVocab) < 2 {
		vocabContext.WriteString("\n[Diretriz Soft N+1]: Poucas palavras cadastradas para este tema. Introduza termos novos com furigana para garantir naturalidade.\n")
	}

	// 2. Grammar Guide RAG: Retrieve matching Tae Kim grammar lesson ONLY if read by the user
	var matchedGrammar *domain.GrammarReference
	var grammarContext strings.Builder
	if e.grammarRepo != nil {
		var lessons []domain.GrammarLesson
		if topic != "" && topic != "Gramática Geral e Vocabulário Conhecido" {
			lessons, _ = e.grammarRepo.SearchReadLessons(ctx, topic, 1)
		} else {
			readLessons, _ := e.grammarRepo.GetReadLessons(ctx)
			if len(readLessons) > 0 {
				picked := readLessons[rand.IntN(len(readLessons))]
				lessons = []domain.GrammarLesson{picked}
			}
		}

		if len(lessons) > 0 {
			l := lessons[0]
			matchedGrammar = l.Reference
			grammarContext.WriteString(fmt.Sprintf("\n[REGRA GRAMATICAL DO TAE KIM'S GUIDE (ESTUDADA/LIDA PELO USUÁRIO)]:\nLição %s: %s (Capítulo: %s)\nTrecho da regra:\n%s\n", l.SectionKey, l.Title, l.Chapter, l.Snippet(200)))
		} else {
			grammarContext.WriteString("\n[Aviso]: Crie a questão baseando-se estritamente no vocabulário fornecido e em estruturas gramaticais elementares.\n")
		}
	}

	modesStr := strings.Join(modes, ", ")
	promptBuilder := fmt.Sprintf(`%s

%s

Idioma de suporte para explicações e enunciados: %s.
Tópico solicitado: "%s".
Modos permitidos para as questões: [%s].

%s
%s

Gere exatamente %d questões nos modos permitidos [%s] com kanji contendo furigana Anki (漢字[かんじ]).

DIRETRIZES DE FORMATO E ECONOMIA DE TOKENS:
- Responda EXCLUSIVAMENTE em JSON estruturado de acordo com o schema, preenchendo a lista "questions".
- NUNCA inclua marcações HTML (<div>, <form>, etc). O HTML da página é renderizado pelo servidor.
- Mantenha explicação, enunciado e dicas breves e diretos (máximo 2 a 3 frases) para economizar tokens.

DIRETRIZES PARA CADA MODO:
1. No modo "free" (Tradução/Composição Livre):
   - O campo "prompt_text" deve conter EXCLUSIVAMENTE a frase na língua de suporte (%s) para ser traduzida (ex: "Traduza para o japonês: '...'").
   - NUNCA inclua a resposta em japonês no "prompt_text" ou "hint".
   - A resposta correta com furigana pertence ao campo "reference_answer".
   - Deixe o campo "sentence" vazio.
2. No modo "multiple_choice":
   - "sentence": frase em japonês com a lacuna "____".
   - "options": 4 opções plausíveis.
   - "correct_answer": a opção correta.
3. No modo "cloze":
   - "sentence": frase em japonês com a lacuna "____".
   - "correct_answer": a resposta que preenche a lacuna.
4. "hint" (opcional): Dica sutil e curta, sem revelar a resposta.
`, generatorPrompt, softN1Rule, langName, topic, modesStr, vocabContext.String(), grammarContext.String(), count, modesStr, langName)

	var raw struct {
		Questions []struct {
			Mode            string   `json:"mode"`
			PromptText      string   `json:"prompt_text"`
			Sentence        string   `json:"sentence"`
			Options         []string `json:"options"`
			CorrectAnswer   string   `json:"correct_answer"`
			ReferenceAnswer string   `json:"reference_answer"`
			Hint            string   `json:"hint"`
			Explanation     string   `json:"explanation"`
		} `json:"questions"`
	}

	if err := e.llm.ExecuteStructured(ctx, promptBuilder, BatchQuestionsSchema, &raw); err != nil {
		return nil, fmt.Errorf("falha ao gerar lote de questões: %w", err)
	}

	if len(raw.Questions) == 0 {
		return nil, fmt.Errorf("o modelo não retornou nenhuma questão válida")
	}

	now := time.Now().UnixNano()
	total := len(raw.Questions)
	var questions []*Question

	for i, item := range raw.Questions {
		mode := item.Mode
		if mode != "multiple_choice" && mode != "cloze" && mode != "free" {
			if len(modes) > 0 {
				mode = modes[i%len(modes)]
			} else {
				mode = "multiple_choice"
			}
		}

		correctAns := item.CorrectAnswer
		if correctAns == "" && item.ReferenceAnswer != "" {
			correctAns = item.ReferenceAnswer
		}

		refAns := item.ReferenceAnswer
		if refAns == "" && item.CorrectAnswer != "" {
			refAns = item.CorrectAnswer
		}

		sentence := item.Sentence
		sentenceHTML := ""
		if mode != "free" && sentence != "" {
			sentenceHTML = chat.FuriganaToHTML(sentence)
		} else if mode == "free" {
			sentence = ""
		}

		q := &Question{
			ID:                fmt.Sprintf("q_%d_%d", now, i+1),
			Index:             i + 1,
			Total:             total,
			Mode:              mode,
			Topic:             topic,
			PromptText:        item.PromptText,
			Sentence:          sentence,
			SentenceHTML:      sentenceHTML,
			Options:           item.Options,
			CorrectAnswer:     correctAns,
			ReferenceAnswer:   refAns,
			Hint:              item.Hint,
			Explanation:       item.Explanation,
			GrammarRef:        matchedGrammar,
			MatchedVocabCount: len(matchedVocab),
		}
		questions = append(questions, q)
	}

	return questions, nil
}

// EvaluateAnswer evaluates a user's answer and persists the attempt in the quiz repository.
func (e *Engine) EvaluateAnswer(ctx context.Context, q *Question, userAnswer string) (*EvaluationResult, error) {
	trimmedUser := strings.TrimSpace(userAnswer)

	var eval *EvaluationResult

	// For multiple choice and cloze, do deterministic check with LLM explanation
	if q.Mode == "multiple_choice" {
		isCorrect := strings.EqualFold(trimmedUser, strings.TrimSpace(q.CorrectAnswer)) ||
			strings.HasPrefix(strings.ToLower(trimmedUser), strings.ToLower(q.CorrectAnswer))
		score := 0
		if isCorrect {
			score = 100
		}
		eval = &EvaluationResult{
			IsCorrect:         isCorrect,
			Score:             score,
			UserAnswer:        userAnswer,
			CorrectAnswer:     q.CorrectAnswer,
			CorrectAnswerHTML: chat.FuriganaToHTML(q.CorrectAnswer),
			Explanation:       q.Explanation,
			Feedback:          fmt.Sprintf("Opção correta: %s", q.CorrectAnswer),
		}
	} else if q.Mode == "cloze" {
		normUser := strings.Trim(trimmedUser, "。、,.!?！？")
		normCorrect := strings.Trim(strings.TrimSpace(q.CorrectAnswer), "。、,.!?！？")
		isCorrect := strings.EqualFold(normUser, normCorrect)
		score := 0
		if isCorrect {
			score = 100
		}
		eval = &EvaluationResult{
			IsCorrect:         isCorrect,
			Score:             score,
			UserAnswer:        userAnswer,
			CorrectAnswer:     q.CorrectAnswer,
			CorrectAnswerHTML: chat.FuriganaToHTML(q.CorrectAnswer),
			Explanation:       q.Explanation,
			Feedback:          q.Explanation,
		}
	} else {
		// For free composition, use LLM evaluator
		ctx = domain.WithOperation(ctx, "quiz_evaluate")
		evaluatorPrompt, _ := e.prompts.GetPrompt(ctx, "quiz_evaluator")
		supportLang := e.prompts.GetSetting(ctx, "support_lang", "pt")

		prompt := fmt.Sprintf(`%s

Idioma de suporte para o feedback: %s.
Enunciado original: "%s"
Resposta de referência esperada: "%s"
Explicação gramatical de referência: "%s"

Resposta submetida pelo estudante:
"%s"

Avalie com gentileza e rigor pedagógico, indicando se a frase está correta ou compreensível, nota (0 a 100), versão natural corrigida com furigana Anki (漢字[かんじ]), e explicação detalhada.
`, evaluatorPrompt, supportLang, q.PromptText, q.ReferenceAnswer, q.Explanation, trimmedUser)

		var raw struct {
			IsCorrect         bool   `json:"is_correct"`
			Score             int    `json:"score"`
			CorrectedSentence string `json:"corrected_sentence"`
			Explanation       string `json:"explanation"`
			Feedback          string `json:"feedback"`
		}

		if err := e.llm.ExecuteStructured(ctx, prompt, EvaluationSchema, &raw); err != nil {
			return nil, fmt.Errorf("falha ao avaliar resposta: %w", err)
		}

		eval = &EvaluationResult{
			IsCorrect:         raw.IsCorrect,
			Score:             raw.Score,
			UserAnswer:        userAnswer,
			CorrectAnswer:     raw.CorrectedSentence,
			CorrectAnswerHTML: chat.FuriganaToHTML(raw.CorrectedSentence),
			Explanation:       raw.Explanation,
			Feedback:          raw.Feedback,
		}
	}

	// Persist attempt via injected QuizRepository
	if e.quizRepo != nil {
		_ = e.quizRepo.SaveQuizRecord(ctx, domain.QuizRecord{
			Mode:          q.Mode,
			Topic:         q.Topic,
			Question:      q.PromptText,
			UserAnswer:    userAnswer,
			CorrectAnswer: eval.CorrectAnswer,
			IsCorrect:     eval.IsCorrect,
			Explanation:   eval.Explanation,
		})
	}

	eval.GrammarRef = q.GrammarRef
	return eval, nil
}
