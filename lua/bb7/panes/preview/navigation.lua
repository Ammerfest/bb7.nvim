-- Navigation helpers for preview pane
local M = {}

local shared = require('bb7.panes.preview.shared')
local render = require('bb7.panes.preview.render')
local format = require('bb7.panes.preview.format')
local log = require('bb7.log')

-- Active message info popup (nil when no popup is open)
local active_popup_win = nil

-- Close the active info popup if it exists, returning focus to preview
local function close_info_popup()
  if active_popup_win and vim.api.nvim_win_is_valid(active_popup_win) then
    vim.api.nvim_win_close(active_popup_win, true)
  end
  active_popup_win = nil
  require('bb7.ui').set_picker_open(false)
  -- Return focus to preview pane
  if shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
    vim.api.nvim_set_current_win(shared.state.win)
  end
end

-- Toggle reasoning block collapse state at current cursor position
function M.toggle_reasoning()
  if not shared.state.win or not vim.api.nvim_win_is_valid(shared.state.win) then
    return
  end

  local cursor = vim.api.nvim_win_get_cursor(shared.state.win)
  local line_nr = cursor[1]  -- 1-indexed

  -- Check if current line belongs to a reasoning block
  local reasoning_id = shared.state.reasoning_line_map[line_nr]
  if not reasoning_id then
    return  -- Not on a reasoning block
  end

  -- Toggle collapsed state
  -- Default is collapsed (true), so we toggle to explicitly expanded (false) or back
  if shared.state.collapsed_reasoning[reasoning_id] == false then
    shared.state.collapsed_reasoning[reasoning_id] = true
  else
    shared.state.collapsed_reasoning[reasoning_id] = false
  end

  -- Re-render and try to restore cursor position
  render.render()

  -- Keep cursor on the same line if possible
  local line_count = vim.api.nvim_buf_line_count(shared.state.buf)
  if line_nr > line_count then
    line_nr = line_count
  end
  vim.api.nvim_win_set_cursor(shared.state.win, { line_nr, 0 })
end

