# BB-7 Communication Protocol

Line-delimited JSON over stdin/stdout.

Each request should include a unique `request_id`. Responses to that request will include the same `request_id` so the client can match out-of-order replies.

## Requests (Neovim → BB7)

### Initialize

```json
{"request_id": "1", "action": "bb7_init", "project_root": "/path/to/project"}
{"request_id": "2", "action": "init", "project_root": "/path/to/project"}
```

`bb7_init` creates the `.bb7/` directory (like `git init`). `init` requires `.bb7/` to already exist and initializes the backend session.

### Chat Management

```json
{"request_id": "3", "action": "chat_new", "name": "optional-name"}
{"request_id": "4", "action": "chat_list"}
{"request_id": "5", "action": "chat_select", "id": "abc123"}
{"request_id": "6", "action": "chat_get"}
{"request_id": "7", "action": "chat_delete", "id": "abc123"}
{"request_id": "8", "action": "chat_rename", "id": "abc123", "name": "New name"}
{"request_id": "9", "action": "chat_active"}
{"request_id": "10", "action": "search_chats", "query": "physics"}
{"request_id": "11", "action": "chat_edit", "chat_id": "abc123", "message_index": 4, "content": "Updated message text"}
{"request_id": "12", "action": "fork_chat", "chat_id": "abc123", "fork_message_index": 4}
{"request_id": "13", "action": "save_draft", "draft": "Work in progress message"}
```

### Context Management

```json
{"request_id": "14", "action": "context_add", "path": "math.cs", "content": "...", "readonly": false}
{"request_id": "15", "action": "context_add_section", "path": "math.cs", "content": "...", "start_line": 10, "end_line": 50}
{"request_id": "16", "action": "context_update", "path": "math.cs", "content": "..."}
{"request_id": "17", "action": "context_remove", "path": "math.cs"}
{"request_id": "18", "action": "context_remove_section", "path": "math.cs", "start_line": 10, "end_line": 50}
{"request_id": "19", "action": "context_list"}
{"request_id": "20", "action": "context_set_readonly", "path": "math.cs", "readonly": true}
```

Section lines are 1-indexed, inclusive. Sections are always read-only.

### Messaging

```json
{"request_id": "21", "action": "send", "content": "Refactor to use ref parameters", "model": "anthropic/claude-sonnet-4"}
```

The `model` field is optional; if omitted, uses the default model from config.

### Edit Message (Fork In Place)

```json
{"request_id": "22", "action": "chat_edit", "chat_id": "abc123", "message_index": 4, "content": "Updated message text"}
```

`message_index` is 0-based. The chat is truncated to the message before `message_index`, and the provided `content` becomes the chat draft. Returns `context_warnings` for any context files that have changed since the target message.

### Fork Chat

```json
{"request_id": "23", "action": "fork_chat", "chat_id": "abc123", "fork_message_index": 4}
```

Creates a new chat from messages up to `fork_message_index` (0-based, must be a user message). Restores context state from the message's context snapshot. The fork message content becomes the draft in the new chat.

### Title Generation

```json
{"request_id": "24", "action": "generate_title", "chat_id": "abc123", "content": "First message content"}
```

### Token Estimation & Balance

```json
{"request_id": "25", "action": "get_balance"}
{"request_id": "26", "action": "estimate_tokens"}
```

### Models

```json
{"request_id": "27", "action": "get_models"}
```

### File Operations

```json
{"request_id": "28", "action": "get_context_file", "path": "math.cs"}
{"request_id": "29", "action": "get_output_file", "path": "math.cs"}
{"request_id": "30", "action": "get_diff_paths", "path": "math.cs"}
{"request_id": "31", "action": "get_file_statuses"}
{"request_id": "32", "action": "apply_file", "path": "math.cs"}
{"request_id": "33", "action": "apply_file_as", "path": "math.cs", "destination": "src/renamed.cs"}
{"request_id": "34", "action": "output_delete", "path": "math.cs"}
```

