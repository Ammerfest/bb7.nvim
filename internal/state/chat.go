package state

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// generateID creates a random 6-character hex ID.
func generateID() (string, error) {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// releasePreviousLock releases the lock on the previously active chat (if any).
func (s *State) releasePreviousLock() {
	if s.lockedChatDir != "" {
		ReleaseLock(s.lockedChatDir)
		s.lockedChatDir = ""
	}
}

// ChatNew creates a new chat and sets it as active.
func (s *State) ChatNew(name string) (*Chat, error) {
	if err := s.requireInit(); err != nil {
		return nil, err
	}

	id, err := generateID()
	if err != nil {
		return nil, err
	}

	created := time.Now().UTC()

	// Generate default name if none provided
	if name == "" {
		name = "Chat " + created.Format("2006-01-02 15:04")
	}

	chat := &Chat{
		ID:           id,
		Name:         name,
		Created:      created,
		Model:        "anthropic/claude-sonnet-4",
		ContextFiles: []ContextFile{},
		Messages:     []Message{},
	}

	chatDir := s.chatDir(id)
	if err := os.MkdirAll(s.contextDir(id), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.outputDir(id), 0755); err != nil {
		return nil, err
	}

	if err := s.saveChat(chat); err != nil {
		os.RemoveAll(chatDir)
		return nil, err
	}
	if err := s.updateChatIndexEntry(chat); err != nil {
		// Index is a cache; do not fail chat creation if it can't be updated.
	}

	// Release old lock, acquire new one
	s.releasePreviousLock()
	if err := AcquireLock(chatDir); err != nil {
		// Lock failure is not fatal for chat creation
	} else {
		s.lockedChatDir = chatDir
	}

	s.ActiveChat = chat
	s.saveActiveChatID(chat.ID)
	return chat, nil
}

// ChatList returns summaries of all chats, sorted by creation time (newest first).
func (s *State) ChatList() ([]ChatSummary, error) {
	if err := s.requireInit(); err != nil {
		return nil, err
	}

	idx, err := s.ensureChatIndex()
	if err != nil {
		if os.IsNotExist(err) {
			return []ChatSummary{}, nil
		}
		return nil, err
	}

	// Check lock status for each chat
	for i := range idx.Chats {
		idx.Chats[i].Locked = IsLocked(s.chatDir(idx.Chats[i].ID))
	}

	sort.Slice(idx.Chats, func(i, j int) bool {
		return idx.Chats[i].Created.After(idx.Chats[j].Created)
	})

	return idx.Chats, nil
}

type chatSummaryFile struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
}

// loadChatSummary reads chat.json and extracts only summary fields.
func (s *State) loadChatSummary(id string) (ChatSummary, error) {
	return loadChatSummaryFrom(s.chatJSONPath(id))
}

// ChatSelect loads a chat by ID and sets it as active.
func (s *State) ChatSelect(id string) (*Chat, error) {
	if err := s.requireInit(); err != nil {
		return nil, err
	}

	chatDir := s.chatDir(id)

	// Check if locked by another process
	if IsLocked(chatDir) {
		return nil, ErrChatLocked
	}

	chat, err := s.loadChat(id)
	if err != nil {
		return nil, err
	}

	// Release old lock, acquire new one
	s.releasePreviousLock()
	if err := AcquireLock(chatDir); err != nil {
		// Lock failure is not fatal
	} else {
		s.lockedChatDir = chatDir
	}

	s.ActiveChat = chat
	s.saveActiveChatID(chat.ID)
	return chat, nil
}

// ChatDelete removes a chat by ID.
func (s *State) ChatDelete(id string) error {
	if err := s.requireInit(); err != nil {
		return err
	}

	chatDir := s.chatDir(id)
	if _, err := os.Stat(chatDir); os.IsNotExist(err) {
		return ErrChatNotFound
	}

	// Check if locked by another process
	if IsLocked(chatDir) {
		return ErrChatLocked
	}

	if s.ActiveChat != nil && s.ActiveChat.ID == id {
		s.releasePreviousLock()
		s.ActiveChat = nil
		s.saveActiveChatID("")
	}

	if err := os.RemoveAll(chatDir); err != nil {
		return err
	}
	if err := s.removeChatIndexEntry(id); err != nil {
		// Index is a cache; do not fail chat deletion if it can't be updated.
	}
	return nil
}

