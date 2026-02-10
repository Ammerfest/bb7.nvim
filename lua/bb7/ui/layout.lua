-- Layout + scrollbar helpers for UI panes
local M = {}

local shared = require('bb7.ui.shared')

-- Scrollbar characters
local SCROLLBAR_BORDER = '│'  -- Normal border (thin)
local SCROLLBAR_THUMB = '▐'   -- Scrollbar thumb (thick)

-- Calculate window dimensions and positions
-- Border adds 2 to each dimension (1 on each side)
function M.calc_layout()
  local total_width = vim.o.columns
  local total_height = vim.o.lines - 2 -- Reserve 1 for hint line, 1 for cmdline
  if total_width < 20 then
    total_width = 20
  end
  if total_height < 6 then
    total_height = 6
  end

  -- Left column gets 30% of total width (including its border), min 40 chars
  local left_total = math.max(40, math.floor(total_width * 0.30))
  local right_total = total_width - left_total
  if right_total < 10 then
    right_total = 10
    left_total = total_width - right_total
  end

  -- Content dimensions = total - 2 (for borders)
  local left_content_width = math.max(1, left_total - 2)
  local right_content_width = math.max(1, right_total - 2)

  -- Left column: Info at bottom (fixed 5 lines), Chats and Context split the rest
  local provider_total = math.min(5, total_height) -- Including borders (3 content lines + 2 border)
  local left_remaining = math.max(0, total_height - provider_total)
  local chats_total = math.max(2, math.floor(left_remaining * 0.5))
  local context_total = math.max(2, left_remaining - chats_total)

  local chats_content_height = math.max(1, chats_total - 2)
  local context_content_height = math.max(1, context_total - 2)
  local provider_content_height = math.max(1, provider_total - 2)

  -- Right column: preview is larger, input is smaller
  local input_total = math.min(8, total_height) -- Including borders
  local preview_total = math.max(2, total_height - input_total)

  local preview_content_height = math.max(1, preview_total - 2)
  local input_content_height = math.max(1, input_total - 2)

  return {
    -- Pane 1: Chats (top-left)
    [1] = {
      row = 0,
      col = 0,
      width = left_content_width,
      height = chats_content_height,
    },
    -- Pane 2: Context (middle-left)
    [2] = {
      row = chats_total,
      col = 0,
      width = left_content_width,
      height = context_content_height,
    },
    -- Pane 3: Info (bottom-left, fixed height)
    [3] = {
      row = chats_total + context_total,
      col = 0,
      width = left_content_width,
      height = provider_content_height,
    },
    -- Pane 4: Preview (top-right, larger)
    [4] = {
      row = 0,
      col = left_total,
      width = right_content_width,
      height = preview_content_height,
    },
    -- Pane 5: Input (bottom-right, smaller)
    [5] = {
      row = preview_total,
      col = left_total,
      width = right_content_width,
      height = input_content_height,
    },
    -- Hint line at very bottom of screen
    hint = {
      row = vim.o.lines - 2, -- Last visible row (before cmdline)
      col = 0,
      width = total_width,
      height = 1,
    },
  }
end

-- Calculate scrollbar position and height
-- Returns (start, height) or nil if scrollbar not needed
local function calc_scrollbar(win, buf, scroll_area_height)
  if not win or not vim.api.nvim_win_is_valid(win) then
    return nil
  end
  if not buf or not vim.api.nvim_buf_is_valid(buf) then
    return nil
  end

  local total_lines = vim.api.nvim_buf_line_count(buf)
  local win_height = vim.api.nvim_win_get_height(win)

  -- Don't show scrollbar if content fits in window
  if total_lines <= win_height then
    return nil
  end

  -- Get current scroll position (1-indexed line at top of window)
  local topline = vim.fn.line('w0', win)
  local max_position = total_lines - win_height

  -- Calculate thumb height (proportional to visible content)
  local thumb_height = math.max(1, math.floor((win_height / total_lines) * scroll_area_height))

  -- Calculate thumb position
  local position = topline - 1  -- 0-indexed
  if position >= max_position then
    -- At bottom: snap to end
    return scroll_area_height - thumb_height, thumb_height
  end

  -- Calculate start position (ceiling to move scrollbar once we scroll)
  local start = math.ceil((position / max_position) * (scroll_area_height - thumb_height))
  return start, thumb_height
