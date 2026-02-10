package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.ProjectRoot != "" {
		t.Error("Expected empty ProjectRoot")
	}
	if s.ActiveChat != nil {
		t.Error("Expected nil ActiveChat")
	}
}

func TestProjectInit(t *testing.T) {
	tmpDir := t.TempDir()
	s := New()

	if err := s.ProjectInit(tmpDir); err != nil {
		t.Fatalf("ProjectInit failed: %v", err)
	}

	chatsDir := filepath.Join(tmpDir, ".bb7", "chats")
	if _, err := os.Stat(chatsDir); err != nil {
		t.Errorf("Expected chats directory to exist: %v", err)
	}
}

func TestProjectInitAlreadyInitialized(t *testing.T) {
	tmpDir := t.TempDir()
	s := New()

	if err := s.ProjectInit(tmpDir); err != nil {
		t.Fatalf("ProjectInit failed: %v", err)
	}

	// Second call should return ErrAlreadyInit
	err := s.ProjectInit(tmpDir)
	if err != ErrAlreadyInit {
		t.Errorf("Expected ErrAlreadyInit, got %v", err)
	}
}

func TestInit(t *testing.T) {
	tmpDir := t.TempDir()
	s := New()

	// Init without ProjectInit should fail
	err := s.Init(tmpDir)
	if err != ErrNotBB7Project {
		t.Errorf("Expected ErrNotBB7Project, got %v", err)
	}

	// Now init the project
	if err := s.ProjectInit(tmpDir); err != nil {
		t.Fatalf("ProjectInit failed: %v", err)
	}

	// Now Init should succeed
	if err := s.Init(tmpDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if s.ProjectRoot != tmpDir {
		t.Errorf("Expected ProjectRoot %q, got %q", tmpDir, s.ProjectRoot)
	}
}

func TestInitInvalidPath(t *testing.T) {
	s := New()

	err := s.Init("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestInitNotDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file.txt")
	os.WriteFile(filePath, []byte("test"), 0644)

	s := New()
	err := s.Init(filePath)
	if err == nil {
		t.Error("Expected error when path is not a directory")
	}
}

func TestInitialized(t *testing.T) {
	s := New()
	if s.Initialized() {
		t.Error("Expected Initialized() to be false before Init")
	}

	tmpDir := t.TempDir()
	s.ProjectInit(tmpDir)
	s.Init(tmpDir)
	if !s.Initialized() {
		t.Error("Expected Initialized() to be true after Init")
	}
}
