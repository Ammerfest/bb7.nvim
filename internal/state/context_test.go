package state

import (
	"path/filepath"
	"testing"
)

func TestContextAdd(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if err := s.ContextAdd("src/main.go", "package main"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	if len(s.ActiveChat.ContextFiles) != 1 {
		t.Fatalf("Expected 1 context file, got %d", len(s.ActiveChat.ContextFiles))
	}

	if s.ActiveChat.ContextFiles[0].Path != "src/main.go" {
		t.Errorf("Expected 'src/main.go', got %q", s.ActiveChat.ContextFiles[0].Path)
	}

	content, err := s.GetContextFile("src/main.go")
	if err != nil {
		t.Fatalf("GetContextFile failed: %v", err)
	}
	if content != "package main" {
		t.Errorf("Expected 'package main', got %q", content)
	}
}

func TestContextAddDuplicate(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	s.ContextAdd("foo.go", "content")
	err := s.ContextAdd("foo.go", "new content")
	if err != ErrFileExists {
		t.Errorf("Expected ErrFileExists, got %v", err)
	}
}

func TestContextAddDuplicateCanonicalPath(t *testing.T) {
	tests := []struct {
		name      string
		firstAbs  bool
		secondAbs bool
	}{
		{
			name:      "relative then absolute",
			firstAbs:  false,
			secondAbs: true,
		},
		{
			name:      "absolute then relative",
			firstAbs:  true,
			secondAbs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := setupTestState(t)
			s.ChatNew("test", "")

			firstPath := "foo.go"
			secondPath := "foo.go"
			if tt.firstAbs {
				firstPath = filepath.Join(s.ProjectRoot, "foo.go")
			}
			if tt.secondAbs {
				secondPath = filepath.Join(s.ProjectRoot, "foo.go")
			}

			if err := s.ContextAdd(firstPath, "content"); err != nil {
				t.Fatalf("first ContextAdd failed: %v", err)
			}
			if err := s.ContextAdd(secondPath, "new content"); err != ErrFileExists {
				t.Fatalf("expected ErrFileExists for canonical duplicate, got %v", err)
			}
		})
	}
}

func TestContextAddRequiresActiveChat(t *testing.T) {
	s := setupTestState(t)

	err := s.ContextAdd("foo.go", "content")
	if err != ErrNoActiveChat {
		t.Errorf("Expected ErrNoActiveChat, got %v", err)
	}
}

func TestContextRemove(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")
	s.ContextAdd("foo.go", "content")

	if err := s.ContextRemove("foo.go"); err != nil {
		t.Fatalf("ContextRemove failed: %v", err)
	}

	if len(s.ActiveChat.ContextFiles) != 0 {
		t.Errorf("Expected 0 context files, got %d", len(s.ActiveChat.ContextFiles))
	}
}

func TestContextRemoveNotFound(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	err := s.ContextRemove("nonexistent.go")
	if err != ErrFileNotFound {
		t.Errorf("Expected ErrFileNotFound, got %v", err)
	}
}

