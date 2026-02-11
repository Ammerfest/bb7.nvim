package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChatIndexRecoveryFromCorruptFile(t *testing.T) {
	s := setupTestState(t)

	chat, err := s.ChatNew("first")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	indexPath := filepath.Join(s.ProjectRoot, ".bb7", "chats", "index.json")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("index.json not created: %v", err)
	}

	if err := os.WriteFile(indexPath, []byte("{broken"), 0644); err != nil {
		t.Fatalf("Failed to corrupt index.json: %v", err)
	}

	chats, err := s.ChatList()
	if err != nil {
		t.Fatalf("ChatList failed: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("Expected 1 chat after recovery, got %d", len(chats))
	}
	if chats[0].ID != chat.ID {
		t.Errorf("Expected chat ID %q, got %q", chat.ID, chats[0].ID)
	}
}

func TestChatIndexRebuildWhenMissing(t *testing.T) {
	s := setupTestState(t)

	chat, err := s.ChatNew("first")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	indexPath := filepath.Join(s.ProjectRoot, ".bb7", "chats", "index.json")
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("Failed to remove index.json: %v", err)
	}

	chats, err := s.ChatList()
	if err != nil {
		t.Fatalf("ChatList failed: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("Expected 1 chat after rebuild, got %d", len(chats))
	}
	if chats[0].ID != chat.ID {
		t.Errorf("Expected chat ID %q, got %q", chat.ID, chats[0].ID)
	}
}
