package state

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestState(t *testing.T) *State {
	t.Helper()
	tmpDir := t.TempDir()
	s := New()
	if err := s.ProjectInit(tmpDir); err != nil {
		t.Fatalf("ProjectInit failed: %v", err)
	}
	if err := s.Init(tmpDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return s
}

func TestChatNew(t *testing.T) {
	s := setupTestState(t)

	chat, err := s.ChatNew("test-chat", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	if chat.ID == "" {
		t.Error("Expected chat ID to be set")
	}
	if chat.Name != "test-chat" {
		t.Errorf("Expected name 'test-chat', got %q", chat.Name)
	}
	if s.ActiveChat != chat {
		t.Error("Expected chat to be set as active")
	}

	chatDir := filepath.Join(s.ProjectRoot, ".bb7", "chats", chat.ID)
	if _, err := os.Stat(filepath.Join(chatDir, "chat.json")); err != nil {
		t.Errorf("chat.json not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chatDir, "context")); err != nil {
		t.Errorf("context dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(chatDir, "output")); err != nil {
		t.Errorf("output dir not created: %v", err)
	}
}

func TestChatNewRequiresInit(t *testing.T) {
	s := New()
	_, err := s.ChatNew("test", "")
	if err != ErrNotInitialized {
		t.Errorf("Expected ErrNotInitialized, got %v", err)
	}
}

func TestChatList(t *testing.T) {
	s := setupTestState(t)

	s.ChatNew("first", "")
	s.ChatNew("second", "")
	s.ChatNew("third", "")

	chats, err := s.ChatList()
	if err != nil {
		t.Fatalf("ChatList failed: %v", err)
	}

	if len(chats) != 3 {
		t.Errorf("Expected 3 chats, got %d", len(chats))
	}

	if chats[0].Name != "third" {
		t.Errorf("Expected newest first ('third'), got %q", chats[0].Name)
	}
}

func TestChatListEmpty(t *testing.T) {
	s := setupTestState(t)

	chats, err := s.ChatList()
	if err != nil {
		t.Fatalf("ChatList failed: %v", err)
	}

	if len(chats) != 0 {
		t.Errorf("Expected 0 chats, got %d", len(chats))
	}
}

func TestChatSelect(t *testing.T) {
	s := setupTestState(t)

	chat1, _ := s.ChatNew("first", "")
	chat2, _ := s.ChatNew("second", "")

	if s.ActiveChat.ID != chat2.ID {
		t.Error("Expected chat2 to be active after creation")
	}

	selected, err := s.ChatSelect(chat1.ID)
	if err != nil {
		t.Fatalf("ChatSelect failed: %v", err)
	}
	if selected.ID != chat1.ID {
		t.Error("Expected to select chat1")
	}
	if s.ActiveChat.ID != chat1.ID {
		t.Error("Expected chat1 to be active")
	}
}

func TestChatSelectNotFound(t *testing.T) {
	s := setupTestState(t)

	_, err := s.ChatSelect("nonexistent")
	if err != ErrChatNotFound {
		t.Errorf("Expected ErrChatNotFound, got %v", err)
	}
}

func TestAddSystemMessage(t *testing.T) {
	s := setupTestState(t)

	if _, err := s.ChatNew("test", ""); err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	if err := s.AddSystemMessage("Request timed out"); err != nil {
		t.Fatalf("AddSystemMessage failed: %v", err)
	}

	if s.ActiveChat == nil || len(s.ActiveChat.Messages) == 0 {
		t.Fatalf("Expected system message to be appended")
	}

	msg := s.ActiveChat.Messages[len(s.ActiveChat.Messages)-1]
	if msg.Role != "system" {
		t.Errorf("Expected system role, got %q", msg.Role)
	}
	if MessageText(msg) != "Request timed out" {
		t.Errorf("Expected content to be saved, got %q", MessageText(msg))
	}
}

func TestChatDelete(t *testing.T) {
	s := setupTestState(t)

	chat, _ := s.ChatNew("to-delete", "")
	chatDir := s.chatDir(chat.ID)

	if err := s.ChatDelete(chat.ID); err != nil {
		t.Fatalf("ChatDelete failed: %v", err)
	}

	if s.ActiveChat != nil {
		t.Error("Expected active chat to be cleared")
	}

	if _, err := os.Stat(chatDir); !os.IsNotExist(err) {
		t.Error("Expected chat directory to be removed")
	}
}

func TestChatDeleteNotFound(t *testing.T) {
	s := setupTestState(t)

	err := s.ChatDelete("nonexistent")
	if err != ErrChatNotFound {
		t.Errorf("Expected ErrChatNotFound, got %v", err)
	}
}

func TestChatRename(t *testing.T) {
	s := setupTestState(t)

	chat, _ := s.ChatNew("old-name", "")
	if err := s.ChatRename(chat.ID, "new-name"); err != nil {
		t.Fatalf("ChatRename failed: %v", err)
	}
	if s.ActiveChat.Name != "new-name" {
		t.Errorf("Expected active chat name to be updated, got %q", s.ActiveChat.Name)
	}
	loaded, err := s.loadChat(chat.ID)
	if err != nil {
		t.Fatalf("loadChat failed: %v", err)
	}
	if loaded.Name != "new-name" {
		t.Errorf("Expected stored chat name to be updated, got %q", loaded.Name)
	}
}

func TestChatRenameInactiveChat(t *testing.T) {
	s := setupTestState(t)

	chat1, _ := s.ChatNew("chat1", "")
	chat2, _ := s.ChatNew("chat2", "")

	if err := s.ChatRename(chat1.ID, "renamed"); err != nil {
		t.Fatalf("ChatRename failed: %v", err)
	}
	if s.ActiveChat.ID != chat2.ID {
		t.Errorf("Expected active chat to remain chat2, got %q", s.ActiveChat.ID)
	}
	if s.ActiveChat.Name != "chat2" {
		t.Errorf("Expected active chat name to remain unchanged, got %q", s.ActiveChat.Name)
	}
	loaded, err := s.loadChat(chat1.ID)
	if err != nil {
		t.Fatalf("loadChat failed: %v", err)
	}
	if loaded.Name != "renamed" {
		t.Errorf("Expected renamed chat to be updated, got %q", loaded.Name)
	}
}

func TestChatRenameNotFound(t *testing.T) {
	s := setupTestState(t)

	err := s.ChatRename("nonexistent", "new-name")
	if err != ErrChatNotFound {
		t.Errorf("Expected ErrChatNotFound, got %v", err)
	}
}

func TestChatRenameEmpty(t *testing.T) {
	s := setupTestState(t)

	chat, _ := s.ChatNew("chat", "")
	err := s.ChatRename(chat.ID, "   ")
	if err != ErrChatNameEmpty {
		t.Errorf("Expected ErrChatNameEmpty, got %v", err)
	}
}

func TestAddUserMessage(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if err := s.AddUserMessage("Hello", "openai/gpt-5.2"); err != nil {
		t.Fatalf("AddUserMessage failed: %v", err)
	}

	if len(s.ActiveChat.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.ActiveChat.Messages))
	}

	msg := s.ActiveChat.Messages[0]
	if msg.Role != "user" {
		t.Errorf("Expected role 'user', got %q", msg.Role)
	}
	if MessageText(msg) != "Hello" {
		t.Errorf("Expected content 'Hello', got %q", MessageText(msg))
	}
	if msg.Model != "openai/gpt-5.2" {
		t.Errorf("Expected model 'openai/gpt-5.2', got %q", msg.Model)
	}
}

func TestAddAssistantMessage(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	outputFiles := []string{"foo.go", "bar.go"}
	if err := s.AddAssistantMessage([]MessagePart{{Type: PartTypeText, Content: "Here are the changes"}}, outputFiles, "test-model", nil); err != nil {
		t.Fatalf("AddAssistantMessage failed: %v", err)
	}

	if len(s.ActiveChat.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.ActiveChat.Messages))
	}

	msg := s.ActiveChat.Messages[0]
	if msg.Role != "assistant" {
		t.Errorf("Expected role 'assistant', got %q", msg.Role)
	}
	if msg.Model != "test-model" {
		t.Errorf("Expected model 'test-model', got %q", msg.Model)
	}
	if len(msg.OutputFiles) != 2 {
		t.Errorf("Expected 2 output files, got %d", len(msg.OutputFiles))
	}
}

