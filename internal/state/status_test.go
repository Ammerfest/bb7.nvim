package state

import (
	"os"
	"testing"
)

func TestGetFileStatuses_Empty(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 0 {
		t.Errorf("Expected 0 files, got %d", len(statuses))
	}
}

func TestGetFileStatuses_ContextOnly(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")
	s.ContextAdd("main.go", "package main")

	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(statuses))
	}

	f := statuses[0]
	if f.Path != "main.go" {
		t.Errorf("Expected path 'main.go', got %q", f.Path)
	}
	if f.Status != StatusUnchanged {
		t.Errorf("Expected status '', got %q", f.Status)
	}
	if !f.InContext {
		t.Error("Expected InContext=true")
	}
	if f.HasOutput {
		t.Error("Expected HasOutput=false")
	}
}

func TestGetFileStatuses_Modified(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")
	s.ContextAdd("main.go", "package main")
	s.WriteOutputFile("main.go", "package main\n\nfunc main() {}")

	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(statuses))
	}

	f := statuses[0]
	if f.Status != StatusModified {
		t.Errorf("Expected status 'M', got %q", f.Status)
	}
	if !f.InContext {
		t.Error("Expected InContext=true")
	}
	if !f.HasOutput {
		t.Error("Expected HasOutput=true")
	}
}

func TestGetFileStatuses_Added(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")
	s.WriteOutputFile("new_file.go", "package newfile")

	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(statuses))
	}

	f := statuses[0]
	if f.Path != "new_file.go" {
		t.Errorf("Expected path 'new_file.go', got %q", f.Path)
	}
	if f.Status != StatusAdded {
		t.Errorf("Expected status 'A', got %q", f.Status)
	}
	if f.InContext {
		t.Error("Expected InContext=false")
	}
	if !f.HasOutput {
		t.Error("Expected HasOutput=true")
	}
}

func TestGetFileStatuses_Applied(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	content := "package main\n\nfunc main() {}"
	s.ContextAdd("main.go", content)
	s.WriteOutputFile("main.go", content) // Same content = applied

	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(statuses))
	}

	f := statuses[0]
	if f.Status != StatusUnchanged {
		t.Errorf("Expected status '' (applied), got %q", f.Status)
	}
	if !f.HasOutput {
		t.Error("Expected HasOutput=true")
	}
}

func TestGetFileStatuses_AppliedWithLineEndingDiff(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	// Same content but different line endings should be considered applied
	s.ContextAdd("main.go", "line1\nline2")
	s.WriteOutputFile("main.go", "line1\r\nline2")

	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	f := statuses[0]
	if f.Status != StatusUnchanged {
		t.Errorf("Expected status '' (line endings normalized), got %q", f.Status)
	}
}

func TestGetFileStatuses_Mixed(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	// Unchanged context file
	s.ContextAdd("unchanged.go", "package unchanged")

	// Modified file
	s.ContextAdd("modified.go", "package modified")
	s.WriteOutputFile("modified.go", "package modified\n// comment")

	// Applied file
	s.ContextAdd("applied.go", "package applied")
	s.WriteOutputFile("applied.go", "package applied")

	// Added file (output only)
	s.WriteOutputFile("added.go", "package added")

	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 4 {
		t.Fatalf("Expected 4 files, got %d", len(statuses))
	}

	// Build map for easier checking
	byPath := make(map[string]FileInfo)
	for _, f := range statuses {
		byPath[f.Path] = f
	}

	if f := byPath["unchanged.go"]; f.Status != StatusUnchanged || f.HasOutput {
		t.Errorf("unchanged.go: expected status='', hasOutput=false, got %q, %v", f.Status, f.HasOutput)
	}
	if f := byPath["modified.go"]; f.Status != StatusModified {
		t.Errorf("modified.go: expected status='M', got %q", f.Status)
	}
	if f := byPath["applied.go"]; f.Status != StatusUnchanged || !f.HasOutput {
		t.Errorf("applied.go: expected status='', hasOutput=true, got %q, %v", f.Status, f.HasOutput)
	}
	if f := byPath["added.go"]; f.Status != StatusAdded || f.InContext {
		t.Errorf("added.go: expected status='A', inContext=false, got %q, %v", f.Status, f.InContext)
	}
}

func TestApplyFile(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")
	s.ContextAdd("main.go", "old content")
	s.WriteOutputFile("main.go", "new content")

	// Verify initial status is Modified
	statuses, _ := s.GetFileStatuses()
	if statuses[0].Status != StatusModified {
		t.Fatalf("Expected initial status 'M', got %q", statuses[0].Status)
	}

	// Apply the file
	content, err := s.ApplyFile("main.go")
	if err != nil {
		t.Fatalf("ApplyFile failed: %v", err)
	}

	if content != "new content" {
		t.Errorf("Expected content 'new content', got %q", content)
	}

	// Verify status is now Unchanged (applied)
	statuses, _ = s.GetFileStatuses()
	if statuses[0].Status != StatusUnchanged {
		t.Errorf("Expected status '' after apply, got %q", statuses[0].Status)
	}

	// Verify context was updated
	contextContent, _ := s.GetContextFile("main.go")
	if contextContent != "new content" {
		t.Errorf("Expected context 'new content', got %q", contextContent)
	}

	if len(s.ActiveChat.Messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(s.ActiveChat.Messages))
	}
	part := s.ActiveChat.Messages[1].Parts[0]
	if part.Action != "UserApplyFile" {
		t.Errorf("Expected action 'UserApplyFile', got %q", part.Action)
	}
}

