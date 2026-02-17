package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/youruser/bb7/internal/config"
	"github.com/youruser/bb7/internal/llm"
	"github.com/youruser/bb7/internal/state"
)

func setupSendIntegrationEnv(t *testing.T, baseURL string) {
	t.Helper()

	oldAppState := appState
	oldAppConfig := appConfig
	oldLLMClient := llmClient
	oldActiveStream := activeStream

	appState = state.New()
	diffMode := "search_replace"
	appConfig = &config.Config{
		APIKey:       "test-key",
		BaseURL:      baseURL,
		DefaultModel: "test-model",
		TitleModel:   "test-title-model",
		DiffMode:     &diffMode,
	}
	llmClient = llm.NewClient(baseURL, appConfig.APIKey, false, true)
	resetActiveStreamForTest()

	projectRoot := t.TempDir()
	if err := appState.ProjectInit(projectRoot); err != nil {
		t.Fatalf("ProjectInit failed: %v", err)
	}
	if err := appState.Init(projectRoot); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if _, err := appState.ChatNew("integration-test-chat"); err != nil {
		t.Fatalf("ChatNew failed: %v", err)
	}

	t.Cleanup(func() {
		appState = oldAppState
		appConfig = oldAppConfig
		llmClient = oldLLMClient
		activeStream = oldActiveStream
	})
}

func captureJSONResponses(t *testing.T, fn func()) []map[string]any {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}

	var outBuf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&outBuf, r)
		done <- copyErr
	}()

	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("closing write pipe failed: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("reading captured stdout failed: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("closing read pipe failed: %v", err)
	}

	raw := strings.TrimSpace(outBuf.String())
	if raw == "" {
		return nil
	}

	lines := strings.Split(raw, "\n")
	responses := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var resp map[string]any
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("failed to parse JSON response line %q: %v", line, err)
		}
		responses = append(responses, resp)
	}
	return responses
}

func countResponsesByType(responses []map[string]any, msgType string) int {
	count := 0
	for _, resp := range responses {
		if gotType, _ := resp["type"].(string); gotType == msgType {
			count++
		}
	}
	return count
}

func firstResponseByType(responses []map[string]any, msgType string) map[string]any {
	for _, resp := range responses {
		if gotType, _ := resp["type"].(string); gotType == msgType {
			return resp
		}
	}
	return nil
}

func writeSSEJSON(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json marshal failed: %v", err)
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(data)); err != nil {
		t.Fatalf("failed to write SSE payload: %v", err)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeSSEDone(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
		t.Fatalf("failed to write SSE done marker: %v", err)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func writeFileArgsJSON(t *testing.T, path, content string) string {
	t.Helper()
	data, err := json.Marshal(map[string]string{
		"path":    path,
		"content": content,
	})
	if err != nil {
		t.Fatalf("marshal write_file args failed: %v", err)
	}
	return string(data)
}

func TestHandleSendIntegrationSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}

		// Check if this is a non-streaming request (title generation)
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var reqBody map[string]any
		json.Unmarshal(body, &reqBody)
		if stream, ok := reqBody["stream"].(bool); !ok || !stream {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{
					map[string]any{"message": map[string]any{"content": "Test Title"}},
				},
			})
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")

		writeSSEJSON(t, w, map[string]any{
			"choices": []any{
				map[string]any{
					"delta": map[string]any{"content": "I updated the file."},
				},
			},
		})

		writeSSEJSON(t, w, map[string]any{
			"choices": []any{
				map[string]any{
					"delta": map[string]any{"reasoning": "Checking context and writing output."},
				},
			},
		})

		writeSSEJSON(t, w, map[string]any{
			"choices": []any{
				map[string]any{
					"delta": map[string]any{
						"tool_calls": []any{
							map[string]any{
								"index": 0,
								"id":    "call_1",
								"type":  "function",
								"function": map[string]any{
									"name":      "write_file",
									"arguments": writeFileArgsJSON(t, "src/generated.go", "package main\n\nfunc main() {}\n"),
								},
							},
						},
					},
				},
			},
		})

		writeSSEJSON(t, w, map[string]any{
			"usage": map[string]any{
				"prompt_tokens":     12,
				"completion_tokens": 7,
				"cached_tokens":     3,
				"total_tokens":      19,
				"cost":              0.0012,
			},
		})
		writeSSEDone(t, w)
	}))
	defer func() {
		// Wait briefly for async title generation goroutine to complete
		time.Sleep(50 * time.Millisecond)
		server.Close()
	}()

	setupSendIntegrationEnv(t, server.URL)

	reqID := "req-send-ok"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}

	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content": "Please update the file",
			"model":   "test-model",
		})
	})

	if countResponsesByType(responses, "error") != 0 {
		t.Fatalf("expected no error responses, got %+v", responses)
	}
	if countResponsesByType(responses, "done") != 1 {
		t.Fatalf("expected exactly one done response, got %+v", responses)
	}
	if countResponsesByType(responses, "chunk") == 0 {
		t.Fatalf("expected at least one chunk response, got %+v", responses)
	}
	if countResponsesByType(responses, "thinking") == 0 {
		t.Fatalf("expected at least one thinking response, got %+v", responses)
	}

	doneResp := firstResponseByType(responses, "done")
	outputFiles, ok := doneResp["output_files"].([]any)
	if !ok || len(outputFiles) != 1 || outputFiles[0] != "src/generated.go" {
		t.Fatalf("unexpected output_files in done response: %+v", doneResp["output_files"])
	}

	usage, ok := doneResp["usage"].(map[string]any)
	if !ok {
		t.Fatalf("missing usage in done response: %+v", doneResp)
	}
	if got := usage["total_tokens"]; got != float64(19) {
		t.Fatalf("unexpected usage total_tokens: %v", got)
	}

	gotOutput, err := appState.GetOutputFile("src/generated.go")
	if err != nil {
		t.Fatalf("GetOutputFile failed: %v", err)
	}
	if gotOutput != "package main\n\nfunc main() {}\n" {
		t.Fatalf("unexpected output content: %q", gotOutput)
	}

	msgs := appState.ActiveChat.Messages
	if len(msgs) != 2 {
		t.Fatalf("expected user+assistant messages, got %d", len(msgs))
	}
	assistant := msgs[1]
	if assistant.Role != "assistant" {
		t.Fatalf("expected second message role assistant, got %q", assistant.Role)
	}
	if len(assistant.Parts) < 3 {
		t.Fatalf("expected assistant parts including thinking/text/context_event, got %+v", assistant.Parts)
	}

	foundFileEvent := false
	for _, part := range assistant.Parts {
		if part.Type == "context_event" && part.Action == "AssistantWriteFile" && part.Path == "src/generated.go" {
			foundFileEvent = true
			if !part.Added {
				t.Fatalf("expected AssistantWriteFile event to be marked as added, got %+v", part)
			}
		}
	}
	if !foundFileEvent {
		t.Fatalf("missing AssistantWriteFile context_event in assistant message: %+v", assistant.Parts)
	}
}

