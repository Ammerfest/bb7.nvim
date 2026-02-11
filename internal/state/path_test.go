package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	base := "/project"

	tests := []struct {
		name    string
		rel     string
		wantErr error
	}{
		{"simple file", "file.go", nil},
		{"nested path", "src/main.go", nil},
		{"deep nesting", "a/b/c/d/e.go", nil},
		{"parent escape", "../etc/passwd", ErrPathEscape},
		{"hidden parent escape", "src/../../etc/passwd", ErrPathEscape},
		{"dot path", "./file.go", nil},
		{"double dot in name", "file..go", nil},
		{"empty path", "", ErrInvalidPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeJoin(base, tt.rel)
			if err != tt.wantErr {
				t.Errorf("SafeJoin(%q, %q) error = %v, want %v", base, tt.rel, err, tt.wantErr)
			}
		})
	}
}

func TestSafeJoin_ReturnsCorrectPath(t *testing.T) {
	base := "/project"
	rel := "src/main.go"

	result, err := SafeJoin(base, rel)
	if err != nil {
		t.Fatalf("SafeJoin failed: %v", err)
	}

	expected := filepath.Join(base, rel)
	absExpected, _ := filepath.Abs(expected)
	if result != absExpected {
		t.Errorf("SafeJoin returned %q, want %q", result, absExpected)
	}
}

func TestIsWithinDir(t *testing.T) {
	base := "/project"

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"inside", "/project/src/file.go", true},
		{"same dir", "/project", true},
		{"outside", "/etc/passwd", false},
		{"sibling", "/project2/file.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsWithinDir(base, tt.target)
			if err != nil {
				t.Fatalf("IsWithinDir error: %v", err)
			}
			if got != tt.want {
				t.Errorf("IsWithinDir(%q, %q) = %v, want %v", base, tt.target, got, tt.want)
			}
		})
	}
}

func TestValidateRelativePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{"simple", "file.go", nil},
		{"nested", "src/main.go", nil},
		{"absolute", "/etc/passwd", ErrAbsolutePath},
		{"empty", "", ErrInvalidPath},
		{"null byte", "file\x00.go", ErrInvalidPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRelativePath(tt.path)
			if err != tt.wantErr {
				t.Errorf("ValidateRelativePath(%q) error = %v, want %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestIsWithinDirReal(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "nested", "file.go")

	within, err := IsWithinDirReal(base, target)
	if err != nil {
		t.Fatalf("IsWithinDirReal returned error: %v", err)
	}
	if !within {
		t.Fatalf("expected target path to be within base")
	}
}

func TestIsWithinDirRealRejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(base, "link")

	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	target := filepath.Join(link, "escape.go")
	within, err := IsWithinDirReal(base, target)
	if err != nil {
		t.Fatalf("IsWithinDirReal returned error: %v", err)
	}
	if within {
		t.Fatalf("expected symlink target path to be outside base")
	}
}
