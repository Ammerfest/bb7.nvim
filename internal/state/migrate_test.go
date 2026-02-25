package state

import (
	"encoding/json"
	"testing"
)

func TestMigrateChatV0ToV2(t *testing.T) {
	// Simulate a v0 chat JSON (no version field, uses Content on messages).
	raw := `{
		"id": "abc123",
		"name": "Old Chat",
		"created": "2025-01-01T00:00:00Z",
		"model": "test-model",
		"context_files": [],
		"messages": [
			{
				"role": "user",
				"content": "Hello world",
				"timestamp": "2025-01-01T00:00:01Z"
			},
			{
				"role": "assistant",
				"content": "Hi there!",
				"timestamp": "2025-01-01T00:00:02Z"
			},
			{
				"role": "assistant",
				"parts": [{"type": "text", "content": "Already migrated"}],
				"timestamp": "2025-01-01T00:00:03Z"
			}
		]
	}`

	var chat Chat
	if err := json.Unmarshal([]byte(raw), &chat); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Version should be 0 (missing field).
	if chat.Version != 0 {
		t.Errorf("Expected version 0, got %d", chat.Version)
	}

	// UnmarshalJSON should have migrated Content to Parts.
	if len(chat.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(chat.Messages))
	}

	// First message: legacy content should be in Parts now.
	msg0 := chat.Messages[0]
	if len(msg0.Parts) != 1 {
		t.Fatalf("Expected 1 part in msg[0], got %d", len(msg0.Parts))
	}
	if msg0.Parts[0].Type != PartTypeText {
		t.Errorf("Expected part type %q, got %q", PartTypeText, msg0.Parts[0].Type)
	}
	if msg0.Parts[0].Content != "Hello world" {
		t.Errorf("Expected content 'Hello world', got %q", msg0.Parts[0].Content)
	}

	// Second message: same migration.
	msg1 := chat.Messages[1]
	if len(msg1.Parts) != 1 || msg1.Parts[0].Content != "Hi there!" {
		t.Errorf("Expected msg[1] to have migrated content")
	}

	// Third message: already had Parts, should be untouched.
	msg2 := chat.Messages[2]
	if len(msg2.Parts) != 1 || msg2.Parts[0].Content != "Already migrated" {
		t.Errorf("Expected msg[2] to be unchanged")
	}

	// Now run migrateChat to bump version.
	changed := migrateChat(&chat)
	if !changed {
		t.Error("Expected migrateChat to report changes")
	}
	if chat.Version != CurrentChatVersion {
		t.Errorf("Expected version %d, got %d", CurrentChatVersion, chat.Version)
	}

	// Running again should be a no-op.
	if migrateChat(&chat) {
		t.Error("Expected migrateChat to be a no-op on already-migrated chat")
	}
}

func TestMigrateChatV2NoOp(t *testing.T) {
	chat := Chat{
		Version: CurrentChatVersion,
		Messages: []Message{{
			Role:  "user",
			Parts: []MessagePart{{Type: PartTypeText, Content: "hello"}},
		}},
	}

	if migrateChat(&chat) {
		t.Error("Expected no migration for current version")
	}
}

func TestMigrateChatEmptyContent(t *testing.T) {
	// A message with empty content and no parts should result in no parts.
	raw := `{
		"id": "test",
		"name": "Test",
		"created": "2025-01-01T00:00:00Z",
		"model": "m",
		"context_files": [],
		"messages": [
			{
				"role": "system",
				"content": "",
				"timestamp": "2025-01-01T00:00:01Z"
			}
		]
	}`

	var chat Chat
	if err := json.Unmarshal([]byte(raw), &chat); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(chat.Messages[0].Parts) != 0 {
		t.Errorf("Expected 0 parts for empty content, got %d", len(chat.Messages[0].Parts))
	}
}

func TestMessageUnmarshalPreservesFields(t *testing.T) {
	raw := `{
		"role": "user",
		"model": "test-model",
		"content": "Hello",
		"timestamp": "2025-01-01T00:00:01Z",
		"context_snapshot": [{"path": "foo.go", "file_id": "abc"}]
	}`

	var msg Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if msg.Role != "user" {
		t.Errorf("Expected role 'user', got %q", msg.Role)
	}
	if msg.Model != "test-model" {
		t.Errorf("Expected model 'test-model', got %q", msg.Model)
	}
	if len(msg.ContextSnapshot) != 1 {
		t.Errorf("Expected 1 context snapshot, got %d", len(msg.ContextSnapshot))
	}
	if len(msg.Parts) != 1 || msg.Parts[0].Content != "Hello" {
		t.Errorf("Expected content to be migrated to Parts")
	}
}
