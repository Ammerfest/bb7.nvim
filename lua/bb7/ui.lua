local M = {}

-- Pane modules
local client = require('bb7.client')
local log = require('bb7.log')
local models = require('bb7.models')
local panes_chats = require('bb7.panes.chats')
local panes_context = require('bb7.panes.context')
local panes_provider = require('bb7.panes.provider')
local panes_preview = require('bb7.panes.preview')
local panes_input = require('bb7.panes.input')

local shared = require('bb7.ui.shared')
local session = require('bb7.ui.session')
local layout = require('bb7.ui.layout')

local config = shared.config
local state = shared.state
local session_state = shared.session_state
local PANES = shared.PANES
local KEY_TO_PANE = shared.KEY_TO_PANE
local PANE_HINTS = shared.PANE_HINTS

-- Update context estimate in provider pane from current input estimate + model info
local function update_context_estimate()
  local estimate = panes_input.get_estimate()
  local model_info = models.get_model_info()
  if estimate and model_info then
    panes_provider.set_context_estimate(
      estimate.total,
      model_info.context_length,
      model_info.max_completion_tokens
    )
  end
end

-- Get hints for a pane (from module or static)
local function get_pane_hints(pane_id)
  if pane_id == 1 then
    return panes_chats.get_hints()
  elseif pane_id == 2 then
    return panes_context.get_hints()
  elseif pane_id == 3 then
    return panes_provider.get_hints()
  elseif pane_id == 4 then
    return panes_preview.get_hints()
  elseif pane_id == 5 then
    return panes_input.get_hints()
  end
  return PANE_HINTS[pane_id] or ''
end



-- Get view switching hints (gc/gd/gf) based on current preview mode
local function get_view_hints()
  local mode = panes_preview.get_mode()
  local file = panes_preview.get_current_file()
  local hints = {}

  -- Always show the two "other" mode options
  if mode ~= 'chat' then
    table.insert(hints, 'Chat: gc')
  end
  if mode ~= 'file' and file then
    table.insert(hints, 'File: gf')
  end
  if mode ~= 'diff' and file and file.status == 'M' then
    table.insert(hints, 'Diff: gd')
  end

  if #hints > 0 then
    return table.concat(hints, ' | ')
  end
  return nil
end

-- Update hint line content
local function update_hints()
  if not state.hint_buf or not vim.api.nvim_buf_is_valid(state.hint_buf) then
    return
  end

  local left_hint
  if state.picker_open then
    left_hint = ' Select: <CR> | Favorite: <C-f> | Navigate: <C-n>/<C-p> | Cancel: <Esc>'
  else
    local hint_text = get_pane_hints(state.active_pane)
    local view_hints = get_view_hints()
    local global_hints = 'Panes: g1-g5 | Cycle: <Tab> | Close: <C-c>'

    -- Build full hint: pane hints | view hints | global hints
    local parts = {}
    if hint_text and hint_text ~= '' then
      table.insert(parts, hint_text)
    end
    if view_hints then
      table.insert(parts, view_hints)
    end
    table.insert(parts, global_hints)

    left_hint = ' ' .. table.concat(parts, ' | ')
  end

  -- Build version string for right side
  local version_str = state.version and ('BB-7 ' .. state.version) or ''

  -- Calculate padding to right-align version
  local win_width = state.hint_win and vim.api.nvim_win_is_valid(state.hint_win)
    and vim.api.nvim_win_get_width(state.hint_win) or 80
  local padding = win_width - #left_hint - #version_str - 1
  if padding < 1 then padding = 1 end

  local full_hint = left_hint .. string.rep(' ', padding) .. version_str

  vim.bo[state.hint_buf].modifiable = true
  vim.api.nvim_buf_set_lines(state.hint_buf, 0, -1, false, { full_hint })
  vim.bo[state.hint_buf].modifiable = false

  -- Apply highlights (simple approach: whole line as hint)
  vim.api.nvim_buf_add_highlight(state.hint_buf, -1, 'BB7HintText', 0, 0, -1)
end

