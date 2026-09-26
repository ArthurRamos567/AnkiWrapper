# CLAUDE.md

## Overview & Architecture

AnkiWrapper is a Japanese learning assistant built in **Go + HTMX**, running locally. It generates contextual quizzes and conversational practice calibrated against vocabulary read directly from the user's local Anki SQLite database.

### Core Architecture Rules
- **Anki is strictly Read-Only**: Access `collection.anki2` via `file:<path>?mode=ro`. Never write to or lock the Anki database.
- **Decoupled Domain Layer**: Core interfaces and entities live in `internal/domain` (`LLMProvider`, `TelemetryRepository`, `QuizRepository`, `ChatRepository`, `PromptRepository`, `VectorRepository`). All modules and handlers depend on these interfaces via dependency injection.
- **Multi-LLM Support**:
  - `GeminiProvider` (`internal/llm/gemini.go`): Direct REST client to Google AI Studio API (`gemini-3.8-flash`), configured via `.env` (`GEMINI_API_KEY`, `GEMINI_MODEL`) or CLI flag.
  - `Driver` (`internal/llm/driver.go`): Subprocess executing local `agy` CLI (`agy --print --output-format json --json-schema`) using existing local subscription.
  - Configurable via `LLM_PROVIDER` (`auto`, `gemini`, `agy`).
- **Full Token Telemetry**: Every LLM interaction is tracked in `llm_usage` table via `TelemetryRepository` (`prompt_tokens`, `output_tokens`, `thinking_tokens`, `cached_tokens`, `total_tokens`, `duration_ms`, `operation`, `provider`, `status`). Operations are tagged using `domain.WithOperation(ctx, op)`.
- **Tae Kim Grammar Guide & Reference Linking**:
  - Complete 61-lesson Tae Kim Grammar Guide parsed from `data/knowledge/taekim/grammar` and indexed locally in `grammar_lessons` table.
  - Zero-token indexing: Semantic embeddings computed locally in pure Go (`internal/rag`).
  - Strict Rule Linking: Whenever grammar rules are used in quizzes or chat explanations, clickable links are provided to the authoritative official source (`guidetojapanese.org/learn/grammar/...`) and the local in-app lesson reader (`/grammar/lesson?id=...`).
  - Browseable grammar hub at `/grammar` structured into sub-tabs: **Guia Tae Kim** (`?tab=taekim`), **Referência Rápida** (`?tab=quickref`), and **Minhas Anotações** (`?tab=notes`), with furigana hover/always mode support.

---

## UI / UX Design Guidelines

Strict design constraints requested by the user:
- **Ultra-Minimalist Light Mode**: Plain semantic HTML (`<table>`, `<p>`, `<strong>`, `<input>`, `<button>`). Background `#ffffff`, text `#111111`, subtle `#ccc` borders.
- **Zero Gamification**: No streak counters, no fire/flame icons ("foguinho"), no progress bars, no badges.
- **No Clutter**: No decorative logos, no captions/legends, no canned suggestion chips, no placeholder tips instructing what to ask.
- **Uniform Typography**: Form inputs, textareas, and buttons must inherit project font size (`font-family: inherit; font-size: 0.95rem;`).
- **Furigana Rendering**: Furigana rendered via standard `<ruby>漢字<rt>かんじ</rt></ruby>`. Toggleable between hover mode (`ruby:hover rt`) and always visible.
- **New Vocabulary Highlighting**: Japanese words not present in the user's Anki collection are automatically tagged with the `vocab-new` class and styled in subtle blue (`#1a66cc`). Toggleable via header button ("Novos Termos: Azul | Normal") and saved in `localStorage`. Zero-overhead in-memory O(1) set lookup.
- **Quizzes**: Mode selection must be a **multiselect** (allowing users to pick any combination of Multiple Choice, Cloze, Free Composition).

---

## Commands

```bash
# Run all tests
go test -v ./...

# Build binary
go build -o anki-wrapper ./cmd/anki-wrapper

# Run server (default: port 8080)
./anki-wrapper serve

# Diagnostic CLI subcommands
./anki-wrapper test-anki
./anki-wrapper test-gemini
./anki-wrapper test-agy
```

---

## Environment Variables (.env)

- `GEMINI_API_KEY`: API key for Google AI Studio (optional if using `agy`).
- `GEMINI_MODEL`: Gemini model name (default: `gemini-3.8-flash`).
- `LLM_PROVIDER`: `auto` (default), `gemini`, or `agy`.
- `PORT`: HTTP port (default: `8080`).
- `ANKI_PATH`: Path to `collection.anki2` (auto-detected if empty).
- `APP_DB_PATH`: SQLite application database path (default: `data/app.db`).

---

## Testing Standards

- Maintain 100% passing tests across all packages.
- Always use in-memory SQLite (`:memory:`) or `httptest.Server` for tests; never touch `data/app.db` or real Anki databases during testing.
- When modifying LLM adapters, engines, repositories, or HTTP routes, always add or update unit tests in `*_test.go`.
