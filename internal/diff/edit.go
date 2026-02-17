package diff

import (
	"fmt"
	"strings"
)

// Replace performs a search/replace operation on file content.
// It finds oldString in content using multi-pass matching and replaces it
// with newString.
//
// Matching passes (tried in order):
//  1. Exact line-by-line match
//  2. Trailing whitespace trimmed
//  3. All whitespace trimmed (with indentation adjustment)
//  4. Boundary prefix match (first/last lines can be truncated prefixes)
//  5. Raw substring match
//
// If replaceAll is false, oldString must match exactly once (error if 0 or >1).
// If replaceAll is true, oldString must match at least once.
// Returns the new file content or an error.
func Replace(content, oldString, newString string, replaceAll bool) (string, error) {
	if oldString == newString {
		return "", fmt.Errorf("old_string and new_string are identical (no-op)")
	}

	lines := SplitLines(content)
	oldLines := SplitLines(oldString)

	if len(oldLines) == 0 {
		return "", fmt.Errorf("old_string is empty")
	}

	// 3-pass line-based matching, same as findAnchor
	type matchResult struct {
		positions []int
		pass      int
	}

	var result matchResult

	// Pass 1: exact match
	matches := findConsecutive(lines, oldLines, 0, func(a, b string) bool {
		return a == b
	})
	if len(matches) > 0 {
		result = matchResult{positions: matches, pass: 1}
	}

	// Pass 2: trailing whitespace trimmed
	if len(result.positions) == 0 {
		matches = findConsecutive(lines, oldLines, 0, func(a, b string) bool {
			return strings.TrimRight(a, " \t\r") == strings.TrimRight(b, " \t\r")
		})
		if len(matches) > 0 {
			result = matchResult{positions: matches, pass: 2}
		}
	}

	// Pass 3: all whitespace trimmed
	if len(result.positions) == 0 {
		matches = findConsecutive(lines, oldLines, 0, func(a, b string) bool {
			return strings.TrimSpace(a) == strings.TrimSpace(b)
		})
		if len(matches) > 0 {
			result = matchResult{positions: matches, pass: 3}
		}
	}

	// Pass 4: boundary prefix match (first/last lines of old_string can be
	// truncated prefixes of the actual file lines, e.g. "// ParseConfig" matching
	// "// ParseConfig reads a configuration file...")
	if len(result.positions) == 0 && len(oldLines) >= 2 {
		matches = findBoundaryPrefix(lines, oldLines, 0)
		if len(matches) > 0 {
			result = matchResult{positions: matches, pass: 4}
		}
	}

	// Pass 5: raw substring match (last resort)
	if len(result.positions) == 0 {
		return replaceSubstring(content, oldString, newString, replaceAll)
	}

	if !replaceAll && len(result.positions) > 1 {
		return "", fmt.Errorf("old_string not unique (%d matches at lines %s)",
			len(result.positions), formatLineNumbers(result.positions))
	}

	newLines := SplitLines(newString)

	// Adjust new_string lines based on which matching pass was used.
	adjustNewLines := func(pos int) []string {
		// Pass 4: boundary prefix match — expand truncated boundary lines
		// in new_string to the file's full lines.
		if result.pass == 4 {
			return expandBoundaryLines(lines, pos, oldLines, newLines)
		}
		if result.pass < 3 {
			return newLines
		}
		// Pass 3: per-line indentation adjustment.
		// Compute the delta between each old_string line and the file line,
		// and only adjust the corresponding new_string line when there's an
		// actual indentation mismatch.
		firstFix, firstDel := computeIndentDelta(lines[pos], oldLines[0])
		if firstFix == "" && firstDel == "" {
			return newLines
		}
		// If old_string and new_string have different leading whitespace on
		// their first non-empty lines, the model likely got new_string's
		// indentation right. Skip adjustment to avoid over-correcting.
		oldIndent := leadingWhitespace(oldLines[0])
		newIndent := firstNonEmptyIndent(newLines)
		if oldIndent != newIndent {
			return newLines
		}
		adjusted := make([]string, len(newLines))
		for i, line := range newLines {
			if i < len(oldLines) {
				fix, del := computeIndentDelta(lines[pos+i], oldLines[i])
				if fix != "" || del != "" {
					adjusted[i] = adjustIndent(line, fix, del)
				} else {
					adjusted[i] = line
				}
			} else {
				// Extra lines in new_string — use first line's delta
				adjusted[i] = adjustIndent(line, firstFix, firstDel)
			}
		}
		return adjusted
	}

	// Replace bottom-to-top to preserve line numbers
	positionsToReplace := result.positions
	if !replaceAll {
		positionsToReplace = result.positions[:1]
	}

	// Sort positions in reverse order for bottom-to-top application
	for i, j := 0, len(positionsToReplace)-1; i < j; i, j = i+1, j-1 {
		positionsToReplace[i], positionsToReplace[j] = positionsToReplace[j], positionsToReplace[i]
	}

	for _, pos := range positionsToReplace {
		replacement := adjustNewLines(pos)
		// Splice: remove old lines and insert new lines
		newResult := make([]string, 0, len(lines)-len(oldLines)+len(replacement))
		newResult = append(newResult, lines[:pos]...)
		newResult = append(newResult, replacement...)
		newResult = append(newResult, lines[pos+len(oldLines):]...)
		lines = newResult
	}

	return JoinLines(lines), nil
}

