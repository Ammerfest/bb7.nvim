-- Chats pane: list and manage chat sessions
local M = {}

local client = require('bb7.client')
local log = require('bb7.log')

-- Pinned chats set (chat_id -> true), per-project
local pinned_chats = {}
local current_project_root = nil
local viewing_global = false  -- true when viewing global chats

-- Get pinned chats path for current project or global
local function get_pinned_path()
  if viewing_global then
    local home = os.getenv('HOME')
    if not home then return nil end
    return home .. '/.bb7/pinned_chats.json'
  end
  local project_root = current_project_root or client.get_project_root()
  if not project_root then
    return nil
  end
  return project_root .. '/.bb7/pinned_chats.json'
end

-- Load pinned chats from disk (per-project)
local function load_pinned()
  pinned_chats = {}
  local path = get_pinned_path()
  if not path then
    return
  end

  local file = io.open(path, 'r')
  if not file then
    return
  end

  local content = file:read('*a')
  file:close()

  local ok, data = pcall(vim.json.decode, content)
  if ok and data and data.chats then
    for _, id in ipairs(data.chats) do
      pinned_chats[id] = true
    end
  end
end

-- Save pinned chats to disk (per-project)
local function save_pinned()
  local path = get_pinned_path()
  if not path then
    return
  end

  -- Convert set to list
  local list = {}
  for id, _ in pairs(pinned_chats) do
    table.insert(list, id)
  end
  table.sort(list)

  local data = { chats = list }
  local json = vim.json.encode(data)

  local file = io.open(path, 'w')
  if file then
    file:write(json)
    file:close()
  end
end

-- Check if a chat is pinned
local function is_pinned(chat_id)
  return pinned_chats[chat_id] == true
end

-- Toggle pinned status for a chat
local function toggle_pin(chat_id)
  if pinned_chats[chat_id] then
    pinned_chats[chat_id] = nil
  else
    pinned_chats[chat_id] = true
  end
  save_pinned()
end

-- Pane state
local state = {
  buf = nil,
  win = nil,
  chats = {},       -- List of chat summaries from backend
  selected_idx = 1, -- Currently highlighted chat (cursor position)
  active_idx = nil, -- Currently active/selected chat (marked with ●), nil if none
  active_chat = nil, -- Full chat data for active chat
  augroup = nil,
  on_before_chat_switch = nil, -- Callback before switching chats (to flush draft)
  on_chat_selected = nil, -- Callback when chat is selected
  on_chat_created = nil,  -- Callback when new chat is created
}

-- Truncate string to fit within width (accounting for display width)
local function truncate(str, max_width)
  local width = vim.fn.strwidth(str)
  if width <= max_width then
    return str
  end
  -- Binary search for the right truncation point
  local len = #str
  while len > 0 and vim.fn.strwidth(str:sub(1, len)) > max_width - 1 do
    len = len - 1
  end
  return str:sub(1, len) .. '…'
end

local function truncate_prompt(str, max_width)
  local width = vim.fn.strwidth(str)
  if width <= max_width then
    return str
  end

  local suffix = '...'
  local suffix_width = vim.fn.strwidth(suffix)
  local target_width = max_width - suffix_width
  if target_width < 1 then
    return suffix
  end

  local len = #str
  while len > 0 and vim.fn.strwidth(str:sub(1, len)) > target_width do
    len = len - 1
  end
  return str:sub(1, len) .. suffix
end

local function sync_selection_with_cursor()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    local cursor = vim.api.nvim_win_get_cursor(state.win)
    if cursor[1] >= 1 and cursor[1] <= #state.chats then
      state.selected_idx = cursor[1]
    end
  end
end