// ChatRename updates a chat's name.
func (s *State) ChatRename(id, name string) error {
	if err := s.requireInit(); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return ErrChatNameEmpty
	}

	if s.ActiveChat != nil && s.ActiveChat.ID == id {
		s.ActiveChat.Name = name
		if err := s.SaveActiveChat(); err != nil {
			return err
		}
		if err := s.updateChatIndexEntry(s.ActiveChat); err != nil {
			// Index is a cache; do not fail rename if it can't be updated.
		}
		return nil
	}

	chat, err := s.loadChat(id)
	if err != nil {
		return err
	}
	chat.Name = name
	if err := s.saveChat(chat); err != nil {
		return err
	}
	if err := s.updateChatIndexEntry(chat); err != nil {
		// Index is a cache; do not fail rename if it can't be updated.
	}
	return nil
}

// loadChat reads a chat from disk.
func (s *State) loadChat(id string) (*Chat, error) {
	data, err := os.ReadFile(s.chatJSONPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrChatNotFound
		}
		return nil, err
	}

	var chat Chat
	if err := json.Unmarshal(data, &chat); err != nil {
		return nil, err
	}

	return &chat, nil
}

// saveChat writes a chat to disk.
func (s *State) saveChat(chat *Chat) error {
	data, err := json.MarshalIndent(chat, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.chatJSONPath(chat.ID), data, 0644)
}

// SaveActiveChat persists the current active chat to disk.
func (s *State) SaveActiveChat() error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}
	if s.ActiveChat.Global {
		return s.SaveActiveChatGlobal()
	}
	if err := s.saveChat(s.ActiveChat); err != nil {
		return err
	}
	if err := s.updateChatIndexEntry(s.ActiveChat); err != nil {
		// Index is a cache; do not fail chat save if it can't be updated.
	}
	return nil
}

// AddSystemMessage appends a system message to the active chat and saves it.
func (s *State) AddSystemMessage(content string) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" {
		return nil
	}
	msg := Message{
		Role:      "system",
		Content:   content,
		Timestamp: time.Now().UTC(),
	}
	s.ActiveChat.Messages = append(s.ActiveChat.Messages, msg)
	return s.SaveActiveChat()
}

// AddUserMessage adds a user message to the active chat.
// The model is recorded so the UI can show model switches over time.
// A snapshot of current context files is also recorded for fork support.
func (s *State) AddUserMessage(content, model string) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	// Capture context snapshot for fork support
	var snapshot []ContextFileRef
	for _, cf := range s.ActiveChat.ContextFiles {
		snapshot = append(snapshot, ContextFileRef{
			Path:      cf.Path,
			FileID:    cf.Version,
			StartLine: cf.StartLine,
			EndLine:   cf.EndLine,
		})
	}

	msg := Message{
		Role:            "user",
		Content:         content,
		Model:           model,
		Timestamp:       time.Now().UTC(),
		ContextSnapshot: snapshot,
	}

	s.ActiveChat.Messages = append(s.ActiveChat.Messages, msg)
	return s.SaveActiveChat()
}

// AddAssistantMessage adds an assistant message to the active chat.
// If parts are provided, they are used; otherwise content is used for backward compatibility.
func (s *State) AddAssistantMessage(content string, parts []MessagePart, outputFiles []string, model string, usage *MessageUsage) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	msg := Message{
		Role:        "assistant",
		Model:       model,
		Timestamp:   time.Now().UTC(),
		OutputFiles: outputFiles,
		Usage:       usage,
	}

	if len(parts) > 0 {
		msg.Parts = parts
	} else {
		msg.Content = content
	}

	s.ActiveChat.Messages = append(s.ActiveChat.Messages, msg)
	return s.SaveActiveChat()
}

// SetChatName updates the name of a chat by ID.
func (s *State) SetChatName(id, name string) error {
	if err := s.requireInit(); err != nil {
		return err
	}

	// Route to global if the target chat is global
	isGlobal := s.GlobalOnly
	if !isGlobal && s.ActiveChat != nil && s.ActiveChat.ID == id && s.ActiveChat.Global {
		isGlobal = true
	}
	if isGlobal {
		return s.ChatRenameGlobal(id, name)
	}

	chat, err := s.loadChat(id)
	if err != nil {
		return err
	}

	chat.Name = name
	if err := s.saveChat(chat); err != nil {
		return err
	}

	// Update active chat if it's the same one
	if s.ActiveChat != nil && s.ActiveChat.ID == id {
		s.ActiveChat.Name = name
	}

	if err := s.updateChatIndexEntry(chat); err != nil {
		// Index is a cache; do not fail rename if it can't be updated.
	}
	return nil
}

