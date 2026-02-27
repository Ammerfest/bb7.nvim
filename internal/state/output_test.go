package state

import "testing"

func TestWriteOutputFile(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if err := s.WriteOutputFile("result.go", "package result"); err != nil {
		t.Fatalf("WriteOutputFile failed: %v", err)
	}

	content, err := s.GetOutputFile("result.go")
	if err != nil {
		t.Fatalf("GetOutputFile failed: %v", err)
	}

	if content != "package result" {
		t.Errorf("Expected 'package result', got %q", content)
	}
}

func TestWriteOutputFileOverwrite(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	s.WriteOutputFile("file.go", "version 1")
	s.WriteOutputFile("file.go", "version 2")

	content, _ := s.GetOutputFile("file.go")
	if content != "version 2" {
		t.Errorf("Expected 'version 2', got %q", content)
	}
}

func TestWriteOutputFileRequiresActiveChat(t *testing.T) {
	s := setupTestState(t)

	err := s.WriteOutputFile("file.go", "content")
	if err != ErrNoActiveChat {
		t.Errorf("Expected ErrNoActiveChat, got %v", err)
	}
}

func TestGetOutputFileNotFound(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	_, err := s.GetOutputFile("nonexistent.go")
	if err != ErrFileNotFound {
		t.Errorf("Expected ErrFileNotFound, got %v", err)
	}
}

func TestListOutputFiles(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	s.WriteOutputFile("a.go", "a")
	s.WriteOutputFile("b.go", "b")

	files, err := s.ListOutputFiles()
	if err != nil {
		t.Fatalf("ListOutputFiles failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(files))
	}
}

func TestListOutputFilesEmpty(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	files, err := s.ListOutputFiles()
	if err != nil {
		t.Fatalf("ListOutputFiles failed: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("Expected 0 files, got %d", len(files))
	}
}

func TestListOutputFilesRequiresActiveChat(t *testing.T) {
	s := setupTestState(t)

	_, err := s.ListOutputFiles()
	if err != ErrNoActiveChat {
		t.Errorf("Expected ErrNoActiveChat, got %v", err)
	}
}

func TestWriteOutputFileReadOnly(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Add external file (automatically read-only)
	s.ContextAdd("/etc/hosts", "content")

	// Attempt to write to read-only file
	err := s.WriteOutputFile("/etc/hosts", "modified")
	if err != ErrReadOnly {
		t.Errorf("Expected ErrReadOnly, got %v", err)
	}
}

func TestWriteOutputFilePathTraversal(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Attempt path traversal
	err := s.WriteOutputFile("../../../etc/passwd", "malicious")
	if err != ErrPathEscape {
		t.Errorf("Expected ErrPathEscape, got %v", err)
	}
}

func TestWriteOutputFileAbsoluteOutside(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Attempt to write to absolute path outside project
	err := s.WriteOutputFile("/etc/passwd", "malicious")
	if err != ErrPathEscape {
		t.Errorf("Expected ErrPathEscape, got %v", err)
	}
}

func TestWriteOutputFileNestedPath(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Write to nested path
	if err := s.WriteOutputFile("src/utils/new.go", "package utils"); err != nil {
		t.Fatalf("WriteOutputFile failed: %v", err)
	}

	// Verify content is retrievable
	content, err := s.GetOutputFile("src/utils/new.go")
	if err != nil {
		t.Fatalf("GetOutputFile failed: %v", err)
	}
	if content != "package utils" {
		t.Errorf("Expected 'package utils', got %q", content)
	}

	// Verify ListOutputFiles includes nested path
	files, err := s.ListOutputFiles()
	if err != nil {
		t.Fatalf("ListOutputFiles failed: %v", err)
	}
	found := false
	for _, f := range files {
		if f == "src/utils/new.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'src/utils/new.go' in output files, got %v", files)
	}
}

func TestFatalProjectEscapeExits(t *testing.T) {
	originalExit := exitProcess
	exited := false
	exitProcess = func(code int) { exited = true }
	defer func() { exitProcess = originalExit }()

	fatalProjectEscape("/project", "/outside")
	if !exited {
		t.Fatal("Expected fatalProjectEscape to exit the process")
	}
}

func TestGetOutputPath(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	// Write a file first
	if err := s.WriteOutputFile("result.go", "package result"); err != nil {
		t.Fatalf("WriteOutputFile failed: %v", err)
	}

	// Get the path
	path, err := s.GetOutputPath("result.go")
	if err != nil {
		t.Fatalf("GetOutputPath failed: %v", err)
	}

	// Path should be absolute and contain expected components
	if path == "" {
		t.Error("Expected non-empty path")
	}
	if path[0] != '/' {
		t.Errorf("Expected absolute path, got %q", path)
	}
}

func TestGetOutputPathNotFound(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	_, err := s.GetOutputPath("nonexistent.go")
	if err != ErrFileNotFound {
		t.Errorf("Expected ErrFileNotFound, got %v", err)
	}
}

func TestGetOutputPathRequiresActiveChat(t *testing.T) {
	s := setupTestState(t)

	_, err := s.GetOutputPath("file.go")
	if err != ErrNoActiveChat {
		t.Errorf("Expected ErrNoActiveChat, got %v", err)
	}
}

func TestGetLocalPath(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	path, err := s.GetLocalPath("src/main.go")
	if err != nil {
		t.Fatalf("GetLocalPath failed: %v", err)
	}

	// Path should be absolute and end with the relative path
	if path == "" {
		t.Error("Expected non-empty path")
	}
	if path[0] != '/' {
		t.Errorf("Expected absolute path, got %q", path)
	}
}

func TestGetLocalPathRequiresActiveChat(t *testing.T) {
	s := setupTestState(t)

	_, err := s.GetLocalPath("file.go")
	if err != ErrNoActiveChat {
		t.Errorf("Expected ErrNoActiveChat, got %v", err)
	}
}

func TestWriteOutputFileGlobalChat(t *testing.T) {
	s := setupGlobalTestState(t)
	s.ChatNewGlobal("test", "")

	err := s.WriteOutputFile("result.go", "package result")
	if err != ErrGlobalReadOnly {
		t.Errorf("Expected ErrGlobalReadOnly for global chat, got %v", err)
	}
}
