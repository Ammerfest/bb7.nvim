package state

import (
	"os"
	"path/filepath"
	"testing"
)

// Security tests for path traversal, read-only enforcement, and boundary validation.
// These tests verify that malicious or malformed paths cannot escape the sandbox.

func TestSecurityPathTraversalVariants(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Paths that MUST be rejected (actual traversal)
	mustReject := []string{
		"../secret",
		"../../secret",
		"../../../etc/passwd",
		"../../../../etc/passwd",
		"src/../../../etc/passwd",
		"src/sub/../../../../../../etc/passwd",
		"./../../etc/passwd",
		"./../secret",
		"src/./../../../secret",
	}

	for _, path := range mustReject {
		err := s.ContextAdd(path, "malicious")
		if err != ErrPathEscape {
			t.Errorf("ContextAdd(%q): expected ErrPathEscape, got %v", path, err)
		}
	}

	// Paths that MUST be allowed (not traversal, just weird names)
	mustAllow := []string{
		"...",       // Three dots is a valid filename
		"....",      // Four dots is a valid filename
		"src/...",   // Valid filename in subdirectory
		"..hidden",  // File starting with two dots
		"file..go",  // Double dot in middle
	}

	for _, path := range mustAllow {
		err := s.ContextAdd(path, "content")
		if err != nil {
			t.Errorf("ContextAdd(%q): expected success, got %v", path, err)
		} else {
			s.ContextRemove(path) // Clean up
		}
	}
}

func TestSecurityWriteOutputTraversalVariants(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	traversalPaths := []string{
		"../secret.go",
		"../../secret.go",
		"../../../etc/passwd",
		"src/../../../etc/passwd",
		"src/sub/../../../../etc/passwd",
	}

	for _, path := range traversalPaths {
		err := s.WriteOutputFile(path, "malicious")
		if err != ErrPathEscape {
			t.Errorf("WriteOutputFile(%q): expected ErrPathEscape, got %v", path, err)
		}
	}
}

func TestSecurityNoFileCreatedOnTraversalAttempt(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Attempt traversal
	s.WriteOutputFile("../escape.txt", "malicious")

	// Verify no file was created in parent directory
	parentPath := filepath.Join(s.ProjectRoot, ".bb7", "escape.txt")
	if _, err := os.Stat(parentPath); !os.IsNotExist(err) {
		t.Errorf("File was created at escaped location: %s", parentPath)
		os.Remove(parentPath) // Clean up
	}

	// Also check the chats directory
	chatsEscape := filepath.Join(s.ProjectRoot, ".bb7", "chats", "escape.txt")
	if _, err := os.Stat(chatsEscape); !os.IsNotExist(err) {
		t.Errorf("File was created at escaped location: %s", chatsEscape)
		os.Remove(chatsEscape)
	}
}

func TestSecurityReadOnlyEnforcementComprehensive(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Add external file (automatically read-only)
	externalPath := "/usr/share/dict/words"
	s.ContextAdd(externalPath, "dictionary content")

	// 1. WriteOutputFile should reject
	err := s.WriteOutputFile(externalPath, "modified")
	if err != ErrReadOnly {
		t.Errorf("WriteOutputFile to external path: expected ErrReadOnly, got %v", err)
	}

	// 2. Verify IsReadOnly returns true
	if !s.IsReadOnly(externalPath) {
		t.Error("IsReadOnly should return true for external file")
	}

	// 3. Verify file status shows ReadOnly
	statuses, err := s.GetFileStatuses()
	if err != nil {
		t.Fatalf("GetFileStatuses failed: %v", err)
	}
	found := false
	for _, status := range statuses {
		if status.Path == externalPath {
			found = true
			if !status.ReadOnly {
				t.Error("FileInfo.ReadOnly should be true for external file")
			}
			if !status.External {
				t.Error("FileInfo.External should be true for external file")
			}
		}
	}
	if !found {
		t.Error("External file not found in file statuses")
	}
}

func TestSecurityAbsolutePathInProjectConvertsToRelative(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Absolute path that's actually inside the project should be converted
	absPath := filepath.Join(s.ProjectRoot, "src", "internal.go")

	err := s.ContextAdd(absPath, "package src")
	if err != nil {
		t.Fatalf("ContextAdd with absolute project path failed: %v", err)
	}

	// Should be stored as relative path
	cf := s.ActiveChat.ContextFiles[0]
	if cf.External {
		t.Error("Path inside project should not be marked as External")
	}
	if cf.ReadOnly {
		t.Error("Path inside project should not be marked as ReadOnly")
	}
	expectedRel := "src/internal.go"
	if cf.Path != expectedRel {
		t.Errorf("Expected relative path %q, got %q", expectedRel, cf.Path)
	}
}

func TestSecurityGetOutputFileTraversal(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Write a legitimate file first
	s.WriteOutputFile("legit.go", "package legit")

	// Try to read with traversal
	_, err := s.GetOutputFile("../../../etc/passwd")
	if err != ErrPathEscape {
		t.Errorf("GetOutputFile with traversal: expected ErrPathEscape, got %v", err)
	}
}

