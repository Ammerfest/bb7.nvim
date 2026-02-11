package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/youruser/bb7/internal/config"
	"github.com/youruser/bb7/internal/llm"
	"github.com/youruser/bb7/internal/logging"
	"github.com/youruser/bb7/internal/state"
)

//go:embed system_prompt.txt
var systemPrompt string

//go:embed title_prompt.txt
var titlePrompt string

//go:embed version.txt
var version string

var (
	appState  = state.New()
	appConfig *config.Config
	llmClient *llm.Client
	log       = logging.Get()

	llmMsgLogMu   sync.Mutex
	llmMsgLogFile *os.File
	respondMu     sync.Mutex
	configMu      sync.Mutex
	stateMu       sync.Mutex
)

const markerLen = 60

type streamState struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	requestID string
	canceled  bool
}

var activeStream streamState

func makeMarker(label string, ch rune) string {
	text := " " + label + " "
	if len(text) >= markerLen {
		return text[:markerLen]
	}
	pad := markerLen - len(text)
	left := pad / 2
	right := pad - left
	return strings.Repeat(string(ch), left) + text + strings.Repeat(string(ch), right)
}

func main() {
	// Handle --version flag
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("bb7 %s\n", strings.TrimSpace(version))
		return
	}

	// Debug: show if BB7_DEBUG is set
	if os.Getenv("BB7_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "BB-7: process started with BB7_DEBUG=1\n")
	}
	logBuildInfo()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		handleRequest(line)
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			respond("", map[string]any{
				"type":    "error",
				"message": "Request too large (max 1MB). Reduce context size or split the request.",
			})
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "stdin error: %v\n", err)
		os.Exit(1)
	}
}

func logBuildInfo() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		log.Info("Build info: unavailable")
		return
	}

	var revision string
	var buildTime string
	var modified string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.time":
			buildTime = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}

	version := info.Main.Version
	if revision != "" {
		version = revision
	}
	if modified == "true" {
		version += " (modified)"
	}

	if buildTime != "" {
		log.Info("Build: %s; go=%s; time=%s", version, runtime.Version(), buildTime)
		return
	}
	log.Info("Build: %s; go=%s", version, runtime.Version())
}

