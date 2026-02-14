-- Files pane: manage files in chat context with tree view
local M = {}

local client = require('bb7.client')
local utils = require('bb7.utils')
local log = require('bb7.log')

-- Tree symbols
local TREE_EXPANDED = '▼'
local TREE_COLLAPSED = '▶'
local TREE_INDENT = '  '  -- 2 spaces per level

-- Pane state
local state = {
  buf = nil,
  win = nil,
  files = {},          -- List of file entries with status
  tree = nil,          -- Tree structure built from files
  flat_list = {},      -- Flattened tree for display (respects collapsed state)
  collapsed = {},      -- Set of collapsed directory paths
  selected_idx = 1,    -- Currently highlighted item in flat_list
  has_focus = false,   -- Whether this pane currently has focus
  active_chat = nil,   -- Current chat data
  applied_files = {},  -- Files applied since last message (for LLM notification)
  augroup = nil,
  on_file_selected = nil, -- Callback when file highlight changes
}

-- Status indicator display
local STATUS_DISPLAY = {
  [''] = '   ',    -- in context, unchanged
  M = ' M ',       -- LLM modified (output exists)
  A = ' A ',       -- LLM added (new file)
  ['!A'] = '!A ',  -- LLM added, but file exists locally (conflict)
  R = ' R ',       -- read-only
  S = ' S ',       -- section (partial file, immutable)
  ['~'] = ' ~ ',   -- out of sync (local changed)
  ['~M'] = '~M ',  -- out of sync + LLM modified (conflict)
}

-- Status highlight groups
local STATUS_HL = {
  [''] = nil,
  M = 'BB7StatusM',
  A = 'BB7StatusA',
  ['!A'] = 'BB7StatusConflictA',
  R = nil,      -- black/normal
  S = 'BB7StatusM',  -- sections use comment color like M
  ['~'] = nil,  -- black/normal
  ['~M'] = nil, -- handled specially in render (~ black, M comment)
}

-- Forward declaration for render (defined later, used by toggle_collapse)
local render

