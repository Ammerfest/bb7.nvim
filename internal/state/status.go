package state

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/youruser/bb7/internal/llm"
)

// FileStatus represents the status of a file in the context/output system.
type FileStatus string

const (
	StatusUnchanged      FileStatus = ""   // In context, no output or output == context (applied)
	StatusModified       FileStatus = "M"  // In context, has different output
	StatusAdded          FileStatus = "A"  // Not in context, has output (LLM created new file)
	StatusConflictAdded  FileStatus = "!A" // Not in context, has output, but file exists locally
	StatusSection        FileStatus = "S"  // Section (partial file, immutable)
)

// FileInfo represents a file with its status and content info.
type FileInfo struct {
	Path           string     `json:"path"`
	Status         FileStatus `json:"status"`
	InContext      bool       `json:"in_context"`
	HasOutput      bool       `json:"has_output"`
	ReadOnly       bool       `json:"readonly"`
	External       bool       `json:"external"`
	ContextContent string     `json:"context_content,omitempty"` // For sync comparison
	OutputContent  string     `json:"output_content,omitempty"`  // For preview
	Tokens         int        `json:"tokens"`                    // Total tokens for this file
	OriginalTokens int        `json:"original_tokens,omitempty"` // Tokens in context version
	OutputTokens   int        `json:"output_tokens,omitempty"`   // Tokens in output version (if M status)
	StartLine      int        `json:"start_line,omitempty"`      // For sections: 1-indexed start line
	EndLine        int        `json:"end_line,omitempty"`        // For sections: 1-indexed end line (inclusive)
}

// GetFileStatuses returns status information for all files in context and output.
// This is the single source of truth for M/A status calculation.
func (s *State) GetFileStatuses() ([]FileInfo, error) {
	if err := s.requireActiveChat(); err != nil {
		return nil, err
	}

	var files []FileInfo

	// Get all output files (recursive)
	outputFiles, err := s.ListOutputFiles()
	if err != nil {
		return nil, err
	}
	outputSet := make(map[string]bool)
	for _, f := range outputFiles {
		outputSet[f] = true
	}

	// Process context files
	for _, cf := range s.ActiveChat.ContextFiles {
		contextContent, _ := s.GetContextFile(cf.Path)

		// Handle sections (partial files) - always read-only, no output
		if cf.IsSection() {
			contextTokens := llm.EstimateTokensSimple(contextContent)
			files = append(files, FileInfo{
				Path:           cf.Path,
				Status:         StatusSection,
				InContext:      true,
				HasOutput:      false,
				ReadOnly:       true,
				External:       cf.External,
				ContextContent: contextContent,
				Tokens:         contextTokens,
				OriginalTokens: contextTokens,
				StartLine:      cf.StartLine,
				EndLine:        cf.EndLine,
			})
			continue
		}

		// For external files, output lookup uses absolute path
		// For internal files, output uses relative path
		lookupPath := cf.Path
		if cf.External {
			// External files can't have output (read-only), so hasOutput is always false
			lookupPath = "" // Don't check
		}
		hasOutput := outputSet[lookupPath]

		info := FileInfo{
			Path:           cf.Path,
			InContext:      true,
			HasOutput:      hasOutput,
			ReadOnly:       cf.ReadOnly,
			External:       cf.External,
			ContextContent: contextContent,
		}

		// Calculate tokens for context content
		contextTokens := llm.EstimateTokensSimple(contextContent)
		info.OriginalTokens = contextTokens

		if hasOutput {
			outputContent, _ := s.GetOutputFile(lookupPath)
			info.OutputContent = outputContent

			// Calculate tokens for output content
			outputTokens := llm.EstimateTokensSimple(outputContent)
			info.OutputTokens = outputTokens

			// Compare content to determine if applied
			if normalizeContent(contextContent) == normalizeContent(outputContent) {
				info.Status = StatusUnchanged // Applied - context matches output
				info.Tokens = contextTokens   // Only one version sent
			} else {
				info.Status = StatusModified
				// M status: both versions are sent to LLM
				info.Tokens = contextTokens + outputTokens
			}
		} else {
			info.Status = StatusUnchanged
			info.Tokens = contextTokens
		}

		files = append(files, info)
		delete(outputSet, lookupPath)
	}

	// Add output-only files (LLM added)
	for path := range outputSet {
		outputContent, _ := s.GetOutputFile(path)
		outputTokens := llm.EstimateTokensSimple(outputContent)

		// Check if file exists locally (conflict)
		status := StatusAdded
		localPath := filepath.Join(s.ProjectRoot, path)
		if _, err := os.Stat(localPath); err == nil {
			status = StatusConflictAdded
		}

		files = append(files, FileInfo{
			Path:          path,
			Status:        status,
			InContext:     false,
			HasOutput:     true,
			ReadOnly:      false,
			External:      false,
			OutputContent: outputContent,
			Tokens:        outputTokens,
			OutputTokens:  outputTokens,
		})
	}

	return files, nil
}

