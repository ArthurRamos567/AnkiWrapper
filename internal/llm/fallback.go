package llm

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"anki-wrapper/internal/domain"
)

// Ensure FallbackProvider implements domain.LLMProvider at compile time.
var _ domain.LLMProvider = (*FallbackProvider)(nil)

// FallbackProvider wraps a primary LLMProvider and falls back to a secondary LLMProvider
// when the primary encounters an error (e.g. rate limit, depleted credits/payment required 402, network failure).
type FallbackProvider struct {
	primary          domain.LLMProvider
	secondary        domain.LLMProvider
	primaryName      string
	secondaryName    string
	mu               sync.RWMutex
	primaryDownUntil time.Time
}

// NewFallbackProvider creates a provider that tries primary first and fails over to secondary on error.
func NewFallbackProvider(primary domain.LLMProvider, primaryName string, secondary domain.LLMProvider, secondaryName string) *FallbackProvider {
	return &FallbackProvider{
		primary:       primary,
		secondary:     secondary,
		primaryName:   primaryName,
		secondaryName: secondaryName,
	}
}

// Primary returns the underlying primary provider.
func (f *FallbackProvider) Primary() domain.LLMProvider {
	return f.primary
}

// Secondary returns the underlying fallback provider.
func (f *FallbackProvider) Secondary() domain.LLMProvider {
	return f.secondary
}

func (f *FallbackProvider) isPrimaryInCooldown() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return time.Now().Before(f.primaryDownUntil)
}

func (f *FallbackProvider) handlePrimaryFailure(err error) {
	if isBillingOrDepletedError(err) {
		f.mu.Lock()
		f.primaryDownUntil = time.Now().Add(5 * time.Minute)
		f.mu.Unlock()
		log.Printf("⚠️ Provedor principal (%s) com créditos esgotados/erro de faturamento: %v. Alternando para (%s) com cooldown de 5 minutos.", f.primaryName, err, f.secondaryName)
		return
	}
	log.Printf("⚠️ Provedor LLM principal (%s) falhou: %v. Acionando fallback automático (%s)...", f.primaryName, err, f.secondaryName)
}

func isBillingOrDepletedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "402") ||
		strings.Contains(msg, "depleted") ||
		strings.Contains(msg, "prepayment") ||
		strings.Contains(msg, "billing") ||
		strings.Contains(msg, "resource_exhausted")
}

// ExecutePrompt attempts execution with primary; if in cooldown or fails, tries secondary.
func (f *FallbackProvider) ExecutePrompt(ctx context.Context, prompt string) (string, error) {
	if !f.isPrimaryInCooldown() {
		resp, err := f.primary.ExecutePrompt(ctx, prompt)
		if err == nil {
			return resp, nil
		}
		f.handlePrimaryFailure(err)
	}

	return f.secondary.ExecutePrompt(ctx, prompt)
}

// ExecuteStructured attempts structured execution with primary; if in cooldown or fails, tries secondary.
func (f *FallbackProvider) ExecuteStructured(ctx context.Context, prompt string, schema string, target any) error {
	if !f.isPrimaryInCooldown() {
		err := f.primary.ExecuteStructured(ctx, prompt, schema, target)
		if err == nil {
			return nil
		}
		f.handlePrimaryFailure(err)
	}

	return f.secondary.ExecuteStructured(ctx, prompt, schema, target)
}
