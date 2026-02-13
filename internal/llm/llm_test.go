package llm

import (
	"strings"
	"testing"
)

func TestParseWriteFileArgs(t *testing.T) {
	t.Run("valid args", func(t *testing.T) {
		args, err := ParseWriteFileArgs(`{"path": "math.cs", "content": "using System;"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args.Path != "math.cs" {
			t.Errorf("Path = %q, want %q", args.Path, "math.cs")
		}
		if args.Content != "using System;" {
			t.Errorf("Content = %q, want %q", args.Content, "using System;")
		}
	})

	t.Run("missing path", func(t *testing.T) {
		_, err := ParseWriteFileArgs(`{"content": "using System;"}`)
		if err == nil {
			t.Error("expected error for missing path")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		_, err := ParseWriteFileArgs(`{"path": "", "content": "using System;"}`)
		if err == nil {
			t.Error("expected error for empty path")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseWriteFileArgs(`not json`)
		if err == nil {
			t.Error("expected error for invalid json")
		}
	})

	t.Run("content with newlines and escapes", func(t *testing.T) {
		args, err := ParseWriteFileArgs(`{"path": "test.go", "content": "line1\nline2\ttab"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(args.Content, "\n") {
			t.Error("expected content to contain newline")
		}
		if !strings.Contains(args.Content, "\t") {
			t.Error("expected content to contain tab")
		}
	})
}

func TestDefaultTools(t *testing.T) {
	tools := DefaultTools()
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}

	tool := tools[0]
	if tool.Type != "function" {
		t.Errorf("tool.Type = %q, want %q", tool.Type, "function")
	}
	if tool.Function.Name != "write_file" {
		t.Errorf("tool.Function.Name = %q, want %q", tool.Function.Name, "write_file")
	}

	params := tool.Function.Parameters
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties in parameters")
	}
	if _, ok := props["path"]; !ok {
		t.Error("expected 'path' property")
	}
	if _, ok := props["content"]; !ok {
		t.Error("expected 'content' property")
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("https://api.example.com/v1/", "sk-test", false, true)
	if client.baseURL != "https://api.example.com/v1" {
		t.Errorf("baseURL = %q, want trailing slash stripped", client.baseURL)
	}
	if client.apiKey != "sk-test" {
		t.Errorf("apiKey = %q, want %q", client.apiKey, "sk-test")
	}
}
