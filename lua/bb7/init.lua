local M = {}

local ui = require('bb7.ui')
local log = require('bb7.log')

--- Ensure the backend is initialized, auto-initializing if needed.
--- Calls `callback()` on success, or returns silently on failure (with logged error).
local function ensure_initialized(callback)
  local client = require('bb7.client')
  if client.is_initialized() then
    callback()
    return
  end
  local project_root = vim.fn.getcwd()
  client.init(project_root, function(resp, init_err)
    if init_err then
      log.error('Failed to initialize: ' .. init_err)
      return
    end
    -- Pass config default_model to models module (if backend has one)
    local models = require('bb7.models')
    if resp and resp.default_model then
      models.set_config_default(resp.default_model)
    end
    -- Ensure models are loaded
    if not models.did_refresh_once() then
      models.init()
      models.refresh(function()
        callback()
      end)
    else
      callback()
    end
  end)
end

-- Default configuration
local default_config = {
  -- Optional direct pane navigation keys (in addition to <C-w>h/j/k/l)
  -- Set to false to disable, or a key string to enable
  -- Useful if you have global <C-h/j/k/l> -> <C-w><C-h/j/k/l> mappings
  nav_left = false,   -- e.g., '<C-h>'
  nav_down = false,   -- e.g., '<C-j>'
  nav_up = false,     -- e.g., '<C-k>'
  nav_right = false,  -- e.g., '<C-l>'

  -- Chat styling configuration
  -- Chat message styles are defined via BB7* highlight groups (see setup_highlights).
  -- Users can override these in their init.lua before calling setup().
  chat_style = {
    bar_char = '🮇',  -- Character used for the vertical bar

    -- Diff view colors (for gd mode)
    diff = {
      add = 'DiffAdd',       -- Added lines background
      remove = 'DiffDelete', -- Removed lines background
      hunk = 'DiffText',     -- Hunk header (@@ ... @@)
    },

    -- Spinner animation (shown during streaming)
    spinner = {
      frames = { '⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏' },  -- Braille animation
      color = 'Comment',  -- Spinner and "Generating..." text color
    },
  },

  -- Statusline indicator configuration
  status = {
    streaming = { enabled = true, symbol = '○', highlight = 'DiagnosticWarn' },
    unread    = { enabled = true, symbol = '●', highlight = 'DiagnosticInfo' },
  },
}

local config = {}

