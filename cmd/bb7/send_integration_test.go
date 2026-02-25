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
	"github.com/youruser/bb7/internal/diff"
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
	llmClient = llm.NewClient(baseURL, appConfig.APIKey, false, true, false)
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

func requireDiffErrorResponse(t *testing.T, responses []map[string]any) map[string]any {
	t.Helper()
	if countResponsesByType(responses, "diff_error") != 1 {
		t.Fatalf("expected one diff_error response, got %+v", responses)
	}
	return firstResponseByType(responses, "diff_error")
}

func requireToolCallEntries(t *testing.T, diffErr map[string]any) []map[string]any {
	t.Helper()
	raw, ok := diffErr["tool_calls"].([]any)
	if !ok {
		t.Fatalf("expected tool_calls array in diff_error, got %+v", diffErr["tool_calls"])
	}
	entries := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("expected tool_calls entry to be map, got %T", item)
		}
		entries = append(entries, m)
	}
	return entries
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

func editFileMultiArgsJSON(t *testing.T, edits []map[string]any) string {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"edits": edits,
	})
	if err != nil {
		t.Fatalf("marshal edit_file args failed: %v", err)
	}
	return string(data)
}

func editFileArgsJSON(t *testing.T, args map[string]any) string {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal edit_file args failed: %v", err)
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
		if _, hasPromptCacheKey := reqBody["prompt_cache_key"]; hasPromptCacheKey {
			t.Fatalf("did not expect prompt_cache_key in default config request: %+v", reqBody["prompt_cache_key"])
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
				"prompt_tokens_details": map[string]any{
					"cached_tokens":      3,
					"cache_write_tokens": 1,
				},
				"total_tokens": 19,
				"cost":         0.0012,
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
	if got := usage["cached_tokens"]; got != float64(3) {
		t.Fatalf("unexpected usage cached_tokens: %v", got)
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

func TestHandleSendIntegrationExplicitCacheKey(t *testing.T) {
	var seenPromptCacheKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}

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

		if key, ok := reqBody["prompt_cache_key"].(string); ok {
			seenPromptCacheKey = key
		}

		w.Header().Set("Content-Type", "text/event-stream")
		writeSSEJSON(t, w, map[string]any{
			"choices": []any{
				map[string]any{
					"delta": map[string]any{"content": "Done."},
				},
			},
		})
		writeSSEDone(t, w)
	}))
	defer func() {
		time.Sleep(50 * time.Millisecond)
		server.Close()
	}()

	setupSendIntegrationEnv(t, server.URL)
	enabled := true
	appConfig.ExplicitCacheKey = &enabled
	llmClient = llm.NewClient(server.URL, appConfig.APIKey, false, true, true)

	reqID := "req-send-cache-key"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}

	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content": "Check cache key",
			"model":   "test-model",
		})
	})

	if countResponsesByType(responses, "error") != 0 {
		t.Fatalf("expected no error responses, got %+v", responses)
	}
	if countResponsesByType(responses, "done") != 1 {
		t.Fatalf("expected one done response, got %+v", responses)
	}
	if seenPromptCacheKey == "" {
		t.Fatal("expected prompt_cache_key to be sent, got empty")
	}
	expectedSuffix := appState.ActiveChat.ID + ":test-model"
	if !strings.HasSuffix(seenPromptCacheKey, expectedSuffix) {
		t.Fatalf("unexpected prompt_cache_key %q, expected suffix %q", seenPromptCacheKey, expectedSuffix)
	}
}