-- Update pane borders to reflect active state
local function update_pane_borders()
  for pane_id, pane in pairs(state.panes) do
    if pane.win and vim.api.nvim_win_is_valid(pane.win) then
      local is_active = pane_id == state.active_pane
      local border_hl = is_active and 'BB7BorderActive' or 'BB7BorderInactive'

      vim.api.nvim_set_option_value('winhighlight',
        'FloatBorder:' .. border_hl .. ',NormalFloat:Normal,CursorLine:BB7Selection',
        { win = pane.win })

      -- Update title and footer
      local footer = layout.format_footer(pane_id, is_active)
      vim.api.nvim_win_set_config(pane.win, {
        title = layout.format_title(pane_id, is_active),
        footer = footer,
        footer_pos = footer and 'right' or nil,
      })

      -- Show cursorline in the focused pane (helps locate cursor across colorschemes)
      vim.wo[pane.win].cursorline = is_active
    end
  end
  update_hints()
  layout.update_all_scrollbars()
end

-- Focus a specific pane
-- via_key: optional, the key used to navigate (e.g., 'g4')
local function focus_pane(pane_id, via_key)
  if pane_id < 1 or pane_id > 5 then return end

  local pane = state.panes[pane_id]
  if pane and pane.win and vim.api.nvim_win_is_valid(pane.win) then
    local prev_pane = state.active_pane
    state.active_pane = pane_id
    vim.api.nvim_set_current_win(pane.win)
    update_pane_borders()

    -- Update context pane focus state (for selection indicator)
    if pane_id == 2 then
      panes_context.set_focus(true)
    elseif prev_pane == 2 then
      panes_context.set_focus(false)
    end

    -- Handle preview mode based on focused pane
    if pane_id == 2 then
      -- Files pane: show selected file in preview
      local file = panes_context.get_selected_file()
      if file then
        panes_preview.show_context_file(file)
      end
    elseif prev_pane == 2 and (pane_id == 1 or pane_id == 5) then
      -- Leaving context pane to chats/input: show chat in preview
      panes_preview.show_chat()
    end

    -- Special case: g5 to Input pane with empty buffer -> auto insert mode
    if pane_id == 5 and via_key == 'g5' and panes_input.is_empty() then
      vim.schedule(function()
        vim.cmd('startinsert')
      end)
    end
  end
end

-- Cycle to next/previous pane
local function cycle_pane(delta)
  local next_pane = state.active_pane + delta
  if next_pane < 1 then
    next_pane = 5
  elseif next_pane > 5 then
    next_pane = 1
  end
  focus_pane(next_pane)
end

-- Pane navigation map: for each pane, which pane is in each direction
-- Layout:
--   1=Chats(top-left), 2=Files(mid-left), 3=Info(bottom-left)
--   4=Preview(top-right), 5=Input(bottom-right)
-- nil means no neighbor in that direction (no-op)
local PANE_NEIGHBORS = {
  [1] = { h = nil, j = 2,   k = nil, l = 4   },  -- Chats
  [2] = { h = nil, j = 3,   k = 1,   l = 4   },  -- Files
  [3] = { h = nil, j = nil, k = 2,   l = 5   },  -- Info
  [4] = { h = 1,   j = 5,   k = nil, l = nil },  -- Preview
  [5] = { h = 3,   j = nil, k = 4,   l = nil },  -- Input
}

