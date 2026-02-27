# BB-7 Specification

## Overview

BB-7 is an external process that manages LLM chat sessions for a Neovim plugin. It communicates via stdin/stdout using line-delimited JSON. The Neovim plugin (Lua) handles UI; BB-7 handles state, API calls, and response parsing.

## Non-Goals

- No filesystem access beyond config and project state directories
- No autonomous behavior (no shell access, no browsing filesystem, no reading arbitrary files)
- No autocompletion features

## Tool Use

BB-7 uses LLM tool use (function calling) with two tools:

- **`write_file`**: Create new files or full-file rewrites (path + complete content)
- **`edit_file`**: Targeted edits to existing files (search/replace or anchor-based)

The `diff_mode` config controls which `edit_file` schema is exposed (default: `search_replace_multi`). See [DIFFS.md](DIFFS.md) for tool schemas, modes, and configuration.

The LLM cannot:
- Read files (context is provided explicitly by the user)
- Execute commands
- Access the network
- Modify files outside the output directory

Tool use provides robust parsing (handled by the API provider) while maintaining strict control over LLM capabilities.

## Process Model

Neovim starts BB-7 via `jobstart()`. One BB-7 process per Neovim instance. Process lifetime matches Neovim session. Multiple Neovim instances = multiple independent BB-7 processes.

## Data Model

### Project State Directory

```
{project}/.bb7/
├── instructions             # Optional project-specific instructions
├── pinned_chats.json        # Pinned chat IDs for this project
└── chats/
    ├── index.json           # Lightweight chat index (id, name, created)
    └── {chat-id}/
        ├── chat.json
        ├── context/
        │   ├── {filename}   # Immutable snapshot (full file)
        │   └── _sections/   # Immutable snapshots (partial files)
        │       └── {hash}   # Section content keyed by path+lines hash
        └── output/
            └── {filename}   # LLM-modified files only
```

### Terminology

- **local**: The actual project files (e.g., `math.cs`)
- **context**: Immutable snapshot of files when added to chat
- **output**: Files modified by the LLM

### chat.json

```json
{
  "version": 2,
  "id": "abc123",
  "name": "physics-refactor",
  "created": "2025-01-19T22:00:00Z",
  "model": "anthropic/claude-sonnet-4",
  "draft": "",
  "context_files": [
    {"path": "src/math.cs", "readonly": false, "external": false, "version": "a1b2c3d4"},
    {"path": "/home/user/reference/physics.cs", "readonly": true, "external": true, "version": "e5f6a7b8"},
    {"path": "src/utils.cs", "readonly": true, "external": false, "version": "11223344", "start_line": 10, "end_line": 50}
  ],
  "messages": [
    {
      "role": "user",
      "parts": [{"type": "text", "content": "Refactor to use ref parameters"}],
      "model": "anthropic/claude-sonnet-4",
      "timestamp": "2025-01-19T22:01:00Z",
      "context_snapshot": [
        {"path": "src/math.cs", "file_id": "a1b2c3d4"},
        {"path": "src/utils.cs", "file_id": "11223344", "start_line": 10, "end_line": 50}
      ]
    },
    {
      "role": "assistant",
      "model": "anthropic/claude-sonnet-4",
      "timestamp": "2025-01-19T22:01:05Z",
      "output_files": ["math.cs"],
      "parts": [
        {"type": "text", "content": "The issue is..."},
        {"type": "context_event", "action": "AssistantWriteFile", "path": "math.cs", "version": "abcd1234"}
      ]
    }
  ]
}
```

`version` tracks the chat format version. Current version is 2. Old chats (version 0 or 1) are lazily migrated on load: the legacy `content` field on messages is converted into `parts`, then the chat is re-saved.

`draft` stores unsent input text, persisted across sessions and restored when switching chats.

`context_files[*].version` is a client/backend-generated file id: a short (8
hex chars) SHA-256 hash over `path + NUL + content`. This ensures identical
content at different paths yields different ids. The LLM never generates file ids.

