package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEstimateTokens_PotentialSavings(t *testing.T) {
	tmpDir := t.TempDir()
	s := New()

	// Initialize project
	if err := s.ProjectInit(tmpDir); err != nil {
		t.Fatalf("ProjectInit failed: %v", err)
	}
	if err := s.Init(tmpDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Create a new chat
	chat, err := s.ChatNew("")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	// Create a source file with known content
	// Using simple content so token estimation is predictable
	// The token estimator uses ~4 chars per token
	srcFile := filepath.Join(tmpDir, "test.go")
	originalContent := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n" // ~50 chars, ~12 tokens
	if err := os.WriteFile(srcFile, []byte(originalContent), 0644); err != nil {
		t.Fatalf("Failed to write source file: %v", err)
	}

	// Add the file to context
	if err := s.ContextAdd("test.go", originalContent); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	// First estimate: no output file, so no potential savings
	estimate1, err := s.EstimateTokens("system prompt", "")
	if err != nil {
		t.Fatalf("EstimateTokens failed: %v", err)
	}

	if estimate1.PotentialSavings != 0 {
		t.Errorf("Expected PotentialSavings=0 when no output files, got %d", estimate1.PotentialSavings)
	}

	// Now create an output file (simulating LLM modification)
	outputContent := "package main\n\nfunc main() {\n\tprintln(\"hello world\")\n\tprintln(\"goodbye\")\n}\n" // ~80 chars, ~20 tokens
	outputDir := filepath.Join(tmpDir, ".bb7", "chats", chat.ID, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "test.go"), []byte(outputContent), 0644); err != nil {
		t.Fatalf("Failed to write output file: %v", err)
	}

	// Second estimate: should have potential savings equal to original tokens
	estimate2, err := s.EstimateTokens("system prompt", "")
	if err != nil {
		t.Fatalf("EstimateTokens failed: %v", err)
	}

	// Find the file info for test.go
	var fileInfo *FileTokenInfo
	for i := range estimate2.Files {
		if estimate2.Files[i].Path == "test.go" {
			fileInfo = &estimate2.Files[i]
			break
		}
	}

	if fileInfo == nil {
		t.Fatal("Expected to find test.go in estimate files")
	}

	if !fileInfo.HasOutput {
		t.Error("Expected HasOutput=true for file with output")
	}

	// Key assertion: potential savings should equal original tokens, NOT output tokens
	// When user applies the output, the original file is replaced, saving original_tokens
	if estimate2.PotentialSavings != fileInfo.OriginalTokens {
		t.Errorf("Expected PotentialSavings=%d (original_tokens), got %d",
			fileInfo.OriginalTokens, estimate2.PotentialSavings)
	}

	// Also verify that total tokens includes both original and output
	expectedFileTokens := fileInfo.OriginalTokens + fileInfo.OutputTokens
	if fileInfo.Tokens != expectedFileTokens {
		t.Errorf("Expected file Tokens=%d (original+output), got %d",
			expectedFileTokens, fileInfo.Tokens)
	}
}

func TestEstimateTokens_MultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()
	s := New()

	if err := s.ProjectInit(tmpDir); err != nil {
		t.Fatalf("ProjectInit failed: %v", err)
	}
	if err := s.Init(tmpDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	chat, err := s.ChatNew("")
	if err != nil {
		t.Fatalf("NewChat failed: %v", err)
	}

	// Create two files with outputs and one without
	files := []struct {
		name      string
		original  string
		output    string
		hasOutput bool
	}{
		{"a.go", "package a // original a", "package a // modified a with more content", true},
		{"b.go", "package b // original b", "package b // modified b", true},
		{"c.go", "package c // original c", "", false}, // no output
	}

	outputDir := filepath.Join(tmpDir, ".bb7", "chats", chat.ID, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output dir: %v", err)
	}

	for _, f := range files {
		srcPath := filepath.Join(tmpDir, f.name)
		if err := os.WriteFile(srcPath, []byte(f.original), 0644); err != nil {
			t.Fatalf("Failed to write %s: %v", f.name, err)
		}
		if err := s.ContextAdd(f.name, f.original); err != nil {
			t.Fatalf("ContextAdd %s failed: %v", f.name, err)
		}
		if f.hasOutput {
			if err := os.WriteFile(filepath.Join(outputDir, f.name), []byte(f.output), 0644); err != nil {
				t.Fatalf("Failed to write output %s: %v", f.name, err)
			}
		}
	}

	estimate, err := s.EstimateTokens("system prompt", "")
	if err != nil {
		t.Fatalf("EstimateTokens failed: %v", err)
	}

	// Calculate expected savings: sum of original_tokens for files with output
	var expectedSavings int
	for _, fi := range estimate.Files {
		if fi.HasOutput {
			expectedSavings += fi.OriginalTokens
		}
	}

	if estimate.PotentialSavings != expectedSavings {
		t.Errorf("Expected PotentialSavings=%d (sum of original_tokens for M files), got %d",
			expectedSavings, estimate.PotentialSavings)
	}
}
