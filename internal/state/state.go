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
)

// State holds the runtime state of BB-7.
type State struct {
	ProjectRoot string
	ActiveChat  *Chat
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
func (s *State) Init(projectRoot string) error {
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
	return nil
}

// Initialized returns true if Init has been called.
func (s *State) Initialized() bool {
	return s.ProjectRoot != ""
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

// Guard functions

func (s *State) requireInit() error {
	if !s.Initialized() {
		return ErrNotInitialized
	}
	return nil
}

func (s *State) requireActiveChat() error {
	if err := s.requireInit(); err != nil {
		return err
	}
	if s.ActiveChat == nil {
		return ErrNoActiveChat
	}
	return nil
}
