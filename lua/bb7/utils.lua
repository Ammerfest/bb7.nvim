-- Shared utility functions for BB7
local M = {}

-- Format token count for display
-- Returns "0" for 0/nil, "999" for under 1000, "1.5k" for 1000+
function M.format_tokens(count)
  if not count or count == 0 then return '0' end
  if count >= 1000 then
    return string.format('%.1fk', count / 1000)
  end
  return tostring(count)
end

-- Normalize line endings to \n
function M.normalize_line_endings(content)
  if not content then return '' end
  return content:gsub('\r\n', '\n'):gsub('\r', '\n')
end

-- Compare two content strings with line ending normalization
-- Returns true if they are the same (ignoring line ending differences and trailing newlines)
function M.contents_equal(a, b)
  local normalized_a = M.normalize_line_endings(a)
  local normalized_b = M.normalize_line_endings(b)
  -- Strip trailing newlines (context uses readfile+concat which omits trailing newline,
  -- but io.open read('*a') preserves it)
  normalized_a = normalized_a:gsub('\n+$', '')
  normalized_b = normalized_b:gsub('\n+$', '')
  return normalized_a == normalized_b
end

-- Format ISO 8601 timestamp for display
-- Input: "2024-01-15T14:30:00Z" or similar
-- Output: "2024-01-15 14:30"
function M.format_time(timestamp)
  if not timestamp then return '' end
  -- Handle ISO 8601 format from Go
  local pattern = '(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)'
  local year, month, day, hour, min, _ = timestamp:match(pattern)
  if year then
    return string.format('%s-%s-%s %s:%s', year, month, day, hour, min)
  end
  return timestamp
end

