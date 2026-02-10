# Configuration

## Setup Options

```lua
require('bb7').setup({
  -- Path to bb7 binary (nil = search PATH)
  bin = nil,

  -- Optional direct pane navigation keys (false = disabled)
  nav_left = false,   -- e.g., '<C-h>'
  nav_down = false,   -- e.g., '<C-j>'
  nav_up = false,     -- e.g., '<C-k>'
  nav_right = false,  -- e.g., '<C-l>'

  -- Chat styling (see below)
  chat_style = { ... },
})
```

## Instruction Files

BB-7 supports persistent instructions that are sent with every request. Both files are optional and support `@@` comments (lines starting with `@@` are stripped before sending).

**Global instructions** (`~/.config/bb7/instructions.md`):

Preferences that apply across all projects — your background, tools, communication style, etc.

```text
@@ Background — helps the LLM calibrate explanations
I have 5 years of web dev experience (React, Node, PostgreSQL).
I'm learning game development and am new to Godot, GDScript, and ECS patterns.
Explain game dev concepts, but skip basics on git, HTTP, async, etc.

@@ Style
Keep answers concise. Show code, not prose.
When suggesting architecture changes, explain the trade-off briefly.
```

**Project instructions** (`.bb7/instructions` in your project root):

Project-specific context. Supports `@include` directives to inline files from the project.

```text
This is a 2D roguelike built with Godot 4.3 and GDScript.

@@ Include the architecture doc so the LLM knows the project layout.
@include docs/ARCHITECTURE.md

Use signals for decoupled communication between nodes — do not
call methods on siblings directly.
Do NOT use @onready — we initialize in _ready() explicitly.
Prefer composition (child nodes) over deep inheritance hierarchies.
```

`@include` reference:
- `@include <path>` — include a file relative to project root
- `@include "path with spaces"` — quoted form for paths with spaces
- Included files must be inside the project directory
- Directives must start at column 0 and are ignored inside fenced code blocks
- Included files are inserted verbatim and not re-parsed

Everything in these files (after stripping comments and expanding includes) is included in every request, so keep them focused. The chat header shows which instruction files are active.

Use `:BB7EditInstructions Project` or `:BB7EditInstructions Global` to open the files for editing.

## Chat Styling

Chat styling uses two mechanisms: **highlight groups** for colors, and **`vim.g` variables** for icons. Both should be set before calling `setup()`.

### Highlight Groups

Override any `BB7*` highlight group with `vim.api.nvim_set_hl()`. BB-7 sets sensible defaults that adapt to your colorscheme — you only need to override what you want to change.

**Message highlights:**

| Group | Default | Purpose |
|-------|---------|---------|
| `BB7UserMessageBar` | Blue (`DiagnosticInfo`) | Left bar on user messages |
| `BB7UserMessageText` | Normal text on `Pmenu` bg | User message body |
| `BB7AssistantMessageBar` | Invisible (matches bg) | Left bar on assistant messages |
| `BB7AssistantMessageText` | Normal | Assistant message body |
| `BB7UserCodeBar` | Blue | Left bar on code blocks in user messages |
| `BB7UserCodeText` | `Identifier` on `Pmenu` bg | Code block text in user messages |
| `BB7AssistantCodeBar` | Invisible | Left bar on code blocks in assistant messages |
| `BB7AssistantCodeText` | `Identifier` | Code block text in assistant messages |
| `BB7ThinkingBar` | `Comment` | Left bar on thinking/reasoning blocks |
| `BB7ThinkingText` | `Comment` | Thinking/reasoning text |
| `BB7ErrorBar` | Red (`DiagnosticError`) | Left bar on error messages |
| `BB7ErrorText` | Red | Error message text |
| `BB7UserActionBar` | Invisible | Left bar on user actions (file added, etc.) |
| `BB7UserActionText` | `DiagnosticInfo` | User action text |
| `BB7AssistantActionBar` | Invisible | Left bar on assistant actions |
| `BB7AssistantActionText` | `Comment` | Assistant action text |
| `BB7SystemMessageBar` | Invisible | Left bar on system messages |
| `BB7SystemMessageText` | `Comment` | System message text |

**Example — custom colors:**