func TestContextList(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	s.ContextAdd("a.go", "a")
	s.ContextAdd("b.go", "b")
	s.ContextAdd("c.go", "c")

	files, err := s.ContextList()
	if err != nil {
		t.Fatalf("ContextList failed: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files))
	}
}

func TestContextListRequiresActiveChat(t *testing.T) {
	s := setupTestState(t)

	_, err := s.ContextList()
	if err != ErrNoActiveChat {
		t.Errorf("Expected ErrNoActiveChat, got %v", err)
	}
}

func TestGetContextFileNotFound(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	_, err := s.GetContextFile("nonexistent.go")
	if err != ErrFileNotFound {
		t.Errorf("Expected ErrFileNotFound, got %v", err)
	}
}

func TestContextAddPathTraversal(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Attempt path traversal
	err := s.ContextAdd("../../../etc/passwd", "malicious")
	if err != ErrPathEscape {
		t.Errorf("Expected ErrPathEscape, got %v", err)
	}

	// Ensure nothing was added
	if len(s.ActiveChat.ContextFiles) != 0 {
		t.Errorf("Expected 0 context files, got %d", len(s.ActiveChat.ContextFiles))
	}
}

func TestContextAddNestedPath(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Add file with subdirectory
	if err := s.ContextAdd("src/utils/helper.go", "package utils"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	// Verify path is preserved
	cf := s.ActiveChat.ContextFiles[0]
	if cf.Path != "src/utils/helper.go" {
		t.Errorf("Expected path 'src/utils/helper.go', got %q", cf.Path)
	}

	// Verify content is retrievable
	content, err := s.GetContextFile("src/utils/helper.go")
	if err != nil {
		t.Fatalf("GetContextFile failed: %v", err)
	}
	if content != "package utils" {
		t.Errorf("Expected 'package utils', got %q", content)
	}
}

func TestContextAddExternal(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Add external file (absolute path outside project)
	externalPath := "/etc/hosts"
	if err := s.ContextAdd(externalPath, "127.0.0.1 localhost"); err != nil {
		t.Fatalf("ContextAdd external failed: %v", err)
	}

	cf := s.ActiveChat.ContextFiles[0]

	// Verify it's marked as external and read-only
	if !cf.External {
		t.Error("Expected External=true for absolute path")
	}
	if !cf.ReadOnly {
		t.Error("Expected ReadOnly=true for external file")
	}
	if cf.Path != externalPath {
		t.Errorf("Expected path %q, got %q", externalPath, cf.Path)
	}

	// Verify IsReadOnly works
	if !s.IsReadOnly(externalPath) {
		t.Error("IsReadOnly should return true for external file")
	}

	// Verify content is retrievable
	content, err := s.GetContextFile(externalPath)
	if err != nil {
		t.Fatalf("GetContextFile failed: %v", err)
	}
	if content != "127.0.0.1 localhost" {
		t.Errorf("Expected '127.0.0.1 localhost', got %q", content)
	}
}

func TestContextAddInternalNotReadOnly(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	s.ContextAdd("main.go", "package main")

	cf := s.ActiveChat.ContextFiles[0]
	if cf.ReadOnly {
		t.Error("Expected ReadOnly=false for internal file")
	}
	if cf.External {
		t.Error("Expected External=false for internal file")
	}
}

func TestContextAddInternalReadOnly(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if err := s.ContextAddWithReadOnly("main.go", "package main", true); err != nil {
		t.Fatalf("ContextAddWithReadOnly failed: %v", err)
	}

	cf := s.ActiveChat.ContextFiles[0]
	if !cf.ReadOnly {
		t.Error("Expected ReadOnly=true for internal file")
	}
	if cf.External {
		t.Error("Expected External=false for internal file")
	}
}

func TestContextSetReadOnly(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if err := s.ContextAdd("main.go", "package main"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	if err := s.ContextSetReadOnly("main.go", true); err != nil {
		t.Fatalf("ContextSetReadOnly failed: %v", err)
	}

	if !s.ActiveChat.ContextFiles[0].ReadOnly {
		t.Error("Expected ReadOnly=true after toggle")
	}

	if err := s.ContextSetReadOnly("main.go", false); err != nil {
		t.Fatalf("ContextSetReadOnly failed: %v", err)
	}
	if s.ActiveChat.ContextFiles[0].ReadOnly {
		t.Error("Expected ReadOnly=false after toggle")
	}
}

func TestContextSetReadOnlyExternal(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	externalPath := "/etc/hosts"
	if err := s.ContextAdd(externalPath, "127.0.0.1 localhost"); err != nil {
		t.Fatalf("ContextAdd external failed: %v", err)
	}

	if err := s.ContextSetReadOnly(externalPath, false); err != ErrExternalReadOnly {
		t.Errorf("Expected ErrExternalReadOnly, got %v", err)
	}
}

func TestContextSetReadOnlyWithOutput(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if err := s.ContextAdd("main.go", "package main"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if err := s.WriteOutputFile("main.go", "package main\n"); err != nil {
		t.Fatalf("WriteOutputFile failed: %v", err)
	}

	if err := s.ContextSetReadOnly("main.go", true); err != ErrContextModified {
		t.Errorf("Expected ErrContextModified, got %v", err)
	}
}

func TestContextAddAddsEventAndVersion(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	content := "package main"
	if err := s.ContextAdd("main.go", content); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	cf := s.ActiveChat.ContextFiles[0]
	if cf.Version == "" {
		t.Error("Expected context file version to be set")
	}
	expectedVersion := HashFileVersion("main.go", content)
	if cf.Version != expectedVersion {
		t.Errorf("Expected version %q, got %q", expectedVersion, cf.Version)
	}

	if len(s.ActiveChat.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.ActiveChat.Messages))
	}
	part := s.ActiveChat.Messages[0].Parts[0]
	if part.Type != "context_event" {
		t.Fatalf("Expected context_event part, got %q", part.Type)
	}
	if part.Action != "UserAddFile" {
		t.Errorf("Expected action 'UserAddFile', got %q", part.Action)
	}
	if part.Version != cf.Version {
		t.Errorf("Expected version %q, got %q", cf.Version, part.Version)
	}
	if part.ReadOnly == nil || *part.ReadOnly {
		t.Errorf("Expected readonly=false, got %v", part.ReadOnly)
	}
	if part.External == nil || *part.External {
		t.Errorf("Expected external=false, got %v", part.External)
	}
}

func TestContextRemoveAddsEvent(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	content := "package main"
	if err := s.ContextAdd("main.go", content); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if err := s.ContextRemove("main.go"); err != nil {
		t.Fatalf("ContextRemove failed: %v", err)
	}

	if len(s.ActiveChat.Messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(s.ActiveChat.Messages))
	}
	part := s.ActiveChat.Messages[1].Parts[0]
	if part.Action != "UserRemoveFile" {
		t.Errorf("Expected action 'UserRemoveFile', got %q", part.Action)
	}
	expectedVersion := HashFileVersion("main.go", content)
	if part.Version != expectedVersion {
		t.Errorf("Expected version %q, got %q", expectedVersion, part.Version)
	}
}

func TestContextUpdateAddsEvent(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	original := "package main"
	updated := "package main\n\n// updated"
	if err := s.ContextAdd("main.go", original); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if err := s.ContextUpdate("main.go", updated); err != nil {
		t.Fatalf("ContextUpdate failed: %v", err)
	}

	if len(s.ActiveChat.Messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(s.ActiveChat.Messages))
	}
	part := s.ActiveChat.Messages[1].Parts[0]
	if part.Action != "UserWriteFile" {
		t.Errorf("Expected action 'UserWriteFile', got %q", part.Action)
	}
	expectedPrev := HashFileVersion("main.go", original)
	if part.PrevVersion != expectedPrev {
		t.Errorf("Expected prev_version %q, got %q", expectedPrev, part.PrevVersion)
	}
	expectedVersion := HashFileVersion("main.go", updated)
	if part.Version != expectedVersion {
		t.Errorf("Expected version %q, got %q", expectedVersion, part.Version)
	}
}