-- Render the chat list
local function render()
  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return
  end

  -- Get window width for truncation
  local win_width = 30  -- fallback
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    win_width = vim.api.nvim_win_get_width(state.win)
  end

  local lines = {}
  local highlights = {}

  for i, chat in ipairs(state.chats) do
    -- Active chat marker (only if a chat is actually selected) and pinned indicator
    local is_active = state.active_idx and i == state.active_idx
    local is_locked = chat.locked
    local marker = is_active and '●' or ' '
    local pin = is_pinned(chat.id) and '!' or ' '
    local lock = is_locked and '⊘' or ' '
    local prefix = pin .. marker .. lock .. ' '
    local prefix_width = vim.fn.strwidth(prefix)
    local name_max_width = win_width - prefix_width
    local name = truncate(chat.name, name_max_width)
    local line = prefix .. name
    table.insert(lines, line)

    -- Highlight the active marker (no background so cursorline shows through)
    if is_active then
      table.insert(highlights, {
        line = i - 1,
        col_start = 1,
        col_end = 1 + #'●',
        hl = 'BB7MarkerActive',
      })
    end
    -- Highlight the pin indicator
    if is_pinned(chat.id) then
      table.insert(highlights, {
        line = i - 1,
        col_start = 0,
        col_end = 1,  -- Just the '!'
        hl = 'DiagnosticWarn',
      })
    end
    -- Highlight locked chats
    if is_locked then
      local name_start = #prefix
      table.insert(highlights, {
        line = i - 1,
        col_start = name_start,
        col_end = #line,
        hl = 'Comment',
      })
    end
  end

  vim.bo[state.buf].modifiable = true
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
  vim.bo[state.buf].modifiable = false

  -- Apply highlights
  local ns = vim.api.nvim_create_namespace('bb7_chats')
  vim.api.nvim_buf_clear_namespace(state.buf, ns, 0, -1)

  for _, hl in ipairs(highlights) do
    vim.api.nvim_buf_add_highlight(state.buf, ns, hl.hl, hl.line, hl.col_start, hl.col_end)
  end
  -- Note: cursorline is managed by ui.lua's update_pane_borders()
end

-- Move selection up/down
local function move_selection(delta)
  if #state.chats == 0 then return end

  local new_idx = state.selected_idx + delta

  -- Clamp to valid range (no wrap)
  if new_idx < 1 then
    new_idx = 1
  elseif new_idx > #state.chats then
    new_idx = #state.chats
  end

  state.selected_idx = new_idx

  -- Move cursor (always column 0)
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    vim.api.nvim_win_set_cursor(state.win, { state.selected_idx, 0 })
  end
end

-- Keep cursor at column 0
local function lock_cursor_column()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    local cursor = vim.api.nvim_win_get_cursor(state.win)
    if cursor[2] ~= 0 then
      vim.api.nvim_win_set_cursor(state.win, { cursor[1], 0 })
    end
    -- Also sync selection with cursor row
    if cursor[1] ~= state.selected_idx and cursor[1] <= #state.chats then
      state.selected_idx = cursor[1]
    end
  end
end

-- Select the highlighted chat (make it active)
local function select_chat(callback)
  if #state.chats == 0 then return end

  local chat = state.chats[state.selected_idx]
  if not chat then return end

  -- Block switching while streaming
  local panes_input = require('bb7.panes.input')
  if panes_input.is_sending() then
    log.error('Cannot switch chats while streaming')
    return
  end

  -- Flush draft before switching
  if state.on_before_chat_switch then
    state.on_before_chat_switch()
  end

  -- Block selecting locked chats
  if chat.locked then
    log.warn('Chat is locked by another instance')
    M.refresh()
    return
  end

  -- Select in backend
  local req = { action = 'chat_select', id = chat.id }
  if viewing_global then req.global = true end
  client.request(req, function(response, err)
    if err then
      if err:find('locked') then
        log.warn('Chat is locked by another instance')
      else
        log.error('Failed to select chat: ' .. tostring(err))
      end
      M.refresh()
      return
    end

    -- Get full chat data
    client.request({ action = 'chat_get' }, function(chat_response, chat_err)
      if chat_err then
        log.error('Failed to get chat: ' .. tostring(chat_err))
        return
      end

      state.active_idx = state.selected_idx
      state.active_chat = chat_response
      M.refresh()

      if state.on_chat_selected then
        state.on_chat_selected(chat_response)
      end

      if callback then
        callback(chat_response)
      end
    end)
  end)
end

-- Create a new chat
function M.new_chat()
  -- Generate name with current timestamp
  local name = 'Untitled chat - ' .. os.date('%Y-%m-%d %H:%M')

  -- Check if client is running
  if not client.is_initialized() then
    log.warn('Not initialized')
    return
  end

  local req = { action = 'chat_new', name = name }
  local current_model = require('bb7.models').get_current()
  if current_model then
    req.model = current_model
  end
  if viewing_global then req.global = true end
  client.request(req, function(response, err)
    if err then
      log.error('Failed to create chat: ' .. err)
      return
    end

    if not response or not response.id then
      log.error('Invalid response from chat_new')
      return
    end

    log.info('Created: ' .. name)

    -- Refresh chat list and select the new chat
    M.refresh(function()
      -- Find and select the new chat
      for i, chat in ipairs(state.chats) do
        if chat.id == response.id then
          state.selected_idx = i
          select_chat()
          break
        end
      end

      if state.on_chat_created then
        state.on_chat_created(response.id)
      end
    end)
  end)