end

-- Create or update scrollbar for a pane
function M.update_scrollbar(pane_id, layout)
  local pane = shared.state.panes[pane_id]
  if not pane or not pane.win or not vim.api.nvim_win_is_valid(pane.win) then
    return
  end

  local l = layout[pane_id]
  local scroll_height = l.height  -- Inner height (same as content area)

  -- Calculate scrollbar state
  local scroll_start, scroll_thumb_height = calc_scrollbar(pane.win, pane.buf, scroll_height)

  -- Hide scrollbar if content fits in window
  if not scroll_start then
    -- Hide existing scrollbar
    local sb = shared.state.scrollbars[pane_id]
    if sb and sb.win and vim.api.nvim_win_is_valid(sb.win) then
      vim.api.nvim_win_hide(sb.win)
    end
    return
  end

  -- Build scrollbar content (mix of border and thumb characters)
  local lines = {}
  for i = 0, scroll_height - 1 do
    if i >= scroll_start and i < scroll_start + scroll_thumb_height then
      table.insert(lines, SCROLLBAR_THUMB)
    else
      table.insert(lines, SCROLLBAR_BORDER)
    end
  end

  -- Create or update scrollbar window
  local sb = shared.state.scrollbars[pane_id]
  if not sb or not sb.buf or not vim.api.nvim_buf_is_valid(sb.buf) then
    -- Create new buffer
    local buf = vim.api.nvim_create_buf(false, true)
    vim.bo[buf].bufhidden = 'wipe'
    vim.bo[buf].buftype = 'nofile'
    sb = { buf = buf, win = nil }
    shared.state.scrollbars[pane_id] = sb
  end

  -- Update buffer content
  vim.bo[sb.buf].modifiable = true
  vim.api.nvim_buf_set_lines(sb.buf, 0, -1, false, lines)
  vim.bo[sb.buf].modifiable = false

  -- Scrollbar position: right border of the pane
  -- The pane window is at (l.row, l.col) with size (l.width, l.height)
  -- With border='rounded', the right border is at col + width + 1
  -- We position the scrollbar overlay at row+1 (inside top border), col+width+1 (on right border)
  local sb_row = l.row + 1  -- Skip top border
  local sb_col = l.col + l.width + 1  -- Right border column

  if not sb.win or not vim.api.nvim_win_is_valid(sb.win) then
    -- Create new window
    sb.win = vim.api.nvim_open_win(sb.buf, false, {
      relative = 'editor',
      row = sb_row,
      col = sb_col,
      width = 1,
      height = scroll_height,
      style = 'minimal',
      focusable = false,
      zindex = 100,  -- Above pane borders
      noautocmd = true,
    })
    -- Match border color to pane's active state
    local is_active = shared.state.active_pane == pane_id
    local sb_hl = is_active and 'BB7BorderActive' or 'BB7BorderInactive'
    vim.api.nvim_set_option_value('winhighlight', 'NormalFloat:' .. sb_hl, { win = sb.win })
  else
    -- Update existing window position
    vim.api.nvim_win_set_config(sb.win, {
      relative = 'editor',
      row = sb_row,
      col = sb_col,
      width = 1,
      height = scroll_height,
    })
    -- Show if hidden
    -- (nvim_win_set_config will show the window if it was hidden)

    -- Update scrollbar color to match pane's active state
    local is_active = shared.state.active_pane == pane_id
    local sb_hl = is_active and 'BB7BorderActive' or 'BB7BorderInactive'
    vim.api.nvim_set_option_value('winhighlight', 'NormalFloat:' .. sb_hl, { win = sb.win })
  end
end

