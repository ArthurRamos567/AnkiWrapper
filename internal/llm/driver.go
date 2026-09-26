package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"anki-wrapper/internal/domain"
)

// Ensure Driver implements domain.LLMProvider at compile time.
var _ domain.LLMProvider = (*Driver)(nil)

// AgyUsage holds token counts reported by agy.
type AgyUsage struct {
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	ThinkingTokens  int `json:"thinking_tokens"`
	CacheReadTokens int `json:"cache_read_tokens"`
	TotalTokens     int `json:"total_tokens"`
}

// AgyResponse represents the wrapper format output by agy --output-format json.
type AgyResponse struct {
	ConversationID   string          `json:"conversation_id"`
	Status           string          `json:"status"`
	Response         string          `json:"response"`
	DurationSeconds  float64         `json:"duration_seconds"`
	StructuredOutput json.RawMessage `json:"structured_output"`
	Usage            AgyUsage        `json:"usage"`
	Error            string          `json:"error,omitempty"`
}

// Driver handles execution of the local agy CLI with concurrency limiting.
type Driver struct {
	AgyPath   string
	Model     string
	Timeout   time.Duration
	Telemetry domain.TelemetryRepository
	sem       chan struct{} // Semaphore limiting concurrent OS process executions
}

// SetTelemetry attaches a telemetry repository to record token metrics.
func (d *Driver) SetTelemetry(t domain.TelemetryRepository) {
	d.Telemetry = t
}

// NewDriver creates an AgyDriver, auto-detecting the agy binary, resolving the model and configuring concurrency limit.
func NewDriver(customPath string, model ...string) (*Driver, error) {
	path := customPath
	if path == "" {
		if p, err := exec.LookPath("agy"); err == nil {
			path = p
		} else {
			home, _ := os.UserHomeDir()
			candidate := filepath.Join(home, ".local", "bin", "agy")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
			} else {
				return nil, fmt.Errorf("binário do agy não encontrado no PATH nem em ~/.local/bin/agy")
			}
		}
	}

	selectedModel := ""
	if len(model) > 0 && model[0] != "" {
		selectedModel = model[0]
	} else if envModel := os.Getenv("AGY_MODEL"); envModel != "" {
		selectedModel = envModel
	} else {
		selectedModel = "gemini-3.8-flash-low"
	}

	return &Driver{
		AgyPath: path,
		Model:   selectedModel,
		Timeout: 180 * time.Second,
		sem:     make(chan struct{}, 1), // Max 1 active CLI subprocess at a time
	}, nil
}

// ExecutePrompt executes agy with a prompt and returns the raw text response.
func (d *Driver) ExecutePrompt(ctx context.Context, prompt string) (string, error) {
	// Acquire semaphore slot or wait for active call to finish
	select {
	case d.sem <- struct{}{}:
		defer func() { <-d.sem }()
	case <-ctx.Done():
		return "", ctx.Err()
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()

	args := []string{
		"--dangerously-skip-permissions",
		"--disable-slash-commands",
		"--effort", "low",
	}
	if d.Model != "" {
		args = append(args, "--model", d.Model)
	}
	args = append(args, "--print", prompt, "--output-format", "text")

	cmd := exec.CommandContext(ctxWithTimeout, d.AgyPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stdout.Len() > 0 {
			return stdout.String(), nil
		}
		return "", fmt.Errorf("falha ao executar agy: %w, stderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// ExecuteStructured executes agy with a prompt and enforces a JSON schema, unmarshaling the result into target.
func (d *Driver) ExecuteStructured(ctx context.Context, prompt string, schema string, target any) error {
	// Acquire semaphore slot or wait for active call to finish
	select {
	case d.sem <- struct{}{}:
		defer func() { <-d.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	start := time.Now()
	ctxWithTimeout, cancel := context.WithTimeout(ctx, d.Timeout)
	defer cancel()

	args := []string{
		"--dangerously-skip-permissions",
		"--disable-slash-commands",
		"--effort", "low",
	}
	if d.Model != "" {
		args = append(args, "--model", d.Model)
	}
	args = append(args, "--print", prompt, "--output-format", "json")

	if schema != "" {
		args = append(args, "--json-schema", schema)
	}

	cmd := exec.CommandContext(ctxWithTimeout, d.AgyPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// Parse JSON wrapper from stdout
	var wrapper AgyResponse
	unmarshalErr := json.Unmarshal(stdout.Bytes(), &wrapper)

	if d.Telemetry != nil {
		status := "SUCCESS"
		if runErr != nil && unmarshalErr != nil {
			status = "ERROR"
		}
		modelName := "agy-cli"
		if d.Model != "" {
			modelName = "agy:" + d.Model
		}
		_ = d.Telemetry.RecordUsage(ctx, domain.LLMUsageRecord{
			Provider:       "agy",
			Model:          modelName,
			Operation:      domain.GetOperation(ctx),
			PromptTokens:   wrapper.Usage.InputTokens,
			OutputTokens:   wrapper.Usage.OutputTokens,
			ThinkingTokens: wrapper.Usage.ThinkingTokens,
			CachedTokens:   wrapper.Usage.CacheReadTokens,
			TotalTokens:    wrapper.Usage.TotalTokens,
			DurationMs:     time.Since(start).Milliseconds(),
			Status:         status,
			ErrorMsg:       wrapper.Error,
			CreatedAt:      time.Now(),
		})
	}

	// If cmd failed but we already got a valid SUCCESS structured_output in stdout, salvage it!
	if runErr != nil && unmarshalErr != nil {
		return fmt.Errorf("falha ao executar agy: %w, stderr: %s, stdout: %s", runErr, stderr.String(), stdout.String())
	}

	if wrapper.Status != "SUCCESS" && wrapper.Status != "" {
		if runErr != nil {
			return fmt.Errorf("falha ao executar agy: %w (status: %s, erro: %s, stderr: %s)", runErr, wrapper.Status, wrapper.Error, stderr.String())
		}
		return fmt.Errorf("agy retornou status diferente de SUCCESS: %s (erro: %s)", wrapper.Status, wrapper.Error)
	}

	if len(wrapper.StructuredOutput) == 0 || string(wrapper.StructuredOutput) == "null" {
		// Fallback: try unmarshaling the response string itself if structured_output wasn't populated
		if wrapper.Response != "" {
			if err := json.Unmarshal([]byte(wrapper.Response), target); err == nil {
				return nil
			}
		}
		if runErr != nil {
			return fmt.Errorf("falha ao executar agy: %w, nenhum structured_output gerado", runErr)
		}
		return fmt.Errorf("nenhum structured_output gerado na resposta")
	}

	if err := json.Unmarshal(wrapper.StructuredOutput, target); err != nil {
		return fmt.Errorf("falha ao converter structured_output para o tipo esperado: %w", err)
	}

	return nil
}