end

-- Rename the selected chat
local function rename_chat()
  if #state.chats == 0 then return end
  sync_selection_with_cursor()
  local chat = state.chats[state.selected_idx]
  if not chat then return end

  local function prompt_rename(target_chat)
    local display_name = truncate_prompt(target_chat.name or '', 50)
    local prompt = string.format('New name for chat "%s": ', display_name)
    vim.fn.inputsave()
    local new_name = vim.fn.input({ prompt = prompt })
    vim.fn.inputrestore()
    if not new_name or new_name == '' then
      return
    end
    if new_name == target_chat.name then
      return
    end

    local req = { action = 'chat_rename', id = target_chat.id, name = new_name }
    if viewing_global then req.global = true end
    client.request(req, function(_, err)
      if err then
        log.error('Failed to rename chat: ' .. tostring(err))
        return
      end
      if state.active_chat and state.active_chat.id == target_chat.id then
        state.active_chat.name = new_name
      end
      M.refresh()
    end)
  end

  if state.active_chat and state.active_chat.id == chat.id then
    prompt_rename(chat)
    return
  end

  select_chat(prompt_rename)
end

-- Delete the selected chat
local function delete_chat()
  if #state.chats == 0 then return end
  sync_selection_with_cursor()
  local chat = state.chats[state.selected_idx]
  if not chat then return end

  local function prompt_delete(target_chat)
    local display_name = truncate_prompt(target_chat.name or '', 50)
    local prompt = string.format('Delete chat "%s"?', display_name)
    local confirmed = vim.fn.confirm(prompt, '&Yes\n&No', 2)
    if confirmed ~= 1 then
      return
    end

    local req = { action = 'chat_delete', id = target_chat.id }
    if viewing_global then req.global = true end
    client.request(req, function(_, err)
      if err then
        log.error('Failed to delete chat: ' .. tostring(err))
        return
      end

      state.active_chat = nil
      state.active_idx = nil
      if state.on_chat_selected then
        state.on_chat_selected(nil)
      end
      M.refresh()
    end)
  end

  if state.active_chat and state.active_chat.id == chat.id then
    prompt_delete(chat)
    return
  end

  select_chat(prompt_delete)
end

-- Toggle pin status for selected chat
local function toggle_pin_selected()
  if #state.chats == 0 then return end
  local chat = state.chats[state.selected_idx]
  if not chat then return end

  toggle_pin(chat.id)
  -- Re-sort and re-render
  M.refresh()
end

-- Toggle between project and global chats
local function toggle_mode()
  if client.is_global_only() then
    return -- Can't toggle in global-only mode
  end
  viewing_global = not viewing_global
  state.selected_idx = 1
  state.active_idx = nil
  state.active_chat = nil
  -- Reload pinned chats for the new mode
  load_pinned()
  -- Notify that chat was deselected
  if state.on_chat_selected then
    state.on_chat_selected(nil)
  end
  M.refresh()
  -- Notify data changed (for footer/title updates)
  if state.on_data_changed then
    state.on_data_changed()
  end
end

-- Force unlock the selected chat
local function force_unlock()
  if #state.chats == 0 then return end
  sync_selection_with_cursor()
  local chat = state.chats[state.selected_idx]
  if not chat or not chat.locked then return end

  local display_name = truncate_prompt(chat.name or '', 50)
  local prompt = string.format('Force unlock chat "%s"?', display_name)
  local confirmed = vim.fn.confirm(prompt, '&Yes\n&No', 2)
  if confirmed ~= 1 then return end

  local req = { action = 'chat_force_unlock', id = chat.id }
  if viewing_global then req.global = true end
  client.request(req, function(_, err)
    if err then
      log.error('Failed to unlock: ' .. tostring(err))
      return
    end
    log.info('Chat unlocked')
    M.refresh()
  end)
end