func TestContextSetReadOnlyAddsEvent(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	content := "package main"
	if err := s.ContextAdd("main.go", content); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if err := s.ContextSetReadOnly("main.go", true); err != nil {
		t.Fatalf("ContextSetReadOnly failed: %v", err)
	}

	if len(s.ActiveChat.Messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(s.ActiveChat.Messages))
	}
	part := s.ActiveChat.Messages[1].Parts[0]
	if part.Action != "UserSetReadOnly" {
		t.Errorf("Expected action 'UserSetReadOnly', got %q", part.Action)
	}
	expectedVersion := HashFileVersion("main.go", content)
	if part.Version != expectedVersion {
		t.Errorf("Expected version %q, got %q", expectedVersion, part.Version)
	}
	if part.ReadOnly == nil || !*part.ReadOnly {
		t.Errorf("Expected readonly=true, got %v", part.ReadOnly)
	}
}

func TestAssistantWriteFileAddsEvent(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	content := "package main"
	if err := s.AssistantWriteFile("main.go", content, true); err != nil {
		t.Fatalf("AssistantWriteFile failed: %v", err)
	}

	if len(s.ActiveChat.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.ActiveChat.Messages))
	}
	part := s.ActiveChat.Messages[0].Parts[0]
	if part.Action != "AssistantWriteFile" {
		t.Errorf("Expected action 'AssistantWriteFile', got %q", part.Action)
	}
	if !part.Added {
		t.Error("Expected Added to be true for new file")
	}
	expectedVersion := HashFileVersion("main.go", content)
	if part.Version != expectedVersion {
		t.Errorf("Expected version %q, got %q", expectedVersion, part.Version)
	}
}

