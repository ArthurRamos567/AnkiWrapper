package quiz

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"anki-wrapper/internal/domain"
)

// MockLLMProvider for Quiz Engine tests
type MockLLMProvider struct {
	mcRaw      map[string]any
	clozeRaw   map[string]any
	freeRaw    map[string]any
	evalRaw    map[string]any
	batchRaw   map[string]any
	lastPrompt string
}

func (m *MockLLMProvider) ExecutePrompt(ctx context.Context, prompt string) (string, error) {
	m.lastPrompt = prompt
	return "ok", nil
}

func (m *MockLLMProvider) ExecuteStructured(ctx context.Context, prompt string, schema string, target any) error {
	m.lastPrompt = prompt
	var data any
	if strings.Contains(prompt, "Modo do Quiz: \"multiple_choice\"") {
		data = m.mcRaw
	} else if strings.Contains(prompt, "Modo do Quiz: \"cloze\"") {
		data = m.clozeRaw
	} else if strings.Contains(prompt, "Modo do Quiz: \"free\"") {
		data = m.freeRaw
	} else if strings.Contains(prompt, "Gere exatamente") {
		data = m.batchRaw
	} else if strings.Contains(prompt, "Você é um avaliador pedagógico") {
		data = m.evalRaw
	}
	bytes, _ := json.Marshal(data)
	return json.Unmarshal(bytes, target)
}

// MockPromptRepository for Quiz Engine tests
type MockPromptRepository struct{}

func (m *MockPromptRepository) GetAllPrompts(ctx context.Context) ([]domain.PromptItem, error) {
	return nil, nil
}
func (m *MockPromptRepository) GetPrompt(ctx context.Context, key string) (string, error) {
	return "Prompt content for " + key, nil
}
func (m *MockPromptRepository) UpdatePrompt(ctx context.Context, key, content string) error {
	return nil
}
func (m *MockPromptRepository) GetSetting(ctx context.Context, key, defaultValue string) string {
	return defaultValue
}
func (m *MockPromptRepository) SetSetting(ctx context.Context, key, value string) error {
	return nil
}

func TestQuizGenerateMultipleChoice(t *testing.T) {
	mockLLM := &MockLLMProvider{
		mcRaw: map[string]any{
			"prompt_text":    "Qual partícula indica o local de destino?",
			"sentence":       "駅[えき]____行[い]きます。",
			"options":        []string{"に", "で", "を", "が"},
			"correct_option": "に",
			"hint":           "Verbo de deslocamento",
			"explanation":    "A partícula に indica o destino.",
		},
	}

	engine := NewEngine(mockLLM, &MockPromptRepository{}, nil, nil, nil)

	q, err := engine.GenerateQuestion(context.Background(), "multiple_choice", "Partículas")
	if err != nil {
		t.Fatalf("erro ao gerar questão: %v", err)
	}

	if q.Mode != "multiple_choice" {
		t.Errorf("esperado mode multiple_choice, obtido %s", q.Mode)
	}
	if len(q.Options) != 4 {
		t.Errorf("esperado 4 opções, obtido %d", len(q.Options))
	}
	if q.CorrectAnswer != "に" {
		t.Errorf("esperado resposta correta 'に', obtido %s", q.CorrectAnswer)
	}
}

func TestQuizEvaluateMultipleChoice(t *testing.T) {
	engine := NewEngine(&MockLLMProvider{}, &MockPromptRepository{}, nil, nil, nil)

	q := &Question{
		ID:            "q_1",
		Mode:          "multiple_choice",
		CorrectAnswer: "に",
		Explanation:   "Explicação teste",
	}

	// Correct answer
	eval1, err := engine.EvaluateAnswer(context.Background(), q, "に")
	if err != nil {
		t.Fatalf("erro ao avaliar: %v", err)
	}
	if !eval1.IsCorrect {
		t.Errorf("esperado resposta correta")
	}

	// Incorrect answer
	eval2, err := engine.EvaluateAnswer(context.Background(), q, "で")
	if err != nil {
		t.Fatalf("erro ao avaliar: %v", err)
	}
	if eval2.IsCorrect {
		t.Errorf("esperado resposta incorreta")
	}
}

