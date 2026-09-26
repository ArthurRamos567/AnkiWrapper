package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"anki-wrapper/internal/domain"
)

func TestGeminiProviderExecutePrompt(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("esperado POST, obtido %s", r.Method)
		}
		if r.URL.Query().Get("key") != "test-api-key" {
			t.Errorf("chave de API ausente ou incorreta: %s", r.URL.Query().Get("key"))
		}

		resp := geminiResponse{
			Candidates: []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			}{
				{
					Content: struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					}{
						Parts: []struct {
							Text string `json:"text"`
						}{
							{Text: "こんにちは！"},
						},
					},
					FinishReason: "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	provider, err := NewGeminiProvider(GeminiConfig{
		APIKey:  "test-api-key",
		BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatalf("erro ao criar provider: %v", err)
	}

	out, err := provider.ExecutePrompt(context.Background(), "Diga olá")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if out != "こんにちは！" {
		t.Errorf("esperado 'こんにちは！', obtido '%s'", out)
	}
}

func TestGeminiProviderExecuteStructured(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulates returning JSON wrapped in markdown code fence
		fencedJSON := "```json\n{\"reply_japanese\":\"はい\",\"translation\":\"Sim\"}\n```"

		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": fencedJSON},
						},
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	provider, err := NewGeminiProvider(GeminiConfig{
		APIKey:  "test-api-key",
		BaseURL: ts.URL,
	})
	if err != nil {
		t.Fatalf("erro ao criar provider: %v", err)
	}

	var target struct {
		ReplyJapanese string `json:"reply_japanese"`
		Translation   string `json:"translation"`
	}

	schema := `{"type":"object","properties":{"reply_japanese":{"type":"string"},"translation":{"type":"string"}}}`
	err = provider.ExecuteStructured(context.Background(), "teste", schema, &target)
	if err != nil {
		t.Fatalf("erro inesperado ao executar estruturado: %v", err)
	}

	if target.ReplyJapanese != "はい" || target.Translation != "Sim" {
		t.Errorf("conteúdo decodificado incorreto: %+v", target)
	}
}

func TestGeminiProviderMissingKey(t *testing.T) {
	// Without APIKey and without env var
	_, err := NewGeminiProvider(GeminiConfig{
		APIKey: "",
	})
	if err == nil {
		t.Errorf("esperado erro quando nenhuma chave é informada")
	}
}

type mockTelemetry struct {
	records []domain.LLMUsageRecord
}

func (m *mockTelemetry) RecordUsage(ctx context.Context, rec domain.LLMUsageRecord) error {
	m.records = append(m.records, rec)
	return nil
}
func (m *mockTelemetry) GetUsageStats(ctx context.Context) (*domain.LLMUsageStats, error) {
	return nil, nil
}
func (m *mockTelemetry) GetRecentUsage(ctx context.Context, limit int) ([]domain.LLMUsageRecord, error) {
	return nil, nil
}
func (m *mockTelemetry) ClearUsage(ctx context.Context) error {
	return nil
}

func TestGeminiProviderTelemetry(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": "Resposta de teste"},
						},
					},
					"finishReason": "STOP",
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":        42,
				"candidatesTokenCount":    18,
				"cachedContentTokenCount": 5,
				"totalTokenCount":         60,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	telem := &mockTelemetry{}
	provider, err := NewGeminiProvider(GeminiConfig{
		APIKey:     "test-key",
		BaseURL:    ts.URL,
		Telemetry:  telem,
	})
	if err != nil {
		t.Fatalf("falha ao criar provider: %v", err)
	}

	ctx := domain.WithOperation(context.Background(), "chat_reply")
	_, err = provider.ExecutePrompt(ctx, "Olá")
	if err != nil {
		t.Fatalf("erro ao executar prompt: %v", err)
	}

	if len(telem.records) != 1 {
		t.Fatalf("esperado 1 registro de telemetria, obtido %d", len(telem.records))
	}

	rec := telem.records[0]
	if rec.Provider != "gemini" {
		t.Errorf("esperado provedor 'gemini', obtido '%s'", rec.Provider)
	}
	if rec.Operation != "chat_reply" {
		t.Errorf("esperado operação 'chat_reply', obtido '%s'", rec.Operation)
	}
	if rec.PromptTokens != 42 || rec.OutputTokens != 18 || rec.CachedTokens != 5 || rec.TotalTokens != 60 {
		t.Errorf("contagem de tokens incorreta: %+v", rec)
	}
	if rec.Status != "SUCCESS" {
		t.Errorf("esperado status SUCCESS, obtido '%s'", rec.Status)
	}
}
