package domain

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Operation context key
type contextKey string

const OperationContextKey contextKey = "llm_operation"

// WithOperation annotates a context with an operation name (e.g. "quiz_generate", "chat_reply").
func WithOperation(ctx context.Context, op string) context.Context {
	return context.WithValue(ctx, OperationContextKey, op)
}

// GetOperation returns the operation name from context, or "general" if not set.
func GetOperation(ctx context.Context) string {
	if val, ok := ctx.Value(OperationContextKey).(string); ok && val != "" {
		return val
	}
	return "general"
}

// LLMUsageRecord represents a telemetry entry for an LLM call.
type LLMUsageRecord struct {
	ID             int64     `json:"id"`
	Provider       string    `json:"provider"`        // "agy", "gemini", etc.
	Model          string    `json:"model"`           // "gemini-2.5-flash", etc.
	Operation      string    `json:"operation"`       // "quiz_generate", "quiz_evaluate", "chat_reply", etc.
	PromptTokens   int       `json:"prompt_tokens"`
	OutputTokens   int       `json:"output_tokens"`
	ThinkingTokens int       `json:"thinking_tokens"`
	CachedTokens   int       `json:"cached_tokens"`
	TotalTokens    int       `json:"total_tokens"`
	DurationMs     int64     `json:"duration_ms"`
	Status         string    `json:"status"`          // "SUCCESS", "ERROR"
	ErrorMsg       string    `json:"error_msg,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// LLMUsageStats aggregates token and request metrics.
type LLMUsageStats struct {
	TotalCalls          int              `json:"total_calls"`
	TotalPromptTokens   int64            `json:"total_prompt_tokens"`
	TotalOutputTokens   int64            `json:"total_output_tokens"`
	TotalThinkingTokens int64            `json:"total_thinking_tokens"`
	TotalCachedTokens   int64            `json:"total_cached_tokens"`
	TotalTokens         int64            `json:"total_tokens"`
	AvgDurationMs       int64            `json:"avg_duration_ms"`
	CallsByProvider     map[string]int   `json:"calls_by_provider"`
	CallsByOperation    map[string]int   `json:"calls_by_operation"`
	TokensByOperation   map[string]int64 `json:"tokens_by_operation"`
}

// VocabItem represents a single vocabulary entry extracted from Anki or external source.
type VocabItem struct {
	ID       int64  `json:"id"`
	DeckID   int64  `json:"deck_id"`
	DeckName string `json:"deck_name"`
	Word     string `json:"word"`
	Reading  string `json:"reading"`
	Meaning  string `json:"meaning"`
	Sentence string `json:"sentence"`
	Tags     string `json:"tags"`
	IsRead   bool   `json:"is_read"`
}

// DeckInfo aggregates vocabulary and read progress for a specific Anki deck.
type DeckInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	TotalVocab  int    `json:"total_vocab"`
	ReadVocab   int    `json:"read_vocab"`
	UnreadVocab int    `json:"unread_vocab"`
	Percentage  int    `json:"percentage"`
}

// CompactString returns a token-efficient representation for LLM prompt context.
func (v VocabItem) CompactString() string {
	var sb strings.Builder
	sb.WriteString(v.Word)
	if v.Reading != "" && v.Reading != v.Word {
		sb.WriteString(" [")
		sb.WriteString(v.Reading)
		sb.WriteString("]")
	}
	if v.Meaning != "" {
		shortMeaning := v.Meaning
		if len(shortMeaning) > 60 {
			shortMeaning = shortMeaning[:60] + "..."
		}
		sb.WriteString(": ")
		sb.WriteString(shortMeaning)
	}
	return sb.String()
}

// VocabListToString formats a slice of VocabItem into a markdown bullet list for prompt injection.
func VocabListToString(items []VocabItem) string {
	if len(items) == 0 {
		return "Nenhum vocabulário sincronizado no momento."
	}
	var sb strings.Builder
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item.CompactString()))
	}
	return sb.String()
}

// ScoredVocab holds a vocab item with its semantic similarity score.
type ScoredVocab struct {
	Item  VocabItem
	Score float32
}

// PromptItem represents an editable system prompt.
type PromptItem struct {
	Key         string    `json:"key"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// QuizRecord represents an answered quiz question.
type QuizRecord struct {
	ID            int64     `json:"id"`
	Mode          string    `json:"mode"`
	Topic         string    `json:"topic"`
	Question      string    `json:"question"`
	UserAnswer    string    `json:"user_answer"`
	CorrectAnswer string    `json:"correct_answer"`
	IsCorrect     bool      `json:"is_correct"`
	Explanation   string    `json:"explanation"`
	CreatedAt     time.Time `json:"created_at"`
}

// QuizStats aggregates performance metrics.
type QuizStats struct {
	Total     int     `json:"total"`
	Correct   int     `json:"correct"`
	Incorrect int     `json:"incorrect"`
	Accuracy  float64 `json:"accuracy"`
	Streak    int     `json:"streak"`
}

// ChatSession represents a distinct conversational session.
type ChatSession struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatMessage represents a message in a chat session.
type ChatMessage struct {
	ID              int64             `json:"id"`
	SessionID       string            `json:"session_id"`
	Role            string            `json:"role"` // "user" or "assistant"
	Content         string            `json:"content"`
	FuriganaHTML    string            `json:"furigana_html"`
	Translation     string            `json:"translation"`
	PedagogicalNote string            `json:"pedagogical_note"`
	GrammarRef      *GrammarReference `json:"grammar_ref,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

// GrammarReference points to an authoritative Tae Kim grammar rule with external and local links.
type GrammarReference struct {
	ID         string `json:"id"`          // e.g. "basics/noun-related-particles"
	Title      string `json:"title"`       // e.g. "Noun-related Particles"
	SectionKey string `json:"section_key"` // e.g. "3.11"
	Chapter    string `json:"chapter"`     // e.g. "Basic Grammar"
	URL        string `json:"url"`         // Official URL on guidetojapanese.org
	WebURL     string `json:"web_url"`     // Mirror web URL on jp.saeris.gg
}

// LocalURL returns the in-app route to read the lesson.
func (r GrammarReference) LocalURL() string {
	return "/grammar/lesson?id=" + url.QueryEscape(r.ID)
}

// GrammarLesson represents a complete Tae Kim grammar guide lesson.
type GrammarLesson struct {
	ID            string            `json:"id"`             // e.g. "basics/noun-related-particles"
	Title         string            `json:"title"`          // "Noun-related Particles"
	ChapterNumber int               `json:"chapter_number"` // 3
	Chapter       string            `json:"chapter"`        // "Basic Grammar"
	Part          int               `json:"part"`           // 11
	SectionKey    string            `json:"section_key"`    // "3.11"
	Slug          string            `json:"slug"`           // "noun-related-particles"
	URL           string            `json:"url"`            // Official guide link
	WebURL        string            `json:"web_url"`        // Mirror web link
	FilePath      string            `json:"file_path"`      // Relative file path on disk
	Content       string            `json:"content"`        // Raw markdown content
	IsRead        bool              `json:"is_read"`        // Whether marked as read/completed by user
	Reference     *GrammarReference `json:"reference"`
}

// LocalURL returns the in-app route to read this lesson.
func (l GrammarLesson) LocalURL() string {
	return "/grammar/lesson?id=" + url.QueryEscape(l.ID)
}

// Snippet extracts a clean preview of up to maxChars characters.
func (l GrammarLesson) Snippet(maxChars int) string {
	clean := strings.TrimSpace(l.Content)
	if len(clean) > maxChars {
		return clean[:maxChars] + "..."
	}
	return clean
}

