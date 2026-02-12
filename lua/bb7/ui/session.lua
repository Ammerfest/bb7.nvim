-- UI session state helpers
local M = {}

local shared = require('bb7.ui.shared')
local preview_shared = require('bb7.panes.preview.shared')

-- Save view state for all panes before closing
function M.save_session_state()
  shared.session_state.active_pane = shared.state.active_pane
  shared.session_state.first_open = false
  shared.session_state.preview_autoscroll = preview_shared.state.autoscroll

  for pane_id, pane in pairs(shared.state.panes) do
    if pane.win and vim.api.nvim_win_is_valid(pane.win) then
      local cursor = vim.api.nvim_win_get_cursor(pane.win)
      local topline = vim.fn.getwininfo(pane.win)[1].topline
      shared.session_state.pane_views[pane_id] = {
        cursor = cursor,
        topline = topline,
      }
    end
  end
end

-- Restore view state for a pane (called after pane content is rendered)
function M.restore_pane_view(pane_id)
  local pane = shared.state.panes[pane_id]
  local view = shared.session_state.pane_views[pane_id]
  if not pane or not pane.win or not vim.api.nvim_win_is_valid(pane.win) then
    return
  end
  if not view then
    return
  end

  -- Get buffer line count for validation
  local buf = vim.api.nvim_win_get_buf(pane.win)
  local line_count = vim.api.nvim_buf_line_count(buf)

  -- Restore topline (scroll position) first
  if view.topline and view.topline >= 1 and view.topline <= line_count then
    vim.api.nvim_win_call(pane.win, function()
      vim.fn.winrestview({ topline = view.topline })
    end)
  end

  -- Restore cursor position (with validation)
  if view.cursor then
    local row = math.min(view.cursor[1], line_count)
    local col = view.cursor[2]
    -- Validate column against line length
    local line = vim.api.nvim_buf_get_lines(buf, row - 1, row, false)[1] or ''
    col = math.min(col, #line)
    pcall(vim.api.nvim_win_set_cursor, pane.win, { row, col })
  end
end

return M
