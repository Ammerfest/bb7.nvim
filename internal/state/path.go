package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Path validation errors.
var (
	ErrPathEscape   = errors.New("path escapes base directory")
	ErrAbsolutePath = errors.New("absolute paths not allowed for internal files")
	ErrInvalidPath  = errors.New("invalid path")
)

// SafeJoin joins a base directory with a relative path, ensuring the result
// stays within the base directory. Uses OS-level path resolution.
// Returns the absolute path if valid, or an error if the path escapes.
func SafeJoin(baseDir, relativePath string) (string, error) {
	if relativePath == "" {
		return "", ErrInvalidPath
	}

	// Join and clean (this resolves . and .. components)
	joined := filepath.Join(baseDir, relativePath)

	// Resolve to absolute paths for comparison
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}

	// Use Rel to check containment - if result starts with ".." it's a traversal
	rel, err := filepath.Rel(absBase, absJoined)
	if err != nil {
		return "", err
	}

	// Reject if path escapes: exactly ".." or starts with "../"
	// Note: "..." or "..foo" are valid filenames, not traversals
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrPathEscape
	}

	return absJoined, nil
}

// IsWithinDir checks if targetPath is within baseDir.
// Both paths are resolved to absolute before comparison.
func IsWithinDir(baseDir, targetPath string) (bool, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return false, err
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false, err
	}

	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false, err
	}

	// Reject if exactly ".." or starts with "../"
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

// resolvePathForContainment resolves symlinks for containment checks.
// For non-existent paths, it resolves the nearest existing ancestor and
// re-attaches the missing path suffix.
func resolvePathForContainment(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	current := absPath
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}

		if !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}

		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

// IsWithinDirReal checks whether targetPath resolves inside baseDir after
// following symlinks. This is stricter than IsWithinDir and should be used
// for write guards.
func IsWithinDirReal(baseDir, targetPath string) (bool, error) {
	baseResolved, err := resolvePathForContainment(baseDir)
	if err != nil {
		return false, err
	}
	targetResolved, err := resolvePathForContainment(targetPath)
	if err != nil {
		return false, err
	}

	rel, err := filepath.Rel(baseResolved, targetResolved)
	if err != nil {
		return false, err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

// ValidateRelativePath checks that a path is valid for use as a relative path.
// It must not be absolute and must not contain null bytes.
func ValidateRelativePath(path string) error {
	if path == "" {
		return ErrInvalidPath
	}

	if filepath.IsAbs(path) {
		return ErrAbsolutePath
	}

	if strings.ContainsRune(path, '\x00') {
		return ErrInvalidPath
	}

	return nil
}

// RelativeToBase returns the relative path from baseDir to targetPath.
// Returns error if targetPath is not within baseDir.
func RelativeToBase(baseDir, targetPath string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return "", err
	}

	// Reject if exactly ".." or starts with "../"
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrPathEscape
	}

	return rel, nil
}
