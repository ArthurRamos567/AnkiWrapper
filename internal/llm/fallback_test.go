package llm

import (
	"context"
	"errors"
	"testing"
)

type mockProvider struct {
	promptFunc     func(ctx context.Context, prompt string) (string, error)
	structuredFunc func(ctx context.Context, prompt, schema string, target any) error
}

func (m *mockProvider) ExecutePrompt(ctx context.Context, prompt string) (string, error) {
	if m.promptFunc != nil {
		return m.promptFunc(ctx, prompt)
	}
	return "ok", nil
}

func (m *mockProvider) ExecuteStructured(ctx context.Context, prompt, schema string, target any) error {
	if m.structuredFunc != nil {
		return m.structuredFunc(ctx, prompt, schema, target)
	}
	return nil
}

func TestFallbackProvider_PromptSuccessPrimary(t *testing.T) {
	primaryCalled := false
	secondaryCalled := false

	p := &mockProvider{
		promptFunc: func(ctx context.Context, prompt string) (string, error) {
			primaryCalled = true
			return "primary response", nil
		},
	}
	s := &mockProvider{
		promptFunc: func(ctx context.Context, prompt string) (string, error) {
			secondaryCalled = true
			return "secondary response", nil
		},
	}

	fb := NewFallbackProvider(p, "mock-primary", s, "mock-secondary")
	res, err := fb.ExecutePrompt(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "primary response" {
		t.Errorf("expected 'primary response', got %s", res)
	}
	if !primaryCalled || secondaryCalled {
		t.Errorf("primaryCalled=%v, secondaryCalled=%v", primaryCalled, secondaryCalled)
	}
}

func TestFallbackProvider_PromptFallback(t *testing.T) {
	primaryCalled := false
	secondaryCalled := false

	p := &mockProvider{
		promptFunc: func(ctx context.Context, prompt string) (string, error) {
			primaryCalled = true
			return "", errors.New("credits depleted")
		},
	}
	s := &mockProvider{
		promptFunc: func(ctx context.Context, prompt string) (string, error) {
			secondaryCalled = true
			return "secondary response", nil
		},
	}

	fb := NewFallbackProvider(p, "mock-primary", s, "mock-secondary")
	res, err := fb.ExecutePrompt(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "secondary response" {
		t.Errorf("expected 'secondary response', got %s", res)
	}
	if !primaryCalled || !secondaryCalled {
		t.Errorf("expected both called, primaryCalled=%v, secondaryCalled=%v", primaryCalled, secondaryCalled)
	}
}

func TestFallbackProvider_StructuredFallback(t *testing.T) {
	primaryCalled := false
	secondaryCalled := false

	p := &mockProvider{
		structuredFunc: func(ctx context.Context, prompt, schema string, target any) error {
			primaryCalled = true
			return errors.New("402 Payment Required")
		},
	}
	s := &mockProvider{
		structuredFunc: func(ctx context.Context, prompt, schema string, target any) error {
			secondaryCalled = true
			if ptr, ok := target.(*string); ok {
				*ptr = "recovered"
			}
			return nil
		},
	}

	fb := NewFallbackProvider(p, "mock-primary", s, "mock-secondary")
	var out string
	err := fb.ExecuteStructured(context.Background(), "test", "{}", &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "recovered" {
		t.Errorf("expected 'recovered', got %s", out)
	}
	if !primaryCalled || !secondaryCalled {
		t.Errorf("expected both called, primaryCalled=%v, secondaryCalled=%v", primaryCalled, secondaryCalled)
	}
}

func TestFallbackProvider_CircuitBreakerCooldown(t *testing.T) {
	primaryCount := 0
	secondaryCount := 0

	p := &mockProvider{
		promptFunc: func(ctx context.Context, prompt string) (string, error) {
			primaryCount++
			return "", errors.New("402 PAYMENT_REQUIRED: prepayment credits are depleted")
		},
	}
	s := &mockProvider{
		promptFunc: func(ctx context.Context, prompt string) (string, error) {
			secondaryCount++
			return "fallback ok", nil
		},
	}

	fb := NewFallbackProvider(p, "primary", s, "secondary")

	// Call 1: primary fails with 402, triggers cooldown, secondary handles
	res1, err := fb.ExecutePrompt(context.Background(), "test1")
	if err != nil || res1 != "fallback ok" {
		t.Fatalf("call 1 failed: res=%s, err=%v", res1, err)
	}
	if primaryCount != 1 || secondaryCount != 1 {
		t.Fatalf("call 1: primaryCount=%d, secondaryCount=%d", primaryCount, secondaryCount)
	}

	// Call 2: should be in cooldown, primary is NOT called at all!
	res2, err := fb.ExecutePrompt(context.Background(), "test2")
	if err != nil || res2 != "fallback ok" {
		t.Fatalf("call 2 failed: res=%s, err=%v", res2, err)
	}
	if primaryCount != 1 || secondaryCount != 2 {
		t.Fatalf("call 2 in cooldown: primaryCount=%d (expected 1), secondaryCount=%d (expected 2)", primaryCount, secondaryCount)
	}
}

