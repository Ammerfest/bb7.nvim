-- Provider pane: shows account balance, daily cost total, per-project session total
local M = {}

local client = require('bb7.client')
local utils = require('bb7.utils')

local state = {
  buf = nil,
  win = nil,
  balance = nil,        -- { total_credits, total_usage }
  today_cost = nil,     -- Cached daily cost total (from CSV)
  session_cost = 0,     -- Accumulated cost this project session
  project_root = nil,   -- Project root directory
  customization = nil,  -- { system_override, global_instructions, project_instructions, project_instructions_error }
  context_estimate = nil, -- Estimated token count for full context
  context_limit = nil,    -- Model context_length
  max_completion = nil,   -- Model max_completion_tokens
}

-- Get the session cost file path for the current project
local function session_cost_path()
  if not state.project_root then return nil end
  return state.project_root .. '/.bb7/session_cost'
end

-- Load session cost from per-project file
local function load_session_cost()
  local path = session_cost_path()
  if not path then return end
  local file = io.open(path, 'r')
  if not file then return end
  local content = file:read('*a')
  file:close()
  local saved = tonumber(content)
  if saved then
    state.session_cost = saved
  end
end

-- Save session cost to per-project file
local function save_session_cost()
  local path = session_cost_path()
  if not path then return end
  local dir = vim.fn.fnamemodify(path, ':h')
  vim.fn.mkdir(dir, 'p')
  local file = io.open(path, 'w')
  if file then
    file:write(tostring(state.session_cost))
    file:close()
  end
end

-- Append a usage entry to the global CSV log
local function append_usage_csv(model, usage)
  if not usage or not usage.cost then return end
  local csv_path = vim.fn.expand('~/.bb7/usage.csv')
  local dir = vim.fn.fnamemodify(csv_path, ':h')
  vim.fn.mkdir(dir, 'p')
  local file = io.open(csv_path, 'a')
  if not file then return end
  local timestamp = os.date('%Y-%m-%dT%H:%M:%S')
  local project = state.project_root or ''
  local model_str = model or ''
  local line = string.format('%s,%s,%s,%d,%d,%d,%.6f\n',
    timestamp,
    project,
    model_str,
    usage.prompt_tokens or 0,
    usage.completion_tokens or 0,
    usage.cached_tokens or 0,
    usage.cost)
  file:write(line)
  file:close()
end

