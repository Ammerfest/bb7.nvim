-- Streaming helpers for preview pane
local M = {}

local shared = require('bb7.panes.preview.shared')
local format = require('bb7.panes.preview.format')
local render = require('bb7.panes.preview.render')

-- Forward declaration for stop_spinner
local stop_spinner

-- Start the spinner animation timer
local function start_spinner()
  if shared.state.spinner_timer then return end -- Already running
  shared.state.spinner_frame = 1
  shared.state.spinner_timer = vim.fn.timer_start(80, function()
    if not shared.state.streaming then
      stop_spinner()
      return
    end
    shared.state.spinner_frame = (shared.state.spinner_frame % #shared.config.spinner_frames) + 1
    vim.schedule(render.render)
  end, { ['repeat'] = -1 })
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
  shared.state.autoscroll = true

  -- Track streaming in persistent state
  shared.persistent.is_streaming = true
  shared.persistent.stream_start_time = os.time()
  shared.persistent.last_usage = nil  -- Clear last usage when new stream starts
  shared.persistent.last_duration = nil

  start_spinner()
  render.render()
end

-- Append streaming content
function M.append_stream(content)
  if not shared.state.streaming then return end
  shared.state.stream_receiving = true

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

  render.render()
end

-- Append reasoning streaming content
function M.append_reasoning_stream(content)
  if not shared.state.streaming then return end
  -- Skip whitespace-only chunks to avoid rendering an empty reasoning block
  if not content or not content:match('%S') then return end
  shared.state.stream_receiving = true

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

  render.render()
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

function M.stop_spinner()
  stop_spinner()
end

function M.start_spinner()
  start_spinner()
end

return M