func TestQuizEvaluateCloze(t *testing.T) {
	engine := NewEngine(&MockLLMProvider{}, &MockPromptRepository{}, nil, nil, nil)

	q := &Question{
		ID:            "q_2",
		Mode:          "cloze",
		CorrectAnswer: "東京",
		Explanation:   "Capital do Japão",
	}

	// Case & whitespace tolerance
	eval, err := engine.EvaluateAnswer(context.Background(), q, " 東京 ")
	if err != nil {
		t.Fatalf("erro ao avaliar cloze: %v", err)
	}
	if !eval.IsCorrect {
		t.Errorf("esperado resposta correta com tolerância a espaços")
	}
}

func TestQuizGenerateFree(t *testing.T) {
	mockLLM := &MockLLMProvider{
		freeRaw: map[string]any{
			"prompt_text":      "Traduza para o japonês: 'Claro que vou voltar para casa na primavera.'",
			"reference_answer": "もちろん、春[はる]には家[いえ]に帰[かえ]ります。",
			"hint":             "Verbo voltar (kaeru)",
			"explanation":      "Explicação da frase",
		},
	}

	engine := NewEngine(mockLLM, &MockPromptRepository{}, nil, nil, nil)
	q, err := engine.GenerateQuestion(context.Background(), "free", "Primavera")
	if err != nil {
		t.Fatalf("erro ao gerar questão livre: %v", err)
	}

	if q.Mode != "free" {
		t.Errorf("esperado modo free, obtido %s", q.Mode)
	}
	if q.SentenceHTML != "" {
		t.Errorf("SentenceHTML deve ser vazio no modo free para não vazar a resposta, obtido '%s'", q.SentenceHTML)
	}
	if q.ReferenceAnswer != "もちろん、春[はる]には家[いえ]に帰[かえ]ります。" {
		t.Errorf("ReferenceAnswer incorreta: %s", q.ReferenceAnswer)
	}
}

func TestQuizGenerateBatch(t *testing.T) {
	mockLLM := &MockLLMProvider{
		mcRaw: map[string]any{
			"prompt_text":    "Qual partícula indica o local de destino?",
			"sentence":       "駅[えき]____行[い]きます。",
			"options":        []string{"に", "で", "を", "が"},
			"correct_option": "に",
			"hint":           "Verbo de deslocamento",
			"explanation":    "A partícula に indica o destino.",
		},
		batchRaw: map[string]any{
			"questions": []map[string]any{
				{
					"mode":           "cloze",
					"prompt_text":    "Complete com a palavra correta:",
					"sentence":       "朝[あさ] ____ を 食[た]べます。",
					"correct_answer": "ご飯[はん]",
					"hint":           "Refeição matinal",
					"explanation":    "Gohan significa arroz ou refeição.",
				},
				{
					"mode":           "multiple_choice",
					"prompt_text":    "Escolha o verbo adequado:",
					"sentence":       "昼[ひる]ご飯[はん]を ____。",
					"options":        []string{"食[た]べます", "飲[の]みます", "行[い]きます", "見[み]ます"},
					"correct_answer": "食[た]べます",
					"hint":           "Ingerir sólidos",
					"explanation":    "Tabemasu é comer.",
				},
			},
		},
	}

	engine := NewEngine(mockLLM, &MockPromptRepository{}, nil, nil, nil)

	// Test count = 1
	qs1, err := engine.GenerateBatch(context.Background(), []string{"multiple_choice"}, "Comida", 1)
	if err != nil {
		t.Fatalf("erro ao gerar batch com count=1: %v", err)
	}
	if len(qs1) != 1 {
		t.Fatalf("esperado 1 questão, obtido %d", len(qs1))
	}
	if qs1[0].Index != 1 || qs1[0].Total != 1 {
		t.Errorf("esperado Index=1 e Total=1, obtido Index=%d e Total=%d", qs1[0].Index, qs1[0].Total)
	}

	// Test count = 2
	qs2, err := engine.GenerateBatch(context.Background(), []string{"multiple_choice", "cloze"}, "Comida", 2)
	if err != nil {
		t.Fatalf("erro ao gerar batch com count=2: %v", err)
	}
	if len(qs2) != 2 {
		t.Fatalf("esperado 2 questões no batch, obtido %d", len(qs2))
	}
	if qs2[0].Index != 1 || qs2[0].Total != 2 {
		t.Errorf("esperado q[0] Index=1 Total=2, obtido Index=%d Total=%d", qs2[0].Index, qs2[0].Total)
	}
	if qs2[1].Index != 2 || qs2[1].Total != 2 {
		t.Errorf("esperado q[1] Index=2 Total=2, obtido Index=%d Total=%d", qs2[1].Index, qs2[1].Total)
	}
	if qs2[0].Mode != "cloze" || qs2[0].CorrectAnswer != "ご飯[はん]" {
		t.Errorf("dados incorretos na questão 1: %+v", qs2[0])
	}
	if qs2[1].Mode != "multiple_choice" || len(qs2[1].Options) != 4 {
		t.Errorf("dados incorretos na questão 2: %+v", qs2[1])
	}
}

