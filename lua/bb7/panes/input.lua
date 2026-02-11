-- Input pane: message composition and sending
local M = {}

local client = require('bb7.client')
local utils = require('bb7.utils')
local log = require('bb7.log')

-- Reasoning effort levels (cycle order)
local REASONING_LEVELS = { 'none', 'low', 'medium', 'high' }
local REASONING_DISPLAY = {
  none   = '▱▱▱',
  low    = '▰▱▱',
  medium = '▰▰▱',
  high   = '▰▰▰',
}

local state = {
  buf = nil,
  win = nil,
  mode = 'normal',   -- 'normal' or 'insert'
  sending = false,   -- Whether we're currently sending/streaming
  chat_active = false, -- Whether a chat is selected/active
  on_message_sent = nil,  -- Callback when message is sent
  on_stream_chunk = nil,  -- Callback for streaming chunks
  on_stream_reasoning = nil, -- Callback for reasoning chunks
  on_stream_done = nil,   -- Callback when streaming completes
  on_stream_error = nil,  -- Callback when streaming fails with error
  on_mode_changed = nil,  -- Callback when vim mode changes (for hint updates)
  on_footer_changed = nil, -- Callback when footer content changes (reasoning toggle)
  check_send = nil,        -- Callback before send: returns error string to block, nil to allow
  estimate = nil,    -- Current token estimate { total, potential_savings }
  current_model = nil, -- Currently selected model ID
  reasoning_level = 'none', -- Current reasoning effort: 'none', 'low', 'medium', 'high'
  augroup = nil,     -- Autocmd group
  draft_timer = nil, -- Debounce timer for draft saving
  last_saved_draft = nil, -- Track last saved draft to avoid redundant saves
}

-- Get the current input content
local function get_content()
  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return ''
  end
  local lines = vim.api.nvim_buf_get_lines(state.buf, 0, -1, false)
  return table.concat(lines, '\n')
end

-- Clear the input buffer
local function clear_input()
  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return
  end
  vim.bo[state.buf].modifiable = true
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, { '' })
end

-- Send the current message
local function build_stream_handlers()
  return {
    on_chunk = function(chunk)
      if state.on_stream_chunk then
        state.on_stream_chunk(chunk)
      end
    end,
    on_reasoning = function(chunk)
      if state.on_stream_reasoning then
        state.on_stream_reasoning(chunk)
      end
    end,
    on_done = function(output_files, usage)
      state.sending = false
      if state.on_stream_done then
        state.on_stream_done(output_files, usage)
      end
    end,
    on_error = function(err)
      state.sending = false
      if state.on_stream_error then
        state.on_stream_error(err)
      else
        log.error(err)
        if state.on_stream_done then
          state.on_stream_done({})
        end
      end
    end,
  }
end

local function send_message()
  if state.sending then
    log.warn('Already sending')
    return
  end

  if not client.is_running() then
    log.error('BB-7 process not running')
    return
  end

  if not client.is_initialized() then
    log.error('BB-7 not initialized')
    return
  end

  if not state.chat_active then
    log.warn('No active chat yet')
    return
  end

  local content = vim.trim(get_content())
  if content == '' then
    return
  end

  -- Check if send is blocked (e.g., instruction parse errors)
  if state.check_send then
    local block_reason = state.check_send()
    if block_reason then
      log.warn(block_reason)
      return
    end
  end

  -- Check for applied files to notify LLM about
  local context_pane = require('bb7.panes.context')
  local applied_files = context_pane.get_applied_files()
  if #applied_files > 0 then
    -- Application events are recorded as structured history; avoid synthetic messages.
    context_pane.clear_applied_files()
  end

  state.sending = true

  -- Notify that we're starting to send (show original content in UI)
  if state.on_message_sent then
    state.on_message_sent(content)
  end

  -- Clear input and draft
  clear_input()
  state.last_saved_draft = ''
  -- Save empty draft to backend (non-blocking)
  if state.chat_active then
    client.request({ action = 'save_draft', draft = '' }, function() end)
  end

  -- Exit insert mode if in it
  if vim.fn.mode() == 'i' then
    vim.cmd('stopinsert')
  end

  -- Send to backend (with applied files note if any, and current model)
  local request = { action = 'send', content = content }
  if state.current_model then
    request.model = state.current_model
    -- Persist the model selection globally when a message is sent.
    require('bb7.models').persist_current()
  end
  -- Include reasoning effort if enabled
  if state.reasoning_level ~= 'none' then
    request.reasoning_effort = state.reasoning_level
  end
  client.stream(request, build_stream_handlers())
end