func TestHandleSendIntegrationConsolidatesWriteEventsPerFile(t *testing.T) {
	baseContent := "Goblin\nOrc\n"
	baseFileID := state.HashFileVersion("src/game.c", baseContent)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}

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
					"delta": map[string]any{"content": "Applied multi-edit update."},
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
								"id":    "call_edit_1",
								"type":  "function",
								"function": map[string]any{
									"name": "edit_file",
									"arguments": editFileMultiArgsJSON(t, []map[string]any{
										{
											"path":        "src/game.c",
											"file_id":     baseFileID,
											"old_string":  "Goblin",
											"new_string":  "Goblin 👺",
											"replace_all": false,
										},
										{
											"path":        "src/game.c",
											"file_id":     baseFileID,
											"old_string":  "Orc",
											"new_string":  "Orc 🪓",
											"replace_all": false,
										},
									}),
								},
							},
						},
					},
				},
			},
		})
		writeSSEJSON(t, w, map[string]any{
			"usage": map[string]any{
				"prompt_tokens":     20,
				"completion_tokens": 10,
				"total_tokens":      30,
			},
		})
		writeSSEDone(t, w)
	}))
	defer func() {
		time.Sleep(50 * time.Millisecond)
		server.Close()
	}()

	setupSendIntegrationEnv(t, server.URL)
	diffMode := "search_replace_multi"
	appConfig.DiffMode = &diffMode
	if err := appState.ContextAdd("src/game.c", baseContent); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	reqID := "req-send-consolidated-events"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}

	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content": "Add emojis to both enemies",
			"model":   "test-model",
		})
	})

	if countResponsesByType(responses, "error") != 0 {
		t.Fatalf("expected no error responses, got %+v", responses)
	}
	if countResponsesByType(responses, "done") != 1 {
		t.Fatalf("expected one done response, got %+v", responses)
	}

	gotOutput, err := appState.GetOutputFile("src/game.c")
	if err != nil {
		t.Fatalf("GetOutputFile failed: %v", err)
	}
	if gotOutput != "Goblin 👺\nOrc 🪓\n" {
		t.Fatalf("unexpected output content: %q", gotOutput)
	}

	msgs := appState.ActiveChat.Messages
	if len(msgs) == 0 {
		t.Fatal("expected messages in chat")
	}
	assistant := msgs[len(msgs)-1]
	if assistant.Role != "assistant" {
		t.Fatalf("expected last message role assistant, got %q", assistant.Role)
	}

	writeEventCount := 0
	for _, part := range assistant.Parts {
		if part.Type == "context_event" && part.Action == "AssistantWriteFile" && part.Path == "src/game.c" {
			writeEventCount++
		}
	}
	if writeEventCount != 1 {
		t.Fatalf("expected 1 AssistantWriteFile event for src/game.c, got %d (parts=%+v)", writeEventCount, assistant.Parts)
	}
}

func TestHandleSendIntegrationFileIDMismatchReturnsDiffError(t *testing.T) {
	var outputID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}

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
					"delta": map[string]any{"content": "Trying edit with explicit file id."},
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
								"id":    "call_edit_file_id",
								"type":  "function",
								"function": map[string]any{
									"name": "edit_file",
									"arguments": editFileArgsJSON(t, map[string]any{
										"path":        "src/game.c",
										"file_id":     "wrong-file-id",
										"old_string":  "Goblin 👺",
										"new_string":  "Goblin 😈",
										"replace_all": false,
									}),
								},
							},
						},
					},
				},
			},
		})
		writeSSEJSON(t, w, map[string]any{
			"usage": map[string]any{
				"prompt_tokens":     40,
				"completion_tokens": 20,
				"total_tokens":      60,
			},
		})
		writeSSEDone(t, w)
	}))
	defer func() {
		time.Sleep(50 * time.Millisecond)
		server.Close()
	}()

	setupSendIntegrationEnv(t, server.URL)
	if err := appState.ContextAdd("src/game.c", "Goblin\n"); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}
	if err := appState.WriteOutputFile("src/game.c", "Goblin 👺\n"); err != nil {
		t.Fatalf("WriteOutputFile failed: %v", err)
	}
	outputID = state.HashFileVersion("src/game.c", "Goblin 👺\n")

	reqID := "req-send-file-id-mismatch"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}

	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content": "Please apply another emoji edit",
			"model":   "test-model",
		})
	})

	if countResponsesByType(responses, "done") != 0 {
		t.Fatalf("expected no done response on file_id mismatch, got %+v", responses)
	}
	if countResponsesByType(responses, "diff_error") != 1 {
		t.Fatalf("expected one diff_error response, got %+v", responses)
	}
	diffErr := firstResponseByType(responses, "diff_error")
	errs, ok := diffErr["errors"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected non-empty diff_error errors, got %+v", diffErr)
	}
	firstErr, _ := errs[0].(string)
	if !strings.Contains(firstErr, "file_id mismatch") {
		t.Fatalf("expected file_id mismatch error, got %q", firstErr)
	}
	if !strings.Contains(firstErr, outputID) {
		t.Fatalf("expected error to include expected output file id %q, got %q", outputID, firstErr)
	}

	gotOutput, err := appState.GetOutputFile("src/game.c")
	if err != nil {
		t.Fatalf("GetOutputFile failed: %v", err)
	}
	if gotOutput != "Goblin 👺\n" {
		t.Fatalf("expected output to remain unchanged after diff_error, got %q", gotOutput)
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
	msgText := state.MessageText(msgs[1])
	if !strings.Contains(msgText, "Duplicate write_file for path in single response: dup.go") {
		t.Fatalf("unexpected system message content: %q", msgText)
	}
}

