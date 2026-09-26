package sqlite

import (
	"context"
	"database/sql"

	"anki-wrapper/internal/domain"
)

// ChatRepo implements domain.ChatRepository for SQLite.
type ChatRepo struct {
	db *sql.DB
}

// NewChatRepo creates a new ChatRepo.
func NewChatRepo(db *sql.DB) *ChatRepo {
	return &ChatRepo{db: db}
}

func (r *ChatRepo) CreateSession(ctx context.Context, id, title string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO chat_sessions (id, title, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, id, title)
	return err
}

func (r *ChatRepo) GetSessions(ctx context.Context) ([]domain.ChatSession, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, title, created_at, updated_at FROM chat_sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []domain.ChatSession
	for rows.Next() {
		var s domain.ChatSession
		if err := rows.Scan(&s.ID, &s.Title, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (r *ChatRepo) UpdateSessionTitle(ctx context.Context, id, title string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE chat_sessions SET title = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, title, id)
	return err
}

func (r *ChatRepo) DeleteSession(ctx context.Context, id string) error {
	_, _ = r.db.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ?`, id)
	_, err := r.db.ExecContext(ctx, `DELETE FROM chat_sessions WHERE id = ?`, id)
	return err
}

func (r *ChatRepo) SaveChatMessage(ctx context.Context, m domain.ChatMessage) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO chat_messages (session_id, role, content, furigana_html, translation, pedagogical_note)
		VALUES (?, ?, ?, ?, ?, ?)
	`, m.SessionID, m.Role, m.Content, m.FuriganaHTML, m.Translation, m.PedagogicalNote)
	if err == nil {
		_, _ = r.db.ExecContext(ctx, `UPDATE chat_sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, m.SessionID)
	}
	return err
}

func (r *ChatRepo) GetChatMessages(ctx context.Context, sessionID string) ([]domain.ChatMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, session_id, role, content, furigana_html, translation, pedagogical_note, created_at
		FROM chat_messages
		WHERE session_id = ?
		ORDER BY id ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []domain.ChatMessage
	for rows.Next() {
		var m domain.ChatMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.FuriganaHTML, &m.Translation, &m.PedagogicalNote, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

func (r *ChatRepo) ClearSessionMessages(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ?`, sessionID)
	return err
}