-- Setup common keymaps for all panes
-- This provides a consistent base that all panes share
local function setup_common_keymaps(pane_id, buf)
  local opts = { buffer = buf, nowait = true, silent = true }

  -- Pane switching with g-prefixed keys (g1-g5)
  -- Using g-prefix avoids conflicts with vim number counts
  for key, target_pane in pairs(KEY_TO_PANE) do
    vim.keymap.set('n', key, function()
      focus_pane(target_pane, key)
    end, opts)
  end

  -- Tab cycling
  vim.keymap.set('n', '<Tab>', function()
    cycle_pane(1)
  end, opts)

  vim.keymap.set('n', '<S-Tab>', function()
    cycle_pane(-1)
  end, opts)

  -- Close with Esc (vim-native, no need to document)
  vim.keymap.set('n', '<Esc>', function()
    M.close()
  end, opts)

  -- Close with C-c (works from any pane, any mode)
  vim.keymap.set({ 'n', 'i' }, '<C-c>', function()
    M.close()
  end, opts)

  -- Cancel stream with C-x (works from any pane)
  vim.keymap.set('n', '<C-x>', function()
    panes_input.cancel_send()
  end, opts)

  -- C-w direction navigation: move to neighbor pane or no-op
  local neighbors = PANE_NEIGHBORS[pane_id]
  for _, dir in ipairs({ 'h', 'j', 'k', 'l' }) do
    local target = neighbors[dir]
    local action = target and function() focus_pane(target) end or '<Nop>'
    vim.keymap.set('n', '<C-w>' .. dir, action, opts)
    vim.keymap.set('n', '<C-w><C-' .. dir .. '>', action, opts)
  end

  -- Optional direct navigation keys (configured by user)
  local nav_keys = {
    { key = config.nav_left,  target = neighbors.h },
    { key = config.nav_down,  target = neighbors.j },
    { key = config.nav_up,    target = neighbors.k },
    { key = config.nav_right, target = neighbors.l },
  }
  for _, nav in ipairs(nav_keys) do
    if nav.key then
      local action = nav.target and function() focus_pane(nav.target) end or '<Nop>'
      vim.keymap.set('n', nav.key, action, opts)
    end
  end

  -- C-w w / C-w C-w: cycle windows (same as Tab)
  vim.keymap.set('n', '<C-w>w', function() cycle_pane(1) end, opts)
  vim.keymap.set('n', '<C-w><C-w>', function() cycle_pane(1) end, opts)
  vim.keymap.set('n', '<C-w>W', function() cycle_pane(-1) end, opts)

  -- Global view switching (gc/gd/gf) - works from any pane
  vim.keymap.set('n', 'gc', function() panes_preview.switch_to_chat() end, opts)
  vim.keymap.set('n', 'gf', function() panes_preview.switch_to_file() end, opts)
  vim.keymap.set('n', 'gd', function() panes_preview.switch_to_diff() end, opts)

  -- Disable other C-w commands that don't make sense
  local cw_disabled = {
    '<C-w>H', '<C-w>J', '<C-w>K', '<C-w>L',  -- move window
    '<C-w>s', '<C-w>v', '<C-w>c', '<C-w>o', '<C-w>q',  -- split/close
    '<C-w>n', '<C-w>p', '<C-w>r', '<C-w>x', '<C-w>=',  -- misc
    '<C-w>+', '<C-w>-', '<C-w><', '<C-w>>', -- resize
    '<C-w>t', '<C-w>b', '<C-w>T',  -- tab/top/bottom
  }
  for _, key in ipairs(cw_disabled) do
    vim.keymap.set('n', key, '<Nop>', opts)
  end

  -- Keep jumplist navigation within the pane's buffer.
  local function jump_within_buffer(cmd)
    local win = vim.api.nvim_get_current_win()
    if not vim.api.nvim_win_is_valid(win) then
      return
    end
    local bufnr = vim.api.nvim_win_get_buf(win)
    local cursor = vim.api.nvim_win_get_cursor(win)
    vim.cmd(cmd)
    local after_buf = vim.api.nvim_win_get_buf(win)
    if after_buf ~= bufnr and vim.api.nvim_buf_is_valid(bufnr) then
      vim.api.nvim_win_set_buf(win, bufnr)
      vim.api.nvim_win_set_cursor(win, cursor)
    end
  end
  vim.keymap.set('n', '<C-o>', function() jump_within_buffer('normal! \\<C-o>') end, opts)
  vim.keymap.set('n', '<C-i>', function() jump_within_buffer('normal! \\<C-i>') end, opts)

  -- All panes have full vim normal mode available
  -- Pane-specific keymaps (in chats.lua, etc.) override vim defaults where needed
end

-- Setup keymaps for all panes
local function setup_keymaps()
  -- First set common keymaps for all panes
  for pane_id, pane in pairs(state.panes) do
    setup_common_keymaps(pane_id, pane.buf)
  end

  -- Then set pane-specific keymaps (so they override common ones)
  -- Input pane has special keymaps for sending messages and model picker
  panes_input.setup_keymaps(state.panes[5].buf)
