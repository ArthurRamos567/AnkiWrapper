package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"anki-wrapper/internal/anki"
	"anki-wrapper/internal/chat"
	"anki-wrapper/internal/domain"
	"anki-wrapper/internal/grammar"
	"anki-wrapper/internal/llm"
	"anki-wrapper/internal/quiz"
	"anki-wrapper/internal/repository/sqlite"
	"anki-wrapper/internal/web"
)

func init() {
	loadEnv(".env")
}

// loadEnv reads key=value pairs from a .env file and sets them in os.Environ if not already defined.
func loadEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
				val = val[1 : len(val)-1]
			}
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "test-anki":
			runTestAnki()
			return
		case "test-agy":
			runTestAgy()
			return
		case "test-gemini":
			runTestGemini()
			return
		case "help", "-h", "--help":
			printHelp()
			return
		}
	}

	runServe()
}

func printHelp() {
	fmt.Print(`AnkiWrapper — Assistente de Japonês & Quizzes (Go + HTMX)

Uso:
  anki-wrapper [comando] [flags]

Comandos:
  serve (padrão)    Inicia o servidor web HTMX local (padrão: http://localhost:8080)
  test-anki         Testa a conexão e leitura do vocabulário do Anki SQLite
  test-agy          Testa a chamada ao provedor local LLM (agy/Gemini CLI)
  test-gemini       Testa a chamada direta à Gemini API (Google AI Studio)

Flags para serve:
  -port             Porta HTTP para o servidor (padrão: 8080 ou variável PORT)
  -llm              Provedor LLM: 'auto' (padrão), 'gemini' ou 'agy'
  -gemini-key       Chave de API do Gemini (ou variável GEMINI_API_KEY)
  -gemini-model     Modelo do Gemini (padrão: gemini-3.8-flash)
  -agy-model        Modelo do AGY CLI (padrão: gemini-3.8-flash-low ou variável AGY_MODEL)
  -anki-path        Caminho customizado para o arquivo collection.anki2
  -db-path          Caminho do banco local da aplicação (padrão: data/app.db)
`)
}

func runTestAnki() {
	fmt.Println("🔍 Buscando banco de dados do Anki...")
	path, err := anki.FindAnkiCollection()
	if err != nil {
		log.Fatalf("❌ Erro: %v", err)
	}

	fmt.Printf("✅ Encontrado em: %s\n", path)
	fmt.Println("📖 Lendo vocabulário em modo read-only...")

	start := time.Now()
	items, err := anki.ReadAnkiVocab(path)
	if err != nil {
		log.Fatalf("❌ Erro ao ler vocabulário: %v", err)
	}

	fmt.Printf("🎉 Sucesso! %d palavras extraídas em %v\n\n", len(items), time.Since(start))
	fmt.Println("Primeiras 5 palavras:")
	for i := 0; i < len(items) && i < 5; i++ {
		fmt.Printf("  %d. %s\n", i+1, items[i].CompactString())
	}
}

func runTestAgy() {
	fmt.Println("🤖 Testando provedor LLM (AGY)...")
	var provider domain.LLMProvider
	driver, err := llm.NewDriver("")
	if err != nil {
		log.Fatalf("❌ Erro ao inicializar driver do agy: %v", err)
	}
	provider = driver

	fmt.Printf("✅ Provedor LLM pronto (AGY binário: %s, Modelo: %s)\n", driver.AgyPath, driver.Model)
	fmt.Println("⏳ Enviando prompt de teste estruturado...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result struct {
		Greeting string `json:"greeting"`
		Language string `json:"language"`
	}

	schema := `{"type":"object","properties":{"greeting":{"type":"string"},"language":{"type":"string"}},"required":["greeting","language"]}`
	err = provider.ExecuteStructured(ctx, "Diga olá em japonês e informe a língua", schema, &result)
	if err != nil {
		log.Fatalf("❌ Erro na execução da LLM: %v", err)
	}

	fmt.Printf("🎉 Resposta recebida com sucesso!\n  Greeting: %s\n  Language: %s\n", result.Greeting, result.Language)
}

func runTestGemini() {
	fmt.Println("🤖 Testando provedor LLM (Google Gemini API)...")

	fs := flag.NewFlagSet("test-gemini", flag.ExitOnError)
	keyFlag := fs.String("key", "", "Chave de API do Gemini")
	modelFlag := fs.String("model", "gemini-3.8-flash", "Modelo do Gemini")
	_ = fs.Parse(os.Args[2:])

	provider, err := llm.NewGeminiProvider(llm.GeminiConfig{
		APIKey: *keyFlag,
		Model:  *modelFlag,
	})
	if err != nil {
		log.Fatalf("❌ Erro ao inicializar GeminiProvider: %v", err)
	}

	fmt.Printf("✅ GeminiProvider pronto (Modelo: %s)\n", *modelFlag)
	fmt.Println("⏳ Enviando prompt de teste estruturado via HTTP para Google AI Studio...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var result struct {
		Greeting string `json:"greeting"`
		Language string `json:"language"`
	}

	schema := `{"type":"object","properties":{"greeting":{"type":"string"},"language":{"type":"string"}},"required":["greeting","language"]}`
	err = provider.ExecuteStructured(ctx, "Diga olá em japonês e informe a língua", schema, &result)
	if err != nil {
		log.Fatalf("❌ Erro na execução da Gemini API: %v", err)
	}

	fmt.Printf("🎉 Resposta da Gemini API recebida com sucesso!\n  Greeting: %s\n  Language: %s\n", result.Greeting, result.Language)
}

