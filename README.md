# BB-7

A terminal-based LLM code assistant for Neovim. You control exactly which files the LLM sees — no autonomous file access, no shell commands, no magic.

## Features

- **Explicit context control**: Add files to context manually, like staging in git
- **Streaming responses**: See LLM output as it's generated
- **Output review**: LLM changes go to an output directory for review before applying
- **Diff preview**: View unified diff of LLM changes in the preview pane
- **Partial apply**: Cherry-pick changes using vim's native diff mode or your git client
- **Multiple chats**: Maintain separate conversations per task
- **Model selection**: Choose from available OpenRouter models with pricing info
- **Token estimation**: See context size and potential savings

## Requirements

- Neovim 0.11+
- [OpenRouter](https://openrouter.ai) API key (pay-per-token)
- Go 1.21+

## Installation

### Plugin (lazy.nvim)

```lua
{
  "Ammerfest/bb7",
  build = "./install.sh",
  config = function()
    require("bb7").setup()
  end,
}
```

The `install.sh` script builds the Go backend from source automatically.

To build manually:

```bash
cd ~/.local/share/nvim/lazy/bb7
go build -o bb7 ./cmd/bb7
```

### API Key

```bash
mkdir -p ~/.config/bb7
cat > ~/.config/bb7/config.json << 'EOF'
{
  "api_key": "sk-or-v1-your-key-here"
}
EOF
```

Optional settings:
```json
{
  "api_key": "sk-or-v1-...",
  "default_model": "anthropic/claude-sonnet-4",
  "title_model": "anthropic/claude-3-haiku"
}
```

## Usage

### Commands

| Command | Description |
|---------|-------------|
| `:BB7` | Toggle BB-7 UI |
| `:BB7Init` | Initialize BB-7 in the current directory |
| `:BB7Add [path[:start:end]]` | Add file or section to context (default: current buffer) |
| `:BB7AddReadonly [path]` | Add file to context as read-only (default: current buffer) |
| `:BB7Remove [path]` | Remove file from context |
| `:BB7Model` | Open model picker |
| `:BB7RefreshModels` | Refresh model list from OpenRouter |
| `:BB7Diff` | Switch preview pane to diff mode (unified) |
| `:BB7Chat` | Switch preview pane to chat mode |
| `:BB7File` | Switch preview pane to file mode |
| `:BB7DiffLocal` | Open vim native diff for partial apply |
| `:BB7Search` | Search chats via Telescope (requires Telescope) |
| `:BB7EditInstructions [level]` | Edit instructions file (project/global) |
| `:BB7Version` | Show backend version |

Context management works from anywhere — you don't need the BB-7 UI open:

```vim
:BB7Add                      " Add current buffer to context
:BB7Add src/utils.lua        " Add specific file
:BB7Add src/utils.lua:10:50  " Add lines 10-50 only (section)
:'<,'>BB7Add                 " Add visual selection as section (V mode)
:BB7AddReadonly              " Add current buffer as read-only
:bufdo BB7Add                " Add all open buffers
```

### UI Layout

BB-7 opens as a fullscreen overlay with 5 panes:

```
╭─[1] Chats────────────────╮ ╭─[4] Preview─────────────────────────────╮
│                          │ │                                         │
│  Chat list               │ │  Chat history / File preview / Diff    │
│                          │ │                                         │
╰──────────────────────────╯ │                                         │
╭─[2] Context──────────────╮ ╰─────────────────────────────────────────╯
│                          │ ╭─[5] Input───────────────────────────────╮
│  Files in context        │ │                                         │
│                          │ │  Compose messages                       │
╰──────────────────────────╯ ╰─────────────────────────────────────────╯
╭─[3] Provider─────────────╮
│  Balance / Costs         │
╰──────────────────────────╯
```

The focused pane has a highlighted border. Shortcuts for the focused pane are shown in the footer.

### Keybindings

**Global (all panes):**

| Key | Action |
|-----|--------|
| `g1`-`g5` | Jump to pane by number |
| `<Tab>` / `<S-Tab>` | Cycle panes |
| `<Esc>` | Close BB-7 |

**Chats [1]:**

| Key | Action |
|-----|--------|
| `<Space>` or `<CR>` | Select chat |
| `n` | New chat |
| `d` | Delete chat |
| `p` | Pin/unpin chat |
| `r` | Rename chat |

**Context [2]:**

| Key | Action |
|-----|--------|
| `<CR>` or `o` | Toggle directory |
| `x` | Remove file from context |
| `u` | Update out-of-sync file |
| `r` | Toggle read-only |
| `p` | Apply file to local filesystem |
| `P` | Apply all modified files |

**Preview [4]:**

| Key | Action |
|-----|--------|
| `<CR>` or `o` | Toggle reasoning block |
| `]]` / `[[` | Next / previous anchor |
| `]u` / `[u` | Next / previous user message |
| `<C-f>` | Fork chat from current message |
| `<C-e>` | Edit current user message |
| `<C-c>` | Cancel active stream |
| `gc` / `gf` / `gd` | Switch to chat / file / diff mode |

**Input [5]:**

| Key | Action |
|-----|--------|
| `<CR>` (normal) | Send message |
| `<S-CR>` (insert) | Send message |
| `M` (normal) | Open model picker |
| `R` (normal) | Cycle reasoning effort |
| `gc` / `gf` / `gd` | Switch preview mode |

The send key can be changed with `vim.g.bb7_send_key = 'enter'` (makes `<CR>` send in insert mode, `<S-CR>` for newlines).

### Context Status Indicators

| Status | Meaning |
|--------|--------|
| (blank) | In context, unchanged |
| `M` | LLM modified (output exists) |
| `A` | LLM added (new file) |
| `!A` | Conflict: LLM added file but it already exists locally |
| `S` | Section (partial file, read-only) |
| `R` | Read-only |
| `~` | Out of sync (local changed since added) |
| `~M` | Conflict (both local and LLM modified) |

### Workflow

1. **Open BB-7**: `:BB7` (or set your own keymap)
2. **Add files**: `:BB7Add` from any buffer, or use the Context pane (`g2`)
3. **Chat**: Go to Input pane (`g5`), type your message, send with `<S-CR>`
4. **Review**: LLM responses appear in Preview pane. Use `gf` for file view, `gd` for diff
5. **Apply**: In Context pane, `p` to apply a file or `P` for all modified files

### Partial Apply (Cherry-Picking Changes)

BB-7 doesn't include a built-in merge tool — use the diff tools you already know.

**Vim's native diff mode** (recommended):

```vim
:BB7DiffLocal             " Diff the currently selected file
:BB7DiffLocal src/foo.c   " Diff a specific file
```

Standard diff commands (`]c`/`[c` to jump, `do`/`dp` to obtain/put changes) work as expected.

**Git workflow**: Apply all changes with `P`, then use your git client (lazygit, fugitive, etc.) to unstage hunks you don't want.

## Configuration

```lua
require("bb7").setup({
  -- Path to bb7 binary (nil = auto-detect)
  bin = nil,

  -- Direct pane navigation keys (false = disabled)
  nav_left = false,   -- e.g., '<C-h>'
  nav_down = false,   -- e.g., '<C-j>'
  nav_up = false,     -- e.g., '<C-k>'
  nav_right = false,  -- e.g., '<C-l>'
})
```

Chat colors are customized via `BB7*` highlight groups, and icons via `vim.g.bb7_*` variables. See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for details, along with instruction files and Telescope integration.

## Models

Default: `anthropic/claude-sonnet-4`

Open the model picker with `M` in the Input pane or `:BB7Model`. Any OpenRouter model with tool support works. Favorite models (toggle with `<C-f>` in picker) appear first.

## Documentation

- [docs/CONFIGURATION.md](docs/CONFIGURATION.md) — Styling, instruction files, Telescope integration
- [docs/SPEC.md](docs/SPEC.md) — Backend architecture and data model
- [docs/PROTOCOL.md](docs/PROTOCOL.md) — stdin/stdout protocol
- [docs/PLUGIN.md](docs/PLUGIN.md) — Neovim UI architecture
- [docs/DIFFS.md](docs/DIFFS.md) — Diff handling design
- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) — Contributor build/test/debug workflows

## License

MIT
