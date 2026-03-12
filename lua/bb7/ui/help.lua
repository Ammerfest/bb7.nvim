-- Help popup: shows keybinding reference for the current context
local M = {}

local shared = require('bb7.ui.shared')

local help_win = nil
local help_buf = nil

-- Keybinding definitions per pane
-- Each entry: { key, description }
local pane_bindings = {
  [1] = { -- Chats
    title = 'Chats',
    keys = {
      { '<CR>', 'Select chat' },
      { 'n', 'New chat' },
      { 'r', 'Rename chat' },
      { 'd', 'Delete chat' },
      { 'p', 'Toggle pin' },
      { 'u', 'Unlock chat' },
      { 'm', 'Move to project/global' },
      { '<C-s>', 'Toggle project/global' },
    },
  },
  [2] = { -- Files
    title = 'Files',
    keys = {
      { '<CR>', 'Toggle collapse' },
      { 'x', 'Remove / reject' },
      { 'u', 'Update context' },
      { 'U', 'Update all' },
      { 'p', 'Put (apply) file' },
      { 'P', 'Put all' },
      { 'r', 'Toggle read-only' },
    },
  },
  [3] = { -- Info
    title = 'Info',
    keys = {
      { 'r', 'Refresh balance' },
      { 'R', 'Reset session cost' },
      { '<C-g>', 'Edit global instructions' },
      { '<C-p>', 'Edit project instructions' },
    },
  },
  [4] = { -- Preview
    title = 'Preview',
    keys = {
      { ']]', 'Next anchor' },
      { '[[', 'Previous anchor' },
      { ']u', 'Next user message' },
      { '[u', 'Previous user message' },
      { '<CR>', 'Toggle reasoning block' },
      { 'K', 'Message info' },
      { '<C-f>', 'Fork chat' },
      { '<C-e>', 'Edit message' },
      { '<C-r>', 'Reuse files in new chat' },
    },
  },
  [5] = { -- Input
    title = 'Input',
    keys = {
      { '<CR>', 'Send (normal mode)' },
      { '<S-CR>', 'Send (insert mode)' },
      { 'M', 'Model picker' },
      { 'R', 'Cycle reasoning level' },
    },
  },
}

local global_bindings = {
  title = 'Global',
  keys = {
    { 'gc', 'Auto focus (chat)' },
    { 'gf', 'File focus' },
    { 'gd', 'Diff focus' },
    { 'zd', 'Toggle full diff' },
    { '<C-n>', 'Next file' },
    { '<C-p>', 'Previous file' },
    { '<C-d>', 'Scroll preview down' },
    { '<C-u>', 'Scroll preview up' },
    { '<C-x>', 'Cancel request' },
  },
}

local nav_bindings = {
  title = 'Navigation',
  keys = {
    { 'g1-g5', 'Switch pane' },
    { '<Tab>', 'Next pane' },
    { '<S-Tab>', 'Previous pane' },
    { '<C-c>', 'Close BB-7' },
    { '<Esc>', 'Close BB-7' },
  },
}

