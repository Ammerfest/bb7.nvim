-- Preview pane: displays chat history and file previews
local M = {}

local log = require('bb7.log')
local shared = require('bb7.panes.preview.shared')
local render = require('bb7.panes.preview.render')
local files = require('bb7.panes.preview.files')
local stream = require('bb7.panes.preview.stream')
local navigation = require('bb7.panes.preview.navigation')
local highlight = require('bb7.panes.preview.highlight')
local mock = require('bb7.panes.preview.mock')

local state = shared.state
local persistent = shared.persistent

-- Notify title changed
local function notify_title_changed()
  if state.on_title_changed then
    state.on_title_changed()
  end
end

-- Switch to a specific mode
local function switch_mode(new_mode)
  -- Check if mode is valid for current state
  -- Diff mode available for M (modified) and ~M (conflict)
  if new_mode == 'diff' and (not state.current_file or (state.current_file.status ~= 'M' and state.current_file.status ~= '~M')) then
    log.warn('Diff mode only available for modified files')
    return
  end
  if (new_mode == 'file' or new_mode == 'diff') and not state.current_file then
    log.warn('No file selected')
    return
  end

  state.mode = new_mode

  -- Reset filetype when leaving file/diff mode
  if new_mode == 'chat' then
    vim.bo[state.buf].filetype = ''
  end

  -- Render appropriate content
  if new_mode == 'chat' then
    render.render()
  elseif new_mode == 'file' then
    files.render_file()
  elseif new_mode == 'diff' then
    files.render_diff()
  end

  notify_title_changed()
  if state.on_mode_changed then
    state.on_mode_changed(new_mode)
  end
end

