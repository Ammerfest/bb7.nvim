-- Syntax highlight helpers for preview rendering
local M = {}

-- Map common language aliases to treesitter parser names
local lang_aliases = {
  csharp = 'c_sharp',
  cs = 'c_sharp',
  ['c++'] = 'cpp',
  js = 'javascript',
  ts = 'typescript',
  py = 'python',
  rb = 'ruby',
  rs = 'rust',
  sh = 'bash',
  shell = 'bash',
  zsh = 'bash',
  yml = 'yaml',
}

-- Map common language aliases to filetypes (for syntax fallback)
local lang_filetypes = {
  csharp = 'cs',
  cs = 'cs',
  ['c++'] = 'cpp',
  js = 'javascript',
  ts = 'typescript',
  py = 'python',
  rb = 'ruby',
  rs = 'rust',
  sh = 'sh',
  shell = 'sh',
  zsh = 'sh',
  bash = 'sh',
  yml = 'yaml',
}

-- Get regex syntax highlights for a code string (fallback when treesitter unavailable)
-- Returns array of { row, col_start, col_end, hl_group }
function M.get_syntax_highlights(code, lang)
  if not lang or lang == '' then
    return {}
  end

  local filetype = lang_filetypes[lang] or lang
  if not filetype or filetype == '' then
    return {}
  end

  local buf = vim.api.nvim_create_buf(false, true)
  if not buf or buf == 0 then
    return {}
  end

  local highlights = {}
  local lines = vim.split(code, '\n', { plain = true })

  vim.bo[buf].buftype = 'nofile'
  vim.bo[buf].swapfile = false
  vim.bo[buf].modifiable = true
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].filetype = filetype
  vim.bo[buf].modifiable = false

  vim.api.nvim_buf_call(buf, function()
    vim.cmd('syntax enable')
    local line_count = vim.api.nvim_buf_line_count(buf)
    for lnum = 1, line_count do
      local line = vim.api.nvim_buf_get_lines(buf, lnum - 1, lnum, false)[1] or ''
      local line_len = #line
      local col = 1
      while col <= line_len do
        local id = vim.fn.synID(lnum, col, 1)
        local trans = vim.fn.synIDtrans(id)
        local name = vim.fn.synIDattr(trans, 'name')
        if name ~= '' then
          local start_col = col
          col = col + 1
          while col <= line_len do
            local id2 = vim.fn.synID(lnum, col, 1)
            local trans2 = vim.fn.synIDtrans(id2)
            local name2 = vim.fn.synIDattr(trans2, 'name')
            if name2 ~= name then
              break
            end
            col = col + 1
          end
          table.insert(highlights, { lnum - 1, start_col - 1, col - 1, name })
        else
          col = col + 1
        end
      end
    end
  end)

  vim.api.nvim_buf_delete(buf, { force = true })
  return highlights
end

-- Get treesitter syntax highlights for a code string
-- Returns array of { row, col_start, col_end, hl_group }
-- row is 0-indexed relative to the code block
function M.get_treesitter_highlights(code, lang)
  if not lang or lang == '' then
    return {}
  end

  -- Map language alias to treesitter name
  local ts_lang = lang_aliases[lang] or lang

  -- Try to get parser for language
  local ok, parser = pcall(vim.treesitter.get_string_parser, code, ts_lang)
  if not ok or not parser then
    return {}
  end

  -- Parse the code
  local trees = parser:parse()
  if not trees or #trees == 0 then
    return {}
  end

  -- Get highlights query
  local query_ok, query = pcall(vim.treesitter.query.get, ts_lang, 'highlights')
  if not query_ok or not query then
    return {}
  end

  local highlights = {}
  local root = trees[1]:root()

  -- Iterate captures
  for id, node, _ in query:iter_captures(root, code) do
    local name = query.captures[id]
    local sr, sc, er, ec = node:range()

    -- Handle multi-line nodes by creating highlight for each line
    if sr == er then
      -- Single line
      table.insert(highlights, { sr, sc, ec, '@' .. name })
    else
      -- Multi-line: highlight each line separately
      local code_lines = vim.split(code, '\n', { plain = true })
      for row = sr, er do
        local start_col = (row == sr) and sc or 0
        local end_col = (row == er) and ec or #(code_lines[row + 1] or '')
        if end_col > start_col then
          table.insert(highlights, { row, start_col, end_col, '@' .. name })
        end
      end
    end
  end

  return highlights
end

-- Get syntax highlights for a code string (treesitter, then syntax fallback)
function M.get_code_highlights(code, lang)
  local highlights = M.get_treesitter_highlights(code, lang)
  if #highlights > 0 then
    return highlights
  end
  return M.get_syntax_highlights(code, lang)
end

return M