-- Read today's total cost from the CSV log
local function read_today_cost()
  local csv_path = vim.fn.expand('~/.bb7/usage.csv')
  local file = io.open(csv_path, 'r')
  if not file then return 0 end
  local today = os.date('%Y-%m-%d')
  local total = 0
  for line in file:lines() do
    -- Lines starting with today's date
    if line:sub(1, #today) == today then
      -- Cost is the last field
      local cost_str = line:match('([^,]+)$')
      local cost = tonumber(cost_str)
      if cost then
        total = total + cost
      end
    end
  end
  file:close()
  return total
end

-- Format a dollar amount with decimal-aligned padding.
-- Returns the formatted string AND the byte offset of the '$' sign
-- so callers can align multiple values at the decimal point.
-- int_width: minimum width for the integer part (before the dot), default 1.
local function format_dollars(amount, int_width)
  if not amount then return '-' end
  int_width = int_width or 1
  local raw = string.format('%.3f', amount)
  local int_part, frac_part = raw:match('^(-?%d+)(%.%d+)$')
  if not int_part then return '$' .. raw end
  local padded_int = string.format('%' .. int_width .. 's', int_part)
  return '$' .. padded_int .. frac_part
end

-- Render the provider info
local function render()
  if not state.buf or not vim.api.nvim_buf_is_valid(state.buf) then
    return
  end

  -- Get window width for right-aligned customization info
  local win_width = state.win and vim.api.nvim_win_is_valid(state.win)
    and vim.api.nvim_win_get_width(state.win) or 30
  local narrow = win_width < 50

  local lines = {}
  local highlights = {} -- { line, col_start, col_end, hl_group }

  -- Helper to build a line with left and optional right content
  local function build_line(left, right, right_hl)
    if right then
      -- Use strdisplaywidth for correct Unicode character width
      local left_width = vim.fn.strdisplaywidth(left)
      local right_width = vim.fn.strdisplaywidth(right)
      local padding = win_width - left_width - right_width - 1
      if padding < 2 then padding = 2 end
      local line = left .. string.rep(' ', padding) .. right
      if right_hl then
        -- Highlight positions use byte offsets, not display width
        table.insert(highlights, { #lines, #left + padding, #line, right_hl })
      end
      return line
    end
    return left
  end

  -- Compute integer-part width for decimal alignment across all three values
  local balance_amount = state.balance
    and (state.balance.total_credits - state.balance.total_usage) or nil
  local today_amount = state.today_cost
  local session_amount = state.session_cost or 0
  local max_int = 1
  for _, amt in ipairs({ balance_amount, today_amount, session_amount }) do
    if amt then
      local int_part = string.format('%.0f', math.abs(amt))
      if #int_part > max_int then max_int = #int_part end
    end
  end

  -- Balance line + system override
  local balance_str = balance_amount and format_dollars(balance_amount, max_int) or '-'
  local left1 = ' Balance: ' .. balance_str
  local right1, hl1 = nil, nil
  if state.customization and state.customization.system_override then
    right1 = narrow and 'Sys. prompt !' or 'Custom system prompt !'
    hl1 = 'BB7SystemOverride'
  end
  table.insert(lines, build_line(left1, right1, hl1))

  -- Today's cost + global instructions
  local today_str = today_amount and format_dollars(today_amount, max_int) or '-'
  local left2 = ' Today:   ' .. today_str
  local right2, hl2 = nil, nil
  if state.customization and state.customization.global_instructions then
    right2 = narrow and 'Global instr. ✓' or 'Global instructions ✓'
    hl2 = 'Comment'
  end
  table.insert(lines, build_line(left2, right2, hl2))

  -- Session total + project instructions
  local left3 = ' Session: ' .. format_dollars(session_amount, max_int)
  local right3, hl3 = nil, nil
  if state.customization then
    if state.customization.project_instructions_error and state.customization.project_instructions_error ~= '' then
      right3 = narrow and 'Project instr. !' or 'Project instructions !'
      hl3 = 'BB7ErrorText'
    elseif state.customization.project_instructions then
      right3 = narrow and 'Project instr. ✓' or 'Project instructions ✓'
      hl3 = 'Comment'
    end
  end
  table.insert(lines, build_line(left3, right3, hl3))

  -- Context estimate line
  local left4
  if state.context_estimate and state.context_limit then
    local est_str = '~' .. utils.format_tokens(state.context_estimate)
    local limit_str = utils.format_tokens(state.context_limit)
    local pct = math.floor((state.context_estimate / state.context_limit) * 100)
    left4 = ' Context: ' .. est_str .. ' / ' .. limit_str .. ' (' .. pct .. '%)'
    -- Determine warning state
    local effective_limit = state.context_limit
    if state.max_completion and state.max_completion > 0 then
      effective_limit = state.context_limit - state.max_completion
    end
    if state.context_estimate > state.context_limit then
      -- Exceeds total context
      left4 = ' Context: ' .. est_str .. ' / ' .. limit_str .. ' \u{26a0} exceeds limit'
    elseif state.context_estimate > effective_limit then
      -- May truncate output
      left4 = ' Context: ' .. est_str .. ' / ' .. limit_str .. ' \u{26a0} may truncate'
    end
  else
    left4 = ' Context: -'
  end
  table.insert(lines, left4)

  vim.bo[state.buf].modifiable = true
  vim.api.nvim_buf_set_lines(state.buf, 0, -1, false, lines)
  vim.bo[state.buf].modifiable = false

  -- Apply highlights
  local ns = vim.api.nvim_create_namespace('bb7_provider')
  vim.api.nvim_buf_clear_namespace(state.buf, ns, 0, -1)

  -- Left labels (1 space prefix + label up to colon = 9 bytes)
  for i = 0, #lines - 1 do
    vim.api.nvim_buf_add_highlight(state.buf, ns, 'Comment', i, 0, 9)
  end

  -- Context line warning highlight (line index 3)
  if state.context_estimate and state.context_limit then
    local effective_limit = state.context_limit
    if state.max_completion and state.max_completion > 0 then
      effective_limit = state.context_limit - state.max_completion
    end
    if state.context_estimate > state.context_limit or state.context_estimate > effective_limit then
      -- Highlight the entire value portion in red (after the label)
      vim.api.nvim_buf_add_highlight(state.buf, ns, 'BB7ErrorText', 3, 9, -1)
    end
  end

  -- Right-side customization highlights
  for _, hl in ipairs(highlights) do
    vim.api.nvim_buf_add_highlight(state.buf, ns, hl[4], hl[1], hl[2], hl[3])
  end

end

-- Fetch balance from API
function M.refresh_balance()
  client.request({ action = 'get_balance' }, function(response, err)
    if err then
      -- Silently ignore balance errors (might not have permission)
      return
    end
    if response and response.total_credits then
      state.balance = {
        total_credits = response.total_credits,
        total_usage = response.total_usage or 0,
      }
      render()
    end
  end)
end

-- Fetch customization info (system override, instructions)
function M.refresh_customization()
  client.request({ action = 'get_customization_info' }, function(response, err)
    if err then
      return
    end
    if response then
      state.customization = {
        system_override = response.system_override,
        global_instructions = response.global_instructions,
        project_instructions = response.project_instructions,
        project_instructions_error = response.project_instructions_error,
      }
      render()
    end
  end)
end

-- Update with usage from a completed message
function M.update_usage(usage, model)
  if usage and usage.cost then
    state.session_cost = state.session_cost + usage.cost
    save_session_cost()
    append_usage_csv(model, usage)
    state.today_cost = read_today_cost()
    render()
    -- Also refresh balance to get updated account total
    M.refresh_balance()
  end
end

-- Set context estimate for display
function M.set_context_estimate(estimate, context_length, max_completion_tokens)
  state.context_estimate = estimate
  state.context_limit = context_length
  state.max_completion = max_completion_tokens
  render()
end

-- Set chat data (no longer used for last cost display)
function M.set_chat(_)
  -- Usage line is now rendered in the preview pane directly from msg.usage
end

-- Set project root for per-project session cost
function M.set_project_root(project_root)
  state.project_root = project_root
  -- Reload session cost and today's total now that we know the project
  load_session_cost()
  state.today_cost = read_today_cost()
  render()
end

-- Setup keymaps for this pane
function M.setup_keymaps(buf)
  local opts = { buffer = buf, nowait = true, silent = true }

  -- Refresh balance
  vim.keymap.set('n', 'r', M.refresh_balance, opts)

  -- Reset session cost
  vim.keymap.set('n', 'R', M.reset_session, opts)

  -- Edit instructions
  vim.keymap.set('n', '<C-g>', function()
    vim.cmd('BB7EditInstructions Global')
  end, opts)
  vim.keymap.set('n', '<C-p>', function()
    vim.cmd('BB7EditInstructions Project')
  end, opts)
end

-- Initialize the pane
function M.init(buf, win)
  state.buf = buf
  state.win = win
  state.customization = nil

  -- Load persisted session cost and today's total
  load_session_cost()
  state.today_cost = read_today_cost()

  vim.bo[buf].modifiable = false
  vim.bo[buf].buftype = 'nofile'

  -- Setup keymaps
  M.setup_keymaps(buf)

  render()
end

-- Cleanup
function M.cleanup()
  state.buf = nil
  state.win = nil
  -- Keep session_cost for next open - session persists across toggle
end

-- Reset session cost
function M.reset_session()
  state.session_cost = 0
  save_session_cost()
  render()
end

-- Get hints for this pane
function M.get_hints()
  return 'Refresh: r | Reset session: R | Edit Global: <C-g> | Project: <C-p>'
end

-- Inject mock data for screenshot mode (bypasses backend)
function M.set_mock_data(data)
  if data.balance then
    state.balance = data.balance
  end
  if data.today_cost ~= nil then
    state.today_cost = data.today_cost
  end
  if data.session_cost ~= nil then
    state.session_cost = data.session_cost
  end
  if data.customization then
    state.customization = data.customization
  end
  if data.context_estimate then
    state.context_estimate = data.context_estimate.estimate
    state.context_limit = data.context_estimate.context_length
    state.max_completion = data.context_estimate.max_completion_tokens
  end
  render()
end

return M