func TestHandleSendIntegrationSRMultiLargeBatchSuccess(t *testing.T) {
	baseLines := []string{
		"alpha_01",
		"alpha_02",
		"alpha_03",
		"alpha_04",
		"alpha_05",
		"alpha_06",
		"alpha_07",
		"alpha_08",
		"alpha_09",
		"alpha_10",
	}
	baseContent := strings.Join(baseLines, "\n") + "\n"
	expectedLines := make([]string, 0, len(baseLines))
	edits := make([]map[string]any, 0, len(baseLines))
	baseFileID := state.HashFileVersion("src/complex.c", baseContent)
	for _, line := range baseLines {
		newLine := line + "_ok"
		expectedLines = append(expectedLines, newLine)
		edits = append(edits, map[string]any{
			"path":        "src/complex.c",
			"file_id":     baseFileID,
			"old_string":  line,
			"new_string":  newLine,
			"replace_all": false,
		})
	}
	expectedContent := strings.Join(expectedLines, "\n") + "\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
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
					"delta": map[string]any{"content": "Applying many edits in one call."},
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
								"id":    "call_sr_multi_large_success",
								"type":  "function",
								"function": map[string]any{
									"name":      "edit_file",
									"arguments": editFileMultiArgsJSON(t, edits),
								},
							},
						},
					},
				},
			},
		})
		writeSSEJSON(t, w, map[string]any{
			"usage": map[string]any{
				"prompt_tokens":     60,
				"completion_tokens": 20,
				"total_tokens":      80,
			},
		})
		writeSSEDone(t, w)
	}))
	defer server.Close()

	setupSendIntegrationEnv(t, server.URL)
	diffMode := "search_replace_multi"
	appConfig.DiffMode = &diffMode
	if err := appState.ContextAdd("src/complex.c", baseContent); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	reqID := "req-send-sr-multi-large-success"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}
	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content": "Apply large batch",
			"model":   "test-model",
		})
	})

	if countResponsesByType(responses, "error") != 0 {
		t.Fatalf("expected no error responses, got %+v", responses)
	}
	if countResponsesByType(responses, "diff_error") != 0 {
		t.Fatalf("expected no diff_error responses, got %+v", responses)
	}
	if countResponsesByType(responses, "done") != 1 {
		t.Fatalf("expected one done response, got %+v", responses)
	}

	gotOutput, err := appState.GetOutputFile("src/complex.c")
	if err != nil {
		t.Fatalf("GetOutputFile failed: %v", err)
	}
	if gotOutput != expectedContent {
		t.Fatalf("unexpected output content: %q", gotOutput)
	}
}