end

-- Setup autocmds for the UI
local function setup_autocmds()
  state.augroup = vim.api.nvim_create_augroup('BB7Layout', { clear = true })

  -- Handle resize
  vim.api.nvim_create_autocmd('VimResized', {
    group = state.augroup,
    callback = function()
      layout.update_layout()
    end,
  })

  -- Update scrollbars on scroll
  vim.api.nvim_create_autocmd({ 'WinScrolled', 'CursorMoved', 'CursorMovedI' }, {
    group = state.augroup,
    callback = function()
      if not state.is_open then return end
      layout.update_all_scrollbars()
    end,
  })

  -- Close all panes if any window is closed (e.g., via :q)
  for pane_id, pane in pairs(state.panes) do
    vim.api.nvim_create_autocmd('WinClosed', {
      group = state.augroup,
      pattern = tostring(pane.win),
      callback = function()
        -- Defer to avoid issues during window close
        vim.schedule(function()
          M.close()
        end)
      end,
    })
  end

  -- Detect when focus escapes to a non-BB7 window and close
  vim.api.nvim_create_autocmd('WinEnter', {
    group = state.augroup,
    callback = function()
      if not state.is_open then return end
      -- Don't close if a picker/popup is open
      if state.picker_open then return end

      local current_win = vim.api.nvim_get_current_win()

      -- Check if current window is one of our panes
      local is_bb7_window = false
      for _, pane in pairs(state.panes) do
        if pane.win == current_win then
          is_bb7_window = true
          break
        end
      end

      -- If we escaped to a non-BB7 window, close BB7
      if not is_bb7_window then
        vim.schedule(function()
          M.close()
        end)
      end
    end,
  })
end

-- Focus input pane and enter insert mode
local function focus_input_insert()
  focus_pane(5)
  panes_input.focus_insert()
end

-- Config file path
local CONFIG_PATH = vim.fn.expand('~/.config/bb7/config.json')

-- Config template — only api_key is required; all other fields have defaults
local CONFIG_TEMPLATE = [[{
  "api_key": "YOUR_API_KEY_HERE"
}
]]

-- Check config exists; prompt to create if missing. Returns true if config is ready.
local function ensure_config()
  if vim.fn.filereadable(CONFIG_PATH) == 1 then
    return true
  end

  local choice = vim.fn.confirm(
    'BB-7: Config file not found at ' .. CONFIG_PATH .. '\nCreate it now?',
    '&Yes\n&No', 1)
  if choice ~= 1 then
    return false
  end

  -- Create parent directory and write template
  vim.fn.mkdir(vim.fn.fnamemodify(CONFIG_PATH, ':h'), 'p')
  local file = io.open(CONFIG_PATH, 'w')
  if not file then
    vim.notify('BB-7: Failed to create ' .. CONFIG_PATH, vim.log.levels.ERROR)
    return false
  end
  file:write(CONFIG_TEMPLATE)
  file:close()

  -- Open the config file for editing
  vim.cmd('edit ' .. vim.fn.fnameescape(CONFIG_PATH))
  vim.notify('BB-7: Created config file. Add your API key, then run :BB7 again.', vim.log.levels.INFO)
  return false
end

