-- Status bar indicator module for BB-7
-- Provides statusline API to show streaming/unread state
local M = {}

-- State machine: idle -> streaming -> unread -> idle
-- idle: nothing happening
-- streaming: LLM is generating a response
-- unread: response is ready but user hasn't opened full UI
local state = {
  status = nil, -- nil (idle), 'streaming', or 'unread'
}

-- Configuration (set via setup)
local config = {
  streaming = { enabled = true, symbol = '○', highlight = 'DiagnosticWarn' },
  unread    = { enabled = true, symbol = '●', highlight = 'DiagnosticInfo' },
}

-- Set configuration
function M.set_config(cfg)
  if cfg then
    config = vim.tbl_deep_extend('force', config, cfg)
  end
end

-- Set status (called by split.lua and ui.lua)
function M.set(new_status)
  if new_status == 'idle' then
    state.status = nil
  else
    state.status = new_status
  end
  vim.schedule(function()
    vim.cmd('redrawstatus')
  end)
end

-- Get raw status for programmatic use
-- Returns 'streaming', 'unread', or nil
function M.raw_status()
  return state.status
end

-- Get the symbol for the current status (or '')
function M.status()
  if not state.status then
    return ''
  end
  local cfg = config[state.status]
  if not cfg or not cfg.enabled then
    return ''
  end
  return cfg.symbol
end

-- Fg-only highlight groups (defined in init.lua setup_highlights)
local STATUS_HL = {
  streaming = 'BB7StatusStreaming',
  unread    = 'BB7StatusUnread',
}

-- Get the highlight group for the current status (or nil)
-- Returns a fg-only group so bg is inherited from the statusline section
function M.status_hl()
  if not state.status then
    return nil
  end
  local cfg = config[state.status]
  if not cfg or not cfg.enabled then
    return nil
  end
  return STATUS_HL[state.status]
end

return M
