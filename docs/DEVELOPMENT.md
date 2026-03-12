# Development Guide

## Prerequisites

- Go toolchain (for backend build/tests)
- Neovim (for plugin tests)
- `plenary.nvim` available in Neovim (required by Lua test runner)

## Local Build

Use the dev build output in the project root:

```bash
go build -o bb7 ./cmd/bb7
```

The Neovim client resolves the backend binary in this order:
1. Configured `bin` path
2. Local `./bb7`
3. `bb7` on `PATH`

## Tests

Backend tests:

```bash
go test ./...
```

Lua plugin tests:

```bash
bash tests/run_tests.sh
```

Coverage (artifacts in `.tmp/coverage/`):

```bash
bash tests/run_coverage.sh
```

Recommended extra checks for risky backend changes:

```bash
go test -race ./...
go vet ./...
```

## Suggested Verification Flow

For backend or protocol changes:
1. `go test ./...`
2. `go build -o bb7 ./cmd/bb7`
3. `bash tests/run_tests.sh` (if frontend/protocol behavior is affected)

For Lua-only UI changes:
1. `bash tests/run_tests.sh`
2. Manual Neovim smoke check (`:BB7` flow)

## Debug Logs

Logs are written under `~/.bb7/logs/` when either:
- `BB7_DEBUG=1`, or
- `~/.bb7/debug` exists.

Backend logs: `bb7-YYYY-MM-DD_HH-MM-SS.log`
Frontend logs: `bb7-nvim-YYYY-MM-DD_HH-MM-SS.log`

## Mock / Placeholder Content

Three commands populate the UI with mock data for visual testing (run from Neovim command mode while BB-7 is open):

- **`:lua require('bb7.ui').set_screenshot_mode()`** — fills all panes with mock data (Godot signals conversation, chat list, file list, account balance). Used for GitHub release screenshots.
- **`:lua require('bb7.panes.preview').set_mock_chat()`** — loads a comprehensive test chat into the preview pane (8 messages covering refactoring, error handling, logging, etc.). Tests thinking blocks and scrollbars.
- **`:lua require('bb7.panes.preview').set_format_test_chat()`** — loads a formatting stress test into the preview pane. Tests bold, italic, underline, inline code, code blocks, consecutive actions grouping, and edge cases.

