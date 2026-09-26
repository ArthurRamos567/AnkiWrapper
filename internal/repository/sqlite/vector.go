package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"sync"

	"anki-wrapper/internal/domain"
	"anki-wrapper/internal/rag"
)

// VectorRepo implements domain.VectorRepository for SQLite.
type VectorRepo struct {
	db *sql.DB
	mu sync.RWMutex

	cachedItems   []domain.VocabItem
	cachedVectors [][]float32
}

// NewVectorRepo creates a new VectorRepo and loads cached vectors.
func NewVectorRepo(db *sql.DB) (*VectorRepo, error) {
	vr := &VectorRepo{db: db}
	if err := vr.loadCache(); err != nil {
		return nil, fmt.Errorf("falha ao carregar cache de vetores: %w", err)
	}
	return vr, nil
}

func (vr *VectorRepo) loadCache() error {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	rows, err := vr.db.Query(`
		SELECT v.anki_note_id, COALESCE(v.deck_id, 1), COALESCE(v.deck_name, 'Default'),
		       v.word, v.reading, v.meaning, v.sentence, v.tags, v.embedding,
		       CASE WHEN r.vocab_id IS NOT NULL THEN 1 ELSE 0 END AS is_read
		FROM vocab_embeddings v
		LEFT JOIN vocab_reads r ON v.anki_note_id = r.vocab_id
		ORDER BY v.anki_note_id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var items []domain.VocabItem
	var vectors [][]float32

	for rows.Next() {
		var id, deckID int64
		var deckName, word, reading, meaning, sentence, tags string
		var blob []byte
		var isReadInt int

		if err := rows.Scan(&id, &deckID, &deckName, &word, &reading, &meaning, &sentence, &tags, &blob, &isReadInt); err != nil {
			continue
		}

		items = append(items, domain.VocabItem{
			ID:       id,
			DeckID:   deckID,
			DeckName: deckName,
			Word:     word,
			Reading:  reading,
			Meaning:  meaning,
			Sentence: sentence,
			Tags:     tags,
			IsRead:   (isReadInt == 1),
		})
		vectors = append(vectors, rag.DecodeVector(blob))
	}

	vr.cachedItems = items
	vr.cachedVectors = vectors
	return nil
}

func (vr *VectorRepo) SyncVocab(ctx context.Context, items []domain.VocabItem) (int, error) {
	existingRows, err := vr.db.QueryContext(ctx, `SELECT anki_note_id, embedding FROM vocab_embeddings`)
	if err != nil {
		return 0, err
	}
	existingEmbeddings := make(map[int64][]byte)
	for existingRows.Next() {
		var id int64
		var blob []byte
		if err := existingRows.Scan(&id, &blob); err == nil {
			existingEmbeddings[id] = blob
		}
	}
	existingRows.Close()

	tx, err := vr.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	insertStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO vocab_embeddings (anki_note_id, deck_id, deck_name, word, reading, meaning, sentence, tags, embedding)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(anki_note_id) DO UPDATE SET
			deck_id = excluded.deck_id,
			deck_name = excluded.deck_name,
			word = excluded.word,
			reading = excluded.reading,
			meaning = excluded.meaning,
			sentence = excluded.sentence,
			tags = excluded.tags
	`)
	if err != nil {
		return 0, err
	}
	defer insertStmt.Close()

	newCount := 0
	for _, item := range items {
		deckID := item.DeckID
		if deckID == 0 {
			deckID = 1
		}
		deckName := item.DeckName
		if deckName == "" {
			deckName = "Default"
		}

		existingBlob, exists := existingEmbeddings[item.ID]
		var blob []byte
		if exists && len(existingBlob) > 0 {
			blob = existingBlob
		} else {
			textToEmbed := fmt.Sprintf("%s %s %s", item.Word, item.Reading, item.Meaning)
			vec := rag.EmbedText(textToEmbed)
			blob = rag.EncodeVector(vec)
			newCount++
		}

		if _, err := insertStmt.ExecContext(ctx, item.ID, deckID, deckName, item.Word, item.Reading, item.Meaning, item.Sentence, item.Tags, blob); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	_ = vr.loadCache()
	return newCount, nil
}

func (vr *VectorRepo) SearchTopK(ctx context.Context, query string, topK int, threshold float32) []domain.ScoredVocab {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	if len(vr.cachedItems) == 0 || query == "" {
		return nil
	}

	queryVec := rag.EmbedText(query)
	var matches []domain.ScoredVocab

	for i, vec := range vr.cachedVectors {
		score := rag.CosineSimilarity(queryVec, vec)
		if score >= threshold {
			matches = append(matches, domain.ScoredVocab{
				Item:  vr.cachedItems[i],
				Score: score,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > topK {
		matches = matches[:topK]
	}

	return matches
}

func (vr *VectorRepo) SearchReadTopK(ctx context.Context, query string, topK int, threshold float32) []domain.ScoredVocab {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	if len(vr.cachedItems) == 0 || query == "" {
		return nil
	}

	queryVec := rag.EmbedText(query)
	var matches []domain.ScoredVocab

	for i, vec := range vr.cachedVectors {
		if !vr.cachedItems[i].IsRead {
			continue
		}
		score := rag.CosineSimilarity(queryVec, vec)
		if score >= threshold {
			matches = append(matches, domain.ScoredVocab{
				Item:  vr.cachedItems[i],
				Score: score,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > topK {
		matches = matches[:topK]
	}

	return matches
}

func (vr *VectorRepo) Count(ctx context.Context) int {
	vr.mu.RLock()
	defer vr.mu.RUnlock()
	return len(vr.cachedItems)
}

func (vr *VectorRepo) CountRead(ctx context.Context) int {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	count := 0
	for _, it := range vr.cachedItems {
		if it.IsRead {
			count++
		}
	}
	return count
}

func (vr *VectorRepo) MarkVocabRead(ctx context.Context, id int64, isRead bool) error {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if isRead {
		_, err := vr.db.ExecContext(ctx, "INSERT OR IGNORE INTO vocab_reads (vocab_id) VALUES (?)", id)
		if err != nil {
			return err
		}
	} else {
		_, err := vr.db.ExecContext(ctx, "DELETE FROM vocab_reads WHERE vocab_id = ?", id)
		if err != nil {
			return err
		}
	}

	for i := range vr.cachedItems {
		if vr.cachedItems[i].ID == id {
			vr.cachedItems[i].IsRead = isRead
			break
		}
	}
	return nil
}

func (vr *VectorRepo) MarkDeckRead(ctx context.Context, deckName string, isRead bool) error {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if isRead {
		if deckName == "" || deckName == "all" {
			_, err := vr.db.ExecContext(ctx, `
				INSERT OR IGNORE INTO vocab_reads (vocab_id)
				SELECT anki_note_id FROM vocab_embeddings
			`)
			if err != nil {
				return err
			}
			for i := range vr.cachedItems {
				vr.cachedItems[i].IsRead = true
			}
		} else {
			_, err := vr.db.ExecContext(ctx, `
				INSERT OR IGNORE INTO vocab_reads (vocab_id)
				SELECT anki_note_id FROM vocab_embeddings WHERE deck_name = ?
			`, deckName)
			if err != nil {
				return err
			}
			for i := range vr.cachedItems {
				if vr.cachedItems[i].DeckName == deckName {
					vr.cachedItems[i].IsRead = true
				}
			}
		}
	} else {
		if deckName == "" || deckName == "all" {
			_, err := vr.db.ExecContext(ctx, "DELETE FROM vocab_reads")
			if err != nil {
				return err
			}
			for i := range vr.cachedItems {
				vr.cachedItems[i].IsRead = false
			}
		} else {
			_, err := vr.db.ExecContext(ctx, `
				DELETE FROM vocab_reads
				WHERE vocab_id IN (SELECT anki_note_id FROM vocab_embeddings WHERE deck_name = ?)
			`, deckName)
			if err != nil {
				return err
			}
			for i := range vr.cachedItems {
				if vr.cachedItems[i].DeckName == deckName {
					vr.cachedItems[i].IsRead = false
				}
			}
		}
	}
	return nil
}

func (vr *VectorRepo) GetDecks(ctx context.Context) []domain.DeckInfo {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	deckMap := make(map[string]*domain.DeckInfo)
	var deckOrder []string

	for _, item := range vr.cachedItems {
		name := item.DeckName
		if name == "" {
			name = "Default"
		}

		info, exists := deckMap[name]
		if !exists {
			info = &domain.DeckInfo{
				ID:   item.DeckID,
				Name: name,
			}
			deckMap[name] = info
			deckOrder = append(deckOrder, name)
		}

		info.TotalVocab++
		if item.IsRead {
			info.ReadVocab++
		} else {
			info.UnreadVocab++
		}
	}

	var result []domain.DeckInfo
	for _, name := range deckOrder {
		info := deckMap[name]
		if info.TotalVocab > 0 {
			info.Percentage = int(float64(info.ReadVocab) / float64(info.TotalVocab) * 100)
		}
		result = append(result, *info)
	}
	return result
}

func (vr *VectorRepo) GetAllVocab(ctx context.Context) []domain.VocabItem {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	out := make([]domain.VocabItem, len(vr.cachedItems))
	copy(out, vr.cachedItems)
	return out
}

func (vr *VectorRepo) GetVocabByDeck(ctx context.Context, deckName string) []domain.VocabItem {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	var result []domain.VocabItem
	for _, item := range vr.cachedItems {
		if deckName == "" || deckName == "all" || item.DeckName == deckName {
			result = append(result, item)
		}
	}
	return result
}

func (vr *VectorRepo) GetReadVocab(ctx context.Context) []domain.VocabItem {
	vr.mu.RLock()
	defer vr.mu.RUnlock()

	var result []domain.VocabItem
	for _, item := range vr.cachedItems {
		if item.IsRead {
			result = append(result, item)
		}
	}
	return result
}
