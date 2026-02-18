package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/youruser/bb7/internal/config"
	"github.com/youruser/bb7/internal/diff"
	"github.com/youruser/bb7/internal/llm"
)

//go:embed edit_file_anchor_prompt.txt
var editFileAnchorPrompt string

//go:embed edit_file_sr_prompt.txt
var editFileSRPrompt string

//go:embed edit_file_sr_multi_prompt.txt
var editFileSRMultiPrompt string

//go:embed testdata/*
var testdataFS embed.FS

type testCase struct {
	name     string   // display name
	sources  []string // filenames in testdata/
	expected []string // expected filenames in testdata/ (parallel with sources)
	prompt   string   // task description for the model
}

var tests = []testCase{
	{
		name:     "Combined common task",
		sources:  []string{"01_combined.go"},
		expected: []string{"01_combined_expected.go"},
		prompt: `Make the following changes to ` + "`01_combined.go`" + `:

1. Add ` + "`\"strings\"`" + ` to the import block, between ` + "`\"net/http\"`" + ` and ` + "`\"time\"`" + `.

2. Rename ` + "`HandleCreateUser`" + ` to ` + "`HandleRegisterUser`" + ` and change its comment to:
` + "```" + `
// HandleRegisterUser validates and creates a new user from the request body.
` + "```" + `

3. Replace the body of ` + "`HandleRegisterUser`" + ` (everything between the opening and closing braces of the function) with:
` + "```go" + `
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var input struct {
		Username string ` + "`json:\"username\"`" + `
		Email    string ` + "`json:\"email\"`" + `
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)

	if input.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(input.Username) < 3 {
		writeError(w, http.StatusBadRequest, "username must be at least 3 characters")
		return
	}
	if input.Email == "" || !strings.Contains(input.Email, "@") {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return
	}

	for _, u := range s.users {
		if u.Username == input.Username {
			writeError(w, http.StatusConflict, "username already taken")
			return
		}
	}

	user := &User{
		ID:        s.nextID,
		Username:  input.Username,
		Email:     input.Email,
		CreatedAt: time.Now(),
		Active:    true,
	}
	s.users[user.ID] = user
	s.nextID++

	log.Printf("registered user %d: %s", user.ID, user.Username)
	writeJSON(w, http.StatusCreated, user)
` + "```" + `

4. In ` + "`RegisterRoutes`" + `, change ` + "`s.HandleCreateUser`" + ` to ` + "`s.HandleRegisterUser`" + `.`,
	},
	{
		name:     "Reorder functions",
		sources:  []string{"02_reorder.go"},
		expected: []string{"02_reorder_expected.go"},
		prompt: `Reorder the functions in ` + "`02_reorder.go`" + `. Currently the functions appear in this order:
1. FormatOutput
2. ProcessBatch
3. ParseConfig
4. ValidateInput
5. CleanupTemp

Move ` + "`ProcessBatch`" + ` (including its comment) from its current position (between FormatOutput and ParseConfig) to after ` + "`ValidateInput`" + ` (between ValidateInput and CleanupTemp). The new order should be:
1. FormatOutput
2. ParseConfig
3. ValidateInput
4. ProcessBatch
5. CleanupTemp

Do not change any function's content — only move ` + "`ProcessBatch`" + `.`,
	},
	{
		name:     "Multiple scattered edits",
		sources:  []string{"03_scattered.go"},
		expected: []string{"03_scattered_expected.go"},
		prompt: `Make exactly these 4 changes to ` + "`03_scattered.go`" + `:

1. Change the value of the ` + "`MaxRetries`" + ` constant from ` + "`3`" + ` to ` + "`5`" + `.

2. In the ` + "`FilterByTag`" + ` method, rename the local variable ` + "`result`" + ` to ` + "`matched`" + `. There are 3 occurrences inside that function: the declaration (` + "`var result []*Record`" + `), the append (` + "`result = append(result, r)`" + `), and the log line (` + "`len(result)`" + `). Change all three. Do not touch any ` + "`result`" + ` variables in other functions.

3. In the ` + "`FormatRecord`" + ` function, add a nil check at the beginning of the function body. Insert these lines right after the opening brace:
` + "```go" + `
	if r == nil {
		return "<nil record>"
	}
` + "```" + `

4. In the ` + "`MergeDatasets`" + ` function, change the log message from:
` + "```" + `
	log.Printf("merged datasets: %d + %d = %d records", len(a.Records), len(b.Records), len(records))
` + "```" + `
to:
` + "```" + `
	log.Printf("merged %d + %d = %d records from %q and %q", len(a.Records), len(b.Records), len(records), a.Source, b.Source)
` + "```",
	},
	{
		name:     "Edit near duplicates",
		sources:  []string{"04_duplicates.go"},
		expected: []string{"04_duplicates_expected.go"},
		prompt: `In ` + "`04_duplicates.go`" + `, add an authorization check to the ` + "`HandleUpdate`" + ` method only. Do NOT modify HandleCreate, HandleRead, HandleDelete, or any other function.

Insert the following lines at the beginning of ` + "`HandleUpdate`" + `'s body, right after the method check (after the ` + "`if r.Method != http.MethodPut`" + ` block's closing brace and before the ` + "`id := r.URL.Query().Get(\"id\")`" + ` line):

` + "```go" + `
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		respondError(w, http.StatusUnauthorized, "authorization required")
		return
	}
` + "```" + `

This should be inserted between the method check and the id extraction, adding a blank line before the id line.`,
	},
	{
		name:     "Deeply nested code",
		sources:  []string{"05_nested.go"},
		expected: []string{"05_nested_expected.go"},
		prompt: `In ` + "`05_nested.go`" + `, inside the ` + "`processItem`" + ` method, make changes in the ` + "`case \"compute\":`" + ` branch only:

1. In the ` + "`if item.Status == StatusPending`" + ` block within the ` + "`case \"compute\":`" + ` branch, change the timeout from 30 to 60 seconds:
   Change ` + "`timeout := 30 * time.Second`" + ` to ` + "`timeout := 60 * time.Second`" + `

2. In that same block, change the log line from:
` + "```" + `
				log.Printf("computing item %s with timeout %v", item.ID, timeout)
` + "```" + `
   to:
` + "```" + `
				log.Printf("computing item %s (attempt %d) with timeout %v", item.ID, attempt+1, timeout)
` + "```" + `

Do NOT change the timeout or log lines in any other branch (not in the StatusActive block, not in "transform", not in "validate").`,
	},
	{
		name:     "Large region replacement",
		sources:  []string{"06_large_region.go"},
		expected: []string{"06_large_region_expected.go"},
		prompt: `In ` + "`06_large_region.go`" + `, make two changes:

1. Add ` + "`\"text/template\"`" + ` to the import block, between ` + "`\"strings\"`" + ` and ` + "`\"time\"`" + `.

2. Replace the ` + "`GenerateReport`" + ` function and add a template variable before it. Replace everything from the ` + "`// GenerateReport produces`" + ` comment through the closing brace of the ` + "`GenerateReport`" + ` function with:

` + "```go" + `
// reportTemplate is the template used by GenerateReport.
var reportTemplate = template.Must(template.New("report").Parse(` + "`" + `{{ header .Title 60 }}
Author: {{ .Author }}
Date: {{ formatDate .Date }}
Total Items: {{ .TotalItems }}
{{ range $i, $s := .Sections }}
--- Section {{ inc $i }}: {{ $s.Name }} ---
{{ $s.Content }}
{{ range $j, $item := $s.Items }}  {{ inc $j }}. {{ $item }}
{{ end }}Count: {{ $s.Count }}
{{ end }}{{ if .Summary }}SUMMARY: {{ .Summary }}
{{ end }}{{ footer 60 }}
` + "`" + `))

// GenerateReport produces a complete text report from the given data.
func GenerateReport(data *ReportData) string {
	funcMap := template.FuncMap{
		"header":     FormatHeader,
		"footer":     FormatFooter,
		"formatDate": func(t time.Time) string { return t.Format("2006-01-02") },
		"inc":        func(i int) int { return i + 1 },
	}
	tmpl := template.Must(reportTemplate.Clone())
	tmpl.Funcs(funcMap)

	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return fmt.Sprintf("error generating report: %v", err)
	}
	return sb.String()
}
` + "```",
	},
	{
		name:     "Multi-file coordinated edit",
		sources:  []string{"07_service.go", "07_service_test.go"},
		expected: []string{"07_service_expected.go", "07_service_test_expected.go"},
		prompt: `Make the following coordinated changes across both files to add a ` + "`priority`" + ` parameter to ` + "`CreateOrder`" + `.

In ` + "`07_service.go`" + `:

1. Add a ` + "`Priority string`" + ` field to the ` + "`Order`" + ` struct, between ` + "`Total`" + ` and ` + "`Status`" + `.

2. Change the ` + "`CreateOrder`" + ` comment to: ` + "`// CreateOrder creates a new order with the given items and priority.`" + `

3. Add a ` + "`priority string`" + ` parameter to ` + "`CreateOrder`" + `, after ` + "`total float64`" + `.

4. Add this validation after the ` + "`total <= 0`" + ` check:
` + "```go" + `
	if priority != "normal" && priority != "rush" {
		return nil, fmt.Errorf("priority must be \"normal\" or \"rush\"")
	}
` + "```" + `

5. Add ` + "`Priority: priority,`" + ` to the order literal, between ` + "`Total`" + ` and ` + "`Status`" + `.

6. Change the log message to:
` + "```go" + `
	log.Printf("created order %s for %s: %s (total: $%.2f, priority: %s)",
		order.ID, order.Customer, strings.Join(order.Items, ", "), order.Total, order.Priority)
` + "```" + `

In ` + "`07_service_test.go`" + `:

7. Add ` + "`\"normal\"`" + ` as the last argument to every ` + "`CreateOrder`" + ` call. There are 7 calls total across all test functions. Change each one, for example:
   - ` + "`svc.CreateOrder(\"Alice\", []string{\"Widget\", \"Gadget\"}, 29.99)`" + ` becomes ` + "`svc.CreateOrder(\"Alice\", []string{\"Widget\", \"Gadget\"}, 29.99, \"normal\")`" + `

Do NOT add new test functions. Only modify existing ` + "`CreateOrder`" + ` calls.`,
	},
}

