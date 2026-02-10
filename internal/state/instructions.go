package state

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const projectInstructionsFilename = "instructions"
const globalInstructionsFilename = "instructions.md"

// InstructionsInfo holds information about loaded instruction files.
type InstructionsInfo struct {
	GlobalPath    string `json:"global_path,omitempty"`   // Path to global instructions file
	GlobalExists  bool   `json:"global_exists"`           // Whether global instructions file exists
	ProjectPath   string `json:"project_path,omitempty"`  // Path to project instructions file
	ProjectExists bool   `json:"project_exists"`          // Whether project instructions file exists
	ProjectError  string `json:"project_error,omitempty"` // Error parsing project instructions (if any)
}

type instructionsParseError struct {
	Path    string
	Line    int
	Message string
}

func (e *instructionsParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// GetInstructionsInfo returns info about available instruction files.
func (s *State) GetInstructionsInfo() InstructionsInfo {
	info := InstructionsInfo{}

	// Global instructions: ~/.config/bb7/instructions.md
	homeDir, err := os.UserHomeDir()
	if err == nil {
		info.GlobalPath = filepath.Join(homeDir, ".config", "bb7", globalInstructionsFilename)
		if _, err := os.Stat(info.GlobalPath); err == nil {
			info.GlobalExists = true
		}
	}

	// Project instructions: {project_root}/.bb7/instructions
	if s.ProjectRoot != "" {
		info.ProjectPath = filepath.Join(s.ProjectRoot, ".bb7", projectInstructionsFilename)
		if _, err := os.Stat(info.ProjectPath); err == nil {
			info.ProjectExists = true
			if _, err := parseInstructionsFile(info.ProjectPath, s.ProjectRoot); err != nil {
				info.ProjectError = err.Error()
			}
		}
	}

	return info
}

// LoadGlobalInstructions reads the global instructions file content.
// Strips @@ comments and returns empty string if file doesn't exist
// or is empty after stripping.
func (s *State) LoadGlobalInstructions() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", nil // Not an error, just no global instructions
	}

	path := filepath.Join(homeDir, ".config", "bb7", globalInstructionsFilename)
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	stripped := StripComments(string(content))
	if strings.TrimSpace(stripped) == "" {
		return "", nil
	}
	return stripped, nil
}

// LoadProjectInstructions reads the project instructions file content.
// Returns empty string if file doesn't exist.
func (s *State) LoadProjectInstructions() (string, error) {
	if s.ProjectRoot == "" {
		return "", nil
	}

	path := filepath.Join(s.ProjectRoot, ".bb7", projectInstructionsFilename)
	return loadInstructionsFile(path, s.ProjectRoot)
}

// PrepareInstructionsFile ensures the instructions file exists for the given level
// and returns its absolute path. For project/global, creates a default template if missing.
// For system, creates a file with the provided default content if missing.
// Level must be "project", "global", or "system".
func (s *State) PrepareInstructionsFile(level string, defaultSystemPrompt string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var path string
	var defaultContent string

	switch level {
	case "project":
		if s.ProjectRoot == "" {
			return "", errors.New("no project root set")
		}
		path = filepath.Join(s.ProjectRoot, ".bb7", projectInstructionsFilename)
		defaultContent = defaultProjectInstructions()
	case "global":
		path = filepath.Join(homeDir, ".config", "bb7", globalInstructionsFilename)
		defaultContent = defaultGlobalInstructions()
	case "system":
		path = filepath.Join(homeDir, ".config", "bb7", "system_prompt.txt")
		defaultContent = defaultSystemPromptTemplate(defaultSystemPrompt)
	default:
		return "", errors.New("invalid level: must be project, global, or system")
	}

	// Create parent directories if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	// Create file with default content if it doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(defaultContent), 0644); err != nil {
			return "", err
		}
	}

	return path, nil
}

// BuildInstructionsBlock returns the formatted instructions block for LLM injection.
// Returns empty string if no instruction files exist.
func (s *State) BuildInstructionsBlock() (string, error) {
	var result string

	globalContent, err := s.LoadGlobalInstructions()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(globalContent) != "" {
		result += "<user-instructions source=\"~/.config/bb7/instructions.md\">\n"
		result += globalContent
		if globalContent[len(globalContent)-1] != '\n' {
			result += "\n"
		}
		result += "</user-instructions>\n\n"
	}

	projectContent, err := s.LoadProjectInstructions()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(projectContent) != "" {
		result += "<project-instructions source=\".bb7/instructions\">\n"
		result += projectContent
		if projectContent[len(projectContent)-1] != '\n' {
			result += "\n"
		}
		result += "</project-instructions>\n\n"
	}

	return result, nil
}

func defaultProjectInstructions() string {
	return strings.Join([]string{
		"@@ BB-7 project instructions",
		"@@",
		"@@ Everything in this file (after stripping comments and expanding",
		"@@ includes) is sent with every request — keep it focused.",
		"@@ Lines starting with @@ are comments (stripped before sending)",
		"@@ Include other files with the @include directive:",
		"@@",
		"@@ @include <path>           Include a file (relative to project root)",
		"@@ @include \"path with spaces\"  Quoted form for paths with spaces",
		"@@",
		"@@ Included files must be inside the project directory.",
		"@@ Directives are ignored inside fenced code blocks (``` or ~~~).",
		"@@ Included files are inserted verbatim and not re-parsed.",
		"@@",
		"@@ Example:",
		"@@ @include ARCHITECTURE.md",
		"",
		"# Add project context here (Markdown ok)",
		"",
	}, "\n")
}

