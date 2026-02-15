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
	t.Run("diffMode false", func(t *testing.T) {
		tools := DefaultTools(false)
		if len(tools) != 1 {
			t.Fatalf("len(tools) = %d, want 1", len(tools))
		}
		if tools[0].Function.Name != "write_file" {
			t.Errorf("tool name = %q, want %q", tools[0].Function.Name, "write_file")
		}
	})

	t.Run("diffMode true", func(t *testing.T) {
		tools := DefaultTools(true)
		if len(tools) != 2 {
			t.Fatalf("len(tools) = %d, want 2", len(tools))
		}
		if tools[0].Function.Name != "write_file" {
			t.Errorf("tools[0] = %q, want %q", tools[0].Function.Name, "write_file")
		}
		if tools[1].Function.Name != "modify_file" {
			t.Errorf("tools[1] = %q, want %q", tools[1].Function.Name, "modify_file")
		}
	})

	t.Run("write_file schema", func(t *testing.T) {
		tool := DefaultTools(false)[0]
		if tool.Type != "function" {
			t.Errorf("tool.Type = %q, want %q", tool.Type, "function")
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
	})

	t.Run("modify_file schema", func(t *testing.T) {
		tool := DefaultTools(true)[1]
		if tool.Type != "function" {
			t.Errorf("tool.Type = %q, want %q", tool.Type, "function")
		}
		params := tool.Function.Parameters
		props, ok := params["properties"].(map[string]any)
		if !ok {
			t.Fatal("expected properties in parameters")
		}
		if _, ok := props["path"]; !ok {
			t.Error("expected 'path' property")
		}
		if _, ok := props["changes"]; !ok {
			t.Error("expected 'changes' property")
		}
	})
}

func TestParseModifyFileArgs(t *testing.T) {
	t.Run("valid args", func(t *testing.T) {
		args, err := ParseModifyFileArgs(`{
			"path": "main.go",
			"changes": [
				{
					"start": ["func hello()"],
					"content": ["func hello(name string)"]
				}
			]
		}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args.Path != "main.go" {
			t.Errorf("Path = %q, want %q", args.Path, "main.go")
		}
		if len(args.Changes) != 1 {
			t.Fatalf("len(Changes) = %d, want 1", len(args.Changes))
		}
		if len(args.Changes[0].Start) != 1 || args.Changes[0].Start[0] != "func hello()" {
			t.Errorf("Start = %v, want [func hello()]", args.Changes[0].Start)
		}
		if len(args.Changes[0].Content) != 1 {
			t.Errorf("Content = %v, want 1 element", args.Changes[0].Content)
		}
	})

	t.Run("with end anchor", func(t *testing.T) {
		args, err := ParseModifyFileArgs(`{
			"path": "main.go",
			"changes": [{"start": ["a"], "end": ["b"], "content": ["c"]}]
		}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(args.Changes[0].End) != 1 || args.Changes[0].End[0] != "b" {
			t.Errorf("End = %v, want [b]", args.Changes[0].End)
		}
	})

	t.Run("null content becomes empty slice", func(t *testing.T) {
		args, err := ParseModifyFileArgs(`{
			"path": "main.go",
			"changes": [{"start": ["a"], "content": null}]
		}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args.Changes[0].Content == nil {
			t.Error("expected Content to be normalized to empty slice, got nil")
		}
		if len(args.Changes[0].Content) != 0 {
			t.Errorf("len(Content) = %d, want 0", len(args.Changes[0].Content))
		}
	})

	t.Run("missing path", func(t *testing.T) {
		_, err := ParseModifyFileArgs(`{"changes": [{"start": ["a"], "content": ["b"]}]}`)
		if err == nil {
			t.Error("expected error for missing path")
		}
	})

	t.Run("no changes", func(t *testing.T) {
		_, err := ParseModifyFileArgs(`{"path": "x.go", "changes": []}`)
		if err == nil {
			t.Error("expected error for no changes")
		}
	})

	t.Run("empty start", func(t *testing.T) {
		_, err := ParseModifyFileArgs(`{"path": "x.go", "changes": [{"start": [], "content": ["b"]}]}`)
		if err == nil {
			t.Error("expected error for empty start")
		}
	})

	t.Run("start too long", func(t *testing.T) {
		_, err := ParseModifyFileArgs(`{"path": "x.go", "changes": [{"start": ["1","2","3","4","5"], "content": ["b"]}]}`)
		if err == nil {
			t.Error("expected error for start too long")
		}
	})

	t.Run("end too long", func(t *testing.T) {
		_, err := ParseModifyFileArgs(`{"path": "x.go", "changes": [{"start": ["a"], "end": ["1","2","3","4","5"], "content": ["b"]}]}`)
		if err == nil {
			t.Error("expected error for end too long")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseModifyFileArgs(`not json`)
		if err == nil {
			t.Error("expected error for invalid json")
		}
	})
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
