package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"anki-wrapper/internal/domain"
)

var _ domain.TelemetryRepository = (*TelemetryRepo)(nil)

// TelemetryRepo implements domain.TelemetryRepository for SQLite.
type TelemetryRepo struct {
	db *sql.DB
}

// NewTelemetryRepo creates a new SQLite Telemetry repository.
func NewTelemetryRepo(db *sql.DB) *TelemetryRepo {
	return &TelemetryRepo{db: db}
}

// RecordUsage inserts a new LLM token telemetry record.
func (r *TelemetryRepo) RecordUsage(ctx context.Context, rec domain.LLMUsageRecord) error {
	query := `
	INSERT INTO llm_usage (
		provider, model, operation, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, duration_ms, status, error_msg, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := rec.CreatedAt
	if now.IsZero() {
		now = time.Now()
	}

	_, err := r.db.ExecContext(ctx, query,
		rec.Provider,
		rec.Model,
		rec.Operation,
		rec.PromptTokens,
		rec.OutputTokens,
		rec.ThinkingTokens,
		rec.CachedTokens,
		rec.TotalTokens,
		rec.DurationMs,
		rec.Status,
		rec.ErrorMsg,
		now,
	)
	if err != nil {
		return fmt.Errorf("falha ao salvar registro de telemetria LLM: %w", err)
	}
	return nil
}

// GetUsageStats calculates aggregated token telemetry and usage metrics.
func (r *TelemetryRepo) GetUsageStats(ctx context.Context) (*domain.LLMUsageStats, error) {
	stats := &domain.LLMUsageStats{
		CallsByProvider:   make(map[string]int),
		CallsByOperation:  make(map[string]int),
		TokensByOperation: make(map[string]int64),
	}

	// 1. Overall metrics
	row := r.db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*),
			COALESCE(SUM(prompt_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(thinking_tokens), 0),
			COALESCE(SUM(cached_tokens), 0),
			COALESCE(SUM(total_tokens), 0),
			COALESCE(AVG(duration_ms), 0)
		FROM llm_usage
	`)

	var avgDur float64
	if err := row.Scan(
		&stats.TotalCalls,
		&stats.TotalPromptTokens,
		&stats.TotalOutputTokens,
		&stats.TotalThinkingTokens,
		&stats.TotalCachedTokens,
		&stats.TotalTokens,
		&avgDur,
	); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("falha ao consultar totais de telemetria: %w", err)
	}
	stats.AvgDurationMs = int64(avgDur)

	// 2. Calls by provider
	rowsProv, err := r.db.QueryContext(ctx, `SELECT provider, COUNT(*) FROM llm_usage GROUP BY provider`)
	if err == nil {
		defer rowsProv.Close()
		for rowsProv.Next() {
			var provider string
			var count int
			if err := rowsProv.Scan(&provider, &count); err == nil {
				stats.CallsByProvider[provider] = count
			}
		}
	}

	// 3. Calls and tokens by operation
	rowsOp, err := r.db.QueryContext(ctx, `
		SELECT operation, COUNT(*), COALESCE(SUM(total_tokens), 0)
		FROM llm_usage 
		GROUP BY operation 
		ORDER BY SUM(total_tokens) DESC
	`)
	if err == nil {
		defer rowsOp.Close()
		for rowsOp.Next() {
			var op string
			var count int
			var tokens int64
			if err := rowsOp.Scan(&op, &count, &tokens); err == nil {
				stats.CallsByOperation[op] = count
				stats.TokensByOperation[op] = tokens
			}
		}
	}

	return stats, nil
}

// GetRecentUsage returns the latest N records ordered by created_at descending.
func (r *TelemetryRepo) GetRecentUsage(ctx context.Context, limit int) ([]domain.LLMUsageRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
	SELECT id, provider, model, operation, prompt_tokens, output_tokens, thinking_tokens, cached_tokens, total_tokens, duration_ms, status, COALESCE(error_msg, ''), created_at
	FROM llm_usage
	ORDER BY created_at DESC
	LIMIT ?`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("falha ao consultar histórico de telemetria: %w", err)
	}
	defer rows.Close()

	var records []domain.LLMUsageRecord
	for rows.Next() {
		var rec domain.LLMUsageRecord
		if err := rows.Scan(
			&rec.ID,
			&rec.Provider,
			&rec.Model,
			&rec.Operation,
			&rec.PromptTokens,
			&rec.OutputTokens,
			&rec.ThinkingTokens,
			&rec.CachedTokens,
			&rec.TotalTokens,
			&rec.DurationMs,
			&rec.Status,
			&rec.ErrorMsg,
			&rec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("falha ao escanear linha de telemetria: %w", err)
		}
		records = append(records, rec)
	}

	return records, nil
}

// ClearUsage removes all telemetry records.
func (r *TelemetryRepo) ClearUsage(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM llm_usage`)
	if err != nil {
		return fmt.Errorf("falha ao limpar telemetria: %w", err)
	}
	return nil
}