-- Open the UI
function M.open()
  if state.is_open then
    return
  end

  if not ensure_config() then
    return
  end

  local current_layout = layout.calc_layout()

  -- Create all panes
  for pane_id = 1, 5 do
    state.panes[pane_id] = layout.create_pane(pane_id, current_layout, pane_id == 1)
  end

  -- Set initial state (before pane init to allow callbacks to work)
  state.is_open = true
  state.active_pane = 1

  -- Setup callbacks for chat selection
  panes_chats.set_callbacks({
    on_before_chat_switch = function()
      -- Flush draft before switching to preserve unsent text
      panes_input.flush_draft()
    end,
    on_chat_selected = function(chat)
      panes_preview.set_chat(chat)
      panes_context.set_chat(chat)
      panes_provider.set_chat(chat)
      if chat then
        panes_input.set_chat_active(true)
        -- Restore draft for this chat
        panes_input.set_draft(chat.draft)
        -- Refresh token estimate for new context
        panes_input.refresh_estimate(update_context_estimate)
      else
        panes_input.set_chat_active(false)
        panes_input.set_draft('')
      end
    end,
    on_chat_created = function(_)
      -- Focus input pane for new chat
      vim.schedule(function()
        focus_input_insert()
      end)
    end,
    on_data_changed = function()
      update_pane_borders()  -- Refresh footer with chat count
    end,
  })

  -- Setup callbacks for context file selection
  panes_context.set_callbacks({
    on_file_selected = function(file)
      -- Always track the current file in preview pane
      -- But only auto-switch to file mode when context pane is focused
      if state.active_pane == 2 then
        panes_preview.show_context_file(file)
      else
        -- Just store the file reference without switching mode
        panes_preview.set_current_file(file)
      end
      -- Refresh hints (e.g., x = Remove vs Reject depending on file status)
      update_hints()
    end,
    on_data_changed = function()
      update_pane_borders()  -- Refresh footer with new file/token counts
    end,
  })

  -- Setup callbacks for preview pane (title and mode updates)
  panes_preview.set_callbacks({
    on_title_changed = function()
      update_pane_borders()  -- Refresh all titles
    end,
    on_mode_changed = function()
      update_hints()  -- Refresh hints when mode changes
    end,
  })

  -- Setup callbacks for input pane
  panes_input.set_callbacks({
    on_message_sent = function(content)
      -- Start streaming in preview, showing user message immediately
      panes_preview.start_streaming(content)
    end,
    on_stream_chunk = function(chunk)
      panes_preview.append_stream(chunk)
    end,
    on_stream_reasoning = function(chunk)
      panes_preview.append_reasoning_stream(chunk)
    end,
    on_stream_error = function(err)
      -- User-initiated cancel: treat as done (partial response is saved by backend)
      if err == 'Response aborted by user.' then
        panes_preview.end_streaming()
        client.request({ action = 'chat_get' }, function(chat, get_err)
          if not get_err and chat then
            panes_preview.set_chat(chat)
            panes_context.set_chat(chat)
            panes_input.refresh_estimate(update_context_estimate)
          end
        end)
        return
      end
      panes_preview.show_send_error(err)
    end,
    check_send = function()
      if panes_preview.has_send_error() then
        log.warn(panes_preview.get_send_error())
        -- Re-validate instructions in case the user fixed them
        client.request({ action = 'chat_get' }, function(chat, err)
          if not err and chat then
            local current = panes_preview.get_chat()
            if current and chat.id == current.id then
              panes_preview.set_chat(chat)
              panes_context.set_chat(chat)
            end
          end
        end)
        return panes_preview.get_send_error()
      end
    end,
    on_mode_changed = function()
      update_hints()  -- Refresh hints when insert/normal mode changes
    end,
    on_footer_changed = function()
      update_pane_borders()  -- Refresh footer when reasoning level changes
    end,
    on_estimate_refreshed = update_context_estimate,
    on_stream_done = function(_, usage)
      panes_preview.end_streaming(usage)

      -- Update provider pane with usage info
      if usage then
        panes_provider.update_usage(usage, panes_input.get_model())
      end

      -- Refresh chat data to get the complete message
      client.request({ action = 'chat_get' }, function(chat, err)
        if not err and chat then
          panes_preview.set_chat(chat)
          panes_context.set_chat(chat)

          -- Refresh token estimate (context may have changed with new output files)
          panes_input.refresh_estimate(update_context_estimate)

          -- Generate title after first message exchange (exactly one user message)
          -- Note: message count may exceed 2 due to context events (file writes)
          if chat.messages then
            local user_msg_count = 0
            local first_user_msg = nil
            for _, msg in ipairs(chat.messages) do
              if msg.role == 'user' then
                user_msg_count = user_msg_count + 1
                if not first_user_msg then
                  first_user_msg = msg
                end
              end
            end
            if user_msg_count == 1 and first_user_msg and first_user_msg.content then
              client.generate_title(chat.id, first_user_msg.content, function(_, title_err)
                if title_err then
                  -- Silently ignore title generation errors
                end
              end)
            end
          end
        end
      end)
    end,
  })

  -- Setup event handlers for async events (title updates)
  client.set_event_handlers({
    on_title_updated = function(chat_id, title)
      -- Refresh chat list to show new title
      panes_chats.refresh()
      -- Update preview if this is the active chat
      local current_chat = panes_preview.get_chat()
      if current_chat and current_chat.id == chat_id then
        current_chat.name = title
        panes_preview.set_chat(current_chat)
      end
    end,
  })

  -- Initialize pane modules
  panes_chats.init(state.panes[1].buf, state.panes[1].win)
  panes_context.init(state.panes[2].buf, state.panes[2].win)
  panes_provider.init(state.panes[3].buf, state.panes[3].win)
  panes_preview.init(state.panes[4].buf, state.panes[4].win)
  panes_input.init(state.panes[5].buf, state.panes[5].win)

  -- Create hint line
  local hint = layout.create_hint_line(current_layout)
  state.hint_win = hint.win
  state.hint_buf = hint.buf

  -- Setup keymaps
  setup_keymaps()

  -- Setup autocmds (resize, window close handling)
  setup_autocmds()

  -- Set initial pane states (borders, cursorline, hints)
  update_pane_borders()

  -- Initialize backend and load data
  local project_root = vim.fn.getcwd()
  client.init(project_root, function(_, err)
    if err then
      log.error('Failed to initialize: ' .. err)
      return
    end

    -- Query backend version for display in hint line
    client.get_version(function(ver)
      if ver then
        state.version = ver
        update_hints()
      end
    end)

    -- Initialize models (cached state only; network refresh happens after chats load)
    models.init()

    -- Setup model change callback to update input pane (before refresh so default model triggers it)
    models.set_callbacks({
      on_model_changed = function(model_id)
        panes_input.set_model(model_id)
        update_pane_borders()
        update_context_estimate()
        -- Persist model to active chat
        if client.is_initialized() then
          client.send({ action = 'save_chat_settings', model = model_id })
        end
      end,
    })

    local cached_model = models.get_current()
    if cached_model then
      panes_input.set_model(cached_model)
      update_pane_borders()
    end

    -- Load chat list and restore active chat if backend has one
    local function apply_chat_to_panes(chat)
      panes_preview.set_chat(chat)
      panes_context.set_chat(chat)
      panes_provider.set_chat(chat)
      panes_input.set_chat_active(true)
      panes_input.set_draft(chat.draft)

      -- Restore model from chat (single source of truth)
      if chat.model and chat.model ~= '' then
        models.set_current(chat.model, { persist = false })
      end

      -- Restore reasoning level from chat
      panes_input.set_reasoning_level(chat.reasoning_effort or 'none')

      -- Update footer to reflect restored reasoning
      update_pane_borders()

      -- Restore preview scroll position (deferred from restore_ui_state)
      if session_state.restore_preview_scroll then
        session_state.restore_preview_scroll = false
        if session_state.preview_autoscroll == false and session_state.pane_views[4] then
          vim.schedule(function()
            session.restore_pane_view(4)
          end)
        end
      end

      -- Re-restore input pane cursor (set_draft above replaced the buffer content)
      if session_state.pane_views[5] then
        vim.schedule(function()
          session.restore_pane_view(5)
        end)
      end

      -- Fetch token estimate so the context line in Info pane is populated
      panes_input.refresh_estimate(update_context_estimate)

    end

    local function restore_active_chat_if_any()
      -- Try to restore backend's active chat (user must manually select if none)
      client.request({ action = 'chat_active' }, function(active, err)
        if err then
          return
        end
        if not active or not active.id then
          return
        end
        client.request({ action = 'chat_get' }, function(chat, chat_err)
          if not chat_err and chat and chat.id then
            -- Backend has an active chat - sync frontend state
            panes_chats.set_active_by_id(chat.id)
            apply_chat_to_panes(chat)
          end
        end)
      end)
    end

    local function restore_ui_state()
      -- Restore view state for all panes (cursor/scroll positions)
      -- Skip pane 4 (preview) — its content loads asynchronously via apply_chat_to_panes
      for pane_id = 1, 5 do
        if pane_id ~= 4 then
          session.restore_pane_view(pane_id)
        end
      end
      session_state.restore_preview_scroll = true

      -- Determine which pane to focus
      local target_pane = 1  -- Default: chats pane
      if not session_state.first_open and session_state.active_pane then
        -- Not first open - restore previous active pane
        target_pane = session_state.active_pane
        -- Validate it's a valid pane
        if target_pane < 1 or target_pane > 5 then
          target_pane = 1
        end
      end

      focus_pane(target_pane)
    end

    local function on_chats_refreshed()
      restore_active_chat_if_any()
      -- Schedule restore after async operations complete
      vim.schedule(restore_ui_state)

      -- Defer network refreshes until after chat list is visible
      vim.schedule(function()
        panes_provider.refresh_balance()
        panes_provider.refresh_customization()
        local function on_models_refreshed(success)
          if success then
            update_context_estimate()
          end
        end
        if not models.did_refresh_once() then
          models.refresh(on_models_refreshed)
        else
          models.refresh_if_stale(on_models_refreshed)
        end
      end)
    end

    -- Set project root for per-project data
    panes_chats.set_project_root(project_root)
    panes_provider.set_project_root(project_root)
    panes_chats.refresh(on_chats_refreshed)
  end)

  -- Focus first pane initially (will be updated after restore)
  focus_pane(1)
