package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrReadOnly is returned when attempting to write to a read-only file.
var ErrReadOnly = errors.New("file is read-only")

// exitProcess is overridden in tests to avoid terminating the process.
var exitProcess = os.Exit

func fatalProjectEscape(projectRoot, dest string) {
	fmt.Fprintf(os.Stderr, "BB-7 fatal: attempted to write outside project root\nproject_root=%s\ndestination=%s\n", projectRoot, dest)
	exitProcess(1)
}

// WriteOutputFile writes an LLM-generated file to the output directory.
// Returns ErrReadOnly if the file is marked as read-only in context.
// Returns ErrGlobalReadOnly if the active chat is global.
func (s *State) WriteOutputFile(path, content string) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	// Global chats cannot write files
	if s.ActiveChat.Global {
		return ErrGlobalReadOnly
	}

	// Validate path doesn't contain null bytes
	if strings.ContainsRune(path, '\x00') {
		return ErrInvalidPath
	}

	// Check if file is read-only
	if s.IsReadOnly(path) {
		return ErrReadOnly
	}

	// For output files, we only allow relative paths (internal files)
	if filepath.IsAbs(path) {
		// Check if it's actually inside the project - if so, convert to relative
		within, err := IsWithinDir(s.ProjectRoot, path)
		if err != nil {
			return err
		}
		if !within {
			// LLM trying to write outside project - reject
			return ErrPathEscape
		}
		// Convert to relative
		relPath, err := RelativeToBase(s.ProjectRoot, path)
		if err != nil {
			return err
		}
		path = relPath
	}

	// Validate and resolve path
	outputBase := s.outputDir(s.ActiveChat.ID)
	outputPath, err := SafeJoin(outputBase, path)
	if err != nil {
		return err
	}

	// Hard guard: never write outside the project root. If this ever triggers,
	// something is badly wrong; crash and require manual restart.
	withinProject, err := IsWithinDirReal(s.ProjectRoot, outputPath)
	if err != nil {
		return err
	}
	if !withinProject {
		fatalProjectEscape(s.ProjectRoot, outputPath)
		return ErrPathEscape
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(outputPath, []byte(content), 0644)
}

// GetOutputFile returns the content of an output file.
func (s *State) GetOutputFile(path string) (string, error) {
	if err := s.requireActiveChat(); err != nil {
		return "", err
	}

	outputBase := s.outputDir(s.ActiveChat.ID)
	outputPath, err := SafeJoin(outputBase, path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrFileNotFound
		}
		return "", err
	}

	return string(data), nil
}

// GetOutputPath returns the absolute filesystem path to an output file.
// Used by the frontend to open the file in vim's native diff mode.
func (s *State) GetOutputPath(path string) (string, error) {
	if err := s.requireActiveChat(); err != nil {
		return "", err
	}

	outputBase := s.outputDir(s.ActiveChat.ID)
	outputPath, err := SafeJoin(outputBase, path)
	if err != nil {
		return "", err
	}

	// Verify the file exists
	if _, err := os.Stat(outputPath); err != nil {
		if os.IsNotExist(err) {
			return "", ErrFileNotFound
		}
		return "", err
	}

	return outputPath, nil
}

// GetLocalPath returns the absolute filesystem path to a local project file.
func (s *State) GetLocalPath(path string) (string, error) {
	if err := s.requireActiveChat(); err != nil {
		return "", err
	}

	localPath, err := SafeJoin(s.ProjectRoot, path)
	if err != nil {
		return "", err
	}

	return localPath, nil
}

// ListOutputFiles returns all files in the output directory (recursive).
// Returns relative paths from the output directory root.
func (s *State) ListOutputFiles() ([]string, error) {
	if err := s.requireActiveChat(); err != nil {
		return nil, err
	}

	outputBase := s.outputDir(s.ActiveChat.ID)
	var files []string

	err := filepath.WalkDir(outputBase, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // Output directory doesn't exist yet
			}
			return err
		}

		if d.IsDir() {
			return nil
		}

		// Get relative path from output base
		rel, err := filepath.Rel(outputBase, path)
		if err != nil {
			return err
		}

		files = append(files, rel)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// DeleteOutputFile removes an output file.
func (s *State) DeleteOutputFile(path string) error {
	if err := s.requireActiveChat(); err != nil {
		return err
	}

	outputBase := s.outputDir(s.ActiveChat.ID)
	outputPath, err := SafeJoin(outputBase, path)
	if err != nil {
		return err
	}

	if err := os.Remove(outputPath); err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return err
	}

	// Clean up empty parent directories
	s.cleanEmptyDirs(outputBase, filepath.Dir(outputPath))

	return nil
}

// cleanEmptyDirs removes empty directories from path up to (but not including) base.
func (s *State) cleanEmptyDirs(base, path string) {
	for path != base && path != "." && path != "/" {
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(path)
		path = filepath.Dir(path)
	}
}