// SearchChats searches through all chats by title and content.
// If query is empty, returns all chats as title matches.
// Returns results with match_type indicating whether the match was in title or content.
func (s *State) SearchChats(query string) ([]ChatSearchResult, error) {
	if err := s.requireInit(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(s.chatsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []ChatSearchResult{}, nil
		}
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var results []ChatSearchResult

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		chat, err := s.loadChat(entry.Name())
		if err != nil {
			continue
		}

		// Empty query: return all chats as title matches
		if query == "" {
			results = append(results, ChatSearchResult{
				ID:        chat.ID,
				Name:      chat.Name,
				Created:   chat.Created,
				MatchType: "title",
			})
			continue
		}

		// Check title match (case-insensitive)
		if strings.Contains(strings.ToLower(chat.Name), queryLower) {
			results = append(results, ChatSearchResult{
				ID:        chat.ID,
				Name:      chat.Name,
				Created:   chat.Created,
				MatchType: "title",
			})
			continue
		}

		// Check content match
		excerpt := searchChatContent(chat, queryLower)
		if excerpt != "" {
			results = append(results, ChatSearchResult{
				ID:        chat.ID,
				Name:      chat.Name,
				Created:   chat.Created,
				MatchType: "content",
				Excerpt:   excerpt,
			})
		}
	}

	// Sort by creation time (newest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Created.After(results[j].Created)
	})

	return results, nil
}

// searchChatContent searches through message content for a query.
// Returns an excerpt with context around the first match, or empty string if no match.
func searchChatContent(chat *Chat, queryLower string) string {
	for _, msg := range chat.Messages {
		text := MessageText(msg)
		textLower := strings.ToLower(text)

		idx := strings.Index(textLower, queryLower)
		if idx == -1 {
			continue
		}

		// Extract excerpt with context (up to 80 chars around match)
		start := idx - 30
		if start < 0 {
			start = 0
		}
		end := idx + len(queryLower) + 50
		if end > len(text) {
			end = len(text)
		}

		excerpt := text[start:end]
		// Clean up: replace newlines with spaces, trim
		excerpt = strings.ReplaceAll(excerpt, "\n", " ")
		excerpt = strings.TrimSpace(excerpt)

		// Add ellipsis if truncated
		if start > 0 {
			excerpt = "..." + excerpt
		}
		if end < len(text) {
			excerpt = excerpt + "..."
		}

		return excerpt
	}

	return ""
}

// ContextWarning describes an issue restoring a context file during fork.
type ContextWarning struct {
	Path            string `json:"path"`
	Issue           string `json:"issue"` // "modified" or "deleted"
	OriginalVersion string `json:"original_version,omitempty"`
}

// ForkChatResult contains the result of a fork operation.
type ForkChatResult struct {
	NewChatID          string           `json:"new_chat_id"`
	ForkMessageContent string           `json:"fork_message_content"`
	ContextWarnings    []ContextWarning `json:"context_warnings,omitempty"`
}

