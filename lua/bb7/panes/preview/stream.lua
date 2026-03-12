-- Streaming helpers for preview pane
local M = {}

local shared = require('bb7.panes.preview.shared')
local format = require('bb7.panes.preview.format')
local render = require('bb7.panes.preview.render')

-- Forward declaration for stop_spinner
local stop_spinner

-- Get the active spinner config for the current phase
local function spinner_config()
  if shared.state.stream_receiving then
    return shared.config.spinner_streaming
  else
    return shared.config.spinner_waiting
  end
end

-- Compute effective cycle length for a spinner config
local function spinner_cycle_len(cfg)
  local n = #cfg.frames
  if cfg.reverse_loop and n > 2 then
    return 2 * n - 2
  end
  return n
end

-- Resolve spinner frame from a frame index and config
-- For reverse_loop with frames {A,B,C,D}: A,B,C,D,C,B,A,B,...
function M.resolve_spinner_frame(cfg, idx)
  local n = #cfg.frames
  if cfg.reverse_loop and n > 2 then
    local cycle = 2 * n - 2
    local pos = ((idx - 1) % cycle) + 1
    if pos <= n then
      return cfg.frames[pos]
    else
      return cfg.frames[2 * n - pos]
    end
  end
  return cfg.frames[((idx - 1) % n) + 1]
end

