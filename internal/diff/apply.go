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

// ApplyResult contains the result of applying changes.
type ApplyResult struct {
	Lines       []string // The new file lines
	DroppedNoOp []int    // Indices of changes that were no-ops (content identical to region)
}

// Apply takes the original file lines and a set of changes, resolves anchors,
// validates regions, and applies all changes atomically. Returns the new file
// lines or an error. No changes are applied if any change fails.
// No-op changes (where content is identical to the matched region) are silently
// dropped and reported in ApplyResult.DroppedNoOp.
func Apply(lines []string, changes []Change) (*ApplyResult, error) {
	if len(changes) == 0 {
		return &ApplyResult{Lines: lines}, nil
	}

	// Validate changes
	for i, c := range changes {
		if len(c.Start) == 0 {
			return nil, &ApplyError{ChangeIndex: i, Reason: "empty start anchor", Start: c.Start, End: c.End}
		}
		if len(c.Start) > 10 {
			return nil, &ApplyError{ChangeIndex: i, Reason: fmt.Sprintf("start anchor too long (%d lines, max 10)", len(c.Start)), Start: c.Start, End: c.End}
		}
		if len(c.End) > 10 {
			return nil, &ApplyError{ChangeIndex: i, Reason: fmt.Sprintf("end anchor too long (%d lines, max 10)", len(c.End)), Start: c.Start, End: c.End}
		}
		if c.Content == nil {
			return nil, &ApplyError{ChangeIndex: i, Reason: "content is nil (use empty slice for deletion)", Start: c.Start, End: c.End}
		}
	}

	// Resolve anchors into regions
	regions := make([]Region, len(changes))
	for i, c := range changes {
		startResult, err := findAnchor(lines, c.Start, 0)
		if err != nil {
			return nil, &ApplyError{ChangeIndex: i, Reason: err.Error(), Start: c.Start, End: c.End}
		}

		// If anchor matched with indentation tolerance, adjust content lines
		content := c.Content
		if startResult.indentFix != "" || startResult.indentDel != "" {
			content = make([]string, len(c.Content))
			for j, line := range c.Content {
				content[j] = adjustIndent(line, startResult.indentFix, startResult.indentDel)
			}
		}

		if len(c.End) == 0 {
			// No end anchor: region is just the start lines
			regions[i] = Region{
				StartLine: startResult.pos,
				EndLine:   startResult.pos + len(c.Start) - 1,
				Content:   content,
			}
		} else {
			// Search for end anchor forward from after the start match
			searchFrom := startResult.pos + len(c.Start)
			endResult, err := findAnchor(lines, c.End, searchFrom)
			if err != nil {
				return nil, &ApplyError{ChangeIndex: i, Reason: "end: " + err.Error(), Start: c.Start, End: c.End}
			}
			regions[i] = Region{
				StartLine: startResult.pos,
				EndLine:   endResult.pos + len(c.End) - 1,
				Content:   content,
			}
		}
	}

	// Filter out no-op changes (content identical to matched region)
	var droppedNoOp []int
	var activeIndices []int
	for i, r := range regions {
		regionLines := lines[r.StartLine : r.EndLine+1]
		if linesEqual(regionLines, r.Content) {
			droppedNoOp = append(droppedNoOp, i)
		} else {
			activeIndices = append(activeIndices, i)
		}
	}

	if len(activeIndices) == 0 {
		return &ApplyResult{Lines: lines, DroppedNoOp: droppedNoOp}, nil
	}

	// Sort active regions by StartLine for overlap check
	sorted := make([]int, len(activeIndices))
	copy(sorted, activeIndices)
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
		newResult := make([]string, 0, len(result)-(r.EndLine-r.StartLine+1)+len(r.Content))
		newResult = append(newResult, result[:r.StartLine]...)
		newResult = append(newResult, r.Content...)
		newResult = append(newResult, result[r.EndLine+1:]...)
		result = newResult
	}

	return &ApplyResult{Lines: result, DroppedNoOp: droppedNoOp}, nil
}

