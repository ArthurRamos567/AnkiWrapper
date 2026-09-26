package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"anki-wrapper/internal/domain"
)

// PromptRepo implements domain.PromptRepository for SQLite.
type PromptRepo struct {
	db *sql.DB
}

// NewPromptRepo creates a new PromptRepo.
func NewPromptRepo(db *sql.DB) *PromptRepo {
	return &PromptRepo{db: db}
}

func (r *PromptRepo) GetAllPrompts(ctx context.Context) ([]domain.PromptItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT key, title, description, content, updated_at FROM prompts ORDER BY key ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.PromptItem
	for rows.Next() {
		var item domain.PromptItem
		if err := rows.Scan(&item.Key, &item.Title, &item.Description, &item.Content, &item.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, item)
	}
	return list, nil
}

func (r *PromptRepo) GetPrompt(ctx context.Context, key string) (string, error) {
	var content string
	err := r.db.QueryRowContext(ctx, `SELECT content FROM prompts WHERE key = ?`, key).Scan(&content)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("prompt '%s' não encontrado", key)
	}
	return content, err
}

func (r *PromptRepo) UpdatePrompt(ctx context.Context, key, content string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE prompts SET content = ?, updated_at = CURRENT_TIMESTAMP WHERE key = ?`, content, key)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("nenhum prompt encontrado com a chave '%s'", key)
	}
	return nil
}

func (r *PromptRepo) GetSetting(ctx context.Context, key, defaultValue string) string {
	var val string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = ?`, key).Scan(&val)
	if err != nil || val == "" {
		return defaultValue
	}
	return val
}

func (r *PromptRepo) SetSetting(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?`, key, value, value)
	return err
}
