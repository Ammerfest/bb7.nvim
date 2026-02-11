-- File/diff rendering for preview pane
local M = {}

local shared = require('bb7.panes.preview.shared')

-- Get local file content (from buffer if open, otherwise from disk)
local function get_local_content(path, is_external)
  local client = require('bb7.client')
  local project_root = client.get_project_root() or vim.fn.getcwd()
  local full_path = is_external and path or (project_root .. '/' .. path)

  -- Check if file is open in a buffer
  local bufnr = vim.fn.bufnr(full_path)
  if bufnr ~= -1 and vim.api.nvim_buf_is_loaded(bufnr) then
    local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
    return table.concat(lines, '\n')
  end

  -- Read from disk
  local file = io.open(full_path, 'r')
  if file then
    local content = file:read('*a')
    file:close()
    return content
  end

  return nil
end

-- Render diff mode (unified diff with red/green highlighting)
function M.render_diff()
  if not shared.state.buf or not vim.api.nvim_buf_is_valid(shared.state.buf) then
    return
  end

  if not shared.state.current_file then
    vim.bo[shared.state.buf].modifiable = true
    vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, { '  No file selected' })
    vim.bo[shared.state.buf].modifiable = false
    return
  end

  local file = shared.state.current_file

  -- Get old content: local file for ~M (conflict), context for M
  local old_content
  if file.status == '~M' then
    old_content = get_local_content(file.path, file.external)
    if not old_content then
      vim.bo[shared.state.buf].modifiable = true
      vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, { '  Error: could not read local file' })
      vim.bo[shared.state.buf].modifiable = false
      return
    end
  else
    old_content = file.context_content or ''
  end

  -- Get new content: LLM output
  local new_content = file.output_content or ''

  -- Compute diff using vim.diff (backed by xdiff, same as git)
  local diff_text = vim.diff(old_content, new_content, { ctxlen = 3 })

  if not diff_text or diff_text == '' then
    vim.bo[shared.state.buf].modifiable = true
    vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, { '  No differences found' })
    vim.bo[shared.state.buf].modifiable = false
    return
  end

  -- Split diff into lines and apply highlighting
  local lines = vim.split(diff_text, '\n', { plain = true })
  -- Remove trailing empty line if present
  if #lines > 0 and lines[#lines] == '' then
    table.remove(lines)
  end

  local highlights = {}  -- { line_idx, hl_group }
  for i, line in ipairs(lines) do
    local first_char = line:sub(1, 1)
    if first_char == '+' then
      table.insert(highlights, { i - 1, 'BB7DiffAdd' })
    elseif first_char == '-' then
      table.insert(highlights, { i - 1, 'BB7DiffRemove' })
    elseif first_char == '@' then
      table.insert(highlights, { i - 1, 'BB7DiffHunk' })
    end
  end

  -- Update buffer
  vim.bo[shared.state.buf].modifiable = true
  vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, lines)
  vim.bo[shared.state.buf].modifiable = false
  vim.bo[shared.state.buf].filetype = 'diff'

  -- Apply highlights
  vim.api.nvim_buf_clear_namespace(shared.state.buf, shared.ns_id, 0, -1)
  for _, hl in ipairs(highlights) do
    vim.api.nvim_buf_add_highlight(shared.state.buf, shared.ns_id, hl[2], hl[1], 0, -1)
  end

  -- Scroll to top
  if shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
    vim.api.nvim_win_set_cursor(shared.state.win, { 1, 0 })
  end
end

-- Render file mode with treesitter syntax highlighting
function M.render_file()
  if not shared.state.buf or not vim.api.nvim_buf_is_valid(shared.state.buf) then
    return
  end

  if not shared.state.current_file then
    vim.bo[shared.state.buf].modifiable = true
    vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, { '  No file selected' })
    vim.bo[shared.state.buf].modifiable = false
    return
  end

  local client = require('bb7.client')
  local file = shared.state.current_file

  -- Determine what to show based on file status
  local action, path
  if file.has_output then
    action = 'get_output_file'
    path = file.path
  else
    action = 'get_context_file'
    path = file.path
  end

  client.request({ action = action, path = path }, function(response, err)
    if err then
      vim.bo[shared.state.buf].modifiable = true
      vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, { '  Error loading file: ' .. err })
      vim.bo[shared.state.buf].modifiable = false
      return
    end

    local lines = {}

    -- Show warning for out-of-sync files
    if file.status == '~' then
      table.insert(lines, '  Local file differs from context. Use "u" to update context.')
      table.insert(lines, '')
    elseif file.status == '~M' then
      table.insert(lines, '  Both local and LLM changed this file. Manual resolution required.')
      table.insert(lines, '')
    end

    -- File content
    local content = response.content or ''
    local content_lines = vim.split(content, '\n', { plain = true })
    for _, line in ipairs(content_lines) do
      table.insert(lines, line)
    end

    -- Update buffer
    vim.bo[shared.state.buf].modifiable = true
    vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, lines)
    vim.bo[shared.state.buf].modifiable = false

    -- Set filetype using Neovim's built-in detection
    local filetype = vim.filetype.match({ filename = file.path })

    if filetype then
      vim.bo[shared.state.buf].filetype = filetype
      -- Start treesitter highlighting if available
      local ok, _ = pcall(vim.treesitter.start, shared.state.buf, filetype)
      if not ok then
        -- Treesitter parser not available for this filetype, fall back to syntax
        pcall(vim.cmd, 'syntax enable')
      end
    else
      vim.bo[shared.state.buf].filetype = ''
      -- Stop any active treesitter highlighting
      pcall(vim.treesitter.stop, shared.state.buf)
    end

    -- Apply warning highlights for header lines
    vim.api.nvim_buf_clear_namespace(shared.state.buf, shared.ns_id, 0, -1)
    if file.status == '~' then
      vim.api.nvim_buf_add_highlight(shared.state.buf, shared.ns_id, 'DiagnosticWarn', 0, 0, -1)
    elseif file.status == '~M' then
      vim.api.nvim_buf_add_highlight(shared.state.buf, shared.ns_id, 'DiagnosticError', 0, 0, -1)
    end

    -- Scroll to top
    if shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
      vim.api.nvim_win_set_cursor(shared.state.win, { 1, 0 })
    end
  end)
end

return M
