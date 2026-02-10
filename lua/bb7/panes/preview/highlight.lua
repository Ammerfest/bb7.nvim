-- Highlight helpers for preview rendering
local M = {}

local shared = require('bb7.panes.preview.shared')

-- Get or create a bold variant of a highlight group
function M.get_bold_hl(base_hl)
  if not base_hl then return 'Bold' end

  local cache_key = base_hl .. '_Bold'
  if not shared.bold_hl_cache[cache_key] then
    -- Get the base highlight definition
    local base_def = vim.api.nvim_get_hl(0, { name = base_hl, link = false })
    -- Create bold variant
    local bold_def = vim.tbl_extend('force', base_def, { bold = true })
    vim.api.nvim_set_hl(0, cache_key, bold_def)
    shared.bold_hl_cache[cache_key] = true
  end
  return cache_key
end

-- Get or create an italic variant of a highlight group
function M.get_italic_hl(base_hl)
  if not base_hl then return 'Italic' end

  local cache_key = base_hl .. '_Italic'
  if not shared.bold_hl_cache[cache_key] then
    local base_def = vim.api.nvim_get_hl(0, { name = base_hl, link = false })
    local italic_def = vim.tbl_extend('force', base_def, { italic = true })
    vim.api.nvim_set_hl(0, cache_key, italic_def)
    shared.bold_hl_cache[cache_key] = true
  end
  return cache_key
end

-- Get or create an underline variant of a highlight group
function M.get_underline_hl(base_hl)
  if not base_hl then return 'Underlined' end

  local cache_key = base_hl .. '_Underline'
  if not shared.bold_hl_cache[cache_key] then
    local base_def = vim.api.nvim_get_hl(0, { name = base_hl, link = false })
    local underline_def = vim.tbl_extend('force', base_def, { underline = true })
    vim.api.nvim_set_hl(0, cache_key, underline_def)
    shared.bold_hl_cache[cache_key] = true
  end
  return cache_key
end

-- Get or create a highlight group that ensures bg is set (defaults to Normal's bg)
-- This prevents virtual text from inheriting line_hl_group's background
function M.get_hl_with_bg(hl_name)
  if not hl_name then return 'Normal' end

  -- Check if highlight already has a bg
  local hl = vim.api.nvim_get_hl(0, { name = hl_name, link = false })
  if hl.bg then
    return hl_name  -- Already has bg, use as-is
  end

  -- Create derived highlight with Normal's bg
  local derived_name = hl_name .. '_WithBg'
  if not shared.hl_cache[derived_name] then
    local normal_hl = vim.api.nvim_get_hl(0, { name = 'Normal', link = false })
    local new_hl = vim.tbl_extend('force', hl, { bg = normal_hl.bg })
    vim.api.nvim_set_hl(0, derived_name, new_hl)
    shared.hl_cache[derived_name] = true
  end
  return derived_name
end

-- Get or create a highlight group with only a background
function M.get_hl_bg_only(hl_name)
  if not hl_name then return 'Normal' end

  local hl = vim.api.nvim_get_hl(0, { name = hl_name, link = false })
  local bg = hl.bg
  if not bg then
    local normal_hl = vim.api.nvim_get_hl(0, { name = 'Normal', link = false })
    bg = normal_hl.bg
  end

  local derived_name = hl_name .. '_BgOnly'
  if not shared.hl_cache[derived_name] then
    vim.api.nvim_set_hl(0, derived_name, { bg = bg })
    shared.hl_cache[derived_name] = true
  end
  return derived_name
end

-- Get or create a highlight group with only a foreground
function M.get_hl_fg_only(hl_name)
  if not hl_name then return 'Normal' end

  local hl = vim.api.nvim_get_hl(0, { name = hl_name, link = false })
  local fg = hl.fg
  if not fg then
    local normal_hl = vim.api.nvim_get_hl(0, { name = 'Normal', link = false })
    fg = normal_hl.fg
  end

  local derived_name = hl_name .. '_FgOnly'
  if not shared.hl_cache[derived_name] then
    local new_hl = vim.tbl_extend('force', hl, { fg = fg, bg = nil })
    new_hl.bg = nil
    vim.api.nvim_set_hl(0, derived_name, new_hl)
    shared.hl_cache[derived_name] = true
  end
  return derived_name
end

-- Clear highlight cache (call on colorscheme change)
function M.clear_hl_cache()
  shared.hl_cache = {}
  shared.bold_hl_cache = {}
end

return M