-- Start (or restart) the spinner animation timer with the given interval
local function start_spinner_timer(interval)
  if shared.state.spinner_timer then
    vim.fn.timer_stop(shared.state.spinner_timer)
  end
  shared.state.spinner_timer = vim.fn.timer_start(interval, function()
    if not shared.state.streaming then
      stop_spinner()
      return
    end
    local cfg = spinner_config()
    local cycle = spinner_cycle_len(cfg)
    shared.state.spinner_frame = (shared.state.spinner_frame % cycle) + 1
    vim.schedule(function()
      -- Only render if preview is in chat mode (don't overwrite file/diff views)
      if shared.state.mode == 'chat' then
        render.render()
      end
    end)
  end, { ['repeat'] = -1 })
end

-- Start the spinner animation timer
local function start_spinner()
  if shared.state.spinner_timer then return end -- Already running
  shared.state.spinner_frame = 1
  start_spinner_timer(spinner_config().interval)
end

-- Stop the spinner animation timer
stop_spinner = function()
  if shared.state.spinner_timer then
    vim.fn.timer_stop(shared.state.spinner_timer)
    shared.state.spinner_timer = nil
  end
  shared.state.spinner_frame = 1
end

-- Start streaming mode (optionally with user message to show immediately)
function M.start_streaming(user_message)
  shared.state.streaming = true
  shared.state.stream_receiving = false
  shared.state.stream_lines = {}
  shared.state.stream_reasoning_lines = {}
  shared.state.pending_user_message = user_message
  shared.state.send_error = nil -- Clear any previous send error
  shared.state.diff_error = nil -- Clear any previous diff error
  shared.state.autoscroll = true

  -- Track streaming in persistent state
  shared.persistent.is_streaming = true
  shared.persistent.stream_start_time = os.time()
  shared.persistent.last_usage = nil  -- Clear last usage when new stream starts
  shared.persistent.last_duration = nil

  -- Switch to chat mode so the user sees the message was sent
  if shared.state.mode ~= 'chat' then
    shared.state.mode = 'chat'
    if shared.state.buf and vim.api.nvim_buf_is_valid(shared.state.buf) then
      vim.api.nvim_buf_call(shared.state.buf, function()
        vim.treesitter.stop(shared.state.buf)
        vim.bo[shared.state.buf].filetype = ''
      end)
    end
  end

  start_spinner()
  render.render()
end

-- Append streaming content
function M.append_stream(content)
  if not shared.state.streaming then return end
  if not shared.state.stream_receiving then
    local old_interval = shared.config.spinner_waiting.interval
    shared.state.stream_receiving = true
    shared.state.spinner_frame = 1  -- Reset for streaming spinner
    -- Restart timer if streaming interval differs from waiting
    if shared.config.spinner_streaming.interval ~= old_interval then
      start_spinner_timer(shared.config.spinner_streaming.interval)
    end
  end

  -- Split content by newlines and append
  for char in content:gmatch('.') do
    if char == '\n' then
      table.insert(shared.state.stream_lines, '')
    else
      if #shared.state.stream_lines == 0 then
        table.insert(shared.state.stream_lines, '')
      end
      shared.state.stream_lines[#shared.state.stream_lines] = shared.state.stream_lines[#shared.state.stream_lines] .. char
    end
  end

  -- Only render if preview is in chat mode (don't overwrite file/diff views)
  if shared.state.mode == 'chat' then
    render.render()
  end
end

-- Append reasoning streaming content
function M.append_reasoning_stream(content)
  if not shared.state.streaming then return end
  -- Skip whitespace-only chunks to avoid rendering an empty reasoning block
  if not content or not content:match('%S') then return end
  if not shared.state.stream_receiving then
    local old_interval = shared.config.spinner_waiting.interval
    shared.state.stream_receiving = true
    shared.state.spinner_frame = 1  -- Reset for streaming spinner
    if shared.config.spinner_streaming.interval ~= old_interval then
      start_spinner_timer(shared.config.spinner_streaming.interval)
    end
  end

  -- Split content by newlines and append
  for char in content:gmatch('.') do
    if char == '\n' then
      table.insert(shared.state.stream_reasoning_lines, '')
    else
      if #shared.state.stream_reasoning_lines == 0 then
        table.insert(shared.state.stream_reasoning_lines, '')
      end
      shared.state.stream_reasoning_lines[#shared.state.stream_reasoning_lines] = shared.state.stream_reasoning_lines[#shared.state.stream_reasoning_lines] .. char
    end
  end

  -- Only render if preview is in chat mode (don't overwrite file/diff views)
  if shared.state.mode == 'chat' then
    render.render()
  end
end

-- End streaming mode (optionally with usage info)
function M.end_streaming(usage)
  -- Calculate final duration before stopping
  if shared.persistent.stream_start_time then
    shared.persistent.last_duration = os.time() - shared.persistent.stream_start_time
  end

  shared.state.streaming = false
  shared.state.stream_receiving = false
  shared.state.stream_lines = {}
  shared.state.stream_reasoning_lines = {}
  shared.state.pending_user_message = nil
  shared.stream_cache = nil  -- Invalidate render cache

  -- Update persistent state
  shared.persistent.is_streaming = false
  shared.persistent.stream_start_time = nil
  shared.persistent.last_stream_chat_id = shared.state.chat and shared.state.chat.id or nil
  if usage then
    shared.persistent.last_usage = usage
  end

  stop_spinner()
  -- The chat will be refreshed with the complete message
end

-- Show a send error inline in the preview pane
-- Preserves the pending user message so the user can see what they tried to send
function M.show_send_error(error_msg)
  shared.state.streaming = false
  shared.state.stream_receiving = false
  shared.state.stream_lines = {}
  shared.state.stream_reasoning_lines = {}
  shared.stream_cache = nil  -- Invalidate render cache
  -- Keep pending_user_message so it renders above the error
  shared.state.send_error = error_msg
  shared.state.autoscroll = false  -- Don't autoscroll to bottom, we want the top

  shared.persistent.is_streaming = false
  shared.persistent.stream_start_time = nil

  stop_spinner()

  -- Schedule render so it runs in a clean event loop context
  -- (this function is called from a jobstart on_stdout callback)
  vim.schedule(function()
    render.render()
    -- Scroll preview window to line 1 so the error block is visible
    if shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
      vim.api.nvim_win_set_cursor(shared.state.win, { 1, 0 })
    end
  end)
end

-- Show diff error warning in the preview pane
function M.show_diff_error(errors)
  shared.state.diff_error = errors
  stop_spinner()
  vim.schedule(render.render)
end

-- Clear diff error warning
function M.clear_diff_error()
  shared.state.diff_error = nil
  vim.schedule(render.render)
end

function M.stop_spinner()
  stop_spinner()
end

function M.start_spinner()
  start_spinner()
end

return M
