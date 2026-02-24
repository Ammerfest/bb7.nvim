package llm

import (
	"strings"
	"testing"
)

func requiredContains(params map[string]any, name string) bool {
	required, ok := params["required"]
	if !ok {
		return false
	}
	switch v := required.(type) {
	case []any:
		for _, item := range v {
			if s, _ := item.(string); s == name {
				return true
			}
		}
	case []string:
		for _, item := range v {
			if item == name {
				return true
			}
		}
	}
	return false
}

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
	t.Run("diffMode off", func(t *testing.T) {
		tools := DefaultTools("off")
		if len(tools) != 1 {
			t.Fatalf("len(tools) = %d, want 1", len(tools))
		}
		if tools[0].Function.Name != "write_file" {
			t.Errorf("tool name = %q, want %q", tools[0].Function.Name, "write_file")
		}
	})

	t.Run("diffMode search_replace", func(t *testing.T) {
		tools := DefaultTools("search_replace")
		if len(tools) != 2 {
			t.Fatalf("len(tools) = %d, want 2", len(tools))
		}
		if tools[0].Function.Name != "write_file" {
			t.Errorf("tools[0] = %q, want %q", tools[0].Function.Name, "write_file")
		}
		if tools[1].Function.Name != "edit_file" {
			t.Errorf("tools[1] = %q, want %q", tools[1].Function.Name, "edit_file")
		}
		// Verify search/replace schema has old_string
		params := tools[1].Function.Parameters
		props, ok := params["properties"].(map[string]any)
		if !ok {
			t.Fatal("expected properties")
		}
		if _, ok := props["old_string"]; !ok {
			t.Error("expected 'old_string' property in search_replace mode")
		}
		if !requiredContains(params, "file_id") {
			t.Error("expected 'file_id' in required list for search_replace mode")
		}
	})

	t.Run("diffMode anchored", func(t *testing.T) {
		tools := DefaultTools("anchored")
		if len(tools) != 2 {
			t.Fatalf("len(tools) = %d, want 2", len(tools))
		}
		if tools[0].Function.Name != "write_file" {
			t.Errorf("tools[0] = %q, want %q", tools[0].Function.Name, "write_file")
		}
		if tools[1].Function.Name != "edit_file" {
			t.Errorf("tools[1] = %q, want %q", tools[1].Function.Name, "edit_file")
		}
		// Verify anchored schema has changes
		params := tools[1].Function.Parameters
		props, ok := params["properties"].(map[string]any)
		if !ok {
			t.Fatal("expected properties")
		}
		if _, ok := props["changes"]; !ok {
			t.Error("expected 'changes' property in anchored mode")
		}
		if !requiredContains(params, "file_id") {
			t.Error("expected 'file_id' in required list for anchored mode")
		}
	})

	t.Run("strict modes expose edit_file only", func(t *testing.T) {
		strictModes := []string{"search_replace_strict", "search_replace_multi_strict", "anchored_strict"}
		for _, mode := range strictModes {
			tools := DefaultTools(mode)
			if len(tools) != 1 {
				t.Fatalf("%s: len(tools) = %d, want 1", mode, len(tools))
			}
			if tools[0].Function.Name != "edit_file" {
				t.Fatalf("%s: tool[0] = %q, want edit_file", mode, tools[0].Function.Name)
			}
		}
	})

	t.Run("write_file schema", func(t *testing.T) {
		tool := DefaultTools("off")[0]
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
}

func TestParseAnchoredEditArgs(t *testing.T) {
	t.Run("valid args", func(t *testing.T) {
		args, err := ParseAnchoredEditArgs(`{
			"file_id": "f000",
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
		args, err := ParseAnchoredEditArgs(`{
			"path": "main.go",
			"file_id": "f123",
			"changes": [{"start": ["a"], "end": ["b"], "content": ["c"]}]
		}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args.FileID != "f123" {
			t.Errorf("FileID = %q, want %q", args.FileID, "f123")
		}
		if len(args.Changes[0].End) != 1 || args.Changes[0].End[0] != "b" {
			t.Errorf("End = %v, want [b]", args.Changes[0].End)
		}
	})

	t.Run("null content becomes empty slice", func(t *testing.T) {
		args, err := ParseAnchoredEditArgs(`{
			"file_id": "f123",
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
		_, err := ParseAnchoredEditArgs(`{"changes": [{"start": ["a"], "content": ["b"]}]}`)
		if err == nil {
			t.Error("expected error for missing path")
		}
	})

	t.Run("no changes", func(t *testing.T) {
		_, err := ParseAnchoredEditArgs(`{"file_id":"f123","path": "x.go", "changes": []}`)
		if err == nil {
			t.Error("expected error for no changes")
		}
	})

	t.Run("empty start", func(t *testing.T) {
		_, err := ParseAnchoredEditArgs(`{"file_id":"f123","path": "x.go", "changes": [{"start": [], "content": ["b"]}]}`)
		if err == nil {
			t.Error("expected error for empty start")
		}
	})

	t.Run("start too long", func(t *testing.T) {
		_, err := ParseAnchoredEditArgs(`{"file_id":"f123","path": "x.go", "changes": [{"start": ["1","2","3","4","5","6","7","8","9","10","11"], "content": ["b"]}]}`)
		if err == nil {
			t.Error("expected error for start too long")
		}
	})

	t.Run("end too long", func(t *testing.T) {
		_, err := ParseAnchoredEditArgs(`{"file_id":"f123","path": "x.go", "changes": [{"start": ["a"], "end": ["1","2","3","4","5","6","7","8","9","10","11"], "content": ["b"]}]}`)
		if err == nil {
			t.Error("expected error for end too long")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseAnchoredEditArgs(`not json`)
		if err == nil {
			t.Error("expected error for invalid json")
		}
	})
}

func TestParseEditFileArgs(t *testing.T) {
	t.Run("valid args", func(t *testing.T) {
		args, err := ParseEditFileArgs(`{"file_id":"abc123","path": "main.go", "old_string": "hello", "new_string": "world"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args.Path != "main.go" {
			t.Errorf("Path = %q, want %q", args.Path, "main.go")
		}
		if args.OldString != "hello" {
			t.Errorf("OldString = %q, want %q", args.OldString, "hello")
		}
		if args.NewString != "world" {
			t.Errorf("NewString = %q, want %q", args.NewString, "world")
		}
		if args.ReplaceAll {
			t.Error("ReplaceAll should default to false")
		}
	})

	t.Run("with replace_all", func(t *testing.T) {
		args, err := ParseEditFileArgs(`{"file_id":"abc123","path": "main.go", "old_string": "a", "new_string": "b", "replace_all": true}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !args.ReplaceAll {
			t.Error("ReplaceAll should be true")
		}
	})

	t.Run("with file_id", func(t *testing.T) {
		args, err := ParseEditFileArgs(`{"path": "main.go", "file_id": "abc123", "old_string": "hello", "new_string": "world"}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if args.FileID != "abc123" {
			t.Errorf("FileID = %q, want %q", args.FileID, "abc123")
		}
	})

	t.Run("missing path", func(t *testing.T) {
		_, err := ParseEditFileArgs(`{"file_id":"abc123","old_string": "a", "new_string": "b"}`)
		if err == nil {
			t.Error("expected error for missing path")
		}
	})

	t.Run("empty old_string allowed", func(t *testing.T) {
		args, err := ParseEditFileArgs(`{"file_id":"abc123","path": "x.go", "old_string": "", "new_string": "b"}`)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if args.OldString != "" {
			t.Errorf("old_string = %q, want empty", args.OldString)
		}
	})

	t.Run("no-op", func(t *testing.T) {
		_, err := ParseEditFileArgs(`{"file_id":"abc123","path": "x.go", "old_string": "same", "new_string": "same"}`)
		if err == nil {
			t.Error("expected error for no-op")
		}
		if !strings.Contains(err.Error(), "no-op") {
			t.Errorf("error = %q, want to contain 'no-op'", err.Error())
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := ParseEditFileArgs(`not json`)
		if err == nil {
			t.Error("expected error for invalid json")
		}
	})
}

func TestParseEditFileMultiArgs(t *testing.T) {
	t.Run("valid args with file_id", func(t *testing.T) {
		args, err := ParseEditFileMultiArgs(`{
			"edits": [
				{
					"path": "main.go",
					"file_id": "abc123",
					"old_string": "hello",
					"new_string": "world"
				},
					{
						"path": "main.go",
						"file_id": "abc123",
						"old_string": "foo",
						"new_string": "bar"
					}
			]
		}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(args.Edits) != 2 {
			t.Fatalf("len(Edits) = %d, want 2", len(args.Edits))
		}
		if args.Edits[0].FileID != "abc123" {
			t.Errorf("Edits[0].FileID = %q, want %q", args.Edits[0].FileID, "abc123")
		}
	})

	t.Run("all no-op edits", func(t *testing.T) {
		_, err := ParseEditFileMultiArgs(`{
			"edits": [
				{"path": "main.go", "file_id":"abc123", "old_string": "same", "new_string": "same"}
			]
		}`)
		if err == nil {
			t.Error("expected error for all no-op edits")
		}
	})

}

func TestNewClient(t *testing.T) {
	client := NewClient("https://api.example.com/v1/", "sk-test", false, true, true)
	if client.baseURL != "https://api.example.com/v1" {
		t.Errorf("baseURL = %q, want trailing slash stripped", client.baseURL)
	}
	if client.apiKey != "sk-test" {
		t.Errorf("apiKey = %q, want %q", client.apiKey, "sk-test")
	}
	if !client.explicitCacheKey {
		t.Error("explicitCacheKey = false, want true")
	}
}