func TestHasContextFile(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if s.HasContextFile("main.go") {
		t.Error("Expected HasContextFile to be false before add")
	}
	if err := s.ContextAdd("main.go", "package main"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if !s.HasContextFile("main.go") {
		t.Error("Expected HasContextFile to be true after add")
	}
}

func TestHasContextFileNoActiveChat(t *testing.T) {
	s := setupTestState(t)
	if s.HasContextFile("main.go") {
		t.Error("Expected HasContextFile to be false with no active chat")
	}
}

// Section tests

func TestContextAddSection(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	content := "line 2\nline 3\nline 4"
	if err := s.ContextAddSection("main.go", 2, 4, content); err != nil {
		t.Fatalf("ContextAddSection failed: %v", err)
	}

	if len(s.ActiveChat.ContextFiles) != 1 {
		t.Fatalf("Expected 1 context file, got %d", len(s.ActiveChat.ContextFiles))
	}

	cf := s.ActiveChat.ContextFiles[0]
	if cf.Path != "main.go" {
		t.Errorf("Expected path 'main.go', got %q", cf.Path)
	}
	if cf.StartLine != 2 {
		t.Errorf("Expected StartLine=2, got %d", cf.StartLine)
	}
	if cf.EndLine != 4 {
		t.Errorf("Expected EndLine=4, got %d", cf.EndLine)
	}
	if !cf.ReadOnly {
		t.Error("Expected section to be read-only")
	}
	if !cf.IsSection() {
		t.Error("Expected IsSection() to return true")
	}
}

func TestContextAddSectionInvalidLines(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Zero start line
	err := s.ContextAddSection("main.go", 0, 10, "content")
	if err == nil {
		t.Error("Expected error for zero start line")
	}

	// Negative end line
	err = s.ContextAddSection("main.go", 1, -5, "content")
	if err == nil {
		t.Error("Expected error for negative end line")
	}

	// Start > end
	err = s.ContextAddSection("main.go", 10, 5, "content")
	if err == nil {
		t.Error("Expected error for start > end")
	}
}

func TestContextAddSectionDuplicate(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	s.ContextAddSection("main.go", 2, 4, "content")
	err := s.ContextAddSection("main.go", 2, 4, "different content")
	if err != ErrFileExists {
		t.Errorf("Expected ErrFileExists for duplicate section, got %v", err)
	}
}

func TestContextAddSectionDuplicateCanonicalPath(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	relativePath := "main.go"
	absolutePath := filepath.Join(s.ProjectRoot, relativePath)

	if err := s.ContextAddSection(relativePath, 2, 4, "content"); err != nil {
		t.Fatalf("first ContextAddSection failed: %v", err)
	}
	if err := s.ContextAddSection(absolutePath, 2, 4, "different"); err != ErrFileExists {
		t.Fatalf("expected ErrFileExists for canonical duplicate section, got %v", err)
	}
}

func TestContextAddSectionOverlappingAllowed(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Add overlapping sections (allowed per spec)
	if err := s.ContextAddSection("main.go", 1, 10, "lines 1-10"); err != nil {
		t.Fatalf("First section failed: %v", err)
	}
	if err := s.ContextAddSection("main.go", 5, 15, "lines 5-15"); err != nil {
		t.Fatalf("Overlapping section failed: %v", err)
	}

	if len(s.ActiveChat.ContextFiles) != 2 {
		t.Errorf("Expected 2 sections, got %d", len(s.ActiveChat.ContextFiles))
	}
}

func TestContextAddSectionWithFullFile(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Add full file first
	if err := s.ContextAdd("main.go", "full content"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	// Add section of same file (allowed per spec)
	if err := s.ContextAddSection("main.go", 1, 5, "partial content"); err != nil {
		t.Fatalf("ContextAddSection failed: %v", err)
	}

	if len(s.ActiveChat.ContextFiles) != 2 {
		t.Errorf("Expected 2 entries (full + section), got %d", len(s.ActiveChat.ContextFiles))
	}

	// Verify full file is not a section
	if s.ActiveChat.ContextFiles[0].IsSection() {
		t.Error("Full file should not be a section")
	}
	// Verify section is a section
	if !s.ActiveChat.ContextFiles[1].IsSection() {
		t.Error("Section should be a section")
	}
}

func TestContextRemoveSection(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	s.ContextAddSection("main.go", 2, 4, "content")

	if err := s.ContextRemoveSection("main.go", 2, 4); err != nil {
		t.Fatalf("ContextRemoveSection failed: %v", err)
	}

	if len(s.ActiveChat.ContextFiles) != 0 {
		t.Errorf("Expected 0 context files, got %d", len(s.ActiveChat.ContextFiles))
	}
}

func TestContextRemoveSectionNotFound(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	err := s.ContextRemoveSection("main.go", 1, 5)
	if err != ErrFileNotFound {
		t.Errorf("Expected ErrFileNotFound, got %v", err)
	}
}

func TestContextRemoveSectionWrongLines(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	s.ContextAddSection("main.go", 2, 4, "content")

	// Try to remove with different lines
	err := s.ContextRemoveSection("main.go", 2, 5)
	if err != ErrFileNotFound {
		t.Errorf("Expected ErrFileNotFound for wrong lines, got %v", err)
	}
}

func TestContextSectionContent(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	content := "line 2\nline 3\nline 4"
	if err := s.ContextAddSection("main.go", 2, 4, content); err != nil {
		t.Fatalf("ContextAddSection failed: %v", err)
	}

	// GetContextFile should work for sections too
	retrieved, err := s.GetContextFile("main.go")
	if err != nil {
		t.Fatalf("GetContextFile failed: %v", err)
	}
	if retrieved != content {
		t.Errorf("Expected %q, got %q", content, retrieved)
	}
}

func TestContextSectionAddsEvent(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	content := "section content"
	if err := s.ContextAddSection("main.go", 10, 20, content); err != nil {
		t.Fatalf("ContextAddSection failed: %v", err)
	}

	if len(s.ActiveChat.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.ActiveChat.Messages))
	}

	part := s.ActiveChat.Messages[0].Parts[0]
	if part.Action != "UserAddSection" {
		t.Errorf("Expected action 'UserAddSection', got %q", part.Action)
	}
	if part.StartLine != 10 {
		t.Errorf("Expected StartLine=10, got %d", part.StartLine)
	}
	if part.EndLine != 20 {
		t.Errorf("Expected EndLine=20, got %d", part.EndLine)
	}
}