```lua
local hl = vim.api.nvim_set_hl
hl(0, 'BB7UserMessageBar', { fg = '#4b8dc6' })
hl(0, 'BB7UserMessageText', { fg = '#000000', bg = '#d4e7f8' })
hl(0, 'BB7AssistantActionText', { fg = '#842394' })

require('bb7').setup()
```

### Icons

Set icons via `vim.g` variables. Each message type has an `_icon` variable (the character) and an optional `_icon_fg` variable (foreground color — a hex string or highlight group name).

| Variable | Example |
|----------|---------|
| `vim.g.bb7_user_message_icon` | `'●'` |
| `vim.g.bb7_user_message_icon_fg` | `'DiagnosticInfo'` |
| `vim.g.bb7_assistant_message_icon` | `'●'` |
| `vim.g.bb7_assistant_message_icon_fg` | `'#842394'` |
| `vim.g.bb7_user_action_icon` | `'●'` |
| `vim.g.bb7_assistant_action_icon` | `'●'` |
| `vim.g.bb7_thinking_icon` | `'●'` |
| `vim.g.bb7_error_icon` | `'●'` |
| `vim.g.bb7_system_message_icon` | `'●'` |

Icons default to none. You can use any character — Nerd Font codepoints work well:

```lua
vim.g.bb7_assistant_action_icon = vim.fn.nr2char(0xf111) -- nf-fa-circle
vim.g.bb7_thinking_icon = vim.fn.nr2char(0xf5dc)         -- nf-fa-brain
vim.g.bb7_code_icon = vim.fn.nr2char(0xf121)              -- nf-fa-code
vim.g.bb7_error_icon = vim.fn.nr2char(0xf071)             -- nf-fa-warning
```

### Bar Character, Diff, and Spinner

These are the only styling options inside `setup()`:

```lua
require('bb7').setup({
  chat_style = {
    bar_char = '▕',  -- Character used for the left bar (default: '▕')

    diff = {
      add = 'DiffAdd',       -- Highlight for added lines
      remove = 'DiffDelete', -- Highlight for removed lines
      hunk = 'DiffText',     -- Highlight for hunk headers
    },

    spinner = {
      frames = { '·', '✢', '✳', '∗', '✻', '✽', '✻', '∗', '✳', '✢' },
      color = 'DiagnosticHint',
    },
  },
})
```

## Telescope Integration

Add files to BB-7 context directly from any Telescope picker. Add this to your `telescope.setup()`:

```lua
require('telescope').setup({
  defaults = {
    mappings = {
      i = {
        -- Ctrl+a to add file(s) to BB-7 context
        ['<C-a>'] = function(prompt_bufnr)
          local action_state = require('telescope.actions.state')
          local actions = require('telescope.actions')
          local picker = action_state.get_current_picker(prompt_bufnr)

          -- Get all selected entries (multi-select with Tab)
          local selections = picker:get_multi_selection()
          if #selections == 0 then
            -- No multi-selection, use current entry
            selections = { action_state.get_selected_entry() }
          end

          -- Add each file to context
          for _, entry in ipairs(selections) do
            local path = entry.path or entry.filename or entry.value
            if path then
              vim.cmd('BB7Add ' .. path)
            end
          end

          actions.close(prompt_bufnr)
        end,
      },
      n = {
        ['<C-a>'] = function(prompt_bufnr)
          -- Same as insert mode
          local action_state = require('telescope.actions.state')
          local actions = require('telescope.actions')
          local picker = action_state.get_current_picker(prompt_bufnr)
          local selections = picker:get_multi_selection()
          if #selections == 0 then
            selections = { action_state.get_selected_entry() }
          end
          for _, entry in ipairs(selections) do
            local path = entry.path or entry.filename or entry.value
            if path then
              vim.cmd('BB7Add ' .. path)
            end
          end
          actions.close(prompt_bufnr)
        end,
      },
    },
  },
})
```

This adds `<C-a>` (Ctrl+a) to **all** Telescope pickers. Use `Tab` to multi-select files, then `<C-a>` to add them all.

**Workflow example:**
```vim
:Telescope find_files       " Open file finder
Tab Tab Tab                 " Select multiple files
<C-a>                       " Add all selected to BB-7 context
```