func TestHandleSendIntegrationDuplicateWriteTerminatesStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")

		writeSSEJSON(t, w, map[string]any{
			"choices": []any{
				map[string]any{
					"delta": map[string]any{"content": "Applying edits now."},
				},
			},
		})

		writeSSEJSON(t, w, map[string]any{
			"choices": []any{
				map[string]any{
					"delta": map[string]any{
						"tool_calls": []any{
							map[string]any{
								"index": 0,
								"id":    "call_1",
								"type":  "function",
								"function": map[string]any{
									"name":      "write_file",
									"arguments": writeFileArgsJSON(t, "dup.go", "first"),
								},
							},
						},
					},
				},
			},
		})

		writeSSEJSON(t, w, map[string]any{
			"choices": []any{
				map[string]any{
					"delta": map[string]any{
						"tool_calls": []any{
							map[string]any{
								"index": 1,
								"id":    "call_2",
								"type":  "function",
								"function": map[string]any{
									"name":      "write_file",
									"arguments": writeFileArgsJSON(t, "dup.go", "second"),
								},
							},
						},
					},
				},
			},
		})

		writeSSEDone(t, w)
	}))
	defer server.Close()

	setupSendIntegrationEnv(t, server.URL)

	reqID := "req-send-dup"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}

	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content": "Make duplicate writes",
			"model":   "test-model",
		})
	})

	if countResponsesByType(responses, "done") != 0 {
		t.Fatalf("expected no done response on duplicate write_file, got %+v", responses)
	}
	if countResponsesByType(responses, "error") != 1 {
		t.Fatalf("expected exactly one terminal error response, got %+v", responses)
	}

	errResp := firstResponseByType(responses, "error")
	msg, _ := errResp["message"].(string)
	if !strings.Contains(msg, "Duplicate write_file for path in single response: dup.go") {
		t.Fatalf("unexpected duplicate error message: %q", msg)
	}

	// With atomic writes, no files are committed when the stream errors
	_, err := appState.GetOutputFile("dup.go")
	if err == nil {
		t.Fatalf("expected no output file (atomic writes discard on error), but file exists")
	}

	msgs := appState.ActiveChat.Messages
	if len(msgs) != 2 {
		t.Fatalf("expected user + system message on duplicate write, got %d messages", len(msgs))
	}
	if msgs[1].Role != "system" {
		t.Fatalf("expected terminal error to be recorded as system message, got role %q", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Content, "Duplicate write_file for path in single response: dup.go") {
		t.Fatalf("unexpected system message content: %q", msgs[1].Content)
	}
}