-- Update scrollbars for all panes
function M.update_all_scrollbars()
  local layout = M.calc_layout()
  for pane_id = 1, 5 do
    M.update_scrollbar(pane_id, layout)
  end
end

-- Update layout on resize
function M.update_layout()
  if not shared.state.is_open then return end

  local layout = M.calc_layout()

  -- Update each pane's position and size
  for pane_id, pane in pairs(shared.state.panes) do
    if pane.win and vim.api.nvim_win_is_valid(pane.win) then
      local l = layout[pane_id]
      vim.api.nvim_win_set_config(pane.win, {
        relative = 'editor',
        row = l.row,
        col = l.col,
        width = l.width,
        height = l.height,
      })
    end
  end

  -- Update hint line
  if shared.state.hint_win and vim.api.nvim_win_is_valid(shared.state.hint_win) then
    local l = layout.hint
    vim.api.nvim_win_set_config(shared.state.hint_win, {
      relative = 'editor',
      row = l.row,
      col = l.col,
      width = l.width,
      height = l.height,
    })
  end

  -- Update scrollbars (positions may have changed)
  M.update_all_scrollbars()
end

-- Format title with pane display number (indented one char)
-- If title_fn returns nil, show only the pane number
function M.format_title(pane_id, is_active)
  local pane = shared.PANES[pane_id]
  local title = pane.title_fn()
  local hl = is_active and 'BB7TitleActive' or 'BB7TitleInactive'

  if title then
    return {
      { ' [' .. pane.display .. '] ' .. title .. ' ', hl },
    }
  else
    return {
      { ' [' .. pane.display .. '] ', hl },
    }
  end
end

-- Format footer for bottom-right info (if pane has footer_fn)
function M.format_footer(pane_id, is_active)
  local pane = shared.PANES[pane_id]
  if not pane.footer_fn then return nil end

  local footer = pane.footer_fn()
  if not footer then return nil end

  local hl = is_active and 'BB7TitleActive' or 'BB7TitleInactive'
  return {
    { ' ' .. footer .. ' ', hl },
  }
end

-- Create a pane window
function M.create_pane(pane_id, layout, is_active)
  local buf = vim.api.nvim_create_buf(false, true)
  vim.bo[buf].bufhidden = 'wipe'
  vim.bo[buf].buftype = 'nofile'

  -- All panes are now implemented, no placeholder content needed

  local border_hl = is_active and 'BB7BorderActive' or 'BB7BorderInactive'
  local l = layout[pane_id]

  local footer = M.format_footer(pane_id, is_active)
  local win = vim.api.nvim_open_win(buf, is_active, {
    relative = 'editor',
    row = l.row,
    col = l.col,
    width = l.width,
    height = l.height,
    style = 'minimal',
    border = 'rounded',
    title = M.format_title(pane_id, is_active),
    title_pos = 'left',
    footer = footer,
    footer_pos = footer and 'right' or nil,
    noautocmd = true,
  })

  -- Set border highlight and cursorline style
  vim.api.nvim_set_option_value('winhighlight',
    'FloatBorder:' .. border_hl .. ',NormalFloat:Normal,CursorLine:BB7Selection',
    { win = win })

  -- Set cursorlineopt (style), actual cursorline enabled/disabled per-pane in update_pane_borders
  vim.wo[win].cursorlineopt = 'line'

  return { win = win, buf = buf }
end

-- Create hint line at bottom
function M.create_hint_line(layout)
  local buf = vim.api.nvim_create_buf(false, true)
  vim.bo[buf].bufhidden = 'wipe'
  vim.bo[buf].buftype = 'nofile'

  local l = layout.hint
  local win = vim.api.nvim_open_win(buf, false, {
    relative = 'editor',
    row = l.row,
    col = l.col,
    width = l.width,
    height = l.height,
    style = 'minimal',
    border = 'none',
    focusable = false,
    noautocmd = true,
  })

  vim.api.nvim_set_option_value('winhighlight', 'NormalFloat:Normal', { win = win })

  return { win = win, buf = buf }
end

return M