-- Setup keymaps for read-only navigation
local function setup_keymaps(buf)
  local opts = { buffer = buf, nowait = true, silent = true }

  -- Enable vim navigation (j/k/gg/G)
  vim.keymap.set('n', 'j', 'j', opts)
  vim.keymap.set('n', 'k', 'k', opts)
  vim.keymap.set('n', 'gg', 'gg', opts)
  vim.keymap.set('n', 'G', 'G', opts)
  vim.keymap.set('n', '<C-d>', '<C-d>', opts)
  vim.keymap.set('n', '<C-u>', '<C-u>', opts)

  -- Enable yank
  vim.keymap.set('n', 'y', 'y', opts)
  vim.keymap.set('n', 'yy', 'yy', opts)
  vim.keymap.set('v', 'y', 'y', opts)

  -- Enable visual mode for selection
  vim.keymap.set('n', 'v', 'v', opts)
  vim.keymap.set('n', 'V', 'V', opts)

  -- Toggle reasoning block collapse
  vim.keymap.set('n', '<CR>', navigation.toggle_reasoning, opts)
  vim.keymap.set('n', 'o', navigation.toggle_reasoning, opts)

  -- Mode switching with g-prefix (mirrors vim's "go to" commands)
  -- gf = go to file view, gc = go to chat view, gd = go to diff view
  vim.keymap.set('n', 'gc', function() switch_mode('chat') end, opts)
  vim.keymap.set('n', 'gf', function() switch_mode('file') end, opts)
  vim.keymap.set('n', 'gd', function() switch_mode('diff') end, opts)

  -- Anchor navigation (vim-style section movements)
  -- [[ / ]] - jump between all anchors (messages + reasoning blocks)
  vim.keymap.set('n', '[[', function() navigation.jump_prev_anchor(state.anchor_lines) end, opts)
  vim.keymap.set('n', ']]', function() navigation.jump_next_anchor(state.anchor_lines) end, opts)
  -- [u / ]u - jump between user messages only
  vim.keymap.set('n', '[u', function() navigation.jump_prev_anchor(state.user_anchor_lines) end, opts)
  vim.keymap.set('n', ']u', function() navigation.jump_next_anchor(state.user_anchor_lines) end, opts)

  -- Fork chat from current user message
  vim.keymap.set('n', '<C-f>', navigation.fork_chat, opts)

  -- Edit current user message in-place
  vim.keymap.set('n', '<C-e>', navigation.edit_chat_message, opts)

  -- Show message info popup
  vim.keymap.set('n', 'K', navigation.show_message_info, opts)

  -- Cancel current request
  vim.keymap.set('n', '<C-x>', function()
    require('bb7.panes.input').cancel_send()
  end, opts)
end

-- Set callbacks
function M.set_callbacks(callbacks)
  state.on_title_changed = callbacks.on_title_changed
  state.on_mode_changed = callbacks.on_mode_changed
end

-- Set the current chat data
function M.set_chat(chat)
  state.chat = chat
  state.pending_user_message = nil
  if not state.streaming then
    state.stream_lines = {}
    state.stream_reasoning_lines = {}
  end
  state.mode = 'chat'
  state.current_file = nil

  -- Check for instruction parse errors and show/clear accordingly
  local instr_error = nil
  if chat and chat.instructions_info and chat.instructions_info.project_error
      and chat.instructions_info.project_error ~= '' then
    instr_error = chat.instructions_info.project_error
  end
  local had_error = state.send_error ~= nil
  state.send_error = instr_error

  -- Note: don't reset collapsed_reasoning here - preserve collapse state across chat refresh
  render.render()
  notify_title_changed()

  -- Scroll to top when an instruction error is present
  if instr_error and not had_error then
    state.autoscroll = false
    vim.schedule(function()
      if state.win and vim.api.nvim_win_is_valid(state.win) then
        vim.api.nvim_win_set_cursor(state.win, { 1, 0 })
      end
    end)
  end
end

-- Get current chat
function M.get_chat()
  return state.chat
end

-- Start streaming mode (optionally with user message to show immediately)
function M.start_streaming(user_message)
  stream.start_streaming(user_message)
end

-- Append streaming content
function M.append_stream(content)
  stream.append_stream(content)
end

-- Append reasoning streaming content
function M.append_reasoning_stream(content)
  stream.append_reasoning_stream(content)
end

-- End streaming mode (optionally with usage info)
function M.end_streaming(usage)
  stream.end_streaming(usage)
end

-- Show a send error inline in the preview pane
function M.show_send_error(error_msg)
  stream.show_send_error(error_msg)
end

-- Clear send error (e.g., when user edits input to retry)
function M.clear_send_error()
  if state.send_error then
    state.send_error = nil
    state.pending_user_message = nil
    if state.mode == 'chat' then
      render.render()
    end
  end
end

-- Check if there's a pending send error
function M.has_send_error()
  return state.send_error ~= nil
end

-- Get the send error message
function M.get_send_error()
  return state.send_error
end

-- Get the pending user message (preserved after send error)
function M.get_pending_message()
  return state.pending_user_message
end

local function update_autoscroll()
  if not state.win or not vim.api.nvim_win_is_valid(state.win) then
    return
  end
  local line_count = vim.api.nvim_buf_line_count(state.buf)
  local win_height = vim.api.nvim_win_get_height(state.win)
  local view = vim.fn.winsaveview()
  state.autoscroll = (view.topline + win_height - 1) >= line_count
end

-- Initialize the pane
function M.init(buf, win)
  state.buf = buf
  state.win = win
  state.chat = nil
  state.stream_lines = {}
  state.stream_reasoning_lines = {}

  -- Restore streaming state from persistent if applicable
  -- Also verify the client still has active stream handlers
  local client = require('bb7.client')
  if persistent.is_streaming and client.has_active_stream() then
    state.streaming = true
    local buffer = client.get_stream_buffer()
    if buffer then
      state.stream_lines = vim.split(buffer.content or '', '\n', { plain = true })
      state.stream_reasoning_lines = vim.split(buffer.reasoning or '', '\n', { plain = true })
      state.pending_user_message = buffer.user_message
    end
    stream.start_spinner()
  else
    -- If persistent says streaming but no active handlers, stream completed while closed
    if persistent.is_streaming then
      persistent.is_streaming = false
      -- Calculate final duration if we have start time
      if persistent.stream_start_time then
        persistent.last_duration = os.time() - persistent.stream_start_time
        persistent.stream_start_time = nil
      end
    end
    state.streaming = false
  end

  -- Make buffer non-modifiable (don't use readonly, it causes warnings)
  vim.bo[buf].modifiable = false

  -- Enable breakindent so wrapped lines align with content after the bar
  -- Bar char (1) + padding (2) = 3 character indent for wrapped lines
  local bar_width = vim.fn.strwidth(shared.config.bar_char)
  local indent = bar_width + shared.config.style.bar_padding
  vim.wo[win].wrap = true
  vim.wo[win].breakindent = true
  vim.wo[win].breakindentopt = 'shift:' .. indent

  -- Setup keymaps
  setup_keymaps(buf)

  local augroup = vim.api.nvim_create_augroup('BB7Preview', { clear = true })
  vim.api.nvim_create_autocmd({ 'WinScrolled', 'CursorMoved', 'CursorMovedI' }, {
    group = augroup,
    buffer = buf,
    callback = function()
      update_autoscroll()
    end,
  })

  render.render()
end

-- Check if streaming is currently active (for external query)
function M.is_streaming()
  return persistent.is_streaming
end

-- Clear last response stats (call when starting a new message)
function M.clear_last_stats()
  persistent.last_usage = nil
  persistent.last_duration = nil
end

-- Get hints for this pane
function M.get_hints()
  if state.show_placeholder then
    return 'STYLE PREVIEW | Toggle: :lua require("bb7.panes.preview").toggle_placeholder()'
  end

  -- Mode-specific hints
  if state.mode == 'chat' then
    if state.streaming then
      return 'Next anchor: ]] | Next input: ]u | Toggle: <CR> | Cancel: <C-x>'
    end
    return 'Next anchor: ]] | Next input: ]u | Toggle: <CR> | Info: K | Fork: <C-f> | Edit: <C-e>'
  elseif state.mode == 'file' then
    return ''
  elseif state.mode == 'diff' then
    return ''
  end
  return ''
end

-- Get title for this pane (called from ui.lua)
function M.get_title()
  if (state.mode == 'file' or state.mode == 'diff') and state.current_file then
    local status_str = ''
    if state.current_file.status == '~' then
      status_str = ' [out of sync]'
    elseif state.current_file.status == '~M' then
      status_str = ' [conflict]'
    elseif state.current_file.status == 'M' then
      status_str = ' [modified]'
    elseif state.current_file.status == 'A' then
      status_str = ' [added]'
    end
    local mode_str = state.mode == 'diff' and ' (diff)' or ''
    return state.current_file.path .. status_str .. mode_str
  elseif state.chat then
    return state.chat.name or 'Untitled'
  end
  return nil
end

-- Show a context file (called when hovering in Files pane)
function M.show_context_file(file)
  if not file then
    -- No file selected, return to chat mode
    state.mode = 'chat'
    state.current_file = nil
    vim.bo[state.buf].filetype = ''
    render.render()
    notify_title_changed()
    return
  end

  state.current_file = file
  -- Auto-switch to file mode when selecting a file
  state.mode = 'file'
  files.render_file()
  notify_title_changed()
end

-- Get current mode
function M.get_mode()
  return state.mode
end

-- Get current file
function M.get_current_file()
  return state.current_file
end

-- Set current file (without switching mode)
-- Used to track file selection from Files pane when not in Files focus
function M.set_current_file(file)
  state.current_file = file
end

-- Switch to chat mode (exported for commands)
function M.switch_to_chat()
  switch_mode('chat')
end

-- Switch to file mode (exported for commands)
function M.switch_to_file()
  switch_mode('file')
end

-- Switch to diff mode (exported for commands)
function M.switch_to_diff()
  switch_mode('diff')
end

-- Return to chat mode (from file preview)
-- Keeps current_file so gf/gd still work after switching away from Files pane
function M.show_chat()
  state.mode = 'chat'
  render.render()
  notify_title_changed()
end

-- Toggle placeholder mode (for style testing)
function M.toggle_placeholder()
  state.show_placeholder = not state.show_placeholder
  render.render()
end

-- Clear derived highlight cache (call on colorscheme change)
function M.clear_hl_cache()
  highlight.clear_hl_cache()
end

-- Set a mock chat for interactive testing (thinking blocks, scrollbars, etc.)
-- Usage: :lua require('bb7.panes.preview').set_mock_chat()
function M.set_mock_chat()
  state.chat = mock.get_mock_chat()
  state.mode = 'chat'
  state.collapsed_reasoning = {}  -- Reset collapse state for fresh testing
  render.render()
  notify_title_changed()

  log.info('Mock chat loaded - use <CR> on reasoning blocks to toggle collapse')
end

-- Set a mock chat specifically for testing formatting rules
-- Tests: consecutive actions grouping, Model lines, text spacing, etc.
-- Usage: :lua require('bb7.panes.preview').set_format_test_chat()
function M.set_format_test_chat()
  state.chat = mock.get_format_test_chat()
  state.mode = 'chat'
  state.collapsed_reasoning = {}
  render.render()
  notify_title_changed()

  log.info('Format test chat loaded - validates spacing rules')
end

-- Set style options (for experimentation)
function M.set_style(opts)
  for k, v in pairs(opts) do
    shared.config.style[k] = v
  end
  render.render()
end

-- Set spinner frames from init.lua
function M.set_spinner_frames(frames)
  if frames then
    shared.config.spinner_frames = frames
  end
end

-- Scroll the preview window by half-page (viewport scroll, not cursor move)
function M.scroll_down()
  if not state.win or not vim.api.nvim_win_is_valid(state.win) then
    return
  end
  vim.api.nvim_win_call(state.win, function()
    vim.cmd('normal! \x04')  -- C-d
  end)
end

function M.scroll_up()
  if not state.win or not vim.api.nvim_win_is_valid(state.win) then
    return
  end
  vim.api.nvim_win_call(state.win, function()
    vim.cmd('normal! \x15')  -- C-u
  end)
end

-- Cleanup
function M.cleanup()
  stream.stop_spinner()
  state.buf = nil
  state.win = nil
  state.chat = nil
  state.mode = 'chat'
  state.current_file = nil
  state.streaming = false
  state.stream_lines = {}
  state.stream_reasoning_lines = {}
  state.pending_user_message = nil
  state.send_error = nil
  state.collapsed_reasoning = {}
  state.reasoning_line_map = {}
  state.anchor_lines = {}
  state.user_anchor_lines = {}
  state.user_anchor_msg_idx = {}
  state.on_title_changed = nil
end

return M