func ensureLLMMessageLogFile() (*os.File, error) {
	if !log.Enabled() {
		return nil, nil
	}
	llmMsgLogMu.Lock()
	defer llmMsgLogMu.Unlock()
	if llmMsgLogFile != nil {
		return llmMsgLogFile, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	logsDir := filepath.Join(home, ".bb7", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logsDir, "llm-debug.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	llmMsgLogFile = file
	return llmMsgLogFile, nil
}

func logLLMMessage(kind, content, chatID, model string) {
	if !log.Enabled() || content == "" {
		return
	}
	file, err := ensureLLMMessageLogFile()
	if err != nil || file == nil {
		log.Error("Failed to open LLM message log: %v", err)
		return
	}
	sep := makeMarker(strings.ToLower(kind)+" message", '=')
	header := fmt.Sprintf("ts=%s chat_id=%s model=%s\n", time.Now().UTC().Format(time.RFC3339Nano), chatID, model)
	llmMsgLogMu.Lock()
	defer llmMsgLogMu.Unlock()
	_, _ = file.WriteString(sep + "\n")
	_, _ = file.WriteString(header)
	_, _ = file.WriteString(content)
	if !strings.HasSuffix(content, "\n") {
		_, _ = file.WriteString("\n")
	}
	_, _ = file.WriteString("\n")
}

// ensureConfig loads config lazily on first use.
func ensureConfig() error {
	configMu.Lock()
	defer configMu.Unlock()

	if appConfig != nil {
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	appConfig = cfg
	llmClient = llm.NewClient(cfg.BaseURL, cfg.APIKey)
	return nil
}

func reserveActiveStream(reqID string) bool {
	activeStream.mu.Lock()
	defer activeStream.mu.Unlock()
	if activeStream.requestID != "" {
		return false
	}
	activeStream.requestID = reqID
	activeStream.cancel = nil
	activeStream.canceled = false
	return true
}

func setActiveStreamCancel(reqID string, cancel context.CancelFunc) bool {
	activeStream.mu.Lock()
	defer activeStream.mu.Unlock()
	if activeStream.requestID != reqID {
		return false
	}
	activeStream.cancel = cancel
	return true
}

func clearActiveStream(reqID string) {
	activeStream.mu.Lock()
	defer activeStream.mu.Unlock()
	if activeStream.requestID != reqID {
		return
	}
	activeStream.requestID = ""
	activeStream.cancel = nil
	activeStream.canceled = false
}

func cancelActiveStream(targetID string) bool {
	activeStream.mu.Lock()
	if activeStream.requestID == "" {
		activeStream.mu.Unlock()
		return false
	}
	if targetID != "" && activeStream.requestID != targetID {
		activeStream.mu.Unlock()
		return false
	}
	cancel := activeStream.cancel
	activeStream.canceled = true
	activeStream.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return true
}

func wasStreamCanceled(reqID string) bool {
	activeStream.mu.Lock()
	defer activeStream.mu.Unlock()
	return activeStream.requestID == reqID && activeStream.canceled
}

func hasActiveStream() bool {
	activeStream.mu.Lock()
	defer activeStream.mu.Unlock()
	return activeStream.requestID != ""
}

func setTerminalStreamError(streamErr *string, msg string, cancel context.CancelFunc) {
	if streamErr == nil || msg == "" {
		return
	}
	if *streamErr == "" {
		*streamErr = msg
	}
	if cancel != nil {
		cancel()
	}
}

func actionMutatesChatState(action string) bool {
	switch action {
	case "chat_new",
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
		"generate_title":
		return true
	default:
		return false
	}
}

func actionUsesChatState(action string) bool {
	switch action {
	case "bb7_init",
		"init",
		"chat_new",
		"chat_list",
		"search_chats",
		"chat_select",
		"chat_get",
		"chat_edit",
		"chat_delete",
		"chat_active",
		"chat_rename",
		"fork_chat",
		"save_draft",
		"context_add",
		"context_add_section",
		"context_update",
		"context_set_readonly",
		"context_remove",
		"context_remove_section",
		"context_list",
		"get_context_file",
		"get_output_file",
		"output_delete",
		"get_diff_paths",
		"get_file_statuses",
		"apply_file",
		"apply_file_as",
		"estimate_tokens",
		"send",
		"generate_title",
		"get_customization_info",
		"prepare_instructions":
		return true
	default:
		return false
	}
}

// actionBlockedDuringStream returns true for actions that mutate chat state
// in ways that conflict with an active streaming goroutine. Read-only actions
// and idempotent init are allowed through so the UI can reopen mid-stream.
func actionBlockedDuringStream(action string) bool {
	switch action {
	case "chat_new",
		"chat_select",
		"chat_delete",
		"chat_edit",
		"fork_chat",
		"context_add",
		"context_add_section",
		"context_update",
		"context_set_readonly",
		"context_remove",
		"context_remove_section",
		"apply_file",
		"apply_file_as",
		"output_delete",
		"save_draft",
		"prepare_instructions":
		return true
	default:
		return false
	}
}

func handleRequest(line string) {
	var req map[string]any
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		log.Error("Invalid JSON request: %s", line)
		respond("", map[string]any{"type": "error", "message": "Invalid JSON"})
		return
	}

	action, _ := req["action"].(string)
	log.Request(action, line)
	reqID := requestID(req)

	if hasActiveStream() && actionBlockedDuringStream(action) {
		respond(reqID, map[string]any{"type": "error", "message": "Another request is already in progress"})
		return
	}

	if actionUsesChatState(action) {
		stateMu.Lock()
		defer stateMu.Unlock()
	}

	switch action {
	case "ping":
		respond(reqID, map[string]any{"type": "ok"})

	case "version":
		respond(reqID, map[string]any{"type": "version", "version": strings.TrimSpace(version)})

	case "bb7_init":
		projectRoot, _ := req["project_root"].(string)
		if projectRoot == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: project_root"})
			return
		}
		if err := appState.ProjectInit(projectRoot); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "init":
		projectRoot, _ := req["project_root"].(string)
		if projectRoot == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: project_root"})
			return
		}
		if err := appState.Init(projectRoot); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "chat_new":
		name, _ := req["name"].(string)
		chat, err := appState.ChatNew(name)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok", "id": chat.ID})

	case "chat_list":
		chats, err := appState.ChatList()
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "chat_list", "chats": chats})

	case "search_chats":
		query, _ := req["query"].(string)
		results, err := appState.SearchChats(query)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "search_results", "results": results})

	case "chat_select":
		id, _ := req["id"].(string)
		if id == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: id"})
			return
		}
		if _, err := appState.ChatSelect(id); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "chat_get":
		if appState.ActiveChat == nil {
			respond(reqID, errorResponse(state.ErrNoActiveChat))
			return
		}
		respond(reqID, map[string]any{
			"type":              "chat",
			"id":                appState.ActiveChat.ID,
			"name":              appState.ActiveChat.Name,
			"created":           appState.ActiveChat.Created,
			"model":             appState.ActiveChat.Model,
			"draft":             appState.ActiveChat.Draft,
			"messages":          appState.ActiveChat.Messages,
			"instructions_info": appState.GetInstructionsInfo(),
		})

	case "chat_edit":
		if appState.ActiveChat == nil {
			respond(reqID, errorResponse(state.ErrNoActiveChat))
			return
		}
		if chatID, ok := req["chat_id"].(string); ok && chatID != "" {
			if appState.ActiveChat.ID != chatID {
				respond(reqID, map[string]any{"type": "error", "message": "Chat is not active"})
				return
			}
		}
		indexVal, ok := req["message_index"].(float64)
		if !ok {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: message_index"})
			return
		}
		content, _ := req["content"].(string)

		warnings, err := appState.EditUserMessage(int(indexVal), content)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok", "context_warnings": warnings})

	case "chat_delete":
		id, _ := req["id"].(string)
		if id == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: id"})
			return
		}
		if err := appState.ChatDelete(id); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "chat_active":
		if !appState.Initialized() {
			respond(reqID, errorResponse(state.ErrNotInitialized))
			return
		}
		var chatID any
		if appState.ActiveChat != nil {
			chatID = appState.ActiveChat.ID
		}
		respond(reqID, map[string]any{"type": "chat_active", "id": chatID})

	case "chat_rename":
		id, _ := req["id"].(string)
		if id == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: id"})
			return
		}
		name, _ := req["name"].(string)
		if name == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: name"})
			return
		}
		if err := appState.ChatRename(id, name); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "fork_chat":
		chatID, _ := req["chat_id"].(string)
		if chatID == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: chat_id"})
			return
		}
		forkIndexFloat, ok := req["fork_message_index"].(float64)
		if !ok {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: fork_message_index"})
			return
		}
		forkIndex := int(forkIndexFloat)

		result, err := appState.ForkChat(chatID, forkIndex)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}

		respond(reqID, map[string]any{
			"type":                 "fork_result",
			"new_chat_id":          result.NewChatID,
			"fork_message_content": result.ForkMessageContent,
			"context_warnings":     result.ContextWarnings,
		})

	case "save_draft":
		if appState.ActiveChat == nil {
			respond(reqID, errorResponse(state.ErrNoActiveChat))
			return
		}
		draft, _ := req["draft"].(string)
		appState.ActiveChat.Draft = draft
		if err := appState.SaveActiveChat(); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "context_add":
		path, _ := req["path"].(string)
		content, _ := req["content"].(string)
		readOnly, _ := req["readonly"].(bool)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		if err := appState.ContextAddWithReadOnly(path, content, readOnly); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "context_add_section":
		path, _ := req["path"].(string)
		content, _ := req["content"].(string)
		startLine, _ := req["start_line"].(float64)
		endLine, _ := req["end_line"].(float64)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		if startLine <= 0 || endLine <= 0 {
			respond(reqID, map[string]any{"type": "error", "message": "start_line and end_line must be positive integers"})
			return
		}
		if int(startLine) > int(endLine) {
			respond(reqID, map[string]any{"type": "error", "message": fmt.Sprintf("start_line (%d) cannot be greater than end_line (%d)", int(startLine), int(endLine))})
			return
		}
		if err := appState.ContextAddSection(path, int(startLine), int(endLine), content); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "context_update":
		path, _ := req["path"].(string)
		content, _ := req["content"].(string)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		if err := appState.ContextUpdate(path, content); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "context_set_readonly":
		path, _ := req["path"].(string)
		readOnly, ok := req["readonly"].(bool)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		if !ok {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: readonly"})
			return
		}
		if err := appState.ContextSetReadOnly(path, readOnly); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "context_remove":
		path, _ := req["path"].(string)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		if err := appState.ContextRemove(path); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "context_remove_section":
		path, _ := req["path"].(string)
		startLine, _ := req["start_line"].(float64)
		endLine, _ := req["end_line"].(float64)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		if startLine <= 0 || endLine <= 0 {
			respond(reqID, map[string]any{"type": "error", "message": "start_line and end_line must be positive integers"})
			return
		}
		if err := appState.ContextRemoveSection(path, int(startLine), int(endLine)); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "context_list":
		files, err := appState.ContextList()
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "context_list", "files": files})

	case "get_context_file":
		path, _ := req["path"].(string)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		content, err := appState.GetContextFile(path)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "file_content", "path": path, "content": content})

	case "get_output_file":
		path, _ := req["path"].(string)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		content, err := appState.GetOutputFile(path)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "file_content", "path": path, "content": content})

	case "output_delete":
		path, _ := req["path"].(string)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		if err := appState.DeleteOutputFile(path); err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		if err := appState.UserRejectOutput(path); err != nil {
			log.Error("Failed to record output rejection for %s: %v", path, err)
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "get_diff_paths":
		path, _ := req["path"].(string)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		outputPath, err := appState.GetOutputPath(path)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		localPath, err := appState.GetLocalPath(path)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{
			"type":        "diff_paths",
			"path":        path,
			"output_path": outputPath,
			"local_path":  localPath,
		})

	case "get_file_statuses":
		statuses, err := appState.GetFileStatuses()
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "file_statuses", "files": statuses})

	case "apply_file":
		path, _ := req["path"].(string)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		content, err := appState.ApplyFile(path)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok", "content": content})

	case "apply_file_as":
		path, _ := req["path"].(string)
		destination, _ := req["destination"].(string)
		if path == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: path"})
			return
		}
		if destination == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: destination"})
			return
		}
		content, err := appState.ApplyFileAs(path, destination)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "ok", "content": content})

	case "get_balance":
		go handleGetBalance(reqID)

	case "get_models":
		go handleGetModels(reqID)

	case "estimate_tokens":
		handleEstimateTokens(reqID)

	case "send":
		if !reserveActiveStream(reqID) {
			respond(reqID, map[string]any{"type": "error", "message": "Another request is already in progress"})
			return
		}
		go handleSend(reqID, req)

	case "generate_title":
		handleGenerateTitle(reqID, req)

	case "cancel":
		targetID, _ := req["target_request_id"].(string)
		if !cancelActiveStream(targetID) {
			respond(reqID, map[string]any{"type": "error", "message": "No active request to cancel"})
			return
		}
		respond(reqID, map[string]any{"type": "ok"})

	case "shutdown":
		os.Exit(0)

	case "get_customization_info":
		info := appState.GetInstructionsInfo()
		globalValid := info.GlobalExists
		projectValid := info.ProjectExists && info.ProjectError == ""
		systemOverride := false
		if homeDir, err := os.UserHomeDir(); err == nil {
			overridePath := filepath.Join(homeDir, ".config", "bb7", "system_prompt.txt")
			if _, err := os.Stat(overridePath); err == nil {
				systemOverride = true
			}
		}
		respond(reqID, map[string]any{
			"type":                       "customization_info",
			"system_override":            systemOverride,
			"global_instructions":        globalValid,
			"project_instructions":       projectValid,
			"project_instructions_error": info.ProjectError,
		})

	case "prepare_instructions":
		level, _ := req["level"].(string)
		if level == "" {
			respond(reqID, map[string]any{"type": "error", "message": "Missing required field: level"})
			return
		}
		path, err := appState.PrepareInstructionsFile(level, systemPrompt)
		if err != nil {
			respond(reqID, errorResponse(err))
			return
		}
		respond(reqID, map[string]any{"type": "instructions_path", "path": path})

	default:
		respond(reqID, map[string]any{"type": "error", "message": fmt.Sprintf("Unknown action: %s", action)})
	}
}

