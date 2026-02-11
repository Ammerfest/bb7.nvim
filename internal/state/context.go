package state

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// externalDir is the subdirectory for external file snapshots.
const externalDir = "_external"

// sectionsDir is the subdirectory for file section snapshots.
const sectionsDir = "_sections"

// ContextAdd copies a file's content into the chat's context directory.
// For internal files, path must be relative to project root.
// For external files, path must be absolute.
// External files are automatically marked as read-only.
func (s *State) ContextAdd(path, content string) error {
	return s.ContextAddWithReadOnly(path, content, false)
}

// ContextAddWithReadOnly copies a file's content into the chat's context directory.
// Internal files can be marked read-only; external files are always read-only.
func (s *State) ContextAddWithReadOnly(path, content string, readOnly bool) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	normalizedPath := path
	isExternal := filepath.IsAbs(path)
	if isExternal {
		// External file: verify it's actually outside project
		within, err := IsWithinDir(s.ProjectRoot, path)
		if err != nil {
			return err
		}
		if within {
			// It's inside the project - convert to relative and treat as internal.
			relPath, err := RelativeToBase(s.ProjectRoot, path)
			if err != nil {
				return err
			}
			normalizedPath = relPath
			isExternal = false
		}
	} else {
		// Internal file: validate relative path.
		if err := ValidateRelativePath(path); err != nil {
			return err
		}
	}

	// Check if already in context after canonicalization.
	if s.findContextFile(normalizedPath) != nil {
		return ErrFileExists
	}

	if isExternal {
		return s.addExternalFile(normalizedPath, content)
	}

	return s.addInternalFile(normalizedPath, content, readOnly)
}

// addInternalFile adds a file that's inside the project directory.
func (s *State) addInternalFile(relPath, content string, readOnly bool) error {
	// Validate path stays within context directory
	contextBase := s.contextDir(s.ActiveChat.ID)
	fullPath, err := SafeJoin(contextBase, relPath)
	if err != nil {
		return err
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return err
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return err
	}

	version := HashFileVersion(relPath, content)
	s.ActiveChat.ContextFiles = append(s.ActiveChat.ContextFiles, ContextFile{
		Path:     relPath,
		ReadOnly: readOnly,
		External: false,
		Version:  version,
	})

	return s.addContextEvent(ContextEvent{
		Action:   "UserAddFile",
		Path:     relPath,
		ReadOnly: readOnly,
		External: false,
		Version:  version,
	})
}

// addExternalFile adds a file that's outside the project directory.
// External files are always read-only.
func (s *State) addExternalFile(absPath, content string) error {
	// Store in _external subdirectory with hashed filename to avoid conflicts
	externalBase := filepath.Join(s.contextDir(s.ActiveChat.ID), externalDir)
	if err := os.MkdirAll(externalBase, 0755); err != nil {
		return err
	}

	// Use hash of absolute path for unique filename, preserve extension
	storageName := hashPath(absPath)
	storagePath := filepath.Join(externalBase, storageName)

	if err := os.WriteFile(storagePath, []byte(content), 0644); err != nil {
		return err
	}

	version := HashFileVersion(absPath, content)
	s.ActiveChat.ContextFiles = append(s.ActiveChat.ContextFiles, ContextFile{
		Path:     absPath,
		ReadOnly: true, // External files are always read-only
		External: true,
		Version:  version,
	})

	return s.addContextEvent(ContextEvent{
		Action:   "UserAddFile",
		Path:     absPath,
		ReadOnly: true,
		External: true,
		Version:  version,
	})
}

// hashPath creates a storage-safe filename from an absolute path.
func hashPath(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	hash := hex.EncodeToString(h[:8]) // First 8 bytes = 16 hex chars
	ext := filepath.Ext(absPath)
	return hash + ext
}

// hashSectionKey creates a storage-safe filename from path:start:end.
func hashSectionKey(path string, startLine, endLine int) string {
	key := fmt.Sprintf("%s:%d:%d", path, startLine, endLine)
	h := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(h[:8]) // First 8 bytes = 16 hex chars
	ext := filepath.Ext(path)
	return hash + ext
}

