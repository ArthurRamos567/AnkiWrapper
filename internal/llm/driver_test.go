package llm

import (
	"encoding/json"
	"testing"
)

func TestAgyResponseParsing(t *testing.T) {
	sampleJSON := `{
		"conversation_id": "test-123",
		"status": "SUCCESS",
		"response": "{\"reply_japanese\":\"こんにちは\",\"translation\":\"Olá\"}",
		"duration_seconds": 1.5,
		"usage": {
			"input_tokens": 120,
			"output_tokens": 30,
			"thinking_tokens": 15,
			"cache_read_tokens": 50,
			"total_tokens": 150
		},
		"structured_output": {
			"reply_japanese": "こんにちは",
			"translation": "Olá"
		}
	}`

	var wrapper AgyResponse
	if err := json.Unmarshal([]byte(sampleJSON), &wrapper); err != nil {
		t.Fatalf("falha ao decodificar JSON do agy: %v", err)
	}

	if wrapper.Status != "SUCCESS" {
		t.Errorf("esperado status SUCCESS, obtido %s", wrapper.Status)
	}
	if wrapper.Usage.InputTokens != 120 || wrapper.Usage.OutputTokens != 30 || wrapper.Usage.TotalTokens != 150 {
		t.Errorf("telemetria de uso decodificada incorreta: %+v", wrapper.Usage)
	}

	var target struct {
		ReplyJapanese string `json:"reply_japanese"`
		Translation   string `json:"translation"`
	}

	if err := json.Unmarshal(wrapper.StructuredOutput, &target); err != nil {
		t.Fatalf("falha ao unmarshal structured_output: %v", err)
	}

	if target.ReplyJapanese != "こんにちは" {
		t.Errorf("esperado こんにちは, obtido %s", target.ReplyJapanese)
	}
	if target.Translation != "Olá" {
		t.Errorf("esperado Olá, obtido %s", target.Translation)
	}
}

func TestAgyResponseFallbackParsing(t *testing.T) {
	// Case where structured_output is null or empty, but response has valid JSON
	sampleJSON := `{
		"conversation_id": "test-456",
		"status": "SUCCESS",
		"response": "{\"key\":\"value\"}",
		"duration_seconds": 1.2,
		"structured_output": null
	}`

	var wrapper AgyResponse
	if err := json.Unmarshal([]byte(sampleJSON), &wrapper); err != nil {
		t.Fatalf("falha ao decodificar JSON: %v", err)
	}

	var target struct {
		Key string `json:"key"`
	}

	if len(wrapper.StructuredOutput) == 0 || string(wrapper.StructuredOutput) == "null" {
		if err := json.Unmarshal([]byte(wrapper.Response), &target); err != nil {
			t.Fatalf("falha no fallback para response: %v", err)
		}
	}

	if target.Key != "value" {
		t.Errorf("esperado value, obtido %s", target.Key)
	}
}