-- Build a tree structure from a flat list of files
-- Returns a tree where each node has:
--   name: directory or file name
--   path: full path (for directories and files)
--   is_dir: boolean
--   children: list of child nodes (for directories)
--   file: original file entry (for files only)
local function build_tree(files)
  local root = { name = '', path = '', is_dir = true, children = {} }

  for _, file in ipairs(files) do
    local parts = {}
    for part in file.path:gmatch('[^/]+') do
      table.insert(parts, part)
    end

    local current = root
    local path_so_far = ''

    for i, part in ipairs(parts) do
      if i > 1 then
        path_so_far = path_so_far .. '/'
      end
      path_so_far = path_so_far .. part

      local is_last = (i == #parts)

      -- Find or create child
      local found = nil
      for _, child in ipairs(current.children) do
        if child.name == part then
          found = child
          break
        end
      end

      if not found then
        if is_last then
          -- This is a file
          found = {
            name = part,
            path = path_so_far,
            is_dir = false,
            file = file,
          }
        else
          -- This is a directory
          found = {
            name = part,
            path = path_so_far,
            is_dir = true,
            children = {},
          }
        end
        table.insert(current.children, found)
      end

      current = found
    end
  end

  -- Sort children: directories first, then alphabetically
  local function sort_children(node)
    if node.children then
      table.sort(node.children, function(a, b)
        if a.is_dir ~= b.is_dir then
          return a.is_dir  -- directories first
        end
        return a.name < b.name
      end)
      for _, child in ipairs(node.children) do
        sort_children(child)
      end
    end
  end
  sort_children(root)

  return root
end

-- Compress single-child directory chains (like lazygit)
-- e.g., a/b/c/file.txt with single children becomes "a/b/c" as one node
local function compress_tree(node)
  if not node.is_dir or not node.children then
    return node
  end

  -- Recursively compress children first
  for i, child in ipairs(node.children) do
    node.children[i] = compress_tree(child)
  end

  -- If this directory has exactly one child that is also a directory,
  -- merge them into one display node
  while #node.children == 1 and node.children[1].is_dir do
    local child = node.children[1]
    node.name = node.name .. '/' .. child.name
    node.path = child.path
    node.children = child.children
  end

  return node
end

local function center_text(text, width)
  local text_width = vim.fn.strwidth(text)
  if text_width >= width then
    return text
  end
  local left_pad = math.floor((width - text_width) / 2)
  return string.rep(' ', left_pad) .. text
end

local function add_vertical_padding(lines, pad_lines)
  for _ = 1, pad_lines do
    table.insert(lines, '')
  end
end

-- Flatten tree into a list for display, respecting collapsed state
-- Each item has: depth, node, is_last_in_parent
local function flatten_tree(node, depth, collapsed)
  local result = {}

  -- Skip root node itself
  if depth >= 0 then
    table.insert(result, {
      depth = depth,
      node = node,
    })

    -- If this is a collapsed directory, don't recurse
    if node.is_dir and collapsed[node.path] then
      return result
    end
  end

  -- Process children
  if node.children then
    for _, child in ipairs(node.children) do
      local child_items = flatten_tree(child, depth + 1, collapsed)
      for _, item in ipairs(child_items) do
        table.insert(result, item)
      end
    end
  end

  return result
end

-- Toggle collapsed state for directory at current selection
local function toggle_collapse()
  if #state.flat_list == 0 then return end

  local item = state.flat_list[state.selected_idx]
  if not item or not item.node.is_dir then return end

  local path = item.node.path
  if state.collapsed[path] then
    state.collapsed[path] = nil
  else
    state.collapsed[path] = true
  end

  -- Rebuild flat list and re-render
  state.flat_list = flatten_tree(state.tree, -1, state.collapsed)
  render()

  -- Clamp selection
  state.selected_idx = math.min(state.selected_idx, math.max(1, #state.flat_list))

  -- Position cursor
  if state.win and vim.api.nvim_win_is_valid(state.win) and #state.flat_list > 0 then
    vim.api.nvim_win_set_cursor(state.win, { state.selected_idx, 0 })
  end
end


-- Check if a file is out of sync (local differs from context)
-- This is the ONLY status check done in frontend (needs buffer access)
local function is_out_of_sync(path, context_content, is_external)
  -- Find buffer for this file
  local project_root = client.get_project_root() or vim.fn.getcwd()
  local full_path = is_external and path or (project_root .. '/' .. path)

  -- Check if file exists and get its content
  local stat = vim.uv.fs_stat(full_path)
  if not stat then
    -- File doesn't exist locally but is in context - out of sync (deleted)
    return true
  end

  -- Try to get buffer content first (catches unsaved changes)
  local bufnr = vim.fn.bufnr(full_path)
  local local_content

  if bufnr ~= -1 and vim.api.nvim_buf_is_loaded(bufnr) then
    -- Use buffer content (may have unsaved changes)
    local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
    local_content = table.concat(lines, '\n')
  else
    -- Read from disk
    local file = io.open(full_path, 'r')
    if file then
      local_content = file:read('*a')
      file:close()
    end
  end

  if not local_content then
    return false
  end

  -- Compare with normalized line endings
  return not utils.contents_equal(local_content, context_content)
end

-- Build file list with status indicators
-- Uses backend's get_file_statuses for M/A status, frontend only adds ~ for out-of-sync
local function build_file_list(callback)
  if not state.active_chat then
    state.files = {}
    state.tree = { name = '', path = '', is_dir = true, children = {} }
    state.flat_list = {}
    if callback then callback() end
    return
  end

  -- Get file statuses from backend (handles M/A/applied logic)
  client.request({ action = 'get_file_statuses' }, function(response, err)
    if err then
      log.error('Failed to get file statuses: ' .. tostring(err))
      state.files = {}
      state.tree = { name = '', path = '', is_dir = true, children = {} }
      state.flat_list = {}
      if callback then callback() end
      return
    end

    -- Handle null/nil from JSON
    local backend_files = (type(response.files) == 'table') and response.files or {}
    local files = {}
    local pending_checks = #backend_files

    if pending_checks == 0 then
      state.files = files
      state.tree = { name = '', path = '', is_dir = true, children = {} }
      state.flat_list = {}
      if callback then callback() end
      return
    end

    -- Process each file, adding out-of-sync check (frontend only)
    for _, bf in ipairs(backend_files) do
      local entry = {
        path = bf.path,
        in_context = bf.in_context,
        has_output = bf.has_output,
        readonly = bf.readonly,
        external = bf.external,
        context_content = bf.context_content,
        output_content = bf.output_content,
        out_of_sync = false,
        status = bf.status,  -- M, A, S, or '' from backend
        tokens = bf.tokens or 0,
        original_tokens = bf.original_tokens or 0,
        output_tokens = bf.output_tokens or 0,
        start_line = bf.start_line,  -- Section start line (nil for full files)
        end_line = bf.end_line,      -- Section end line (nil for full files)
      }
      table.insert(files, entry)

      -- Check out-of-sync status (needs buffer access, frontend only)
      -- Skip for sections - they are immutable snapshots
      if entry.in_context and entry.context_content and entry.status ~= 'S' then
        entry.out_of_sync = is_out_of_sync(entry.path, entry.context_content, entry.external)

        -- Combine backend status with out-of-sync
        if entry.out_of_sync then
          if entry.status == 'M' then
            entry.status = '~M'  -- Conflict: both local and LLM changed
          elseif entry.status == '' then
            entry.status = '~'   -- Just out of sync
          end
        end
      end

      if entry.readonly and entry.status == '' then
        entry.status = 'R'
      end

      pending_checks = pending_checks - 1
      if pending_checks == 0 then
        state.files = files
        -- Build tree from files
        local raw_tree = build_tree(files)
        state.tree = compress_tree(raw_tree)
        state.flat_list = flatten_tree(state.tree, -1, state.collapsed)
        if callback then callback() end
      end
    end
  end)
end

-- Render the tree view
render = function()
  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return
  end

  -- Get window width for truncation
  local win_width = 40  -- default
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    win_width = vim.api.nvim_win_get_width(state.win)
  end

  local lines = {}
  local highlights = {}

  if not state.active_chat then
    local win_height = 10
    if state.win and vim.api.nvim_win_is_valid(state.win) then
      win_height = vim.api.nvim_win_get_height(state.win)
    end
    local message_lines = 1
    local top_pad = math.max(0, math.floor((win_height - message_lines) / 2))
    add_vertical_padding(lines, top_pad)
    local message = center_text('No chat selected', win_width)
    table.insert(lines, message)
    local message_line = #lines - 1
    table.insert(highlights, {
      line = message_line,
      col_start = 0,
      col_end = #message,
      hl = 'Comment',
    })
  elseif #state.flat_list == 0 then
    local win_height = 10
    if state.win and vim.api.nvim_win_is_valid(state.win) then
      win_height = vim.api.nvim_win_get_height(state.win)
    end
    local message_lines = 1
    local top_pad = math.max(0, math.floor((win_height - message_lines) / 2))
    add_vertical_padding(lines, top_pad)
    local message = center_text('No files added', win_width)
    table.insert(lines, message)
    local message_line = #lines - 1
    table.insert(highlights, {
      line = message_line,
      col_start = 0,
      col_end = #message,
      hl = 'Comment',
    })
  end

  for i, item in ipairs(state.flat_list) do
    local node = item.node
    local depth = item.depth
    local indent = string.rep(TREE_INDENT, depth)

    local line
    local status_str = '   '
    local status_hl = nil

    local is_selected = (i == state.selected_idx)

    if node.is_dir then
      -- Directory line: sel + indent + arrow + name/
      local arrow = state.collapsed[node.path] and TREE_COLLAPSED or TREE_EXPANDED
      line = indent .. arrow .. ' ' .. node.name .. '/'

      -- Truncate based on display width, not byte length (arrow symbols are multi-byte)
      local line_display_width = vim.fn.strwidth(line)
      if line_display_width > win_width then
        line = vim.fn.strcharpart(line, 0, win_width)
      end
      table.insert(lines, line)

      -- Highlight arrow as Comment
      local arrow_start = #indent
      local arrow_end = arrow_start + #arrow
      table.insert(highlights, {
        line = i - 1,
        col_start = arrow_start,
        col_end = arrow_end,
        hl = 'Comment',
      })
    else
      -- File line: sel + indent + status + name + tokens
      local file = node.file
      status_str = STATUS_DISPLAY[file.status] or '   '
      status_hl = STATUS_HL[file.status]

      -- Show sum expression for modified files (original + output)
      local token_str
      if file.original_tokens > 0 and file.output_tokens > 0 then
        token_str = '[' .. utils.format_tokens(file.original_tokens) .. ' + ' .. utils.format_tokens(file.output_tokens) .. ']'
      else
        token_str = '[' .. utils.format_tokens(file.tokens) .. ']'
      end

      -- Calculate available space for name (indent + status + name + space + tokens must fit)
      local fixed_width = #indent + #status_str + 1 + #token_str
      local available_for_name = win_width - fixed_width
      local name_display = node.name

      -- For sections, append line range to name
      if file.start_line and file.end_line then
        name_display = name_display .. ':' .. file.start_line .. '-' .. file.end_line
      end

      if #name_display > available_for_name then
        name_display = name_display:sub(1, available_for_name)
      end

      -- Build line: indent + status + name + padding + tokens
      local padding_needed = win_width - #indent - #status_str - #name_display - #token_str
      local padding = ''
      if padding_needed > 0 then
        padding = string.rep(' ', padding_needed)
      end

      line = indent .. status_str .. name_display .. padding .. token_str

      -- Truncate based on display width, not byte length
      local line_display_width = vim.fn.strwidth(line)
      if line_display_width > win_width then
        -- Truncate to fit (rare edge case)
        line = vim.fn.strcharpart(line, 0, win_width)
      end
      table.insert(lines, line)

      local status_start = #indent

      -- Bold for selected line: filename (not status or tokens)
      if is_selected then
        table.insert(highlights, {
          line = i - 1,
          col_start = status_start + #status_str,
          col_end = status_start + #status_str + #name_display,
          hl = 'BB7Bold',
        })
      end

      -- Status highlight
      -- Special case for ~M: ~ is black (no highlight), M is comment color
      if file.status == '~M' then
        -- Only highlight the M (position 1 in '~M ')
        table.insert(highlights, {
          line = i - 1,
          col_start = status_start + 1,  -- skip the ~
          col_end = status_start + 2,    -- just the M
          hl = 'BB7StatusM',
        })
      elseif status_hl then
        table.insert(highlights, {
          line = i - 1,
          col_start = status_start,
          col_end = status_start + #status_str,
          hl = status_hl,
        })
      end

      -- Token count highlight (always dimmed)
      local token_start = #line - #token_str
      if token_start >= 0 and token_start < #line then
        table.insert(highlights, {
          line = i - 1,
          col_start = token_start,
          col_end = #line,
          hl = 'Comment',
        })
      end
    end
  end

  vim.bo[state.buf].modifiable = true
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
  vim.bo[state.buf].modifiable = false

  -- Apply highlights
  local ns = vim.api.nvim_create_namespace('bb7_context')
  vim.api.nvim_buf_clear_namespace(state.buf, ns, 0, -1)

  for _, hl in ipairs(highlights) do
    vim.api.nvim_buf_add_highlight(state.buf, ns, hl.hl, hl.line, hl.col_start, hl.col_end)
  end
end

-- Get the file entry for the current selection (or nil if directory)
local function get_current_file()
  if #state.flat_list == 0 then return nil end
  local item = state.flat_list[state.selected_idx]
  if item and not item.node.is_dir then
    return item.node.file
  end
  return nil
end

-- Find the first file index (not directory) in flat_list, or 1 if none
local function find_first_file_idx()
  for i, item in ipairs(state.flat_list) do
    if not item.node.is_dir then
      return i
    end
  end
  return 1  -- fallback to first item if no files
end

-- Move selection up/down
local function move_selection(delta)
  if #state.flat_list == 0 then return end

  local new_idx = state.selected_idx + delta

  -- Clamp to valid range
  if new_idx < 1 then
    new_idx = 1
  elseif new_idx > #state.flat_list then
    new_idx = #state.flat_list
  end

  state.selected_idx = new_idx

  -- Re-render to update bold highlighting on selected line
  render()

  -- Move cursor
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    vim.api.nvim_win_set_cursor(state.win, { state.selected_idx, 0 })
  end

  -- Notify about selection change for preview (only for files)
  local file = get_current_file()
  if state.on_file_selected then
    state.on_file_selected(file)
  end
end

-- Cycle through files (skip directories), wrapping at ends
-- delta: 1 for next, -1 for previous
-- modified_only: if true, only cycle through M/~M/A files
function M.cycle_file(delta, modified_only)
  if #state.flat_list == 0 then return end

  -- Build list of candidate indices
  local candidates = {}
  for i, item in ipairs(state.flat_list) do
    if not item.node.is_dir then
      if not modified_only then
        table.insert(candidates, i)
      elseif item.node.file and (item.node.file.status == 'M' or item.node.file.status == '~M' or item.node.file.status == 'A') then
        table.insert(candidates, i)
      end
    end
  end

  if #candidates == 0 then return end

  -- Find current position in candidates
  local current_pos = nil
  for i, idx in ipairs(candidates) do
    if idx == state.selected_idx then
      current_pos = i
      break
    end
  end

  -- Calculate next position (wrap around)
  local next_pos
  if current_pos then
    next_pos = ((current_pos - 1 + delta) % #candidates) + 1
  else
    next_pos = delta > 0 and 1 or #candidates
  end

  state.selected_idx = candidates[next_pos]

  render()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    vim.api.nvim_win_set_cursor(state.win, { state.selected_idx, 0 })
  end

  local file = get_current_file()
  if state.on_file_selected then
    state.on_file_selected(file)
  end
end

-- Keep cursor at column 0 and sync selection
local function lock_cursor_column()
  if state.win and vim.api.nvim_win_is_valid(state.win) then
    local cursor = vim.api.nvim_win_get_cursor(state.win)
    if cursor[2] ~= 0 then
      vim.api.nvim_win_set_cursor(state.win, { cursor[1], 0 })
    end
    -- Sync selection with cursor row
    if cursor[1] ~= state.selected_idx and cursor[1] <= #state.flat_list then
      state.selected_idx = cursor[1]
      -- Re-render to update selection indicator position
      render()
      -- Notify about selection change (only for files)
      local file = get_current_file()
      if state.on_file_selected then
        state.on_file_selected(file)
      end
    end
  end
end

-- Remove selected file: delete output first, then context
local function remove_file()
  local file = get_current_file()
  if not file then
    log.warn('Select a file to remove')
    return
  end

  -- If file has output, delete the output file (dismiss LLM changes)
  if file.has_output then
    client.request({ action = 'output_delete', path = file.path }, function(_, err)
      if err then
        log.error('Failed to delete output file: ' .. tostring(err))
        return
      end
      M.refresh()
    end)
    return
  end

  -- Otherwise, remove from context
  if not file.in_context then
    log.warn('File not in context or output')
    return
  end

  -- For sections, use context_remove_section
  if file.start_line and file.end_line then
    client.request({
      action = 'context_remove_section',
      path = file.path,
      start_line = file.start_line,
      end_line = file.end_line,
    }, function(_, err)
      if err then
        log.error('Failed to remove section: ' .. tostring(err))
        return
      end
      M.refresh()
    end)
    return
  end

  client.request({ action = 'context_remove', path = file.path }, function(_, err)
    if err then
      log.error('Failed to remove file: ' .. tostring(err))
      return
    end

    M.refresh()
  end)
end

-- Update context from local file (re-snapshot)
local function update_context()
  local file = get_current_file()
  if not file or not file.in_context then
    return
  end

  -- Sections are immutable - cannot be updated
  if file.status == 'S' then
    log.info('Sections cannot be updated (immutable)')
    return
  end

  if not file.out_of_sync then
    log.info('File is in sync')
    return
  end

  -- Read current local content
  local project_root = client.get_project_root() or vim.fn.getcwd()
  local full_path = file.external and file.path or (project_root .. '/' .. file.path)

  -- Try buffer first
  local bufnr = vim.fn.bufnr(full_path)
  local content

  if bufnr ~= -1 and vim.api.nvim_buf_is_loaded(bufnr) then
    local lines = vim.api.nvim_buf_get_lines(bufnr, 0, -1, false)
    content = table.concat(lines, '\n')
  else
    local f = io.open(full_path, 'r')
    if f then
      content = f:read('*a')
      f:close()
    end
  end

  if not content then
    log.error('Cannot read file ' .. file.path)
    return
  end

  client.request({ action = 'context_update', path = file.path, content = content }, function(_, err)
    if err then
      log.error('Failed to update context: ' .. tostring(err))
      return
    end

    log.info('Updated ' .. file.path)
    M.refresh()
  end)
end

-- Toggle read-only status for internal context files
local function toggle_readonly()
  local file = get_current_file()
  if not file then
    log.warn('Select a file to update')
    return
  end

  if not file.in_context then
    log.warn('File not in context')
    return
  end

  -- Sections are always read-only
  if file.status == 'S' then
    log.info('Sections are always read-only')
    return
  end

  if file.external then
    log.info('External files are always read-only')
    return
  end

  if not file.readonly and (file.status == 'M' or file.status == '~M') then
    log.warn('Apply changes before marking as read-only')
    return
  end

  local new_state = not file.readonly
  local prompt = new_state and ('Mark "' .. file.path .. '" as read-only?') or ('Make "' .. file.path .. '" writable?')
  local confirmed = vim.fn.confirm(prompt, '&Yes\n&No', 2)
  if confirmed ~= 1 then
    return
  end

  client.request({ action = 'context_set_readonly', path = file.path, readonly = new_state }, function(_, err)
    if err then
      log.error('Failed to update read-only status: ' .. tostring(err))
      return
    end

    M.refresh()
  end)
end

-- Apply (put) selected file - copy output to local and update context
-- Helper to write content to a destination path
local function write_to_destination(file, dest_path, content)
  local project_root = client.get_project_root() or vim.fn.getcwd()
  local full_path = file.external and dest_path or (project_root .. '/' .. dest_path)

  -- Ensure directory exists
  local dir = vim.fn.fnamemodify(full_path, ':h')
  vim.fn.mkdir(dir, 'p')

  local bufnr = vim.fn.bufnr(full_path)
  if bufnr ~= -1 then
    -- Buffer exists: write through Neovim for atomic mtime update.
    -- This preserves undo history and prevents W12 ("file changed since reading").
    local lines = vim.split(content, '\n', { plain = true })
    -- Remove trailing empty string from split (Neovim adds final newline on write)
    if #lines > 0 and lines[#lines] == '' then
      table.remove(lines)
    end
    vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, lines)
    vim.api.nvim_buf_call(bufnr, function()
      vim.cmd('write!')
    end)
  else
    -- No buffer open: write directly (no mtime to desync)
    local f = io.open(full_path, 'w')
    if not f then
      log.error('Cannot write file ' .. dest_path)
      return false
    end
    f:write(content)
    f:close()
  end

  return true
end

local function put_file()
  local file = get_current_file()
  if not file then
    log.warn('Select a file to apply')
    return
  end

  if not file.has_output then
    log.warn('No output to apply')
    return
  end

  -- Already applied (status is blank but has output)
  if file.status == '' and file.has_output then
    log.info('File already applied')
    return
  end

  -- Conflict: file exists locally but wasn't in context
  if file.status == '!A' then
    vim.ui.input({
      prompt = 'File already exists. Write to: ',
      default = file.path,
    }, function(dest_path)
      if not dest_path or dest_path == '' then
        return  -- User cancelled
      end

      -- Choose action based on whether path changed
      local action, params
      if dest_path == file.path then
        -- Overwrite: use regular apply_file
        action = 'apply_file'
        params = { action = action, path = file.path }
      else
        -- Save as different path: use apply_file_as
        action = 'apply_file_as'
        params = { action = action, path = file.path, destination = dest_path }
      end

      client.request(params, function(response, err)
        if err then
          log.error('Failed to apply file: ' .. tostring(err))
          return
        end

        if write_to_destination(file, dest_path, response.content) then
          table.insert(state.applied_files, dest_path)
          if dest_path == file.path then
            log.info('Overwrote ' .. file.path)
          else
            log.info('Saved as ' .. dest_path)
          end
          M.refresh()
        end
      end)
    end)
    return
  end

  -- Use backend's apply_file (updates context to match output)
  client.request({ action = 'apply_file', path = file.path }, function(response, err)
    if err then
      log.error('Failed to apply file: ' .. tostring(err))
      return
    end

    if write_to_destination(file, file.path, response.content) then
      table.insert(state.applied_files, file.path)
      log.info('Applied ' .. file.path)
      M.refresh()
    end
  end)
end

-- Apply all modified files (skips !A conflicts)
local function put_all()
  local to_apply = {}
  local conflicts = 0
  for _, file in ipairs(state.files) do
    -- Apply files with M or A status (not already applied)
    if file.status == 'M' or file.status == 'A' or file.status == '~M' then
      table.insert(to_apply, file)
    elseif file.status == '!A' then
      conflicts = conflicts + 1
    end
  end

  if #to_apply == 0 and conflicts == 0 then
    log.info('No files to apply')
    return
  end

  if #to_apply == 0 and conflicts > 0 then
    log.warn(conflicts .. ' conflicting file(s) skipped - use p to apply individually')
    return
  end

  local applied = 0
  local total = #to_apply

  for _, file in ipairs(to_apply) do
    -- Use backend's apply_file
    client.request({ action = 'apply_file', path = file.path }, function(response, err)
      if err then
        log.error('Failed to apply ' .. file.path)
        applied = applied + 1
        if applied == total then
          M.refresh()
        end
        return
      end

      if write_to_destination(file, file.path, response.content) then
        table.insert(state.applied_files, file.path)
      end

      applied = applied + 1
      if applied == total then
        local msg = 'Applied ' .. total .. ' file(s)'
        if conflicts > 0 then
          msg = msg .. ' (' .. conflicts .. ' conflict(s) skipped)'
        end
        log.info(msg)
        M.refresh()
      end
    end)
  end
end

-- Find the flat_list index for the first file matching any of the given paths
local function find_file_idx_by_paths(paths)
  if not paths or #paths == 0 then return nil end
  local path_set = {}
  for _, p in ipairs(paths) do
    path_set[p] = true
  end
  for i, item in ipairs(state.flat_list) do
    if not item.node.is_dir and item.node.file and path_set[item.node.file.path] then
      return i
    end
  end
  return nil
end

-- Refresh the file list
-- opts (optional table):
--   select_paths: list of paths to prefer for selection (picks first match)
--   callback: function called after refresh completes (legacy positional arg also supported)
function M.refresh(callback_or_opts, _legacy_unused)
  local callback, select_paths
  if type(callback_or_opts) == 'function' then
    callback = callback_or_opts
  elseif type(callback_or_opts) == 'table' then
    callback = callback_or_opts.callback
    select_paths = callback_or_opts.select_paths
  end

  build_file_list(function()
    -- If caller specified paths to select, try those first
    if select_paths and #select_paths > 0 then
      local idx = find_file_idx_by_paths(select_paths)
      if idx then
        state.selected_idx = idx
      end
    end

    -- Clamp selection: prefer selecting a file over a directory (so caret shows)
    if #state.flat_list > 0 then
      if state.selected_idx < 1 or state.selected_idx > #state.flat_list then
        -- Out of range: select first file
        state.selected_idx = find_first_file_idx()
      elseif state.flat_list[state.selected_idx].node.is_dir then
        -- Currently on a directory: try to select first file instead
        state.selected_idx = find_first_file_idx()
      end
    else
      state.selected_idx = 1
    end

    render()

    -- Position cursor
    if state.win and vim.api.nvim_win_is_valid(state.win) then
      if #state.flat_list > 0 then
        vim.api.nvim_win_set_cursor(state.win, { state.selected_idx, 0 })
      else
        vim.api.nvim_win_set_cursor(state.win, { 1, 0 })
      end
    end

    -- Notify about current selection (only for files)
    local file = get_current_file()
    if state.on_file_selected then
      state.on_file_selected(file)
    end

    -- Notify that data changed (for footer updates)
    if state.on_data_changed then
      state.on_data_changed()
    end

    if callback then callback() end
  end)
end

-- Set the active chat (called when chat selection changes)
function M.set_chat(chat)
  state.active_chat = chat
  state.applied_files = {}  -- Clear applied files when switching chats
  state.collapsed = {}      -- Reset tree collapse state
  state.selected_idx = 0    -- Reset selection so refresh will select first file
  M.refresh()
end

-- Setup keymaps
function M.setup_keymaps(buf)
  local opts = { buffer = buf, nowait = true, silent = true }

  -- Navigation
  vim.keymap.set('n', 'j', function() move_selection(1) end, opts)
  vim.keymap.set('n', 'k', function() move_selection(-1) end, opts)

  -- Tree toggle
  vim.keymap.set('n', '<CR>', toggle_collapse, opts)
  vim.keymap.set('n', 'o', toggle_collapse, opts)

  -- Actions
  vim.keymap.set('n', 'x', remove_file, opts)
  vim.keymap.set('n', 'u', update_context, opts)
  vim.keymap.set('n', 'p', put_file, opts)
  vim.keymap.set('n', 'P', put_all, opts)
  vim.keymap.set('n', 'r', toggle_readonly, opts)

  -- Scroll preview pane
  local preview = require('bb7.panes.preview')
  vim.keymap.set('n', '<C-d>', function() preview.scroll_down() end, opts)
  vim.keymap.set('n', '<C-u>', function() preview.scroll_up() end, opts)
end

-- Set callbacks
function M.set_callbacks(callbacks)
  state.on_file_selected = callbacks.on_file_selected
  state.on_data_changed = callbacks.on_data_changed
end

-- Initialize the pane
function M.init(buf, win)
  state.buf = buf
  state.win = win
  state.files = {}
  state.tree = nil
  state.flat_list = {}
  state.collapsed = {}
  -- Preserve selected_idx if already set (for session persistence)
  if not state.selected_idx or state.selected_idx < 1 then
    state.selected_idx = 1
  end
  state.has_focus = false  -- Context pane doesn't have focus initially
  state.active_chat = nil

  -- Setup keymaps
  M.setup_keymaps(buf)

  -- Setup autocmd to lock cursor column
  state.augroup = vim.api.nvim_create_augroup('BB7Context', { clear = true })
  vim.api.nvim_create_autocmd('CursorMoved', {
    group = state.augroup,
    buffer = buf,
    callback = lock_cursor_column,
  })

  -- Initial render (empty)
  render()
end

-- Get hints for this pane
function M.get_hints()
  local file = get_current_file()
  local x_label = 'Remove'
  if file and file.has_output then
    x_label = 'Reject'
  end
  return 'Toggle: <CR> | ' .. x_label .. ': x | Update: u | Read-only: r | Put: p | Put all: P'
end

-- Get currently selected file
function M.get_selected_file()
  return get_current_file()
end

-- Get list of files applied since last message (for LLM notification)
function M.get_applied_files()
  return state.applied_files
end

-- Clear applied files list (call after informing LLM)
function M.clear_applied_files()
  state.applied_files = {}
end

-- Get summary info for footer display
function M.get_summary()
  local file_count = #state.files
  if file_count == 0 then
    return nil
  end

  local total_tokens = 0
  local potential_savings = 0

  for _, file in ipairs(state.files) do
    total_tokens = total_tokens + (file.tokens or 0)
    -- M-status files send both versions, savings = original tokens (removed after applying)
    if file.status == 'M' then
      potential_savings = potential_savings + (file.original_tokens or 0)
    end
  end

  return {
    file_count = file_count,
    total_tokens = total_tokens,
    potential_savings = potential_savings,
    format_tokens = utils.format_tokens,  -- Pass the formatter function
  }
end

-- Set focus state
function M.set_focus(focused)
  state.has_focus = focused
end

-- Cleanup
function M.cleanup()
  if state.augroup then
    vim.api.nvim_del_augroup_by_id(state.augroup)
    state.augroup = nil
  end
  state.buf = nil
  state.win = nil
  state.files = {}
  state.tree = nil
  state.flat_list = {}
  state.collapsed = {}
  state.applied_files = {}
  state.on_file_selected = nil
  state.on_data_changed = nil
end

-- Inject mock file list for screenshot mode (bypasses backend)
function M.set_mock_files(files_list)
  state.files = files_list or {}
  state.active_chat = {} -- Truthy so render doesn't show "No chat selected"
  state.collapsed = {}
  local raw_tree = build_tree(state.files)
  state.tree = compress_tree(raw_tree)
  state.flat_list = flatten_tree(state.tree, -1, state.collapsed)
  -- Select first file (not directory)
  state.selected_idx = find_first_file_idx()
  render()
  if state.win and vim.api.nvim_win_is_valid(state.win) and #state.flat_list > 0 then
    vim.api.nvim_win_set_cursor(state.win, { state.selected_idx, 0 })
  end
end

return M