### Utility

```json
{"request_id": "35", "action": "ping"}
{"request_id": "36", "action": "version"}
{"request_id": "37", "action": "shutdown"}
{"request_id": "38", "action": "prepare_instructions", "level": "project"}
{"request_id": "39", "action": "get_customization_info"}
{"request_id": "40", "action": "cancel", "target_request_id": "21"}
```

Level values for `prepare_instructions`: `project`, `global`, `system`. Creates the file if missing.

## Responses (BB7 → Neovim)

### Acknowledgment

```json
{"type": "ok", "request_id": "1"}
```

### Error

```json
{"type": "error", "request_id": "2", "message": "No active chat"}
```

### Version

```json
{"type": "version", "version": "0.5.0"}
```

### Streaming Response

For `send` action:

```json
{"type": "chunk", "request_id": "3", "content": "The issue "}
{"type": "chunk", "request_id": "3", "content": "is that..."}
{"type": "thinking", "request_id": "3", "content": "Let me analyze the code..."}
{"type": "done", "request_id": "3", "output_files": ["math.cs"], "usage": {
  "prompt_tokens": 4200,
  "completion_tokens": 890,
  "cached_tokens": 3500,
  "total_tokens": 5090,
  "cost": 0.012
}}
```

The `thinking` type delivers reasoning/thinking content from models that support extended thinking.

### Title Updated (async event)

```json
{"type": "title_updated", "chat_id": "abc123", "title": "Generated title"}
```

### Chat List

```json
{"type": "chat_list", "request_id": "4", "chats": [
  {"id": "abc123", "name": "physics-refactor", "created": "2025-01-19T22:00:00Z"}
]}
```

### Chat Details

```json
{"type": "chat", "request_id": "5", "id": "abc123", "name": "physics-refactor", "model": "anthropic/claude-sonnet-4", "draft": "", "messages": [
  {"role": "user", "content": "...", "timestamp": "...", "context_snapshot": [
    {"path": "math.cs", "file_id": "a1b2c3d4"},
    {"path": "utils.cs", "file_id": "e5f6a7b8", "start_line": 10, "end_line": 50}
  ]},
  {"role": "assistant", "content": "...", "model": "...", "timestamp": "...", "output_files": ["file.cs"], "parts": [...]}
], "instructions_info": {
  "global_path": "~/.config/bb7/instructions.md",
  "global_exists": true,
  "project_path": ".bb7/instructions",
  "project_exists": false,
  "project_error": ""
}}
```

User messages include `context_snapshot` — an array of `{path, file_id, start_line?, end_line?}` recording the context state at send time. Used for fork/edit operations.

`project_error` contains parse errors for the project instruction file (empty string if none).

### Search Results

```json
{"type": "search_results", "request_id": "9", "results": [
  {"id": "abc123", "name": "Physics Chat", "created": "2025-01-19T22:00:00Z", "match_type": "title"},
  {"id": "def456", "name": "Game Dev", "created": "2025-01-18T10:00:00Z", "match_type": "content", "excerpt": "...physics engine..."}
]}
```

**match_type values:**
- `"title"`: Query matched the chat name
- `"content"`: Query matched message content (excerpt shows context around match)

### Fork Result

```json
{"type": "fork_result", "new_chat_id": "def456", "fork_message_content": "Original message text", "context_warnings": [
  {"path": "math.cs", "issue": "modified", "original_version": "a1b2c3d4"},
  {"path": "old_file.cs", "issue": "deleted", "original_version": "e5f6a7b8"}
]}
```

Context warnings indicate files from the original context snapshot that have changed or been deleted since the forked message was sent.

### Context List

```json
{"type": "context_list", "files": [
  {"path": "math.cs", "readonly": false, "external": false, "version": "a1b2c3d4"},
  {"path": "physics.cs", "readonly": true, "external": false, "version": "e5f6a7b8", "start_line": 10, "end_line": 50}
]}
```

### Balance