// ForkChat creates a new chat from an existing conversation.
// Messages up to (not including) forkIndex are copied to the new chat.
// The fork message's content becomes the new chat's draft.
// Context files are restored from the fork message's snapshot.
func (s *State) ForkChat(chatID string, forkIndex int) (*ForkChatResult, error) {
	if err := s.requireInit(); err != nil {
		return nil, err
	}

	// Load source chat
	sourceChat, err := s.loadChat(chatID)
	if err != nil {
		return nil, err
	}

	// Validate forkIndex
	if forkIndex < 0 || forkIndex >= len(sourceChat.Messages) {
		return nil, errors.New("fork index out of range")
	}

	// The message at forkIndex must be a user message
	forkMsg := sourceChat.Messages[forkIndex]
	if forkMsg.Role != "user" {
		return nil, errors.New("can only fork from user messages")
	}

	// Create new chat
	newID, err := generateID()
	if err != nil {
		return nil, err
	}

	// Avoid "Fork of Fork of..." by checking if already a fork
	newName := sourceChat.Name
	if !strings.HasPrefix(newName, "Fork of ") {
		newName = "Fork of " + newName
	}

	newChat := &Chat{
		ID:              newID,
		Name:            newName,
		Created:         time.Now().UTC(),
		Model:           sourceChat.Model,
		ReasoningEffort: sourceChat.ReasoningEffort,
		Draft:           MessageText(forkMsg), // The fork message becomes the draft
		ContextFiles:    []ContextFile{},
		Messages:        []Message{},
	}

	// Create directories for new chat
	if err := os.MkdirAll(s.contextDir(newID), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.outputDir(newID), 0755); err != nil {
		os.RemoveAll(s.chatDir(newID))
		return nil, err
	}

	// Copy messages up to (not including) forkIndex
	if forkIndex > 0 {
		newChat.Messages = make([]Message, forkIndex)
		copy(newChat.Messages, sourceChat.Messages[:forkIndex])
	}

	// Restore context from fork message's snapshot
	var warnings []ContextWarning
	snapshot := forkMsg.ContextSnapshot

	// If no snapshot (old messages), fall back to copying current context files
	if len(snapshot) == 0 && len(sourceChat.ContextFiles) > 0 {
		snapshot = make([]ContextFileRef, len(sourceChat.ContextFiles))
		for i, cf := range sourceChat.ContextFiles {
			snapshot[i] = ContextFileRef{
				Path:      cf.Path,
				FileID:    cf.Version,
				StartLine: cf.StartLine,
				EndLine:   cf.EndLine,
			}
		}
	}

		for _, ref := range snapshot {
			// Read content from source chat's context directory
			srcPath, err := s.contextFilePath(chatID, ref)
			if err != nil {
				return nil, err
			}
			content, err := os.ReadFile(srcPath)
			if err != nil {
			if os.IsNotExist(err) {
				warnings = append(warnings, ContextWarning{
					Path:            ref.Path,
					Issue:           "deleted",
					OriginalVersion: ref.FileID,
				})
				continue
			}
			return nil, err
		}

		// Look up original ContextFile to get ReadOnly/External flags
		var readOnly, external bool
		for _, cf := range sourceChat.ContextFiles {
			if cf.Path == ref.Path && cf.StartLine == ref.StartLine && cf.EndLine == ref.EndLine {
				readOnly = cf.ReadOnly
				external = cf.External
				break
			}
		}

		// Check if the file exists on the actual filesystem
		// If not, warn and don't add to new chat's context
		var filesystemPath string
		if external || filepath.IsAbs(ref.Path) {
			filesystemPath = ref.Path
		} else {
			filesystemPath = filepath.Join(s.ProjectRoot, ref.Path)
		}

		if _, err := os.Stat(filesystemPath); os.IsNotExist(err) {
			warnings = append(warnings, ContextWarning{
				Path:            ref.Path,
				Issue:           "deleted",
				OriginalVersion: ref.FileID,
			})
			continue // Don't add deleted files to new chat's context
		}

		// Check if content matches the snapshot version
		currentVersion := HashFileVersion(ref.Path, string(content))
		if currentVersion != ref.FileID {
			warnings = append(warnings, ContextWarning{
				Path:            ref.Path,
				Issue:           "modified",
				OriginalVersion: ref.FileID,
			})
		}

			// Write to new chat's context directory
			dstPath, err := s.contextFilePath(newID, ref)
			if err != nil {
				return nil, err
			}
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return nil, err
			}
		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			return nil, err
		}

		newChat.ContextFiles = append(newChat.ContextFiles, ContextFile{
			Path:      ref.Path,
			ReadOnly:  readOnly,
			External:  external,
			Version:   currentVersion, // Use actual restored content's version
			StartLine: ref.StartLine,
			EndLine:   ref.EndLine,
		})
	}

	// If there are warnings, append a system message at the end
	if len(warnings) > 0 {
		var parts []MessagePart
		for _, w := range warnings {
			// Action includes issue type: "ForkWarningModified" or "ForkWarningDeleted"
			action := "ForkWarningModified"
			if w.Issue == "deleted" {
				action = "ForkWarningDeleted"
			}
			parts = append(parts, MessagePart{
				Type:    "context_event",
				Action:  action,
				Path:    w.Path,
				Version: w.OriginalVersion,
			})
		}
		systemMsg := Message{
			Role:      "system",
			Parts:     parts,
			Timestamp: time.Now().UTC(),
		}
		// Append system message at the end so it appears after copied messages
		newChat.Messages = append(newChat.Messages, systemMsg)
	}

	// Save new chat
	if err := s.saveChat(newChat); err != nil {
		os.RemoveAll(s.chatDir(newID))
		return nil, err
	}

	// Release old lock, acquire new one
	s.releasePreviousLock()
	newChatDir := s.chatDir(newID)
	if err := AcquireLock(newChatDir); err != nil {
		// Lock failure is not fatal
	} else {
		s.lockedChatDir = newChatDir
	}

	// Set as active chat
	s.ActiveChat = newChat
	s.saveActiveChatID(newChat.ID)

	return &ForkChatResult{
		NewChatID:          newID,
		ForkMessageContent: MessageText(forkMsg),
		ContextWarnings:    warnings,
	}, nil
}