func TestSecurityDeleteOutputFileTraversal(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Try to delete with traversal
	err := s.DeleteOutputFile("../../../etc/passwd")
	if err != ErrPathEscape {
		t.Errorf("DeleteOutputFile with traversal: expected ErrPathEscape, got %v", err)
	}
}

func TestSecurityGetContextFileTraversal(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Add a legitimate file
	s.ContextAdd("legit.go", "package legit")

	// Try to read with traversal (should fail because not in context list)
	_, err := s.GetContextFile("../../../etc/passwd")
	if err != ErrFileNotFound {
		t.Errorf("GetContextFile with traversal: expected ErrFileNotFound, got %v", err)
	}
}

func TestSecurityMultipleFilesWithSameBasename(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Add two files with same basename but different paths
	s.ContextAdd("src/util.go", "package src")
	s.ContextAdd("test/util.go", "package test")

	// Verify both are stored and retrievable separately
	srcContent, err := s.GetContextFile("src/util.go")
	if err != nil {
		t.Fatalf("GetContextFile(src/util.go) failed: %v", err)
	}
	if srcContent != "package src" {
		t.Errorf("src/util.go content wrong: got %q", srcContent)
	}

	testContent, err := s.GetContextFile("test/util.go")
	if err != nil {
		t.Fatalf("GetContextFile(test/util.go) failed: %v", err)
	}
	if testContent != "package test" {
		t.Errorf("test/util.go content wrong: got %q", testContent)
	}

	// Verify we have 2 files
	if len(s.ActiveChat.ContextFiles) != 2 {
		t.Errorf("Expected 2 context files, got %d", len(s.ActiveChat.ContextFiles))
	}
}

func TestSecurityOutputFilesWithSameBasename(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Write two files with same basename but different paths
	s.WriteOutputFile("src/new.go", "package src")
	s.WriteOutputFile("test/new.go", "package test")

	// Verify both are stored and retrievable separately
	srcContent, err := s.GetOutputFile("src/new.go")
	if err != nil {
		t.Fatalf("GetOutputFile(src/new.go) failed: %v", err)
	}
	if srcContent != "package src" {
		t.Errorf("src/new.go content wrong: got %q", srcContent)
	}

	testContent, err := s.GetOutputFile("test/new.go")
	if err != nil {
		t.Fatalf("GetOutputFile(test/new.go) failed: %v", err)
	}
	if testContent != "package test" {
		t.Errorf("test/new.go content wrong: got %q", testContent)
	}

	// Verify ListOutputFiles shows both with full paths
	files, _ := s.ListOutputFiles()
	if len(files) != 2 {
		t.Errorf("Expected 2 output files, got %d: %v", len(files), files)
	}
}

func TestSecurityNullByteInPath(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Null byte injection attempt
	err := s.ContextAdd("file\x00.go", "malicious")
	if err != ErrInvalidPath {
		t.Errorf("ContextAdd with null byte: expected ErrInvalidPath, got %v", err)
	}

	err = s.WriteOutputFile("file\x00.go", "malicious")
	if err != ErrInvalidPath {
		t.Errorf("WriteOutputFile with null byte: expected ErrInvalidPath, got %v", err)
	}
}

func TestSecurityExternalFileCannotHaveOutput(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Add external file
	s.ContextAdd("/etc/hosts", "127.0.0.1 localhost")

	// Write a file with same path (should fail because read-only)
	err := s.WriteOutputFile("/etc/hosts", "malicious")
	if err != ErrReadOnly {
		t.Errorf("Expected ErrReadOnly when writing to external file path, got %v", err)
	}

	// Verify GetFileStatuses shows no output for external file
	statuses, _ := s.GetFileStatuses()
	for _, status := range statuses {
		if status.Path == "/etc/hosts" {
			if status.HasOutput {
				t.Error("External file should not have output")
			}
		}
	}
}

func TestSecurityWriteOutputRejectsSymlinkFileEscape(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	originalExit := exitProcess
	exitProcess = func(code int) {}
	defer func() { exitProcess = originalExit }()

	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("original"), 0644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	linkPath := filepath.Join(s.outputDir(s.ActiveChat.ID), "link.go")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	err := s.WriteOutputFile("link.go", "malicious")
	if err != ErrPathEscape {
		t.Fatalf("expected ErrPathEscape, got %v", err)
	}

	data, readErr := os.ReadFile(outsidePath)
	if readErr != nil {
		t.Fatalf("failed to read outside file: %v", readErr)
	}
	if string(data) != "original" {
		t.Fatalf("outside file was modified via symlink escape")
	}
}

func TestSecurityWriteOutputRejectsSymlinkDirectoryEscape(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	originalExit := exitProcess
	exitProcess = func(code int) {}
	defer func() { exitProcess = originalExit }()

	outsideDir := t.TempDir()
	linkDir := filepath.Join(s.outputDir(s.ActiveChat.ID), "linked")
	if err := os.Symlink(outsideDir, linkDir); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	err := s.WriteOutputFile(filepath.Join("linked", "escape.go"), "malicious")
	if err != ErrPathEscape {
		t.Fatalf("expected ErrPathEscape, got %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(outsideDir, "escape.go")); !os.IsNotExist(statErr) {
		t.Fatalf("outside file was created via symlink directory escape")
	}
}