```json
{"type": "balance", "total_credits": 10.00, "total_usage": 3.45}
```

### Token Estimation

```json
{"type": "token_estimate",
  "total": 4200,
  "context_files": 2800,
  "history": 1200,
  "instructions": 200,
  "system_prompt": 150,
  "files": [
    {"path": "main.go", "tokens": 1200, "has_output": false},
    {"path": "utils.go", "tokens": 1800, "has_output": true, "original_tokens": 900, "output_tokens": 900}
  ],
  "potential_savings": 900
}
```

### Models

```json
{"type": "models", "models": [
  {
    "id": "anthropic/claude-sonnet-4",
    "name": "Claude Sonnet 4",
    "context_length": 200000,
    "supports_reasoning": true,
    "supports_tools": true,
    "pricing": {
      "prompt": "0.000003",
      "completion": "0.000015"
    }
  }
]}
```

### File Content

```json
{"type": "file_content", "path": "math.cs", "content": "..."}
```

### Diff Paths

Returns absolute filesystem paths for use with vim's native diff mode.

```json
{"type": "diff_paths", "path": "math.cs", "output_path": "/abs/path/.bb7/.../output/math.cs", "local_path": "/abs/path/math.cs"}
```

### Instructions Path

Returns absolute path to instructions file, creating it if needed.

```json
{"type": "instructions_path", "path": "/abs/path/.bb7/instructions"}
```

### Customization Info

Returns which customization files exist.

```json
{"type": "customization_info", "system_override": false, "global_instructions": true, "project_instructions": false, "project_instructions_error": ""}
```

`global_instructions` is true when the global instruction file exists.
`project_instructions` is true only when the project instruction file exists and parses successfully.
`project_instructions_error` contains the parse error message (empty string if none).

### File Statuses

```json
{"type": "file_statuses", "files": [
  {"path": "math.cs", "status": "", "in_context": true, "has_output": false, "readonly": false, "external": false, "tokens": 1200},
  {"path": "physics.cs", "status": "M", "in_context": true, "has_output": true, "readonly": false, "external": false, "tokens": 2100, "original_tokens": 1000, "output_tokens": 1100, "context_content": "...", "output_content": "..."},
  {"path": "new_file.cs", "status": "A", "in_context": false, "has_output": true, "readonly": false, "external": false, "tokens": 800, "output_content": "..."},
  {"path": "utils.cs", "status": "S", "in_context": true, "has_output": false, "readonly": true, "external": false, "tokens": 400, "start_line": 10, "end_line": 50, "context_content": "..."}
]}
```

**Status values:**
- `""` (empty): In context, no output or output matches context
- `"M"`: In context, has different output (modified by LLM)
- `"A"`: Not in context, has output (added by LLM)
- `"!A"`: Not in context, has output, but file already exists locally (conflict)
- `"S"`: Section (partial file, immutable, always read-only)

Additional fields: `readonly`, `external`, `context_content` (for sync comparison), `output_content` (for preview), `start_line`/`end_line` (for sections, 1-indexed inclusive).

Note: Out-of-sync status (`~`, `~M`) is calculated by the frontend by comparing buffer/local content with context.

Note: Diff computation is done in the frontend using `vim.diff()` (backed by xdiff/libgit2). No backend action needed.

### Apply File

```json
{"type": "ok", "content": "...applied file content..."}
```

### Edit Message

```json
{"type": "ok", "context_warnings": [
  {"path": "math.cs", "issue": "modified", "original_version": "a1b2c3d4"}
]}
```

## Error Handling

Standard error responses:

```json
{"type": "error", "message": "Unknown action: foo"}
{"type": "error", "message": "Missing required field: path"}
{"type": "error", "message": "No active chat"}
{"type": "error", "message": "Chat not found"}
{"type": "error", "message": "File not found"}
{"type": "error", "message": "Config file not found: ~/.config/bb7/config.json"}
{"type": "error", "message": "API key not set in config"}
```