end

-- Close the UI
function M.close()
  if not state.is_open then
    return
  end

  -- Defer close if in command-line window (E11 error)
  if vim.fn.getcmdwintype() ~= '' then
    vim.schedule(function()
      M.close()
    end)
    return
  end

  -- Exit insert mode before saving state so the editor doesn't remain
  -- in insert mode after close
  if vim.fn.mode() == 'i' or vim.fn.mode() == 'I' then
    vim.cmd('stopinsert')
  end

  -- Save session state before cleanup
  session.save_session_state()

  -- Mark as closed immediately to prevent re-entry from WinClosed autocmd
  state.is_open = false

  -- Remove autocmds first to prevent WinClosed from firing during close
  if state.augroup then
    vim.api.nvim_del_augroup_by_id(state.augroup)
    state.augroup = nil
  end

  -- Cleanup pane modules
  panes_chats.cleanup()
  panes_context.cleanup()
  panes_provider.cleanup()
  panes_preview.cleanup()
  panes_input.cleanup()

  -- Stop client (but don't kill the process - it persists)
  -- client.stop() -- Uncomment if we want to stop process on close

  -- Close all pane windows
  for _, pane in pairs(state.panes) do
    if pane.win and vim.api.nvim_win_is_valid(pane.win) then
      vim.api.nvim_win_close(pane.win, true)
    end
  end

  -- Close scrollbar windows
  for _, sb in pairs(state.scrollbars) do
    if sb.win and vim.api.nvim_win_is_valid(sb.win) then
      vim.api.nvim_win_close(sb.win, true)
    end
  end
  state.scrollbars = {}

  -- Close hint window
  if state.hint_win and vim.api.nvim_win_is_valid(state.hint_win) then
    vim.api.nvim_win_close(state.hint_win, true)
  end

  -- Reset state
  state.panes = {}
  state.hint_win = nil
  state.hint_buf = nil
end

-- Toggle the UI
function M.toggle()
  if state.is_open then
    M.close()
  else
    M.open()
  end
end

-- Check if UI is open
function M.is_open()
  return state.is_open
end

-- Get current active pane
function M.get_active_pane()
  return state.active_pane
end

-- Set configuration from init.lua
function M.set_config(cfg)
  config = vim.tbl_deep_extend('force', config, cfg or {})
end

-- Set picker open state (prevents auto-close when focus leaves BB7 panes)
function M.set_picker_open(is_open)
  state.picker_open = is_open
  update_hints()
end

-- Open model picker
function M.open_model_picker()
  models.open_picker()
end

-- Refresh models list
function M.refresh_models()
  models.refresh(function(success)
    if success then
      log.info('Models refreshed (' .. models.get_count() .. ' available)')
    end
  end)
end

-- Switch to a chat by ID (used by fork and search)
-- Optional callback is called after switch completes
function M.switch_chat(chat_id, callback)
  -- Select the chat in the backend
  client.request({ action = 'chat_select', id = chat_id }, function(_, err)
    if err then
      log.error('Failed to select chat: ' .. err)
      return
    end

    -- Refresh the chat list and select the new chat
    panes_chats.refresh(function()
      panes_chats.set_active_by_id(chat_id)
      -- Fetch full chat data and update all panes
      client.request({ action = 'chat_get' }, function(chat, chat_err)
        if chat_err then
          log.error('Failed to get chat: ' .. chat_err)
          return
        end
        panes_preview.set_chat(chat)
        panes_context.set_chat(chat)
        panes_provider.set_chat(chat)
        panes_input.set_chat_active(true)
        panes_input.set_draft(chat.draft or '')
        panes_input.refresh_estimate(update_context_estimate)
        if callback then
          callback()
        end
      end)
    end)
  end)
end

-- Focus the input pane and enter insert mode
function M.focus_input()
  if not state.is_open then return end
  state.active_pane = 5
  local pane = state.panes[5]
  if pane and pane.win and vim.api.nvim_win_is_valid(pane.win) then
    vim.api.nvim_set_current_win(pane.win)
    update_pane_borders()
    panes_input.focus_insert()
  end
end

-- Set screenshot mode: populate all panes with mock data for GitHub release screenshot
-- Usage: :BB7Toggle, then :lua require('bb7.ui').set_screenshot_mode()
function M.set_screenshot_mode()
  if not state.is_open then
    return
  end

  local mock = require('bb7.panes.preview.mock')

  -- Version for hint line
  state.version = 'v0.3.0'

  -- Pane 1: Chats (5 chats, 3rd active, 1st pinned)
  local chats = {
    { id = 'sc-001', name = 'Setting up my first Godot project' },
    { id = 'sc-002', name = 'Player movement with KinematicBody2D' },
    { id = 'sc-003', name = 'Understanding signals vs direct refs' },
    { id = 'sc-004', name = 'Tilemap collision layers explained' },
    { id = 'sc-005', name = 'Exporting to web with Godot 4' },
  }
  panes_chats.set_mock_chats(chats, 3, { 'sc-001' })

  -- Pane 2: Files (3 files)
  local files = {
    { path = 'player_controller.gd', in_context = true, has_output = false, readonly = true, external = false, status = 'R', tokens = 340, original_tokens = 0, output_tokens = 0 },
    { path = 'main_scene.tscn', in_context = true, has_output = false, readonly = false, external = false, status = '', tokens = 180, original_tokens = 0, output_tokens = 0 },
    { path = 'signal_bus.gd', in_context = true, has_output = true, readonly = false, external = false, status = 'M', tokens = 410, original_tokens = 150, output_tokens = 260 },
  }
  panes_context.set_mock_files(files)

  -- Pane 3: Info
  panes_provider.set_mock_data({
    balance = { total_credits = 25, total_usage = 7.45 },
    today_cost = 0.108,
    session_cost = 0.214,
    customization = {
      system_override = false,
      global_instructions = true,
      project_instructions = true,
    },
    context_estimate = {
      estimate = 4200,
      context_length = 200000,
      max_completion_tokens = 8192,
    },
  })

  -- Pane 4: Preview (Godot signals conversation)
  local chat = mock.get_screenshot_chat()
  panes_preview.set_chat(chat)

  -- Pane 5: Input
  panes_input.set_chat_active(true)
  panes_input.set_draft('How would I connect this to the UI health bar?')
  panes_input.set_model('claude-sonnet-4-5-20250929')
  panes_input.set_reasoning_level('medium')

  -- Update all borders and hints
  update_pane_borders()
end

return M
