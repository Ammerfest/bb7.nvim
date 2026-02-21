# BB-7 Neovim Plugin Architecture

## Overview

The BB-7 Neovim plugin provides the UI layer for interacting with the BB-7 backend. It manages a long-running BB-7 process via stdin/stdout and presents a fullscreen 5-pane interface.

For UI details, see [README.md](../README.md#ui-layout).

## Design Principles

- **Fullscreen overlay**: Single interface with multiple panes (like lazygit)
- **Vim-native**: Full vim keybindings in normal mode, g-prefix for BB-7 actions
- **Minimal friction**: Few keystrokes to common actions
- **Visual feedback**: Colors indicate state, streaming shows progress

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                           Neovim                                  │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌─────────────────┐  │
│  │  Chats   │  │  Files   │  │ Provider │  │     Preview     │  │
│  │  Pane    │  │  Pane    │  │   Pane   │  │      Pane       │  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └────────┬────────┘  │
│       │             │             │                  │           │
│       │       ┌─────┴─────────────┴──────────────────┴────┐     │
│       │       │                                           │      │
│       └───────┤               ui.lua                      │      │
│               │         (layout, navigation)              │      │
│               └─────────────────┬─────────────────────────┘      │
│                                 │                                │
│                    ┌────────────┴────────────┐                   │
│                    │       client.lua        │                   │
│                    │   (process, protocol)   │                   │
│                    └────────────┬────────────┘                   │
│                                 │ stdin/stdout                   │
└─────────────────────────────────┼────────────────────────────────┘
                                  ▼
                           ┌─────────────┐
                           │  bb7 bin   │
                           └─────────────┘
```

## Module Structure

```
lua/bb7/
├── init.lua               # Setup, commands, highlights, public API
├── ui.lua                 # Orchestrator (delegates to ui/ submodules)
├── ui/
│   ├── shared.lua         # Shared state and constants
│   ├── layout.lua         # Layout computation, window creation, resize
│   └── session.lua        # Session restore/save (pane views, active pane)
├── split.lua              # Bottom split input view (lightweight alternative to full UI)
├── status.lua             # Statusline indicator (streaming/unread state)
├── client.lua             # Backend process management, JSON protocol
├── picker.lua             # Generic fuzzy picker component
├── models.lua             # Model selection and favorites
├── telescope.lua          # Telescope integration (chat search)
└── panes/
    ├── chats.lua          # Chat list pane (pane 1)
    ├── context.lua        # Files pane (pane 2)
    ├── provider.lua       # Provider balance pane (pane 3)
    ├── preview.lua        # Preview pane facade (delegates to preview/ submodules)
    ├── preview/
    │   ├── shared.lua     # Shared preview state
    │   ├── render.lua     # Render pipeline (line assembly, sections, layout)
    │   ├── format.lua     # Text formatting (bold, italic, underline, wrapping)
    │   ├── highlight.lua  # Highlight logic
    │   ├── navigation.lua # Anchors, jumps, fork, edit
    │   ├── files.lua      # File/diff rendering
    │   ├── stream.lua     # Streaming logic
    │   ├── syntax.lua     # Syntax highlighting
    │   └── mock.lua       # Test fixtures / sample chat data
    └── input.lua          # Message input pane (pane 5)
```

## Module Responsibilities

### init.lua

- `setup(opts)`: Initialize plugin with user configuration
- Register commands (`:BB7`, `:BB7Add`, etc.)
- Define highlight groups
- Process chat styling options

### ui.lua

- Create/manage floating windows for all panes
- Handle pane navigation (`g1`-`g5`, Tab, C-w)
- Coordinate callbacks between panes
- Handle resize events

### client.lua

- Start/stop backend process via `vim.fn.jobstart()`
- Send requests and receive responses
- Handle streaming responses
- Dispatch async events (title updates)

### panes/*

Each pane module follows a common pattern:

```lua
local M = {}

function M.init(buf, win)         -- Initialize with buffer and window
function M.refresh(callback)      -- Reload data from backend
function M.setup_keymaps(buf)     -- Set pane-specific keybindings
function M.set_callbacks(cbs)     -- Register event callbacks
function M.get_hints()            -- Return shortcut hints string
function M.cleanup()              -- Clean up resources

return M
```

## Process Management (client.lua)

### Lifecycle

- Process starts on first UI open
- One process per Neovim instance
- Process persists across UI open/close
- Environment inherited from Neovim

### API

```lua
local client = require('bb7.client')

-- Initialize with project root
client.init(project_root, function(response, err) ... end)

-- Send request
client.request({ action = "chat_list" }, function(response, err) ... end)

-- Stream (for send action)
client.stream({ action = "send", content = "..." }, {
  on_chunk = function(content) ... end,
  on_done = function(output_files, usage) ... end,
  on_error = function(message) ... end,
})

-- Check state
client.is_initialized()
client.is_running()
client.get_project_root()

-- Register event handlers
client.set_event_handlers({
  on_title_updated = function(chat_id, title) ... end,
})
```

## Callback System

Panes communicate through callbacks set during `ui.open()`:

```lua
-- Chats pane callbacks
panes_chats.set_callbacks({
  on_chat_selected = function(chat) ... end,
  on_chat_created = function(id) ... end,
  on_data_changed = function() ... end,
})

-- Files pane callbacks
panes_context.set_callbacks({
  on_file_selected = function(file) ... end,
  on_data_changed = function() ... end,
})

-- Preview pane callbacks
panes_preview.set_callbacks({
  on_title_changed = function() ... end,
  on_mode_changed = function() ... end,
})

-- Input pane callbacks
panes_input.set_callbacks({
  on_message_sent = function(content) ... end,
  on_stream_chunk = function(chunk) ... end,
  on_stream_done = function(output_files, usage) ... end,
  on_mode_changed = function() ... end,
})
```

## Configuration

```lua
require('bb7').setup({
  -- Path to binary (nil = search PATH)
  bin = nil,

  -- Optional navigation keys
  nav_left = false,
  nav_down = false,
  nav_up = false,
  nav_right = false,

  -- Chat styling
  chat_style = {
    bar_char = '🮇',
    user = { bar = 'DiagnosticInfo', bg = 'Pmenu' },
    assistant = {},
    code = { text = 'Identifier' },
    file = { text = 'Keyword' },
    error = { text = 'DiagnosticError' },
    meta = { text = 'Comment' },
    diff = { add = 'DiffAdd', remove = 'DiffDelete', hunk = 'DiffText' },
    spinner = { frames = {...}, color = 'Comment' },
  },
})
```

## Highlights

### UI Highlights

| Highlight | Purpose |
|-----------|---------|
| `BB7BorderActive` | Active pane border |
| `BB7BorderInactive` | Inactive pane border |
| `BB7TitleActive` | Active pane title |
| `BB7TitleInactive` | Inactive pane title |
| `BB7HintText` | Shortcut hints |
| `BB7Selection` | Selected item (cursorline) |
| `BB7MarkerActive` | Active chat indicator |

### Status Highlights

| Highlight | Purpose |
|-----------|---------|
| `BB7StatusM` | Modified file status |
| `BB7StatusA` | Added file status |
| `BB7StatusSync` | Out-of-sync warning |

### Chat Highlights

For each chunk type (UserMessage, UserAction, AssistantMessage, AssistantAction, Thinking, Code, Error):
- `BB7{Type}Bar` - Vertical bar (fg = bar color, bg = normal to avoid inheriting line bg)
- `BB7{Type}Text` - Text and line background (fg = text color, bg = line background)

### Icons

Icons can be set via global variables:

```lua
-- Set icon character
vim.g.bb7_user_message_icon = '●'

-- Set icon color (optional - defaults to bar's fg)
vim.g.bb7_user_message_icon_fg = '#5555ff'  -- hex color
```

Available icon variables: `bb7_{type}_icon` and `bb7_{type}_icon_fg` where `{type}` is:
`user_message`, `user_action`, `assistant_message`, `assistant_action`, `thinking`, `code`, `error`

### Diff Highlights

| Highlight | Purpose |
|-----------|---------|
| `BB7DiffAdd` | Added lines |
| `BB7DiffRemove` | Removed lines |
| `BB7DiffHunk` | Hunk headers |

## Commands

| Command | Description |
|---------|-------------|
| `:BB7` | Toggle UI (closes split if open) |
| `:BB7Split` | Toggle bottom split input (lightweight message composer) |
| `:BB7Init` | Initialize BB-7 in current directory (creates `.bb7/`) |
| `:BB7Add [path[:start:end]]` | Add file or section to context (supports visual selection) |
| `:BB7AddReadonly [path]` | Add file to context as read-only |
| `:BB7Remove [path]` | Remove file from context |
| `:BB7Model` | Open model picker |
| `:BB7RefreshModels` | Refresh models |
| `:BB7Chat` | Switch preview to chat mode |
| `:BB7File` | Switch preview to file mode |
| `:BB7Diff` | Switch preview to diff mode |
| `:BB7DiffLocal [path]` | Open vim native diff for partial apply |
| `:BB7Version` | Show BB-7 version |
| `:BB7Search` | Search chats via Telescope (requires Telescope) |
| `:BB7EditInstructions [level]` | Edit instructions file (project/global/system) |

## Split Input View (`split.lua`)

The split view opens a bottom Neovim split with just the input pane, letting the user compose messages while seeing their editor buffers. It reuses `panes/input.lua` as a singleton — when the split opens, it calls `panes_input.init()` to take ownership.

### Keymaps

Same as full UI input pane (send, model picker, reasoning toggle), plus:
- `<Esc>` / `q` / `<C-c>` — close split

### Winbar

Shows: `BB-7 │ {chat_title} │ {model} │ {reasoning}`

### State transitions

| From | `:BB7` | `:BB7Split` |
|------|--------|-------------|
| Editor (no split) | Open full UI | Open split |
| Editor + split | Close split, open full UI | Close split |
| BB7 full UI | Close full UI | Close full UI, open split |

## Statusline API (`status.lua`)

Provides streaming/unread indicators for the user's statusline.

### State machine

```
idle ──[message sent from split]──> streaming ──[stream done]──> unread ──[full UI opened]──> idle
```

### API

```lua
local status = require('bb7.status')

status.status()      -- Symbol string ('○', '●', or '')
status.status_hl()   -- Highlight group name ('DiagnosticWarn', 'DiagnosticInfo', or nil)
status.raw_status()  -- 'streaming', 'unread', or nil
```

See `docs/CONFIGURATION.md` for statusline integration examples (mini.statusline, lualine, plain).

### Configuration

```lua
require('bb7').setup({
  status = {
    streaming = { enabled = true, symbol = '○', highlight = 'DiagnosticWarn' },
    unread    = { enabled = true, symbol = '●', highlight = 'DiagnosticInfo' },
  },
})
```

## Error Handling

- Backend errors: Display via `vim.notify()`
- Process crash: Logged, can be restarted on next UI open
- API errors: Displayed inline in chat

## Debug Logging

Set `BB7_DEBUG=1` environment variable to enable backend logging to `~/.bb7/logs/`.
