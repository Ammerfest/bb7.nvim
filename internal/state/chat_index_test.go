package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestChatIndexRecoveryFromCorruptFile(t *testing.T) {
	s := setupTestState(t)

	chat, err := s.ChatNew("first", "")
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

	chat, err := s.ChatNew("first", "")
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

// readActiveChatID reads the active_chat_id from the on-disk index.
func readActiveChatID(t *testing.T, projectRoot string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot, ".bb7", "chats", "index.json"))
	if err != nil {
		t.Fatalf("Failed to read index.json: %v", err)
	}
	var idx struct {
		ActiveChatID string `json:"active_chat_id"`
	}
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatalf("Failed to parse index.json: %v", err)
	}
	return idx.ActiveChatID
}

func TestActiveChatIDPersistedAfterChatNew(t *testing.T) {
	s := setupTestState(t)

	chat, err := s.ChatNew("test", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	got := readActiveChatID(t, s.ProjectRoot)
	if got != chat.ID {
		t.Errorf("Expected active_chat_id %q, got %q", chat.ID, got)
	}
}

func TestActiveChatIDPersistedAfterChatSelect(t *testing.T) {
	s := setupTestState(t)

	chat1, err := s.ChatNew("first", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}
	chat2, err := s.ChatNew("second", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	// Active should be chat2 (last created)
	if got := readActiveChatID(t, s.ProjectRoot); got != chat2.ID {
		t.Fatalf("Expected active_chat_id %q after ChatNew, got %q", chat2.ID, got)
	}

	// Select chat1
	if _, err := s.ChatSelect(chat1.ID); err != nil {
		t.Fatalf("ChatSelect failed: %v", err)
	}
	if got := readActiveChatID(t, s.ProjectRoot); got != chat1.ID {
		t.Errorf("Expected active_chat_id %q after ChatSelect, got %q", chat1.ID, got)
	}
}

func TestActiveChatIDClearedAfterDeleteActiveChat(t *testing.T) {
	s := setupTestState(t)

	chat, err := s.ChatNew("doomed", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	if err := s.ChatDelete(chat.ID); err != nil {
		t.Fatalf("ChatDelete failed: %v", err)
	}

	if got := readActiveChatID(t, s.ProjectRoot); got != "" {
		t.Errorf("Expected empty active_chat_id after delete, got %q", got)
	}
}

func TestInitRestoresActiveChat(t *testing.T) {
	s := setupTestState(t)

	chat, err := s.ChatNew("persistent", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	// Simulate restart: create a fresh State and Init with same project root.
	s2 := New()
	if err := s2.Init(s.ProjectRoot); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if s2.ActiveChat == nil {
		t.Fatal("Expected ActiveChat to be restored after Init")
	}
	if s2.ActiveChat.ID != chat.ID {
		t.Errorf("Expected restored chat ID %q, got %q", chat.ID, s2.ActiveChat.ID)
	}
}

func TestInitIgnoresDeletedActiveChat(t *testing.T) {
	s := setupTestState(t)

	chat, err := s.ChatNew("will-delete", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	// Delete the chat but leave index pointing to it.
	os.RemoveAll(s.chatDir(chat.ID))

	// Simulate restart.
	s2 := New()
	if err := s2.Init(s.ProjectRoot); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if s2.ActiveChat != nil {
		t.Errorf("Expected nil ActiveChat when referenced chat is deleted, got %q", s2.ActiveChat.ID)
	}
}