All messages use structured `parts` (there is no top-level `content` field on messages). Part types:
- `text`: Explanation text
- `thinking`: Reasoning/thinking content (from models with extended thinking)
- `code`: Code snippet with optional `language` field
- `file`: File action indicator with `path` field
- `context_event`: Context mutation event with `action`, `path`, `version`, `prev_version`, `readonly`, `external`
  - Actions: `AssistantWriteFile`, `UserWriteFile`, `UserApplyFile`, `UserSaveAs`, `UserRejectOutput`, `UserSetReadOnly`, `UserAddFile`, `UserAddSection`, `UserRemoveFile`, `UserRemoveSection`, `ForkWarningModified`, `ForkWarningDeleted`
- `raw`: Raw content (fallback)

User messages record the selected `model` at send time so the UI can show model switches over the course of a chat.

User messages also record a `context_snapshot` — an array of `{path, file_id, start_line?, end_line?}` capturing the exact context state at send time. This enables fork and edit operations to restore context accurately.

### Context Rules

- Context files are copied into `context/` when added
- The LLM sees the snapshot, not the current local file
- Additional files can be added during a chat
- Files can be updated (re-snapshotted) via the Files pane's `u` command
- Out-of-sync detection compares local file against context snapshot
- Context mutations emit structured history events so the LLM sees when files were added, removed, updated, or applied

### File Sections

Users can add partial files (line ranges) to context instead of full files:

- Sections are always read-only and immutable
- A file can have both a full version and multiple sections in context simultaneously
- Sections are stored in `context/_sections/` with hash-based filenames
- Added via `:BB7Add path:start:end` syntax or visual selection (`:'<,'>BB7Add`)
- Sections don't update when the source file changes
- The LLM sees sections with `lines=START-END` metadata

### LLM Request Assembly

To avoid hidden/synthetic assistant messages and to keep request prefixes stable,
BB-7 assembles a single structured user message that contains:

1) read-only files (sorted by path),
2) structured history (messages + actions),
3) the latest user message (as a separate block, with a file summary),
4) writable files (sorted by path).

System and project prompts remain in the system message. Tools are provided via
the API `tools` field, not embedded in the prompt body.

### Output Rules

- Only files that the LLM modifies appear in `output/`
- Output files contain complete file content, not diffs
- Output must match the code style of context exactly
- No explanatory comments added by LLM to output files
- LLM can create new files in `output/` directory if needed
- Hard safety guard: output writes must resolve within the project root; if not,
  BB-7 exits immediately and must be restarted manually.

### File Status

Files are tracked with status indicators:

| Status | Meaning | Source |
|--------|---------|--------|
| (blank) | In context, no output (or output matches context) | Backend |
| `M` | Modified: in context, has different output | Backend |
| `A` | Added: not in context, LLM created new file | Backend |
| `!A` | Conflict added: LLM created file, but file already exists locally | Backend |
| `S` | Section: partial file (immutable, always read-only) | Backend |
| `R` | Read-only: in context, LLM cannot modify | Frontend |
| `~` | Out of sync: local differs from context snapshot | Frontend |
| `~M` | Conflict: both local and LLM have changes | Frontend |

Backend statuses (`M`, `A`, `!A`, `S`) are returned by `get_file_statuses`. Frontend statuses (`R`, `~`, `~M`) are computed by comparing buffer content against context snapshots.

### Read-Only and Prompt Caching (Design Notes)

- Read-only is a safety and scope control: it tells the LLM not to modify those files.
- External files are always read-only to prevent out-of-project writes.
- Internal files can be toggled read-only via the Files pane's `r` key. Cannot set read-only on files with pending output.
- Prompt caching is provider-specific and confusing across OpenRouter models, so we do not depend on it for correctness.
- For now, we rely on implicit caching where available and keep prompt assembly stable.
- Cache-friendly behavior: keep read-only file content ordered deterministically and early in the prompt so repeated requests reuse the same prefix.
- Optional explicit cache key support is available via config (`"explicit_cache_key": true`), which sends `prompt_cache_key` for chat requests.
- Explicit cache-control breakpoints inside message content are still deferred until we have a clear, provider-verified approach.

## LLM Response Handling

The LLM response contains two parts:

1. **Text content**: Explanation of changes, stored in chat.json as structured message parts
2. **Tool calls**: `write_file` and/or `edit_file` calls