// ContextAddSection adds a file section (partial content) to context.
// Sections are immutable read-only snapshots. StartLine and EndLine are 1-indexed inclusive.
func (s *State) ContextAddSection(path string, startLine, endLine int, content string) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	// Validate line numbers
	if startLine <= 0 || endLine <= 0 {
		return fmt.Errorf("line numbers must be positive (1-indexed)")
	}
	if startLine > endLine {
		return fmt.Errorf("start line (%d) cannot be greater than end line (%d)", startLine, endLine)
	}

	// Determine if external (absolute path outside project)
	normalizedPath := path
	isExternal := filepath.IsAbs(path)
	if isExternal {
		within, err := IsWithinDir(s.ProjectRoot, path)
		if err != nil {
			return err
		}
		if within {
			// Inside project - convert to relative
			relPath, err := RelativeToBase(s.ProjectRoot, path)
			if err != nil {
				return err
			}
			normalizedPath = relPath
			isExternal = false
		}
	} else {
		// Validate relative path
		if err := ValidateRelativePath(path); err != nil {
			return err
		}
	}

	// Check if this exact section already exists after canonicalization.
	for _, cf := range s.ActiveChat.ContextFiles {
		if cf.Path == normalizedPath && cf.StartLine == startLine && cf.EndLine == endLine {
			return ErrFileExists
		}
	}

	// Store in _sections subdirectory with hashed filename
	sectionsBase := filepath.Join(s.contextDir(s.ActiveChat.ID), sectionsDir)
	if err := os.MkdirAll(sectionsBase, 0755); err != nil {
		return err
	}

	storageName := hashSectionKey(normalizedPath, startLine, endLine)
	storagePath := filepath.Join(sectionsBase, storageName)

	if err := os.WriteFile(storagePath, []byte(content), 0644); err != nil {
		return err
	}

	version := HashFileVersion(fmt.Sprintf("%s:%d:%d", normalizedPath, startLine, endLine), content)
	s.ActiveChat.ContextFiles = append(s.ActiveChat.ContextFiles, ContextFile{
		Path:      normalizedPath,
		ReadOnly:  true, // Sections are always read-only
		External:  isExternal,
		Version:   version,
		StartLine: startLine,
		EndLine:   endLine,
	})

	readOnly := true
	return s.addContextEvent(ContextEvent{
		Action:    "UserAddSection",
		Path:      normalizedPath,
		ReadOnly:  readOnly,
		External:  isExternal,
		Version:   version,
		StartLine: startLine,
		EndLine:   endLine,
	})
}

// ContextRemove removes a file from the context list.
// The snapshot file is kept for history purposes.
func (s *State) ContextRemove(path string) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	idx := s.findContextFileIndex(path)
	if idx == -1 {
		return ErrFileNotFound
	}

	cf := s.ActiveChat.ContextFiles[idx]
	version := cf.Version
	if version == "" {
		content, err := s.GetContextFile(cf.Path)
		if err != nil {
			return err
		}
		version = HashFileVersion(cf.Path, content)
	}

	s.ActiveChat.ContextFiles = append(
		s.ActiveChat.ContextFiles[:idx],
		s.ActiveChat.ContextFiles[idx+1:]...,
	)

	return s.addContextEvent(ContextEvent{
		Action:   "UserRemoveFile",
		Path:     cf.Path,
		ReadOnly: cf.ReadOnly,
		External: cf.External,
		Version:  version,
	})
}

// ContextRemoveSection removes a file section from the context list.
// The snapshot file is kept for history purposes.
func (s *State) ContextRemoveSection(path string, startLine, endLine int) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	idx := -1
	for i, cf := range s.ActiveChat.ContextFiles {
		if cf.Path == path && cf.StartLine == startLine && cf.EndLine == endLine {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrFileNotFound
	}

	cf := s.ActiveChat.ContextFiles[idx]
	version := cf.Version

	s.ActiveChat.ContextFiles = append(
		s.ActiveChat.ContextFiles[:idx],
		s.ActiveChat.ContextFiles[idx+1:]...,
	)

	return s.addContextEvent(ContextEvent{
		Action:    "UserRemoveSection",
		Path:      cf.Path,
		ReadOnly:  cf.ReadOnly,
		External:  cf.External,
		Version:   version,
		StartLine: startLine,
		EndLine:   endLine,
	})
}

// ContextSetReadOnly toggles the read-only flag for an internal context file.
// External files are always read-only and cannot be made writable.
func (s *State) ContextSetReadOnly(path string, readOnly bool) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	cf := s.findContextFile(path)
	if cf == nil {
		return ErrFileNotFound
	}

	if cf.External {
		if !readOnly {
			return ErrExternalReadOnly
		}
		return nil
	}

	if readOnly {
		if _, err := s.GetOutputFile(path); err == nil {
			return ErrContextModified
		}
	}

	if cf.ReadOnly == readOnly {
		return nil
	}

	version := cf.Version
	if version == "" {
		content, err := s.GetContextFile(cf.Path)
		if err != nil {
			return err
		}
		version = HashFileVersion(cf.Path, content)
		cf.Version = version
	}

	cf.ReadOnly = readOnly
	return s.addContextEvent(ContextEvent{
		Action:   "UserSetReadOnly",
		Path:     cf.Path,
		ReadOnly: cf.ReadOnly,
		External: cf.External,
		Version:  version,
	})
}