function M.setup(opts)
  config = vim.tbl_deep_extend('force', default_config, opts or {})

  -- Verify the backend binary is available before registering anything
  local client = require('bb7.client')
  local bin = client.get_bin_path()
  if vim.fn.executable(bin) ~= 1 then
    vim.notify(
      'BB-7: backend binary not found. Run :Lazy build bb7 (requires Go).',
      vim.log.levels.ERROR
    )
    return
  end

  -- Set up vim.notify capture for debugging (only active when debug enabled)
  log.setup_notify_capture()

  -- Set up highlight groups (including chat styling)
  M.setup_highlights()

  -- Pass navigation config to ui module
  ui.set_config({
    nav_left = config.nav_left,
    nav_down = config.nav_down,
    nav_up = config.nav_up,
    nav_right = config.nav_right,
  })

  -- Pass chat_style config to preview pane
  local preview = require('bb7.panes.preview')
  if config.chat_style then
    if config.chat_style.spinner then
      preview.set_spinner_frames(config.chat_style.spinner.frames)
    end
    if config.chat_style.bar_char then
      preview.set_bar_char(config.chat_style.bar_char)
    end
  end

  -- Pass status config
  if config.status then
    require('bb7.status').set_config(config.status)
  end

  -- Create user commands
  vim.api.nvim_create_user_command('BB7', function()
    local split = require('bb7.split')
    if split.is_open() then
      split.close()
    end
    ui.toggle()
  end, { desc = 'Toggle BB7' })

  vim.api.nvim_create_user_command('BB7Split', function()
    local split = require('bb7.split')
    if ui.is_open() then
      ui.close()
      split.open()
    else
      split.toggle()
    end
  end, { desc = 'Toggle BB7 split input' })

  -- BB7Init - Initialize bb7 in current directory
  vim.api.nvim_create_user_command('BB7Init', function()
    local client = require('bb7.client')
    local project_root = vim.fn.getcwd()

    -- Start backend if not running
    if not client.is_running() then
      if not client.start() then
        log.error('Failed to start backend')
        return
      end
    end

    -- Send bb7_init action
    client.request({ action = 'bb7_init', project_root = project_root }, function(_, err)
      if err then
        if err:match('[Aa]lready initialized') then
          log.info('Already initialized')
        else
          log.error(err)
        end
        return
      end
      log.info('Initialized in ' .. project_root)
    end)
  end, { desc = 'Initialize BB7 in current directory' })

  -- BB7NewChat - Create a new chat
  vim.api.nvim_create_user_command('BB7NewChat', function()
    ensure_initialized(function()
      require('bb7.panes.chats').new_chat()
    end)
  end, { desc = 'Create a new BB-7 chat' })

  -- Parse path:start:end syntax, returns path, start_line, end_line (or nil for full file)
  local function parse_section_arg(arg)
    -- Match path:start:end where start and end are numbers
    local path, start_str, end_str = arg:match('^(.+):(%d+):(%d+)$')
    if path and start_str and end_str then
      local start_line = tonumber(start_str)
      local end_line = tonumber(end_str)
      if start_line and end_line then
        return path, start_line, end_line
      end
    end
    return arg, nil, nil
  end

  local function add_context_file(opts, read_only)
    local client = require('bb7.client')

    -- Auto-initialize backend if needed
    if not client.is_initialized() then
      ensure_initialized(function()
        add_context_file(opts, read_only)
      end)
      return
    end

    -- Handle visual selection (range == 2 means visual selection)
    if opts.range == 2 then
      -- Check for line-wise selection (V mode)
      local mode = vim.fn.visualmode()
      if mode == 'v' then
        log.error('Use line-wise visual selection (V), not character-wise (v)')
        return
      end

      local path = vim.fn.expand('%:p')
      if path == '' then
        log.warn('No file to add')
        return
      end

      -- Make path relative to project root if possible (skip in global-only mode)
      local project_root = client.get_project_root()
      if project_root and path:sub(1, #project_root) == project_root then
        path = path:sub(#project_root + 2)
      end

      local start_line = opts.line1
      local end_line = opts.line2

      -- Read selected lines
      local lines = vim.api.nvim_buf_get_lines(0, start_line - 1, end_line, false)
      local content = table.concat(lines, '\n')

      -- Check for active chat first
      client.request({ action = 'chat_get' }, function(_, get_err)
        if get_err and get_err:match('[Nn]o active chat') then
          log.info('No active chat - select a chat first')
          return
        end

        -- Add section to context
        client.request({
          action = 'context_add_section',
          path = path,
          start_line = start_line,
          end_line = end_line,
          content = content,
        }, function(_, err)
          if err then
            if err:match('already exists') then
              return
            end
            log.error(err)
            return
          end
          log.info('Added ' .. path .. ':' .. start_line .. '-' .. end_line)
        end)
      end)
      return
    end

    -- Get file path (from argument or current buffer)
    local arg = opts.args ~= '' and opts.args or nil
    if not arg then
      -- No explicit argument — check for oil buffer
      if vim.bo.filetype == 'oil' then
        local ok, oil = pcall(require, 'oil')
        if not ok then
          log.warn('oil.nvim not available')
          return
        end
        local entry = oil.get_cursor_entry()
        if not entry then
          log.warn('No file selected')
          return
        end
        local dir = oil.get_current_dir(0)
        if not dir then
          log.warn('Cannot resolve path (remote oil buffers not supported)')
          return
        end
        local utils = require('bb7.utils')

        if entry.type == 'directory' then
          -- Directory add: scan, estimate tokens, confirm
          local dir_path = dir .. entry.name .. '/'
          local scanned = utils.scan_directory(dir_path)
          if #scanned == 0 then
            log.info('No text files found in directory')
            return
          end

          -- Build texts list for token estimation
          local texts = {}
          for _, file in ipairs(scanned) do
            table.insert(texts, file.content)
          end

          -- Request token estimates in parallel with current context estimate
          local results = { text_estimate = nil, context_estimate = nil }
          local pending = 2

          local function check_done()
            pending = pending - 1
            if pending > 0 then return end

            -- Both responses received
            local new_tokens = 0
            if results.text_estimate and results.text_estimate.tokens then
              for _, t in ipairs(results.text_estimate.tokens) do
                new_tokens = new_tokens + t
              end
            end

            local models = require('bb7.models')
            local model_info = models.get_model_info()
            local context_length = model_info and model_info.context_length or 200000
            local current_estimate = results.context_estimate and results.context_estimate.total or 0
            local new_total = current_estimate + new_tokens
            local pct = new_total / context_length * 100

            local function format_pct(val)
              if val > 0 and val < 1 then
                return string.format('%.1f', val)
              end
              return tostring(math.floor(val))
            end

            -- Check threshold (80% of context)
            if new_total > 0.8 * context_length then
              vim.schedule(function()
                utils.info({
                  'Directory too large for context',
                  '  ' .. entry.name .. '/',
                  #scanned .. ' files (~' .. utils.format_tokens(new_tokens) .. ' tokens)',
                  'Context: ~' .. utils.format_tokens(current_estimate) .. ' → ~' .. utils.format_tokens(new_total) .. ' tokens (' .. format_pct(pct) .. '% of ' .. utils.format_tokens(context_length) .. ')',
                })
              end)
              return
            end

            -- Build confirmation
            local label = read_only
              and 'Add ' .. #scanned .. ' files (~' .. utils.format_tokens(new_tokens) .. ' tokens) as read-only?'
              or  'Add ' .. #scanned .. ' files (~' .. utils.format_tokens(new_tokens) .. ' tokens) to context?'
            local context_line = 'Context: ~' .. utils.format_tokens(current_estimate) .. ' → ~' .. utils.format_tokens(new_total) .. ' tokens (' .. format_pct(pct) .. '% of ' .. utils.format_tokens(context_length) .. ')'

            vim.schedule(function()
              utils.confirm({ label, '  ' .. entry.name .. '/', context_line }, function()
                -- Add files sequentially
                local added = 0
                for _, file in ipairs(scanned) do
                  add_context_file({ args = file.path, range = 0 }, read_only)
                  added = added + 1
                end
                log.info('Added ' .. added .. ' files')
              end)
            end)
          end

          client.request({ action = 'estimate_text_tokens', texts = texts }, function(resp, err)
            if err then
              log.error('Token estimation failed: ' .. err)
              return
            end
            results.text_estimate = resp
            check_done()
          end)

          client.request({ action = 'estimate_tokens' }, function(resp, err)
            if err then
              -- Non-fatal: proceed without current estimate
              results.context_estimate = { total = 0 }
            else
              results.context_estimate = resp
            end
            check_done()
          end)
          return
        end

        -- Single file add from oil
        local oil_path = dir .. entry.name
        local label = read_only and 'Add (read-only) to BB-7 context:' or 'Add to BB-7 context:'
        utils.confirm({ label, '  ' .. entry.name }, function()
          add_context_file({ args = oil_path, range = 0 }, read_only)
        end)
        return
      end
      arg = vim.fn.expand('%:p')
    end
    if arg == '' then
      log.warn('No file to add')
      return
    end

    -- Reject special buffer schemes (e.g. fugitive://, term://, etc.)
    if arg:match('^%w+://') then
      log.warn('Cannot add special buffer: ' .. arg:match('^(%w+)://'))
      return
    end

    -- Parse path:start:end syntax
    local path, start_line, end_line = parse_section_arg(arg)

    -- Resolve and normalize path
    local project_root = client.get_project_root()
    local full_path = path
    if not vim.fn.filereadable(path) then
      full_path = (project_root or vim.fn.getcwd()) .. '/' .. path
    end
    -- Make path relative to project root if possible (skip in global-only mode)
    if project_root and path:sub(1, #project_root) == project_root then
      path = path:sub(#project_root + 2)  -- +2 to skip the trailing slash
    end

    -- Check for binary file
    local utils = require('bb7.utils')
    if utils.is_binary_file(full_path) then
      log.info('BB-7 does not support binary files')
      return
    end
    -- Verify file is readable
    local f = io.open(full_path, 'rb')
    if not f then
      log.error('Cannot read file: ' .. path)
      return
    end
    f:close()

    -- Read file content
    local all_lines = vim.fn.readfile(full_path)
    if not all_lines then
      log.error('Cannot read file: ' .. path)
      return
    end

    -- If section specified, validate and extract lines
    if start_line and end_line then
      if start_line <= 0 or end_line <= 0 then
        log.error('Line numbers must be positive (1-indexed)')
        return
      end
      if start_line > end_line then
        log.error('Start line cannot be greater than end line')
        return
      end
      if end_line > #all_lines then
        log.error('End line (' .. end_line .. ') exceeds file length (' .. #all_lines .. ')')
        return
      end

      local section_lines = {}
      for i = start_line, end_line do
        table.insert(section_lines, all_lines[i])
      end
      local content = table.concat(section_lines, '\n')

      -- Check for active chat first
      client.request({ action = 'chat_get' }, function(_, get_err)
        if get_err and get_err:match('[Nn]o active chat') then
          log.info('No active chat - select a chat first')
          return
        end

        -- Add section to context
        client.request({
          action = 'context_add_section',
          path = path,
          start_line = start_line,
          end_line = end_line,
          content = content,
        }, function(_, err)
          if err then
            if err:match('already exists') then
              return
            end
            log.error(err)
            return
          end
          log.info('Added ' .. path .. ':' .. start_line .. '-' .. end_line)
        end)
      end)
      return
    end

    -- Full file mode
    local content = table.concat(all_lines, '\n')

    -- Check for active chat first (user must manually select)
    client.request({ action = 'chat_get' }, function(_, get_err)
      if get_err and get_err:match('[Nn]o active chat') then
        log.info('No active chat - select a chat first')
        return
      end

      -- Add file to context
      client.request({ action = 'context_add', path = path, content = content, readonly = read_only }, function(_, err)
        if err then
          -- Silently ignore "already exists" (no-op for idempotency)
          if err:match('already exists') then
            return
          end
          log.error(err)
          return
        end
        log.info('Added ' .. path)
      end)
    end)
  end

  -- BB7Add [path[:start:end]] - Add file or section to context (default: current buffer)
  -- Supports: :BB7Add path:10:20 (lines 10-20 of path)
  -- Supports: Visual selection in V mode, then :'<,'>BB7Add
  -- Requires an active chat - user must select one first
  vim.api.nvim_create_user_command('BB7Add', function(opts)
    add_context_file(opts, false)
  end, {
    nargs = '?',
    range = true,
    complete = 'file',
    desc = 'Add file or section to BB7 context',
  })

  -- BB7AddReadonly [path] - Add file to context as read-only (default: current buffer)
  -- Requires an active chat - user must select one first
  vim.api.nvim_create_user_command('BB7AddReadonly', function(opts)
    add_context_file(opts, true)
  end, {
    nargs = '?',
    complete = 'file',
    desc = 'Add file to BB7 context as read-only',
  })

  -- BB7Remove [path] - Remove file from context (default: current buffer)
  -- Requires an active chat - user must select one first
  vim.api.nvim_create_user_command('BB7Remove', function(opts)
    local client = require('bb7.client')

    -- Auto-initialize backend if needed
    if not client.is_initialized() then
      log.info('Not initialized - no file to remove')
      return
    end

    -- Helper to make path relative to project root
    local function make_relative(p)
      local project_root = client.get_project_root()
      if project_root and p:sub(1, #project_root) == project_root then
        return p:sub(#project_root + 2)
      end
      return p
    end

    -- Helper to remove a single file
    local function remove_file(path)
      client.request({ action = 'context_remove', path = path }, function(_, err)
        if err then
          if err:match('[Nn]ot found') then return end
          log.error(err)
          return
        end
        log.info('Removed ' .. path)
      end)
    end

    -- Get file path (from argument or current buffer)
    local arg = opts.args ~= '' and opts.args or nil
    if not arg then
      -- No explicit argument — check for oil buffer
      if vim.bo.filetype == 'oil' then
        local ok, oil = pcall(require, 'oil')
        if not ok then
          log.warn('oil.nvim not available')
          return
        end
        local entry = oil.get_cursor_entry()
        if not entry then
          log.warn('No file selected')
          return
        end
        local dir = oil.get_current_dir(0)
        if not dir then
          log.warn('Cannot resolve path (remote oil buffers not supported)')
          return
        end
        local utils = require('bb7.utils')

        if entry.type == 'directory' then
          -- Directory remove: find all context files under this directory
          local dir_path = dir .. entry.name .. '/'
          local rel_prefix = make_relative(dir_path)

          client.request({ action = 'chat_get' }, function(chat_resp, get_err)
            if get_err and get_err:match('[Nn]o active chat') then
              log.info('No active chat - select a chat first')
              return
            end

            client.request({ action = 'context_list' }, function(resp, list_err)
              if list_err then
                log.error(list_err)
                return
              end
              -- Filter files under this directory
              local matches = {}
              for _, file in ipairs(resp.files or {}) do
                local fp = file.path or file
                if fp:sub(1, #rel_prefix) == rel_prefix then
                  table.insert(matches, fp)
                end
              end

              if #matches == 0 then
                log.info('No context files in this directory')
                return
              end

              -- Build confirmation lines
              local lines = { 'Remove ' .. #matches .. ' files from BB-7 context?', '  ' .. entry.name .. '/' }
              local max_show = 8
              for i, fp in ipairs(matches) do
                if i > max_show then
                  table.insert(lines, '  +' .. (#matches - max_show) .. ' more')
                  break
                end
                table.insert(lines, '  ' .. fp)
              end

              vim.schedule(function()
                utils.confirm(lines, function()
                  for _, fp in ipairs(matches) do
                    remove_file(fp)
                  end
                  log.info('Removed ' .. #matches .. ' files')
                end)
              end)
            end)
          end)
          return
        end

        -- Single file remove from oil
        local oil_path = make_relative(dir .. entry.name)
        utils.confirm({ 'Remove from BB-7 context:', '  ' .. entry.name }, function()
          -- Check for active chat first
          client.request({ action = 'chat_get' }, function(_, get_err)
            if get_err and get_err:match('[Nn]o active chat') then
              log.info('No active chat - select a chat first')
              return
            end
            remove_file(oil_path)
          end)
        end)
        return
      end
      arg = vim.fn.expand('%:p')
    end
    if arg == '' then
      log.warn('No file to remove')
      return
    end

    local path = make_relative(arg)

    -- Check for active chat first (user must manually select)
    client.request({ action = 'chat_get' }, function(_, get_err)
      if get_err and get_err:match('[Nn]o active chat') then
        log.info('No active chat - select a chat first')
        return
      end

      -- Send to backend
      remove_file(path)
    end)
  end, {
    nargs = '?',
    complete = 'file',
    desc = 'Remove file from BB7 context',
  })

  -- BB7Version - Show version
  vim.api.nvim_create_user_command('BB7Version', function()
    local client = require('bb7.client')

    -- Start backend if not running
    if not client.is_running() then
      if not client.start() then
        log.error('Failed to start backend')
        return
      end
    end

    client.get_version(function(ver, err)
      if err then
        log.error('Failed to get version: ' .. err)
      else
        log.info('BB-7 ' .. ver)
      end
    end)
  end, { desc = 'Show BB7 version' })

  -- BB7Model - Open model picker
  vim.api.nvim_create_user_command('BB7Model', function()
    if ui.is_open() then
      ui.open_model_picker()
    else
      log.warn('Open BB7 first to select a model')
    end
  end, { desc = 'Open BB7 model picker' })

  -- BB7RefreshModels - Refresh available models list
  vim.api.nvim_create_user_command('BB7RefreshModels', function()
    if ui.is_open() then
      ui.refresh_models()
    else
      log.warn('Open BB7 first to refresh models')
    end
  end, { desc = 'Refresh BB7 model list from OpenRouter' })

  -- Preview pane mode commands
  local preview = require('bb7.panes.preview')

  vim.api.nvim_create_user_command('BB7Chat', function()
    if ui.is_open() then
      preview.switch_to_chat()
    end
  end, { desc = 'Switch preview pane to chat mode' })

  vim.api.nvim_create_user_command('BB7File', function()
    if ui.is_open() then
      preview.switch_to_file()
    end
  end, { desc = 'Switch preview pane to file mode' })

  vim.api.nvim_create_user_command('BB7Diff', function()
    if ui.is_open() then
      preview.switch_to_diff()
    end
  end, { desc = 'Switch preview pane to diff mode' })

  -- BB7DiffLocal - Open vim's native side-by-side diff for partial applies
  vim.api.nvim_create_user_command('BB7DiffLocal', function(opts)
    local client = require('bb7.client')
    local context_pane = require('bb7.panes.context')

    -- Get file path from argument, current preview file, or context pane selection
    local path = opts.args
    if path == '' then
      local current_file = preview.get_current_file()
      if current_file then
        path = current_file.path
      else
        local selected = context_pane.get_selected_file and context_pane.get_selected_file()
        if selected then
          path = selected.path
        end
      end
    end

    if not path or path == '' then
      log.warn('No file selected for diff')
      return
    end

    -- Request the actual filesystem paths from the backend
    client.request({ action = 'get_diff_paths', path = path }, function(response, err)
      if err then
        log.error(err)
        return
      end

      local local_path = response.local_path
      local output_path = response.output_path

      -- Close BB7 UI first
      ui.close()

      -- Schedule the diff opening to ensure UI is fully closed
      vim.schedule(function()
        -- Open local file
        vim.cmd('edit ' .. vim.fn.fnameescape(local_path))
        local local_win = vim.api.nvim_get_current_win()
        local local_buf = vim.api.nvim_get_current_buf()

        -- Open output file in vertical split
        vim.cmd('vertical diffsplit ' .. vim.fn.fnameescape(output_path))
        local output_win = vim.api.nvim_get_current_win()
        local output_buf = vim.api.nvim_get_current_buf()

        -- Make output buffer readonly
        vim.bo[output_buf].readonly = true
        vim.bo[output_buf].modifiable = false
        vim.bo[output_buf].bufhidden = 'wipe'

        -- Position cursor in the local file (left side)
        vim.cmd('wincmd h')

        -- Set up post-close hook
        local hook_fired = false
        local augroup = vim.api.nvim_create_augroup('BB7DiffLocal', { clear = true })

        local function on_diff_close()
          if hook_fired then return end
          hook_fired = true
          vim.api.nvim_del_augroup_by_id(augroup)

          -- Turn off diff mode on local window if it still exists
          if vim.api.nvim_win_is_valid(local_win) then
            vim.wo[local_win].diff = false
            vim.wo[local_win].scrollbind = false
            vim.wo[local_win].cursorbind = false
            vim.wo[local_win].foldmethod = 'manual'
          end

          -- Auto-save local buffer if modified
          if vim.api.nvim_buf_is_valid(local_buf) and vim.bo[local_buf].modified then
            vim.api.nvim_buf_call(local_buf, function()
              vim.cmd('silent write')
            end)
          end

          -- Tell backend to compare the three files
          client.request({ action = 'diff_local_done', path = path }, function(resp, cb_err)
            if cb_err then
              log.error(cb_err)
              return
            end

            local outcome = resp and resp.outcome
            if outcome == 'full' then
              table.insert(context_pane.get_applied_files(), path)
              log.info('Applied all changes to ' .. path)
            elseif outcome == 'partial' then
              log.info('Partially applied ' .. path .. ' (reopen diff for remaining)')
            end
            -- "none" → silent
          end)
        end

        -- When output window closes: trigger post-close hook
        vim.api.nvim_create_autocmd('WinClosed', {
          group = augroup,
          pattern = tostring(output_win),
          once = true,
          callback = function()
            vim.schedule(on_diff_close)
          end,
        })

        -- When local window closes: close output window too (cascades)
        vim.api.nvim_create_autocmd('WinClosed', {
          group = augroup,
          pattern = tostring(local_win),
          once = true,
          callback = function()
            vim.schedule(function()
              if vim.api.nvim_buf_is_valid(output_buf) then
                -- Wipe the buffer instead of closing the window to avoid E444
                -- (bufhidden=wipe means the window will handle itself)
                vim.cmd('bwipeout! ' .. output_buf)
              end
              on_diff_close()
            end)
          end,
        })

        log.info('Use ]c/[c to navigate hunks, do/dp to apply changes')
      end)
    end)
  end, {
    nargs = '?',
    desc = 'Open vim native diff for partial apply (local vs LLM output)',
  })

  -- BB7Search - Search chats using Telescope (only if Telescope is available)
  local has_telescope = pcall(require, 'telescope')
  if has_telescope then
    vim.api.nvim_create_user_command('BB7Search', function()
      require('bb7.telescope').search_chats()
    end, { desc = 'Search BB-7 chats (requires Telescope)' })
  end

  -- BB7EditInstructions [level] - Edit instructions file
  -- Levels: Project, Global, or (secret) System
  vim.api.nvim_create_user_command('BB7EditInstructions', function(opts)
    local client = require('bb7.client')
    local level = opts.args:lower()

    -- If no argument, prompt user via command line
    if level == '' then
      local choice = vim.fn.input('Edit instructions (Project/Global): ')
      if choice == '' then
        return
      end
      level = choice:lower()
    end

    -- Validate level (project, global, or secret system)
    if level ~= 'project' and level ~= 'global' and level ~= 'system' then
      log.error('Invalid level: use Project or Global')
      return
    end

    -- Start backend if not running
    if not client.is_running() then
      if not client.start() then
        log.error('Failed to start backend')
        return
      end
    end

    -- Close UI if open (editing opens in the current pane, which breaks the UI)
    local ui = require('bb7.ui')
    if ui.is_open() then
      ui.close()
    end

    -- Request the path from backend (creates file if needed)
    client.request({ action = 'prepare_instructions', level = level }, function(response, err)
      if err then
        log.error(err)
        return
      end
      vim.cmd('edit ' .. vim.fn.fnameescape(response.path))
    end)
  end, {
    nargs = '?',
    complete = function(ArgLead)
      -- Only complete Project and Global (not System)
      local candidates = { 'Project', 'Global' }
      if ArgLead == '' then
        return candidates
      end
      local matches = {}
      local pattern = ArgLead:lower()
      for _, c in ipairs(candidates) do
        if c:lower():find(pattern, 1, true) then
          table.insert(matches, c)
        end
      end
      return matches
    end,
    desc = 'Edit BB7 instructions (Project or Global)',
  })

  -- Re-apply highlights and clear caches on colorscheme change
  vim.api.nvim_create_autocmd('ColorScheme', {
    group = vim.api.nvim_create_augroup('BB7Highlights', { clear = true }),
    callback = function()
      M.setup_highlights()
      require('bb7.panes.preview').clear_hl_cache()
    end,
  })

end

function M.setup_highlights()
  local normal_hl = vim.api.nvim_get_hl(0, { name = 'Normal', link = false })
  local normal_bg = normal_hl.bg
  local normal_fg = normal_hl.fg

  -- Helper: resolve a color value to a hex color
  -- Accepts: semantic group name, ANSI number (0-15), hex string, or nil
  local function resolve_color(value, is_bg)
    if value == nil then
      return nil
    end

    -- Hex color (e.g., '#ff5555')
    if type(value) == 'string' and value:match('^#%x%x%x%x%x%x$') then
      return value
    end

    -- ANSI color number (0-15)
    if type(value) == 'number' and value >= 0 and value <= 15 then
      local ansi_color = vim.g['terminal_color_' .. value]
      return ansi_color
    end

    -- Semantic highlight group name
    if type(value) == 'string' then
      local hl = vim.api.nvim_get_hl(0, { name = value, link = false })
      if is_bg then
        return hl.bg
      else
        return hl.fg
      end
    end

    return nil
  end

  -- Helper: get fg color from a highlight group
  local function get_fg(group)
    local hl = vim.api.nvim_get_hl(0, { name = group, link = false })
    return hl.fg
  end

  local function get_bg(group)
    local hl = vim.api.nvim_get_hl(0, { name = group, link = false })
    return hl.bg
  end

  -- ============================================
  -- UI highlights (borders, titles, etc.)
  -- ============================================

  vim.api.nvim_set_hl(0, 'BB7BorderActive', {
    fg = get_fg('DiagnosticOk') or get_fg('String') or get_fg('DiffAdd'),
    bg = normal_bg,
  })

  vim.api.nvim_set_hl(0, 'BB7BorderInactive', {
    fg = normal_fg,
    bg = normal_bg,
  })

  vim.api.nvim_set_hl(0, 'BB7TitleActive', {
    fg = get_fg('DiagnosticOk') or get_fg('String') or get_fg('DiffAdd'),
    bg = normal_bg,
    bold = true,
  })

  vim.api.nvim_set_hl(0, 'BB7TitleInactive', {
    fg = normal_fg,
    bg = normal_bg,
  })

  vim.api.nvim_set_hl(0, 'BB7HintKey', {
    fg = get_fg('DiagnosticInfo'),
    bg = normal_bg,
  })

  vim.api.nvim_set_hl(0, 'BB7HintText', {
    fg = get_fg('Comment'),
    bg = normal_bg,
  })

  vim.api.nvim_set_hl(0, 'BB7Normal', { link = 'Normal' })

  -- Status highlights: no background so cursorline can show through
  vim.api.nvim_set_hl(0, 'BB7StatusM', { fg = get_fg('Comment') })  -- M = comment color
  vim.api.nvim_set_hl(0, 'BB7StatusA', { fg = get_fg('DiagnosticOk') or get_fg('String') or get_fg('DiffAdd') })
  vim.api.nvim_set_hl(0, 'BB7StatusConflictA', { fg = get_fg('DiagnosticError') })
  vim.api.nvim_set_hl(0, 'BB7StatusSync', {})  -- ~ = black/normal (no fg override)
  vim.api.nvim_set_hl(0, 'BB7StatusR', {})  -- R = black/normal (no fg override)

  -- System prompt override warning (red + bold)
  vim.api.nvim_set_hl(0, 'BB7SystemOverride', { fg = get_fg('DiagnosticError'), bold = true })

  vim.api.nvim_set_hl(0, 'BB7Selection', { link = 'CursorLine' })

  -- Bold text (for selected items in context pane)
  vim.api.nvim_set_hl(0, 'BB7Bold', { bold = true })

  vim.api.nvim_set_hl(0, 'BB7MarkerActive', {
    fg = get_fg('DiagnosticOk') or get_fg('String') or get_fg('DiffAdd'),
    bold = true,
  })

  -- Diff view highlights (configurable)
  local diff_style = (config.chat_style or {}).diff or {}
  local diff_add = diff_style.add or 'DiffAdd'
  local diff_remove = diff_style.remove or 'DiffDelete'
  local diff_hunk = diff_style.hunk or 'DiffText'

  -- Resolve diff colors (can be highlight group name or color value)
  local function set_diff_hl(name, value)
    local bg = resolve_color(value, true)
    if bg then
      vim.api.nvim_set_hl(0, name, { bg = bg })
    else
      -- Assume it's a highlight group name to link to
      vim.api.nvim_set_hl(0, name, { link = value })
    end
  end

  set_diff_hl('BB7DiffAdd', diff_add)
  set_diff_hl('BB7DiffRemove', diff_remove)
  set_diff_hl('BB7DiffHunk', diff_hunk)
  vim.api.nvim_set_hl(0, 'BB7DiffChange', { link = 'DiffChange' })

  -- Spinner highlight (configurable)
  local spinner_style = (config.chat_style or {}).spinner or {}
  local spinner_color = spinner_style.color or 'Comment'
  local spinner_fg = resolve_color(spinner_color, false)
  if spinner_fg then
    vim.api.nvim_set_hl(0, 'BB7Spinner', { fg = spinner_fg })
  else
    vim.api.nvim_set_hl(0, 'BB7Spinner', { link = spinner_color })
  end

  -- ============================================
  -- Chat styling
  -- ============================================
  -- Users can override any BB7* highlight group in their init.lua.
  -- Each chunk type has:
  --   Bar  - left gutter (fg = bar color, bg = normal to not inherit line bg)
  --   Text - text and line background (fg = text color, bg = line background)
  --
  -- Chunk types:
  --   UserMessage      - User chat messages
  --   UserAction       - User actions (add/remove/apply files)
  --   AssistantMessage - Assistant chat messages
  --   AssistantAction  - Assistant actions (file writes)
  --   Thinking         - Reasoning/thinking blocks
  --   Code             - Code blocks
  --   Error            - Error messages

  -- User messages: blue bar, subtle background (Pmenu)
  -- User messages keep explicit bg (Pmenu) for visual distinction.
  -- All other groups omit bg so CursorLine can show through.
  vim.api.nvim_set_hl(0, 'BB7UserMessageBar', { fg = get_fg('DiagnosticInfo') })
  vim.api.nvim_set_hl(0, 'BB7UserMessageText', { fg = normal_fg, bg = get_bg('Pmenu') or normal_bg })

  -- User actions: keyword style (add/remove files, etc.)
  vim.api.nvim_set_hl(0, 'BB7UserActionBar', { fg = normal_bg })  -- invisible
  vim.api.nvim_set_hl(0, 'BB7UserActionText', { fg = get_fg('Keyword') })

  -- Assistant messages: no bar, normal text
  vim.api.nvim_set_hl(0, 'BB7AssistantMessageBar', { fg = normal_bg })  -- invisible
  vim.api.nvim_set_hl(0, 'BB7AssistantMessageText', { fg = normal_fg })

  -- Assistant actions: keyword style (file writes, etc.)
  vim.api.nvim_set_hl(0, 'BB7AssistantActionBar', { fg = normal_bg })  -- invisible
  vim.api.nvim_set_hl(0, 'BB7AssistantActionText', { fg = get_fg('Keyword') })

  -- Thinking: dim/comment style
  vim.api.nvim_set_hl(0, 'BB7ThinkingBar', { fg = normal_bg })  -- invisible
  vim.api.nvim_set_hl(0, 'BB7ThinkingText', { fg = get_fg('Comment') })

  -- Code blocks: separate styling for user and assistant code
  -- Same pattern as other chunks: Bar + Text (syntax highlights overlay on top)
  -- User code: blue bar, Pmenu background (same as user messages)
  vim.api.nvim_set_hl(0, 'BB7UserCodeBar', { fg = get_fg('DiagnosticInfo') })
  vim.api.nvim_set_hl(0, 'BB7UserCodeText', { fg = get_fg('Identifier'), bg = get_bg('Pmenu') or normal_bg })
  vim.api.nvim_set_hl(0, 'BB7UserCodeLang', { fg = normal_fg, bg = get_bg('Pmenu') or normal_bg, bold = true })

  -- Assistant code: invisible bar, normal background (same as assistant messages)
  vim.api.nvim_set_hl(0, 'BB7AssistantCodeBar', { fg = normal_bg })
  vim.api.nvim_set_hl(0, 'BB7AssistantCodeText', { fg = get_fg('Identifier') })
  vim.api.nvim_set_hl(0, 'BB7AssistantCodeLang', { fg = normal_fg, bold = true })

  -- Errors: red accent
  vim.api.nvim_set_hl(0, 'BB7ErrorBar', { fg = get_fg('DiagnosticError') })
  vim.api.nvim_set_hl(0, 'BB7ErrorText', { fg = get_fg('DiagnosticError') })

  -- System messages (fork warnings, etc.): hint/info style
  vim.api.nvim_set_hl(0, 'BB7SystemMessageBar', { fg = get_fg('DiagnosticHint') })
  vim.api.nvim_set_hl(0, 'BB7SystemMessageText', { fg = get_fg('Comment') })

  -- Statusline indicators (fg only — bg inherited from statusline section)
  local status_cfg = (config.status or {})
  local streaming_hl = (status_cfg.streaming or {}).highlight or 'DiagnosticWarn'
  local unread_hl = (status_cfg.unread or {}).highlight or 'DiagnosticInfo'
  vim.api.nvim_set_hl(0, 'BB7StatusStreaming', { fg = resolve_color(streaming_hl, false) or get_fg('DiagnosticWarn') })
  vim.api.nvim_set_hl(0, 'BB7StatusUnread', { fg = resolve_color(unread_hl, false) or get_fg('DiagnosticInfo') })
end

function M.open()
  ui.open()
end

function M.close()
  ui.close()
end

function M.toggle()
  ui.toggle()
end

function M.get_config()
  return config
end

return M
