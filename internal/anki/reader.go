package anki

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// DefaultPaths returns a list of standard locations where Anki collection.anki2 might reside.
func DefaultPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	return []string{
		// Flatpak Anki (detected on this system)
		filepath.Join(home, ".var", "app", "net.ankiweb.Anki", "data", "Anki2", "User 1", "collection.anki2"),
		// Standard Linux native Anki
		filepath.Join(home, ".local", "share", "Anki2", "User 1", "collection.anki2"),
		// Alternative default profiles
		filepath.Join(home, ".var", "app", "net.ankiweb.Anki", "data", "Anki2", "Default", "collection.anki2"),
		filepath.Join(home, ".local", "share", "Anki2", "Default", "collection.anki2"),
	}
}

// FindAnkiCollection returns the first existing path from DefaultPaths or custom environment variable.
func FindAnkiCollection() (string, error) {
	if custom := os.Getenv("ANKI_COLLECTION_PATH"); custom != "" {
		if _, err := os.Stat(custom); err == nil {
			return custom, nil
		}
	}

	for _, p := range DefaultPaths() {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("nenhum banco de dados do Anki (collection.anki2) encontrado nos caminhos padrão")
}

// ReadAnkiVocab reads all vocabulary notes from the Anki SQLite database in read-only mode.
func ReadAnkiVocab(collectionPath string) ([]VocabItem, error) {
	if collectionPath == "" {
		var err error
		collectionPath, err = FindAnkiCollection()
		if err != nil {
			return nil, err
		}
	}

	// Open SQLite strictly in read-only mode
	// URI: file:/path/to/collection.anki2?mode=ro&immutable=1 (or ?mode=ro)
	dsn := fmt.Sprintf("file:%s?mode=ro", collectionPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir banco do Anki: %w", err)
	}
	defer db.Close()

	// Query notes joined with cards and decks to get exact deck information
	queryWithDecks := `
		SELECT n.id, COALESCE(c.did, 1), COALESCE(d.name, 'Default'), n.flds, n.tags
		FROM notes n
		LEFT JOIN cards c ON n.id = c.nid
		LEFT JOIN decks d ON c.did = d.id
		WHERE n.flds != ''
		GROUP BY n.id
		ORDER BY n.id ASC
	`
	rows, err := db.Query(queryWithDecks)
	hasDeckColumns := true
	if err != nil {
		hasDeckColumns = false
		rows, err = db.Query(`SELECT id, flds, tags FROM notes WHERE flds != '' ORDER BY id ASC`)
		if err != nil {
			return nil, fmt.Errorf("falha ao consultar notas do Anki: %w", err)
		}
	}
	defer rows.Close()

	var items []VocabItem
	for rows.Next() {
		var id int64
		var deckID int64 = 1
		var deckName string = "Default"
		var rawFlds, tags string

		if hasDeckColumns {
			if err := rows.Scan(&id, &deckID, &deckName, &rawFlds, &tags); err != nil {
				continue
			}
		} else {
			if err := rows.Scan(&id, &rawFlds, &tags); err != nil {
				continue
			}
		}

		parts := strings.Split(rawFlds, "\x1f")
		if len(parts) == 0 {
			continue
		}

		word := strings.TrimSpace(parts[0])
		if word == "" {
			continue
		}

		meaning := ""
		reading := ""
		sentence := ""

		if len(parts) > 1 {
			meaning = SanitizeMeaning(parts[1])
		}
		if len(parts) > 2 {
			reading = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 {
			sentence = strings.TrimSpace(parts[3])
		}

		items = append(items, VocabItem{
			ID:       id,
			DeckID:   deckID,
			DeckName: deckName,
			Word:     word,
			Reading:  reading,
			Meaning:  meaning,
			Sentence: sentence,
			Tags:     strings.TrimSpace(tags),
		})
	}

	return items, nil
}