-- Render the input pane (shows placeholder when no chat selected)
local function render()
  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return
  end

  if not state.chat_active then
    -- Show centered message when no chat selected
    local win_width = 40
    local win_height = 5
    if state.win and vim.api.nvim_win_is_valid(state.win) then
      win_width = vim.api.nvim_win_get_width(state.win)
      win_height = vim.api.nvim_win_get_height(state.win)
    end

    local message = 'No chat selected'
    local text_width = vim.fn.strwidth(message)
    local left_pad = math.floor((win_width - text_width) / 2)
    local top_pad = math.floor((win_height - 1) / 2)

    local lines = {}
    for _ = 1, top_pad do
      table.insert(lines, '')
    end
    table.insert(lines, string.rep(' ', left_pad) .. message)

    vim.bo[state.buf].modifiable = true
    vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
    vim.bo[state.buf].modifiable = false

    -- Add highlight
    local ns = vim.api.nvim_create_namespace('bb7_input')
    vim.api.nvim_buf_clear_namespace(state.buf, ns, 0, -1)
    vim.api.nvim_buf_add_highlight(state.buf, ns, 'Comment', top_pad, 0, -1)
  end
end

-- Enter insert mode
local function enter_insert()
  if not state.win or not vim.api.nvim_win_is_valid(state.win) then
    return
  end

  -- Prevent insert mode if no chat is active
  if not state.chat_active then
    log.info('Select a chat first')
    return
  end

  state.mode = 'insert'
  vim.bo[state.buf].modifiable = true
  vim.cmd('startinsert')
end

