package diff

import (
	"strings"
	"testing"
)

func TestApply_SmallEditNoEnd(t *testing.T) {
	lines := SplitLines("package main\n\nfunc hello() {\n\tprintln(\"hello\")\n}\n")
	changes := []Change{{
		Start:   []string{`	println("hello")`},
		Content: []string{`	println("hello world")`},
	}}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := JoinLines(result)
	want := "package main\n\nfunc hello() {\n\tprintln(\"hello world\")\n}\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApply_InsertAfterLine(t *testing.T) {
	lines := SplitLines("import os\n\nfunc main() {}\n")
	changes := []Change{{
		Start:   []string{"import os"},
		Content: []string{"import os", "import sys"},
	}}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := JoinLines(result)
	want := "import os\nimport sys\n\nfunc main() {}\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApply_ReplaceFunctionBody(t *testing.T) {
	lines := SplitLines("def hello():\n    print('old')\n\ndef goodbye():\n    print('bye')\n")
	changes := []Change{{
		Start:   []string{"def hello():"},
		End:     []string{"def goodbye():"},
		Content: []string{"def hello():", "    print('new body')", "", "def goodbye():"},
	}}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := JoinLines(result)
	want := "def hello():\n    print('new body')\n\ndef goodbye():\n    print('bye')\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApply_ChangeSignature(t *testing.T) {
	lines := SplitLines("def hello():\n    print('hi')\n\ndef goodbye():\n    print('bye')\n")
	changes := []Change{{
		Start:   []string{"def hello():"},
		End:     []string{"def goodbye():"},
		Content: []string{"def hello(name):", "    print(name)", "", "def goodbye():"},
	}}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := JoinLines(result)
	want := "def hello(name):\n    print(name)\n\ndef goodbye():\n    print('bye')\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApply_DeleteBlock(t *testing.T) {
	lines := SplitLines("code before\n# BEGIN DEBUG\nprint('debug')\n# END DEBUG\ncode after\n")
	changes := []Change{{
		Start:   []string{"# BEGIN DEBUG"},
		End:     []string{"# END DEBUG"},
		Content: []string{},
	}}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := JoinLines(result)
	want := "code before\ncode after\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApply_MultipleNonOverlapping(t *testing.T) {
	lines := SplitLines("import os\n\ndef hello():\n    pass\n\ndef goodbye():\n    pass\n")
	changes := []Change{
		{
			Start:   []string{"import os"},
			Content: []string{"import os", "import sys"},
		},
		{
			Start:   []string{"def goodbye():"},
			End:     []string{"    pass"},
			Content: []string{"def goodbye():", "    print('bye')"},
		},
	}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := JoinLines(result)
	// "    pass" after goodbye is the second "    pass" in the file
	want := "import os\nimport sys\n\ndef hello():\n    pass\n\ndef goodbye():\n    print('bye')\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApply_BottomToTopOrdering(t *testing.T) {
	// Two changes: first targets an earlier line, second targets a later line.
	// Both should apply correctly regardless of order in the changes slice.
	lines := SplitLines("line1\nline2\nline3\nline4\n")
	changes := []Change{
		{Start: []string{"line1"}, Content: []string{"LINE1"}},
		{Start: []string{"line4"}, Content: []string{"LINE4"}},
	}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := JoinLines(result)
	want := "LINE1\nline2\nline3\nLINE4\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApply_ErrorAnchorNotFound(t *testing.T) {
	lines := SplitLines("line1\nline2\n")
	changes := []Change{{
		Start:   []string{"nonexistent"},
		Content: []string{"replacement"},
	}}
	_, err := Apply(lines, changes)
	if err == nil {
		t.Fatal("expected error")
	}
	applyErr, ok := err.(*ApplyError)
	if !ok {
		t.Fatalf("expected *ApplyError, got %T", err)
	}
	if applyErr.ChangeIndex != 0 {
		t.Errorf("ChangeIndex = %d, want 0", applyErr.ChangeIndex)
	}
	if !strings.Contains(applyErr.Reason, "anchor not found") {
		t.Errorf("Reason = %q, want to contain 'anchor not found'", applyErr.Reason)
	}
}

