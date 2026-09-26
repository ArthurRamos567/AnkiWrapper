package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"anki-wrapper/internal/domain"
	_ "modernc.org/sqlite"
)

// DB represents a SQLite database connection for repositories.
type DB struct {
	*sql.DB
}

// Open initializes and runs migrations for SQLite storage.
func Open(dbPath string) (*DB, error) {
	if dbPath == "" {
		dbPath = filepath.Join("data", "app.db")
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("falha ao criar diretório do banco local: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("falha ao abrir banco local: %w", err)
	}

	wrapper := &DB{db}
	if err := wrapper.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("falha ao executar migrations: %w", err)
	}

	return wrapper, nil
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS prompts (
		key TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL,
		content TEXT NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS quiz_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mode TEXT NOT NULL,
		topic TEXT NOT NULL,
		question TEXT NOT NULL,
		user_answer TEXT NOT NULL,
		correct_answer TEXT NOT NULL,
		is_correct BOOLEAN NOT NULL,
		explanation TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS chat_sessions (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS chat_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		furigana_html TEXT NOT NULL DEFAULT '',
		translation TEXT NOT NULL DEFAULT '',
		pedagogical_note TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS vocab_embeddings (
		anki_note_id INTEGER PRIMARY KEY,
		deck_id INTEGER NOT NULL DEFAULT 1,
		deck_name TEXT NOT NULL DEFAULT 'Default',
		word TEXT NOT NULL,
		reading TEXT NOT NULL,
		meaning TEXT NOT NULL,
		sentence TEXT NOT NULL,
		tags TEXT NOT NULL,
		embedding BLOB NOT NULL,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS vocab_reads (
		vocab_id INTEGER PRIMARY KEY,
		read_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_vocab_reads_id ON vocab_reads(vocab_id);

	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS llm_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		operation TEXT NOT NULL,
		prompt_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		thinking_tokens INTEGER DEFAULT 0,
		cached_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0,
		duration_ms INTEGER DEFAULT 0,
		status TEXT NOT NULL,
		error_msg TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_llm_usage_created_at ON llm_usage(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_llm_usage_operation ON llm_usage(operation);

	CREATE TABLE IF NOT EXISTS grammar_lessons (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		chapter_number INTEGER NOT NULL,
		chapter TEXT NOT NULL,
		part INTEGER NOT NULL,
		section_key TEXT NOT NULL,
		slug TEXT NOT NULL,
		url TEXT NOT NULL,
		web_url TEXT NOT NULL,
		file_path TEXT NOT NULL,
		content TEXT NOT NULL,
		embedding BLOB,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_grammar_section ON grammar_lessons(section_key);
	CREATE INDEX IF NOT EXISTS idx_grammar_slug ON grammar_lessons(slug);

	CREATE TABLE IF NOT EXISTS lesson_reads (
		lesson_id TEXT PRIMARY KEY,
		read_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_lesson_reads_id ON lesson_reads(lesson_id);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	// Migrations for existing databases
	_, _ = db.Exec("ALTER TABLE vocab_embeddings ADD COLUMN deck_id INTEGER NOT NULL DEFAULT 1")
	_, _ = db.Exec("ALTER TABLE vocab_embeddings ADD COLUMN deck_name TEXT NOT NULL DEFAULT 'Default'")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_vocab_deck_name ON vocab_embeddings(deck_name)")

	return db.seedDefaultPrompts()
}

func (db *DB) seedDefaultPrompts() error {
	defaults := []domain.PromptItem{
		{
			Key:   "chat_system",
			Title: "Agente de Conversação (Chat)",
			Description: "Controla o tom, nível de japonês, uso de furigana e correções pedagógicas suaves no chat.",
			Content: `Você é um parceiro de conversação em Japonês empático, instrutivo e natural.
Seu objetivo é conversar com o usuário em Japonês, priorizando o vocabulário que ele já conhece (indicado na lista de vocabulário fornecida).

Diretrizes:
1. Responda em Japonês fluente, claro e acessível ao nível do usuário.
2. Sempre anote palavras em kanji com furigana no formato Anki: 漢字[かんじ]. Exemplo: 私[わたし]は日本語[にほんご]を勉強[べんきょう]しています。
3. Forneça uma tradução de apoio no idioma de suporte configurado.
4. Se o usuário cometer erros gramaticais ou de partículas na mensagem dele, forneça uma nota pedagógica suave explicando a correção com gentileza e brevidade. Se a mensagem do usuário estiver correta, deixe a nota pedagógica vazia.`,
		},
		{
			Key:   "quiz_generator",
			Title: "Gerador de Quizzes",
			Description: "Define como as questões de Múltipla Escolha, Cloze e Tradução Livre são criadas.",
			Content: `Você é um professor especialista na língua japonesa criando questões de prática e fixação gramatical.
Crie questões claras, naturais e focadas no tópico solicitado pelo usuário (ex: tópicos gramaticais, lições do Tae Kim ou vocabulário do Anki).

Diretrizes:
1. Utilize preferencialmente palavras conhecidas pelo usuário, introduzindo novas expressões apenas quando indispensáveis para a naturalidade da frase.
2. Todas as sentenças em japonês com kanji devem conter furigana no formato Anki: 漢字[かんじ].
3. Para Múltipla Escolha: Forneça exatamente 4 opções plausíveis e explique por que a correta é a adequada e as outras são incorretas.
4. Para Cloze: Crie uma frase com a lacuna "____" indicando o papel gramatical ou significado pretendido.
5. Para Tradução Livre: Forneça uma situação real ou frase no idioma de suporte para o usuário traduzir para o japonês. NUNCA revele a resposta em japonês no enunciado ("prompt_text") ou na dica ("hint"). A resposta esperada em japonês pertence estritamente ao campo "reference_answer".`,
		},
		{
			Key:   "quiz_evaluator",
			Title: "Avaliador de Tradução/Composição Livre",
			Description: "Avalia as frases escritas pelo usuário em modo de composição livre.",
			Content: `Você é um avaliador pedagógico de Japonês.
Analise a resposta fornecida pelo usuário para a questão proposta.

Diretrizes:
1. Avalie a correção gramatical, uso de partículas, conjugação e naturalidade da expressão.
2. Considere correta mesmo se houver formas alternativas válidas e naturais de expressar a mesma ideia.
3. Forneça uma explicação encorajadora detalhando pontos positivos e melhorias sugeridas.`,
		},
		{
			Key:   "soft_n_plus_one",
			Title: "Diretriz de Vocabulário (Soft N+1)",
			Description: "Instrução de preferência flexível de vocabulário inserida em todas as requisições.",
			Content: `[DIRETRIZ DE VOCABULÁRIO CONHECIDO]:
A lista de vocabulário conhecida pelo usuário serve como banco de preferência primário para manter a compreensão acessível. No entanto, ela NÃO é uma restrição rígida: priorize a naturalidade idiomática do Japonês. Caso uma estrutura exija termos novos, use-os livremente, obrigatoriamente anotando com furigana no formato 漢字[かんじ].`,
		},
	}

	for _, p := range defaults {
		_, err := db.Exec(`
			INSERT OR IGNORE INTO prompts (key, title, description, content, updated_at)
			VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		`, p.Key, p.Title, p.Description, p.Content)
		if err != nil {
			return err
		}
	}

	// Update existing databases if quiz_generator does not yet have anti-leak directive
	_, _ = db.Exec(`
		UPDATE prompts
		SET content = ?
		WHERE key = 'quiz_generator' AND content NOT LIKE '%NUNCA revele%'
	`, defaults[1].Content)

	_, err := db.Exec(`
		INSERT OR IGNORE INTO app_settings (key, value)
		VALUES ('support_lang', 'pt')
	`)
	return err
}