-- Show a centered floating confirmation dialog.
-- lines: table of strings to display
-- on_confirm: function called when user confirms
-- on_cancel: optional function called when user cancels
function M.confirm(lines, on_confirm, on_cancel)
  -- Calculate dimensions
  local max_width = 0
  for _, line in ipairs(lines) do
    if #line > max_width then max_width = #line end
  end
  local actions = '  [Y]es  [N]o  '
  if #actions > max_width then max_width = #actions end
  local width = max_width + 4 -- padding
  local height = #lines + 2   -- content + blank + actions

  local buf = vim.api.nvim_create_buf(false, true)
  vim.bo[buf].bufhidden = 'wipe'

  -- Build buffer content: lines + blank + actions
  local buf_lines = {}
  for _, line in ipairs(lines) do
    table.insert(buf_lines, '  ' .. line)
  end
  table.insert(buf_lines, '')
  table.insert(buf_lines, actions)

  vim.api.nvim_buf_set_lines(buf, 0, -1, false, buf_lines)
  vim.bo[buf].modifiable = false

  -- Highlight action hints
  local action_line = #buf_lines - 1
  -- [Y] in [Y]es
  local y_start = actions:find('%[Y%]')
  if y_start then
    vim.api.nvim_buf_add_highlight(buf, -1, 'Special', action_line, y_start - 1, y_start + 2)
  end
  -- [N] in [N]o
  local n_start = actions:find('%[N%]')
  if n_start then
    vim.api.nvim_buf_add_highlight(buf, -1, 'Special', action_line, n_start - 1, n_start + 2)
  end

  -- Hide cursor in the dialog
  vim.api.nvim_set_hl(0, 'BB7ConfirmCursor', { nocombine = true, blend = 100 })
  local original_guicursor = vim.go.guicursor
  vim.go.guicursor = 'a:BB7ConfirmCursor/BB7ConfirmCursor'

  local win = vim.api.nvim_open_win(buf, true, {
    relative = 'editor',
    width = width,
    height = height,
    row = math.floor((vim.o.lines - height) / 2),
    col = math.floor((vim.o.columns - width) / 2),
    style = 'minimal',
    border = 'rounded',
    zindex = 152,
  })

  local closed = false
  local function close(confirmed)
    if closed then return end
    closed = true
    -- Restore cursor (workaround for neovim/neovim#21018)
    vim.go.guicursor = 'a:'
    vim.cmd.redrawstatus()
    vim.go.guicursor = original_guicursor
    if vim.api.nvim_win_is_valid(win) then
      vim.api.nvim_win_close(win, true)
    end
    if confirmed then
      on_confirm()
    elseif on_cancel then
      on_cancel()
    end
  end

  local key_opts = { buffer = buf, nowait = true, silent = true }
  for _, key in ipairs({ 'y', 'Y' }) do
    vim.keymap.set('n', key, function() close(true) end, key_opts)
  end
  for _, key in ipairs({ 'n', 'N', 'q', '<Esc>', '<C-c>' }) do
    vim.keymap.set('n', key, function() close(false) end, key_opts)
  end

  vim.api.nvim_create_autocmd({ 'BufLeave', 'WinLeave' }, {
    buffer = buf,
    once = true,
    callback = function() close(false) end,
  })
end

-- Show a centered floating info dialog (dismissed by any key).
-- lines: table of strings to display
-- on_dismiss: optional callback when dismissed
function M.info(lines, on_dismiss)
  -- Calculate dimensions
  local max_width = 0
  for _, line in ipairs(lines) do
    if #line > max_width then max_width = #line end
  end
  local actions = '  [O]k  '
  if #actions > max_width then max_width = #actions end
  local width = max_width + 4
  local height = #lines + 2

  local buf = vim.api.nvim_create_buf(false, true)
  vim.bo[buf].bufhidden = 'wipe'

  local buf_lines = {}
  for _, line in ipairs(lines) do
    table.insert(buf_lines, '  ' .. line)
  end
  table.insert(buf_lines, '')
  table.insert(buf_lines, actions)

  vim.api.nvim_buf_set_lines(buf, 0, -1, false, buf_lines)
  vim.bo[buf].modifiable = false

  -- Highlight [O] in [O]k
  local action_line = #buf_lines - 1
  local o_start = actions:find('%[O%]')
  if o_start then
    vim.api.nvim_buf_add_highlight(buf, -1, 'Special', action_line, o_start - 1, o_start + 2)
  end

  -- Hide cursor in the dialog
  vim.api.nvim_set_hl(0, 'BB7ConfirmCursor', { nocombine = true, blend = 100 })
  local original_guicursor = vim.go.guicursor
  vim.go.guicursor = 'a:BB7ConfirmCursor/BB7ConfirmCursor'

  local win = vim.api.nvim_open_win(buf, true, {
    relative = 'editor',
    width = width,
    height = height,
    row = math.floor((vim.o.lines - height) / 2),
    col = math.floor((vim.o.columns - width) / 2),
    style = 'minimal',
    border = 'rounded',
    zindex = 152,
  })

  local closed = false
  local function close()
    if closed then return end
    closed = true
    vim.go.guicursor = 'a:'
    vim.cmd.redrawstatus()
    vim.go.guicursor = original_guicursor
    if vim.api.nvim_win_is_valid(win) then
      vim.api.nvim_win_close(win, true)
    end
    if on_dismiss then on_dismiss() end
  end

  -- Any key dismisses
  local key_opts = { buffer = buf, nowait = true, silent = true }
  for _, key in ipairs({ 'o', 'O', 'q', '<CR>', '<Esc>', '<C-c>', '<Space>' }) do
    vim.keymap.set('n', key, close, key_opts)
  end

  vim.api.nvim_create_autocmd({ 'BufLeave', 'WinLeave' }, {
    buffer = buf,
    once = true,
    callback = close,
  })
end

-- Check if a file is binary (null byte in first 8000 bytes, like git).
-- Returns true if binary, false if text or unreadable.
function M.is_binary_file(path)
  local f = io.open(path, 'rb')
  if not f then return false end
  local head = f:read(8000)
  f:close()
  return head and head:find('\0') ~= nil or false
end

-- Scan a directory recursively, returning list of {path=abs_path, content=string} pairs.
-- Filters out directories, hidden paths (.-prefixed components), and binary files.
function M.scan_directory(dir_path)
  -- Ensure trailing slash
  if dir_path:sub(-1) ~= '/' then dir_path = dir_path .. '/' end

  local files = vim.fn.glob(dir_path .. '**', false, true)
  local results = {}
  for _, file_path in ipairs(files) do
    -- Skip directories
    if vim.fn.isdirectory(file_path) == 1 then goto continue end

    -- Skip hidden paths (any component starting with .)
    local rel = file_path:sub(#dir_path + 1)
    if rel:match('/%.') or rel:match('^%.') then goto continue end

    -- Skip binary files
    if M.is_binary_file(file_path) then goto continue end

    -- Read content
    local lines = vim.fn.readfile(file_path)
    if lines then
      table.insert(results, {
        path = file_path,
        content = table.concat(lines, '\n'),
      })
    end

    ::continue::
  end
  return results
end

return M