func defaultGlobalInstructions() string {
	return strings.Join([]string{
		"@@ BB-7 global instructions",
		"@@",
		"@@ This file is sent with every request, across all projects.",
		"@@ Use it for preferences that aren't project-specific:",
		"@@ your background and experience level, tools and dev environment,",
		"@@ communication style, areas you want to learn more about, etc.",
		"@@ Lines starting with @@ are comments (stripped before sending)",
		"",
	}, "\n")
}

func defaultSystemPromptTemplate(builtinPrompt string) string {
	header := strings.Join([]string{
		"@@ BB-7 system prompt override",
		"@@",
		"@@ This file replaces the built-in system prompt entirely.",
		"@@ Edit with care — the system prompt defines core behavior.",
		"@@ Lines starting with @@ are comments (stripped before sending)",
		"",
	}, "\n")
	return header + builtinPrompt
}

// StripComments removes @@ comment lines from instruction content,
// respecting fenced code blocks (``` or ~~~).
func StripComments(content string) string {
	var out strings.Builder
	inFence := false
	for _, line := range strings.Split(content, "\n") {
		if isFenceLine(line) {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "@@") {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		out.WriteString(line)
	}
	return out.String()
}

func loadInstructionsFile(path string, projectRoot string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	return parseInstructionsContent(path, projectRoot, content)
}

func parseInstructionsFile(path string, projectRoot string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return parseInstructionsContent(path, projectRoot, content)
}

func parseInstructionsContent(path string, projectRoot string, content []byte) (string, error) {
	reader := bufio.NewReader(bytes.NewReader(content))
	var out strings.Builder
	lineNum := 0
	inFence := false

	for {
		lineRaw, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		if len(lineRaw) == 0 && err == io.EOF {
			break
		}
		lineNum++

		line := strings.TrimRight(lineRaw, "\r\n")
		hasNewline := strings.HasSuffix(lineRaw, "\n")

		if isFenceLine(line) {
			inFence = !inFence
			out.WriteString(lineRaw)
			if err == io.EOF {
				break
			}
			continue
		}

		if !inFence {
			if strings.HasPrefix(line, "@@") {
				if err == io.EOF {
					break
				}
				continue
			}

			if includePath, ok, parseErr := parseIncludeDirective(line, path, lineNum); ok {
				if parseErr != nil {
					return "", parseErr
				}
				resolvedPath, err := resolveIncludePath(projectRoot, includePath)
				if err != nil {
					return "", &instructionsParseError{Path: path, Line: lineNum, Message: err.Error()}
				}
				included, err := os.ReadFile(resolvedPath)
				if err != nil {
					if os.IsNotExist(err) {
						return "", &instructionsParseError{Path: path, Line: lineNum, Message: fmt.Sprintf("include file not found: %s", includePath)}
					}
					return "", &instructionsParseError{Path: path, Line: lineNum, Message: fmt.Sprintf("include read failed: %v", err)}
				}
				out.Write(included)
				if hasNewline && (len(included) == 0 || included[len(included)-1] != '\n') {
					out.WriteString("\n")
				}
				if err == io.EOF {
					break
				}
				continue
			}
		}

		out.WriteString(lineRaw)
		if err == io.EOF {
			break
		}
	}

	return out.String(), nil
}

func isFenceLine(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func parseIncludeDirective(line string, path string, lineNum int) (string, bool, error) {
	if !strings.HasPrefix(line, "@include") {
		return "", false, nil
	}
	if len(line) == len("@include") {
		return "", true, &instructionsParseError{Path: path, Line: lineNum, Message: "@include missing path"}
	}
	if !isWhitespace(rune(line[len("@include")])) {
		return "", false, nil
	}
	rest := strings.TrimSpace(line[len("@include"):])
	if rest == "" {
		return "", true, &instructionsParseError{Path: path, Line: lineNum, Message: "@include missing path"}
	}
	if strings.HasPrefix(rest, "\"") {
		return parseQuotedPath(rest, path, lineNum)
	}
	return rest, true, nil
}

func parseQuotedPath(rest string, path string, lineNum int) (string, bool, error) {
	end := strings.Index(rest[1:], "\"")
	if end == -1 {
		return "", true, &instructionsParseError{Path: path, Line: lineNum, Message: "@include missing closing quote"}
	}
	end++
	value := rest[1:end]
	trailing := strings.TrimSpace(rest[end+1:])
	if trailing != "" {
		return "", true, &instructionsParseError{Path: path, Line: lineNum, Message: "@include trailing text after quoted path"}
	}
	if value == "" {
		return "", true, &instructionsParseError{Path: path, Line: lineNum, Message: "@include missing path"}
	}
	return value, true, nil
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t'
}

func resolveIncludePath(projectRoot string, includePath string) (string, error) {
	if projectRoot == "" {
		return "", errors.New("include requires project root")
	}
	if includePath == "" {
		return "", errors.New("include path is empty")
	}
	if filepath.IsAbs(includePath) {
		return "", errors.New("include path must be relative to project root")
	}
	clean := filepath.Clean(includePath)
	if clean == "." || clean == "" {
		return "", errors.New("include path is empty")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("include path escapes project root")
	}
	rootAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", err
	}
	rootResolved, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(rootResolved, clean)
	joinedAbs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	joinedResolved, err := filepath.EvalSymlinks(joinedAbs)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootResolved, joinedResolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("include path escapes project root")
	}
	return joinedResolved, nil
}