// EditUserMessage truncates the active chat at a user message and moves its content to draft.
// Messages at and after msgIndex are removed.
func (s *State) EditUserMessage(msgIndex int, content string) ([]ContextWarning, error) {
	if err := s.requireActiveChat(); err != nil {
		return nil, err
	}

	if msgIndex < 0 || msgIndex >= len(s.ActiveChat.Messages) {
		return nil, errors.New("message index out of range")
	}

	targetMsg := s.ActiveChat.Messages[msgIndex]
	if targetMsg.Role != "user" {
		return nil, errors.New("can only edit user messages")
	}

	// Restore context from the target message's snapshot, if available.
	var warnings []ContextWarning
	snapshot := targetMsg.ContextSnapshot

	if len(snapshot) == 0 && len(s.ActiveChat.ContextFiles) > 0 {
		snapshot = make([]ContextFileRef, len(s.ActiveChat.ContextFiles))
		for i, cf := range s.ActiveChat.ContextFiles {
			snapshot[i] = ContextFileRef{
				Path:      cf.Path,
				FileID:    cf.Version,
				StartLine: cf.StartLine,
				EndLine:   cf.EndLine,
			}
		}
	}

	var restoredContext []ContextFile
	for _, ref := range snapshot {
		srcPath, err := s.contextFilePath(s.ActiveChat.ID, ref)
		if err != nil {
			return nil, err
		}
		contentBytes, err := os.ReadFile(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				warnings = append(warnings, ContextWarning{
					Path:            ref.Path,
					Issue:           "deleted",
					OriginalVersion: ref.FileID,
				})
				continue
			}
			return nil, err
		}

		var readOnly, external bool
		for _, cf := range s.ActiveChat.ContextFiles {
			if cf.Path == ref.Path && cf.StartLine == ref.StartLine && cf.EndLine == ref.EndLine {
				readOnly = cf.ReadOnly
				external = cf.External
				break
			}
		}

		var filesystemPath string
		if external || filepath.IsAbs(ref.Path) {
			filesystemPath = ref.Path
		} else {
			filesystemPath = filepath.Join(s.ProjectRoot, ref.Path)
		}

		if _, err := os.Stat(filesystemPath); os.IsNotExist(err) {
			warnings = append(warnings, ContextWarning{
				Path:            ref.Path,
				Issue:           "deleted",
				OriginalVersion: ref.FileID,
			})
			continue
		}

		currentVersion := HashFileVersion(ref.Path, string(contentBytes))
		if currentVersion != ref.FileID {
			warnings = append(warnings, ContextWarning{
				Path:            ref.Path,
				Issue:           "modified",
				OriginalVersion: ref.FileID,
			})
		}

		restoredContext = append(restoredContext, ContextFile{
			Path:      ref.Path,
			ReadOnly:  readOnly,
			External:  external,
			Version:   currentVersion,
			StartLine: ref.StartLine,
			EndLine:   ref.EndLine,
		})
	}

	// Truncate history and move the message content into draft.
	s.ActiveChat.Messages = s.ActiveChat.Messages[:msgIndex]
	s.ActiveChat.Draft = content
	s.ActiveChat.ContextFiles = restoredContext

	// Remove output files that are no longer referenced by remaining messages.
	keep := make(map[string]bool)
	for _, msg := range s.ActiveChat.Messages {
		if msg.Role != "assistant" {
			continue
		}
		for _, path := range msg.OutputFiles {
			keep[path] = true
		}
	}

	outputFiles, err := s.ListOutputFiles()
	if err != nil {
		return nil, err
	}
	for _, path := range outputFiles {
		if !keep[path] {
			if delErr := s.DeleteOutputFile(path); delErr != nil {
				return nil, delErr
			}
		}
	}

	if len(warnings) > 0 {
		var parts []MessagePart
		for _, w := range warnings {
			action := "ForkWarningModified"
			if w.Issue == "deleted" {
				action = "ForkWarningDeleted"
			}
			parts = append(parts, MessagePart{
				Type:    "context_event",
				Action:  action,
				Path:    w.Path,
				Version: w.OriginalVersion,
			})
		}
		systemMsg := Message{
			Role:      "system",
			Parts:     parts,
			Timestamp: time.Now().UTC(),
		}
		s.ActiveChat.Messages = append(s.ActiveChat.Messages, systemMsg)
	}

	return warnings, s.SaveActiveChat()
}

// --- Global chat operations ---

