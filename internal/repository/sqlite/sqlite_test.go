package sqlite

import (
	"context"
	"testing"

	"anki-wrapper/internal/domain"
)

func TestRepositories(t *testing.T) {
	ctx := context.Background()

	// 1. Open in-memory SQLite
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Failed to open SQLite: %v", err)
	}
	defer db.Close()

	// 2. Test PromptRepo
	promptRepo := NewPromptRepo(db.DB)
	prompts, err := promptRepo.GetAllPrompts(ctx)
	if err != nil {
		t.Fatalf("GetAllPrompts failed: %v", err)
	}
	if len(prompts) == 0 {
		t.Fatalf("Expected seeded prompts, got 0")
	}

	err = promptRepo.UpdatePrompt(ctx, "chat_system", "Custom prompt content")
	if err != nil {
		t.Fatalf("UpdatePrompt failed: %v", err)
	}
	content, _ := promptRepo.GetPrompt(ctx, "chat_system")
	if content != "Custom prompt content" {
		t.Fatalf("Expected updated prompt, got %s", content)
	}

	// 3. Test QuizRepo
	quizRepo := NewQuizRepo(db.DB)
	err = quizRepo.SaveQuizRecord(ctx, domain.QuizRecord{
		Mode:          "multiple_choice",
		Topic:         "Partículas",
		Question:      "Teste",
		UserAnswer:    "は",
		CorrectAnswer: "は",
		IsCorrect:     true,
		Explanation:   "Explicação",
	})
	if err != nil {
		t.Fatalf("SaveQuizRecord failed: %v", err)
	}

	stats, err := quizRepo.GetQuizStats(ctx)
	if err != nil {
		t.Fatalf("GetQuizStats failed: %v", err)
	}
	if stats.Total != 1 || stats.Correct != 1 || stats.Streak != 1 {
		t.Fatalf("Unexpected stats: %+v", stats)
	}

	// 4. Test ChatRepo
	chatRepo := NewChatRepo(db.DB)
	err = chatRepo.CreateSession(ctx, "s1", "Sessão 1")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	err = chatRepo.SaveChatMessage(ctx, domain.ChatMessage{
		SessionID: "s1",
		Role:      "user",
		Content:   "Olá",
	})
	if err != nil {
		t.Fatalf("SaveChatMessage failed: %v", err)
	}
	msgs, err := chatRepo.GetChatMessages(ctx, "s1")
	if err != nil || len(msgs) != 1 {
		t.Fatalf("Expected 1 chat message, got %d (err: %v)", len(msgs), err)
	}

	// 5. Test VectorRepo
	vectorRepo, err := NewVectorRepo(db.DB)
	if err != nil {
		t.Fatalf("NewVectorRepo failed: %v", err)
	}
	testVocab := []domain.VocabItem{
		{ID: 1, Word: "駅", Reading: "えき", Meaning: "train station"},
		{ID: 2, Word: "水", Reading: "みず", Meaning: "water"},
	}
	synced, err := vectorRepo.SyncVocab(ctx, testVocab)
	if err != nil || synced != 2 {
		t.Fatalf("SyncVocab failed (synced: %d, err: %v)", synced, err)
	}
	matches := vectorRepo.SearchTopK(ctx, "localidade", 2, 0.1)
	if len(matches) == 0 {
		t.Fatalf("Expected matches for localidade, got 0")
	}
	if matches[0].Item.Word != "駅" {
		t.Fatalf("Expected top match to be 駅, got %s", matches[0].Item.Word)
	}

	// 6. Test VectorRepo Read Status & Decks
	if vectorRepo.CountRead(ctx) != 0 {
		t.Errorf("Esperado 0 lidos inicialmente, obtido %d", vectorRepo.CountRead(ctx))
	}

	readMatches := vectorRepo.SearchReadTopK(ctx, "localidade", 2, 0.1)
	if len(readMatches) != 0 {
		t.Errorf("Esperado 0 resultados em SearchReadTopK quando nada lido, obtido %d", len(readMatches))
	}

	// Mark vocab ID 1 as read
	if err := vectorRepo.MarkVocabRead(ctx, 1, true); err != nil {
		t.Fatalf("MarkVocabRead failed: %v", err)
	}
	if vectorRepo.CountRead(ctx) != 1 {
		t.Errorf("Esperado 1 lido, obtido %d", vectorRepo.CountRead(ctx))
	}

	readMatches = vectorRepo.SearchReadTopK(ctx, "localidade", 2, 0.1)
	if len(readMatches) == 0 || readMatches[0].Item.Word != "駅" {
		t.Errorf("Esperado encontrar 駅 em SearchReadTopK")
	}

	// Test Decks
	decks := vectorRepo.GetDecks(ctx)
	if len(decks) == 0 {
		t.Errorf("Esperado ao menos 1 deck, obtido 0")
	}
	if decks[0].TotalVocab != 2 || decks[0].ReadVocab != 1 {
		t.Errorf("Estatísticas do deck incorretas: %+v", decks[0])
	}

	// Mark entire deck as read
	if err := vectorRepo.MarkDeckRead(ctx, decks[0].Name, true); err != nil {
		t.Fatalf("MarkDeckRead failed: %v", err)
	}
	if vectorRepo.CountRead(ctx) != 2 {
		t.Errorf("Esperado 2 lidos após MarkDeckRead, obtido %d", vectorRepo.CountRead(ctx))
	}

	// Mark entire deck as unread
	if err := vectorRepo.MarkDeckRead(ctx, decks[0].Name, false); err != nil {
		t.Fatalf("MarkDeckRead(false) failed: %v", err)
	}
	if vectorRepo.CountRead(ctx) != 0 {
		t.Errorf("Esperado 0 lidos após desmarcar deck, obtido %d", vectorRepo.CountRead(ctx))
	}
}