// findBoundaryPrefix finds positions where oldLines match consecutively in
// lines, allowing the first and last lines to be prefix matches (after
// trimming whitespace). Interior lines must match via TrimSpace equality.
// Both boundary lines must have at least 8 non-whitespace characters.
// Only applies to multi-line old_string (>= 2 lines).
func findBoundaryPrefix(lines, oldLines []string, from int) []int {
	if len(oldLines) < 2 {
		return nil
	}
	const minLen = 8

	var matches []int
	limit := len(lines) - len(oldLines) + 1
	for i := from; i < limit; i++ {
		fileFirst := strings.TrimSpace(lines[i])
		oldFirst := strings.TrimSpace(oldLines[0])

		// First line: exact TrimSpace match or prefix match (with min length)
		firstExact := fileFirst == oldFirst
		firstPrefix := !firstExact && len(oldFirst) >= minLen && strings.HasPrefix(fileFirst, oldFirst)
		if !firstExact && !firstPrefix {
			continue
		}

		lastIdx := i + len(oldLines) - 1
		fileLast := strings.TrimSpace(lines[lastIdx])
		oldLast := strings.TrimSpace(oldLines[len(oldLines)-1])

		// Last line: exact TrimSpace match or prefix match (with min length)
		lastExact := fileLast == oldLast
		lastPrefix := !lastExact && len(oldLast) >= minLen && strings.HasPrefix(fileLast, oldLast)
		if !lastExact && !lastPrefix {
			continue
		}

		// At least one boundary must be a prefix (not exact) match,
		// otherwise pass 3 would have already matched.
		if !firstPrefix && !lastPrefix {
			continue
		}

		// Interior lines: TrimSpace equality
		ok := true
		for j := 1; j < len(oldLines)-1; j++ {
			if strings.TrimSpace(lines[i+j]) != strings.TrimSpace(oldLines[j]) {
				ok = false
				break
			}
		}
		if ok {
			matches = append(matches, i)
		}
	}
	return matches
}

// expandBoundaryLines adjusts new_string lines for pass 4 (boundary prefix)
// matches. When old_string's first/last line was a truncated prefix of the
// file line, and new_string kept the same truncated text at that position,
// expand it to the file's full line.
func expandBoundaryLines(lines []string, pos int, oldLines, newLines []string) []string {
	adjusted := make([]string, len(newLines))
	copy(adjusted, newLines)

	// Expand first line if it was prefix-matched and new_string kept it
	if len(adjusted) > 0 {
		fileFirst := lines[pos]
		oldFirst := strings.TrimSpace(oldLines[0])
		newFirst := strings.TrimSpace(adjusted[0])
		if newFirst == oldFirst &&
			strings.HasPrefix(strings.TrimSpace(fileFirst), oldFirst) &&
			strings.TrimSpace(fileFirst) != oldFirst {
			adjusted[0] = fileFirst
		}
	}

	// Expand last line if it was prefix-matched and new_string kept it
	if len(adjusted) > 0 {
		fileLast := lines[pos+len(oldLines)-1]
		oldLast := strings.TrimSpace(oldLines[len(oldLines)-1])
		newLast := strings.TrimSpace(adjusted[len(adjusted)-1])
		if newLast == oldLast &&
			strings.HasPrefix(strings.TrimSpace(fileLast), oldLast) &&
			strings.TrimSpace(fileLast) != oldLast {
			adjusted[len(adjusted)-1] = fileLast
		}
	}

	return adjusted
}

// replaceSubstring does a raw substring match/replace on the content.
// Used as a last-resort fallback when all line-based passes fail.
func replaceSubstring(content, oldString, newString string, replaceAll bool) (string, error) {
	// Normalize CRLF for consistent matching
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalizedOld := strings.ReplaceAll(oldString, "\r\n", "\n")
	normalizedNew := strings.ReplaceAll(newString, "\r\n", "\n")

	idx := strings.Index(normalized, normalizedOld)
	if idx < 0 {
		return "", fmt.Errorf("old_string not found in file")
	}

	if replaceAll {
		count := strings.Count(normalized, normalizedOld)
		if count == 0 {
			return "", fmt.Errorf("old_string not found in file")
		}
		return strings.ReplaceAll(normalized, normalizedOld, normalizedNew), nil
	}

	// Check uniqueness
	lastIdx := strings.LastIndex(normalized, normalizedOld)
	if idx != lastIdx {
		return "", fmt.Errorf("old_string not unique (multiple substring matches)")
	}

	return normalized[:idx] + normalizedNew + normalized[idx+len(normalizedOld):], nil
}

// firstNonEmptyIndent returns the leading whitespace of the first non-empty
// line in the slice, or "" if all lines are empty.
func firstNonEmptyIndent(lines []string) string {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return leadingWhitespace(line)
		}
	}
	return ""
}
