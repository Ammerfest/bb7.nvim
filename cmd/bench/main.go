package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/youruser/bb7/internal/config"
	"github.com/youruser/bb7/internal/diff"
	"github.com/youruser/bb7/internal/llm"
)

//go:embed modify_file_prompt.txt
var modifyFilePrompt string

//go:embed testdata/*
var testdataFS embed.FS

type testCase struct {
	name     string // display name
	source   string // filename in testdata/
	expected string // filename in testdata/
	prompt   string // task description for the model
}

var tests = []testCase{
	{
		name:     "Combined common task",
		source:   "01_combined.go",
		expected: "01_combined_expected.go",
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
		source:   "02_reorder.go",
		expected: "02_reorder_expected.go",
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
		source:   "03_scattered.go",
		expected: "03_scattered_expected.go",
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
		source:   "04_duplicates.go",
		expected: "04_duplicates_expected.go",
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
		source:   "05_nested.go",
		expected: "05_nested_expected.go",
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
		source:   "06_large_region.go",
		expected: "06_large_region_expected.go",
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
}

type testResult struct {
	name    string
	passed  bool
	elapsed time.Duration
	cost    float64
	err     string // error details for failures
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: go run ./cmd/bench <model-id>\n")
		fmt.Fprintf(os.Stderr, "   eg: go run ./cmd/bench anthropic/claude-sonnet-4\n")
		os.Exit(1)
	}
	model := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	client := llm.NewClient(cfg.BaseURL, cfg.APIKey, *cfg.AllowTraining, *cfg.AllowDataRetention)

	// Header
	fmt.Printf("modify_file benchmark — model: %s\n", model)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	results := make([]testResult, len(tests))
	passed := 0
	totalCost := 0.0

	for i, tc := range tests {
		results[i] = runTest(client, model, tc, i, len(tests))
		if results[i].passed {
			passed++
		}
		totalCost += results[i].cost
	}

	// Summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Result: %d/%d passed | Cost: $%.3f\n", passed, len(tests), totalCost)
}

func runTest(client *llm.Client, model string, tc testCase, index, total int) testResult {
	result := testResult{name: tc.name}

	// Load source and expected
	source, err := testdataFS.ReadFile("testdata/" + tc.source)
	if err != nil {
		result.err = fmt.Sprintf("read source: %v", err)
		printResult(index, total, result)
		return result
	}
	expected, err := testdataFS.ReadFile("testdata/" + tc.expected)
	if err != nil {
		result.err = fmt.Sprintf("read expected: %v", err)
		printResult(index, total, result)
		return result
	}

	// Build user message with file content and task
	userContent := fmt.Sprintf("Here is the file `%s`:\n\n```go\n%s```\n\n%s", tc.source, string(source), tc.prompt)
	messages := []llm.Message{
		{Role: "user", Content: userContent},
	}

	// Call model
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var toolCalls []*llm.ToolCall
	var usage *llm.Usage

	start := time.Now()
	err = client.ChatStream(ctx, model, modifyFilePrompt, messages, nil, true, func(event llm.StreamEvent) {
		switch event.Type {
		case "tool_call":
			toolCalls = append(toolCalls, event.ToolCall)
		case "done":
			usage = event.Usage
		}
	})
	result.elapsed = time.Since(start)

	if usage != nil {
		result.cost = usage.Cost
	}

	if err != nil {
		result.err = fmt.Sprintf("api error: %v", err)
		printResult(index, total, result)
		return result
	}

	// Find modify_file tool call
	var modifyCall *llm.ToolCall
	for _, tc := range toolCalls {
		if tc.Function.Name == "modify_file" {
			modifyCall = tc
			break
		}
	}

	if modifyCall == nil {
		names := make([]string, len(toolCalls))
		for i, tc := range toolCalls {
			names[i] = tc.Function.Name
		}
		if len(names) == 0 {
			result.err = "no tool calls returned"
		} else {
			result.err = fmt.Sprintf("no modify_file call (got: %s)", strings.Join(names, ", "))
		}
		printResult(index, total, result)
		return result
	}

	// Parse modify_file args
	args, err := llm.ParseModifyFileArgs(modifyCall.Function.Arguments)
	if err != nil {
		result.err = fmt.Sprintf("parse args: %v", err)
		printResult(index, total, result)
		return result
	}

	// Convert to diff.Change
	changes := make([]diff.Change, len(args.Changes))
	for j, c := range args.Changes {
		changes[j] = diff.Change{
			Start:   c.Start,
			End:     c.End,
			Content: c.Content,
		}
	}

	// Apply diff
	sourceLines := diff.SplitLines(string(source))
	applyResult, err := diff.Apply(sourceLines, changes)
	if err != nil {
		result.err = fmt.Sprintf("diff apply: %v", err)
		printResult(index, total, result)
		return result
	}

	// Compare output to expected
	got := normalizeContent(diff.JoinLines(applyResult.Lines))
	want := normalizeContent(string(expected))

	if got == want {
		result.passed = true
	} else {
		result.err = describeMismatch(got, want)
	}

	printResult(index, total, result)
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