// saveChatAt writes a chat to a specific chat directory.
func saveChatAt(chatDir string, chat *Chat) error {
	chatPath := filepath.Join(chatDir, "chat.json")
	data, err := json.MarshalIndent(chat, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(chatPath, data, 0644)
}

// loadChatFrom reads a chat from a specific chat.json path.
func loadChatFrom(chatJSONPath string) (*Chat, error) {
	data, err := os.ReadFile(chatJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrChatNotFound
		}
		return nil, err
	}

	var chat Chat
	if err := json.Unmarshal(data, &chat); err != nil {
		return nil, err
	}

	return &chat, nil
}

// ChatNewGlobal creates a new global chat and sets it as active.
func (s *State) ChatNewGlobal(name string) (*Chat, error) {
	if err := s.ensureGlobalChatsDir(); err != nil {
		return nil, err
	}

	id, err := generateID()
	if err != nil {
		return nil, err
	}

	created := time.Now().UTC()
	if name == "" {
		name = "Chat " + created.Format("2006-01-02 15:04")
	}

	chat := &Chat{
		ID:           id,
		Name:         name,
		Created:      created,
		Model:        "anthropic/claude-sonnet-4",
		Global:       true,
		ContextFiles: []ContextFile{},
		Messages:     []Message{},
	}

	chatDir := s.globalChatDir(id)
	// Global chats only need a context directory (no output dir)
	if err := os.MkdirAll(filepath.Join(chatDir, "context"), 0755); err != nil {
		return nil, err
	}

	if err := saveChatAt(chatDir, chat); err != nil {
		os.RemoveAll(chatDir)
		return nil, err
	}
	if err := updateChatIndexEntryAt(s.globalChatsDir(), chat); err != nil {
		// Index is a cache; do not fail chat creation if it can't be updated.
	}

	// Release old lock, acquire new one
	s.releasePreviousLock()
	if err := AcquireLock(chatDir); err != nil {
		// Lock failure is not fatal
	} else {
		s.lockedChatDir = chatDir
	}

	s.ActiveChat = chat
	saveActiveChatIDAt(s.globalChatsDir(), chat.ID)
	return chat, nil
}

// ChatListGlobal returns summaries of all global chats, sorted by creation time (newest first).
func (s *State) ChatListGlobal() ([]ChatSummary, error) {
	if err := s.ensureGlobalChatsDir(); err != nil {
		return nil, err
	}

	idx, err := ensureChatIndexAt(s.globalChatsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []ChatSummary{}, nil
		}
		return nil, err
	}

	// Mark all as global and check lock status
	for i := range idx.Chats {
		idx.Chats[i].Global = true
		idx.Chats[i].Locked = IsLocked(s.globalChatDir(idx.Chats[i].ID))
	}

	sort.Slice(idx.Chats, func(i, j int) bool {
		return idx.Chats[i].Created.After(idx.Chats[j].Created)
	})

	return idx.Chats, nil
}

// ChatSelectGlobal loads a global chat by ID and sets it as active.
func (s *State) ChatSelectGlobal(id string) (*Chat, error) {
	chatDir := s.globalChatDir(id)

	// Check if locked by another process
	if IsLocked(chatDir) {
		return nil, ErrChatLocked
	}

	chatPath := s.globalChatJSONPath(id)
	chat, err := loadChatFrom(chatPath)
	if err != nil {
		return nil, err
	}
	chat.Global = true // Ensure the flag is set

	// Release old lock, acquire new one
	s.releasePreviousLock()
	if err := AcquireLock(chatDir); err != nil {
		// Lock failure is not fatal
	} else {
		s.lockedChatDir = chatDir
	}

	s.ActiveChat = chat
	saveActiveChatIDAt(s.globalChatsDir(), chat.ID)
	if s.ProjectRoot != "" {
		s.saveActiveGlobalChatID(chat.ID)
	}
	return chat, nil
}

// ChatDeleteGlobal removes a global chat by ID.
func (s *State) ChatDeleteGlobal(id string) error {
	chatDir := s.globalChatDir(id)
	if _, err := os.Stat(chatDir); os.IsNotExist(err) {
		return ErrChatNotFound
	}

	// Check if locked by another process
	if IsLocked(chatDir) {
		return ErrChatLocked
	}

	if s.ActiveChat != nil && s.ActiveChat.ID == id && s.ActiveChat.Global {
		s.releasePreviousLock()
		s.ActiveChat = nil
		saveActiveChatIDAt(s.globalChatsDir(), "")
		if s.ProjectRoot != "" {
			s.saveActiveChatID("")
		}
	}

	if err := os.RemoveAll(chatDir); err != nil {
		return err
	}
	if err := removeChatIndexEntryAt(s.globalChatsDir(), id); err != nil {
		// Index is a cache
	}
	return nil
}