BB-7 processes responses as follows:
- Stream text content to Neovim as `chunk` messages
- Stream reasoning/thinking content as `thinking` messages (for models with extended thinking)
- Execute `write_file` calls by writing to the output directory
- Execute `edit_file` calls by applying edits to the output copy of the file (see [DIFFS.md](DIFFS.md))
- Send `done` message with list of written files and usage stats
- Store the text content, reasoning, and output file list in chat.json

No custom parsing required - the API provider handles tool call extraction.

### Stream Cancellation

The user can cancel an in-flight LLM response (`<C-c>`). When cancelled:
- Any partial assistant content (text, reasoning, file writes) is saved as an assistant message
- A system message `"Response aborted by user."` is appended after the partial response
- Both messages are included in conversation history sent to the LLM on subsequent requests
- File writes that completed before cancellation remain in the output directory

### Chat Forking

Users can fork a chat from any user message, creating a new conversation branch:

1. The new chat receives messages up to the fork point
2. Context is restored from the forked message's `context_snapshot`
3. The fork message content becomes the draft in the new chat
4. Context warnings are generated for files that have changed or been deleted since the original message

Forking is available via `<C-f>` in the Preview pane (cursor must be on a user message).

### Edit Message (Fork In Place)

Users can edit a previous user message via `<C-e>` in the Preview pane:

1. The chat is truncated to just before the target message
2. The message content becomes the chat draft
3. Context warnings are generated if any context files have changed
4. The user can modify the draft and re-send

## Diff View

Implemented:
- **context → output**: Unified diff inside the BB-7 preview pane (LCS-based).
- **local → output**: Native Neovim side-by-side diff via `:BB7DiffLocal` for partial apply workflows.

## Global Config

`~/.config/bb7/config.json`:
```json
{
  "api_key": "sk-or-...",
  "base_url": "https://openrouter.ai/api/v1",
  "default_model": "anthropic/claude-sonnet-4",
  "title_model": "anthropic/claude-3-haiku",
  "explicit_cache_key": false,
  "auto_retry_partial_edits": false
}
```

## Global State

`~/.bb7/state.json` stores global application state:
```json
{
  "favorites": ["anthropic/claude-sonnet-4", "openai/gpt-4o"],
  "last_model": "anthropic/claude-sonnet-4"
}
```

Model selection behavior:
- The last used model is persisted globally in `~/.bb7/state.json`.
- New chats default to that last used model when available.
- When continuing an existing chat, the model used is the last model recorded on
  a user message in that chat (unless the user explicitly changes it).

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `api_key` | Yes | - | OpenRouter API key |
| `base_url` | No | `https://openrouter.ai/api/v1` | API base URL |
| `default_model` | No | `anthropic/claude-sonnet-4` | Initial model for new chats (overrides last-used when explicitly set) |
| `title_model` | No | `anthropic/claude-3-haiku` | Model for title generation |
| `allow_data_retention` | No | `true` | Allow providers that retain data transiently |
| `allow_training` | No | `false` | Allow providers that train on user data |
| `auto_retry_partial_edits` | No | `false` | If true, perform one hidden repair attempt after partial `edit_file` apply failures |

## Instructions

BB-7 supports user instruction files that are injected into the system prompt:

**Global instructions**: `~/.config/bb7/instructions.md`
**Project instructions**: `.bb7/instructions`

Both are optional. When present, they are wrapped in XML tags:
```xml
<user-instructions source="~/.config/bb7/instructions.md">
...content...
</user-instructions>
```

### Instruction File Format (Project)

Instruction files are markdown-like with two directives:

- `@@` at column 0: comment line (ignored)
- `@include <path>` at column 0: include a file verbatim
- Directives must start at column 0