func handleGetBalance(reqID string) {
	// Load config if needed
	if err := ensureConfig(); err != nil {
		respond(reqID, errorResponse(err))
		return
	}

	balance, err := llmClient.GetBalance()
	if err != nil {
		respond(reqID, errorResponse(err))
		return
	}

	respond(reqID, map[string]any{
		"type":          "balance",
		"total_credits": balance.Data.TotalCredits,
		"total_usage":   balance.Data.TotalUsage,
	})
}

func handleGetModels(reqID string) {
	// Load config if needed
	if err := ensureConfig(); err != nil {
		respond(reqID, errorResponse(err))
		return
	}

	models, err := llmClient.GetModels()
	if err != nil {
		respond(reqID, errorResponse(err))
		return
	}

	// Transform to simplified format for frontend
	var modelList []map[string]any
	for _, m := range models.Data {
		// Check supported parameters
		supportsReasoning := false
		supportsTools := false
		for _, param := range m.SupportedParameters {
			if param == "reasoning" {
				supportsReasoning = true
			}
			if param == "tools" {
				supportsTools = true
			}
		}
		modelList = append(modelList, map[string]any{
			"id":                 m.ID,
			"name":               m.Name,
			"context_length":     m.ContextLength,
			"supports_reasoning": supportsReasoning,
			"supports_tools":     supportsTools,
			"pricing": map[string]any{
				"prompt":     m.Pricing.Prompt,
				"completion": m.Pricing.Completion,
			},
		})
	}

	respond(reqID, map[string]any{
		"type":   "models",
		"models": modelList,
	})
}

