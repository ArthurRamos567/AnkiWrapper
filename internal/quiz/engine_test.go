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
	mcRaw    map[string]any
	clozeRaw map[string]any
	freeRaw  map[string]any
	evalRaw  map[string]any
}

func (m *MockLLMProvider) ExecutePrompt(ctx context.Context, prompt string) (string, error) {
	return "ok", nil
}

func (m *MockLLMProvider) ExecuteStructured(ctx context.Context, prompt string, schema string, target any) error {
	var data any
	if strings.Contains(prompt, "Modo do Quiz: \"multiple_choice\"") {
		data = m.mcRaw
	} else if strings.Contains(prompt, "Modo do Quiz: \"cloze\"") {
		data = m.clozeRaw
	} else if strings.Contains(prompt, "Modo do Quiz: \"free\"") {
		data = m.freeRaw
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
