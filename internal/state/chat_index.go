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
	Version int           `json:"version"`
	Chats   []ChatSummary `json:"chats"`
}

func (s *State) chatIndexPath() string {
	return filepath.Join(s.chatsDir(), "index.json")
}

func (s *State) loadChatIndex() (chatIndex, error) {
	file, err := os.Open(s.chatIndexPath())
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

func (s *State) writeChatIndex(idx chatIndex) error {
	idx.Version = chatIndexVersion
	sort.Slice(idx.Chats, func(i, j int) bool {
		return idx.Chats[i].Created.After(idx.Chats[j].Created)
	})

	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.chatIndexPath(), data, 0644)
}

// ensureChatIndex validates the chat index and repairs it if needed.
func (s *State) ensureChatIndex() (chatIndex, error) {
	idx, err := s.loadChatIndex()
	if err != nil {
		idx = chatIndex{Version: chatIndexVersion}
	}

	entries, readErr := os.ReadDir(s.chatsDir())
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
		chatPath := s.chatJSONPath(chatID)
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
		summary, summaryErr := s.loadChatSummary(chatID)
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
		if writeErr := s.writeChatIndex(idx); writeErr != nil {
			return idx, writeErr
		}
	}

	return idx, nil
}

// updateChatIndexEntry updates or inserts a chat summary in the index.
func (s *State) updateChatIndexEntry(chat *Chat) error {
	if chat == nil {
		return nil
	}

	idx, err := s.ensureChatIndex()
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

	return s.writeChatIndex(idx)
}

// removeChatIndexEntry removes a chat from the index.
func (s *State) removeChatIndexEntry(chatID string) error {
	if chatID == "" {
		return nil
	}

	idx, err := s.ensureChatIndex()
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

	return s.writeChatIndex(idx)
}