func handleEstimateTokens(reqID string) {
	if appState.ActiveChat == nil {
		respond(reqID, errorResponse(state.ErrNoActiveChat))
		return
	}

	estimate, err := appState.EstimateTokens(systemPrompt)
	if err != nil {
		respond(reqID, errorResponse(err))
		return
	}

	respond(reqID, map[string]any{
		"type":              "token_estimate",
		"total":             estimate.Total,
		"context_files":     estimate.ContextFiles,
		"history":           estimate.History,
		"instructions":      estimate.Instructions,
		"system_prompt":     estimate.SystemPrompt,
		"files":             estimate.Files,
		"potential_savings": estimate.PotentialSavings,
	})
}

func handleSend(reqID string, req map[string]any) {
	defer clearActiveStream(reqID)

	if wasStreamCanceled(reqID) {
		respond(reqID, map[string]any{"type": "error", "message": "Response aborted by user."})
		return
	}

	content, _ := req["content"].(string)
	if content == "" {
		respond(reqID, map[string]any{"type": "error", "message": "Missing required field: content"})
		return
	}

	// Load config if needed
	if err := ensureConfig(); err != nil {
		respond(reqID, errorResponse(err))
		return
	}

	// Get reasoning config (optional)
	var reasoningConfig *llm.ReasoningConfig
	if reasoningEffort, ok := req["reasoning_effort"].(string); ok && reasoningEffort != "" {
		reasoningConfig = &llm.ReasoningConfig{Effort: reasoningEffort}
		log.Info("Reasoning enabled with effort: %s", reasoningEffort)
	}

	// Get model (from request or fall back to chat's model, then config default)
	model, _ := req["model"].(string)
	var instructionsBlock string
	var body string
	var err error
	var activeChatID string
	stateMu.Lock()
	if appState.ActiveChat == nil {
		stateMu.Unlock()
		respond(reqID, errorResponse(state.ErrNoActiveChat))
		return
	}
	if model == "" {
		model = appState.ActiveChat.Model
	}
	if model == "" {
		model = appConfig.DefaultModel
	}

	// Update chat's model to remember the selection
	appState.ActiveChat.Model = model

	// Build instructions block (fail fast if invalid)
	instructionsBlock, err = appState.BuildInstructionsBlock()
	if err != nil {
		stateMu.Unlock()
		respond(reqID, errorResponse(err))
		return
	}

	// Add user message
	if err := appState.AddUserMessage(content, model); err != nil {
		stateMu.Unlock()
		respond(reqID, errorResponse(err))
		return
	}

	// Build a single structured user message containing context, history, and latest input.
	body, err = buildLLMUserMessage()
	if err != nil {
		stateMu.Unlock()
		respond(reqID, errorResponse(err))
		return
	}
	activeChatID = appState.ActiveChat.ID
	stateMu.Unlock()

	// Track response
	var parts []state.MessagePart
	var textContent strings.Builder
	var thinkingContent strings.Builder
	var outputFiles []string
	var lastUsage *llm.Usage
	var toolCallArgs []string
	var writeCalls []llm.WriteFileArgs
	seenOutputPaths := make(map[string]bool)
	duplicatePathDetected := false

	// Build full system prompt with user/project instructions
	// Check for system prompt override (development feature)
	effectiveSystemPrompt := systemPrompt
	if homeDir, err := os.UserHomeDir(); err == nil {
		overridePath := filepath.Join(homeDir, ".config", "bb7", "system_prompt.txt")
		if content, err := os.ReadFile(overridePath); err == nil {
			stripped := state.StripComments(string(content))
			if strings.TrimSpace(stripped) != "" {
				effectiveSystemPrompt = stripped
				log.Info("Using system prompt override from %s", overridePath)
			}
		}
	}
	fullSystemPrompt := effectiveSystemPrompt
	if instructionsBlock != "" {
		fullSystemPrompt = effectiveSystemPrompt + "\n" + instructionsBlock
	}

	logLLMMessage("SYSTEM", fullSystemPrompt, activeChatID, model)
	logLLMMessage("USER", body, activeChatID, model)
	messages := []llm.Message{{
		Role:    "user",
		Content: body,
	}}

	ctx, cancel := context.WithCancel(context.Background())
	if !setActiveStreamCancel(reqID, cancel) {
		cancel()
		respond(reqID, map[string]any{"type": "error", "message": "Another request is already in progress"})
		return
	}
	defer func() {
		cancel()
	}()

	var streamErr string

	// Stream response
	log.Info("Starting LLM stream for model: %s", model)
	streamStart := time.Now()
	err = llmClient.ChatStream(ctx, model, fullSystemPrompt, messages, reasoningConfig, func(event llm.StreamEvent) {
		switch event.Type {
		case "content":
			// Regular text content - stream to UI and accumulate
			log.Stream("content", event.Content)
			textContent.WriteString(event.Content)
			respond(reqID, map[string]any{"type": "chunk", "content": event.Content})

		case "reasoning":
			// Reasoning/thinking content - stream to UI and accumulate separately
			log.Stream("reasoning", event.Reasoning)
			thinkingContent.WriteString(event.Reasoning)
			respond(reqID, map[string]any{"type": "thinking", "content": event.Reasoning})

		case "tool_call":
			if event.ToolCall == nil {
				return
			}

			log.ToolCall(event.ToolCall.Function.Name, event.ToolCall.Function.Arguments)
			if event.ToolCall.Function.Arguments != "" {
				toolCallArgs = append(toolCallArgs, event.ToolCall.Function.Arguments)
			}

			if event.ToolCall.Function.Name == "write_file" {
				args, err := llm.ParseWriteFileArgs(event.ToolCall.Function.Arguments)
				if err != nil {
					log.Error("Failed to parse write_file args: %v", err)
					return // Skip malformed tool calls
				}
				writeCalls = append(writeCalls, *args)
				if seenOutputPaths[args.Path] {
					log.Error("Duplicate write_file for path in single response: %s", args.Path)
					if !duplicatePathDetected {
						setTerminalStreamError(&streamErr, "Duplicate write_file for path in single response: "+args.Path, cancel)
						duplicatePathDetected = true
					}
					return
				}
				seenOutputPaths[args.Path] = true
				// Determine whether this is a new file or a modification of existing context.
				stateMu.Lock()
				inContext := appState.HasContextFile(args.Path)
				isNew := !inContext
				if err := appState.WriteOutputFile(args.Path, args.Content); err != nil {
					stateMu.Unlock()
					log.Error("Failed to write file %s: %v", args.Path, err)
					setTerminalStreamError(&streamErr, fmt.Sprintf("Failed to write file %s: %v", args.Path, err), cancel)
					return
				}
				stateMu.Unlock()
				// Don't call AssistantWriteFile here - we'll add file events as parts
				// of the main assistant message (after thinking/text) for correct ordering
				action := "Assistant modified"
				if isNew {
					action = "Assistant added"
				}
				log.Info("%s: %s (%d bytes)", action, args.Path, len(args.Content))
				outputFiles = append(outputFiles, args.Path)
				respond(reqID, map[string]any{"type": "chunk", "content": "\n[" + action + ": " + args.Path + "]\n"})
			}

		case "done":
			log.Stream("done", "")
			if event.Usage != nil {
				lastUsage = event.Usage
			}

		case "error":
			log.Error("Stream error: %s", event.Error)
			setTerminalStreamError(&streamErr, event.Error, cancel)
		}
	})

	if err == nil && streamErr != "" {
		err = errors.New(streamErr)
	}

	if err != nil {
		msg := streamErr
		if msg == "" {
			msg = err.Error()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			msg = "Request timed out."
		} else if errors.Is(err, context.Canceled) && wasStreamCanceled(reqID) {
			// Save any partial assistant response so the user and LLM can
			// refer to the incomplete answer in follow-up messages.
			if textContent.Len() > 0 || thinkingContent.Len() > 0 || len(writeCalls) > 0 {
				var cancelParts []state.MessagePart
				if thinkingContent.Len() > 0 {
					cancelParts = append(cancelParts, state.MessagePart{
						Type:    "thinking",
						Content: thinkingContent.String(),
					})
				}
				if textContent.Len() > 0 {
					cancelParts = append(cancelParts, state.MessagePart{
						Type:    "text",
						Content: textContent.String(),
					})
				}
				stateMu.Lock()
				for _, wc := range writeCalls {
					cf := appState.FindContextFile(wc.Path)
					readOnly := false
					external := false
					if cf != nil {
						readOnly = cf.ReadOnly
						external = cf.External
					}
					isNew := cf == nil
					cancelParts = append(cancelParts, state.MessagePart{
						Type:     "context_event",
						Action:   "AssistantWriteFile",
						Path:     wc.Path,
						ReadOnly: &readOnly,
						External: &external,
						Version:  state.HashFileVersion(wc.Path, wc.Content),
						Added:    isNew,
					})
				}
				stateMu.Unlock()
				stateMu.Lock()
				if addErr := appState.AddAssistantMessage("", cancelParts, outputFiles, model, nil); addErr != nil {
					log.Error("Failed to save partial assistant message: %v", addErr)
				} else if reasoningConfig != nil {
					msgs := appState.ActiveChat.Messages
					msgs[len(msgs)-1].ReasoningEffort = reasoningConfig.Effort
					if saveErr := appState.SaveActiveChat(); saveErr != nil {
						log.Error("Failed to save reasoning effort on partial message: %v", saveErr)
					}
				}
				stateMu.Unlock()
			}
			msg = "Response aborted by user."
		}
		stateMu.Lock()
		if addErr := appState.AddSystemMessage(msg); addErr != nil {
			log.Error("Failed to record system message: %v", addErr)
		}
		stateMu.Unlock()
		respond(reqID, map[string]any{"type": "error", "message": msg})
		return
	}

	var assistantRaw strings.Builder
	if thinkingContent.Len() > 0 {
		assistantRaw.WriteString(thinkingContent.String())
		if textContent.Len() > 0 {
			assistantRaw.WriteString("\n\n")
		}
	}
	if textContent.Len() > 0 {
		assistantRaw.WriteString(textContent.String())
	}
	if len(writeCalls) > 0 || len(toolCallArgs) > 0 {
		assistantRaw.WriteString("\n\n")
		assistantRaw.WriteString(makeMarker("tool calls", '-'))
		assistantRaw.WriteString("\n")
		for i, call := range writeCalls {
			header := fmt.Sprintf("@write_file index=%d path=%s", i, call.Path)
			assistantRaw.WriteString(header + "\n")
			if call.Content != "" {
				assistantRaw.WriteString(call.Content)
				if !strings.HasSuffix(call.Content, "\n") {
					assistantRaw.WriteString("\n")
				}
			}
			assistantRaw.WriteString("\n")
		}
		// Include any raw tool arguments that did not parse into write calls.
		if len(writeCalls) == 0 && len(toolCallArgs) > 0 {
			for i, raw := range toolCallArgs {
				assistantRaw.WriteString(fmt.Sprintf("@tool_raw index=%d\n", i))
				assistantRaw.WriteString(raw)
				if !strings.HasSuffix(raw, "\n") {
					assistantRaw.WriteString("\n")
				}
				assistantRaw.WriteString("\n")
			}
		}
	}
	logLLMMessage("ASSISTANT", assistantRaw.String(), activeChatID, model)

	// Build final parts: thinking first, then text content, then file events at the end
	if textContent.Len() > 0 {
		parts = append([]state.MessagePart{{
			Type:    "text",
			Content: textContent.String(),
		}}, parts...)
	}
	if thinkingContent.Len() > 0 {
		parts = append([]state.MessagePart{{
			Type:    "thinking",
			Content: thinkingContent.String(),
		}}, parts...)
	}

	// Add file write events as context_event parts (at the end, after thinking/text)
	stateMu.Lock()
	for _, wc := range writeCalls {
		cf := appState.FindContextFile(wc.Path)
		readOnly := false
		external := false
		if cf != nil {
			readOnly = cf.ReadOnly
			external = cf.External
		}
		isNew := cf == nil
		parts = append(parts, state.MessagePart{
			Type:     "context_event",
			Action:   "AssistantWriteFile",
			Path:     wc.Path,
			ReadOnly: &readOnly,
			External: &external,
			Version:  state.HashFileVersion(wc.Path, wc.Content),
			Added:    isNew,
		})
	}
	stateMu.Unlock()

	// Convert usage for storage
	streamDuration := time.Since(streamStart).Seconds()
	var msgUsage *state.MessageUsage
	if lastUsage != nil {
		msgUsage = &state.MessageUsage{
			PromptTokens:     lastUsage.PromptTokens,
			CompletionTokens: lastUsage.CompletionTokens,
			CachedTokens:     lastUsage.CachedTokens,
			TotalTokens:      lastUsage.TotalTokens,
			Cost:             lastUsage.Cost,
			Duration:         streamDuration,
		}
	}

	// Save assistant message with parts and usage
	stateMu.Lock()
	if err := appState.AddAssistantMessage("", parts, outputFiles, model, msgUsage); err != nil {
		stateMu.Unlock()
		respond(reqID, errorResponse(err))
		return
	}
	// Set reasoning effort on the saved message
	if reasoningConfig != nil {
		msgs := appState.ActiveChat.Messages
		msgs[len(msgs)-1].ReasoningEffort = reasoningConfig.Effort
		if saveErr := appState.SaveActiveChat(); saveErr != nil {
			log.Error("Failed to save reasoning effort: %v", saveErr)
		}
	}
	stateMu.Unlock()

	// Send done with usage info
	doneResp := map[string]any{"type": "done", "output_files": outputFiles}
	if lastUsage != nil {
		doneResp["usage"] = map[string]any{
			"prompt_tokens":     lastUsage.PromptTokens,
			"completion_tokens": lastUsage.CompletionTokens,
			"cached_tokens":     lastUsage.CachedTokens,
			"total_tokens":      lastUsage.TotalTokens,
			"cost":              lastUsage.Cost,
		}
	}
	doneResp["duration"] = streamDuration
	respond(reqID, doneResp)
}

