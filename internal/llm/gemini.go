package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"anki-wrapper/internal/domain"
)

// Ensure GeminiProvider implements domain.LLMProvider at compile time.
var _ domain.LLMProvider = (*GeminiProvider)(nil)

// GeminiConfig holds options for creating a GeminiProvider.
type GeminiConfig struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
	Telemetry  domain.TelemetryRepository
}

// GeminiProvider implements domain.LLMProvider communicating directly with the Google Gemini API (Google AI Studio).
type GeminiProvider struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
	Telemetry  domain.TelemetryRepository
}

// SetTelemetry attaches a telemetry repository to record token metrics.
func (p *GeminiProvider) SetTelemetry(t domain.TelemetryRepository) {
	p.Telemetry = t
}

// NewGeminiProvider creates a new GeminiProvider.
// It resolves the API Key from cfg.APIKey, then GEMINI_API_KEY environment variable.
func NewGeminiProvider(cfg GeminiConfig) (*GeminiProvider, error) {
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	}
	if key == "" {
		return nil, fmt.Errorf("chave de API do Gemini não informada (defina a variável de ambiente GEMINI_API_KEY ou a flag -gemini-key)")
	}

	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("GEMINI_MODEL"))
	}
	if model == "" {
		model = "gemini-3.8-flash"
	}

	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}

	return &GeminiProvider{
		apiKey:     key,
		model:      model,
		baseURL:    baseURL,
		httpClient: client,
		Telemetry:  cfg.Telemetry,
	}, nil
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents         []geminiContent `json:"contents"`
	GenerationConfig map[string]any  `json:"generationConfig,omitempty"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
		TotalTokenCount         int `json:"totalTokenCount"`
	} `json:"usageMetadata,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// ExecutePrompt sends a prompt to the Gemini API and returns the text response.
func (p *GeminiProvider) ExecutePrompt(ctx context.Context, prompt string) (string, error) {
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{{Text: prompt}},
			},
		},
	}

	rawResp, err := p.sendRequest(ctx, reqBody)
	if err != nil {
		return "", err
	}

	return rawResp, nil
}

// ExecuteStructured sends a prompt and enforces a JSON response, unmarshaling the result into target.
func (p *GeminiProvider) ExecuteStructured(ctx context.Context, prompt string, schema string, target any) error {
	genConfig := map[string]any{
		"responseMimeType": "application/json",
	}

	if schema != "" {
		var schemaObj map[string]any
		if err := json.Unmarshal([]byte(schema), &schemaObj); err == nil {
			genConfig["responseSchema"] = schemaObj
		}
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{{Text: prompt}},
			},
		},
		GenerationConfig: genConfig,
	}

	rawResp, err := p.sendRequest(ctx, reqBody)
	if err != nil {
		return err
	}

	cleanJSON := cleanMarkdownFences(rawResp)

	if err := json.Unmarshal([]byte(cleanJSON), target); err != nil {
		return fmt.Errorf("falha ao converter JSON da Gemini API para o tipo esperado: %w (resposta: %s)", err, rawResp)
	}

	return nil
}

func (p *GeminiProvider) sendRequest(ctx context.Context, reqBody geminiRequest) (string, error) {
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.baseURL, p.model, p.apiKey)

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("falha ao serializar requisição para Gemini API: %w", err)
	}

	maxRetries := 2
	for attempt := 0; attempt <= maxRetries; attempt++ {
		start := time.Now()
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBytes))
		if err != nil {
			return "", fmt.Errorf("falha ao criar requisição HTTP: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			if attempt < maxRetries && ctx.Err() == nil {
				time.Sleep(time.Duration(attempt+1) * time.Second)
				continue
			}
			if p.Telemetry != nil {
				_ = p.Telemetry.RecordUsage(ctx, domain.LLMUsageRecord{
					Provider:   "gemini",
					Model:      p.model,
					Operation:  domain.GetOperation(ctx),
					DurationMs: time.Since(start).Milliseconds(),
					Status:     "ERROR",
					ErrorMsg:   err.Error(),
					CreatedAt:  time.Now(),
				})
			}
			return "", fmt.Errorf("erro na chamada à Gemini API: %w", err)
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("falha ao ler corpo da resposta da Gemini API: %w", err)
		}

		var geminiResp geminiResponse
		if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
			if p.Telemetry != nil {
				_ = p.Telemetry.RecordUsage(ctx, domain.LLMUsageRecord{
					Provider:   "gemini",
					Model:      p.model,
					Operation:  domain.GetOperation(ctx),
					DurationMs: time.Since(start).Milliseconds(),
					Status:     "ERROR",
					ErrorMsg:   fmt.Sprintf("HTTP %d: resposta inválida", resp.StatusCode),
					CreatedAt:  time.Now(),
				})
			}
			return "", fmt.Errorf("resposta inválida da Gemini API (HTTP %d): %s", resp.StatusCode, string(bodyBytes))
		}

		// Retry on transient spikes: 503 UNAVAILABLE or 429 RATE LIMIT
		isRetryable := resp.StatusCode == 503 || resp.StatusCode == 429
		if geminiResp.Error != nil && (geminiResp.Error.Code == 503 || geminiResp.Error.Code == 429 || geminiResp.Error.Status == "UNAVAILABLE" || geminiResp.Error.Status == "RESOURCE_EXHAUSTED") {
			isRetryable = true
		}

		if isRetryable && attempt < maxRetries && ctx.Err() == nil {
			time.Sleep(time.Duration(attempt+1) * 1500 * time.Millisecond)
			continue
		}

		if p.Telemetry != nil {
			var promptTok, outTok, cachedTok, totTok int
			if geminiResp.UsageMetadata != nil {
				promptTok = geminiResp.UsageMetadata.PromptTokenCount
				outTok = geminiResp.UsageMetadata.CandidatesTokenCount
				cachedTok = geminiResp.UsageMetadata.CachedContentTokenCount
				totTok = geminiResp.UsageMetadata.TotalTokenCount
			}
			status := "SUCCESS"
			var errMsg string
			if geminiResp.Error != nil {
				status = "ERROR"
				errMsg = geminiResp.Error.Message
			}
			_ = p.Telemetry.RecordUsage(ctx, domain.LLMUsageRecord{
				Provider:     "gemini",
				Model:        p.model,
				Operation:    domain.GetOperation(ctx),
				PromptTokens: promptTok,
				OutputTokens: outTok,
				CachedTokens: cachedTok,
				TotalTokens:  totTok,
				DurationMs:   time.Since(start).Milliseconds(),
				Status:       status,
				ErrorMsg:     errMsg,
				CreatedAt:    time.Now(),
			})
		}

		if geminiResp.Error != nil {
			return "", fmt.Errorf("erro da Gemini API (código %d, %s): %s", geminiResp.Error.Code, geminiResp.Error.Status, geminiResp.Error.Message)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("erro HTTP da Gemini API (%d): %s", resp.StatusCode, string(bodyBytes))
		}

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			return "", fmt.Errorf("Gemini API retornou resposta vazia (sem candidatos)")
		}

		return geminiResp.Candidates[0].Content.Parts[0].Text, nil
	}

	return "", fmt.Errorf("todas as tentativas de requisição à Gemini API falharam")
}

func cleanMarkdownFences(s string) string {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) >= 2 && strings.HasPrefix(lines[0], "```") {
			lines = lines[1:]
		}
		if len(lines) >= 1 && strings.HasSuffix(lines[len(lines)-1], "```") {
			lines = lines[:len(lines)-1]
		}
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	return trimmed
}