-- Format a section into lines and record highlights
-- Returns: lines appended to `out_lines`, highlights appended to `out_hls`
local function format_section(section, out_lines, out_hls, key_col_width)
  -- Section header
  local header = '── ' .. section.title .. ' '
  table.insert(out_lines, header)
  table.insert(out_hls, { line = #out_lines - 1, group = 'BB7BorderActive' })

  for _, binding in ipairs(section.keys) do
    local key = binding[1]
    local desc = binding[2]
    local pad = string.rep(' ', key_col_width - #key)
    local line = '  ' .. key .. pad .. desc
    table.insert(out_lines, line)
    -- Highlight the key portion
    table.insert(out_hls, {
      line = #out_lines - 1,
      group = 'BB7HintKey',
      col_start = 2,
      col_end = 2 + #key,
    })
  end
end

function M.toggle()
  -- Close if already open
  if help_win and vim.api.nvim_win_is_valid(help_win) then
    M.close()
    return
  end

  local active_pane = shared.state.active_pane or 1
  local pane_section = pane_bindings[active_pane]

  -- Build content
  local lines = {}
  local highlights = {}
  local key_col_width = 10  -- Fixed column width for key alignment

  -- Current pane section first
  if pane_section then
    format_section(pane_section, lines, highlights, key_col_width)
    table.insert(lines, '')
  end

  -- Global section
  format_section(global_bindings, lines, highlights, key_col_width)
  table.insert(lines, '')

  -- Navigation section
  format_section(nav_bindings, lines, highlights, key_col_width)

  -- Calculate popup dimensions
  local max_width = 0
  for _, line in ipairs(lines) do
    max_width = math.max(max_width, vim.fn.strwidth(line))
  end
  max_width = max_width + 2  -- Padding

  local height = #lines
  local total_width = vim.o.columns
  local total_height = vim.o.lines

  -- Cap dimensions
  if height > total_height - 4 then
    height = total_height - 4
  end
  if max_width > total_width - 4 then
    max_width = total_width - 4
  end

  -- Center the popup
  local row = math.floor((total_height - height) / 2)
  local col = math.floor((total_width - max_width) / 2)

  -- Prevent BB-7 auto-close before opening the floating window
  -- Set directly to avoid changing the hint line
  shared.state.picker_open = true

  -- Create buffer
  help_buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(help_buf, 0, -1, false, lines)
  vim.bo[help_buf].modifiable = false
  vim.bo[help_buf].bufhidden = 'wipe'

  -- Create floating window
  help_win = vim.api.nvim_open_win(help_buf, true, {
    relative = 'editor',
    row = row,
    col = col,
    width = max_width,
    height = height,
    style = 'minimal',
    border = 'rounded',
    title = ' Keybindings ',
    title_pos = 'center',
    zindex = 200,
  })

  -- Apply highlights
  local ns = vim.api.nvim_create_namespace('bb7_help')
  for _, hl in ipairs(highlights) do
    if hl.col_start then
      vim.api.nvim_buf_add_highlight(help_buf, ns, hl.group, hl.line, hl.col_start, hl.col_end)
    else
      vim.api.nvim_buf_add_highlight(help_buf, ns, hl.group, hl.line, 0, -1)
    end
  end

  -- Extend section header lines with ─ to fill width
  for i, line in ipairs(lines) do
    if line:match('^── .+ $') then
      local display_width = vim.fn.strwidth(line)
      local fill = string.rep('─', max_width - display_width)
      local new_line = line .. fill
      vim.bo[help_buf].modifiable = true
      vim.api.nvim_buf_set_lines(help_buf, i - 1, i, false, { new_line })
      vim.bo[help_buf].modifiable = false
      -- Re-apply highlight for the full line
      vim.api.nvim_buf_add_highlight(help_buf, ns, 'BB7BorderActive', i - 1, 0, -1)
    end
  end

  -- Window options
  vim.wo[help_win].cursorline = false
  vim.wo[help_win].wrap = false

  -- Keymaps to close
  local close = function() M.close() end
  local opts = { buffer = help_buf, nowait = true, silent = true }
  vim.keymap.set('n', '?', close, opts)
  vim.keymap.set('n', 'q', close, opts)
  vim.keymap.set('n', '<Esc>', close, opts)

  -- Auto-close when focus leaves the help window (catches all navigation keys)
  local augroup = vim.api.nvim_create_augroup('BB7Help', { clear = true })
  vim.api.nvim_create_autocmd('WinLeave', {
    group = augroup,
    buffer = help_buf,
    callback = function()
      vim.schedule(function()
        M.close()
      end)
    end,
  })
  vim.api.nvim_create_autocmd('WinClosed', {
    group = augroup,
    pattern = tostring(help_win),
    once = true,
    callback = function()
      M.close()
    end,
  })
end

function M.close()
  if not help_win then return end
  local win = help_win
  help_win = nil
  help_buf = nil
  pcall(vim.api.nvim_del_augroup_by_name, 'BB7Help')
  if vim.api.nvim_win_is_valid(win) then
    vim.api.nvim_win_close(win, true)
  end
  -- Return focus to the active BB-7 pane
  local active = shared.state.active_pane or 1
  local pane = shared.state.panes[active]
  if pane and pane.win and vim.api.nvim_win_is_valid(pane.win) then
    vim.api.nvim_set_current_win(pane.win)
  end
  vim.schedule(function()
    shared.state.picker_open = false
  end)
end

return M