func runServe() {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.String("port", "8080", "Porta HTTP do servidor")
	llmProviderFlag := fs.String("llm", "auto", "Provedor LLM: 'auto' (padrão), 'gemini' ou 'agy'")
	geminiKeyFlag := fs.String("gemini-key", "", "Chave de API do Gemini")
	geminiModelFlag := fs.String("gemini-model", "gemini-3.8-flash", "Modelo do Gemini")
	agyModelFlag := fs.String("agy-model", "gemini-3.8-flash-low", "Modelo do AGY CLI (padrão: gemini-3.8-flash-low)")
	ankiPathFlag := fs.String("anki-path", "", "Caminho do banco collection.anki2")
	dbPath := fs.String("db-path", "data/app.db", "Caminho do banco local SQLite")

	args := os.Args[1:]
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	_ = fs.Parse(args)

	if envPort := os.Getenv("PORT"); envPort != "" {
		*port = envPort
	}
	if *llmProviderFlag == "auto" && os.Getenv("LLM_PROVIDER") != "" {
		*llmProviderFlag = os.Getenv("LLM_PROVIDER")
	}
	if *agyModelFlag == "gemini-3.8-flash-low" && os.Getenv("AGY_MODEL") != "" {
		*agyModelFlag = os.Getenv("AGY_MODEL")
	}
	if *ankiPathFlag == "" && os.Getenv("ANKI_PATH") != "" {
		*ankiPathFlag = os.Getenv("ANKI_PATH")
	}
	if *dbPath == "data/app.db" && os.Getenv("DB_PATH") != "" {
		*dbPath = os.Getenv("DB_PATH")
	}

	fmt.Println("🚀 Iniciando AnkiWrapper (Modular Architecture)...")

	// 1. Initialize SQLite Database connection
	appDB, err := sqlite.Open(*dbPath)
	if err != nil {
		log.Fatalf("❌ Erro ao abrir banco de dados local: %v", err)
	}
	defer appDB.Close()
	fmt.Println("✅ Banco de dados local inicializado em:", *dbPath)

	// 2. Discover and read Anki collection (Read-Only)
	ankiPath := *ankiPathFlag
	if ankiPath == "" {
		var err error
		ankiPath, err = anki.FindAnkiCollection()
		if err != nil {
			fmt.Printf("⚠️ Aviso: %v\n", err)
		}
	}

	var vocab []domain.VocabItem
	if ankiPath != "" {
		var err error
		vocab, err = anki.ReadAnkiVocab(ankiPath)
		if err != nil {
			fmt.Printf("⚠️ Aviso ao ler Anki: %v\n", err)
		} else {
			fmt.Printf("✅ Anki conectado (Read-Only): %d palavras carregadas (%s)\n", len(vocab), ankiPath)
			chat.SetKnownVocab(vocab)
		}
	}

	// 3. Initialize Repositories (implementing domain interfaces)
	promptRepo := sqlite.NewPromptRepo(appDB.DB)
	quizRepo := sqlite.NewQuizRepo(appDB.DB)
	chatRepo := sqlite.NewChatRepo(appDB.DB)
	telemetryRepo := sqlite.NewTelemetryRepo(appDB.DB)
	grammarRepo, err := sqlite.NewGrammarRepo(appDB.DB)
	if err != nil {
		log.Fatalf("❌ Erro ao inicializar GrammarRepo: %v", err)
	}
	vectorRepo, err := sqlite.NewVectorRepo(appDB.DB)
	if err != nil {
		log.Fatalf("❌ Erro ao inicializar VectorRepo: %v", err)
	}

	// 4. Initialize LLM Provider (implementing domain.LLMProvider)
	var llmProvider domain.LLMProvider
	chosenLLM := strings.ToLower(strings.TrimSpace(*llmProviderFlag))

	// Attempt to prepare both Gemini and AGY for high availability
	var geminiProv *llm.GeminiProvider
	var geminiErr error
	if *geminiKeyFlag != "" || os.Getenv("GEMINI_API_KEY") != "" {
		geminiProv, geminiErr = llm.NewGeminiProvider(llm.GeminiConfig{
			APIKey:    *geminiKeyFlag,
			Model:     *geminiModelFlag,
			Telemetry: telemetryRepo,
		})
	}

	var agyDriver *llm.Driver
	var agyErr error
	agyDriver, agyErr = llm.NewDriver("", *agyModelFlag)
	if agyErr == nil {
		agyDriver.SetTelemetry(telemetryRepo)
	}

	switch chosenLLM {
	case "gemini":
		if geminiErr != nil || geminiProv == nil {
			log.Fatalf("❌ Erro ao inicializar Gemini API Provider: %v", geminiErr)
		}
		if agyDriver != nil {
			llmProvider = llm.NewFallbackProvider(geminiProv, fmt.Sprintf("Gemini API (%s)", *geminiModelFlag), agyDriver, fmt.Sprintf("AGY CLI (%s)", agyDriver.Model))
			fmt.Printf("✅ Provedor LLM pronto: Gemini API Direta (%s) com fallback automático para AGY CLI (%s)\n", *geminiModelFlag, agyDriver.Model)
		} else {
			llmProvider = geminiProv
			fmt.Printf("✅ Provedor LLM pronto: Google Gemini API Direta (Modelo: %s)\n", *geminiModelFlag)
		}

	case "agy":
		if agyErr != nil || agyDriver == nil {
			log.Fatalf("❌ Erro ao inicializar provedor AGY: %v", agyErr)
		}
		llmProvider = agyDriver
		fmt.Printf("✅ Provedor LLM pronto: AGY CLI Local (Modelo: %s)\n", agyDriver.Model)

	case "auto":
		fallthrough
	default:
		if geminiProv != nil && agyDriver != nil {
			llmProvider = llm.NewFallbackProvider(geminiProv, fmt.Sprintf("Gemini API (%s)", *geminiModelFlag), agyDriver, fmt.Sprintf("AGY CLI (%s)", agyDriver.Model))
			fmt.Printf("✅ Provedor LLM pronto: Híbrido Automático (Gemini API com Fallback transparente para AGY CLI [%s])\n", agyDriver.Model)
		} else if geminiProv != nil {
			llmProvider = geminiProv
			fmt.Printf("✅ Provedor LLM pronto: Google Gemini API Direta (Modelo: %s)\n", *geminiModelFlag)
		} else if agyDriver != nil {
			llmProvider = agyDriver
			fmt.Printf("✅ Provedor LLM pronto: AGY CLI Local (Modelo: %s)\n", agyDriver.Model)
		} else {
			log.Fatalf("❌ Nenhum provedor LLM funcional disponível: Gemini (%v), AGY (%v)", geminiErr, agyErr)
		}
	}

	// 5. Sync Vocab into Vector Repository
	if len(vocab) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		synced, err := vectorRepo.SyncVocab(ctx, vocab)
		cancel()
		if err != nil {
			fmt.Printf("⚠️ Aviso na sincronização de vetores: %v\n", err)
		} else if synced > 0 {
			fmt.Printf("🧠 RAG Semântico: %d novos embeddings gerados no banco local!\n", synced)
		} else {
			fmt.Printf("🧠 RAG Semântico pronto: %d palavras indexadas em cache\n", vectorRepo.Count(context.Background()))
		}
	}

	// 6. Index Tae Kim's Grammar Guide into local repository
	grammarDir := "data/knowledge/taekim/grammar"
	if _, err := os.Stat(grammarDir); err == nil {
		lessons, err := grammar.ParseDir(grammarDir)
		if err != nil {
			fmt.Printf("⚠️ Aviso ao analisar arquivos do Tae Kim: %v\n", err)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			synced, err := grammarRepo.SyncLessons(ctx, lessons)
			cancel()
			if err != nil {
				fmt.Printf("⚠️ Aviso ao sincronizar lições de gramática: %v\n", err)
			} else if synced > 0 {
				fmt.Printf("📖 Guia Tae Kim: %d lições indexadas com vetores semânticos no banco local!\n", synced)
			} else {
				fmt.Printf("📖 Guia Tae Kim pronto: %d lições disponíveis com links de referência\n", grammarRepo.Count(context.Background()))
			}
		}
	}

	// 7. Inject Dependencies into Core Engines
	quizEngine := quiz.NewEngine(llmProvider, promptRepo, vectorRepo, quizRepo, grammarRepo)
	chatService := chat.NewService(llmProvider, promptRepo, vectorRepo, chatRepo, grammarRepo)

	// 8. Inject Dependencies into Web Server
	srv, err := web.NewServer(web.Config{
		QuizEngine:  quizEngine,
		ChatService: chatService,
		Prompts:     promptRepo,
		QuizRepo:    quizRepo,
		ChatRepo:    chatRepo,
		VectorRepo:  vectorRepo,
		Telemetry:   telemetryRepo,
		GrammarRepo: grammarRepo,
		Vocab:       vocab,
		AnkiPath:    ankiPath,
	})
	if err != nil {
		log.Fatalf("❌ Erro ao criar servidor web: %v", err)
	}

	addr := fmt.Sprintf(":%s", *port)
	fmt.Printf("\n✨ Servidor rodando com sucesso!\n")
	fmt.Printf("🌐 Acesse no seu navegador: http://localhost:%s\n", *port)
	fmt.Printf("🛑 Pressione Ctrl+C para encerrar\n\n")

	httpServer := &http.Server{
		Addr:    addr,
		Handler: srv.Routes(),
	}

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("❌ Falha no servidor HTTP: %v", err)
	}
}
