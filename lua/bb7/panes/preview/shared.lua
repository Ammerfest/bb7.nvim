-- Shared state and config for the preview pane
local M = {}

M.state = {
  buf = nil,
  win = nil,
  chat = nil,        -- Current chat data
  mode = 'chat',     -- 'chat', 'file', or 'diff'
  current_file = nil, -- Current file being previewed (when mode == 'file' or 'diff')
  streaming = false, -- Whether we're currently receiving a streaming response
  stream_receiving = false, -- Whether we've received at least one token
  stream_lines = {}, -- Lines accumulated during streaming
  stream_reasoning_lines = {}, -- Reasoning lines accumulated during streaming
  pending_user_message = nil, -- User message being responded to (shown during streaming)
  show_placeholder = false,  -- Set to true for style testing
  on_title_changed = nil, -- Callback when title changes
  on_mode_changed = nil, -- Callback when mode changes
  spinner_timer = nil, -- Timer for spinner animation
  spinner_frame = 1,   -- Current spinner frame index
  collapsed_reasoning = {}, -- Set of collapsed reasoning block IDs (msg_idx:part_idx)
  reasoning_line_map = {}, -- Maps line number (1-indexed) to reasoning_id
  anchor_lines = {},       -- All anchor positions (messages + reasoning blocks)
  anchor_msg_idx = {},     -- Maps any anchor line -> message index (1-indexed)
  user_anchor_lines = {},  -- User message anchor positions only
  user_anchor_msg_idx = {}, -- Maps user anchor line -> message index (1-indexed)
  last_rendered_type = nil, -- Track content type for grouping across messages
  last_rendered_role = nil, -- Track role for separating user/assistant content
  autoscroll = true, -- Keep view pinned to bottom while streaming
  send_error = nil, -- Error message when send fails (shown inline in preview)
  diff_error = nil, -- List of diff error strings (shown after messages on failure)
}

-- Persistent state that survives window close/reopen
-- This tracks streaming across UI sessions
M.persistent = {
  is_streaming = false,      -- Whether a stream is currently active
  stream_start_time = nil,   -- When streaming started (os.time())
  last_usage = nil,          -- Last completed usage (prompt/completion/total/cached tokens + cost)
  last_duration = nil,       -- Last stream duration in seconds
  last_stream_chat_id = nil, -- Chat ID of the last completed stream (for duration display)
  duration_timer = nil,      -- Timer for updating duration display
}

M.config = {
  bar_char = '🮇',
  spinner_frames = { '⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏' },
  code_indent = '    ',
  style = {
    bar_padding = 2, -- Padding after bar (spaces in virtual text)
  },
}

M.extmarks = {}
M.syntax_highlights = {}
M.inline_highlights = {}  -- Bold/italic/underline/code regions (applied with full hl, not _FgOnly)
M.bold_hl_cache = {}
M.hl_cache = {}

M.ns_id = vim.api.nvim_create_namespace('bb7_preview')
M.syntax_ns = vim.api.nvim_create_namespace('bb7_syntax')

return M
