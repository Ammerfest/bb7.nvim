-- Split input view: bottom split with just the input pane
local M = {}

local client = require('bb7.client')
local log = require('bb7.log')
local models = require('bb7.models')
local panes_input = require('bb7.panes.input')
local status = require('bb7.status')

local state = {
  buf = nil,
  win = nil,
  augroup = nil,
  is_open = false,
  chat = nil, -- cached chat data for winbar
}

-- Config file path (same as ui.lua)
local CONFIG_PATH = vim.fn.expand('~/.config/bb7/config.json')

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

  vim.fn.mkdir(vim.fn.fnamemodify(CONFIG_PATH, ':h'), 'p')
  local file = io.open(CONFIG_PATH, 'w')
  if not file then
    vim.notify('BB-7: Failed to create ' .. CONFIG_PATH, vim.log.levels.ERROR)
    return false
  end
  file:write('{\n  "api_key": "YOUR_API_KEY_HERE"\n}\n')
  file:close()

  vim.cmd('edit ' .. vim.fn.fnameescape(CONFIG_PATH))
  vim.notify('BB-7: Created config file. Add your API key, then run :BB7Split again.', vim.log.levels.INFO)
  return false
end

-- Reasoning effort display symbols
local REASONING_DISPLAY = {
  none   = '▱▱▱',
  low    = '▰▱▱',
  medium = '▰▰▱',
  high   = '▰▰▰',
}

-- Build winbar string
local function build_winbar()
  local parts = {}

  -- BB-7 title
  table.insert(parts, '%#BB7TitleActive# BB-7%*')

  -- Separator + chat title
  table.insert(parts, '%#Comment# │%*')
  local title = 'No chat'
  if state.chat then
    title = state.chat.name or 'Untitled'
    if #title > 50 then
      title = title:sub(1, 48) .. '..'
    end
  end
  table.insert(parts, ' ' .. title)

  -- Separator + model name
  local model = panes_input.get_model()
  if model then
    table.insert(parts, '%#Comment# │%*')
    local display = model
    if #display > 30 then
      display = display:sub(1, 28) .. '..'
    end
    table.insert(parts, ' ' .. display)

    -- Reasoning indicator (after model, if supported)
    if models.supports_reasoning(model) then
      local level = panes_input.get_reasoning_level and panes_input.get_reasoning_level() or 'none'
      local indicator = REASONING_DISPLAY[level] or '▱▱▱'
      table.insert(parts, '%#Comment# │%*')
      table.insert(parts, ' ' .. indicator)
    end
  end

  return table.concat(parts, '')
end

local function update_winbar()
  if not state.win or not vim.api.nvim_win_is_valid(state.win) then
    return
  end
  vim.wo[state.win].winbar = build_winbar()
end

-- Setup split-specific keymaps (overrides some input pane defaults)
local function setup_split_keymaps()
  local buf = state.buf
  local opts = { buffer = buf, nowait = true, silent = true }

  -- Close split with Esc in normal mode
  vim.keymap.set('n', '<Esc>', function()
    M.close()
  end, opts)

  -- Close split with C-c in normal/insert mode
  vim.keymap.set({ 'n', 'i' }, '<C-c>', function()
    M.close()
  end, opts)

  -- Close split with q in normal mode
  vim.keymap.set('n', 'q', function()
    M.close()
  end, opts)

  -- Override M to call models.open_picker() directly (not through ui module)
  vim.keymap.set('n', 'M', function()
    models.open_picker()
  end, opts)

  -- Remove keymaps that don't apply in split context
  -- These are set by panes_input.setup_keymaps but don't make sense without preview
  local remove_keys = {
    { 'n', 'gc' },
    { 'n', 'gf' },
    { 'n', 'gd' },
    { { 'n', 'i' }, '<C-d>' },
    { { 'n', 'i' }, '<C-u>' },
  }
  for _, key in ipairs(remove_keys) do
    pcall(vim.keymap.del, key[1], key[2], { buffer = buf })
  end
end

-- Close the split window/augroup without cleaning up input pane callbacks
-- Used when sending a message (callbacks must survive for stream handlers)
local function close_window()
  -- Delete augroup first to prevent WinClosed from re-triggering
  if state.augroup then
    vim.api.nvim_del_augroup_by_id(state.augroup)
    state.augroup = nil
  end

  -- Close window
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    vim.api.nvim_win_close(state.win, true)
  end

  state.buf = nil
  state.win = nil
  state.chat = nil
end