// handleGenerateTitle generates a title for a chat based on the first message.
// This runs asynchronously and sends a title_updated response when done.
func handleGenerateTitle(reqID string, req map[string]any) {
	chatID, _ := req["chat_id"].(string)
	content, _ := req["content"].(string)

	if chatID == "" {
		respond(reqID, map[string]any{"type": "error", "message": "Missing required field: chat_id"})
		return
	}
	if content == "" {
		respond(reqID, map[string]any{"type": "error", "message": "Missing required field: content"})
		return
	}

	// Load config if needed
	if err := ensureConfig(); err != nil {
		respond(reqID, errorResponse(err))
		return
	}

	// Get context files if this is the active chat
	var contextFiles []string
	if appState.ActiveChat != nil && appState.ActiveChat.ID == chatID {
		for _, cf := range appState.ActiveChat.ContextFiles {
			contextFiles = append(contextFiles, cf.Path)
		}
	}

	// Acknowledge immediately - title generation happens async
	respond(reqID, map[string]any{"type": "ok"})

	// Generate title asynchronously
	go func() {
		// Build message with context info if files were attached
		fullContent := content
		if len(contextFiles) > 0 {
			fullContent = fmt.Sprintf("User message: %s\n\nContext files attached: %s", content, strings.Join(contextFiles, ", "))
		}

		messages := []llm.Message{
			{Role: "user", Content: fullContent},
		}

		title, err := llmClient.ChatSimple(appConfig.TitleModel, titlePrompt, messages)
		if err != nil {
			log.Error("Failed to generate title: %v", err)
			return
		}

		// Clean up title (trim whitespace, remove quotes)
		title = strings.TrimSpace(title)
		title = strings.Trim(title, "\"'")

		// Truncate if too long (allow longer titles - pane border has space)
		if len(title) > 80 {
			title = title[:77] + "..."
		}

		// Update chat name
		stateMu.Lock()
		if err := appState.SetChatName(chatID, title); err != nil {
			stateMu.Unlock()
			log.Error("Failed to set chat name: %v", err)
			return
		}
		stateMu.Unlock()

		log.Info("Generated title for chat %s: %s", chatID, title)

		// Send title_updated event
		respond("", map[string]any{
			"type":    "title_updated",
			"chat_id": chatID,
			"title":   title,
		})
	}()
}