func TestHandleSendIntegrationSRMultiMidBatchFailureIsAtomicAndSingleToolCallRecorded(t *testing.T) {
	baseContent := "L01\nL02\nL03\nL04\nL05\nL06\nL07\nL08\n"
	baseFileID := state.HashFileVersion("src/big.c", baseContent)
	edits := []map[string]any{
		{
			"path":        "src/big.c",
			"file_id":     baseFileID,
			"old_string":  "L01",
			"new_string":  "L01_OK",
			"replace_all": false,
		},
		{
			"path":        "src/big.c",
			"file_id":     baseFileID,
			"old_string":  "L02",
			"new_string":  "L02_OK",
			"replace_all": false,
		},
		{
			"path":        "src/big.c",
			"file_id":     baseFileID,
			"old_string":  "MISSING_LINE",
			"new_string":  "SHOULD_FAIL",
			"replace_all": false,
		},
		{
			"path":        "src/big.c",
			"file_id":     baseFileID,
			"old_string":  "L04",
			"new_string":  "L04_OK",
			"replace_all": false,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
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
					"delta": map[string]any{"content": "Attempting large patch."},
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
								"id":    "call_sr_multi_failure_mid_batch",
								"type":  "function",
								"function": map[string]any{
									"name":      "edit_file",
									"arguments": editFileMultiArgsJSON(t, edits),
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
	diffMode := "search_replace_multi"
	appConfig.DiffMode = &diffMode
	if err := appState.ContextAdd("src/big.c", baseContent); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	reqID := "req-send-sr-multi-mid-batch-failure"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}
	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content": "Apply risky large batch",
			"model":   "test-model",
		})
	})

	if countResponsesByType(responses, "done") != 0 {
		t.Fatalf("expected no done response on diff error, got %+v", responses)
	}
	diffErr := requireDiffErrorResponse(t, responses)
	errs, ok := diffErr["errors"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected non-empty diff_error errors, got %+v", diffErr)
	}
	firstErr, _ := errs[0].(string)
	if !strings.Contains(firstErr, "edit_file edit 2 (src/big.c") {
		t.Fatalf("expected failing edit index in error, got %q", firstErr)
	}
	if !strings.Contains(firstErr, "old_string not found in file") {
		t.Fatalf("expected old_string not found detail, got %q", firstErr)
	}

	toolCalls := requireToolCallEntries(t, diffErr)
	if len(toolCalls) != 1 {
		t.Fatalf("expected exactly one tool_call entry (one model call), got %+v", toolCalls)
	}
	if toolCalls[0]["tool"] != "edit_file" {
		t.Fatalf("unexpected tool entry: %+v", toolCalls[0])
	}
	if toolCalls[0]["path"] != "src/big.c" {
		t.Fatalf("unexpected tool path summary: %+v", toolCalls[0]["path"])
	}

	if _, err := appState.GetOutputFile("src/big.c"); err == nil {
		t.Fatalf("expected no output file written after failed batch (atomic rollback)")
	}
}

func TestHandleSendIntegrationSRMultiMissingFileIDInOneEditFails(t *testing.T) {
	baseContent := "A\nB\nC\n"
	baseFileID := state.HashFileVersion("src/ids.c", baseContent)
	edits := []map[string]any{
		{
			"path":        "src/ids.c",
			"file_id":     baseFileID,
			"old_string":  "A",
			"new_string":  "A1",
			"replace_all": false,
		},
		{
			"path":        "src/ids.c",
			"old_string":  "B",
			"new_string":  "B1",
			"replace_all": false,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
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
					"delta": map[string]any{"content": "Applying edits with one missing file id."},
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
								"id":    "call_sr_multi_missing_file_id_entry",
								"type":  "function",
								"function": map[string]any{
									"name":      "edit_file",
									"arguments": editFileMultiArgsJSON(t, edits),
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
	diffMode := "search_replace_multi"
	appConfig.DiffMode = &diffMode
	if err := appState.ContextAdd("src/ids.c", baseContent); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	reqID := "req-send-sr-multi-missing-file-id"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}
	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content": "Apply with missing file_id in one edit",
			"model":   "test-model",
		})
	})

	if countResponsesByType(responses, "done") != 0 {
		t.Fatalf("expected no done response on diff error, got %+v", responses)
	}
	diffErr := requireDiffErrorResponse(t, responses)
	errs, ok := diffErr["errors"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected non-empty diff_error errors, got %+v", diffErr)
	}
	firstErr, _ := errs[0].(string)
	if !strings.Contains(firstErr, "edit_file edit 1 (src/ids.c)") {
		t.Fatalf("expected failing edit index/path in error, got %q", firstErr)
	}
	if !strings.Contains(firstErr, "file_id missing") {
		t.Fatalf("expected file_id missing error detail, got %q", firstErr)
	}

	toolCalls := requireToolCallEntries(t, diffErr)
	if len(toolCalls) != 1 {
		t.Fatalf("expected exactly one tool_call entry, got %+v", toolCalls)
	}
	if _, err := appState.GetOutputFile("src/ids.c"); err == nil {
		t.Fatalf("expected no output file written after file_id validation error")
	}
}