// ChatRenameGlobal updates a global chat's name.
func (s *State) ChatRenameGlobal(id, name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrChatNameEmpty
	}

	chatDir := s.globalChatDir(id)
	chatPath := s.globalChatJSONPath(id)

	if s.ActiveChat != nil && s.ActiveChat.ID == id && s.ActiveChat.Global {
		s.ActiveChat.Name = name
		if err := saveChatAt(chatDir, s.ActiveChat); err != nil {
			return err
		}
		updateChatIndexEntryAt(s.globalChatsDir(), s.ActiveChat)
		return nil
	}

	chat, err := loadChatFrom(chatPath)
	if err != nil {
		return err
	}
	chat.Name = name
	if err := saveChatAt(chatDir, chat); err != nil {
		return err
	}
	updateChatIndexEntryAt(s.globalChatsDir(), chat)
	return nil
}

// SearchChatsGlobal searches through all global chats by title and content.
func (s *State) SearchChatsGlobal(query string) ([]ChatSearchResult, error) {
	if err := s.ensureGlobalChatsDir(); err != nil {
		return nil, err
	}

	chatsDir := s.globalChatsDir()
	entries, err := os.ReadDir(chatsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ChatSearchResult{}, nil
		}
		return nil, err
	}

	queryLower := strings.ToLower(query)
	var results []ChatSearchResult

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		chatPath := filepath.Join(chatsDir, entry.Name(), "chat.json")
		chat, err := loadChatFrom(chatPath)
		if err != nil {
			continue
		}

		if query == "" {
			results = append(results, ChatSearchResult{
				ID:        chat.ID,
				Name:      chat.Name,
				Created:   chat.Created,
				MatchType: "title",
			})
			continue
		}

		if strings.Contains(strings.ToLower(chat.Name), queryLower) {
			results = append(results, ChatSearchResult{
				ID:        chat.ID,
				Name:      chat.Name,
				Created:   chat.Created,
				MatchType: "title",
			})
			continue
		}

		excerpt := searchChatContent(chat, queryLower)
		if excerpt != "" {
			results = append(results, ChatSearchResult{
				ID:        chat.ID,
				Name:      chat.Name,
				Created:   chat.Created,
				MatchType: "content",
				Excerpt:   excerpt,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Created.After(results[j].Created)
	})

	return results, nil
}

// ChatForceUnlock forcefully removes the lock on a chat.
func (s *State) ChatForceUnlock(id string, global bool) error {
	chatDir := s.chatDirFor(id, global)
	return ForceUnlock(chatDir)
}

// SaveActiveChatGlobal persists the current active global chat to disk.
func (s *State) SaveActiveChatGlobal() error {
	if s.ActiveChat == nil {
		return ErrNoActiveChat
	}
	chatDir := s.globalChatDir(s.ActiveChat.ID)
	if err := saveChatAt(chatDir, s.ActiveChat); err != nil {
		return err
	}
	updateChatIndexEntryAt(s.globalChatsDir(), s.ActiveChat)
	return nil
}

