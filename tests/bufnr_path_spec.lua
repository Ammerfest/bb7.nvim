-- Tests for write_to_destination's buffer write behavior
--
-- write_to_destination uses two strategies:
-- 1. Buffer exists: nvim_buf_set_lines + :write (atomic mtime, preserves undo)
-- 2. No buffer: io.open (safe, no mtime to desync)
--
-- The old approach (io.open + edit!) is kept as regression tests for the
-- no-buffer fallback path and for bufnr() path resolution.

-- Simulate the NEW write_to_destination (buffer-native write)
local function simulate_native_write(lookup_path, content)
  local bufnr = vim.fn.bufnr(lookup_path)
  if bufnr ~= -1 then
    local lines = vim.split(content, '\n', { plain = true })
    if #lines > 0 and lines[#lines] == '' then
      table.remove(lines)
    end
    vim.api.nvim_buf_set_lines(bufnr, 0, -1, false, lines)
    vim.api.nvim_buf_call(bufnr, function()
      vim.cmd('write!')
    end)
    return true
  end
  return false
end

-- Simulate the OLD write_to_destination (io.open + edit!) for regression tests
local function simulate_io_write_and_reload(lookup_path, content)
  local f = io.open(lookup_path, 'w')
  if not f then
    error('Cannot write to ' .. lookup_path)
  end
  f:write(content)
  f:close()

  local bufnr = vim.fn.bufnr(lookup_path)
  if bufnr ~= -1 then
    vim.api.nvim_buf_call(bufnr, function()
      vim.cmd('edit!')
    end)
  end

  return bufnr ~= -1
end

describe('write_to_destination', function()
  local tmpdir

  before_each(function()
    tmpdir = vim.fn.tempname()
    vim.fn.mkdir(tmpdir, 'p')
  end)

  after_each(function()
    for _, buf in ipairs(vim.api.nvim_list_bufs()) do
      if vim.api.nvim_buf_is_valid(buf) then
        pcall(vim.api.nvim_buf_delete, buf, { force = true })
      end
    end
    vim.fn.delete(tmpdir, 'rf')
  end)

  describe('buffer-native write (buffer exists)', function()
    it('writes content through Neovim and updates disk', function()
      local file_path = tmpdir .. '/file.lua'
      local f = io.open(file_path, 'w')
      f:write('original\n')
      f:close()
      vim.cmd('edit ' .. vim.fn.fnameescape(file_path))

      simulate_native_write(file_path, 'modified by llm\n')

      -- Buffer has new content
      local buf = vim.fn.bufnr(file_path)
      local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
      assert.equals('modified by llm', lines[1])

      -- Disk has new content
      f = io.open(file_path, 'r')
      local disk = f:read('*a')
      f:close()
      assert.equals('modified by llm\n', disk)

      -- Buffer is not modified (just written)
      assert.is_false(vim.bo[buf].modified)
    end)

    it('preserves undo history after apply', function()
      local file_path = tmpdir .. '/file.lua'
      local f = io.open(file_path, 'w')
      f:write('original\n')
      f:close()
      vim.cmd('edit ' .. vim.fn.fnameescape(file_path))

      simulate_native_write(file_path, 'modified by llm\n')

      -- Undo should restore original content
      local buf = vim.fn.bufnr(file_path)
      vim.api.nvim_buf_call(buf, function()
        vim.cmd('undo')
      end)

      local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
      assert.equals('original', lines[1])
    end)

    it('allows clean :w after user edits applied content', function()
      local file_path = tmpdir .. '/file.lua'
      local f = io.open(file_path, 'w')
      f:write('original\n')
      f:close()
      vim.cmd('edit ' .. vim.fn.fnameescape(file_path))

      -- BB-7 applies LLM output
      simulate_native_write(file_path, 'modified by llm\n')

      -- User edits the buffer
      local buf = vim.fn.bufnr(file_path)
      vim.api.nvim_buf_set_lines(buf, 0, -1, false, { 'user edit on top' })
      assert.is_true(vim.bo[buf].modified)

      -- :write should succeed without W12 warning
      vim.api.nvim_buf_call(buf, function()
        local ok, err = pcall(vim.cmd, 'write')
        assert.is_true(ok, 'write should succeed: ' .. tostring(err))
      end)

      f = io.open(file_path, 'r')
      local disk = f:read('*a')
      f:close()
      assert.equals('user edit on top\n', disk)
    end)

    it('handles content without trailing newline', function()
      local file_path = tmpdir .. '/file.lua'
      local f = io.open(file_path, 'w')
      f:write('original\n')
      f:close()
      vim.cmd('edit ' .. vim.fn.fnameescape(file_path))

      -- Content without trailing newline (Neovim adds one on write)
      simulate_native_write(file_path, 'no trailing newline')

      local buf = vim.fn.bufnr(file_path)
      local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
      assert.equals(1, #lines)
      assert.equals('no trailing newline', lines[1])
    end)
  end)

  describe('io.open fallback (no buffer)', function()
    it('writes to disk when no buffer is open', function()
      local file_path = tmpdir .. '/new_file.lua'

      -- No buffer open — simulate the fallback path
      local f = io.open(file_path, 'w')
      f:write('new content\n')
      f:close()

      -- Verify disk
      f = io.open(file_path, 'r')
      local disk = f:read('*a')
      f:close()
      assert.equals('new content\n', disk)

      -- No buffer should exist
      assert.equals(-1, vim.fn.bufnr(file_path))
    end)
  end)

  describe('bufnr path resolution (regression)', function()
    it('finds buffer via resolved symlink path', function()
      local real_path = tmpdir .. '/file.lua'
      local link_dir = tmpdir .. '/link'
      vim.fn.mkdir(tmpdir .. '/sub', 'p')
      vim.fn.system({ 'ln', '-s', tmpdir, link_dir })
      local link_path = link_dir .. '/file.lua'

      local f = io.open(real_path, 'w')
      f:write('original\n')
      f:close()
      vim.cmd('edit ' .. vim.fn.fnameescape(link_path))

      -- Lookup via real path should find the buffer
      assert.is_not.equals(-1, vim.fn.bufnr(real_path))
    end)

    it('finds buffer via symlink when opened with real path', function()
      local real_path = tmpdir .. '/file.lua'
      local link_dir = tmpdir .. '/link'
      vim.fn.system({ 'ln', '-s', tmpdir, link_dir })
      local link_path = link_dir .. '/file.lua'

      local f = io.open(real_path, 'w')
      f:write('original\n')
      f:close()
      vim.cmd('edit ' .. vim.fn.fnameescape(real_path))

      -- Lookup via symlink path should find the buffer
      assert.is_not.equals(-1, vim.fn.bufnr(link_path))
    end)

    it('old io.open+edit! path still updates mtime correctly', function()
      local file_path = tmpdir .. '/file.lua'
      local f = io.open(file_path, 'w')
      f:write('original\n')
      f:close()
      vim.cmd('edit ' .. vim.fn.fnameescape(file_path))

      simulate_io_write_and_reload(file_path, 'modified\n')

      -- checktime should not detect a change (mtime matches)
      local file_changed = false
      local augroup = vim.api.nvim_create_augroup('test_mtime', { clear = true })
      vim.api.nvim_create_autocmd('FileChangedShell', {
        group = augroup,
        pattern = '*',
        callback = function()
          file_changed = true
          return true
        end,
      })
      vim.cmd('checktime')
      vim.api.nvim_del_augroup_by_id(augroup)

      assert.is_false(file_changed, 'edit! should update mtime record')
    end)
  end)
end)
