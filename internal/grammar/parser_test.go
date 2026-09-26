package grammar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFile(t *testing.T) {
	tempDir := t.TempDir()
	sampleMD := `---
title: Conditionals
chapterNumber: 4
chapter: Essential Grammar
part: 9
---

## How to say "if" in Japanese

This whole section is dedicated to learning how to say "if" in Japanese.
`
	filePath := filepath.Join(tempDir, "conditionals.md")
	if err := os.WriteFile(filePath, []byte(sampleMD), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	lesson, err := ParseFile(tempDir, filePath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if lesson.Title != "Conditionals" {
		t.Errorf("expected title Conditionals, got %s", lesson.Title)
	}
	if lesson.ChapterNumber != 4 || lesson.Part != 9 {
		t.Errorf("expected chapter 4 part 9, got %d.%d", lesson.ChapterNumber, lesson.Part)
	}
	if lesson.SectionKey != "4.9" {
		t.Errorf("expected section key 4.9, got %s", lesson.SectionKey)
	}
	if lesson.URL != "https://guidetojapanese.org/learn/grammar/conditionals" {
		t.Errorf("unexpected URL: %s", lesson.URL)
	}
	if lesson.Reference == nil || lesson.Reference.SectionKey != "4.9" {
		t.Errorf("reference not set properly: %+v", lesson.Reference)
	}
}

func TestParseDirTaeKim(t *testing.T) {
	baseDir := filepath.Join("..", "..", "data", "knowledge", "taekim", "grammar")
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		t.Skip("Tae Kim grammar directory not present, skipping")
	}

	lessons, err := ParseDir(baseDir)
	if err != nil {
		t.Fatalf("ParseDir failed: %v", err)
	}

	if len(lessons) < 50 {
		t.Errorf("expected at least 50 lessons, got %d", len(lessons))
	}

	// Verify section keys and URLs exist on all lessons
	for _, l := range lessons {
		if l.Title == "" || l.URL == "" || l.SectionKey == "" {
			t.Errorf("invalid lesson parsed: %+v", l)
		}
	}
}
