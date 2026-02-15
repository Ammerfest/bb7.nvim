package diff

import (
	"fmt"
	"sort"
	"strings"
)

// Change represents a single region change from the LLM's modify_file tool call.
type Change struct {
	Start   []string // 1-4 anchor lines marking the beginning of the region
	End     []string // 0-4 anchor lines marking the end of the region (optional)
	Content []string // exact replacement lines for the matched region
}

// Region is a resolved change: line positions in the original file.
type Region struct {
	StartLine int      // 0-indexed first line of the region
	EndLine   int      // 0-indexed last line of the region (inclusive)
	Content   []string // replacement lines
}

// ApplyError provides structured error details for a failed change.
type ApplyError struct {
	ChangeIndex int
	Reason      string
	Start       []string
	End         []string
}

func (e *ApplyError) Error() string {
	msg := fmt.Sprintf("change %d: %s", e.ChangeIndex, e.Reason)
	if len(e.Start) > 0 {
		msg += fmt.Sprintf(" (start: %q", e.Start[0])
		if len(e.Start) > 1 {
			msg += " ..."
		}
		msg += ")"
	}
	return msg
}

// Apply takes the original file lines and a set of changes, resolves anchors,
// validates regions, and applies all changes atomically. Returns the new file
// lines or an error. No changes are applied if any change fails.
func Apply(lines []string, changes []Change) ([]string, error) {
	if len(changes) == 0 {
		return lines, nil
	}

	// Validate changes
	for i, c := range changes {
		if len(c.Start) == 0 {
			return nil, &ApplyError{ChangeIndex: i, Reason: "empty start anchor", Start: c.Start, End: c.End}
		}
		if len(c.Start) > 4 {
			return nil, &ApplyError{ChangeIndex: i, Reason: fmt.Sprintf("start anchor too long (%d lines, max 4)", len(c.Start)), Start: c.Start, End: c.End}
		}
		if len(c.End) > 4 {
			return nil, &ApplyError{ChangeIndex: i, Reason: fmt.Sprintf("end anchor too long (%d lines, max 4)", len(c.End)), Start: c.Start, End: c.End}
		}
		if c.Content == nil {
			return nil, &ApplyError{ChangeIndex: i, Reason: "content is nil (use empty slice for deletion)", Start: c.Start, End: c.End}
		}
	}

	// Resolve anchors into regions
	regions := make([]Region, len(changes))
	for i, c := range changes {
		startPos, err := findAnchor(lines, c.Start, 0)
		if err != nil {
			return nil, &ApplyError{ChangeIndex: i, Reason: err.Error(), Start: c.Start, End: c.End}
		}

		if len(c.End) == 0 {
			// No end anchor: region is just the start lines
			regions[i] = Region{
				StartLine: startPos,
				EndLine:   startPos + len(c.Start) - 1,
				Content:   c.Content,
			}
		} else {
			// Search for end anchor forward from after the start match
			searchFrom := startPos + len(c.Start)
			endPos, err := findAnchor(lines, c.End, searchFrom)
			if err != nil {
				return nil, &ApplyError{ChangeIndex: i, Reason: "end: " + err.Error(), Start: c.Start, End: c.End}
			}
			regions[i] = Region{
				StartLine: startPos,
				EndLine:   endPos + len(c.End) - 1,
				Content:   c.Content,
			}
		}
	}

	// Sort regions by StartLine for overlap check
	sorted := make([]int, len(regions))
	for i := range sorted {
		sorted[i] = i
	}
	sort.Slice(sorted, func(a, b int) bool {
		return regions[sorted[a]].StartLine < regions[sorted[b]].StartLine
	})

	// Check for overlaps
	for i := 1; i < len(sorted); i++ {
		prev := regions[sorted[i-1]]
		curr := regions[sorted[i]]
		if curr.StartLine <= prev.EndLine {
			return nil, &ApplyError{
				ChangeIndex: sorted[i],
				Reason:      fmt.Sprintf("regions overlap (lines %d-%d and %d-%d)", prev.StartLine+1, prev.EndLine+1, curr.StartLine+1, curr.EndLine+1),
				Start:       changes[sorted[i]].Start,
				End:         changes[sorted[i]].End,
			}
		}
	}

	// Apply bottom-to-top so line numbers don't shift
	sort.Slice(sorted, func(a, b int) bool {
		return regions[sorted[a]].StartLine > regions[sorted[b]].StartLine
	})

	result := make([]string, len(lines))
	copy(result, lines)

	for _, idx := range sorted {
		r := regions[idx]
		// Splice: remove lines[StartLine..EndLine] and insert Content
		newResult := make([]string, 0, len(result)-( r.EndLine-r.StartLine+1)+len(r.Content))
		newResult = append(newResult, result[:r.StartLine]...)
		newResult = append(newResult, r.Content...)
		newResult = append(newResult, result[r.EndLine+1:]...)
		result = newResult
	}

	return result, nil
}

// findAnchor searches for a consecutive sequence of anchor lines in the file,
// starting from position `from`. Uses two-pass matching: exact first, then
// trailing whitespace trimmed.
func findAnchor(lines []string, anchor []string, from int) (int, error) {
	if len(anchor) == 0 {
		return 0, fmt.Errorf("empty anchor")
	}

	// Pass 1: exact match
	matches := findConsecutive(lines, anchor, from, func(a, b string) bool {
		return a == b
	})
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("anchor not unique (lines %s)", formatLineNumbers(matches))
	}

	// Pass 2: trailing whitespace trimmed
	matches = findConsecutive(lines, anchor, from, func(a, b string) bool {
		return strings.TrimRight(a, " \t\r") == strings.TrimRight(b, " \t\r")
	})
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("anchor not unique (lines %s)", formatLineNumbers(matches))
	}

	return 0, fmt.Errorf("anchor not found")
}

// findConsecutive finds all positions where anchor lines match consecutively
// in the file starting from `from`, using the given comparison function.
func findConsecutive(lines []string, anchor []string, from int, eq func(string, string) bool) []int {
	var matches []int
	limit := len(lines) - len(anchor) + 1
	for i := from; i < limit; i++ {
		found := true
		for j, a := range anchor {
			if !eq(lines[i+j], a) {
				found = false
				break
			}
		}
		if found {
			matches = append(matches, i)
		}
	}
	return matches
}

func formatLineNumbers(positions []int) string {
	parts := make([]string, len(positions))
	for i, p := range positions {
		parts[i] = fmt.Sprintf("%d", p+1) // 1-indexed for user display
	}
	return strings.Join(parts, ", ")
}

// SplitLines splits file content into lines. Handles both LF and CRLF.
// A trailing newline does not produce an extra empty line.
func SplitLines(content string) []string {
	if content == "" {
		return nil
	}
	// Normalize CRLF to LF
	content = strings.ReplaceAll(content, "\r\n", "\n")
	// Remove trailing newline to avoid ghost empty line
	content = strings.TrimSuffix(content, "\n")
	return strings.Split(content, "\n")
}

// JoinLines joins lines back into file content with a trailing newline.
func JoinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}
