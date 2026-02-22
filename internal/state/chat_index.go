package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

const chatIndexVersion = 1

type chatIndex struct {
	Version      int           `json:"version"`
	ActiveChatID string        `json:"active_chat_id,omitempty"`
	Chats        []ChatSummary `json:"chats"`
}

// chatIndexPathFor returns the index.json path for a given chats directory.
func chatIndexPathFor(chatsDir string) string {
	return filepath.Join(chatsDir, "index.json")
}

func (s *State) chatIndexPath() string {
	return chatIndexPathFor(s.chatsDir())
}

// loadChatIndexFrom reads and decodes the chat index from a chats directory.
func loadChatIndexFrom(chatsDir string) (chatIndex, error) {
	file, err := os.Open(chatIndexPathFor(chatsDir))
	if err != nil {
		if os.IsNotExist(err) {
			return chatIndex{}, ErrFileNotFound
		}
		return chatIndex{}, err
	}
	defer file.Close()

	var idx chatIndex
	dec := json.NewDecoder(file)
	if err := dec.Decode(&idx); err != nil {
		return chatIndex{}, err
	}
	if idx.Version == 0 {
		idx.Version = chatIndexVersion
	}
	if idx.Version != chatIndexVersion {
		return chatIndex{}, errors.New("unsupported chat index version")
	}
	return idx, nil
}

func (s *State) loadChatIndex() (chatIndex, error) {
	return loadChatIndexFrom(s.chatsDir())
}

// writeChatIndexTo writes the chat index to a chats directory.
func writeChatIndexTo(chatsDir string, idx chatIndex) error {
	idx.Version = chatIndexVersion
	sort.Slice(idx.Chats, func(i, j int) bool {
		return idx.Chats[i].Created.After(idx.Chats[j].Created)
	})

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(chatIndexPathFor(chatsDir), data, 0644)
}

func (s *State) writeChatIndex(idx chatIndex) error {
	return writeChatIndexTo(s.chatsDir(), idx)
}

// ensureChatIndexAt validates the chat index in a chats directory and repairs it if needed.
func ensureChatIndexAt(chatsDir string) (chatIndex, error) {
	idx, err := loadChatIndexFrom(chatsDir)
	if err != nil {
		idx = chatIndex{Version: chatIndexVersion}
	}

	entries, readErr := os.ReadDir(chatsDir)
	if readErr != nil {
		if errors.Is(err, ErrFileNotFound) {
			return idx, nil
		}
		return chatIndex{}, readErr
	}

	changed := err != nil
	seen := make(map[string]ChatSummary, len(idx.Chats))
	for _, chat := range idx.Chats {
		if chat.ID == "" {
			changed = true
			continue
		}
		seen[chat.ID] = chat
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		chatID := entry.Name()
		chatPath := filepath.Join(chatsDir, chatID, "chat.json")
		if _, statErr := os.Stat(chatPath); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				if _, ok := seen[chatID]; ok {
					changed = true
					delete(seen, chatID)
				}
				continue
			}
			return chatIndex{}, statErr
		}
		if _, ok := seen[chatID]; ok {
			continue
		}
		summary, summaryErr := loadChatSummaryFrom(filepath.Join(chatsDir, chatID, "chat.json"))
		if summaryErr != nil {
			changed = true
			continue
		}
		seen[chatID] = summary
		changed = true
	}

	idx.Chats = idx.Chats[:0]
	for _, summary := range seen {
		idx.Chats = append(idx.Chats, summary)
	}

	if changed {
		if writeErr := writeChatIndexTo(chatsDir, idx); writeErr != nil {
			return idx, writeErr
		}
	}

	return idx, nil
}

// ensureChatIndex validates the project chat index and repairs it if needed.
func (s *State) ensureChatIndex() (chatIndex, error) {
	return ensureChatIndexAt(s.chatsDir())
}

// updateChatIndexEntryAt updates or inserts a chat summary in the index at a chats directory.
func updateChatIndexEntryAt(chatsDir string, chat *Chat) error {
	if chat == nil {
		return nil
	}

	idx, err := ensureChatIndexAt(chatsDir)
	if err != nil {
		return err
	}

	found := false
	for i := range idx.Chats {
		if idx.Chats[i].ID == chat.ID {
			idx.Chats[i].Name = chat.Name
			idx.Chats[i].Created = chat.Created
			found = true
			break
		}
	}
	if !found {
		idx.Chats = append(idx.Chats, ChatSummary{
			ID:      chat.ID,
			Name:    chat.Name,
			Created: chat.Created,
		})
	}

	return writeChatIndexTo(chatsDir, idx)
}

// updateChatIndexEntry updates or inserts a chat summary in the project index.
func (s *State) updateChatIndexEntry(chat *Chat) error {
	return updateChatIndexEntryAt(s.chatsDir(), chat)
}

// saveActiveChatIDAt persists the active chat ID to the index at a chats directory.
func saveActiveChatIDAt(chatsDir string, id string) {
	idx, err := ensureChatIndexAt(chatsDir)
	if err != nil {
		return
	}
	idx.ActiveChatID = id
	writeChatIndexTo(chatsDir, idx)
}

// saveActiveChatID persists the active chat ID to the project index.
func (s *State) saveActiveChatID(id string) {
	saveActiveChatIDAt(s.chatsDir(), id)
}

// removeChatIndexEntryAt removes a chat from the index at a chats directory.
func removeChatIndexEntryAt(chatsDir string, chatID string) error {
	if chatID == "" {
		return nil
	}

	idx, err := ensureChatIndexAt(chatsDir)
	if err != nil {
		return err
	}

	filtered := idx.Chats[:0]
	for _, chat := range idx.Chats {
		if chat.ID == chatID {
			continue
		}
		filtered = append(filtered, chat)
	}
	idx.Chats = filtered

	return writeChatIndexTo(chatsDir, idx)
}

// removeChatIndexEntry removes a chat from the project index.
func (s *State) removeChatIndexEntry(chatID string) error {
	return removeChatIndexEntryAt(s.chatsDir(), chatID)
}

// loadChatSummaryFrom reads chat.json and extracts only summary fields.
func loadChatSummaryFrom(chatJSONPath string) (ChatSummary, error) {
	file, err := os.Open(chatJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ChatSummary{}, ErrChatNotFound
		}
		return ChatSummary{}, err
	}
	defer file.Close()

	var summary chatSummaryFile
	dec := json.NewDecoder(file)
	if err := dec.Decode(&summary); err != nil {
		return ChatSummary{}, err
	}

	return ChatSummary{
		ID:      summary.ID,
		Name:    summary.Name,
		Created: summary.Created,
	}, nil
}