func TestHandleSendIntegrationHiddenRetryAppliesPartialAndSucceeds(t *testing.T) {
	baseContent := "L01\nL02\nL03\nL04\n"
	baseFileID := state.HashFileVersion("src/retry.c", baseContent)
	firstEdits := []map[string]any{
		{
			"path":        "src/retry.c",
			"file_id":     baseFileID,
			"old_string":  "L01",
			"new_string":  "L01_OK",
			"replace_all": false,
		},
		{
			"path":        "src/retry.c",
			"file_id":     baseFileID,
			"old_string":  "MISSING_LINE",
			"new_string":  "NOPE",
			"replace_all": false,
		},
	}
	partialContent, err := diff.Replace(baseContent, "L01", "L01_OK", false)
	if err != nil {
		t.Fatalf("failed to prepare partial content: %v", err)
	}
	partialFileID := state.HashFileVersion("src/retry.c", partialContent)
	secondEdits := []map[string]any{
		{
			"path":        "src/retry.c",
			"file_id":     partialFileID,
			"old_string":  "L02",
			"new_string":  "L02_OK",
			"replace_all": false,
		},
		{
			"path":        "src/retry.c",
			"file_id":     partialFileID,
			"old_string":  "L03",
			"new_string":  "L03_OK",
			"replace_all": false,
		},
	}
	expectedContent := "L01_OK\nL02_OK\nL03_OK\nL04\n"

	streamCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}

		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var reqBody map[string]any
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request failed: %v", err)
		}

		stream, _ := reqBody["stream"].(bool)
		if !stream {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{
					map[string]any{"message": map[string]any{"content": "Test Title"}},
				},
			})
			return
		}

		streamCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		switch streamCalls {
		case 1:
			if _, ok := reqBody["reasoning"]; !ok {
				t.Fatalf("expected reasoning on initial request")
			}
			writeSSEJSON(t, w, map[string]any{
				"choices": []any{
					map[string]any{"delta": map[string]any{"content": "Applying edits."}},
				},
			})
			writeSSEJSON(t, w, map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"tool_calls": []any{
								map[string]any{
									"index": 0,
									"id":    "call_first_retry_attempt",
									"type":  "function",
									"function": map[string]any{
										"name":      "edit_file",
										"arguments": editFileMultiArgsJSON(t, firstEdits),
									},
								},
							},
						},
					},
				},
			})
			writeSSEJSON(t, w, map[string]any{
				"usage": map[string]any{
					"prompt_tokens":     10,
					"completion_tokens": 3,
					"cached_tokens":     2,
					"total_tokens":      13,
					"cost":              0.1,
				},
			})
			writeSSEDone(t, w)
		case 2:
			if _, ok := reqBody["reasoning"]; ok {
				t.Fatalf("retry request must disable reasoning")
			}
			messagesRaw, ok := reqBody["messages"].([]any)
			if !ok || len(messagesRaw) < 2 {
				t.Fatalf("unexpected retry messages payload: %+v", reqBody["messages"])
			}
			lastMessage, ok := messagesRaw[len(messagesRaw)-1].(map[string]any)
			if !ok {
				t.Fatalf("unexpected retry user message payload: %+v", messagesRaw[len(messagesRaw)-1])
			}
			retryUserContent, _ := lastMessage["content"].(string)
			if !strings.Contains(retryUserContent, "@retry_context") {
				t.Fatalf("expected retry_context block in hidden retry request, got:\n%s", retryUserContent)
			}
			if !strings.Contains(retryUserContent, "L01_OK") {
				t.Fatalf("expected writable retry content with partial edits, got:\n%s", retryUserContent)
			}
			writeSSEJSON(t, w, map[string]any{
				"choices": []any{
					map[string]any{"delta": map[string]any{"content": "Retrying remaining edits."}},
				},
			})
			writeSSEJSON(t, w, map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"tool_calls": []any{
								map[string]any{
									"index": 0,
									"id":    "call_second_retry_attempt",
									"type":  "function",
									"function": map[string]any{
										"name":      "edit_file",
										"arguments": editFileMultiArgsJSON(t, secondEdits),
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
					"completion_tokens": 4,
					"cached_tokens":     3,
					"total_tokens":      16,
					"cost":              0.2,
				},
			})
			writeSSEDone(t, w)
		default:
			t.Fatalf("unexpected number of streaming calls: %d", streamCalls)
		}
	}))
	defer server.Close()

	setupSendIntegrationEnv(t, server.URL)
	diffMode := "search_replace_multi"
	autoRetry := true
	appConfig.DiffMode = &diffMode
	appConfig.AutoRetryPartialEdits = &autoRetry
	if err := appState.ContextAdd("src/retry.c", baseContent); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	reqID := "req-send-hidden-retry-success"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}
	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content":          "Apply edits with hidden retry",
			"model":            "test-model",
			"reasoning_effort": "high",
		})
	})

	if streamCalls != 2 {
		t.Fatalf("expected exactly two streaming calls (initial + retry), got %d", streamCalls)
	}
	if countResponsesByType(responses, "diff_error") != 0 {
		t.Fatalf("expected no diff_error responses, got %+v", responses)
	}
	if countResponsesByType(responses, "done") != 1 {
		t.Fatalf("expected one done response, got %+v", responses)
	}

	reattemptSeen := false
	for _, resp := range responses {
		if respType, _ := resp["type"].(string); respType == "chunk" {
			content, _ := resp["content"].(string)
			if strings.Contains(content, "Reattempting to apply file changes") {
				reattemptSeen = true
				break
			}
		}
	}
	if !reattemptSeen {
		t.Fatalf("expected visible reattempt status chunk, got %+v", responses)
	}

	done := firstResponseByType(responses, "done")
	usage, ok := done["usage"].(map[string]any)
	if !ok {
		t.Fatalf("expected done usage, got %+v", done)
	}
	if usage["prompt_tokens"] != float64(22) {
		t.Fatalf("unexpected merged prompt_tokens: %+v", usage["prompt_tokens"])
	}
	if usage["completion_tokens"] != float64(7) {
		t.Fatalf("unexpected merged completion_tokens: %+v", usage["completion_tokens"])
	}
	if usage["cached_tokens"] != float64(5) {
		t.Fatalf("unexpected merged cached_tokens: %+v", usage["cached_tokens"])
	}
	if usage["total_tokens"] != float64(29) {
		t.Fatalf("unexpected merged total_tokens: %+v", usage["total_tokens"])
	}

	gotOutput, err := appState.GetOutputFile("src/retry.c")
	if err != nil {
		t.Fatalf("GetOutputFile failed: %v", err)
	}
	if gotOutput != expectedContent {
		t.Fatalf("unexpected output content: %q", gotOutput)
	}
}

