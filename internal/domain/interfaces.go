package domain

import (
	"context"
)

// LLMProvider abstracts communication with any LLM engine (e.g. AGY CLI, Gemini API, OpenAI, Ollama, Anthropic).
type LLMProvider interface {
	ExecutePrompt(ctx context.Context, prompt string) (string, error)
	ExecuteStructured(ctx context.Context, prompt string, schema string, target any) error
}

// PromptRepository manages editable system prompts and runtime application settings.
type PromptRepository interface {
	GetAllPrompts(ctx context.Context) ([]PromptItem, error)
	GetPrompt(ctx context.Context, key string) (string, error)
	UpdatePrompt(ctx context.Context, key, content string) error
	GetSetting(ctx context.Context, key, defaultValue string) string
	SetSetting(ctx context.Context, key, value string) error
}

// QuizRepository stores quiz attempts and calculates aggregate performance metrics.
type QuizRepository interface {
	SaveQuizRecord(ctx context.Context, r QuizRecord) error
	GetRecentQuizzes(ctx context.Context, limit int) ([]QuizRecord, error)
	GetQuizStats(ctx context.Context) (*QuizStats, error)
}

// ChatRepository manages multiconversation sessions and persistent messages.
type ChatRepository interface {
	CreateSession(ctx context.Context, id, title string) error
	GetSessions(ctx context.Context) ([]ChatSession, error)
	UpdateSessionTitle(ctx context.Context, id, title string) error
	DeleteSession(ctx context.Context, id string) error
	SaveChatMessage(ctx context.Context, m ChatMessage) error
	GetChatMessages(ctx context.Context, sessionID string) ([]ChatMessage, error)
	ClearSessionMessages(ctx context.Context, sessionID string) error
}

// VectorRepository manages dense vocabulary vector embeddings and sub-millisecond semantic search.
type VectorRepository interface {
	SyncVocab(ctx context.Context, items []VocabItem) (int, error)
	SearchTopK(ctx context.Context, query string, topK int, threshold float32) []ScoredVocab
	SearchReadTopK(ctx context.Context, query string, topK int, threshold float32) []ScoredVocab
	Count(ctx context.Context) int
	CountRead(ctx context.Context) int
	MarkVocabRead(ctx context.Context, id int64, isRead bool) error
	MarkDeckRead(ctx context.Context, deckName string, isRead bool) error
	GetDecks(ctx context.Context) []DeckInfo
	GetAllVocab(ctx context.Context) []VocabItem
	GetVocabByDeck(ctx context.Context, deckName string) []VocabItem
	GetReadVocab(ctx context.Context) []VocabItem
}

// VocabReader abstracts reading vocabulary from Anki or other sources.
type VocabReader interface {
	FindCollection() (string, error)
	ReadVocab(path string) ([]VocabItem, error)
}

// TelemetryRepository stores and aggregates LLM token usage telemetry.
type TelemetryRepository interface {
	RecordUsage(ctx context.Context, record LLMUsageRecord) error
	GetUsageStats(ctx context.Context) (*LLMUsageStats, error)
	GetRecentUsage(ctx context.Context, limit int) ([]LLMUsageRecord, error)
	ClearUsage(ctx context.Context) error
}

// GrammarRepository manages indexing and retrieval of Tae Kim's grammar guide lessons.
type GrammarRepository interface {
	SyncLessons(ctx context.Context, lessons []GrammarLesson) (int, error)
	SearchLessons(ctx context.Context, query string, topK int) ([]GrammarLesson, error)
	GetLessonByID(ctx context.Context, id string) (*GrammarLesson, error)
	GetLessonBySection(ctx context.Context, section string) (*GrammarLesson, error)
	GetAllLessons(ctx context.Context) ([]GrammarLesson, error)
	Count(ctx context.Context) int

	// Read status methods
	MarkLessonRead(ctx context.Context, id string, isRead bool) error
	GetReadLessons(ctx context.Context) ([]GrammarLesson, error)
	SearchReadLessons(ctx context.Context, query string, topK int) ([]GrammarLesson, error)
	CountRead(ctx context.Context) int
}

