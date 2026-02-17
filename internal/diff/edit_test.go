package diff

import (
	"strings"
	"testing"
)

func TestReplace_Basic(t *testing.T) {
	content := "apple\nbanana\ncherry\n"
	got, err := Replace(content, "banana", "blueberry", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "apple\nblueberry\ncherry\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplace_MultiLine(t *testing.T) {
	content := "apple\nbanana\ncherry\ndate\n"
	got, err := Replace(content, "banana\ncherry", "blueberry\ncranberry", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "apple\nblueberry\ncranberry\ndate\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplace_Insert(t *testing.T) {
	content := "import os\n\nfunc main() {}\n"
	got, err := Replace(content, "import os", "import os\nimport sys", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "import os\nimport sys\n\nfunc main() {}\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplace_Deletion(t *testing.T) {
	content := "apple\nbanana\ncherry\n"
	got, err := Replace(content, "banana\n", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "apple\ncherry\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplace_ReplaceAll(t *testing.T) {
	content := "foo\nbar\nfoo\nbaz\nfoo\n"
	got, err := Replace(content, "foo", "qux", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "qux\nbar\nqux\nbaz\nqux\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplace_ReplaceAllNotFound(t *testing.T) {
	content := "apple\nbanana\n"
	_, err := Replace(content, "orange", "tangerine", true)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

func TestReplace_NotFound(t *testing.T) {
	content := "apple\nbanana\n"
	_, err := Replace(content, "orange", "tangerine", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want to contain 'not found'", err.Error())
	}
}

func TestReplace_NotUnique(t *testing.T) {
	content := "foo\nbar\nfoo\nbaz\n"
	_, err := Replace(content, "foo", "qux", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not unique") {
		t.Errorf("error = %q, want to contain 'not unique'", err.Error())
	}
}

func TestReplace_NoOp(t *testing.T) {
	content := "apple\nbanana\n"
	_, err := Replace(content, "banana", "banana", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no-op") {
		t.Errorf("error = %q, want to contain 'no-op'", err.Error())
	}
}

func TestReplace_TrailingWhitespace(t *testing.T) {
	// File has trailing spaces, old_string does not
	content := "func hello()  \n    body\n}\n"
	got, err := Replace(content, "func hello()", "func hello(name string)", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "func hello(name string)\n    body\n}\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplace_LeadingWhitespace(t *testing.T) {
	// File has 8 spaces, old_string has 4 — pass 3 should match and adjust new_string
	content := "        if condition:\n            do_something()\n        else:\n"
	got, err := Replace(content,
		"    if condition:\n        do_something()",
		"    if condition:\n        do_new_thing()",
		false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "        if condition:\n            do_new_thing()\n        else:\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplace_LeadingWhitespaceEmptyLines(t *testing.T) {
	content := "        code_here\n        more_code\n"
	got, err := Replace(content,
		"    code_here",
		"    new_code\n\n    more_new_code",
		false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := SplitLines(got)
	if lines[0] != "        new_code" {
		t.Errorf("lines[0] = %q, want %q", lines[0], "        new_code")
	}
	if lines[1] != "" {
		t.Errorf("lines[1] = %q, want empty string", lines[1])
	}
	if lines[2] != "        more_new_code" {
		t.Errorf("lines[2] = %q, want %q", lines[2], "        more_new_code")
	}
}

func TestReplace_EmptyOldString(t *testing.T) {
	_, err := Replace("content\n", "", "new", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error = %q, want to contain 'empty'", err.Error())
	}
}

func TestReplace_BoundaryPrefix(t *testing.T) {
	// old_string's last line "// ParseConfig" is a truncated prefix of the actual
	// file line "// ParseConfig reads a configuration file..."
	// Pass 4 (boundary prefix) should match and expand the truncated line.
	content := "// ProcessBatch does batch work.\nfunc ProcessBatch() {\n}\n\n// ParseConfig reads a configuration file and returns the parsed result.\nfunc ParseConfig() {}\n"
	got, err := Replace(content,
		"// ProcessBatch does batch work.\nfunc ProcessBatch() {\n}\n\n// ParseConfig",
		"// ParseConfig",
		false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The truncated "// ParseConfig" in new_string should be expanded to the full line
	want := "// ParseConfig reads a configuration file and returns the parsed result.\nfunc ParseConfig() {}\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestReplace_BoundaryPrefixFirstLine(t *testing.T) {
	// old_string's first line is a truncated prefix of the file line
	content := "// Helper function that does many things and is very useful.\nfunc Helper() {\n\treturn\n}\n"
	got, err := Replace(content,
		"// Helper function\nfunc Helper() {\n\treturn\n}",
		"// Helper function\nfunc Helper() {\n\tlog.Println(\"called\")\n\treturn\n}",
		false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First line should be expanded to file's full line
	if !strings.Contains(got, "// Helper function that does many things and is very useful.") {
		t.Errorf("expected full first line to be preserved, got:\n%s", got)
	}
	if !strings.Contains(got, "log.Println") {
		t.Errorf("expected replacement body to be applied, got:\n%s", got)
	}
}

func TestReplace_SubstringNotUnique(t *testing.T) {
	content := "foo bar\nbaz\nfoo bar\n"
	_, err := Replace(content, "foo", "qux", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not unique") {
		t.Errorf("error = %q, want to contain 'not unique'", err.Error())
	}
}

func TestReplace_IndentSkipWhenNewStringDiffers(t *testing.T) {
	// File has 2 tabs, old_string has 1 tab (wrong), new_string has 2 tabs (correct).
	// Pass 3 matches, but adjustment should be skipped because old and new have different indent.
	content := "\t\tif item.Status == StatusPending {\n\t\t\ttimeout := 30 * time.Second\n\t\t}\n"
	got, err := Replace(content,
		"\tif item.Status == StatusPending {\n\t\ttimeout := 30 * time.Second\n\t}",
		"\t\tif item.Status == StatusPending {\n\t\t\ttimeout := 60 * time.Second\n\t\t}",
		false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "\t\tif item.Status == StatusPending {\n\t\t\ttimeout := 60 * time.Second\n\t\t}\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestReplace_PerLineIndentDelta(t *testing.T) {
	// File has 3 tabs on "if", 4 tabs on "timeout".
	// old_string has 2 tabs on "if" (wrong), 4 tabs on "timeout" (correct).
	// Per-line delta: line 0 needs +1 tab, line 1 needs 0.
	// new_string should only get +1 tab on line 0, not on line 1.
	content := "\t\t\tif item.Status == StatusPending {\n\t\t\t\ttimeout := 30 * time.Second\n\t\t\t}\n"
	got, err := Replace(content,
		"\t\tif item.Status == StatusPending {\n\t\t\t\ttimeout := 30 * time.Second",
		"\t\tif item.Status == StatusPending {\n\t\t\t\ttimeout := 60 * time.Second",
		false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "\t\t\tif item.Status == StatusPending {\n\t\t\t\ttimeout := 60 * time.Second\n\t\t\t}\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestReplace_EmptyNewString(t *testing.T) {
	// Replacing with empty string removes the matched lines entirely
	content := "apple\nbanana\ncherry\n"
	got, err := Replace(content, "banana", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "apple\ncherry\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