func TestApply_ErrorAnchorNotUnique(t *testing.T) {
	lines := SplitLines("    pass\ndef mid():\n    pass\n")
	changes := []Change{{
		Start:   []string{"    pass"},
		Content: []string{"    return"},
	}}
	_, err := Apply(lines, changes)
	if err == nil {
		t.Fatal("expected error")
	}
	applyErr, ok := err.(*ApplyError)
	if !ok {
		t.Fatalf("expected *ApplyError, got %T", err)
	}
	if !strings.Contains(applyErr.Reason, "anchor not unique") {
		t.Errorf("Reason = %q, want to contain 'anchor not unique'", applyErr.Reason)
	}
	if !strings.Contains(applyErr.Reason, "1, 3") {
		t.Errorf("Reason = %q, want to contain line numbers '1, 3'", applyErr.Reason)
	}
}

func TestApply_ErrorRegionsOverlap(t *testing.T) {
	lines := SplitLines("line1\nline2\nline3\nline4\n")
	changes := []Change{
		{Start: []string{"line1"}, End: []string{"line3"}, Content: []string{"new"}},
		{Start: []string{"line2"}, Content: []string{"new2"}},
	}
	_, err := Apply(lines, changes)
	if err == nil {
		t.Fatal("expected error")
	}
	applyErr, ok := err.(*ApplyError)
	if !ok {
		t.Fatalf("expected *ApplyError, got %T", err)
	}
	if !strings.Contains(applyErr.Reason, "regions overlap") {
		t.Errorf("Reason = %q, want to contain 'regions overlap'", applyErr.Reason)
	}
}

func TestApply_TrailingWhitespaceFallback(t *testing.T) {
	// File has trailing spaces, anchor does not
	lines := []string{"func hello()  ", "    body", "}"}
	changes := []Change{{
		Start:   []string{"func hello()"},
		Content: []string{"func hello(name string)"},
	}}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0] != "func hello(name string)" {
		t.Errorf("result[0] = %q, want %q", result[0], "func hello(name string)")
	}
}

func TestApply_LeadingWhitespacePreserved(t *testing.T) {
	// Leading whitespace must match exactly — no fuzzy matching
	lines := []string{"  indented", "    more indented"}
	changes := []Change{{
		Start:   []string{"indented"}, // missing leading spaces
		Content: []string{"changed"},
	}}
	_, err := Apply(lines, changes)
	if err == nil {
		t.Fatal("expected error for mismatched leading whitespace")
	}
	if !strings.Contains(err.Error(), "anchor not found") {
		t.Errorf("error = %q, want 'anchor not found'", err.Error())
	}
}