-- Clone context to new chat
-- Refresh the chat list from backend
function M.refresh(callback)
  -- Capture active chat ID before refresh (from active_chat or current list)
  local active_chat_id = nil
  if state.active_chat and state.active_chat.id then
    active_chat_id = state.active_chat.id
  elseif #state.chats > 0 and type(state.active_idx) == 'number' and state.active_idx >= 1 and state.active_idx <= #state.chats then
    active_chat_id = state.chats[state.active_idx].id
  end

  local req = { action = 'chat_list' }
  if viewing_global then req.global = true end
  client.request(req, function(response, err)
    if err then
      log.error('Failed to list chats: ' .. tostring(err))
      if callback then callback() end
      return
    end

    -- Handle null/nil from JSON (becomes vim.NIL userdata)
    state.chats = (type(response.chats) == 'table') and response.chats or {}

    -- Sort chats: pinned first, then preserve backend order (usually by modified date)
    -- Add original index for stable sorting
    for i, chat in ipairs(state.chats) do
      chat._sort_idx = i
    end
    table.sort(state.chats, function(a, b)
      local a_pinned = is_pinned(a.id)
      local b_pinned = is_pinned(b.id)
      if a_pinned ~= b_pinned then
        return a_pinned  -- pinned chats come first
      end
      -- For chats with same pinned status, preserve original order
      return a._sort_idx < b._sort_idx
    end)
    -- Clean up temporary sort indices
    for _, chat in ipairs(state.chats) do
      chat._sort_idx = nil
    end

    -- Find the active chat's new position after sorting
    if active_chat_id then
      for i, chat in ipairs(state.chats) do
        if chat.id == active_chat_id then
          state.active_idx = i
          break
        end
      end
    end

    -- Clamp indices to valid range (fallback if chat was deleted)
    if not state.selected_idx or state.selected_idx < 1 then
      state.selected_idx = 1
    end
    state.selected_idx = math.min(state.selected_idx, math.max(1, #state.chats))
    -- Only clamp active_idx if it's set (nil means no selection)
    if type(state.active_idx) == 'number' then
      state.active_idx = math.min(state.active_idx, math.max(1, #state.chats))
    else
      state.active_idx = nil
    end

    render()

    -- Position cursor
    if state.win and vim.api.nvim_win_is_valid(state.win) and #state.chats > 0 then
      vim.api.nvim_win_set_cursor(state.win, { state.selected_idx, 0 })
    end

    -- Notify that data changed (for footer updates)
    if state.on_data_changed then
      state.on_data_changed()
    end

    if callback then callback() end
  end)
end

-- Setup keymaps for this pane
function M.setup_keymaps(buf)
  local opts = { buffer = buf, nowait = true, silent = true }

  -- Navigation
  vim.keymap.set('n', 'j', function() move_selection(1) end, opts)
  vim.keymap.set('n', 'k', function() move_selection(-1) end, opts)

  -- Actions
  vim.keymap.set('n', '<CR>', select_chat, opts)
  vim.keymap.set('n', 'n', M.new_chat, opts)
  vim.keymap.set('n', 'r', rename_chat, opts)
  vim.keymap.set('n', 'd', delete_chat, opts)
  vim.keymap.set('n', 'p', toggle_pin_selected, opts)
  vim.keymap.set('n', '<C-s>', toggle_mode, opts)
  vim.keymap.set('n', 'u', force_unlock, opts)
end

-- Set callbacks
function M.set_callbacks(callbacks)
  state.on_before_chat_switch = callbacks.on_before_chat_switch
  state.on_chat_selected = callbacks.on_chat_selected
  state.on_chat_created = callbacks.on_chat_created
  state.on_data_changed = callbacks.on_data_changed
end

-- Initialize the pane with a buffer and window
function M.init(buf, win)
  state.buf = buf
  state.win = win
  state.chats = {}
  -- Preserve selected_idx if already set (for session persistence)
  if not state.selected_idx or state.selected_idx < 1 then
    state.selected_idx = 1
  end
  -- Preserve active_idx and active_chat (don't reset to nil if already set)

  -- Disable line wrapping - truncate instead
  vim.wo[win].wrap = false

  -- Setup keymaps (after common keymaps, so these override disabled keys)
  M.setup_keymaps(buf)

  -- Setup autocmd to lock cursor to column 0
  state.augroup = vim.api.nvim_create_augroup('BB7Chats', { clear = true })
  vim.api.nvim_create_autocmd('CursorMoved', {
    group = state.augroup,
    buffer = buf,
    callback = lock_cursor_column,
  })

  -- Initial render (empty, will be populated by refresh)
  render()
end

-- Auto-create a chat if none exist
function M.ensure_chat_exists(callback)
  if #state.chats > 0 then
    if callback then callback(false) end -- false = didn't create new
    return
  end

  -- Create new chat
  local name = 'Untitled chat - ' .. os.date('%Y-%m-%d %H:%M')

  local req = { action = 'chat_new', name = name }
  local current_model = require('bb7.models').get_current()
  if current_model then
    req.model = current_model
  end
  if viewing_global then req.global = true end
  client.request(req, function(response, err)
    if err then
      log.error('Failed to create chat: ' .. tostring(err))
      if callback then callback(false) end
      return
    end

    -- Refresh and select
    M.refresh(function()
      for i, chat in ipairs(state.chats) do
        if chat.id == response.id then
          state.selected_idx = i
          state.active_idx = i
          break
        end
      end

      -- Get full chat data
      local sel_req = { action = 'chat_select', id = response.id }
      if viewing_global then sel_req.global = true end
      client.request(sel_req, function(_, sel_err)
        if sel_err then
          if callback then callback(true) end
          return
        end

        client.request({ action = 'chat_get' }, function(chat_response, chat_err)
          if not chat_err then
            state.active_chat = chat_response
            if state.on_chat_selected then
              state.on_chat_selected(chat_response)
            end
          end
          if callback then callback(true) end -- true = created new
        end)
      end)
    end)
  end)
end

-- Get the shortcut hints for this pane
function M.get_hints()
  local parts = {}
  -- Show scope toggle hint only when toggling is possible (not global-only)
  if not client.is_global_only() then
    local toggle_label = viewing_global and 'Show Project' or 'Show Global'
    table.insert(parts, toggle_label .. ': <C-s>')
  end
  table.insert(parts, 'Select: <CR>')
  table.insert(parts, 'New: n')
  table.insert(parts, 'Pin: p')
  table.insert(parts, 'Rename: r')
  table.insert(parts, 'Delete: d')
  -- Show unlock hint if any chat in view is locked
  for _, chat in ipairs(state.chats) do
    if chat.locked then
      table.insert(parts, 'Unlock: u')
      break
    end
  end
  return table.concat(parts, ' | ')
end

-- Cleanup
function M.cleanup()
  if state.augroup then
    vim.api.nvim_del_augroup_by_id(state.augroup)
    state.augroup = nil
  end
  state.buf = nil
  state.win = nil
  state.on_before_chat_switch = nil
  state.on_chat_selected = nil
  state.on_chat_created = nil
  state.on_data_changed = nil
end

-- Get currently active chat
function M.get_active_chat()
  return state.active_chat
end

-- Get chat count
function M.get_chat_count()
  return #state.chats
end

-- Get selection info for footer display
function M.get_selection_info()
  local total = #state.chats
  if total == 0 then
    return nil
  end
  return {
    selected = state.selected_idx,
    total = total,
  }
end

-- Set active chat by ID (for restoring backend state on reopen)
function M.set_active_by_id(chat_id)
  for i, chat in ipairs(state.chats) do
    if chat.id == chat_id then
      state.active_idx = i
      state.selected_idx = i
      -- Fetch full chat data
      client.request({ action = 'chat_get' }, function(chat_response, err)
        if not err and chat_response then
          state.active_chat = chat_response
        end
      end)
      render()
      -- Position cursor
      if state.win and vim.api.nvim_win_is_valid(state.win) and #state.chats > 0 then
        vim.api.nvim_win_set_cursor(state.win, { state.selected_idx, 0 })
      end
      return true
    end
  end
  return false
end

-- Set project root and load pinned chats (called when project is initialized)
function M.set_project_root(project_root)
  current_project_root = project_root
  load_pinned()
end

-- Set global mode (for global-only init)
function M.set_global_mode(global)
  viewing_global = global
  load_pinned()
end

-- Check if viewing global chats
function M.is_viewing_global()
  return viewing_global
end

-- Inject mock chat list for screenshot mode (bypasses backend)
function M.set_mock_chats(chats_list, active_idx, mock_pinned_ids)
  state.chats = chats_list or {}
  state.active_idx = active_idx
  state.selected_idx = active_idx or 1
  -- Inject pinned set
  pinned_chats = {}
  if mock_pinned_ids then
    for _, id in ipairs(mock_pinned_ids) do
      pinned_chats[id] = true
    end
  end
  render()
  -- Position cursor
  if state.win and vim.api.nvim_win_is_valid(state.win) and #state.chats > 0 then
    vim.api.nvim_win_set_cursor(state.win, { state.selected_idx, 0 })
  end
end

return M
