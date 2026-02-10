-- Generic fuzzy picker component (telescope-style)
local M = {}

local state = {
  buf_list = nil,    -- Buffer for results list
  buf_input = nil,   -- Buffer for filter input
  win_list = nil,    -- Window for results list
  win_input = nil,   -- Window for filter input
  items = {},        -- All items
  filtered = {},     -- Filtered items
  selected_idx = 1,  -- Currently highlighted item (1-indexed)
  filter_text = '',  -- Current filter string
  on_select = nil,   -- Callback when item selected
  on_cancel = nil,   -- Callback when picker cancelled
  on_toggle_favorite = nil, -- Callback for favorite toggle
  format_item = nil, -- Function to format item for display
  get_filter_text = nil, -- Function to get searchable text from item
  is_favorite = nil, -- Function to check if item is favorite
  augroup = nil,     -- Autocmd group
}

local ns_id = vim.api.nvim_create_namespace('bb7_picker')

-- Simple fuzzy match: all characters must appear in order
local function fuzzy_match(text, pattern)
  if pattern == '' then return true, 0 end

  text = text:lower()
  pattern = pattern:lower()

  local ti = 1
  local score = 0
  local last_match = 0

  for pi = 1, #pattern do
    local pc = pattern:sub(pi, pi)
    local found = false

    while ti <= #text do
      local tc = text:sub(ti, ti)
      ti = ti + 1

      if tc == pc then
        found = true
        -- Bonus for consecutive matches
        if ti - 1 == last_match + 1 then
          score = score + 10
        end
        -- Bonus for match at word boundary
        if ti == 2 or text:sub(ti - 2, ti - 2):match('[^%w]') then
          score = score + 5
        end
        last_match = ti - 1
        score = score + 1
        break
      end
    end

    if not found then
      return false, 0
    end
  end

  return true, score
end