// anchorResult holds the result of anchor matching.
type anchorResult struct {
	pos       int    // 0-indexed line position in the file
	indentFix string // indentation adjustment to prepend to content lines ("" if none)
	indentDel string // indentation prefix to strip from content lines ("" if none)
}

// findAnchor searches for a consecutive sequence of anchor lines in the file,
// starting from position `from`. Uses three-pass matching:
// 1. Exact match
// 2. Trailing whitespace trimmed
// 3. All leading+trailing whitespace trimmed (tolerates indentation errors)
//
// When pass 3 matches, the result includes an indentation adjustment so that
// content lines can be shifted to match the file's actual indentation.
func findAnchor(lines []string, anchor []string, from int) (anchorResult, error) {
	if len(anchor) == 0 {
		return anchorResult{}, fmt.Errorf("empty anchor")
	}

	// Pass 1: exact match
	matches := findConsecutive(lines, anchor, from, func(a, b string) bool {
		return a == b
	})
	if len(matches) == 1 {
		return anchorResult{pos: matches[0]}, nil
	}
	if len(matches) > 1 {
		return anchorResult{}, fmt.Errorf("anchor not unique (lines %s)", formatLineNumbers(matches))
	}

	// Pass 2: trailing whitespace trimmed
	matches = findConsecutive(lines, anchor, from, func(a, b string) bool {
		return strings.TrimRight(a, " \t\r") == strings.TrimRight(b, " \t\r")
	})
	if len(matches) == 1 {
		return anchorResult{pos: matches[0]}, nil
	}
	if len(matches) > 1 {
		return anchorResult{}, fmt.Errorf("anchor not unique (lines %s)", formatLineNumbers(matches))
	}

	// Pass 3: all whitespace trimmed (tolerates leading indentation errors)
	matches = findConsecutive(lines, anchor, from, func(a, b string) bool {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	})
	if len(matches) == 1 {
		// Compute indentation delta from first anchor line vs actual file line
		fix, del := computeIndentDelta(lines[matches[0]], anchor[0])
		return anchorResult{pos: matches[0], indentFix: fix, indentDel: del}, nil
	}
	if len(matches) > 1 {
		return anchorResult{}, fmt.Errorf("anchor not unique (lines %s)", formatLineNumbers(matches))
	}

	return anchorResult{}, fmt.Errorf("anchor not found")
}

// computeIndentDelta compares the leading whitespace of a file line and the
// corresponding anchor line. Returns (fix, del) where:
//   - fix = whitespace to prepend to content lines
//   - del = whitespace prefix to strip from content lines
//
// Example: file="        foo", anchor="    foo" → fix="    ", del=""
// (file has 8 spaces, anchor has 4 → content needs 4 more spaces prepended)
//
// Example: file="    foo", anchor="        foo" → fix="", del="    "
// (file has 4 spaces, anchor has 8 → content needs 4 spaces stripped)
func computeIndentDelta(fileLine, anchorLine string) (string, string) {
	fileIndent := leadingWhitespace(fileLine)
	anchorIndent := leadingWhitespace(anchorLine)
	if fileIndent == anchorIndent {
		return "", ""
	}
	// If file has more indentation, we need to add the difference
	if strings.HasPrefix(fileIndent, anchorIndent) {
		return fileIndent[len(anchorIndent):], ""
	}
	// If anchor has more indentation, we need to strip the difference
	if strings.HasPrefix(anchorIndent, fileIndent) {
		return "", anchorIndent[len(fileIndent):]
	}
	// Mixed tabs/spaces or incompatible — just replace entirely
	return fileIndent, anchorIndent
}

// leadingWhitespace returns the leading whitespace prefix of a string.
func leadingWhitespace(s string) string {
	trimmed := strings.TrimLeft(s, " \t")
	return s[:len(s)-len(trimmed)]
}

// adjustIndent shifts a content line's indentation by the given delta.
func adjustIndent(line, fix, del string) string {
	if line == "" {
		return line // don't add indentation to empty lines
	}
	if del != "" && strings.HasPrefix(line, del) {
		line = line[len(del):]
	}
	if fix != "" {
		line = fix + line
	}
	return line
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

// linesEqual checks if two slices of strings are identical.
func linesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