// ApplyFile marks an output file as applied by updating context to match output.
// After applying, the output file is deleted (the change is now in context).
// Returns the content that was applied.
func (s *State) ApplyFile(path string) (string, error) {
	if err := s.requireActiveChat(); err != nil {
		return "", err
	}

	// Get output content
	content, err := s.GetOutputFile(path)
	if err != nil {
		return "", err
	}

	var prevVersion string
	cf := s.findContextFile(path)
	if cf != nil {
		if cf.Version != "" {
			prevVersion = cf.Version
		} else {
			existing, err := s.GetContextFile(cf.Path)
			if err != nil {
				return "", err
			}
			prevVersion = HashFileVersion(cf.Path, existing)
		}

		storagePath, err := s.contextStoragePath(cf)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(storagePath, []byte(content), 0644); err != nil {
			return "", err
		}
		cf.Version = HashFileVersion(cf.Path, content)
	} else {
		if err := ValidateRelativePath(path); err != nil {
			return "", err
		}

		contextBase := s.contextDir(s.ActiveChat.ID)
		fullPath, err := SafeJoin(contextBase, path)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return "", err
		}

		s.ActiveChat.ContextFiles = append(s.ActiveChat.ContextFiles, ContextFile{
			Path:     path,
			ReadOnly: false,
			External: false,
			Version:  HashFileVersion(path, content),
		})
		cf = s.findContextFile(path)
	}

	if cf == nil {
		return "", ErrFileNotFound
	}

	if err := s.addContextEvent(ContextEvent{
		Action:      "UserApplyFile",
		Path:        cf.Path,
		ReadOnly:    cf.ReadOnly,
		External:    cf.External,
		Version:     cf.Version,
		PrevVersion: prevVersion,
	}); err != nil {
		return "", err
	}

	// Delete the output file - it's now part of context
	s.DeleteOutputFile(path) // Ignore error (file might not exist)

	return content, nil
}

// ApplyFileAs saves an output file to a different destination path.
// Used when user chooses to save a conflicting file (!A status) to a new location.
// Records a UserSaveAs event so the LLM knows the file was renamed.
// Returns the content that was applied.
func (s *State) ApplyFileAs(originalPath, destPath string) (string, error) {
	if err := s.requireActiveChat(); err != nil {
		return "", err
	}

	// Get output content from original path
	content, err := s.GetOutputFile(originalPath)
	if err != nil {
		return "", err
	}

	// Validate destination path
	if err := ValidateRelativePath(destPath); err != nil {
		return "", err
	}

	// Add destination to context
	contextBase := s.contextDir(s.ActiveChat.ID)
	fullPath, err := SafeJoin(contextBase, destPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", err
	}

	s.ActiveChat.ContextFiles = append(s.ActiveChat.ContextFiles, ContextFile{
		Path:     destPath,
		ReadOnly: false,
		External: false,
		Version:  HashFileVersion(destPath, content),
	})
	cf := s.findContextFile(destPath)

	if cf == nil {
		return "", ErrFileNotFound
	}

	// Record the save-as event with both paths
	if err := s.addContextEvent(ContextEvent{
		Action:       "UserSaveAs",
		Path:         destPath,
		OriginalPath: originalPath,
		ReadOnly:     cf.ReadOnly,
		External:     cf.External,
		Version:      cf.Version,
	}); err != nil {
		return "", err
	}

	// Delete the original output file
	s.DeleteOutputFile(originalPath)

	return content, nil
}

// DiffLocalDoneResult describes what happened after a vimdiff session closed.
type DiffLocalDoneResult struct {
	Outcome string // "none", "full", "partial"
}

