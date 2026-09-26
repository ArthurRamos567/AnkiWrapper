package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sync"

	"anki-wrapper/internal/chat"
	"anki-wrapper/internal/domain"
	"anki-wrapper/internal/quiz"
)

//go:embed templates/*
var templateFS embed.FS

// Server holds web application state and HTTP handlers using decoupled domain interfaces.
type Server struct {
	quizEngine  *quiz.Engine
	chatService *chat.Service
	prompts     domain.PromptRepository
	quizRepo    domain.QuizRepository
	chatRepo    domain.ChatRepository
	vectorRepo  domain.VectorRepository
	telemetry   domain.TelemetryRepository
	grammarRepo domain.GrammarRepository

	mu              sync.RWMutex
	vocab           []domain.VocabItem
	ankiPath        string
	activeQuestions map[string]*quiz.Question
	tmpl            *template.Template
}

// Config provides initial dependencies for Server via interfaces.
type Config struct {
	QuizEngine  *quiz.Engine
	ChatService *chat.Service
	Prompts     domain.PromptRepository
	QuizRepo    domain.QuizRepository
	ChatRepo    domain.ChatRepository
	VectorRepo  domain.VectorRepository
	Telemetry   domain.TelemetryRepository
	GrammarRepo domain.GrammarRepository
	Vocab       []domain.VocabItem
	AnkiPath    string
}

// NewServer initializes the Web Server and compiles templates.
func NewServer(cfg Config) (*Server, error) {
	funcMap := template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"raw": func(s string) template.HTML { return template.HTML(s) },
	}

	tmpl, err := template.New("base").Funcs(funcMap).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("falha ao carregar templates HTML: %w", err)
	}

	s := &Server{
		quizEngine:      cfg.QuizEngine,
		chatService:     cfg.ChatService,
		prompts:         cfg.Prompts,
		quizRepo:        cfg.QuizRepo,
		chatRepo:        cfg.ChatRepo,
		vectorRepo:      cfg.VectorRepo,
		telemetry:       cfg.Telemetry,
		grammarRepo:     cfg.GrammarRepo,
		vocab:           cfg.Vocab,
		ankiPath:        cfg.AnkiPath,
		activeQuestions: make(map[string]*quiz.Question),
		tmpl:            tmpl,
	}

	if len(cfg.Vocab) > 0 {
		chat.SetKnownVocab(cfg.Vocab)
	}

	return s, nil
}

// Routes sets up all HTTP routes.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Core
	mux.HandleFunc("/", s.handleIndex)

	// Quizzes
	mux.HandleFunc("/quiz", s.handleQuizPage)
	mux.HandleFunc("/quiz/generate", s.handleQuizGenerate)
	mux.HandleFunc("/quiz/answer", s.handleQuizAnswer)

	// Chat & Sessions
	mux.HandleFunc("/chat", s.handleChatPage)
	mux.HandleFunc("/chat/send", s.handleChatSend)
	mux.HandleFunc("/chat/clear", s.handleChatClear)
	mux.HandleFunc("/chat/new-session", s.handleChatNewSession)
	mux.HandleFunc("/chat/delete-session", s.handleChatDeleteSession)

	// Grammar & Reference Guide
	mux.HandleFunc("/grammar", s.handleGrammarPage)
	mux.HandleFunc("/grammar/lesson", s.handleGrammarLesson)
	mux.HandleFunc("/grammar/lesson/toggle-read", s.handleGrammarLessonToggleRead)
	mux.HandleFunc("/grammar/notes/save", s.handleGrammarNoteSave)
	mux.HandleFunc("/grammar/notes/delete", s.handleGrammarNoteDelete)

	// Anki Decks & Vocabulary Checklist
	mux.HandleFunc("/decks", s.handleDecksPage)
	mux.HandleFunc("/vocab/toggle-read", s.handleVocabToggleRead)
	mux.HandleFunc("/decks/toggle-read", s.handleDeckToggleRead)

	// Project Prompts & Tuning
	mux.HandleFunc("/prompts", s.handlePromptsPage)
	mux.HandleFunc("/prompts/update", s.handlePromptUpdate)
	mux.HandleFunc("/prompts/setting", s.handleSettingUpdate)
	mux.HandleFunc("/anki/reload", s.handleAnkiReload)

	// Telemetry & Token Usage
	mux.HandleFunc("/telemetry", s.handleTelemetryPage)
	mux.HandleFunc("/telemetry/clear", s.handleTelemetryClear)

	return mux
}

func (s *Server) getVocabSnapshot() []domain.VocabItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vocab
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/quiz", http.StatusSeeOther)
}
