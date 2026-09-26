package quiz

import "anki-wrapper/internal/domain"

// NewVocabCard represents a newly introduced vocabulary word formatted like an Anki card.
type NewVocabCard struct {
	Word         string `json:"word"`
	Reading      string `json:"reading"`
	Meaning      string `json:"meaning"`
	Sentence     string `json:"sentence"`
	SentenceHTML string `json:"sentence_html,omitempty"`
}

// Question represents a generated quiz item.
type Question struct {
	ID                string                   `json:"id"`
	Mode              string                   `json:"mode"` // "multiple_choice", "cloze", "free"
	Topic             string                   `json:"topic"`
	PromptText        string                   `json:"prompt_text"`
	Sentence          string                   `json:"sentence"`
	SentenceHTML      string                   `json:"sentence_html"`
	Options           []string                 `json:"options,omitempty"`
	CorrectAnswer     string                   `json:"correct_answer"`
	ReferenceAnswer   string                   `json:"reference_answer,omitempty"`
	Hint              string                   `json:"hint,omitempty"`
	Explanation       string                   `json:"explanation"`
	GrammarRef        *domain.GrammarReference `json:"grammar_ref,omitempty"`
	NewVocab          []NewVocabCard           `json:"new_vocab,omitempty"`
	MatchedVocabCount int                      `json:"matched_vocab_count"`
}

// EvaluationResult represents the evaluation of a user's answer.
type EvaluationResult struct {
	IsCorrect         bool                     `json:"is_correct"`
	Score             int                      `json:"score"` // 0 to 100
	UserAnswer        string                   `json:"user_answer"`
	CorrectAnswer     string                   `json:"correct_answer"`
	CorrectAnswerHTML string                   `json:"correct_answer_html"`
	Explanation       string                   `json:"explanation"`
	Feedback          string                   `json:"feedback"`
	GrammarRef        *domain.GrammarReference `json:"grammar_ref,omitempty"`
}

// Schemas for structured output from LLM:
const MultipleChoiceSchema = `{
  "type": "object",
  "properties": {
    "prompt_text": {"type": "string"},
    "sentence": {"type": "string"},
    "options": {
      "type": "array",
      "items": {"type": "string"},
      "minItems": 4,
      "maxItems": 4
    },
    "correct_option": {"type": "string"},
    "hint": {"type": "string"},
    "explanation": {"type": "string"}
  },
  "required": ["prompt_text", "sentence", "options", "correct_option", "explanation"]
}`

const ClozeSchema = `{
  "type": "object",
  "properties": {
    "prompt_text": {"type": "string"},
    "sentence": {"type": "string"},
    "correct_answer": {"type": "string"},
    "hint": {"type": "string"},
    "explanation": {"type": "string"}
  },
  "required": ["prompt_text", "sentence", "correct_answer", "hint", "explanation"]
}`

const FreeSchema = `{
  "type": "object",
  "properties": {
    "prompt_text": {"type": "string"},
    "reference_answer": {"type": "string"},
    "hint": {"type": "string"},
    "explanation": {"type": "string"}
  },
  "required": ["prompt_text", "reference_answer", "hint", "explanation"]
}`

const EvaluationSchema = `{
  "type": "object",
  "properties": {
    "is_correct": {"type": "boolean"},
    "score": {"type": "integer"},
    "corrected_sentence": {"type": "string"},
    "explanation": {"type": "string"},
    "feedback": {"type": "string"}
  },
  "required": ["is_correct", "score", "corrected_sentence", "explanation", "feedback"]
}`
