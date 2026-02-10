package main

import (
	"context"
	"reflect"
	"testing"
)

func resetActiveStreamForTest() {
	activeStream = streamState{}
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name string
		req  map[string]any
		want string
	}{
		{name: "string", req: map[string]any{"request_id": "abc"}, want: "abc"},
		{name: "int", req: map[string]any{"request_id": 42}, want: "42"},
		{name: "float", req: map[string]any{"request_id": 42.0}, want: "42"},
		{name: "none", req: map[string]any{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestID(tt.req); got != tt.want {
				t.Fatalf("requestID(%v) = %q, want %q", tt.req, got, tt.want)
			}
		})
	}
}

func TestAddResponseID(t *testing.T) {
	data := map[string]any{"type": "ok"}
	out := addResponseID("req-1", data)
	if got := out["request_id"]; got != "req-1" {
		t.Fatalf("request_id = %v, want %q", got, "req-1")
	}

	// Ensure empty id leaves map unchanged
	orig := map[string]any{"type": "ok"}
	out2 := addResponseID("", orig)
	if !reflect.DeepEqual(out2, orig) {
		t.Fatalf("expected map unchanged when id is empty")
	}
}

func TestReserveActiveStream(t *testing.T) {
	resetActiveStreamForTest()
	t.Cleanup(resetActiveStreamForTest)

	if !reserveActiveStream("req-1") {
		t.Fatalf("expected first reservation to succeed")
	}
	if reserveActiveStream("req-2") {
		t.Fatalf("expected second reservation to fail while active")
	}
	if !hasActiveStream() {
		t.Fatalf("expected active stream after reservation")
	}

	clearActiveStream("req-1")
	if hasActiveStream() {
		t.Fatalf("expected no active stream after clear")
	}
}

func TestCancelReservedStreamWithoutCancelFunc(t *testing.T) {
	resetActiveStreamForTest()
	t.Cleanup(resetActiveStreamForTest)

	if !reserveActiveStream("req-1") {
		t.Fatalf("failed to reserve stream")
	}
	if !cancelActiveStream("req-1") {
		t.Fatalf("expected cancel to succeed for reserved stream")
	}
	if !wasStreamCanceled("req-1") {
		t.Fatalf("expected canceled flag to be set")
	}
}

func TestSetActiveStreamCancelAndCancel(t *testing.T) {
	resetActiveStreamForTest()
	t.Cleanup(resetActiveStreamForTest)

	if !reserveActiveStream("req-1") {
		t.Fatalf("failed to reserve stream")
	}

	called := false
	if !setActiveStreamCancel("req-1", func() {
		called = true
	}) {
		t.Fatalf("expected cancel func to be set")
	}
	if !cancelActiveStream("req-1") {
		t.Fatalf("expected cancel to succeed")
	}
	if !called {
		t.Fatalf("expected cancel function to be invoked")
	}
}

func TestSetActiveStreamCancelRejectsMismatchedRequest(t *testing.T) {
	resetActiveStreamForTest()
	t.Cleanup(resetActiveStreamForTest)

	if !reserveActiveStream("req-1") {
		t.Fatalf("failed to reserve stream")
	}
	if setActiveStreamCancel("req-2", func() {}) {
		t.Fatalf("expected mismatched request ID to be rejected")
	}
}

func TestActionMutatesChatState(t *testing.T) {
	mutating := []string{
		"chat_new",
		"chat_select",
		"chat_edit",
		"chat_delete",
		"chat_rename",
		"fork_chat",
		"save_draft",
		"context_add",
		"context_add_section",
		"context_update",
		"context_set_readonly",
		"context_remove",
		"context_remove_section",
		"output_delete",
		"apply_file",
		"apply_file_as",
		"generate_title",
	}

	for _, action := range mutating {
		if !actionMutatesChatState(action) {
			t.Fatalf("expected action %q to be mutating", action)
		}
	}

	nonMutating := []string{
		"ping",
		"version",
		"chat_get",
		"chat_list",
		"context_list",
		"get_file_statuses",
		"send",
		"cancel",
	}

	for _, action := range nonMutating {
		if actionMutatesChatState(action) {
			t.Fatalf("expected action %q to be non-mutating", action)
		}
	}
}

func TestActionUsesChatState(t *testing.T) {
	stateActions := []string{
		"bb7_init",
		"init",
		"chat_get",
		"chat_list",
		"context_list",
		"estimate_tokens",
		"send",
		"generate_title",
		"prepare_instructions",
	}

	for _, action := range stateActions {
		if !actionUsesChatState(action) {
			t.Fatalf("expected action %q to use state", action)
		}
	}

	nonStateActions := []string{
		"ping",
		"version",
		"get_models",
		"get_balance",
		"cancel",
		"shutdown",
		"unknown_action",
	}

	for _, action := range nonStateActions {
		if actionUsesChatState(action) {
			t.Fatalf("expected action %q to not use state", action)
		}
	}
}

func TestSetTerminalStreamError(t *testing.T) {
	canceled := 0
	cancel := func() { canceled++ }
	streamErr := ""

	setTerminalStreamError(&streamErr, "duplicate write", cancel)
	if streamErr != "duplicate write" {
		t.Fatalf("expected streamErr to be set, got %q", streamErr)
	}
	if canceled != 1 {
		t.Fatalf("expected cancel to be called once, got %d", canceled)
	}

	setTerminalStreamError(&streamErr, "later error", cancel)
	if streamErr != "duplicate write" {
		t.Fatalf("expected first stream error to be preserved, got %q", streamErr)
	}
	if canceled != 2 {
		t.Fatalf("expected cancel to be called on each terminal signal, got %d", canceled)
	}
}

func TestSetTerminalStreamErrorNoOpCases(t *testing.T) {
	streamErr := ""
	setTerminalStreamError(&streamErr, "", func() {
		t.Fatal("cancel should not be called for empty messages")
	})
	if streamErr != "" {
		t.Fatalf("expected empty streamErr, got %q", streamErr)
	}

	called := false
	setTerminalStreamError(nil, "ignored", func() {
		called = true
	})
	if called {
		t.Fatal("cancel should not be called for nil streamErr pointer")
	}

	// Ensure nil cancel does not panic.
	setTerminalStreamError(&streamErr, "set without cancel", nil)
	if streamErr != "set without cancel" {
		t.Fatalf("expected streamErr to be set without cancel func, got %q", streamErr)
	}
}

func TestSetTerminalStreamErrorWithCanceledContext(t *testing.T) {
	streamErr := ""
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setTerminalStreamError(&streamErr, "terminal", cancel)
	if streamErr != "terminal" {
		t.Fatalf("expected streamErr to be set, got %q", streamErr)
	}
	if ctx.Err() == nil {
		t.Fatal("expected context to be canceled")
	}
}