// ForkChatGlobal creates a new global chat from an existing global conversation.
func (s *State) ForkChatGlobal(chatID string, forkIndex int) (*ForkChatResult, error) {
	chatsDir := s.globalChatsDir()

	// Load source chat
	sourceChat, err := loadChatFrom(filepath.Join(chatsDir, chatID, "chat.json"))
	if err != nil {
		return nil, err
	}

	if forkIndex < 0 || forkIndex >= len(sourceChat.Messages) {
		return nil, errors.New("fork index out of range")
	}

	forkMsg := sourceChat.Messages[forkIndex]
	if forkMsg.Role != "user" {
		return nil, errors.New("can only fork from user messages")
	}

	newID, err := generateID()
	if err != nil {
		return nil, err
	}

	newName := sourceChat.Name
	if !strings.HasPrefix(newName, "Fork of ") {
		newName = "Fork of " + newName
	}

	newChat := &Chat{
		ID:              newID,
		Name:            newName,
		Created:         time.Now().UTC(),
		Model:           sourceChat.Model,
		ReasoningEffort: sourceChat.ReasoningEffort,
		Global:          true,
		Draft:           MessageText(forkMsg),
		ContextFiles:    []ContextFile{},
		Messages:        []Message{},
	}

	newChatDir := s.globalChatDir(newID)
	if err := os.MkdirAll(filepath.Join(newChatDir, "context"), 0755); err != nil {
		return nil, err
	}

	// Copy messages up to (not including) forkIndex
	if forkIndex > 0 {
		newChat.Messages = make([]Message, forkIndex)
		copy(newChat.Messages, sourceChat.Messages[:forkIndex])
	}

	// Copy context files from source chat
	var warnings []ContextWarning
	snapshot := forkMsg.ContextSnapshot
	if len(snapshot) == 0 && len(sourceChat.ContextFiles) > 0 {
		snapshot = make([]ContextFileRef, len(sourceChat.ContextFiles))
		for i, cf := range sourceChat.ContextFiles {
			snapshot[i] = ContextFileRef{
				Path:      cf.Path,
				FileID:    cf.Version,
				StartLine: cf.StartLine,
				EndLine:   cf.EndLine,
			}
		}
	}

	srcContextDir := filepath.Join(chatsDir, chatID, "context")
	for _, ref := range snapshot {
		// Determine source path
		var srcPath string
		if ref.StartLine > 0 && ref.EndLine > 0 {
			srcPath = filepath.Join(srcContextDir, sectionsDir, hashSectionKey(ref.Path, ref.StartLine, ref.EndLine))
		} else if filepath.IsAbs(ref.Path) {
			srcPath = filepath.Join(srcContextDir, externalDir, hashPath(ref.Path))
		} else {
			var joinErr error
			srcPath, joinErr = SafeJoin(srcContextDir, ref.Path)
			if joinErr != nil {
				return nil, joinErr
			}
		}

		content, err := os.ReadFile(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				warnings = append(warnings, ContextWarning{
					Path:            ref.Path,
					Issue:           "deleted",
					OriginalVersion: ref.FileID,
				})
				continue
			}
			return nil, err
		}

		// Check if file still exists on filesystem
		if _, err := os.Stat(ref.Path); os.IsNotExist(err) {
			warnings = append(warnings, ContextWarning{
				Path:            ref.Path,
				Issue:           "deleted",
				OriginalVersion: ref.FileID,
			})
			continue
		}

		currentVersion := HashFileVersion(ref.Path, string(content))
		if currentVersion != ref.FileID {
			warnings = append(warnings, ContextWarning{
				Path:            ref.Path,
				Issue:           "modified",
				OriginalVersion: ref.FileID,
			})
		}

		// Write to new chat's context directory
		newContextDir := filepath.Join(newChatDir, "context")
		var dstPath string
		if ref.StartLine > 0 && ref.EndLine > 0 {
			dstPath = filepath.Join(newContextDir, sectionsDir, hashSectionKey(ref.Path, ref.StartLine, ref.EndLine))
		} else if filepath.IsAbs(ref.Path) {
			dstPath = filepath.Join(newContextDir, externalDir, hashPath(ref.Path))
		} else {
			var joinErr error
			dstPath, joinErr = SafeJoin(newContextDir, ref.Path)
			if joinErr != nil {
				return nil, joinErr
			}
		}

		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			return nil, err
		}

		var readOnly, external bool
		for _, cf := range sourceChat.ContextFiles {
			if cf.Path == ref.Path && cf.StartLine == ref.StartLine && cf.EndLine == ref.EndLine {
				readOnly = cf.ReadOnly
				external = cf.External
				break
			}
		}

		newChat.ContextFiles = append(newChat.ContextFiles, ContextFile{
			Path:      ref.Path,
			ReadOnly:  readOnly,
			External:  external,
			Version:   currentVersion,
			StartLine: ref.StartLine,
			EndLine:   ref.EndLine,
		})
	}

	if len(warnings) > 0 {
		var parts []MessagePart
		for _, w := range warnings {
			action := "ForkWarningModified"
			if w.Issue == "deleted" {
				action = "ForkWarningDeleted"
			}
			parts = append(parts, MessagePart{
				Type:    "context_event",
				Action:  action,
				Path:    w.Path,
				Version: w.OriginalVersion,
			})
		}
		newChat.Messages = append(newChat.Messages, Message{
			Role:      "system",
			Parts:     parts,
			Timestamp: time.Now().UTC(),
		})
	}

	if err := saveChatAt(newChatDir, newChat); err != nil {
		os.RemoveAll(newChatDir)
		return nil, err
	}

	// Release old lock, acquire new one
	s.releasePreviousLock()
	if err := AcquireLock(newChatDir); err != nil {
		// Lock failure is not fatal
	} else {
		s.lockedChatDir = newChatDir
	}

	s.ActiveChat = newChat
	saveActiveChatIDAt(s.globalChatsDir(), newChat.ID)

	return &ForkChatResult{
		NewChatID:          newID,
		ForkMessageContent: MessageText(forkMsg),
		ContextWarnings:    warnings,
	}, nil
}
