-- Lua frontend logging module
-- Writes to ~/.bb7/logs/ when BB7_DEBUG=1 or ~/.bb7/debug exists
local M = {}

local state = {
  enabled = false,
  file = nil,
  file_path = nil,
  enabled_by = nil,  -- 'env' or 'file'
}

-- Format timestamp for log entries
local function format_timestamp()
  return os.date('%H:%M:%S') .. '.' .. string.format('%03d', (vim.loop.now() % 1000))
end

-- Write a log entry to file
local function write_log(level, msg)
  if not state.enabled or not state.file then
    return
  end

  local line = string.format('[%s] %s [nvim]: %s\n', format_timestamp(), level, msg)
  state.file:write(line)
  state.file:flush()
end

-- Initialize logging (called once on first use)
local function init()
  if state.file then
    return  -- Already initialized
  end

  -- Check if debug mode is enabled via env var or config file
  local debug_env = vim.env.BB7_DEBUG
  local home = vim.env.HOME or vim.fn.expand('~')
  local debug_file = home .. '/.bb7/debug'
  local debug_file_exists = vim.fn.filereadable(debug_file) == 1

  if debug_env ~= '1' and not debug_file_exists then
    state.enabled = false
    return
  end

  state.enabled = true
  state.enabled_by = debug_env == '1' and 'env' or 'file'

  -- Create logs directory
  local logs_dir = home .. '/.bb7/logs'
  vim.fn.mkdir(logs_dir, 'p')

  -- Create log file with timestamp
  local file_timestamp = os.date('%Y-%m-%d_%H-%M-%S')
  state.file_path = logs_dir .. '/bb7-nvim-' .. file_timestamp .. '.log'

  state.file = io.open(state.file_path, 'a')
  if not state.file then
    vim.notify('BB-7: Failed to open log file: ' .. state.file_path, vim.log.levels.ERROR)
    state.enabled = false
    return
  end

  -- Log startup with how debugging was enabled
  local reason = state.enabled_by == 'env' and 'BB7_DEBUG=1' or '~/.bb7/debug exists'
  write_log('INFO', 'Logging started (' .. reason .. ')')
  write_log('INFO', 'Log file: ' .. state.file_path)
  -- Defer notification so it appears after all init is done (otherwise gets cleared)
  local log_path = state.file_path
  vim.defer_fn(function()
    vim.notify('BB-7: Logging to ' .. log_path, vim.log.levels.INFO)
  end, 100)  -- 100ms delay to let startup complete
end

-- Log and notify at INFO level
function M.info(msg)
  init()
  write_log('INFO', msg)
  vim.notify('BB-7: ' .. msg, vim.log.levels.INFO)
end

-- Log and notify at WARN level
function M.warn(msg)
  init()
  write_log('WARN', msg)
  vim.notify('BB-7: ' .. msg, vim.log.levels.WARN)
end

-- Log and notify at ERROR level
function M.error(msg)
  init()
  write_log('ERROR', msg)
  vim.notify('BB-7: ' .. msg, vim.log.levels.ERROR)
end

-- Log at DEBUG level (file only, no notification)
function M.debug(msg)
  init()
  write_log('DEBUG', msg)
end

-- Log without BB7 prefix (for raw messages like stderr)
function M.raw(level, msg)
  init()
  write_log(level, msg)
  vim.notify(msg, vim.log.levels[level] or vim.log.levels.INFO)
end

-- Check if logging is enabled
function M.is_enabled()
  init()
  return state.enabled
end

-- Capture vim.notify messages to log file (file only, no vim.notify call)
-- Used by the notify override to avoid recursion
function M.capture_notify(msg, level)
  init()
  if not state.enabled then return end

  local level_name = 'INFO'
  if level == vim.log.levels.ERROR then
    level_name = 'ERROR'
  elseif level == vim.log.levels.WARN then
    level_name = 'WARN'
  elseif level == vim.log.levels.DEBUG then
    level_name = 'DEBUG'
  end

  write_log(level_name, '[vim.notify] ' .. tostring(msg))
end

-- Set up vim.notify override to capture errors/warnings to log file
-- Call this once at startup; it's a no-op if logging is disabled
function M.setup_notify_capture()
  init()
  if not state.enabled then return end

  local original_notify = vim.notify
  vim.notify = function(msg, level, opts)
    -- Skip our own BB7 messages (already logged by log.info/warn/error)
    local msg_str = tostring(msg)
    if not msg_str:match('^BB7:') then
      -- Capture errors and warnings to log file
      if level == vim.log.levels.ERROR or level == vim.log.levels.WARN then
        M.capture_notify(msg, level)
      end
    end
    -- Always call original
    return original_notify(msg, level, opts)
  end

  write_log('INFO', 'vim.notify capture enabled')
end

-- Get the log file path (useful for telling users where to look)
function M.get_path()
  init()
  return state.file_path
end

-- Close log file (call on cleanup)
function M.close()
  if state.file then
    state.file:close()
    state.file = nil
  end
end

return M