// ContextUpdate replaces the snapshot content for a context file and updates its version.
func (s *State) ContextUpdate(path, content string) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	cf := s.findContextFile(path)
	if cf == nil {
		return ErrFileNotFound
	}

	prevVersion := cf.Version
	if prevVersion == "" {
		existing, err := s.GetContextFile(cf.Path)
		if err != nil {
			return err
		}
		prevVersion = HashFileVersion(cf.Path, existing)
	}

	storagePath, err := s.contextStoragePath(cf)
	if err != nil {
		return err
	}

	if err := os.WriteFile(storagePath, []byte(content), 0644); err != nil {
		return err
	}

	cf.Version = HashFileVersion(cf.Path, content)

	return s.addContextEvent(ContextEvent{
		Action:      "UserWriteFile",
		Path:        cf.Path,
		ReadOnly:    cf.ReadOnly,
		External:    cf.External,
		Version:     cf.Version,
		PrevVersion: prevVersion,
	})
}

// ContextList returns the list of context files for the active chat.
func (s *State) ContextList() ([]ContextFile, error) {
	if err := s.requireActiveChat(); err != nil {
		return nil, err
	}

	return s.ActiveChat.ContextFiles, nil
}

// GetContextFile returns the content of a context file.
func (s *State) GetContextFile(path string) (string, error) {
	if err := s.requireActiveChat(); err != nil {
		return "", err
	}

	cf := s.findContextFile(path)
	if cf == nil {
		return "", ErrFileNotFound
	}

	storagePath, err := s.contextStoragePath(cf)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(storagePath)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// IsReadOnly returns true if the given path is marked as read-only in context.
func (s *State) IsReadOnly(path string) bool {
	cf := s.findContextFile(path)
	if cf == nil {
		return false
	}
	return cf.ReadOnly
}

// contextStoragePath returns the actual filesystem path for a context file.
func (s *State) contextStoragePath(cf *ContextFile) (string, error) {
	contextBase := s.contextDir(s.ActiveChat.ID)

	// Sections are stored in _sections subdirectory
	if cf.StartLine > 0 && cf.EndLine > 0 {
		return filepath.Join(contextBase, sectionsDir, hashSectionKey(cf.Path, cf.StartLine, cf.EndLine)), nil
	}

	if cf.External {
		return filepath.Join(contextBase, externalDir, hashPath(cf.Path)), nil
	}

	return SafeJoin(contextBase, cf.Path)
}

// findContextFile returns the ContextFile for the given path, or nil if not found.
func (s *State) findContextFile(path string) *ContextFile {
	for i := range s.ActiveChat.ContextFiles {
		if s.ActiveChat.ContextFiles[i].Path == path {
			return &s.ActiveChat.ContextFiles[i]
		}
	}
	return nil
}

// FindContextFile returns the ContextFile for the given path, or nil if not found.
// Exported version of findContextFile for use by main.go.
func (s *State) FindContextFile(path string) *ContextFile {
	return s.findContextFile(path)
}

// FindContextSection returns the ContextFile for the given path and line range, or nil if not found.
func (s *State) FindContextSection(path string, startLine, endLine int) *ContextFile {
	for i := range s.ActiveChat.ContextFiles {
		cf := &s.ActiveChat.ContextFiles[i]
		if cf.Path == path && cf.StartLine == startLine && cf.EndLine == endLine {
			return cf
		}
	}
	return nil
}

// IsSection returns true if the context file is a section (partial file).
func (cf *ContextFile) IsSection() bool {
	return cf.StartLine > 0 && cf.EndLine > 0
}

// findContextFileIndex returns the index of the context file, or -1 if not found.
func (s *State) findContextFileIndex(path string) int {
	for i, cf := range s.ActiveChat.ContextFiles {
		if cf.Path == path {
			return i
		}
	}
	return -1
}

// ContextFilePaths returns just the paths of context files (for compatibility).
func (s *State) ContextFilePaths() []string {
	paths := make([]string, len(s.ActiveChat.ContextFiles))
	for i, cf := range s.ActiveChat.ContextFiles {
		paths[i] = cf.Path
	}
	return paths
}

// HasContextFile returns true if the given path exists in the active chat's context.
func (s *State) HasContextFile(path string) bool {
	if s.ActiveChat == nil {
		return false
	}
	return s.findContextFile(path) != nil
}

// contextFilePath returns the storage path for a context file given a chat ID and ContextFileRef.
// This is used by ForkChat/EditUserMessage to read context files from snapshots.
func (s *State) contextFilePath(chatID string, ref ContextFileRef) (string, error) {
	contextBase := s.contextDir(chatID)

	// Sections are stored in _sections subdirectory
	if ref.StartLine > 0 && ref.EndLine > 0 {
		return filepath.Join(contextBase, sectionsDir, hashSectionKey(ref.Path, ref.StartLine, ref.EndLine)), nil
	}

	// External files are stored in _external subdirectory
	if filepath.IsAbs(ref.Path) {
		return filepath.Join(contextBase, externalDir, hashPath(ref.Path)), nil
	}

	// Internal file refs come from persisted chat snapshots, so treat them as untrusted.
	return SafeJoin(contextBase, ref.Path)
}