func TestApply_MultiLineAnchor(t *testing.T) {
	lines := SplitLines("if a {\n    foo()\n}\nif a {\n    bar()\n}\n")
	// "if a {" appears twice, but with the second anchor line it becomes unique
	changes := []Change{{
		Start:   []string{"if a {", "    bar()"},
		Content: []string{"if a {", "    baz()"},
	}}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := JoinLines(result)
	want := "if a {\n    foo()\n}\nif a {\n    baz()\n}\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApply_EndAnchorForwardOnly(t *testing.T) {
	// End anchor exists before start, but should only search forward
	lines := SplitLines("marker\nstuff\nstart_here\nmore stuff\nmarker\n")
	changes := []Change{{
		Start:   []string{"start_here"},
		End:     []string{"marker"},
		Content: []string{"start_here", "replaced", "marker"},
	}}
	result, err := Apply(lines, changes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := JoinLines(result)
	want := "marker\nstuff\nstart_here\nreplaced\nmarker\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestApply_ValidationEmptyStart(t *testing.T) {
	lines := []string{"line1"}
	changes := []Change{{
		Start:   []string{},
		Content: []string{"x"},
	}}
	_, err := Apply(lines, changes)
	if err == nil {
		t.Fatal("expected error for empty start")
	}
	if !strings.Contains(err.Error(), "empty start anchor") {
		t.Errorf("error = %q, want 'empty start anchor'", err.Error())
	}
}

func TestApply_ValidationStartTooLong(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	changes := []Change{{
		Start:   []string{"a", "b", "c", "d", "e"},
		Content: []string{"x"},
	}}
	_, err := Apply(lines, changes)
	if err == nil {
		t.Fatal("expected error for start too long")
	}
	if !strings.Contains(err.Error(), "start anchor too long") {
		t.Errorf("error = %q, want 'start anchor too long'", err.Error())
	}
}

func TestApply_ValidationEndTooLong(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e", "f"}
	changes := []Change{{
		Start:   []string{"a"},
		End:     []string{"b", "c", "d", "e", "f"},
		Content: []string{"x"},
	}}
	_, err := Apply(lines, changes)
	if err == nil {
		t.Fatal("expected error for end too long")
	}
	if !strings.Contains(err.Error(), "end anchor too long") {
		t.Errorf("error = %q, want 'end anchor too long'", err.Error())
	}
}

func TestApply_ValidationNilContent(t *testing.T) {
	lines := []string{"line1"}
	changes := []Change{{
		Start:   []string{"line1"},
		Content: nil,
	}}
	_, err := Apply(lines, changes)
	if err == nil {
		t.Fatal("expected error for nil content")
	}
	if !strings.Contains(err.Error(), "content is nil") {
		t.Errorf("error = %q, want 'content is nil'", err.Error())
	}
}

func TestApply_NoChanges(t *testing.T) {
	lines := []string{"a", "b"}
	result, err := Apply(lines, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 || result[0] != "a" || result[1] != "b" {
		t.Errorf("result = %v, want [a b]", result)
	}
}

func TestSplitLines_Roundtrip(t *testing.T) {
	content := "line1\nline2\nline3\n"
	lines := SplitLines(content)
	got := JoinLines(lines)
	if got != content {
		t.Errorf("roundtrip failed: got %q, want %q", got, content)
	}
}

func TestSplitLines_Empty(t *testing.T) {
	lines := SplitLines("")
	if lines != nil {
		t.Errorf("SplitLines(\"\") = %v, want nil", lines)
	}
}

func TestSplitLines_CRLF(t *testing.T) {
	lines := SplitLines("a\r\nb\r\nc\r\n")
	if len(lines) != 3 {
		t.Fatalf("len = %d, want 3", len(lines))
	}
	for _, l := range lines {
		if strings.Contains(l, "\r") {
			t.Errorf("line contains \\r: %q", l)
		}
	}
}

func TestSplitLines_NoTrailingNewline(t *testing.T) {
	lines := SplitLines("a\nb")
	if len(lines) != 2 {
		t.Fatalf("len = %d, want 2", len(lines))
	}
	if lines[0] != "a" || lines[1] != "b" {
		t.Errorf("lines = %v, want [a b]", lines)
	}
}

func TestJoinLines_Empty(t *testing.T) {
	got := JoinLines(nil)
	if got != "" {
		t.Errorf("JoinLines(nil) = %q, want \"\"", got)
	}
}

func TestApply_EndAnchorNotFound(t *testing.T) {
	lines := SplitLines("start\nmiddle\nend\n")
	changes := []Change{{
		Start:   []string{"start"},
		End:     []string{"nonexistent"},
		Content: []string{"new"},
	}}
	_, err := Apply(lines, changes)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "end: anchor not found") {
		t.Errorf("error = %q, want to contain 'end: anchor not found'", err.Error())
	}
}