func TestFindContextSection(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	s.ContextAddSection("main.go", 1, 10, "content 1")
	s.ContextAddSection("main.go", 20, 30, "content 2")

	// Find first section
	cf := s.FindContextSection("main.go", 1, 10)
	if cf == nil {
		t.Fatal("Expected to find section 1-10")
	}
	if cf.StartLine != 1 || cf.EndLine != 10 {
		t.Errorf("Wrong section found: %d-%d", cf.StartLine, cf.EndLine)
	}

	// Find second section
	cf = s.FindContextSection("main.go", 20, 30)
	if cf == nil {
		t.Fatal("Expected to find section 20-30")
	}
	if cf.StartLine != 20 || cf.EndLine != 30 {
		t.Errorf("Wrong section found: %d-%d", cf.StartLine, cf.EndLine)
	}

	// Non-existent section
	cf = s.FindContextSection("main.go", 5, 15)
	if cf != nil {
		t.Error("Expected nil for non-existent section")
	}
}

func TestContextAddGlobalChat(t *testing.T) {
	s := setupGlobalTestState(t)
	s.ChatNewGlobal("test", "")

	// Add with absolute path — should be forced read-only and external
	absPath := "/tmp/test-global-context.go"
	if err := s.ContextAdd(absPath, "package test"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	if len(s.ActiveChat.ContextFiles) != 1 {
		t.Fatalf("Expected 1 context file, got %d", len(s.ActiveChat.ContextFiles))
	}

	cf := s.ActiveChat.ContextFiles[0]
	if !cf.ReadOnly {
		t.Error("Global chat context should be read-only")
	}
	if !cf.External {
		t.Error("Global chat context should be external")
	}
}

func TestContextAddGlobalChatRejectsRelativeWithoutProject(t *testing.T) {
	s := setupGlobalTestState(t)
	s.ChatNewGlobal("test", "")

	// Relative path with no project root should fail
	err := s.ContextAdd("main.go", "package main")
	if err == nil {
		t.Error("Expected error for relative path in global chat without project root")
	}
}