type fileBlock struct {
	ID        string
	Path      string
	Mode      string // "ro" or "rw"
	Source    string // "context" or "output"
	Status    string // optional: "original", "pending_output", "added_output"
	Content   string
	StartLine int // For sections: 1-indexed start line, 0 = full file
	EndLine   int // For sections: 1-indexed end line (inclusive), 0 = full file
}

func writeSectionHeader(b *strings.Builder, title string) {
	b.WriteString(makeMarker(title, '-'))
	b.WriteString("\n")
}

func writeRawBlock(b *strings.Builder, header, content, footer string) {
	b.WriteString(header)
	b.WriteString("\n")
	if content != "" {
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString(footer)
	b.WriteString("\n\n")
}

func writeFileBlocks(b *strings.Builder, blocks []fileBlock) {
	for _, fb := range blocks {
		header := fmt.Sprintf("@file id=%s path=%s mode=%s source=%s", fb.ID, fb.Path, fb.Mode, fb.Source)
		if fb.StartLine > 0 && fb.EndLine > 0 {
			header += fmt.Sprintf(" lines=%d-%d", fb.StartLine, fb.EndLine)
		}
		if fb.Status != "" {
			header += " status=" + fb.Status
		}
		footer := fmt.Sprintf("@end file id=%s", fb.ID)
		writeRawBlock(b, header, fb.Content, footer)
	}
}

func writeHistoryAction(b *strings.Builder, id int, part state.MessagePart) {
	var fields []string
	fields = append(fields, fmt.Sprintf("@action id=%d", id))
	if part.Action != "" {
		fields = append(fields, "type="+part.Action)
	}
	if part.Version != "" {
		fields = append(fields, "file_id="+part.Version)
	}
	if part.Path != "" {
		fields = append(fields, "path="+part.Path)
	}
	if part.StartLine > 0 && part.EndLine > 0 {
		fields = append(fields, fmt.Sprintf("lines=%d-%d", part.StartLine, part.EndLine))
	}
	if part.OriginalPath != "" {
		fields = append(fields, "original_path="+part.OriginalPath)
	}
	if part.PrevVersion != "" {
		fields = append(fields, "prev_file_id="+part.PrevVersion)
	}
	if part.ReadOnly != nil {
		fields = append(fields, fmt.Sprintf("readonly=%t", *part.ReadOnly))
	}
	if part.External != nil {
		fields = append(fields, fmt.Sprintf("external=%t", *part.External))
	}
	if part.Added {
		fields = append(fields, "added=true")
	}
	b.WriteString(strings.Join(fields, " "))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("@end action id=%d\n\n", id))
}

