-- Shared UI state/config for the main BB7 view
local M = {}

local panes_chats = require('bb7.panes.chats')
local panes_context = require('bb7.panes.context')
local panes_provider = require('bb7.panes.provider')
local panes_preview = require('bb7.panes.preview')
local panes_input = require('bb7.panes.input')

-- Configuration (set via set_config)
M.config = {
  nav_left = false,
  nav_down = false,
  nav_up = false,
  nav_right = false,
}

-- UI State (reset on each open)
M.state = {
  is_open = false,
  active_pane = 1,
  panes = {}, -- { [1] = { win = ..., buf = ... }, ... }
  scrollbars = {}, -- { [pane_id] = { win = ..., buf = ... }, ... }
  hint_win = nil,
  hint_buf = nil,
  augroup = nil,
  picker_open = false, -- True when a picker/popup is open (suppresses auto-close)
  version = nil, -- Backend version string
}

-- Session state (persists across open/close within the same Neovim session)
M.session_state = {
  first_open = true,   -- True until window has been opened at least once
  active_pane = nil,   -- Last active pane (nil = use default)
  -- Per-pane view state: { cursor = {row, col}, topline = n }
  pane_views = {},     -- { [pane_id] = { cursor = ..., topline = ... }, ... }
}

-- Pane definitions
-- Internal IDs are 1-5
-- 'display' is for title brackets (matches the keymap), 'key' is the actual keymap
-- footer_fn returns string for bottom-right display (or nil for none)
M.PANES = {
  { id = 1, display = 'g1', key = 'g1', name = 'Chats',    title_fn = function() return 'Chats' end,
    footer_fn = function()
      local info = panes_chats.get_selection_info()
      if info then return info.selected .. ' of ' .. info.total end
      return nil
    end },
  { id = 2, display = 'g2', key = 'g2', name = 'Files',  title_fn = function() return 'Files' end,
    footer_fn = function()
      local summary = panes_context.get_summary()
      if summary then
        local text = summary.file_count .. ' file' .. (summary.file_count == 1 and '' or 's')
        text = text .. ' · ' .. summary.format_tokens(summary.total_tokens)
        if summary.potential_savings > 0 then
          text = text .. ' (save ' .. summary.format_tokens(summary.potential_savings) .. ')'
        end
        return text
      end
      return nil
    end },
  { id = 3, display = 'g3', key = 'g3', name = 'Info', title_fn = function() return 'Info' end },
  { id = 4, display = 'g4', key = 'g4', name = 'Preview',  title_fn = function() return panes_preview.get_title() end },
  { id = 5, display = 'g5', key = 'g5', name = 'Input',    title_fn = function() return 'Input' end,
    footer_fn = function() return panes_input.get_footer() end },
}

-- Map key to internal pane ID
M.KEY_TO_PANE = {
  ['g1'] = 1,  -- Chats
  ['g2'] = 2,  -- Files
  ['g3'] = 3,  -- Info
  ['g4'] = 4,  -- Preview
  ['g5'] = 5,  -- Input
}

-- Shortcut hints per pane (static fallbacks, panes can override)
-- Format: \"Action: <key>\" with | separator
-- Note: All panes now use their own get_hints(), these are just fallbacks
M.PANE_HINTS = {
  [1] = nil, -- Provided by panes_chats.get_hints()
  [2] = nil, -- Provided by panes_context.get_hints()
  [3] = nil, -- Provided by panes_provider.get_hints()
  [4] = nil, -- Provided by panes_preview.get_hints()
  [5] = nil, -- Provided by panes_input.get_hints()
}

return M