func TestApplyFile_Added(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")
	s.WriteOutputFile("new.go", "new file content")

	// Verify initial status is Added
	statuses, _ := s.GetFileStatuses()
	if statuses[0].Status != StatusAdded {
		t.Fatalf("Expected initial status 'A', got %q", statuses[0].Status)
	}

	// Apply the file
	_, err := s.ApplyFile("new.go")
	if err != nil {
		t.Fatalf("ApplyFile failed: %v", err)
	}

	// Verify status is now Unchanged and file is in context
	statuses, _ = s.GetFileStatuses()
	f := statuses[0]
	if f.Status != StatusUnchanged {
		t.Errorf("Expected status '' after apply, got %q", f.Status)
	}
	if !f.InContext {
		t.Error("Expected InContext=true after apply")
	}

	if len(s.ActiveChat.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.ActiveChat.Messages))
	}
	part := s.ActiveChat.Messages[0].Parts[0]
	if part.Action != "UserApplyFile" {
		t.Errorf("Expected action 'UserApplyFile', got %q", part.Action)
	}
}

func TestApplyFile_NotFound(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	_, err := s.ApplyFile("nonexistent.go")
	if err != ErrFileNotFound {
		t.Errorf("Expected ErrFileNotFound, got %v", err)
	}
}

func TestConflictAdded_LocalFileExists(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	// Create a local file in the project root (simulating existing file)
	localPath := s.ProjectRoot + "/existing.go"
	if err := os.WriteFile(localPath, []byte("local content"), 0644); err != nil {
		t.Fatalf("Failed to create local file: %v", err)
	}

	// LLM writes to the same path (not in context)
	s.WriteOutputFile("existing.go", "llm content")

	// Verify status is ConflictAdded (!A)
	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(statuses))
	}

	if statuses[0].Status != StatusConflictAdded {
		t.Errorf("Expected status '!A', got %q", statuses[0].Status)
	}
}

func TestAdded_LocalFileDoesNotExist(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	// LLM writes a file that doesn't exist locally
	s.WriteOutputFile("brand_new.go", "llm content")

	// Verify status is Added (A), not ConflictAdded
	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(statuses))
	}

	if statuses[0].Status != StatusAdded {
		t.Errorf("Expected status 'A', got %q", statuses[0].Status)
	}
}

func TestDeleteOutputFile(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")
	s.WriteOutputFile("reject.go", "content")

	err := s.DeleteOutputFile("reject.go")
	if err != nil {
		t.Fatalf("DeleteOutputFile failed: %v", err)
	}

	_, err = s.GetOutputFile("reject.go")
	if err != ErrFileNotFound {
		t.Errorf("Expected ErrFileNotFound after delete, got %v", err)
	}
}

func TestDeleteOutputFile_NotFound(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	err := s.DeleteOutputFile("nonexistent.go")
	if err != ErrFileNotFound {
		t.Errorf("Expected ErrFileNotFound, got %v", err)
	}
}

func TestGetFileStatuses_RequiresActiveChat(t *testing.T) {
	s := setupTestState(t)

	_, err := s.GetFileStatuses()
	if err != ErrNoActiveChat {
		t.Errorf("Expected ErrNoActiveChat, got %v", err)
	}
}

func TestApplyFile_RequiresActiveChat(t *testing.T) {
	s := setupTestState(t)

	_, err := s.ApplyFile("file.go")
	if err != ErrNoActiveChat {
		t.Errorf("Expected ErrNoActiveChat, got %v", err)
	}
}

func TestGetFileStatuses_Section(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	s.ContextAddSection("main.go", 10, 20, "section content")

	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(statuses))
	}

	f := statuses[0]
	if f.Path != "main.go" {
		t.Errorf("Expected path 'main.go', got %q", f.Path)
	}
	if f.Status != StatusSection {
		t.Errorf("Expected status 'S', got %q", f.Status)
	}
	if f.StartLine != 10 {
		t.Errorf("Expected StartLine=10, got %d", f.StartLine)
	}
	if f.EndLine != 20 {
		t.Errorf("Expected EndLine=20, got %d", f.EndLine)
	}
	if !f.ReadOnly {
		t.Error("Expected ReadOnly=true for section")
	}
	if !f.InContext {
		t.Error("Expected InContext=true")
	}
	if f.HasOutput {
		t.Error("Expected HasOutput=false for section")
	}
}

func TestGetFileStatuses_MixedFullAndSection(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test")

	// Add full file
	s.ContextAdd("main.go", "full file content")
	// Add section of same file
	s.ContextAddSection("main.go", 5, 10, "section content")

	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}

	if len(statuses) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(statuses))
	}

	// First should be full file
	if statuses[0].Status != StatusUnchanged {
		t.Errorf("Expected first status '' (full file), got %q", statuses[0].Status)
	}
	if statuses[0].StartLine != 0 || statuses[0].EndLine != 0 {
		t.Error("Expected full file to have StartLine=0, EndLine=0")
	}

	// Second should be section
	if statuses[1].Status != StatusSection {
		t.Errorf("Expected second status 'S' (section), got %q", statuses[1].Status)
	}
	if statuses[1].StartLine != 5 || statuses[1].EndLine != 10 {
		t.Errorf("Expected section lines 5-10, got %d-%d", statuses[1].StartLine, statuses[1].EndLine)
	}
}