func TestAddAssistantMessageWithParts(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	parts := []MessagePart{
		{Type: PartTypeText, Content: "Here's the explanation"},
		{Type: PartTypeCode, Language: "go", Content: "fmt.Println(\"hello\")"},
		{Type: PartTypeFile, Path: "main.go"},
	}
	outputFiles := []string{"main.go"}
	if err := s.AddAssistantMessage(parts, outputFiles, "gpt-4", nil); err != nil {
		t.Fatalf("AddAssistantMessage failed: %v", err)
	}

	msg := s.ActiveChat.Messages[0]
	if len(msg.Parts) != 3 {
		t.Errorf("Expected 3 parts, got %d", len(msg.Parts))
	}
}

func TestAddMessageRequiresActiveChat(t *testing.T) {
	s := setupTestState(t)

	err := s.AddUserMessage("Hello", "")
	if err != ErrNoActiveChat {
		t.Errorf("Expected ErrNoActiveChat, got %v", err)
	}
}

func TestEditUserMessageTruncates(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if err := s.AddUserMessage("first", "model"); err != nil {
		t.Fatalf("AddUserMessage failed: %v", err)
	}
	if err := s.AddAssistantMessage([]MessagePart{{Type: PartTypeText, Content: "resp1"}}, []string{"out1.txt"}, "model", nil); err != nil {
		t.Fatalf("AddAssistantMessage failed: %v", err)
	}
	if err := s.WriteOutputFile("out1.txt", "one"); err != nil {
		t.Fatalf("WriteOutputFile failed: %v", err)
	}
	if err := s.AddUserMessage("second", "model"); err != nil {
		t.Fatalf("AddUserMessage failed: %v", err)
	}
	if err := s.AddAssistantMessage([]MessagePart{{Type: PartTypeText, Content: "resp2"}}, []string{"out2.txt"}, "model", nil); err != nil {
		t.Fatalf("AddAssistantMessage failed: %v", err)
	}
	if err := s.WriteOutputFile("out2.txt", "two"); err != nil {
		t.Fatalf("WriteOutputFile failed: %v", err)
	}

	warnings, err := s.EditUserMessage(2, "second edited")
	if err != nil {
		t.Fatalf("EditUserMessage failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("Expected no warnings, got %d", len(warnings))
	}

	if len(s.ActiveChat.Messages) != 2 {
		t.Fatalf("Expected 2 messages after edit, got %d", len(s.ActiveChat.Messages))
	}
	if s.ActiveChat.Draft != "second edited" {
		t.Errorf("Expected draft to be updated, got %q", s.ActiveChat.Draft)
	}

	if _, err := s.GetOutputFile("out1.txt"); err != nil {
		t.Errorf("Expected out1.txt to remain, got error: %v", err)
	}
	if _, err := s.GetOutputFile("out2.txt"); err == nil {
		t.Error("Expected out2.txt to be removed")
	} else if !os.IsNotExist(err) && err != ErrFileNotFound {
		t.Errorf("Expected out2.txt to be missing, got error: %v", err)
	}
}

func TestEditUserMessageInvalidIndex(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if err := s.AddUserMessage("only", "model"); err != nil {
		t.Fatalf("AddUserMessage failed: %v", err)
	}

	if _, err := s.EditUserMessage(2, "edited"); err == nil {
		t.Fatal("Expected EditUserMessage to fail for out-of-range index")
	}
}

func TestEditUserMessageNonUser(t *testing.T) {
	s := setupTestState(t)
	s.ChatNew("test", "")

	if err := s.AddUserMessage("first", "model"); err != nil {
		t.Fatalf("AddUserMessage failed: %v", err)
	}
	if err := s.AddAssistantMessage([]MessagePart{{Type: PartTypeText, Content: "resp1"}}, nil, "model", nil); err != nil {
		t.Fatalf("AddAssistantMessage failed: %v", err)
	}

	if _, err := s.EditUserMessage(1, "edited"); err == nil {
		t.Fatal("Expected EditUserMessage to fail for non-user message")
	}
}

// Test that context files are isolated between chats
func TestContextIsolationBetweenChats(t *testing.T) {
	s := setupTestState(t)

	// Create first chat and add context files
	chat1, _ := s.ChatNew("chat1", "")
	s.ContextAdd("file1.go", "package chat1")
	s.ContextAdd("shared.go", "package shared_v1")

	// Create second chat and add different context files
	chat2, _ := s.ChatNew("chat2", "")
	s.ContextAdd("file2.go", "package chat2")
	s.ContextAdd("shared.go", "package shared_v2") // Same path, different content

	// Verify chat2 has its own context
	files2, _ := s.ContextList()
	if len(files2) != 2 {
		t.Errorf("Chat2 expected 2 files, got %d", len(files2))
	}
	content2, _ := s.GetContextFile("shared.go")
	if content2 != "package shared_v2" {
		t.Errorf("Chat2 shared.go expected 'package shared_v2', got %q", content2)
	}

	// Switch back to chat1 and verify its context is preserved
	s.ChatSelect(chat1.ID)
	files1, _ := s.ContextList()
	if len(files1) != 2 {
		t.Errorf("Chat1 expected 2 files, got %d", len(files1))
	}
	content1, _ := s.GetContextFile("shared.go")
	if content1 != "package shared_v1" {
		t.Errorf("Chat1 shared.go expected 'package shared_v1', got %q", content1)
	}

	// Verify file1.go exists in chat1 but not file2.go
	_, err := s.GetContextFile("file1.go")
	if err != nil {
		t.Error("Chat1 should have file1.go")
	}
	_, err = s.GetContextFile("file2.go")
	if err != ErrFileNotFound {
		t.Error("Chat1 should NOT have file2.go")
	}

	// Switch to chat2 and verify inverse
	s.ChatSelect(chat2.ID)
	_, err = s.GetContextFile("file2.go")
	if err != nil {
		t.Error("Chat2 should have file2.go")
	}
	_, err = s.GetContextFile("file1.go")
	if err != ErrFileNotFound {
		t.Error("Chat2 should NOT have file1.go")
	}
}

// Test that messages are isolated between chats
func TestMessageIsolationBetweenChats(t *testing.T) {
	s := setupTestState(t)

	// Create first chat with messages
	chat1, _ := s.ChatNew("chat1", "")
	s.AddUserMessage("Hello from chat1", "")
	s.AddAssistantMessage([]MessagePart{{Type: PartTypeText, Content: "Response in chat1"}}, nil, "model1", nil)

	// Create second chat with different messages
	s.ChatNew("chat2", "")
	s.AddUserMessage("Hello from chat2", "")

	// Verify chat2 messages
	if len(s.ActiveChat.Messages) != 1 {
		t.Errorf("Chat2 expected 1 message, got %d", len(s.ActiveChat.Messages))
	}
	if MessageText(s.ActiveChat.Messages[0]) != "Hello from chat2" {
		t.Error("Chat2 first message wrong")
	}

	// Switch to chat1 and verify its messages
	s.ChatSelect(chat1.ID)
	if len(s.ActiveChat.Messages) != 2 {
		t.Errorf("Chat1 expected 2 messages, got %d", len(s.ActiveChat.Messages))
	}
	if MessageText(s.ActiveChat.Messages[0]) != "Hello from chat1" {
		t.Error("Chat1 first message wrong")
	}
}

// Test that output files are isolated between chats
func TestOutputIsolationBetweenChats(t *testing.T) {
	s := setupTestState(t)

	// Create first chat with output
	chat1, _ := s.ChatNew("chat1", "")
	s.ContextAdd("main.go", "package main_original")
	s.WriteOutputFile("main.go", "package main_modified_chat1")

	// Create second chat with different output
	s.ChatNew("chat2", "")
	s.ContextAdd("main.go", "package main_original")
	s.WriteOutputFile("main.go", "package main_modified_chat2")

	// Verify chat2 output
	output2, _ := s.GetOutputFile("main.go")
	if output2 != "package main_modified_chat2" {
		t.Errorf("Chat2 output expected 'package main_modified_chat2', got %q", output2)
	}

	// Switch to chat1 and verify its output is preserved
	s.ChatSelect(chat1.ID)
	output1, _ := s.GetOutputFile("main.go")
	if output1 != "package main_modified_chat1" {
		t.Errorf("Chat1 output expected 'package main_modified_chat1', got %q", output1)
	}
}

// Test that operations always affect the active chat
func TestOperationsAffectActiveChat(t *testing.T) {
	s := setupTestState(t)

	// Create two chats
	chat1, _ := s.ChatNew("chat1", "")
	chat2, _ := s.ChatNew("chat2", "") // chat2 becomes active

	// Add file to chat2 (currently active)
	s.ContextAdd("added_to_chat2.go", "package chat2")

	// Switch to chat1 and add file
	s.ChatSelect(chat1.ID)
	s.ContextAdd("added_to_chat1.go", "package chat1")

	// Verify chat1 has only its file
	files1, _ := s.ContextList()
	if len(files1) != 1 {
		t.Errorf("Chat1 expected 1 file, got %d", len(files1))
	}
	if files1[0].Path != "added_to_chat1.go" {
		t.Errorf("Chat1 expected 'added_to_chat1.go', got %q", files1[0].Path)
	}

	// Verify chat2 has only its file
	s.ChatSelect(chat2.ID)
	files2, _ := s.ContextList()
	if len(files2) != 1 {
		t.Errorf("Chat2 expected 1 file, got %d", len(files2))
	}
	if files2[0].Path != "added_to_chat2.go" {
		t.Errorf("Chat2 expected 'added_to_chat2.go', got %q", files2[0].Path)
	}
}

// Test that ChatSelect properly reloads chat state from disk
func TestChatSelectReloadsFromDisk(t *testing.T) {
	s := setupTestState(t)

	// Create chat and add content
	chat1, _ := s.ChatNew("chat1", "")
	s.ContextAdd("file.go", "content")
	s.AddUserMessage("test message", "")

	// Create another chat (this switches active away from chat1)
	s.ChatNew("other", "")

	// Switch back to chat1 - state should be reloaded from disk
	loaded, _ := s.ChatSelect(chat1.ID)

	// Verify context files were reloaded
	if len(loaded.ContextFiles) != 1 {
		t.Errorf("Expected 1 context file after reload, got %d", len(loaded.ContextFiles))
	}

	// Verify messages were reloaded (context event + user message)
	if len(loaded.Messages) != 2 {
		t.Errorf("Expected 2 messages after reload, got %d", len(loaded.Messages))
	}
	foundUser := false
	for _, msg := range loaded.Messages {
		if msg.Role == "user" && MessageText(msg) == "test message" {
			foundUser = true
		}
	}
	if !foundUser {
		t.Errorf("Expected user message 'test message' after reload")
	}
}

// Test that no active chat means operations fail gracefully
func TestNoActiveChatOperationsFail(t *testing.T) {
	s := setupTestState(t)

	// Create and then delete a chat to have no active chat
	chat, _ := s.ChatNew("temp", "")
	s.ChatDelete(chat.ID)

	// All these should fail with ErrNoActiveChat
	if err := s.ContextAdd("file.go", "content"); err != ErrNoActiveChat {
		t.Errorf("ContextAdd expected ErrNoActiveChat, got %v", err)
	}
	if err := s.ContextRemove("file.go"); err != ErrNoActiveChat {
		t.Errorf("ContextRemove expected ErrNoActiveChat, got %v", err)
	}
	if _, err := s.ContextList(); err != ErrNoActiveChat {
		t.Errorf("ContextList expected ErrNoActiveChat, got %v", err)
	}
	if _, err := s.GetContextFile("file.go"); err != ErrNoActiveChat {
		t.Errorf("GetContextFile expected ErrNoActiveChat, got %v", err)
	}
	if err := s.WriteOutputFile("file.go", "content"); err != ErrNoActiveChat {
		t.Errorf("WriteOutputFile expected ErrNoActiveChat, got %v", err)
	}
	if _, err := s.GetOutputFile("file.go"); err != ErrNoActiveChat {
		t.Errorf("GetOutputFile expected ErrNoActiveChat, got %v", err)
	}
	if err := s.AddUserMessage("msg", ""); err != ErrNoActiveChat {
		t.Errorf("AddUserMessage expected ErrNoActiveChat, got %v", err)
	}
	if err := s.AddAssistantMessage(nil, nil, "model", nil); err != ErrNoActiveChat {
		t.Errorf("AddAssistantMessage expected ErrNoActiveChat, got %v", err)
	}
	if _, err := s.GetFileStatuses(); err != ErrNoActiveChat {
		t.Errorf("GetFileStatuses expected ErrNoActiveChat, got %v", err)
	}
	if _, err := s.EstimateTokens("prompt", ""); err != ErrNoActiveChat {
		t.Errorf("EstimateTokens expected ErrNoActiveChat, got %v", err)
	}
}

func TestSearchChatsEmpty(t *testing.T) {
	s := setupTestState(t)

	results, err := s.SearchChats("")
	if err != nil {
		t.Fatalf("SearchChats failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestSearchChatsEmptyQueryReturnsAll(t *testing.T) {
	s := setupTestState(t)

	s.ChatNew("Physics Chat", "")
	s.ChatNew("Math Chat", "")
	s.ChatNew("Chemistry Chat", "")

	results, err := s.SearchChats("")
	if err != nil {
		t.Fatalf("SearchChats failed: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	// All should be title matches
	for _, r := range results {
		if r.MatchType != "title" {
			t.Errorf("Expected match_type 'title', got %q", r.MatchType)
		}
	}
}

func TestSearchChatsTitleMatch(t *testing.T) {
	s := setupTestState(t)

	s.ChatNew("Physics Chat", "")
	s.ChatNew("Math Chat", "")
	s.ChatNew("Chemistry Chat", "")

	results, err := s.SearchChats("Physics")
	if err != nil {
		t.Fatalf("SearchChats failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].Name != "Physics Chat" {
		t.Errorf("Expected 'Physics Chat', got %q", results[0].Name)
	}
	if results[0].MatchType != "title" {
		t.Errorf("Expected match_type 'title', got %q", results[0].MatchType)
	}
}

func TestSearchChatsTitleMatchCaseInsensitive(t *testing.T) {
	s := setupTestState(t)

	s.ChatNew("Physics Chat", "")

	results, err := s.SearchChats("physics")
	if err != nil {
		t.Fatalf("SearchChats failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
}

func TestSearchChatsContentMatch(t *testing.T) {
	s := setupTestState(t)

	// Create a chat with a message containing searchable content
	s.ChatNew("Random Chat", "")
	s.AddUserMessage("Tell me about quantum mechanics", "")

	results, err := s.SearchChats("quantum")
	if err != nil {
		t.Fatalf("SearchChats failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}
	if results[0].MatchType != "content" {
		t.Errorf("Expected match_type 'content', got %q", results[0].MatchType)
	}
	if results[0].Excerpt == "" {
		t.Error("Expected excerpt to be set for content match")
	}
}

func TestSearchChatsTitleMatchTakesPriority(t *testing.T) {
	s := setupTestState(t)

	// Create chat with title that matches
	s.ChatNew("Quantum Physics", "")
	// Create chat with content that matches
	s.ChatNew("Other Chat", "")
	s.AddUserMessage("This is about quantum computers", "")

	results, err := s.SearchChats("quantum")
	if err != nil {
		t.Fatalf("SearchChats failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// First result should be title match (created later, so newer)
	// But both should be found
	foundTitle := false
	foundContent := false
	for _, r := range results {
		if r.MatchType == "title" && r.Name == "Quantum Physics" {
			foundTitle = true
		}
		if r.MatchType == "content" && r.Name == "Other Chat" {
			foundContent = true
		}
	}
	if !foundTitle {
		t.Error("Expected to find title match")
	}
	if !foundContent {
		t.Error("Expected to find content match")
	}
}

func TestSearchChatsNoMatch(t *testing.T) {
	s := setupTestState(t)

	s.ChatNew("Physics Chat", "")
	s.AddUserMessage("This is about physics", "")

	results, err := s.SearchChats("biology")
	if err != nil {
		t.Fatalf("SearchChats failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestForkChatBasic(t *testing.T) {
	s := setupTestState(t)

	// Create source chat with messages
	sourceChat, _ := s.ChatNew("source", "")
	s.AddUserMessage("Message 1", "model-1")
	s.AddAssistantMessage([]MessagePart{{Type: PartTypeText, Content: "Response 1"}}, nil, "model-1", nil)
	s.AddUserMessage("Message 2", "model-2")
	s.AddAssistantMessage([]MessagePart{{Type: PartTypeText, Content: "Response 2"}}, nil, "model-2", nil)
	s.AddUserMessage("Message 3", "model-3")

	// Fork from message 3 (index 4, the last user message)
	result, err := s.ForkChat(sourceChat.ID, 4)
	if err != nil {
		t.Fatalf("ForkChat failed: %v", err)
	}

	// Verify result
	if result.NewChatID == "" {
		t.Error("Expected new chat ID")
	}
	if result.ForkMessageContent != "Message 3" {
		t.Errorf("Expected fork message content 'Message 3', got %q", result.ForkMessageContent)
	}

	// Verify new chat is now active
	if s.ActiveChat == nil || s.ActiveChat.ID != result.NewChatID {
		t.Error("Expected forked chat to be active")
	}

	// Verify new chat properties
	if s.ActiveChat.Name != "Fork of source" {
		t.Errorf("Expected name 'Fork of source', got %q", s.ActiveChat.Name)
	}
	if s.ActiveChat.Draft != "Message 3" {
		t.Errorf("Expected draft 'Message 3', got %q", s.ActiveChat.Draft)
	}

	// Verify messages: should have 4 messages (not including the fork message)
	if len(s.ActiveChat.Messages) != 4 {
		t.Errorf("Expected 4 messages, got %d", len(s.ActiveChat.Messages))
	}
}

func TestForkChatFirstMessage(t *testing.T) {
	s := setupTestState(t)

	// Create source chat with messages
	sourceChat, _ := s.ChatNew("source", "")
	s.AddUserMessage("First message", "model-1")
	s.AddAssistantMessage([]MessagePart{{Type: PartTypeText, Content: "Response"}}, nil, "model-1", nil)

	// Fork from first message (index 0)
	result, err := s.ForkChat(sourceChat.ID, 0)
	if err != nil {
		t.Fatalf("ForkChat failed: %v", err)
	}

	// New chat should have no messages, just draft
	if len(s.ActiveChat.Messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(s.ActiveChat.Messages))
	}
	if s.ActiveChat.Draft != "First message" {
		t.Errorf("Expected draft 'First message', got %q", s.ActiveChat.Draft)
	}
	if result.ForkMessageContent != "First message" {
		t.Errorf("Expected fork message content 'First message', got %q", result.ForkMessageContent)
	}
}

func TestForkChatWithContext(t *testing.T) {
	s := setupTestState(t)

	// Create actual files on filesystem (fork checks if files exist)
	os.WriteFile(filepath.Join(s.ProjectRoot, "file1.go"), []byte("package one"), 0644)
	os.WriteFile(filepath.Join(s.ProjectRoot, "file2.go"), []byte("package two"), 0644)

	// Create source chat with context files
	sourceChat, _ := s.ChatNew("source", "")
	s.ContextAdd("file1.go", "package one")
	s.ContextAdd("file2.go", "package two")
	s.AddUserMessage("Process these files", "model-1")

	// Fork from the user message (index 2: context_event, context_event, user)
	result, err := s.ForkChat(sourceChat.ID, 2)
	if err != nil {
		t.Fatalf("ForkChat failed: %v", err)
	}

	// Verify context files were copied to new chat
	if len(s.ActiveChat.ContextFiles) != 2 {
		t.Errorf("Expected 2 context files, got %d", len(s.ActiveChat.ContextFiles))
	}

	// Verify content is accessible
	content1, err := s.GetContextFile("file1.go")
	if err != nil {
		t.Fatalf("GetContextFile failed: %v", err)
	}
	if content1 != "package one" {
		t.Errorf("Expected 'package one', got %q", content1)
	}

	content2, err := s.GetContextFile("file2.go")
	if err != nil {
		t.Fatalf("GetContextFile failed: %v", err)
	}
	if content2 != "package two" {
		t.Errorf("Expected 'package two', got %q", content2)
	}

	// Verify no warnings
	if len(result.ContextWarnings) != 0 {
		t.Errorf("Expected no warnings, got %d", len(result.ContextWarnings))
	}
}

func TestForkChatInvalidIndex(t *testing.T) {
	s := setupTestState(t)

	sourceChat, _ := s.ChatNew("source", "")
	s.AddUserMessage("Message", "model")

	// Test negative index
	_, err := s.ForkChat(sourceChat.ID, -1)
	if err == nil {
		t.Error("Expected error for negative index")
	}

	// Test index out of range
	_, err = s.ForkChat(sourceChat.ID, 5)
	if err == nil {
		t.Error("Expected error for index out of range")
	}
}

func TestForkChatFromAssistantMessage(t *testing.T) {
	s := setupTestState(t)

	sourceChat, _ := s.ChatNew("source", "")
	s.AddUserMessage("Message", "model")
	s.AddAssistantMessage([]MessagePart{{Type: PartTypeText, Content: "Response"}}, nil, "model", nil)

	// Try to fork from assistant message (index 1)
	_, err := s.ForkChat(sourceChat.ID, 1)
	if err == nil {
		t.Error("Expected error when forking from assistant message")
	}
	if err.Error() != "can only fork from user messages" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestForkChatNotFound(t *testing.T) {
	s := setupTestState(t)

	_, err := s.ForkChat("nonexistent", 0)
	if err != ErrChatNotFound {
		t.Errorf("Expected ErrChatNotFound, got %v", err)
	}
}

func TestForkChatContextIsolation(t *testing.T) {
	s := setupTestState(t)

	// Create actual file on filesystem (fork checks if files exist)
	os.WriteFile(filepath.Join(s.ProjectRoot, "shared.go"), []byte("package original"), 0644)

	// Create source chat with context
	sourceChat, _ := s.ChatNew("source", "")
	s.ContextAdd("shared.go", "package original")
	s.AddUserMessage("Message", "model")

	// Fork from the user message (index 1: context_event, user)
	result, err := s.ForkChat(sourceChat.ID, 1)
	if err != nil {
		t.Fatalf("ForkChat failed: %v", err)
	}

	// Modify context in forked chat
	s.ContextRemove("shared.go")
	s.ContextAdd("shared.go", "package modified_in_fork")

	// Switch back to source chat
	s.ChatSelect(sourceChat.ID)

	// Verify source context is unchanged
	content, _ := s.GetContextFile("shared.go")
	if content != "package original" {
		t.Errorf("Source chat context should be unchanged, got %q", content)
	}

	// Verify forked chat has modified content
	s.ChatSelect(result.NewChatID)
	content, _ = s.GetContextFile("shared.go")
	if content != "package modified_in_fork" {
		t.Errorf("Forked chat should have modified content, got %q", content)
	}
}

func TestAddUserMessageCapturesContextSnapshot(t *testing.T) {
	s := setupTestState(t)

	s.ChatNew("test", "")
	s.ContextAdd("file1.go", "package one")
	s.ContextAdd("file2.go", "package two")

	s.AddUserMessage("Process files", "model")

	// Verify snapshot was captured on the user message (index 2: context_event, context_event, user)
	msg := s.ActiveChat.Messages[2]
	if msg.Role != "user" {
		t.Fatalf("Expected user message at index 2, got %s", msg.Role)
	}
	if len(msg.ContextSnapshot) != 2 {
		t.Errorf("Expected 2 files in snapshot, got %d", len(msg.ContextSnapshot))
	}

	// Verify snapshot contains correct paths
	paths := make(map[string]bool)
	for _, ref := range msg.ContextSnapshot {
		paths[ref.Path] = true
		if ref.FileID == "" {
			t.Errorf("Expected FileID to be set for %s", ref.Path)
		}
	}
	if !paths["file1.go"] || !paths["file2.go"] {
		t.Error("Snapshot should contain both file paths")
	}
}

func TestForkChatDeletedFile(t *testing.T) {
	s := setupTestState(t)

	// Create file on filesystem
	filePath := filepath.Join(s.ProjectRoot, "deleteme.go")
	os.WriteFile(filePath, []byte("package deleteme"), 0644)

	// Create chat with context file
	sourceChat, _ := s.ChatNew("source", "")
	s.ContextAdd("deleteme.go", "package deleteme")
	s.AddUserMessage("Process this file", "model")

	// Delete the file from filesystem
	os.Remove(filePath)

	// Fork - should warn about deleted file and NOT add it to context
	result, err := s.ForkChat(sourceChat.ID, 1) // index 1: context_event, user
	if err != nil {
		t.Fatalf("ForkChat failed: %v", err)
	}

	// Should have exactly one warning for deleted file
	if len(result.ContextWarnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(result.ContextWarnings))
	}
	if result.ContextWarnings[0].Issue != "deleted" {
		t.Errorf("Expected issue 'deleted', got %q", result.ContextWarnings[0].Issue)
	}
	if result.ContextWarnings[0].Path != "deleteme.go" {
		t.Errorf("Expected path 'deleteme.go', got %q", result.ContextWarnings[0].Path)
	}

	// Forked chat should NOT have the deleted file in context
	if len(s.ActiveChat.ContextFiles) != 0 {
		t.Errorf("Expected 0 context files (deleted file should be excluded), got %d", len(s.ActiveChat.ContextFiles))
	}
}

func TestForkChatRejectsEscapedSnapshotPath(t *testing.T) {
	s := setupTestState(t)

	chat, err := s.ChatNew("source", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}
	if err := s.ContextAdd("safe.go", "package safe"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if err := s.AddUserMessage("update safe.go", "model"); err != nil {
		t.Fatalf("AddUserMessage failed: %v", err)
	}

	msgIdx := len(s.ActiveChat.Messages) - 1
	if msgIdx < 0 {
		t.Fatalf("expected at least one message")
	}
	s.ActiveChat.Messages[msgIdx].ContextSnapshot[0].Path = "../escape.go"
	if err := s.SaveActiveChat(); err != nil {
		t.Fatalf("SaveActiveChat failed: %v", err)
	}

	_, err = s.ForkChat(chat.ID, msgIdx)
	if err != ErrPathEscape {
		t.Fatalf("expected ErrPathEscape, got %v", err)
	}
}

func TestEditUserMessageRejectsEscapedSnapshotPath(t *testing.T) {
	s := setupTestState(t)

	if _, err := s.ChatNew("source", ""); err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}
	if err := s.ContextAdd("safe.go", "package safe"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if err := s.AddUserMessage("update safe.go", "model"); err != nil {
		t.Fatalf("AddUserMessage failed: %v", err)
	}

	msgIdx := len(s.ActiveChat.Messages) - 1
	s.ActiveChat.Messages[msgIdx].ContextSnapshot[0].Path = "../escape.go"

	_, err := s.EditUserMessage(msgIdx, "edited content")
	if err != ErrPathEscape {
		t.Fatalf("expected ErrPathEscape, got %v", err)
	}
}

// --- Global chat tests ---

func setupGlobalTestState(t *testing.T) *State {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	s := New()
	if err := s.Init(""); err != nil {
		t.Fatalf("Init (global-only) failed: %v", err)
	}
	return s
}

func TestChatNewGlobal(t *testing.T) {
	s := setupGlobalTestState(t)

	chat, err := s.ChatNewGlobal("global-test", "")
	if err != nil {
		t.Fatalf("ChatNewGlobal failed: %v", err)
	}
	if chat.Name != "global-test" {
		t.Errorf("Expected name 'global-test', got %q", chat.Name)
	}
	if !chat.Global {
		t.Error("Expected Global=true")
	}
	if s.ActiveChat != chat {
		t.Error("Expected ActiveChat to be set")
	}
}

func TestChatListGlobal(t *testing.T) {
	s := setupGlobalTestState(t)

	s.ChatNewGlobal("first", "")
	s.ChatNewGlobal("second", "")

	chats, err := s.ChatListGlobal()
	if err != nil {
		t.Fatalf("ChatListGlobal failed: %v", err)
	}
	if len(chats) != 2 {
		t.Fatalf("Expected 2 chats, got %d", len(chats))
	}
	for _, c := range chats {
		if !c.Global {
			t.Errorf("Chat %s should have Global=true", c.ID)
		}
	}
}

func TestChatSelectGlobal(t *testing.T) {
	s := setupGlobalTestState(t)

	chat, _ := s.ChatNewGlobal("test", "")
	chatID := chat.ID

	// Create another to deselect first
	s.ChatNewGlobal("other", "")

	// Re-select first
	selected, err := s.ChatSelectGlobal(chatID)
	if err != nil {
		t.Fatalf("ChatSelectGlobal failed: %v", err)
	}
	if selected.ID != chatID {
		t.Errorf("Expected ID %q, got %q", chatID, selected.ID)
	}
	if !selected.Global {
		t.Error("Expected Global=true after select")
	}
}

func TestGlobalChatNoProjectRoot(t *testing.T) {
	s := setupGlobalTestState(t)

	if s.ProjectRoot != "" {
		t.Errorf("Expected empty ProjectRoot, got %q", s.ProjectRoot)
	}
	if !s.GlobalOnly {
		t.Error("Expected GlobalOnly=true")
	}
	if !s.Initialized() {
		t.Error("Expected Initialized()=true for global-only mode")
	}
}

func TestChatDeleteGlobal(t *testing.T) {
	s := setupGlobalTestState(t)

	chat, _ := s.ChatNewGlobal("to-delete", "")
	s.ChatNewGlobal("keep", "") // switch away

	if err := s.ChatDeleteGlobal(chat.ID); err != nil {
		t.Fatalf("ChatDeleteGlobal failed: %v", err)
	}

	chats, _ := s.ChatListGlobal()
	for _, c := range chats {
		if c.ID == chat.ID {
			t.Error("Deleted chat still in list")
		}
	}
}

func TestChatRenameGlobal(t *testing.T) {
	s := setupGlobalTestState(t)

	chat, _ := s.ChatNewGlobal("old-name", "")

	if err := s.ChatRenameGlobal(chat.ID, "new-name"); err != nil {
		t.Fatalf("ChatRenameGlobal failed: %v", err)
	}

	chats, _ := s.ChatListGlobal()
	for _, c := range chats {
		if c.ID == chat.ID && c.Name != "new-name" {
			t.Errorf("Expected name 'new-name', got %q", c.Name)
		}
	}
}

func TestForkChatGlobal(t *testing.T) {
	s := setupGlobalTestState(t)

	chat, _ := s.ChatNewGlobal("source", "")
	if err := s.AddUserMessage("hello", "model"); err != nil {
		t.Fatalf("AddUserMessage failed: %v", err)
	}
	if err := s.SaveActiveChatGlobal(); err != nil {
		t.Fatalf("SaveActiveChatGlobal failed: %v", err)
	}

	forked, err := s.ForkChatGlobal(chat.ID, 0)
	if err != nil {
		t.Fatalf("ForkChatGlobal failed: %v", err)
	}
	if forked.NewChatID == chat.ID {
		t.Error("Forked chat should have different ID")
	}
	// ActiveChat should now be the forked global chat
	if s.ActiveChat == nil || !s.ActiveChat.Global {
		t.Error("Active chat after fork should be global")
	}
}

// --- Chat move tests ---

// setupBothScopesState creates a state with both project and global dirs initialized.
func setupBothScopesState(t *testing.T) *State {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	tmpDir := t.TempDir()
	s := New()
	if err := s.ProjectInit(tmpDir); err != nil {
		t.Fatalf("ProjectInit failed: %v", err)
	}
	if err := s.Init(tmpDir); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return s
}

func TestChatMoveToGlobal(t *testing.T) {
	s := setupBothScopesState(t)

	chat, err := s.ChatNew("move-me", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}
	chatID := chat.ID

	// Deselect so move doesn't trip over active chat
	s.ActiveChat = nil

	err = s.ChatMoveToGlobal(chatID)
	if err != nil {
		t.Fatalf("ChatMoveToGlobal failed: %v", err)
	}

	// Verify source is gone
	srcDir := filepath.Join(s.ProjectRoot, ".bb7", "chats", chatID)
	if _, err := os.Stat(srcDir); !os.IsNotExist(err) {
		t.Error("Expected source chat directory to be removed")
	}

	// Verify destination exists
	home := os.Getenv("HOME")
	dstDir := filepath.Join(home, ".bb7", "chats", chatID)
	if _, err := os.Stat(dstDir); err != nil {
		t.Error("Expected destination chat directory to exist")
	}

	// Verify no output directory in global
	outputDir := filepath.Join(dstDir, "output")
	if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
		t.Error("Expected no output directory in global chat")
	}

	// Verify chat appears in global list
	globalChats, err := s.ChatListGlobal()
	if err != nil {
		t.Fatalf("ChatListGlobal failed: %v", err)
	}
	found := false
	for _, c := range globalChats {
		if c.ID == chatID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected chat to appear in global list")
	}

	// Verify chat is gone from project list
	projectChats, err := s.ChatList()
	if err != nil {
		t.Fatalf("ChatList failed: %v", err)
	}
	for _, c := range projectChats {
		if c.ID == chatID {
			t.Error("Expected chat to be removed from project list")
		}
	}
}

func TestChatMoveToProject(t *testing.T) {
	s := setupBothScopesState(t)

	chat, err := s.ChatNewGlobal("move-me-back", "")
	if err != nil {
		t.Fatalf("ChatNewGlobal failed: %v", err)
	}
	chatID := chat.ID

	s.ActiveChat = nil

	err = s.ChatMoveToProject(chatID)
	if err != nil {
		t.Fatalf("ChatMoveToProject failed: %v", err)
	}

	// Verify source is gone
	home := os.Getenv("HOME")
	srcDir := filepath.Join(home, ".bb7", "chats", chatID)
	if _, err := os.Stat(srcDir); !os.IsNotExist(err) {
		t.Error("Expected source global chat directory to be removed")
	}

	// Verify destination exists
	dstDir := filepath.Join(s.ProjectRoot, ".bb7", "chats", chatID)
	if _, err := os.Stat(dstDir); err != nil {
		t.Error("Expected destination project chat directory to exist")
	}

	// Verify output directory was created
	outputDir := filepath.Join(dstDir, "output")
	if _, err := os.Stat(outputDir); err != nil {
		t.Error("Expected output directory in project chat")
	}

	// Verify chat appears in project list
	projectChats, err := s.ChatList()
	if err != nil {
		t.Fatalf("ChatList failed: %v", err)
	}
	found := false
	for _, c := range projectChats {
		if c.ID == chatID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected chat to appear in project list")
	}

	// Verify chat is gone from global list
	globalChats, err := s.ChatListGlobal()
	if err != nil {
		t.Fatalf("ChatListGlobal failed: %v", err)
	}
	for _, c := range globalChats {
		if c.ID == chatID {
			t.Error("Expected chat to be removed from global list")
		}
	}
}

func TestChatMoveToGlobalBlockedByOutputFiles(t *testing.T) {
	s := setupBothScopesState(t)

	chat, err := s.ChatNew("has-output", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	// Create a fake output file
	outputDir := filepath.Join(s.ProjectRoot, ".bb7", "chats", chat.ID, "output")
	if err := os.WriteFile(filepath.Join(outputDir, "file.txt"), []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to create output file: %v", err)
	}

	s.ActiveChat = nil

	err = s.ChatMoveToGlobal(chat.ID)
	if err == nil {
		t.Fatal("Expected error when moving chat with output files")
	}

	// Verify source still exists (not moved)
	srcDir := filepath.Join(s.ProjectRoot, ".bb7", "chats", chat.ID)
	if _, err := os.Stat(srcDir); err != nil {
		t.Error("Expected source chat directory to still exist")
	}
}

func TestChatMoveToGlobalNotFound(t *testing.T) {
	s := setupBothScopesState(t)

	err := s.ChatMoveToGlobal("nonexistent")
	if err != ErrChatNotFound {
		t.Errorf("Expected ErrChatNotFound, got %v", err)
	}
}

func TestChatMoveToProjectNotFound(t *testing.T) {
	s := setupBothScopesState(t)

	err := s.ChatMoveToProject("nonexistent")
	if err != ErrChatNotFound {
		t.Errorf("Expected ErrChatNotFound, got %v", err)
	}
}

func TestChatMoveActivatesInDestination(t *testing.T) {
	s := setupBothScopesState(t)

	chat, err := s.ChatNew("active-move", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}
	chatID := chat.ID

	// Chat is active in project scope
	if s.ActiveChat == nil || s.ActiveChat.ID != chatID {
		t.Fatal("Expected chat to be active")
	}

	err = s.ChatMoveToGlobal(chatID)
	if err != nil {
		t.Fatalf("ChatMoveToGlobal failed: %v", err)
	}

	// Active chat should be the moved chat in global scope
	if s.ActiveChat == nil {
		t.Fatal("Expected ActiveChat to be set after move to global")
	}
	if s.ActiveChat.ID != chatID {
		t.Errorf("Expected active chat ID %s, got %s", chatID, s.ActiveChat.ID)
	}

	// Move back to project
	err = s.ChatMoveToProject(chatID)
	if err != nil {
		t.Fatalf("ChatMoveToProject failed: %v", err)
	}

	if s.ActiveChat == nil {
		t.Fatal("Expected ActiveChat to be set after move to project")
	}
	if s.ActiveChat.ID != chatID {
		t.Errorf("Expected active chat ID %s, got %s", chatID, s.ActiveChat.ID)
	}
}

func TestChatMovePreservesMessages(t *testing.T) {
	s := setupBothScopesState(t)

	chat, err := s.ChatNew("with-messages", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}
	chatID := chat.ID

	// Add a message
	if err := s.AddUserMessage("hello from project", ""); err != nil {
		t.Fatalf("AddUserMessage failed: %v", err)
	}

	s.ActiveChat = nil

	err = s.ChatMoveToGlobal(chatID)
	if err != nil {
		t.Fatalf("ChatMoveToGlobal failed: %v", err)
	}

	// Select the moved chat and verify messages
	_, err = s.ChatSelectGlobal(chatID)
	if err != nil {
		t.Fatalf("ChatSelectGlobal failed: %v", err)
	}

	if len(s.ActiveChat.Messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(s.ActiveChat.Messages))
	}
	if MessageText(s.ActiveChat.Messages[0]) != "hello from project" {
		t.Error("Expected message content to be preserved")
	}
}

func TestChatMoveToGlobalSetsFilesReadonly(t *testing.T) {
	s := setupBothScopesState(t)

	chat, err := s.ChatNew("with-files", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}
	chatID := chat.ID

	// Add writable and readonly context files
	if err := s.ContextAdd("foo.go", "package foo"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if err := s.ContextAddWithReadOnly("bar.go", "package bar", true); err != nil {
		t.Fatalf("ContextAddWithReadOnly failed: %v", err)
	}

	// Verify one is writable
	found := false
	for _, f := range s.ActiveChat.ContextFiles {
		if f.Path == "foo.go" && !f.ReadOnly {
			found = true
		}
	}
	if !found {
		t.Fatal("Expected foo.go to be writable before move")
	}

	// Save and deactivate before move
	s.SaveActiveChat()
	s.ActiveChat = nil

	err = s.ChatMoveToGlobal(chatID)
	if err != nil {
		t.Fatalf("ChatMoveToGlobal failed: %v", err)
	}

	// After move, all context files should be readonly
	if s.ActiveChat == nil {
		t.Fatal("Expected ActiveChat to be set after move")
	}
	for _, f := range s.ActiveChat.ContextFiles {
		if !f.ReadOnly {
			t.Errorf("Expected file %s to be readonly after move to global", f.Path)
		}
	}
}

// TestChatMoveRoundTripRestoresWritable verifies that files forced readonly by
// ChatMoveToGlobal become writable again after ChatMoveToProject.
func TestChatMoveRoundTripRestoresWritable(t *testing.T) {
	s := setupBothScopesState(t)

	chat, err := s.ChatNew("round-trip", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}
	chatID := chat.ID

	// Add a writable context file
	if err := s.ContextAdd("main.go", "package main"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	// Verify writable before move
	cf := s.findContextFile("main.go")
	if cf == nil {
		t.Fatal("Expected to find main.go in context")
	}
	if cf.ReadOnly {
		t.Fatal("Expected main.go to be writable before move")
	}

	s.SaveActiveChat()
	s.ActiveChat = nil

	// Move project → global → project
	if err := s.ChatMoveToGlobal(chatID); err != nil {
		t.Fatalf("ChatMoveToGlobal failed: %v", err)
	}
	if err := s.ChatMoveToProject(chatID); err != nil {
		t.Fatalf("ChatMoveToProject failed: %v", err)
	}

	// After round-trip, the file should NOT be permanently stuck as readonly.
	// The user should be able to toggle it back to writable.
	cf = s.findContextFile("main.go")
	if cf == nil {
		t.Fatal("Expected to find main.go after round-trip")
	}
	if cf.External {
		t.Error("Expected main.go to NOT be external after round-trip to project")
	}

	// Even if ReadOnly is still true from the global phase, toggling must work
	if err := s.ContextSetReadOnly("main.go", false); err != nil {
		t.Errorf("ContextSetReadOnly(false) should succeed after round-trip, got: %v", err)
	}
}

// TestChatMoveToProjectConvertsExternalFilesBack verifies that files added
// while in global scope (forced external+absolute) are converted back to
// internal relative paths when the chat is moved to project scope.
func TestChatMoveToProjectConvertsExternalFilesBack(t *testing.T) {
	s := setupBothScopesState(t)

	// Create a global chat and add a file that's inside the project
	chat, err := s.ChatNewGlobal("global-with-project-file", "")
	if err != nil {
		t.Fatalf("ChatNewGlobal failed: %v", err)
	}
	chatID := chat.ID

	// Add file using absolute path (as global chats require)
	absPath := filepath.Join(s.ProjectRoot, "main.go")
	if err := s.ContextAdd(absPath, "package main"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	// In global scope, file should be external and readonly
	cf := s.findContextFile(absPath)
	if cf == nil {
		t.Fatal("Expected to find file in global chat context")
	}
	if !cf.External {
		t.Error("Expected file to be external in global chat")
	}
	if !cf.ReadOnly {
		t.Error("Expected file to be readonly in global chat")
	}

	s.SaveActiveChat()
	s.ActiveChat = nil

	// Move to project
	if err := s.ChatMoveToProject(chatID); err != nil {
		t.Fatalf("ChatMoveToProject failed: %v", err)
	}

	// After moving to project, the file is inside the project directory,
	// so it should be converted to a relative internal path and be writable.
	cf = s.findContextFile("main.go")
	if cf == nil {
		// Also check absolute path in case conversion didn't happen
		cf = s.findContextFile(absPath)
		if cf != nil {
			t.Errorf("File still has absolute path %s instead of relative 'main.go'", absPath)
		} else {
			t.Fatal("File not found under either relative or absolute path after move to project")
		}
	}
	if cf != nil {
		if cf.External {
			t.Error("Expected file to NOT be external after move to project (it's inside project dir)")
		}
		// Should be toggleable to writable
		if err := s.ContextSetReadOnly(cf.Path, false); err != nil {
			t.Errorf("ContextSetReadOnly(false) should work for project-internal file, got: %v", err)
		}
	}
}

// TestChatMoveRoundTripReadonlyToggle verifies the full scenario: create project
// chat with writable file, move to global (files forced readonly), move back to
// project, then toggle readonly off. This is the exact user-reported bug.
func TestChatMoveRoundTripReadonlyToggle(t *testing.T) {
	s := setupBothScopesState(t)

	chat, err := s.ChatNew("toggle-test", "")
	if err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}
	chatID := chat.ID

	// Add writable and readonly files
	if err := s.ContextAdd("writable.go", "package w"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if err := s.ContextAddWithReadOnly("locked.go", "package l", true); err != nil {
		t.Fatalf("ContextAddWithReadOnly failed: %v", err)
	}

	s.SaveActiveChat()
	s.ActiveChat = nil

	// Round-trip: project → global → project
	if err := s.ChatMoveToGlobal(chatID); err != nil {
		t.Fatalf("ChatMoveToGlobal failed: %v", err)
	}
	if err := s.ChatMoveToProject(chatID); err != nil {
		t.Fatalf("ChatMoveToProject failed: %v", err)
	}

	// writable.go was originally writable — user should be able to make it writable again
	if err := s.ContextSetReadOnly("writable.go", false); err != nil {
		t.Errorf("Failed to make writable.go writable after round-trip: %v", err)
	}

	// locked.go was originally readonly — toggling to writable should also work
	// (user explicitly chose readonly, but should still be able to change their mind)
	if err := s.ContextSetReadOnly("locked.go", false); err != nil {
		t.Errorf("Failed to make locked.go writable after round-trip: %v", err)
	}

	// Verify both files are not marked as external
	for _, f := range s.ActiveChat.ContextFiles {
		if f.External {
			t.Errorf("File %s should not be external after round-trip to project", f.Path)
		}
	}
}
