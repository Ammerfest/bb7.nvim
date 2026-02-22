package state

import (
	"errors"
	"os"
	"path/filepath"
)

// Sentinel errors for expected conditions.
var (
	ErrNotInitialized   = errors.New("bb7 not initialized: call init first")
	ErrNotBB7Project    = errors.New("not a bb7 project: .bb7 directory not found")
	ErrAlreadyInit      = errors.New("bb7 already initialized")
	ErrNoActiveChat     = errors.New("no active chat")
	ErrChatNotFound     = errors.New("chat not found")
	ErrChatNameEmpty    = errors.New("chat name cannot be empty")
	ErrFileNotFound     = errors.New("file not found")
	ErrFileExists       = errors.New("context file already exists")
	ErrExternalReadOnly = errors.New("external files are always read-only")
	ErrContextModified  = errors.New("file has pending output")
	ErrChatLocked       = errors.New("chat is locked by another process")
	ErrGlobalReadOnly   = errors.New("global chats are read-only: file operations are not available")
)

// State holds the runtime state of BB-7.
type State struct {
	ProjectRoot    string
	ActiveChat     *Chat
	GlobalOnly     bool   // True when no project root is set (global-only mode)
	lockedChatDir  string // Chat directory currently locked by this instance
}

// New creates a new State instance. ProjectRoot must be set via Init.
func New() *State {
	return &State{}
}

// ProjectInit creates the .bb7 directory structure (like git init).
// Returns ErrAlreadyInit if already initialized.
func (s *State) ProjectInit(projectRoot string) error {
	info, err := os.Stat(projectRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("project_root must be a directory")
	}

	bb7Dir := filepath.Join(projectRoot, ".bb7")

	// Check if already initialized
	if _, err := os.Stat(bb7Dir); err == nil {
		return ErrAlreadyInit
	}

	// Create .bb7/chats directory
	chatsDir := filepath.Join(bb7Dir, "chats")
	if err := os.MkdirAll(chatsDir, 0755); err != nil {
		return err
	}

	return nil
}

// Init sets the project root. Requires .bb7 directory to exist.
// Call ProjectInit first to create the directory structure.
// If projectRoot is empty, enters global-only mode.
func (s *State) Init(projectRoot string) error {
	if projectRoot == "" {
		// Global-only mode: no project root, only global chats available
		s.GlobalOnly = true
		if err := s.ensureGlobalChatsDir(); err != nil {
			return err
		}
		// Best-effort restore of last active global chat.
		idx, err := loadChatIndexFrom(s.globalChatsDir())
		if err == nil && idx.ActiveChatID != "" {
			s.ChatSelectGlobal(idx.ActiveChatID)
		}
		return nil
	}

	info, err := os.Stat(projectRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("project_root must be a directory")
	}

	// Check that .bb7 directory exists
	bb7Dir := filepath.Join(projectRoot, ".bb7")
	if _, err := os.Stat(bb7Dir); os.IsNotExist(err) {
		return ErrNotBB7Project
	}

	s.ProjectRoot = projectRoot

	// Best-effort restore of last active chat.
	idx, err := s.loadChatIndex()
	if err == nil && idx.ActiveChatID != "" {
		s.ChatSelect(idx.ActiveChatID)
	}

	return nil
}

// Initialized returns true if Init has been called (project mode or global-only mode).
func (s *State) Initialized() bool {
	return s.ProjectRoot != "" || s.GlobalOnly
}

// Path helpers

func (s *State) bb7Dir() string {
	return filepath.Join(s.ProjectRoot, ".bb7")
}

func (s *State) chatsDir() string {
	return filepath.Join(s.bb7Dir(), "chats")
}

func (s *State) chatDir(chatID string) string {
	return filepath.Join(s.chatsDir(), chatID)
}

func (s *State) contextDir(chatID string) string {
	return filepath.Join(s.chatDir(chatID), "context")
}

func (s *State) outputDir(chatID string) string {
	return filepath.Join(s.chatDir(chatID), "output")
}

func (s *State) chatJSONPath(chatID string) string {
	return filepath.Join(s.chatDir(chatID), "chat.json")
}

// Global path helpers

func globalBB7Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".bb7")
	}
	return filepath.Join(home, ".bb7")
}

func (s *State) globalChatsDir() string {
	return filepath.Join(globalBB7Dir(), "chats")
}

func (s *State) ensureGlobalChatsDir() error {
	return os.MkdirAll(s.globalChatsDir(), 0755)
}

// globalChatDir returns the directory for a specific global chat.
func (s *State) globalChatDir(chatID string) string {
	return filepath.Join(s.globalChatsDir(), chatID)
}

func (s *State) globalChatJSONPath(chatID string) string {
	return filepath.Join(s.globalChatDir(chatID), "chat.json")
}

func (s *State) globalContextDir(chatID string) string {
	return filepath.Join(s.globalChatDir(chatID), "context")
}

// chatDirFor returns the chat directory for a given chat ID and global flag.
func (s *State) chatDirFor(chatID string, global bool) string {
	if global {
		return s.globalChatDir(chatID)
	}
	return s.chatDir(chatID)
}

// chatsDirFor returns the chats directory for a given global flag.
func (s *State) chatsDirFor(global bool) string {
	if global {
		return s.globalChatsDir()
	}
	return s.chatsDir()
}

// Cleanup releases any locks held by this instance.
func (s *State) Cleanup() {
	if s.lockedChatDir != "" {
		ReleaseLock(s.lockedChatDir)
		s.lockedChatDir = ""
	}
}

// Guard functions

func (s *State) requireInit() error {
	if !s.Initialized() {
		return ErrNotInitialized
	}
	return nil
}

func (s *State) requireActiveChat() error {
	if !s.Initialized() {
		return ErrNotInitialized
	}
	if s.ActiveChat == nil {
		return ErrNoActiveChat
	}
	return nil
}
