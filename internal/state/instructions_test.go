package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProjectInstructions_DirectivesAndFences(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".bb7"), 0755); err != nil {
		t.Fatalf("mkdir .bb7: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "ARCHITECTURE.md"), []byte("ARCH\n"), 0644); err != nil {
		t.Fatalf("write ARCHITECTURE.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "GRAPHICS.md"), []byte("GRAPH\n"), 0644); err != nil {
		t.Fatalf("write docs/GRAPHICS.md: %v", err)
	}

	instructions := strings.Join([]string{
		"@@ comment",
		"@include ARCHITECTURE.md",
		"",
		"```",
		"@include docs/GRAPHICS.md",
		"```",
		"",
		"# Header",
		"@include \"docs/GRAPHICS.md\"",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ".bb7", projectInstructionsFilename), []byte(instructions), 0644); err != nil {
		t.Fatalf("write instructions: %v", err)
	}

	s := New()
	s.ProjectRoot = root
	got, err := s.LoadProjectInstructions()
	if err != nil {
		t.Fatalf("LoadProjectInstructions error: %v", err)
	}

	want := strings.Join([]string{
		"ARCH",
		"",
		"```",
		"@include docs/GRAPHICS.md",
		"```",
		"",
		"# Header",
		"GRAPH",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("unexpected output:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestLoadProjectInstructions_InvalidInclude(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".bb7"), 0755); err != nil {
		t.Fatalf("mkdir .bb7: %v", err)
	}
	instructions := "@include ../secret\n"
	if err := os.WriteFile(filepath.Join(root, ".bb7", projectInstructionsFilename), []byte(instructions), 0644); err != nil {
		t.Fatalf("write instructions: %v", err)
	}

	s := New()
	s.ProjectRoot = root
	_, err := s.LoadProjectInstructions()
	if err == nil || !strings.Contains(err.Error(), "include path escapes project root") {
		t.Fatalf("expected include escape error, got: %v", err)
	}
}

func TestLoadGlobalInstructions_StripsComments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".config", "bb7"), 0755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	content := strings.Join([]string{
		"@@ comment",
		"Hello",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(home, ".config", "bb7", globalInstructionsFilename), []byte(content), 0644); err != nil {
		t.Fatalf("write global instructions: %v", err)
	}

	s := New()
	s.ProjectRoot = t.TempDir()
	got, err := s.LoadGlobalInstructions()
	if err != nil {
		t.Fatalf("LoadGlobalInstructions error: %v", err)
	}
	want := "Hello\n"
	if got != want {
		t.Fatalf("unexpected global output:\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestLoadGlobalInstructions_EmptyAfterStripping(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".config", "bb7"), 0755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	content := strings.Join([]string{
		"@@ only comments",
		"@@ nothing else",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(home, ".config", "bb7", globalInstructionsFilename), []byte(content), 0644); err != nil {
		t.Fatalf("write global instructions: %v", err)
	}

	s := New()
	s.ProjectRoot = t.TempDir()
	got, err := s.LoadGlobalInstructions()
	if err != nil {
		t.Fatalf("LoadGlobalInstructions error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty after stripping comments, got: %q", got)
	}
}

func TestGetInstructionsInfo_Errors(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".bb7"), 0755); err != nil {
		t.Fatalf("mkdir .bb7: %v", err)
	}
	instructions := "@include missing.md\n"
	if err := os.WriteFile(filepath.Join(root, ".bb7", projectInstructionsFilename), []byte(instructions), 0644); err != nil {
		t.Fatalf("write instructions: %v", err)
	}

	s := New()
	s.ProjectRoot = root
	info := s.GetInstructionsInfo()
	if !info.ProjectExists {
		t.Fatalf("expected ProjectExists to be true")
	}
	if info.ProjectError == "" {
		t.Fatalf("expected ProjectError to be set")
	}
}

func TestPrepareInstructionsFile_CreatesDefault(t *testing.T) {
	root := t.TempDir()
	s := New()
	s.ProjectRoot = root
	path, err := s.PrepareInstructionsFile("project", "")
	if err != nil {
		t.Fatalf("PrepareInstructionsFile error: %v", err)
	}
	if path == "" {
		t.Fatalf("expected path to be returned")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read instructions: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "BB-7 project instructions") {
		t.Fatalf("default content missing header")
	}
	if !strings.Contains(content, "@include ARCHITECTURE.md") {
		t.Fatalf("default content missing include example")
	}
}

func TestStripComments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips @@ lines",
			input: "@@ comment\nHello\n@@ another\nWorld\n",
			want:  "Hello\nWorld\n",
		},
		{
			name:  "preserves @@ inside fences",
			input: "before\n```\n@@ inside fence\n```\nafter\n",
			want:  "before\n```\n@@ inside fence\n```\nafter\n",
		},
		{
			name:  "all comments returns empty-ish",
			input: "@@ only\n@@ comments\n",
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripComments(tt.input)
			if got != tt.want {
				t.Fatalf("StripComments:\n--- got ---\n%q\n--- want ---\n%q", got, tt.want)
			}
		})
	}
}

func TestBuildInstructionsBlock_WrapsContent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".config", "bb7"), 0755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "bb7", globalInstructionsFilename), []byte("global\n"), 0644); err != nil {
		t.Fatalf("write global instructions: %v", err)
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".bb7"), 0755); err != nil {
		t.Fatalf("mkdir .bb7: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".bb7", projectInstructionsFilename), []byte("project\n"), 0644); err != nil {
		t.Fatalf("write project instructions: %v", err)
	}

	s := New()
	s.ProjectRoot = root
	block, err := s.BuildInstructionsBlock()
	if err != nil {
		t.Fatalf("BuildInstructionsBlock error: %v", err)
	}
	if !strings.Contains(block, "<user-instructions source=\"~/.config/bb7/instructions.md\">") {
		t.Fatalf("missing global instructions tag")
	}
	if !strings.Contains(block, "<project-instructions source=\".bb7/instructions\">") {
		t.Fatalf("missing project instructions tag")
	}
	if !strings.Contains(block, "global\n") || !strings.Contains(block, "project\n") {
		t.Fatalf("missing instruction content")
	}
}
