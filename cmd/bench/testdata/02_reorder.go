package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FormatOutput converts a result into a human-readable string.
func FormatOutput(name string, values []int, verbose bool) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== %s ===\n", name))

	for i, v := range values {
		if verbose {
			sb.WriteString(fmt.Sprintf("  [%d] value: %d (hex: 0x%x)\n", i, v, v))
		} else {
			sb.WriteString(fmt.Sprintf("  %d\n", v))
		}
	}

	sb.WriteString(fmt.Sprintf("Total entries: %d\n", len(values)))
	return sb.String()
}

// ProcessBatch handles a batch of items, applying the transform function
// to each item and collecting the results.
func ProcessBatch(items []string, batchSize int, transform func(string) string) []string {
	results := make([]string, 0, len(items))

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		for _, item := range batch {
			transformed := transform(item)
			if transformed != "" {
				results = append(results, transformed)
			}
		}

		fmt.Printf("processed batch %d-%d of %d\n", i, end-1, len(items))
	}

	return results
}

// ParseConfig reads a configuration file and returns key-value pairs.
func ParseConfig(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	config := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("line %d: invalid format (expected key=value)", lineNum+1)
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		config[key] = value
	}

	return config, nil
}

// ValidateInput checks that the provided input meets all requirements.
func ValidateInput(input string, minLen, maxLen int, allowedChars string) error {
	if len(input) < minLen {
		return fmt.Errorf("input too short: %d < %d", len(input), minLen)
	}
	if len(input) > maxLen {
		return fmt.Errorf("input too long: %d > %d", len(input), maxLen)
	}

	if allowedChars != "" {
		for i, ch := range input {
			if !strings.ContainsRune(allowedChars, ch) {
				return fmt.Errorf("invalid character %q at position %d", ch, i)
			}
		}
	}

	return nil
}

// CleanupTemp removes temporary files older than the given duration.
func CleanupTemp(dir string, maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	removed := 0
	cutoff := time.Now().Add(-maxAge)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			fullPath := filepath.Join(dir, entry.Name())
			if err := os.Remove(fullPath); err != nil {
				return removed, fmt.Errorf("removing %s: %w", fullPath, err)
			}
			removed++
			fmt.Printf("removed old temp file: %s\n", entry.Name())
		}
	}

	return removed, nil
}