-- Jump to previous anchor in a given list
function M.jump_prev_anchor(anchor_list)
  if not shared.state.win or not vim.api.nvim_win_is_valid(shared.state.win) then
    return
  end
  if #anchor_list == 0 then return end

  local cursor = vim.api.nvim_win_get_cursor(shared.state.win)
  local current_line = cursor[1]

  local target = nil
  for _, anchor in ipairs(anchor_list) do
    if anchor < current_line then
      target = anchor
    else
      break
    end
  end
  if not target then
    target = anchor_list[#anchor_list]  -- wrap to last
  end

  vim.api.nvim_win_set_cursor(shared.state.win, { target, 0 })
end

-- Jump to next anchor in a given list
function M.jump_next_anchor(anchor_list)
  if not shared.state.win or not vim.api.nvim_win_is_valid(shared.state.win) then
    return
  end
  if #anchor_list == 0 then return end

  local cursor = vim.api.nvim_win_get_cursor(shared.state.win)
  local current_line = cursor[1]

  local target = nil
  for _, anchor in ipairs(anchor_list) do
    if anchor > current_line then
      target = anchor
      break
    end
  end
  if not target then
    target = anchor_list[1]  -- wrap to first
  end

  vim.api.nvim_win_set_cursor(shared.state.win, { target, 0 })
end

-- Get the user message index of the current cursor position
local function get_current_user_msg_idx()
  if not shared.state.win or not vim.api.nvim_win_is_valid(shared.state.win) then
    return nil
  end

  local cursor = vim.api.nvim_win_get_cursor(shared.state.win)
  local current_line = cursor[1]

  -- Find the largest user anchor line that is <= current line
  local target_anchor = nil
  for _, anchor in ipairs(shared.state.user_anchor_lines) do
    if anchor <= current_line then
      target_anchor = anchor
    else
      break
    end
  end

  if target_anchor then
    return shared.state.user_anchor_msg_idx[target_anchor]
  end
  return nil
end

-- Get the anchor line for a given user message index
local function get_user_msg_anchor_line(msg_idx)
  for anchor_line, idx in pairs(shared.state.user_anchor_msg_idx) do
    if idx == msg_idx then
      return anchor_line
    end
  end
  return nil
end

-- Handle fork action: create a new chat from current user message
function M.fork_chat()
  local panes_input = require('bb7.panes.input')

  -- Check if currently streaming
  if panes_input.is_sending() then
    log.warn('Cannot fork while streaming')
    return
  end

  local msg_idx = get_current_user_msg_idx()
  if not msg_idx then
    log.warn('Navigate to a user message to fork')
    return
  end

  if not shared.state.chat then
    log.warn('No chat loaded')
    return
  end

  -- Move cursor to the fork message and highlight it
  local anchor_line = get_user_msg_anchor_line(msg_idx)
  local highlight_ns = vim.api.nvim_create_namespace('bb7_fork_highlight')

  if anchor_line and shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
    vim.api.nvim_win_set_cursor(shared.state.win, { anchor_line, 0 })
    -- Add temporary highlight with high priority to override custom styling
    vim.api.nvim_buf_set_extmark(shared.state.buf, highlight_ns, anchor_line - 1, 0, {
      end_row = anchor_line - 1,
      end_col = 0,
      hl_group = 'Visual',
      hl_eol = true,
      line_hl_group = 'Visual',
      priority = 10000,  -- High priority to override other highlights
    })
    -- Redraw to show the highlight before prompting
    vim.cmd('redraw')
  end

  -- Ask for confirmation (use vim.fn.input to avoid triggering telescope)
  local confirm = vim.fn.input('Fork from this message? (y/n): ')

  -- Clear the highlight and command line
  vim.api.nvim_buf_clear_namespace(shared.state.buf, highlight_ns, 0, -1)
  vim.cmd('echo ""')

  if confirm ~= 'y' and confirm ~= 'Y' then
    log.info('Fork cancelled')
    return
  end

  -- Flush any unsaved draft before forking
  panes_input.flush_draft()

  local client = require('bb7.client')
  client.request({
    action = 'fork_chat',
    chat_id = shared.state.chat.id,
    fork_message_index = msg_idx - 1,  -- Convert to 0-indexed
  }, function(response, err)
    if err then
      log.error('Fork failed: ' .. err)
      return
    end

    local warnings = response.context_warnings
    if type(warnings) == 'table' and #warnings > 0 then
      log.info('Fork created with ' .. #warnings .. ' context note(s)')
    else
      log.info('Fork created')
    end

    -- Switch to new chat and focus input pane
    local ui = require('bb7.ui')
    ui.switch_chat(response.new_chat_id, function()
      ui.focus_input()
    end)
  end)
end

-- Edit a user message in-place (truncate below and move to draft)
function M.edit_chat_message()
  local panes_input = require('bb7.panes.input')

  if panes_input.is_sending() then
    log.warn('Cannot edit while streaming')
    return
  end

  local msg_idx = get_current_user_msg_idx()
  if not msg_idx then
    log.warn('Navigate to a user message to edit')
    return
  end

  if not shared.state.chat or not shared.state.chat.messages then
    log.warn('No chat loaded')
    return
  end

  local msg = shared.state.chat.messages[msg_idx]
  if not msg or msg.role ~= 'user' then
    log.warn('Navigate to a user message to edit')
    return
  end

  local anchor_line = get_user_msg_anchor_line(msg_idx)
  local highlight_ns = vim.api.nvim_create_namespace('bb7_edit_highlight')

  if anchor_line and shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
    vim.api.nvim_win_set_cursor(shared.state.win, { anchor_line, 0 })
    vim.api.nvim_buf_set_extmark(shared.state.buf, highlight_ns, anchor_line - 1, 0, {
      end_row = anchor_line - 1,
      end_col = 0,
      hl_group = 'Visual',
      hl_eol = true,
      line_hl_group = 'Visual',
      priority = 10000,
    })
    vim.cmd('redraw')
  end

  local confirm = vim.fn.input('Edit this message and discard everything below? (y/n): ')

  vim.api.nvim_buf_clear_namespace(shared.state.buf, highlight_ns, 0, -1)
  vim.cmd('echo ""')

  if confirm ~= 'y' and confirm ~= 'Y' then
    log.info('Edit cancelled')
    return
  end

  panes_input.flush_draft()

  local client = require('bb7.client')
  client.request({
    action = 'chat_edit',
    chat_id = shared.state.chat.id,
    message_index = msg_idx - 1,
    content = msg.content or '',
  }, function(response, err)
    if err then
      log.error('Edit failed: ' .. err)
      return
    end

    local warnings = response.context_warnings
    if type(warnings) == 'table' and #warnings > 0 then
      log.info('Edit restored with ' .. #warnings .. ' context note(s)')
    else
      log.info('Edit applied')
    end

    client.request({ action = 'chat_get' }, function(chat, chat_err)
      if chat_err then
        log.error('Failed to refresh chat: ' .. chat_err)
        return
      end

      local panes_preview = require('bb7.panes.preview')
      local panes_context = require('bb7.panes.context')
      local panes_provider = require('bb7.panes.provider')
      local ui = require('bb7.ui')

      panes_preview.set_chat(chat)
      panes_context.set_chat(chat)
      panes_provider.set_chat(chat)
      panes_input.set_chat_active(true)
      panes_input.set_draft(chat.draft or '')
      panes_input.refresh_estimate()
      ui.focus_input()
    end)
  end)
end

-- Get text content from a message (handles parts vs legacy content)
local function get_message_text(msg)
  if msg.parts and #msg.parts > 0 then
    for _, part in ipairs(msg.parts) do
      if part.type == 'text' and part.content and part.content ~= '' then
        return part.content
      end
    end
    return ''
  end
  return msg.content or ''
end

-- Truncate text to max_len chars, adding "..." if truncated
local function truncate(text, max_len)
  if not text or text == '' then return '—' end
  -- Replace newlines with spaces for single-line display
  text = text:gsub('\n', ' ')
  if #text > max_len then
    return text:sub(1, max_len) .. '...'
  end
  return text
end

-- Format ISO timestamp for popup display
local function format_timestamp(timestamp)
  if not timestamp then return '—' end
  local pattern = '(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)'
  local year, month, day, hour, min, sec = timestamp:match(pattern)
  if year then
    return string.format('%s-%s-%s, %s:%s:%s', year, month, day, hour, min, sec)
  end
  return timestamp
end

-- Determine reasoning info for a message
local function get_reasoning_info(msg)
  -- Explicit reasoning_effort field
  if msg.reasoning_effort and msg.reasoning_effort ~= '' then
    return msg.reasoning_effort
  end
  -- Infer from thinking parts
  if msg.parts then
    for _, part in ipairs(msg.parts) do
      if part.type == 'thinking' then
        return 'yes'
      end
    end
  end
  return 'none'
end

-- Show message info popup for the message pair under cursor
-- First K opens the popup; second K enters it for navigation/yank
function M.show_message_info()
  -- If popup already exists, enter it
  if active_popup_win and vim.api.nvim_win_is_valid(active_popup_win) then
    local ui = require('bb7.ui')
    ui.set_picker_open(true)
    vim.api.nvim_set_current_win(active_popup_win)
    return
  end

  if not shared.state.win or not vim.api.nvim_win_is_valid(shared.state.win) then
    return
  end
  if not shared.state.chat or not shared.state.chat.messages then
    return
  end

  local cursor = vim.api.nvim_win_get_cursor(shared.state.win)
  local current_line = cursor[1]

  -- Find nearest anchor <= cursor line and look up msg_idx
  local target_anchor = nil
  for _, anchor in ipairs(shared.state.anchor_lines) do
    if anchor <= current_line then
      target_anchor = anchor
    else
      break
    end
  end

  if not target_anchor then return end

  local msg_idx = shared.state.anchor_msg_idx[target_anchor]
  if not msg_idx then return end

  local messages = shared.state.chat.messages

  -- Find the user+assistant pair
  local user_msg, assistant_msg
  local msg = messages[msg_idx]
  if not msg then return end

  if msg.role == 'user' then
    user_msg = msg
    -- Next message should be assistant
    if msg_idx + 1 <= #messages and messages[msg_idx + 1].role == 'assistant' then
      assistant_msg = messages[msg_idx + 1]
    end
  elseif msg.role == 'assistant' then
    assistant_msg = msg
    -- Previous message should be user
    if msg_idx - 1 >= 1 and messages[msg_idx - 1].role == 'user' then
      user_msg = messages[msg_idx - 1]
    end
  else
    return  -- system message, skip
  end

  -- Build popup lines
  local info_lines = {}
  local label_width = 12  -- Width of the label column

  local function add_row(label, value)
    table.insert(info_lines, string.format('%-' .. label_width .. 's %s', label .. ':', value or '—'))
  end

  -- Timestamp (use assistant if available, otherwise user)
  local ts_msg = assistant_msg or user_msg
  if ts_msg then
    add_row('Timestamp', format_timestamp(ts_msg.timestamp))
  end

  -- Message preview (user)
  if user_msg then
    add_row('Message', truncate(get_message_text(user_msg), 50))
  end

  -- Response preview (assistant)
  if assistant_msg then
    add_row('Response', truncate(get_message_text(assistant_msg), 50))
  end

  -- Model
  local model = (assistant_msg and assistant_msg.model) or (user_msg and user_msg.model) or nil
  add_row('Model', model or '—')

  -- Reasoning
  if assistant_msg then
    add_row('Reasoning', get_reasoning_info(assistant_msg))
  end

  -- Usage info (from assistant message)
  if assistant_msg and assistant_msg.usage then
    local usage = assistant_msg.usage
    add_row('Tokens In', format.format_tokens_short(usage.prompt_tokens or 0))
    add_row('Tokens Out', format.format_tokens_short(usage.completion_tokens or 0))
    if usage.cost and usage.cost > 0 then
      add_row('Cost', string.format('$%.4f', usage.cost))
    else
      add_row('Cost', '—')
    end
    if usage.duration and usage.duration > 0 then
      add_row('Duration', format.format_duration(math.floor(usage.duration)))
    else
      add_row('Duration', '—')
    end
  end

  if #info_lines == 0 then return end

  -- Calculate popup dimensions
  local max_width = 0
  for _, line in ipairs(info_lines) do
    local w = vim.fn.strwidth(line)
    if w > max_width then max_width = w end
  end

  -- Add padding
  local pad = 1
  for i, line in ipairs(info_lines) do
    info_lines[i] = string.rep(' ', pad) .. line .. string.rep(' ', pad)
  end
  max_width = max_width + pad * 2

  -- Create float buffer
  local float_buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(float_buf, 0, -1, false, info_lines)
  vim.bo[float_buf].modifiable = false
  vim.bo[float_buf].bufhidden = 'wipe'

  -- Position: relative to cursor, zindex above BB-7 UI (panes=50, scrollbars=100)
  local float_win = vim.api.nvim_open_win(float_buf, false, {
    relative = 'cursor',
    row = 1,
    col = 0,
    width = max_width,
    height = #info_lines,
    style = 'minimal',
    border = 'rounded',
    zindex = 200,
  })

  vim.api.nvim_set_option_value('winhl', 'Normal:NormalFloat,FloatBorder:FloatBorder', { win = float_win })

  -- Track active popup
  active_popup_win = float_win

  -- Setup keymaps inside popup (q to close, K to close)
  local popup_opts = { buffer = float_buf, nowait = true, silent = true }
  vim.keymap.set('n', 'q', function() close_info_popup() end, popup_opts)
  vim.keymap.set('n', 'K', function() close_info_popup() end, popup_opts)
  vim.keymap.set('n', '<Esc>', function() close_info_popup() end, popup_opts)

  -- Auto-close when leaving the popup window
  local augroup = vim.api.nvim_create_augroup('BB7MessageInfoPopup', { clear = true })
  vim.api.nvim_create_autocmd('WinClosed', {
    group = augroup,
    pattern = tostring(float_win),
    once = true,
    callback = function()
      active_popup_win = nil
      require('bb7.ui').set_picker_open(false)
      pcall(vim.api.nvim_del_augroup_by_id, augroup)
    end,
  })

  -- Close popup when cursor moves in preview buffer (user scrolled away)
  vim.api.nvim_create_autocmd('CursorMoved', {
    group = augroup,
    buffer = shared.state.buf,
    once = true,
    callback = function() close_info_popup() end,
  })
end

return M