func TestHandleSendIntegrationHiddenRetryFailureFallsBackToDiffError(t *testing.T) {
	baseContent := "A1\nA2\nA3\n"
	baseFileID := state.HashFileVersion("src/retry-fail.c", baseContent)
	firstEdits := []map[string]any{
		{
			"path":        "src/retry-fail.c",
			"file_id":     baseFileID,
			"old_string":  "A1",
			"new_string":  "A1_OK",
			"replace_all": false,
		},
		{
			"path":        "src/retry-fail.c",
			"file_id":     baseFileID,
			"old_string":  "NO_MATCH",
			"new_string":  "NOPE",
			"replace_all": false,
		},
	}
	partialContent, err := diff.Replace(baseContent, "A1", "A1_OK", false)
	if err != nil {
		t.Fatalf("failed to prepare partial content: %v", err)
	}
	partialFileID := state.HashFileVersion("src/retry-fail.c", partialContent)
	secondEdits := []map[string]any{
		{
			"path":        "src/retry-fail.c",
			"file_id":     partialFileID,
			"old_string":  "STILL_MISSING",
			"new_string":  "FAIL_AGAIN",
			"replace_all": false,
		},
	}

	streamCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}

		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var reqBody map[string]any
		if err := json.Unmarshal(body, &reqBody); err != nil {
			t.Fatalf("unmarshal request failed: %v", err)
		}

		stream, _ := reqBody["stream"].(bool)
		if !stream {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"choices": []any{
					map[string]any{"message": map[string]any{"content": "Test Title"}},
				},
			})
			return
		}

		streamCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		switch streamCalls {
		case 1:
			writeSSEJSON(t, w, map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"tool_calls": []any{
								map[string]any{
									"index": 0,
									"id":    "call_retry_failure_first",
									"type":  "function",
									"function": map[string]any{
										"name":      "edit_file",
										"arguments": editFileMultiArgsJSON(t, firstEdits),
									},
								},
							},
						},
					},
				},
			})
			writeSSEDone(t, w)
		case 2:
			writeSSEJSON(t, w, map[string]any{
				"choices": []any{
					map[string]any{
						"delta": map[string]any{
							"tool_calls": []any{
								map[string]any{
									"index": 0,
									"id":    "call_retry_failure_second",
									"type":  "function",
									"function": map[string]any{
										"name":      "edit_file",
										"arguments": editFileMultiArgsJSON(t, secondEdits),
									},
								},
							},
						},
					},
				},
			})
			writeSSEDone(t, w)
		default:
			t.Fatalf("unexpected number of streaming calls: %d", streamCalls)
		}
	}))
	defer server.Close()

	setupSendIntegrationEnv(t, server.URL)
	diffMode := "search_replace_multi"
	autoRetry := true
	appConfig.DiffMode = &diffMode
	appConfig.AutoRetryPartialEdits = &autoRetry
	if err := appState.ContextAdd("src/retry-fail.c", baseContent); err != nil {
		t.Fatalf("ContextAdd failed: %v", err)
	}

	reqID := "req-send-hidden-retry-failure"
	if !reserveActiveStream(reqID) {
		t.Fatal("failed to reserve active stream")
	}
	responses := captureJSONResponses(t, func() {
		handleSend(reqID, map[string]any{
			"content": "Apply edits and fail hidden retry",
			"model":   "test-model",
		})
	})

	if streamCalls != 2 {
		t.Fatalf("expected two streaming calls (initial + retry), got %d", streamCalls)
	}
	if countResponsesByType(responses, "done") != 0 {
		t.Fatalf("expected no done response, got %+v", responses)
	}
	diffErr := requireDiffErrorResponse(t, responses)
	errs, ok := diffErr["errors"].([]any)
	if !ok || len(errs) < 2 {
		t.Fatalf("expected errors from initial and retry attempts, got %+v", diffErr)
	}
	firstErr, _ := errs[0].(string)
	lastErr, _ := errs[len(errs)-1].(string)
	if !strings.Contains(firstErr, "old_string not found in file") {
		t.Fatalf("expected original diff error detail, got %q", firstErr)
	}
	if !strings.Contains(lastErr, "retry attempt:") {
		t.Fatalf("expected retry-attempt prefix in errors, got %q", lastErr)
	}

	toolCalls := requireToolCallEntries(t, diffErr)
	if len(toolCalls) != 2 {
		t.Fatalf("expected two tool call log entries across attempts, got %+v", toolCalls)
	}
	if _, err := appState.GetOutputFile("src/retry-fail.c"); err == nil {
		t.Fatalf("expected no output file written after retry failure")
	}
}