-- Cycle reasoning effort level
local function cycle_reasoning()
  local models = require('bb7.models')
  if not models.supports_reasoning(state.current_model) then
    log.info('Model does not support reasoning')
    return
  end

  -- Find current index and cycle to next
  local current_idx = 1
  for i, level in ipairs(REASONING_LEVELS) do
    if level == state.reasoning_level then
      current_idx = i
      break
    end
  end
  local next_idx = (current_idx % #REASONING_LEVELS) + 1
  state.reasoning_level = REASONING_LEVELS[next_idx]

  -- Show feedback
  if state.reasoning_level == 'none' then
    log.info('Reasoning: off')
  else
    log.info('Reasoning: ' .. state.reasoning_level)
  end

  -- Update footer display
  if state.on_footer_changed then
    state.on_footer_changed()
  end
end

local function cancel_send()
  if not state.sending then
    log.info('No request in progress')
    return
  end
  state.sending = false
  client.cancel_active_stream()
end

-- Setup keymaps for this pane
function M.setup_keymaps(buf)
  local opts = { buffer = buf, nowait = true, silent = true }

  -- Full vim normal mode is available (i, a, A, I, o, O all work as expected)

  -- Send message with Enter (in normal mode)
  vim.keymap.set('n', '<CR>', send_message, opts)

  -- Check send key preference: "enter" or "shift_enter" (default)
  -- Set via vim.g.bb7_send_key in user's init.lua
  local send_key = vim.g.bb7_send_key or 'shift_enter'

  if send_key == 'enter' then
    -- Enter sends, Shift+Enter adds newline
    vim.keymap.set('i', '<CR>', function()
      vim.cmd('stopinsert')
      send_message()
    end, opts)
    -- S-CR inserts newline (need to manually handle since we override CR)
    vim.keymap.set('i', '<S-CR>', function()
      vim.api.nvim_put({ '', '' }, 'c', false, true)
    end, opts)
  else
    -- Shift+Enter sends (default), Enter adds newline
    vim.keymap.set('i', '<S-CR>', function()
      vim.cmd('stopinsert')
      send_message()
    end, opts)
  end

  -- Also support Ctrl+Enter (may not work in all terminals)
  vim.keymap.set('i', '<C-CR>', function()
    vim.cmd('stopinsert')
    send_message()
  end, opts)

  -- Escape in insert mode: just exit to normal mode (don't close BB7)
  vim.keymap.set('i', '<Esc>', function()
    vim.cmd('stopinsert')
  end, opts)

  -- Cancel current request
  vim.keymap.set('n', '<C-c>', cancel_send, opts)
  vim.keymap.set('i', '<C-c>', function()
    vim.cmd('stopinsert')
    cancel_send()
  end, opts)

  -- Model picker (M in normal mode)
  vim.keymap.set('n', 'M', function()
    local ui = require('bb7.ui')
    ui.open_model_picker()
  end, opts)

  -- Reasoning level toggle (R in normal mode)
  vim.keymap.set('n', 'R', cycle_reasoning, opts)

  -- Preview mode switching (normal mode only)
  local preview = require('bb7.panes.preview')
  vim.keymap.set('n', 'gc', function() preview.switch_to_chat() end, opts)
  vim.keymap.set('n', 'gf', function() preview.switch_to_file() end, opts)
  vim.keymap.set('n', 'gd', function() preview.switch_to_diff() end, opts)

  -- Scroll preview pane (works in both normal and insert mode)
  vim.keymap.set({ 'n', 'i' }, '<C-d>', function() preview.scroll_down() end, opts)
  vim.keymap.set({ 'n', 'i' }, '<C-u>', function() preview.scroll_up() end, opts)
end

-- Set callbacks for message events
function M.set_callbacks(callbacks)
  state.on_message_sent = callbacks.on_message_sent
  state.on_stream_chunk = callbacks.on_stream_chunk
  state.on_stream_reasoning = callbacks.on_stream_reasoning
  state.on_stream_done = callbacks.on_stream_done
  state.on_stream_error = callbacks.on_stream_error
  state.on_mode_changed = callbacks.on_mode_changed
  state.on_footer_changed = callbacks.on_footer_changed
  state.check_send = callbacks.check_send
end


-- Update the estimate display in window title
local function update_estimate_display()
  if not state.win or not vim.api.nvim_win_is_valid(state.win) then
    return
  end

  local title_parts = { ' [g5] Input' }

  if state.estimate then
    local est_str = ' • ~' .. utils.format_tokens(state.estimate.total) .. ' tokens'
    if state.estimate.potential_savings and state.estimate.potential_savings > 0 then
      est_str = est_str .. ' • Apply M files to save ' .. utils.format_tokens(state.estimate.potential_savings)
    end
    table.insert(title_parts, est_str)
  end

  local title = table.concat(title_parts)

  -- Update window title
  vim.api.nvim_win_set_config(state.win, {
    title = { { title, 'BB7TitleActive' } },
  })
end

-- Refresh token estimate from backend
function M.refresh_estimate()
  if not state.chat_active then
    state.estimate = nil
    update_estimate_display()
    return
  end
  client.request({ action = 'estimate_tokens' }, function(response, err)
    if err then
      state.estimate = nil
    else
      state.estimate = {
        total = response.total or 0,
        potential_savings = response.potential_savings or 0,
      }
    end
    update_estimate_display()
  end)
end

-- Save draft to backend (debounced)
local function save_draft_now()
  if not state.chat_active then
    return
  end
  local content = get_content()
  -- Skip if unchanged
  if content == state.last_saved_draft then
    return
  end
  state.last_saved_draft = content
  client.request({ action = 'save_draft', draft = content }, function(_, err)
    if err then
      -- Silently ignore errors - draft saving is best-effort
    end
  end)
end

-- Schedule draft save with debouncing (500ms delay)
local function schedule_draft_save()
  if state.draft_timer then
    vim.fn.timer_stop(state.draft_timer)
  end
  state.draft_timer = vim.fn.timer_start(500, function()
    state.draft_timer = nil
    vim.schedule(save_draft_now)
  end)
end

-- Initialize the pane
function M.init(buf, win)
  state.buf = buf
  state.win = win
  state.mode = 'normal'
  state.chat_active = false  -- Start with no chat active

  -- Check if streaming is actually in progress (client has active handlers)
  local c = require('bb7.client')
  state.sending = c.has_active_stream()
  if state.sending then
    c.set_stream_handlers(build_stream_handlers())
  end

  vim.bo[buf].buftype = 'nofile'

  -- Setup keymaps
  M.setup_keymaps(buf)

  -- Show initial placeholder (no chat selected)
  render()

  -- Setup autocmds
  state.augroup = vim.api.nvim_create_augroup('BB7Input', { clear = true })

  -- Update hints on mode change
  vim.api.nvim_create_autocmd('ModeChanged', {
    group = state.augroup,
    buffer = buf,
    callback = function()
      if state.on_mode_changed then
        state.on_mode_changed()
      end
    end,
  })

  -- Save draft on text change (debounced)
  vim.api.nvim_create_autocmd({ 'TextChanged', 'TextChangedI' }, {
    group = state.augroup,
    buffer = buf,
    callback = schedule_draft_save,
  })

  -- Trim leading whitespace when leaving the input pane
  vim.api.nvim_create_autocmd('WinLeave', {
    group = state.augroup,
    buffer = buf,
    callback = function()
      if not state.chat_active then
        return
      end
      if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
        return
      end
      local lines = vim.api.nvim_buf_get_lines(state.buf, 0, -1, false)
      local content = table.concat(lines, '\n')
      local trimmed = content:gsub('^%s+', '')  -- trim leading whitespace
      if trimmed ~= content then
        local new_lines = vim.split(trimmed, '\n', { plain = true })
        vim.bo[state.buf].modifiable = true
        vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, new_lines)
      end
    end,
  })

  -- Prevent insert mode when no chat is active
  vim.api.nvim_create_autocmd('InsertEnter', {
    group = state.augroup,
    buffer = buf,
    callback = function()
      if not state.chat_active then
        vim.schedule(function()
          vim.cmd('stopinsert')
          log.info('Select a chat first')
        end)
      end
    end,
  })
end

-- Focus and enter insert mode
function M.focus_insert()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    vim.api.nvim_set_current_win(state.win)
    enter_insert()
  end
end

-- Check if currently sending
function M.is_sending()
  return state.sending
end

-- Check if input buffer is empty
function M.is_empty()
  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return true
  end
  local lines = vim.api.nvim_buf_get_lines(state.buf, 0, -1, false)
  local content = table.concat(lines, '')
  return vim.trim(content) == ''
end

-- Get hints for this pane (dynamic based on vim mode)
function M.get_hints()
  if not state.chat_active then
    return 'Select a chat to start'
  end

  if state.sending then
    return 'Streaming... | Cancel: <C-c>'
  end

  local send_key = vim.g.bb7_send_key or 'shift_enter'

  local mode = vim.fn.mode()
  if mode == 'i' then
    -- Insert mode hints
    if send_key == 'enter' then
      return 'Send: <CR> | Newline: <S-CR> | Cancel: <C-c> | Normal: <Esc>'
    else
      return 'Send: <S-CR> | Cancel: <C-c> | Normal: <Esc>'
    end
  else
    -- Normal mode hints (show R only if model supports reasoning)
    local models = require('bb7.models')
    if models.supports_reasoning(state.current_model) then
      return 'Send: <CR> | Model: M | Cancel: <C-c> | Reasoning: R'
    else
      return 'Send: <CR> | Model: M | Cancel: <C-c>'
    end
  end
end

-- Set draft content (called when switching chats)
function M.set_draft(draft)
  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return
  end
  local content = draft or ''
  local lines = vim.split(content, '\n', { plain = true })
  vim.bo[state.buf].modifiable = true
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
  state.last_saved_draft = content
end

function M.set_chat_active(active)
  local was_active = state.chat_active
  state.chat_active = active == true

  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return
  end

  if state.chat_active and not was_active then
    -- Becoming active: make editable and clear placeholder
    vim.bo[state.buf].modifiable = true
    vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, { '' })
    local ns = vim.api.nvim_create_namespace('bb7_input')
    vim.api.nvim_buf_clear_namespace(state.buf, ns, 0, -1)
  elseif not state.chat_active then
    -- No chat: show placeholder
    render()
  end
