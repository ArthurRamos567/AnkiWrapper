package anki

import (
	"testing"
)

func TestReadAnkiVocab(t *testing.T) {
	path, err := FindAnkiCollection()
	if err != nil {
		t.Skipf("Anki collection not found locally: %v", err)
	}

	items, err := ReadAnkiVocab(path)
	if err != nil {
		t.Fatalf("Failed to read Anki vocab: %v", err)
	}

	if len(items) == 0 {
		t.Fatalf("Expected notes in collection, got 0")
	}

	t.Logf("Successfully read %d vocabulary notes from %s", len(items), path)
	t.Logf("Sample item 1: Word=%s, Reading=%s, Meaning=%s", items[0].Word, items[0].Reading, items[0].Meaning)
}
