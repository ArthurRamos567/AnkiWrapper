package sqlite

import (
	"context"
	"testing"

	"anki-wrapper/internal/domain"
)

func TestGrammarRepo(t *testing.T) {
	ctx := context.Background()

	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("falha ao abrir banco em memória: %v", err)
	}
	defer db.Close()

	repo, err := NewGrammarRepo(db.DB)
	if err != nil {
		t.Fatalf("falha ao inicializar GrammarRepo: %v", err)
	}

	if repo.Count(ctx) != 0 {
		t.Errorf("esperado 0 lições inicialmente, obtido %d", repo.Count(ctx))
	}

	lessons := []domain.GrammarLesson{
		{
			ID:            "basics/noun-related-particles",
			Title:         "Noun-related Particles",
			ChapterNumber: 3,
			Chapter:       "Basic Grammar",
			Part:          11,
			SectionKey:    "3.11",
			Slug:          "noun-related-particles",
			URL:           "https://guidetojapanese.org/learn/grammar/nounparticles",
			WebURL:        "https://jp.saeris.gg/learn/grammar/basics/noun-related-particles",
			FilePath:      "basics/noun-related-particles.md",
			Content:       "The last three particles: to, ya, toka.",
		},
		{
			ID:            "essentials/conditionals",
			Title:         "Conditionals",
			ChapterNumber: 4,
			Chapter:       "Essential Grammar",
			Part:          9,
			SectionKey:    "4.9",
			Slug:          "conditionals",
			URL:           "https://guidetojapanese.org/learn/grammar/conditionals",
			WebURL:        "https://jp.saeris.gg/learn/grammar/essentials/conditionals",
			FilePath:      "essentials/conditionals.md",
			Content:       "How to say if in Japanese using tara, ba, to, nara.",
		},
	}

	synced, err := repo.SyncLessons(ctx, lessons)
	if err != nil {
		t.Fatalf("SyncLessons falhou: %v", err)
	}
	if synced != 2 {
		t.Errorf("esperado 2 sincronizados, obtido %d", synced)
	}

	if repo.Count(ctx) != 2 {
		t.Errorf("esperado Count=2, obtido %d", repo.Count(ctx))
	}

	// Test GetLessonBySection
	l1, err := repo.GetLessonBySection(ctx, "3.11")
	if err != nil {
		t.Fatalf("GetLessonBySection falhou: %v", err)
	}
	if l1.Title != "Noun-related Particles" {
		t.Errorf("esperado Noun-related Particles, obtido %s", l1.Title)
	}
	if l1.Reference == nil || l1.Reference.URL != "https://guidetojapanese.org/learn/grammar/nounparticles" {
		t.Errorf("referência ausente ou incorreta: %+v", l1.Reference)
	}

	// Test SearchLessons with section key
	matches, err := repo.SearchLessons(ctx, "Tae Kim 4.9", 2)
	if err != nil {
		t.Fatalf("SearchLessons falhou: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("esperado resultado para 'Tae Kim 4.9', obtido 0")
	}
	if matches[0].SectionKey != "4.9" {
		t.Errorf("esperado topo do resultado ser 4.9, obtido %s", matches[0].SectionKey)
	}

	// Test SearchLessons with topic keyword
	matches, err = repo.SearchLessons(ctx, "partículas", 2)
	if err != nil {
		t.Fatalf("SearchLessons falhou: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("esperado resultado para 'partículas', obtido 0")
	}
	if matches[0].SectionKey != "3.11" {
		t.Errorf("esperado lição 3.11 para 'partículas', obtido %s", matches[0].SectionKey)
	}

	// Test read status defaults
	if repo.CountRead(ctx) != 0 {
		t.Errorf("esperado 0 lições lidas inicialmente, obtido %d", repo.CountRead(ctx))
	}
	readList, err := repo.GetReadLessons(ctx)
	if err != nil || len(readList) != 0 {
		t.Errorf("esperado lista vazia de lidos, obtido %d", len(readList))
	}

	// SearchReadLessons should return nil when none read
	readMatches, err := repo.SearchReadLessons(ctx, "partículas", 2)
	if err != nil {
		t.Fatalf("SearchReadLessons falhou: %v", err)
	}
	if len(readMatches) != 0 {
		t.Errorf("esperado 0 resultados lidos, obtido %d", len(readMatches))
	}

	// Mark 3.11 as read
	if err := repo.MarkLessonRead(ctx, "basics/noun-related-particles", true); err != nil {
		t.Fatalf("MarkLessonRead falhou: %v", err)
	}

	if repo.CountRead(ctx) != 1 {
		t.Errorf("esperado 1 lição lida, obtido %d", repo.CountRead(ctx))
	}

	l1Updated, err := repo.GetLessonBySection(ctx, "3.11")
	if err != nil || !l1Updated.IsRead {
		t.Errorf("esperado l1Updated.IsRead == true")
	}

	// SearchReadLessons should now find 3.11
	readMatches, err = repo.SearchReadLessons(ctx, "partículas", 2)
	if err != nil || len(readMatches) == 0 {
		t.Fatalf("esperado encontrar 3.11 em SearchReadLessons")
	}
	if readMatches[0].SectionKey != "3.11" {
		t.Errorf("esperado 3.11, obtido %s", readMatches[0].SectionKey)
	}

	// SearchReadLessons for unread 4.9 should NOT return 4.9
	readMatches49, err := repo.SearchReadLessons(ctx, "conditionals", 2)
	if err != nil {
		t.Fatalf("SearchReadLessons falhou: %v", err)
	}
	if len(readMatches49) != 0 {
		t.Errorf("esperado 0 resultados para conditionals não lido, obtido %d", len(readMatches49))
	}

	// Unmark 3.11 as read
	if err := repo.MarkLessonRead(ctx, "basics/noun-related-particles", false); err != nil {
		t.Fatalf("MarkLessonRead(false) falhou: %v", err)
	}
	if repo.CountRead(ctx) != 0 {
		t.Errorf("esperado 0 lições lidas após desmarcar, obtido %d", repo.CountRead(ctx))
	}
}