type testResult struct {
	name      string
	passed    bool
	elapsed   time.Duration
	cost      float64
	err       string          // error details for failures
	toolCalls []*llm.ToolCall // raw tool calls for logging
}

// logEntry is the JSON structure written to log files.
type logEntry struct {
	Model     string            `json:"model"`
	Mode      string            `json:"mode"`
	Test      string            `json:"test"`
	Passed    bool              `json:"passed"`
	Error     string            `json:"error,omitempty"`
	Elapsed   float64           `json:"elapsed_seconds"`
	Cost      float64           `json:"cost"`
	ToolCalls []json.RawMessage `json:"tool_calls"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: go run ./cmd/bench <model-id> [--mode=sr|sr_multi|anchored] [--test=N]\n")
		fmt.Fprintf(os.Stderr, "   eg: go run ./cmd/bench anthropic/claude-sonnet-4\n")
		fmt.Fprintf(os.Stderr, "   eg: go run ./cmd/bench anthropic/claude-sonnet-4 --mode=sr\n")
		fmt.Fprintf(os.Stderr, "   eg: go run ./cmd/bench anthropic/claude-sonnet-4 --mode=sr --test=2\n")
		os.Exit(1)
	}
	model := os.Args[1]

	mode := "anchored"
	testFilter := 0 // 0 = run all
	for _, arg := range os.Args[2:] {
		if strings.HasPrefix(arg, "--mode=") {
			mode = strings.TrimPrefix(arg, "--mode=")
		}
		if strings.HasPrefix(arg, "--test=") {
			fmt.Sscanf(strings.TrimPrefix(arg, "--test="), "%d", &testFilter)
		}
	}
	if mode != "anchored" && mode != "sr" && mode != "sr_multi" {
		fmt.Fprintf(os.Stderr, "invalid mode: %s (use 'anchored', 'sr', or 'sr_multi')\n", mode)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	client := llm.NewClient(cfg.BaseURL, cfg.APIKey, *cfg.AllowTraining, *cfg.AllowDataRetention, *cfg.ExplicitCacheKey)

	// Create log directory
	modelSlug := strings.ReplaceAll(model, "/", "_")
	ts := time.Now().Format("20060102_150405")
	logDir := filepath.Join("cmd", "bench", "logs", fmt.Sprintf("%s_%s_%s", ts, modelSlug, mode))
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log dir: %v\n", err)
		os.Exit(1)
	}

	// Header
	fmt.Printf("edit_file benchmark — model: %s, mode: %s\n", model, mode)
	if testFilter > 0 {
		fmt.Printf("  (running test %d only)\n", testFilter)
	}
	fmt.Printf("  logs: %s/\n", logDir)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	passed := 0
	totalCost := 0.0
	total := 0

	for i, tc := range tests {
		if testFilter > 0 && i+1 != testFilter {
			continue
		}
		total++
		result := runTest(client, model, mode, tc, i, len(tests))
		if result.passed {
			passed++
		}
		totalCost += result.cost

		// Write log file
		writeLog(logDir, model, mode, i+1, tc, result)
	}

	// Summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Result: %d/%d passed | Cost: $%.3f\n", passed, total, totalCost)
}

func writeLog(logDir, model, mode string, testNum int, tc testCase, result testResult) {
	entry := logEntry{
		Model:   model,
		Mode:    mode,
		Test:    fmt.Sprintf("%02d_%s", testNum, tc.name),
		Passed:  result.passed,
		Error:   result.err,
		Elapsed: result.elapsed.Seconds(),
		Cost:    result.cost,
	}

	for _, tc := range result.toolCalls {
		entry.ToolCalls = append(entry.ToolCalls, json.RawMessage(tc.Function.Arguments))
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal log entry: %v\n", err)
		return
	}

	logPath := filepath.Join(logDir, fmt.Sprintf("%02d.json", testNum))
	if err := os.WriteFile(logPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write log: %v\n", err)
	}
}

func runTest(client *llm.Client, model, mode string, tc testCase, index, total int) testResult {
	result := testResult{name: tc.name}

	// Load source and expected files
	files := make(map[string]string)
	expectedFiles := make(map[string]string)
	for i, src := range tc.sources {
		data, err := testdataFS.ReadFile("testdata/" + src)
		if err != nil {
			result.err = fmt.Sprintf("read source %s: %v", src, err)
			printResult(index, total, result)
			return result
		}
		files[src] = string(data)

		exp, err := testdataFS.ReadFile("testdata/" + tc.expected[i])
		if err != nil {
			result.err = fmt.Sprintf("read expected %s: %v", tc.expected[i], err)
			printResult(index, total, result)
			return result
		}
		expectedFiles[src] = string(exp)
	}

	// Build user message with all file contents and task
	var userContent string
	for _, src := range tc.sources {
		userContent += fmt.Sprintf("Here is the file `%s`:\n\n```go\n%s```\n\n", src, files[src])
	}
	userContent += tc.prompt
	messages := []llm.Message{
		{Role: "user", Content: userContent},
	}

	// Select prompt and diff mode based on mode flag
	var systemPrompt string
	var diffMode string
	switch mode {
	case "sr":
		systemPrompt = editFileSRPrompt
		diffMode = "search_replace"
	case "sr_multi":
		systemPrompt = editFileSRMultiPrompt
		diffMode = "search_replace_multi"
	default:
		systemPrompt = editFileAnchorPrompt
		diffMode = "anchored"
	}

	// Call model
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var toolCalls []*llm.ToolCall
	var usage *llm.Usage

	start := time.Now()
	err := client.ChatStream(ctx, model, systemPrompt, messages, nil, diffMode, "", func(event llm.StreamEvent) {
		switch event.Type {
		case "tool_call":
			toolCalls = append(toolCalls, event.ToolCall)
		case "done":
			usage = event.Usage
		}
	})
	result.elapsed = time.Since(start)
	result.toolCalls = toolCalls

	if usage != nil {
		result.cost = usage.Cost
	}

	if err != nil {
		result.err = fmt.Sprintf("api error: %v", err)
		printResult(index, total, result)
		return result
	}

	switch mode {
	case "sr":
		result = applySRToolCalls(result, toolCalls, files, expectedFiles)
	case "sr_multi":
		result = applySRMultiToolCalls(result, toolCalls, files, expectedFiles)
	default:
		result = applyAnchoredToolCall(result, toolCalls, files, expectedFiles)
	}

	printResult(index, total, result)
	return result
}

// applySRToolCalls processes search/replace edit_file tool calls sequentially.
func applySRToolCalls(result testResult, toolCalls []*llm.ToolCall, files, expected map[string]string) testResult {
	// Collect all edit_file calls
	var editCalls []*llm.ToolCall
	for _, tc := range toolCalls {
		if tc.Function.Name == "edit_file" {
			editCalls = append(editCalls, tc)
		}
	}

	if len(editCalls) == 0 {
		names := make([]string, len(toolCalls))
		for i, tc := range toolCalls {
			names[i] = tc.Function.Name
		}
		if len(names) == 0 {
			result.err = "no tool calls returned"
		} else {
			result.err = fmt.Sprintf("no edit_file call (got: %s)", strings.Join(names, ", "))
		}
		return result
	}

	// Apply each edit_file call sequentially
	for i, ec := range editCalls {
		args, err := llm.ParseEditFileArgs(ec.Function.Arguments)
		if err != nil {
			result.err = fmt.Sprintf("parse edit_file[%d]: %v", i, err)
			return result
		}

		content, ok := files[args.Path]
		if !ok {
			result.err = fmt.Sprintf("edit_file[%d]: unknown file %q", i, args.Path)
			return result
		}

		newContent, err := diff.Replace(content, args.OldString, args.NewString, args.ReplaceAll)
		if err != nil {
			result.err = fmt.Sprintf("edit_file[%d] (%s): %v", i, args.Path, err)
			return result
		}
		files[args.Path] = newContent
	}

	return compareFiles(result, files, expected)
}

// applySRMultiToolCalls processes a batched search/replace edit_file tool call.
func applySRMultiToolCalls(result testResult, toolCalls []*llm.ToolCall, files, expected map[string]string) testResult {
	// Find edit_file tool call
	var editCall *llm.ToolCall
	for _, tc := range toolCalls {
		if tc.Function.Name == "edit_file" {
			editCall = tc
			break
		}
	}

	if editCall == nil {
		names := make([]string, len(toolCalls))
		for i, tc := range toolCalls {
			names[i] = tc.Function.Name
		}
		if len(names) == 0 {
			result.err = "no tool calls returned"
		} else {
			result.err = fmt.Sprintf("no edit_file call (got: %s)", strings.Join(names, ", "))
		}
		return result
	}

	args, err := llm.ParseEditFileMultiArgs(editCall.Function.Arguments)
	if err != nil {
		result.err = fmt.Sprintf("parse edit_file: %v", err)
		return result
	}

	// Apply each edit sequentially
	for i, edit := range args.Edits {
		content, ok := files[edit.Path]
		if !ok {
			result.err = fmt.Sprintf("edit %d: unknown file %q", i, edit.Path)
			return result
		}

		newContent, err := diff.Replace(content, edit.OldString, edit.NewString, edit.ReplaceAll)
		if err != nil {
			result.err = fmt.Sprintf("edit %d (%s): %v", i, edit.Path, err)
			return result
		}
		files[edit.Path] = newContent
	}

	return compareFiles(result, files, expected)
}

// applyAnchoredToolCall processes anchored edit_file tool calls.
func applyAnchoredToolCall(result testResult, toolCalls []*llm.ToolCall, files, expected map[string]string) testResult {
	// Find edit_file tool calls
	var editCalls []*llm.ToolCall
	for _, tc := range toolCalls {
		if tc.Function.Name == "edit_file" {
			editCalls = append(editCalls, tc)
		}
	}

	if len(editCalls) == 0 {
		names := make([]string, len(toolCalls))
		for i, tc := range toolCalls {
			names[i] = tc.Function.Name
		}
		if len(names) == 0 {
			result.err = "no tool calls returned"
		} else {
			result.err = fmt.Sprintf("no edit_file call (got: %s)", strings.Join(names, ", "))
		}
		return result
	}

	for ci, editCall := range editCalls {
		args, err := llm.ParseAnchoredEditArgs(editCall.Function.Arguments)
		if err != nil {
			result.err = fmt.Sprintf("parse args[%d]: %v", ci, err)
			return result
		}

		content, ok := files[args.Path]
		if !ok {
			result.err = fmt.Sprintf("edit_file[%d]: unknown file %q", ci, args.Path)
			return result
		}

		changes := make([]diff.Change, len(args.Changes))
		for j, c := range args.Changes {
			changes[j] = diff.Change{
				Start:   c.Start,
				End:     c.End,
				Content: c.Content,
			}
		}

		sourceLines := diff.SplitLines(content)
		applyResult, err := diff.Apply(sourceLines, changes)
		if err != nil {
			result.err = fmt.Sprintf("diff apply[%d] (%s): %v", ci, args.Path, err)
			return result
		}
		files[args.Path] = diff.JoinLines(applyResult.Lines)
	}

	return compareFiles(result, files, expected)
}

// compareFiles checks all files against expected content.
func compareFiles(result testResult, files, expected map[string]string) testResult {
	for path, want := range expected {
		got, ok := files[path]
		if !ok {
			result.err = fmt.Sprintf("file %s: not produced", path)
			return result
		}
		gotNorm := normalizeContent(got)
		wantNorm := normalizeContent(want)
		if gotNorm != wantNorm {
			result.err = fmt.Sprintf("%s: %s", path, describeMismatch(gotNorm, wantNorm))
			return result
		}
	}
	result.passed = true
	return result
}

func printResult(index, total int, r testResult) {
	label := fmt.Sprintf("[%d/%d] %s", index+1, total, r.name)

	// Pad with dots to align result
	dots := 50 - len(label)
	if dots < 3 {
		dots = 3
	}

	if r.passed {
		fmt.Printf("%s %s PASS  (%.1fs, $%.3f)\n", label, strings.Repeat(".", dots), r.elapsed.Seconds(), r.cost)
	} else {
		fmt.Printf("%s %s FAIL  (%.1fs, $%.3f)\n", label, strings.Repeat(".", dots), r.elapsed.Seconds(), r.cost)
		fmt.Printf("      %s\n", r.err)
	}
}

// normalizeContent trims trailing whitespace from each line for comparison.
func normalizeContent(content string) string {
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
}

// describeMismatch finds the first differing line between got and want.
func describeMismatch(got, want string) string {
	gotLines := strings.Split(got, "\n")
	wantLines := strings.Split(want, "\n")

	maxLines := len(gotLines)
	if len(wantLines) > maxLines {
		maxLines = len(wantLines)
	}

	for i := 0; i < maxLines; i++ {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			// Truncate long lines for display
			if len(g) > 60 {
				g = g[:57] + "..."
			}
			if len(w) > 60 {
				w = w[:57] + "..."
			}
			return fmt.Sprintf("output mismatch at line %d: got %q, want %q", i+1, g, w)
		}
	}

	return "output mismatch (unknown difference)"
}