end

-- Flush pending draft save immediately (called before chat switch or close)
function M.flush_draft()
  if state.draft_timer then
    vim.fn.timer_stop(state.draft_timer)
    state.draft_timer = nil
  end
  save_draft_now()
end

-- Cleanup
function M.cleanup()
  -- Flush any pending draft before cleanup
  M.flush_draft()

  if state.augroup then
    vim.api.nvim_del_augroup_by_id(state.augroup)
    state.augroup = nil
  end
  if state.draft_timer then
    vim.fn.timer_stop(state.draft_timer)
    state.draft_timer = nil
  end
  state.buf = nil
  state.win = nil
  state.mode = 'normal'
  state.sending = false
  state.estimate = nil
  state.current_model = nil
  state.reasoning_level = 'none'
  state.last_saved_draft = nil
  state.on_message_sent = nil
  state.on_stream_chunk = nil
  state.on_stream_reasoning = nil
  state.on_stream_done = nil
  state.on_stream_error = nil
  state.on_mode_changed = nil
  state.on_footer_changed = nil
  state.check_send = nil
end

-- Set current model
function M.set_model(model_id)
  state.current_model = model_id
end

-- Get current model
function M.get_model()
  return state.current_model
end

-- Set reasoning level (for screenshot mode)
function M.set_reasoning_level(level)
  state.reasoning_level = level or 'none'
end

-- Get footer text for display (shows reasoning indicator and current model)
function M.get_footer()
  if not state.current_model then
    return nil
  end

  local parts = {}

  -- Add reasoning indicator if model supports it
  local models = require('bb7.models')
  if models.supports_reasoning(state.current_model) then
    local indicator = REASONING_DISPLAY[state.reasoning_level] or '▱▱▱'
    table.insert(parts, indicator)
  end

  -- Shorten model ID for display (remove provider prefix if present)
  local display = state.current_model
  -- Truncate if too long
  if #display > 30 then
    display = display:sub(1, 28) .. '..'
  end
  table.insert(parts, display)

  -- Join with border-style separator
  return table.concat(parts, ' ─ ')
end

return M
