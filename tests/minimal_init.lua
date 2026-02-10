-- Minimal init for running tests
-- Usage: nvim --headless -u tests/minimal_init.lua -c "PlenaryBustedDirectory tests/ {minimal_init = 'tests/minimal_init.lua'}"

-- Add plugin to runtimepath
local plugin_root = vim.fn.fnamemodify(debug.getinfo(1, 'S').source:sub(2), ':h:h')
vim.opt.runtimepath:prepend(plugin_root)

-- Add plenary to runtimepath (assumes it's installed)
local plenary_path = vim.fn.stdpath('data') .. '/lazy/plenary.nvim'
if vim.fn.isdirectory(plenary_path) == 1 then
  vim.opt.runtimepath:prepend(plenary_path)
else
  -- Try other common locations
  local paths = {
    vim.fn.expand('~/.local/share/nvim/site/pack/*/start/plenary.nvim'),
    vim.fn.expand('~/.local/share/nvim/lazy/plenary.nvim'),
  }
  for _, path in ipairs(paths) do
    local expanded = vim.fn.glob(path)
    if expanded ~= '' then
      vim.opt.runtimepath:prepend(expanded)
      break
    end
  end
end

-- Disable swap files for tests
vim.opt.swapfile = false

-- Load plenary
local ok, _ = pcall(require, 'plenary')
if not ok then
  print('WARNING: plenary.nvim not found, some tests may fail')
end