func writeHistoryMessage(b *strings.Builder, id int, role, kind, content string) {
	header := fmt.Sprintf("@msg id=%d role=%s", id, role)
	if kind != "" {
		header += " kind=" + kind
	}
	footer := fmt.Sprintf("@end msg id=%d", id)
	writeRawBlock(b, header, content, footer)
}

func collectFileBlocks(chat *state.Chat) ([]fileBlock, []fileBlock, bool, error) {
	var readonly []fileBlock
	var writable []fileBlock
	versionChanged := false

	contextPaths := make(map[string]bool)
	for i := range chat.ContextFiles {
		cf := &chat.ContextFiles[i]
		contextPaths[cf.Path] = true

		contextContent, err := appState.GetContextFile(cf.Path)
		if err != nil {
			return nil, nil, false, err
		}
		contextVersion := state.HashFileVersion(cf.Path, contextContent)
		if cf.Version != contextVersion {
			cf.Version = contextVersion
			versionChanged = true
		}

		// Sections are always read-only and have no output
		if cf.IsSection() {
			readonly = append(readonly, fileBlock{
				ID:        contextVersion,
				Path:      cf.Path,
				Mode:      "ro",
				Source:    "context",
				Content:   contextContent,
				StartLine: cf.StartLine,
				EndLine:   cf.EndLine,
			})
			continue
		}

		var outputContent string
		var hasOutput bool
		if !cf.External {
			if out, err := appState.GetOutputFile(cf.Path); err == nil && out != "" {
				outputContent = out
				hasOutput = true
			}
		}

		switch {
		case cf.ReadOnly || cf.External:
			readonly = append(readonly, fileBlock{
				ID:      contextVersion,
				Path:    cf.Path,
				Mode:    "ro",
				Source:  "context",
				Content: contextContent,
			})
		case hasOutput:
			readonly = append(readonly, fileBlock{
				ID:      contextVersion,
				Path:    cf.Path,
				Mode:    "ro",
				Source:  "context",
				Status:  "original",
				Content: contextContent,
			})
			writable = append(writable, fileBlock{
				ID:      state.HashFileVersion(cf.Path, outputContent),
				Path:    cf.Path,
				Mode:    "rw",
				Source:  "output",
				Status:  "pending_output",
				Content: outputContent,
			})
		default:
			writable = append(writable, fileBlock{
				ID:      contextVersion,
				Path:    cf.Path,
				Mode:    "rw",
				Source:  "context",
				Content: contextContent,
			})
		}
	}

	outputFiles, err := appState.ListOutputFiles()
	if err != nil {
		return nil, nil, false, err
	}
	for _, path := range outputFiles {
		if contextPaths[path] {
			continue
		}
		content, err := appState.GetOutputFile(path)
		if err != nil || content == "" {
			continue
		}
		writable = append(writable, fileBlock{
			ID:      state.HashFileVersion(path, content),
			Path:    path,
			Mode:    "rw",
			Source:  "output",
			Status:  "added_output",
			Content: content,
		})
	}

	sort.Slice(readonly, func(i, j int) bool {
		if readonly[i].Path == readonly[j].Path {
			return readonly[i].ID < readonly[j].ID
		}
		return readonly[i].Path < readonly[j].Path
	})
	sort.Slice(writable, func(i, j int) bool {
		if writable[i].Path == writable[j].Path {
			return writable[i].ID < writable[j].ID
		}
		return writable[i].Path < writable[j].Path
	})

	return readonly, writable, versionChanged, nil
}

