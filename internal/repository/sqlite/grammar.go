package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"

	"anki-wrapper/internal/domain"
	"anki-wrapper/internal/rag"
)

var _ domain.GrammarRepository = (*GrammarRepo)(nil)

// GrammarRepo implements domain.GrammarRepository using SQLite and in-memory semantic search.
type GrammarRepo struct {
	db *sql.DB
	mu sync.RWMutex

	cachedLessons []domain.GrammarLesson
	cachedVectors [][]float32
}

// NewGrammarRepo creates and initializes a new GrammarRepo.
func NewGrammarRepo(db *sql.DB) (*GrammarRepo, error) {
	gr := &GrammarRepo{db: db}
	if err := gr.loadCache(); err != nil {
		return nil, fmt.Errorf("falha ao carregar cache de gramática: %w", err)
	}
	return gr, nil
}

func (gr *GrammarRepo) loadCache() error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	rows, err := gr.db.Query(`
		SELECT g.id, g.title, g.chapter_number, g.chapter, g.part, g.section_key, g.slug, g.url, g.web_url, g.file_path, g.content, g.embedding,
		       CASE WHEN r.lesson_id IS NOT NULL THEN 1 ELSE 0 END AS is_read
		FROM grammar_lessons g
		LEFT JOIN lesson_reads r ON g.id = r.lesson_id
		ORDER BY g.chapter_number ASC, g.part ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var lessons []domain.GrammarLesson
	var vectors [][]float32

	for rows.Next() {
		var l domain.GrammarLesson
		var blob []byte
		var isReadInt int

		if err := rows.Scan(
			&l.ID,
			&l.Title,
			&l.ChapterNumber,
			&l.Chapter,
			&l.Part,
			&l.SectionKey,
			&l.Slug,
			&l.URL,
			&l.WebURL,
			&l.FilePath,
			&l.Content,
			&blob,
			&isReadInt,
		); err != nil {
			continue
		}

		l.IsRead = (isReadInt == 1)
		l.Reference = &domain.GrammarReference{
			ID:         l.ID,
			Title:      l.Title,
			SectionKey: l.SectionKey,
			Chapter:    l.Chapter,
			URL:        l.URL,
			WebURL:     l.WebURL,
		}

		lessons = append(lessons, l)
		vectors = append(vectors, rag.DecodeVector(blob))
	}

	gr.cachedLessons = lessons
	gr.cachedVectors = vectors
	return nil
}

// SyncLessons saves a batch of lessons into SQLite, computing embeddings if needed.
func (gr *GrammarRepo) SyncLessons(ctx context.Context, lessons []domain.GrammarLesson) (int, error) {
	if len(lessons) == 0 {
		return 0, nil
	}

	tx, err := gr.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO grammar_lessons (
			id, title, chapter_number, chapter, part, section_key, slug, url, web_url, file_path, content, embedding, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title,
			chapter_number = excluded.chapter_number,
			chapter = excluded.chapter,
			part = excluded.part,
			section_key = excluded.section_key,
			slug = excluded.slug,
			url = excluded.url,
			web_url = excluded.web_url,
			file_path = excluded.file_path,
			content = excluded.content,
			embedding = excluded.embedding,
			updated_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for _, l := range lessons {
		textToEmbed := fmt.Sprintf("%s %s %s %s", l.Title, l.Chapter, l.SectionKey, l.Snippet(400))
		vec := rag.EmbedText(textToEmbed)
		blob := rag.EncodeVector(vec)

		if _, err := stmt.ExecContext(ctx,
			l.ID,
			l.Title,
			l.ChapterNumber,
			l.Chapter,
			l.Part,
			l.SectionKey,
			l.Slug,
			l.URL,
			l.WebURL,
			l.FilePath,
			l.Content,
			blob,
		); err != nil {
			return 0, fmt.Errorf("falha ao salvar lição %s: %w", l.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	_ = gr.loadCache()
	return len(lessons), nil
}

type scoredLesson struct {
	lesson domain.GrammarLesson
	score  float32
}

// SearchLessons performs hybrid search (exact section match, title/slug match, semantic vector similarity).
func (gr *GrammarRepo) SearchLessons(ctx context.Context, query string, topK int) ([]domain.GrammarLesson, error) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	if len(gr.cachedLessons) == 0 || strings.TrimSpace(query) == "" {
		return nil, nil
	}

	if topK <= 0 {
		topK = 3
	}

	lowerQuery := strings.ToLower(query)
	queryVec := rag.EmbedText(query)

	var scored []scoredLesson

	for i, l := range gr.cachedLessons {
		var score float32

		// 1. Direct section match (e.g. "3.11", "4.9", "tae kim 3.11")
		if strings.Contains(lowerQuery, strings.ToLower(l.SectionKey)) {
			score += 10.0
		}
		if strings.Contains(lowerQuery, strings.ToLower(l.Slug)) {
			score += 5.0
		}

		// 2. Title & chapter match
		lowerTitle := strings.ToLower(l.Title)
		if strings.Contains(lowerQuery, lowerTitle) {
			score += 6.0
		}
		for _, w := range strings.Fields(lowerTitle) {
			if len(w) > 3 && strings.Contains(lowerQuery, w) {
				score += 2.0
			}
		}

		// 3. Vector similarity
		if i < len(gr.cachedVectors) {
			sim := rag.CosineSimilarity(queryVec, gr.cachedVectors[i])
			if sim > 0.15 {
				score += sim * 3.0
			}
		}

		if score > 0.3 {
			scored = append(scored, scoredLesson{
				lesson: l,
				score:  score,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	var result []domain.GrammarLesson
	for i := 0; i < len(scored) && i < topK; i++ {
		result = append(result, scored[i].lesson)
	}

	return result, nil
}

// GetLessonByID fetches a lesson by ID (e.g. "basics/noun-related-particles").
func (gr *GrammarRepo) GetLessonByID(ctx context.Context, id string) (*domain.GrammarLesson, error) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	for _, l := range gr.cachedLessons {
		if l.ID == id || l.Slug == id {
			return &l, nil
		}
	}
	return nil, fmt.Errorf("lição de gramática '%s' não encontrada", id)
}

// GetLessonBySection fetches a lesson by section number (e.g. "3.11" or "4.9").
func (gr *GrammarRepo) GetLessonBySection(ctx context.Context, section string) (*domain.GrammarLesson, error) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	cleanSec := strings.TrimSpace(section)
	for _, l := range gr.cachedLessons {
		if l.SectionKey == cleanSec {
			return &l, nil
		}
	}
	return nil, fmt.Errorf("seção do Tae Kim '%s' não encontrada", section)
}

// GetAllLessons returns all cached lessons sorted by chapter and part.
func (gr *GrammarRepo) GetAllLessons(ctx context.Context) ([]domain.GrammarLesson, error) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	out := make([]domain.GrammarLesson, len(gr.cachedLessons))
	copy(out, gr.cachedLessons)
	return out, nil
}

// Count returns the number of indexed grammar lessons.
func (gr *GrammarRepo) Count(ctx context.Context) int {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return len(gr.cachedLessons)
}

// MarkLessonRead marks a lesson as read or unread in the database and in-memory cache.
func (gr *GrammarRepo) MarkLessonRead(ctx context.Context, id string, isRead bool) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	canonicalID := id
	for _, l := range gr.cachedLessons {
		if l.ID == id || l.Slug == id {
			canonicalID = l.ID
			break
		}
	}

	if isRead {
		_, err := gr.db.ExecContext(ctx, "INSERT OR IGNORE INTO lesson_reads (lesson_id) VALUES (?)", canonicalID)
		if err != nil {
			return err
		}
	} else {
		_, err := gr.db.ExecContext(ctx, "DELETE FROM lesson_reads WHERE lesson_id = ?", canonicalID)
		if err != nil {
			return err
		}
	}

	for i := range gr.cachedLessons {
		if gr.cachedLessons[i].ID == canonicalID {
			gr.cachedLessons[i].IsRead = isRead
			break
		}
	}
	return nil
}

// GetReadLessons returns all lessons marked as read.
func (gr *GrammarRepo) GetReadLessons(ctx context.Context) ([]domain.GrammarLesson, error) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	var result []domain.GrammarLesson
	for _, l := range gr.cachedLessons {
		if l.IsRead {
			result = append(result, l)
		}
	}
	return result, nil
}

// SearchReadLessons finds relevant grammar lessons restricted strictly to those marked as read.
func (gr *GrammarRepo) SearchReadLessons(ctx context.Context, query string, topK int) ([]domain.GrammarLesson, error) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	if topK <= 0 {
		topK = 5
	}

	var readLessons []domain.GrammarLesson
	var readVectors [][]float32
	for i, l := range gr.cachedLessons {
		if l.IsRead {
			readLessons = append(readLessons, l)
			if i < len(gr.cachedVectors) {
				readVectors = append(readVectors, gr.cachedVectors[i])
			} else {
				readVectors = append(readVectors, nil)
			}
		}
	}

	if len(readLessons) == 0 {
		return nil, nil
	}

	query = strings.TrimSpace(query)
	if query == "" {
		if len(readLessons) > topK {
			return readLessons[:topK], nil
		}
		return readLessons, nil
	}

	lowerQuery := strings.ToLower(query)
	queryVec := rag.EmbedText(query)

	type scoredLesson struct {
		lesson domain.GrammarLesson
		score  float32
	}

	var scored []scoredLesson
	for i, l := range readLessons {
		var score float32

		// 1. Direct section match (e.g. "3.11", "4.9", "tae kim 3.11")
		if strings.Contains(lowerQuery, strings.ToLower(l.SectionKey)) {
			score += 10.0
		}
		if strings.Contains(lowerQuery, strings.ToLower(l.Slug)) {
			score += 5.0
		}

		// 2. Title & chapter match
		lowerTitle := strings.ToLower(l.Title)
		if strings.Contains(lowerQuery, lowerTitle) {
			score += 6.0
		}
		for _, w := range strings.Fields(lowerTitle) {
			if len(w) > 3 && strings.Contains(lowerQuery, w) {
				score += 2.0
			}
		}

		// 3. Vector similarity
		if i < len(readVectors) && readVectors[i] != nil {
			sim := rag.CosineSimilarity(queryVec, readVectors[i])
			if sim > 0.15 {
				score += sim * 3.0
			}
		}

		if score > 0.3 {
			scored = append(scored, scoredLesson{
				lesson: l,
				score:  score,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	var result []domain.GrammarLesson
	for i := 0; i < len(scored) && i < topK; i++ {
		result = append(result, scored[i].lesson)
	}

	return result, nil
}

// CountRead returns the total number of lessons marked as read.
func (gr *GrammarRepo) CountRead(ctx context.Context) int {
	gr.mu.RLock()
	defer gr.mu.RUnlock()

	count := 0
	for _, l := range gr.cachedLessons {
		if l.IsRead {
			count++
		}
	}
	return count
}