-- Filter and sort items based on current filter text
local function update_filtered()
  state.filtered = {}

  for _, item in ipairs(state.items) do
    local text = state.get_filter_text(item)
    local matches, score = fuzzy_match(text, state.filter_text)

    if matches then
      table.insert(state.filtered, {
        item = item,
        score = score,
        is_fav = state.is_favorite and state.is_favorite(item) or false,
      })
    end
  end

  -- Sort: favorites first, then by score (descending), then alphabetically
  table.sort(state.filtered, function(a, b)
    if a.is_fav ~= b.is_fav then
      return a.is_fav
    end
    if a.score ~= b.score then
      return a.score > b.score
    end
    return state.get_filter_text(a.item) < state.get_filter_text(b.item)
  end)

  -- Clamp selection
  state.selected_idx = math.max(1, math.min(state.selected_idx, #state.filtered))
end

-- Render the list
local function render_list()
  if not state.buf_list or not vim.api.nvim_buf_is_valid(state.buf_list) then
    return
  end

  local lines = {}
  local highlights = {}

  -- Get window height for visible items
  local max_visible = 15
  if state.win_list and vim.api.nvim_win_is_valid(state.win_list) then
    max_visible = vim.api.nvim_win_get_height(state.win_list)
  end

  -- Calculate scroll offset to keep selection visible
  local scroll_offset = 0
  if state.selected_idx > max_visible then
    scroll_offset = state.selected_idx - max_visible
  end

  for i = 1, math.min(#state.filtered, max_visible) do
    local idx = i + scroll_offset
    if idx > #state.filtered then break end

    local entry = state.filtered[idx]
    local line = state.format_item(entry.item, entry.is_fav)
    table.insert(lines, line)

    -- Highlight selected line
    if idx == state.selected_idx then
      table.insert(highlights, {
        line = i - 1,
        hl = 'BB7Selection',
      })
    end
  end

  -- Show empty state
  if #lines == 0 then
    table.insert(lines, '  No matches')
    table.insert(highlights, { line = 0, hl = 'Comment' })
  end

  vim.bo[state.buf_list].modifiable = true
  vim.api.nvim_buf_set_lines(state.buf_list, 0, -1, false, lines)
  vim.bo[state.buf_list].modifiable = false

  -- Apply highlights
  vim.api.nvim_buf_clear_namespace(state.buf_list, ns_id, 0, -1)
  for _, hl in ipairs(highlights) do
    vim.api.nvim_buf_add_highlight(state.buf_list, ns_id, hl.hl, hl.line, 0, -1)
  end
end

-- Update filter and re-render
local function on_filter_changed()
  -- Get current filter text from input buffer
  local lines = vim.api.nvim_buf_get_lines(state.buf_input, 0, 1, false)
  state.filter_text = lines[1] or ''

  -- Re-filter and render
  update_filtered()
  render_list()
end

-- Move selection up/down
local function move_selection(delta)
  if #state.filtered == 0 then return end

  state.selected_idx = state.selected_idx + delta
  if state.selected_idx < 1 then
    state.selected_idx = 1
  elseif state.selected_idx > #state.filtered then
    state.selected_idx = #state.filtered
  end

  render_list()
end

-- Jump to top/bottom
local function jump_to_top()
  if #state.filtered == 0 then return end
  state.selected_idx = 1
  render_list()
end

local function jump_to_bottom()
  if #state.filtered == 0 then return end
  state.selected_idx = #state.filtered
  render_list()
end

-- Page scroll (half window)
local function page_down()
  if #state.filtered == 0 then return end
  local max_visible = 15
  if state.win_list and vim.api.nvim_win_is_valid(state.win_list) then
    max_visible = vim.api.nvim_win_get_height(state.win_list)
  end
  move_selection(math.floor(max_visible / 2))
end

local function page_up()
  if #state.filtered == 0 then return end
  local max_visible = 15
  if state.win_list and vim.api.nvim_win_is_valid(state.win_list) then
    max_visible = vim.api.nvim_win_get_height(state.win_list)
  end
  move_selection(-math.floor(max_visible / 2))
end

-- Confirm selection
local function confirm()
  if #state.filtered == 0 then return end

  local entry = state.filtered[state.selected_idx]
  if entry and state.on_select then
    M.close()
    state.on_select(entry.item)
  end
end

-- Toggle favorite for selected item
local function toggle_favorite()
  if #state.filtered == 0 then return end

  local entry = state.filtered[state.selected_idx]
  if entry and state.on_toggle_favorite then
    state.on_toggle_favorite(entry.item)
    -- Update the is_fav status but don't re-sort (will re-sort on filter change)
    entry.is_fav = not entry.is_fav
    render_list()
  end
end

-- Close the picker
-- cancelled: if true, calls on_cancel callback
function M.close(cancelled)
  if state.augroup then
    vim.api.nvim_del_augroup_by_id(state.augroup)
    state.augroup = nil
  end

  if state.win_input and vim.api.nvim_win_is_valid(state.win_input) then
    vim.api.nvim_win_close(state.win_input, true)
  end
  if state.win_list and vim.api.nvim_win_is_valid(state.win_list) then
    vim.api.nvim_win_close(state.win_list, true)
  end

  state.win_input = nil
  state.win_list = nil
  state.buf_input = nil
  state.buf_list = nil

  -- Notify UI that picker is closed and refocus Input pane
  local ok, ui = pcall(require, 'bb7.ui')
  if ok then
    -- Schedule to run after window close events settle
    vim.schedule(function()
      -- Only refocus if picker is actually closed (not reopening)
      -- Check if our windows still don't exist
      if state.win_input == nil and state.win_list == nil then
        ui.set_picker_open(false)
        -- Refocus Input pane (pane 5) if BB7 is still open
        if ui.is_open() then
          local panes_input = require('bb7.panes.input')
          panes_input.focus_insert()
        end
      end
    end)
  end

  if cancelled and state.on_cancel then
    state.on_cancel()
  end
end

-- Setup keymaps for the input buffer
local function setup_keymaps()
  local buf = state.buf_input
  local opts = { buffer = buf, nowait = true, silent = true }

  -- Navigation (insert mode)
  vim.keymap.set('i', '<C-n>', function() move_selection(1) end, opts)
  vim.keymap.set('i', '<C-p>', function() move_selection(-1) end, opts)
  vim.keymap.set('i', '<Down>', function() move_selection(1) end, opts)
  vim.keymap.set('i', '<Up>', function() move_selection(-1) end, opts)
  vim.keymap.set('i', '<C-d>', page_down, opts)
  vim.keymap.set('i', '<C-u>', page_up, opts)

  -- Confirm (insert mode)
  vim.keymap.set('i', '<CR>', confirm, opts)
  vim.keymap.set('i', '<C-y>', confirm, opts)

  -- Toggle favorite (insert mode)
  vim.keymap.set('i', '<C-f>', toggle_favorite, opts)

  -- Cancel (insert mode goes to normal, normal Esc closes)
  vim.keymap.set('i', '<Esc>', '<Esc>', opts)  -- Let it go to normal mode
  vim.keymap.set('n', '<Esc>', function() M.close(true) end, opts)

  -- Navigation (normal mode)
  vim.keymap.set('n', '<C-n>', function() move_selection(1) end, opts)
  vim.keymap.set('n', '<C-p>', function() move_selection(-1) end, opts)
  vim.keymap.set('n', 'j', function() move_selection(1) end, opts)
  vim.keymap.set('n', 'k', function() move_selection(-1) end, opts)
  vim.keymap.set('n', '<C-d>', page_down, opts)
  vim.keymap.set('n', '<C-u>', page_up, opts)
  vim.keymap.set('n', 'gg', jump_to_top, opts)
  vim.keymap.set('n', 'G', jump_to_bottom, opts)

  -- Confirm (normal mode)
  vim.keymap.set('n', '<CR>', confirm, opts)

  -- Toggle favorite (normal mode)
  vim.keymap.set('n', '<C-f>', toggle_favorite, opts)

  -- Re-enter insert mode
  vim.keymap.set('n', 'i', 'i', opts)
  vim.keymap.set('n', 'a', 'a', opts)
end

-- Open the picker
-- opts:
--   items: list of items to pick from
--   format_item(item, is_favorite) -> string: format item for display
--   get_filter_text(item) -> string: get searchable text
--   get_id(item) -> string: get unique identifier for item (for selected_id matching)
--   selected_id: initial selection (matched via get_id)
--   is_favorite(item) -> bool: check if item is favorite
--   on_select(item): called when item is selected
--   on_cancel(): called when picker is cancelled
--   on_toggle_favorite(item): called when favorite is toggled
--   title: title for the picker window
function M.open(opts)
  -- Close any existing picker
  M.close()

  -- Notify UI that picker is opening (prevents auto-close)
  local ok, ui = pcall(require, 'bb7.ui')
  if ok then
    ui.set_picker_open(true)
  end

  -- Store options
  state.items = opts.items or {}
  state.format_item = opts.format_item or function(item) return tostring(item) end
  state.get_filter_text = opts.get_filter_text or function(item) return tostring(item) end
  state.is_favorite = opts.is_favorite
  state.on_select = opts.on_select
  state.on_cancel = opts.on_cancel
  state.on_toggle_favorite = opts.on_toggle_favorite
  state.filter_text = ''
  state.selected_idx = 1

  -- Initial filter
  update_filtered()

  -- Find and select the item matching selected_id
  if opts.selected_id and opts.get_id then
    for idx, entry in ipairs(state.filtered) do
      if opts.get_id(entry.item) == opts.selected_id then
        state.selected_idx = idx
        break
      end
    end
  end

  -- Calculate dimensions
  local width = 84
  local list_height = 15
  local input_height = 1
  local total_height = list_height + input_height + 2  -- +2 for borders

  local ui_width = vim.o.columns
  local ui_height = vim.o.lines
  local col = math.floor((ui_width - width) / 2)
  local row = math.floor((ui_height - total_height) / 2)

  -- Create list buffer and window
  state.buf_list = vim.api.nvim_create_buf(false, true)
  vim.bo[state.buf_list].bufhidden = 'wipe'
  vim.bo[state.buf_list].buftype = 'nofile'
  vim.bo[state.buf_list].modifiable = false

  state.win_list = vim.api.nvim_open_win(state.buf_list, false, {
    relative = 'editor',
    row = row,
    col = col,
    width = width,
    height = list_height,
    style = 'minimal',
    border = 'rounded',
    title = opts.title and {{ ' ' .. opts.title .. ' ', 'BB7TitleActive' }} or nil,
    title_pos = 'left',
    focusable = false,
  })

  vim.api.nvim_set_option_value('winhighlight',
    'FloatBorder:BB7BorderActive,NormalFloat:Normal',
    { win = state.win_list })

  -- Create input buffer and window (below list)
  state.buf_input = vim.api.nvim_create_buf(false, true)
  vim.bo[state.buf_input].bufhidden = 'wipe'
  vim.bo[state.buf_input].buftype = 'nofile'

  state.win_input = vim.api.nvim_open_win(state.buf_input, true, {
    relative = 'editor',
    row = row + list_height + 2,  -- Below list + border
    col = col,
    width = width,
    height = input_height,
    style = 'minimal',
    border = 'rounded',
    title = {{ ' Filter ', 'BB7TitleActive' }},
    title_pos = 'left',
  })

  vim.api.nvim_set_option_value('winhighlight',
    'FloatBorder:BB7BorderActive,NormalFloat:Normal',
    { win = state.win_input })

  -- Setup keymaps
  setup_keymaps()

  -- Setup autocmd for filter changes
  state.augroup = vim.api.nvim_create_augroup('BB7Picker', { clear = true })
  vim.api.nvim_create_autocmd({ 'TextChanged', 'TextChangedI' }, {
    group = state.augroup,
    buffer = state.buf_input,
    callback = on_filter_changed,
  })

  -- Close when leaving the input window
  vim.api.nvim_create_autocmd('WinLeave', {
    group = state.augroup,
    buffer = state.buf_input,
    callback = function()
      vim.schedule(function() M.close(true) end)
    end,
  })

  -- Initial render
  render_list()

  -- Start in insert mode
  vim.cmd('startinsert')
end

return M