// DiffLocalDone compares local, context, and output files after vimdiff closes
// to determine what the user did (no change, full apply, or partial apply).
func (s *State) DiffLocalDone(path string) (*DiffLocalDoneResult, error) {
	if err := s.requireActiveChat(); err != nil {
		return nil, err
	}

	// Read output file — if missing, nothing to compare
	outputContent, err := s.GetOutputFile(path)
	if err != nil {
		return &DiffLocalDoneResult{Outcome: "none"}, nil
	}

	// Read local file from disk
	localPath, err := SafeJoin(s.ProjectRoot, path)
	if err != nil {
		return nil, err
	}
	localData, err := os.ReadFile(localPath)
	if err != nil {
		return &DiffLocalDoneResult{Outcome: "none"}, nil
	}
	localContent := string(localData)

	// Read context file — if not in context (A/!A status), treat as empty
	contextContent := ""
	cf := s.findContextFile(path)
	if cf != nil {
		contextContent, _ = s.GetContextFile(path)
	}

	localNorm := normalizeContent(localContent)
	contextNorm := normalizeContent(contextContent)
	outputNorm := normalizeContent(outputContent)

	localMatchesContext := localNorm == contextNorm
	localMatchesOutput := localNorm == outputNorm

	switch {
	case localMatchesContext:
		// No change — user closed without applying anything
		return &DiffLocalDoneResult{Outcome: "none"}, nil

	case localMatchesOutput:
		// Full apply — delegate to ApplyFile which handles both in-context and not-in-context
		if _, err := s.ApplyFile(path); err != nil {
			return nil, err
		}
		// Re-read local file in case a formatter changed it on save
		if err := s.SyncContextToLocal(path); err != nil {
			return nil, err
		}
		return &DiffLocalDoneResult{Outcome: "full"}, nil

	default:
		// Partial apply — update context to match local, keep output
		if cf != nil {
			// File is in context — update context storage
			var prevVersion string
			if cf.Version != "" {
				prevVersion = cf.Version
			} else {
				prevVersion = HashFileVersion(cf.Path, contextContent)
			}

			storagePath, err := s.contextStoragePath(cf)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(storagePath, []byte(localContent), 0644); err != nil {
				return nil, err
			}
			cf.Version = HashFileVersion(cf.Path, localContent)

			if err := s.addContextEvent(ContextEvent{
				Action:      "UserPartialApplyFile",
				Path:        cf.Path,
				ReadOnly:    cf.ReadOnly,
				External:    cf.External,
				Version:     cf.Version,
				PrevVersion: prevVersion,
			}); err != nil {
				return nil, err
			}
		} else {
			// File not in context (A/!A status) — add to context with local content
			if err := ValidateRelativePath(path); err != nil {
				return nil, err
			}

			contextBase := s.contextDir(s.ActiveChat.ID)
			fullPath, err := SafeJoin(contextBase, path)
			if err != nil {
				return nil, err
			}
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(fullPath, []byte(localContent), 0644); err != nil {
				return nil, err
			}

			s.ActiveChat.ContextFiles = append(s.ActiveChat.ContextFiles, ContextFile{
				Path:     path,
				ReadOnly: false,
				External: false,
				Version:  HashFileVersion(path, localContent),
			})
			cf = s.findContextFile(path)

			if cf == nil {
				return nil, ErrFileNotFound
			}

			if err := s.addContextEvent(ContextEvent{
				Action:   "UserPartialApplyFile",
				Path:     cf.Path,
				ReadOnly: cf.ReadOnly,
				External: cf.External,
				Version:  cf.Version,
				Added:    true,
			}); err != nil {
				return nil, err
			}
		}

		return &DiffLocalDoneResult{Outcome: "partial"}, nil
	}
}

// SyncContextToLocal re-reads the local file and updates context if it differs.
// This catches changes made by formatters that run on save.
func (s *State) SyncContextToLocal(path string) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	cf := s.findContextFile(path)
	if cf == nil {
		return nil // not in context, nothing to sync
	}

	// Read local file from disk
	localPath, err := SafeJoin(s.ProjectRoot, path)
	if err != nil {
		return err
	}
	localData, err := os.ReadFile(localPath)
	if err != nil {
		return nil // file doesn't exist locally, nothing to sync
	}
	localContent := string(localData)

	// Read context content
	contextContent, err := s.GetContextFile(path)
	if err != nil {
		return err
	}

	// Compare normalized; if equal, nothing to do
	if normalizeContent(localContent) == normalizeContent(contextContent) {
		return nil
	}

	// Update context storage with local content
	storagePath, err := s.contextStoragePath(cf)
	if err != nil {
		return err
	}
	if err := os.WriteFile(storagePath, []byte(localContent), 0644); err != nil {
		return err
	}
	cf.Version = HashFileVersion(cf.Path, localContent)

	return nil
}

// normalizeContent normalizes content for comparison (handles line endings).
func normalizeContent(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}