func TestQuizGenerateWithChatContext(t *testing.T) {
	mockLLM := &MockLLMProvider{
		mcRaw: map[string]any{
			"prompt_text":    "Qual partícula indica o local de destino?",
			"sentence":       "駅[えき]____行[い]きます。",
			"options":        []string{"に", "で", "を", "が"},
			"correct_option": "に",
			"hint":           "Verbo de deslocamento",
			"explanation":    "A partícula に indica o destino.",
		},
		batchRaw: map[string]any{
			"questions": []map[string]any{
				{
					"mode":           "multiple_choice",
					"prompt_text":    "Qual partícula indica o local de destino?",
					"sentence":       "駅[えき]____行[い]きます。",
					"options":        []string{"に", "で", "を", "が"},
					"correct_answer": "に",
					"hint":           "Verbo de deslocamento",
					"explanation":    "A partícula に indica o destino.",
				},
			},
		},
	}

	engine := NewEngine(mockLLM, &MockPromptRepository{}, nil, nil, nil)
	chatTranscript := "Estudante: Como uso a partícula に com verbos de movimento?\nTutor: A partícula に indica o destino com verbos como 行く e 来る."

	// Test single question generation with chatContext
	_, err := engine.GenerateQuestion(context.Background(), "multiple_choice", "Partículas", chatTranscript)
	if err != nil {
		t.Fatalf("erro ao gerar questão com chatContext: %v", err)
	}
	if !strings.Contains(mockLLM.lastPrompt, "[CONTEÚDO DA CONVERSA RECENTE DO CHAT ANEXADA]") {
		t.Errorf("esperado prompt com seção de conversa anexada, obtido:\n%s", mockLLM.lastPrompt)
	}
	if !strings.Contains(mockLLM.lastPrompt, "Como uso a partícula に") {
		t.Errorf("esperado conteúdo da conversa no prompt, obtido:\n%s", mockLLM.lastPrompt)
	}

	// Test batch generation with chatContext
	_, err = engine.GenerateBatch(context.Background(), []string{"multiple_choice"}, "Partículas", 2, chatTranscript)
	if err != nil {
		t.Fatalf("erro ao gerar batch com chatContext: %v", err)
	}
	if !strings.Contains(mockLLM.lastPrompt, "[CONTEÚDO DA CONVERSA RECENTE DO CHAT ANEXADA]") {
		t.Errorf("esperado prompt do batch com seção de conversa anexada, obtido:\n%s", mockLLM.lastPrompt)
	}
	if !strings.Contains(mockLLM.lastPrompt, "Como uso a partícula に") {
		t.Errorf("esperado conteúdo da conversa no prompt do batch, obtido:\n%s", mockLLM.lastPrompt)
	}
}

