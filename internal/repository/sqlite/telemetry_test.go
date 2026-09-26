package sqlite

import (
	"context"
	"testing"
	"time"

	"anki-wrapper/internal/domain"
)

func TestTelemetryRepo(t *testing.T) {
	ctx := context.Background()

	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir SQLite em memória: %v", err)
	}
	defer db.Close()

	repo := NewTelemetryRepo(db.DB)

	// Initially stats should be empty
	stats, err := repo.GetUsageStats(ctx)
	if err != nil {
		t.Fatalf("GetUsageStats inicial falhou: %v", err)
	}
	if stats.TotalCalls != 0 || stats.TotalTokens != 0 {
		t.Fatalf("esperado 0 chamadas e 0 tokens, obtido %+v", stats)
	}

	// Insert record 1
	rec1 := domain.LLMUsageRecord{
		Provider:       "gemini",
		Model:          "gemini-2.5-flash",
		Operation:      "quiz_generate",
		PromptTokens:   100,
		OutputTokens:   50,
		ThinkingTokens: 20,
		CachedTokens:   10,
		TotalTokens:    150,
		DurationMs:     1200,
		Status:         "SUCCESS",
		CreatedAt:      time.Now().Add(-2 * time.Minute),
	}
	if err := repo.RecordUsage(ctx, rec1); err != nil {
		t.Fatalf("RecordUsage 1 falhou: %v", err)
	}

	// Insert record 2
	rec2 := domain.LLMUsageRecord{
		Provider:       "gemini",
		Model:          "gemini-2.5-flash",
		Operation:      "chat_reply",
		PromptTokens:   200,
		OutputTokens:   80,
		ThinkingTokens: 0,
		CachedTokens:   0,
		TotalTokens:    280,
		DurationMs:     800,
		Status:         "SUCCESS",
		CreatedAt:      time.Now().Add(-1 * time.Minute),
	}
	if err := repo.RecordUsage(ctx, rec2); err != nil {
		t.Fatalf("RecordUsage 2 falhou: %v", err)
	}

	// Insert record 3 (error record from agy)
	rec3 := domain.LLMUsageRecord{
		Provider:       "agy",
		Model:          "claude-3-7-sonnet",
		Operation:      "chat_reply",
		PromptTokens:   50,
		OutputTokens:   0,
		ThinkingTokens: 0,
		CachedTokens:   0,
		TotalTokens:    50,
		DurationMs:     3000,
		Status:         "ERROR",
		ErrorMsg:       "timeout",
		CreatedAt:      time.Now(),
	}
	if err := repo.RecordUsage(ctx, rec3); err != nil {
		t.Fatalf("RecordUsage 3 falhou: %v", err)
	}

	// Check aggregated stats
	stats, err = repo.GetUsageStats(ctx)
	if err != nil {
		t.Fatalf("GetUsageStats falhou: %v", err)
	}

	if stats.TotalCalls != 3 {
		t.Errorf("esperado TotalCalls=3, obtido %d", stats.TotalCalls)
	}
	if stats.TotalPromptTokens != 350 {
		t.Errorf("esperado TotalPromptTokens=350, obtido %d", stats.TotalPromptTokens)
	}
	if stats.TotalOutputTokens != 130 {
		t.Errorf("esperado TotalOutputTokens=130, obtido %d", stats.TotalOutputTokens)
	}
	if stats.TotalThinkingTokens != 20 {
		t.Errorf("esperado TotalThinkingTokens=20, obtido %d", stats.TotalThinkingTokens)
	}
	if stats.TotalCachedTokens != 10 {
		t.Errorf("esperado TotalCachedTokens=10, obtido %d", stats.TotalCachedTokens)
	}
	if stats.TotalTokens != 480 {
		t.Errorf("esperado TotalTokens=480, obtido %d", stats.TotalTokens)
	}
	// Average latency: (1200 + 800 + 3000) / 3 = 1666.66 => ~1666
	if stats.AvgDurationMs < 1660 || stats.AvgDurationMs > 1670 {
		t.Errorf("esperado AvgDurationMs ~1666, obtido %d", stats.AvgDurationMs)
	}

	// By provider
	if stats.CallsByProvider["gemini"] != 2 {
		t.Errorf("esperado CallsByProvider[gemini]=2, obtido %d", stats.CallsByProvider["gemini"])
	}
	if stats.CallsByProvider["agy"] != 1 {
		t.Errorf("esperado CallsByProvider[agy]=1, obtido %d", stats.CallsByProvider["agy"])
	}

	// By operation
	if stats.CallsByOperation["chat_reply"] != 2 {
		t.Errorf("esperado CallsByOperation[chat_reply]=2, obtido %d", stats.CallsByOperation["chat_reply"])
	}
	if stats.CallsByOperation["quiz_generate"] != 1 {
		t.Errorf("esperado CallsByOperation[quiz_generate]=1, obtido %d", stats.CallsByOperation["quiz_generate"])
	}
	if stats.TokensByOperation["chat_reply"] != 330 {
		t.Errorf("esperado TokensByOperation[chat_reply]=330, obtido %d", stats.TokensByOperation["chat_reply"])
	}
	if stats.TokensByOperation["quiz_generate"] != 150 {
		t.Errorf("esperado TokensByOperation[quiz_generate]=150, obtido %d", stats.TokensByOperation["quiz_generate"])
	}

	// Check recent usage
	recent, err := repo.GetRecentUsage(ctx, 2)
	if err != nil {
		t.Fatalf("GetRecentUsage falhou: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("esperado 2 registros recentes, obtido %d", len(recent))
	}
	// Order is newest first
	if recent[0].Provider != "agy" {
		t.Errorf("esperado primeiro recente ser agy, obtido %s", recent[0].Provider)
	}
	if recent[0].ErrorMsg != "timeout" {
		t.Errorf("esperado erro 'timeout', obtido %s", recent[0].ErrorMsg)
	}

	// Test clear usage
	if err := repo.ClearUsage(ctx); err != nil {
		t.Fatalf("ClearUsage falhou: %v", err)
	}
	statsAfter, err := repo.GetUsageStats(ctx)
	if err != nil {
		t.Fatalf("GetUsageStats após clear falhou: %v", err)
	}
	if statsAfter.TotalCalls != 0 || statsAfter.TotalTokens != 0 {
		t.Errorf("esperado 0 registros após clear, obtido %+v", statsAfter)
	}
}