func latestUserIndex(messages []state.Message) int {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return i
		}
	}
	return -1
}

func summarizeFiles(readonly, writable []fileBlock) string {
	if len(readonly) == 0 && len(writable) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Files:\n")
	for _, fb := range readonly {
		line := fmt.Sprintf("  id=%s path=%s mode=%s", fb.ID, fb.Path, fb.Mode)
		if fb.StartLine > 0 && fb.EndLine > 0 {
			line += fmt.Sprintf(" lines=%d-%d", fb.StartLine, fb.EndLine)
		}
		if fb.Status != "" {
			line += " status=" + fb.Status
		}
		b.WriteString(line + "\n")
	}
	for _, fb := range writable {
		line := fmt.Sprintf("  id=%s path=%s mode=%s", fb.ID, fb.Path, fb.Mode)
		if fb.StartLine > 0 && fb.EndLine > 0 {
			line += fmt.Sprintf(" lines=%d-%d", fb.StartLine, fb.EndLine)
		}
		if fb.Status != "" {
			line += " status=" + fb.Status
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildLLMUserMessage constructs a single structured user message that includes
// current context files, structured history, the latest user message, and
// writable files. This avoids hidden assistant messages and keeps ordering stable.
func buildLLMUserMessage() (string, error) {
	chat := appState.ActiveChat
	if chat == nil {
		return "", state.ErrNoActiveChat
	}

	readonly, writable, versionChanged, err := collectFileBlocks(chat)
	if err != nil {
		return "", err
	}
	if versionChanged {
		if err := appState.SaveActiveChat(); err != nil {
			return "", err
		}
	}

	latestIdx := latestUserIndex(chat.Messages)
	var latestContent string
	var history []state.Message
	if latestIdx >= 0 {
		latestContent = chat.Messages[latestIdx].Content
		history = chat.Messages[:latestIdx]
	} else {
		history = chat.Messages
	}

	var b strings.Builder

	if len(readonly) > 0 {
		writeSectionHeader(&b, "readonly files")
		writeFileBlocks(&b, readonly)
	}

	var historyBuf strings.Builder
	entryID := 0
	for _, msg := range history {
		if msg.HasParts() {
			for _, part := range msg.Parts {
				switch part.Type {
				case "context_event":
					writeHistoryAction(&historyBuf, entryID, part)
					entryID++
				case "thinking":
					writeHistoryMessage(&historyBuf, entryID, "assistant", "reasoning", part.Content)
					entryID++
				case "text":
					writeHistoryMessage(&historyBuf, entryID, msg.Role, "", part.Content)
					entryID++
				case "code", "raw":
					writeHistoryMessage(&historyBuf, entryID, msg.Role, part.Type, part.Content)
					entryID++
				}
			}
			continue
		}
		writeHistoryMessage(&historyBuf, entryID, msg.Role, "", msg.Content)
		entryID++
	}
	if entryID > 0 {
		writeSectionHeader(&b, "history")
		b.WriteString(historyBuf.String())
		if !strings.HasSuffix(historyBuf.String(), "\n") {
			b.WriteString("\n")
		}
	}

	fileSummary := summarizeFiles(readonly, writable)
	if latestContent != "" || fileSummary != "" {
		writeSectionHeader(&b, "latest")
		latestBody := fileSummary
		if latestContent != "" {
			if latestBody != "" {
				latestBody += "\n\n"
			}
			latestBody += latestContent
		}
		writeRawBlock(&b, "@latest", latestBody, "@end latest")
	}

	if len(writable) > 0 {
		writeSectionHeader(&b, "writable files")
		writeFileBlocks(&b, writable)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

func errorResponse(err error) map[string]any {
	var msg string
	switch {
	case errors.Is(err, state.ErrNotInitialized):
		msg = "Not initialized"
	case errors.Is(err, state.ErrNotBB7Project):
		msg = "Not initialized. Run :BB7Init"
	case errors.Is(err, state.ErrAlreadyInit):
		msg = "Already initialized"
	case errors.Is(err, state.ErrNoActiveChat):
		msg = "No active chat"
	case errors.Is(err, state.ErrChatNotFound):
		msg = "Chat not found"
	case errors.Is(err, state.ErrFileNotFound):
		msg = "File not found"
	case errors.Is(err, state.ErrFileExists):
		msg = "Context file already exists"
	case errors.Is(err, state.ErrContextModified):
		msg = "File has pending output. Apply changes before setting read-only."
	case errors.Is(err, config.ErrNoConfig):
		msg = "Config file not found: ~/.config/bb7/config.json"
	case errors.Is(err, config.ErrNoAPIKey):
		msg = "API key not set in config"
	default:
		msg = err.Error()
	}
	return map[string]any{"type": "error", "message": msg}
}

func respond(reqID string, data map[string]any) {
	out, _ := json.Marshal(addResponseID(reqID, data))
	msgType, _ := data["type"].(string)
	respondMu.Lock()
	defer respondMu.Unlock()
	log.Response(msgType, string(out))
	fmt.Println(string(out))
}

func addResponseID(reqID string, data map[string]any) map[string]any {
	if reqID == "" {
		return data
	}
	data["request_id"] = reqID
	return data
}

func requestID(req map[string]any) string {
	switch v := req["request_id"].(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%v", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		return ""
	}
}
