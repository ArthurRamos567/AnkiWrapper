package sqlite

import (
	"context"
	"database/sql"

	"anki-wrapper/internal/domain"
)

// QuizRepo implements domain.QuizRepository for SQLite.
type QuizRepo struct {
	db *sql.DB
}

// NewQuizRepo creates a new QuizRepo.
func NewQuizRepo(db *sql.DB) *QuizRepo {
	return &QuizRepo{db: db}
}

func (r *QuizRepo) SaveQuizRecord(ctx context.Context, rec domain.QuizRecord) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO quiz_history (mode, topic, question, user_answer, correct_answer, is_correct, explanation)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, rec.Mode, rec.Topic, rec.Question, rec.UserAnswer, rec.CorrectAnswer, rec.IsCorrect, rec.Explanation)
	return err
}

func (r *QuizRepo) GetRecentQuizzes(ctx context.Context, limit int) ([]domain.QuizRecord, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, mode, topic, question, user_answer, correct_answer, is_correct, explanation, created_at
		FROM quiz_history
		ORDER BY id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []domain.QuizRecord
	for rows.Next() {
		var q domain.QuizRecord
		if err := rows.Scan(&q.ID, &q.Mode, &q.Topic, &q.Question, &q.UserAnswer, &q.CorrectAnswer, &q.IsCorrect, &q.Explanation, &q.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, q)
	}
	return records, nil
}

func (r *QuizRepo) GetQuizStats(ctx context.Context) (*domain.QuizStats, error) {
	stats := &domain.QuizStats{}
	err := r.db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*),
			COALESCE(SUM(CASE WHEN is_correct = 1 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN is_correct = 0 THEN 1 ELSE 0 END), 0)
		FROM quiz_history
	`).Scan(&stats.Total, &stats.Correct, &stats.Incorrect)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if stats.Total > 0 {
		stats.Accuracy = float64(stats.Correct) / float64(stats.Total) * 100
	}

	// Calculate recent streak of correct answers
	rows, err := r.db.QueryContext(ctx, `SELECT is_correct FROM quiz_history ORDER BY id DESC LIMIT 50`)
	if err == nil {
		defer rows.Close()
		streak := 0
		for rows.Next() {
			var isCorrect bool
			if err := rows.Scan(&isCorrect); err == nil {
				if isCorrect {
					streak++
				} else {
					break
				}
			}
		}
		stats.Streak = streak
	}

	return stats, nil
}
