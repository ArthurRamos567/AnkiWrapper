package rag

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"unicode"
)

const VectorDim = 128

// SemanticDimensions maps core conceptual clusters to vector indices.
var semanticKeywords = map[int][]string{
	// 0-9: Lugares, Localidades, Transporte, Geografia
	0: {"lugar", "place", "location", "onde", "where", "localidade", "espaço", "ponto"},
	1: {"estação", "station", "trem", "train", "metrô", "subway", "aeroporto", "airport", "porto"},
	2: {"cidade", "city", "país", "country", "bairro", "town", "vila", "village", "rua", "street"},
	3: {"casa", "home", "house", "quarto", "room", "prédio", "building", "apartamento", "porta", "janela"},
	4: {"escola", "school", "universidade", "university", "faculdade", "aula", "classroom", "biblioteca"},
	5: {"restaurante", "restaurant", "loja", "shop", "mercado", "market", "hotel", "hospital", "banco", "parque"},

	// 10-19: Alimentação, Bebidas, Refeições
	10: {"comida", "food", "comer", "eat", "refeição", "meal", "almoço", "lunch", "jantar", "dinner", "café"},
	11: {"bebida", "drink", "beber", "água", "water", "chá", "tea", "cerveja", "beer", "suco", "juice"},
	12: {"carne", "meat", "peixe", "fish", "arroz", "rice", "fruta", "fruit", "legume", "vegetable", "doce"},

	// 20-29: Tempo, Horários, Calendário
	20: {"tempo", "time", "hora", "hour", "minuto", "minute", "segundo", "second", "quando", "when"},
	21: {"dia", "day", "hoje", "today", "amanhã", "tomorrow", "ontem", "yesterday", "semana", "week", "mês", "month", "ano", "year"},
	22: {"manhã", "morning", "tarde", "afternoon", "noite", "night", "agora", "now", "cedo", "early", "tarde", "late"},

	// 30-39: Pessoas, Família, Relações
	30: {"pessoa", "person", "gente", "people", "amigo", "friend", "humano", "alguém", "someone"},
	31: {"família", "family", "pai", "father", "mãe", "mother", "irmão", "brother", "irmã", "sister", "filho", "child"},
	32: {"professor", "teacher", "aluno", "student", "médico", "doctor", "trabalhador", "colega", "chefe"},

	// 40-49: Ações e Verbos Comuns
	40: {"fazer", "do", "make", "agir", "ir", "go", "vir", "come", "voltar", "return", "sair", "chegar"},
	41: {"ver", "see", "olhar", "look", "ouvir", "hear", "escutar", "listen", "falar", "speak", "dizer", "say"},
	42: {"comprar", "buy", "vender", "sell", "pagar", "pay", "usar", "use", "levar", "take", "dar", "give"},
	43: {"estudar", "study", "aprender", "learn", "ensinar", "teach", "ler", "read", "escrever", "write"},

	// 50-59: Gramática, Partículas e Estrutura
	50: {"partícula", "particle", "tópico", "topic", "sujeito", "subject", "objeto", "object", "marcador"},
	51: {"verbo", "verb", "adjetivo", "adjective", "substantivo", "noun", "conjugação", "forma", "passado", "negativo"},
	52: {"pergunta", "question", "dúvida", "por que", "why", "como", "how", "quanto", "how much"},
}

// EncodeVector converts []float32 to byte slice (BLOB for SQLite).
func EncodeVector(v []float32) []byte {
	buf := new(bytes.Buffer)
	for _, f := range v {
		_ = binary.Write(buf, binary.LittleEndian, f)
	}
	return buf.Bytes()
}

// DecodeVector decodes []byte from SQLite BLOB back to []float32.
func DecodeVector(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := 0; i < len(v); i++ {
		bits := binary.LittleEndian.Uint32(b[i*4 : (i+1)*4])
		v[i] = math.Float32frombits(bits)
	}
	return v
}

// CosineSimilarity computes cosine similarity between two normalized vectors.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float32
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(normA))) * float32(math.Sqrt(float64(normB))))
}

// EmbedText generates a normalized 128-dimensional dense semantic vector for a text string.
func EmbedText(text string) []float32 {
	v := make([]float32, VectorDim)
	lower := strings.ToLower(text)
	words := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	if len(words) == 0 {
		return v
	}

	// 1. Semantic category projection
	for dim, keywords := range semanticKeywords {
		for _, kw := range keywords {
			matched := false
			if strings.Contains(lower, kw) {
				v[dim] += 2.0
				matched = true
			}
			for _, w := range words {
				if w == kw {
					v[dim] += 3.0
					matched = true
				}
			}
			// Propagate activation to the cluster root (0, 10, 20, 30, 40, 50)
			if matched {
				clusterRoot := (dim / 10) * 10
				v[clusterRoot] += 1.0
			}
		}
	}

	// 2. Character n-gram hashing for broad subword matching (dimensions 60-127)
	for _, w := range words {
		runes := []rune(w)
		for i := 0; i < len(runes)-2; i++ {
			gram := string(runes[i : i+3])
			hash := fnv1a(gram)
			idx := 60 + int(hash%68)
			v[idx] += 1.0
		}
		// Also hash individual Kanji / Kana
		for _, r := range runes {
			if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
				idx := 60 + int(uint32(r)%68)
				v[idx] += 1.5
			}
		}
	}

	// 3. Normalize vector to unit length
	var sumSq float32
	for _, val := range v {
		sumSq += val * val
	}
	if sumSq > 0 {
		norm := float32(math.Sqrt(float64(sumSq)))
		for i := range v {
			v[i] /= norm
		}
	}

	return v
}

func fnv1a(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