Directives are ignored inside fenced code blocks (lines starting with ``` or ~~~).
Included files are added raw and are not re-parsed (one-level only).

Path rules:
- Paths are relative to the project root
- Absolute paths are rejected
- Paths that resolve outside the project root are rejected (including via symlinks)

Parsing errors are surfaced to the UI and block sends until fixed.

Global instructions are plain markdown and are included as-is (no directives).

## System Prompt

The system prompt is embedded in the binary (`cmd/bb7/system_prompt.txt`). It defines the LLM's role and constraints. User instructions are appended to it.

## Token Estimation

BB-7 provides token estimation for the current context:
- System prompt tokens
- Instruction files tokens
- Chat history tokens
- Context files tokens (including output for M-status files)
- Potential savings (output tokens that could be removed by applying changes)

Uses `cl100k_base` tokenizer for accurate counts.

## Auto-Generated Titles

Chat titles are automatically generated after the first message exchange:
1. After receiving the first assistant response, a background request is sent
2. Uses the configured `title_model` (fast/cheap model)
3. Generates a 3-6 word descriptive title
4. UI receives `title_updated` event to refresh display

Default name format: `"Chat YYYY-MM-DD HH:MM"` (until title is generated)

## Initialization & State Machine

BB-7 follows a git-like initialization model. The `.bb7/` directory must be explicitly created before any operations.

### States

1. **Not initialized** - No `.bb7/` directory in project root
2. **Initialized, no chats** - `.bb7/` exists, `chats/` is empty
3. **Initialized, chats exist, none active** - Chats exist but none selected this session
4. **Chat active** - User has selected or created a chat

### Commands

| Command | Description |
|---------|-------------|
| `:BB7Init` | Initialize bb7 in current directory (creates `.bb7/`) |
| `:BB7` | Open bb7 UI |
| `:BB7Add` | Add current file to active chat's context |
| `:BB7AddReadonly` | Add current file to active chat's context as read-only |
| `:BB7Remove` | Remove current file from active chat's context |

### Behavior by State

| State | `:BB7Init` | `:BB7` | `:BB7Add` / `:BB7AddReadonly` / `:BB7Remove` |
|-------|-------------|---------|----------------------------|
| Not initialized | Create `.bb7/` | Error message | Error message |
| No chats | Already initialized | Open UI (empty state) | "No active chat" |
| Chats exist, none active | Already initialized | Open chat picker | "No active chat" |
| Chat active | Already initialized | Open UI (show chat) | Works |

### Error Messages

- Not initialized: `"BB-7: Not initialized. Run :BB7Init"`
- No active chat: `"BB-7: No active chat"`
- Already initialized: `"BB-7: Already initialized"`

### Session Flow

```
User runs :BB7Init
  → Creates .bb7/ directory structure
  → "BB-7: Initialized in {path}"

User runs :BB7
  → If chats exist but none active: show chat picker
  → If no chats: show empty state with prompt to create
  → If chat active: show chat UI

User selects/creates chat
  → Chat becomes active for this session
  → Context commands now work

User runs :BB7Add
  → If no active chat: "BB-7: No active chat"
  → If active: add file, "BB-7: Added {path}"
```

## User Feedback

All user actions should provide appropriate feedback via `vim.notify()`. Messages follow a consistent format.

### Message Format

```
"BB-7: {action result}"
```

### Severity Levels

| Level | Use Case | Example |
|-------|----------|---------|
| INFO | Success confirmations | "BB-7: Added src/main.go" |
| WARN | No-op or invalid state | "BB-7: No active chat" |
| ERROR | Failures | "BB-7: Failed to add file: permission denied" |

### Standard Messages

**Initialization:**
- `"BB-7: Initialized in {path}"` (INFO)
- `"BB-7: Already initialized"` (INFO)
- `"BB-7: Not initialized. Run :BB7Init"` (WARN)

**Context operations:**
- `"BB-7: Added {path}"` (INFO)
- `"BB-7: Removed {path}"` (INFO)
- `"BB-7: Updated {path}"` (INFO)
- `"BB-7: No active chat"` (WARN)
- `"BB-7: File already in context"` (WARN)
- `"BB-7: File not in context"` (WARN)

**Apply operations:**
- `"BB-7: Applied {path}"` (INFO)
- `"BB-7: Applied {n} files"` (INFO)
- `"BB-7: No output to apply"` (WARN)
- `"BB-7: File already applied"` (INFO)

**Errors:**
- `"BB-7: Failed to {action}"` (ERROR)
- `"BB-7: Cannot read file {path}"` (ERROR)
- `"BB-7: Cannot write file {path}"` (ERROR)