-- Close the split (full cleanup including input pane)
function M.close()
  if not state.is_open then
    return
  end

  state.is_open = false

  -- Flush draft
  panes_input.flush_draft()

  -- Cleanup input pane
  panes_input.cleanup()

  close_window()
end

-- Open the split
function M.open()
  if state.is_open then
    return
  end

  -- Close full UI if open
  local ui = require('bb7.ui')
  if ui.is_open() then
    ui.close()
  end

  if not ensure_config() then
    return
  end

  -- Ensure backend is initialized
  local project_root = vim.fn.getcwd()
  local function do_open()
    -- Check for active chat
    client.request({ action = 'chat_active' }, function(active, err)
      if err or not active or not active.id then
        log.warn('No active chat — open BB-7 first to select a chat')
        return
      end

      -- Get full chat data
      client.request({ action = 'chat_get' }, function(chat, chat_err)
        if chat_err or not chat then
          log.warn('No active chat — open BB-7 first to select a chat')
          return
        end

        state.chat = chat

        -- Create buffer
        local buf = vim.api.nvim_create_buf(false, true)
        vim.bo[buf].buftype = 'nofile'
        state.buf = buf

        -- Open bottom split
        vim.cmd('botright split')
        local win = vim.api.nvim_get_current_win()
        vim.api.nvim_win_set_buf(win, buf)
        state.win = win

        -- Set height
        vim.api.nvim_win_set_height(win, 8)

        -- Set window options
        vim.wo[win].number = false
        vim.wo[win].relativenumber = false
        vim.wo[win].signcolumn = 'no'
        vim.wo[win].winfixheight = true

        state.is_open = true

        -- Initialize input pane (takes ownership of buf/win)
        panes_input.init(buf, win)

        -- Restore chat state
        panes_input.set_chat_active(true)
        panes_input.set_draft(chat.draft or '')

        -- Restore model from chat
        if chat.model and chat.model ~= '' then
          models.set_current(chat.model, { persist = false, notify = false })
        end
        local current_model = models.get_current()
        if current_model then
          panes_input.set_model(current_model)
        end

        -- Restore reasoning level
        panes_input.set_reasoning_level(chat.reasoning_effort or 'none')

        -- Setup split-specific keymaps (after input keymaps so we override)
        setup_split_keymaps()

        -- Register model change callback (updates input pane + winbar)
        models.set_callbacks({
          on_model_changed = function(model_id)
            panes_input.set_model(model_id)
            update_winbar()
          end,
        })

        -- Set winbar
        update_winbar()

        -- Setup callbacks
        panes_input.set_callbacks({
          on_message_sent = function(_)
            -- Set status to streaming
            status.set('streaming')
            -- Close the split window but keep input pane callbacks alive
            -- (send_message continues after this callback to call client.stream)
            state.is_open = false
            vim.schedule(function()
              close_window()
              log.info('Message sent')
            end)
          end,
          on_stream_chunk = function(_)
            -- Status stays 'streaming'
          end,
          on_stream_done = function(_, _)
            -- Transition to unread
            status.set('unread')
          end,
          on_stream_error = function(err_msg)
            if err_msg == 'Response aborted by user.' then
              status.set('idle')
              return
            end
            status.set('idle')
            log.error(err_msg)
          end,
          on_diff_error = function(_)
            status.set('idle')
            log.warn('Diff failed — reopen to retry')
          end,
          on_footer_changed = function()
            update_winbar()
          end,
        })

        -- Setup autocmds
        state.augroup = vim.api.nvim_create_augroup('BB7Split', { clear = true })

        vim.api.nvim_create_autocmd('WinClosed', {
          group = state.augroup,
          pattern = tostring(win),
          callback = function()
            vim.schedule(function()
              M.close()
            end)
          end,
        })

        -- Enter insert mode
        vim.cmd('startinsert')
      end)
    end)
  end

  if not client.is_initialized() then
    if not client.is_running() then
      if not client.start() then
        log.error('Failed to start backend')
        return
      end
    end
    client.init(project_root, function(_, init_err)
      if init_err then
        log.error('Failed to initialize: ' .. init_err)
        return
      end
      -- Initialize models if needed
      if not models.did_refresh_once() then
        models.init()
        models.refresh(function()
          do_open()
        end)
      else
        do_open()
      end
    end)
  else
    -- Ensure models are loaded
    if not models.did_refresh_once() then
      models.init()
      models.refresh(function()
        do_open()
      end)
    else
      do_open()
    end
  end
end

-- Toggle the split
function M.toggle()
  if state.is_open then
    M.close()
  else
    M.open()
  end
end

-- Check if split is open
function M.is_open()
  return state.is_open
end

return M
