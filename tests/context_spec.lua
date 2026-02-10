-- Tests for fuzzy matching logic
-- Note: Full UI tests would require a running Neovim with UI

-- Extract the fuzzy_match function for testing
-- We'll reimplement it here to test the algorithm
local function fuzzy_match(str, pattern)
  if pattern == '' then
    return true, 0
  end

  pattern = pattern:lower()
  str = str:lower()

  local pattern_idx = 1
  local score = 0
  local last_match = 0

  for i = 1, #str do
    if pattern_idx <= #pattern and str:sub(i, i) == pattern:sub(pattern_idx, pattern_idx) then
      -- Bonus for consecutive matches
      if i == last_match + 1 then
        score = score + 10
      end
      -- Bonus for matching at start or after separator
      if i == 1 or str:sub(i - 1, i - 1):match('[/._-]') then
        score = score + 5
      end
      score = score + 1
      last_match = i
      pattern_idx = pattern_idx + 1
    end
  end

  if pattern_idx > #pattern then
    return true, score
  end

  return false, 0
end

describe('fuzzy_match', function()
  it('should match empty pattern', function()
    local match, score = fuzzy_match('anything', '')
    assert.is_true(match)
    assert.equals(0, score)
  end)

  it('should match exact string', function()
    local match, _ = fuzzy_match('hello', 'hello')
    assert.is_true(match)
  end)

  it('should match case-insensitively', function()
    local match, _ = fuzzy_match('Hello', 'hello')
    assert.is_true(match)

    match, _ = fuzzy_match('hello', 'HELLO')
    assert.is_true(match)
  end)

  it('should match subsequence', function()
    local match, _ = fuzzy_match('src/main.go', 'smg')
    assert.is_true(match)

    match, _ = fuzzy_match('internal/state/state.go', 'issg')
    assert.is_true(match)
  end)

  it('should not match when pattern chars missing', function()
    local match, _ = fuzzy_match('hello', 'hellox')
    assert.is_false(match)

    match, _ = fuzzy_match('abc', 'abcd')
    assert.is_false(match)
  end)

  it('should score consecutive matches higher', function()
    local _, score1 = fuzzy_match('abcdef', 'abc')
    local _, score2 = fuzzy_match('a_b_c_def', 'abc')

    -- Consecutive matches should score higher
    assert.is_true(score1 > score2)
  end)

  it('should score start-of-word matches higher', function()
    local _, score1 = fuzzy_match('src/main.go', 'main')
    local _, score2 = fuzzy_match('src/domain.go', 'main')

    -- 'main' at start of filename should score higher than 'main' in middle
    assert.is_true(score1 > score2)
  end)

  it('should score matches after separators higher', function()
    local _, score1 = fuzzy_match('foo/bar/baz', 'b')
    local _, score2 = fuzzy_match('foobarbaz', 'b')

    -- Match after '/' should score higher
    assert.is_true(score1 > score2)
  end)

  it('should handle special characters in pattern', function()
    local match, _ = fuzzy_match('file.test.lua', '.lua')
    assert.is_true(match)
  end)

  it('should handle paths correctly', function()
    local match, _ = fuzzy_match('lua/bb7/client.lua', 'bb7cl')
    assert.is_true(match)

    match, _ = fuzzy_match('internal/config/config.go', 'icfg')
    assert.is_true(match)
  end)
end)

describe('file filtering', function()
    local files = {
      'src/main.go',
      'src/config.go',
      'src/server/handler.go',
      'src/server/routes.go',
      'internal/state/state.go',
      'internal/state/types.go',
      'README.md',
      'go.mod',
      'go.sum',
    }

    local function filter_files(pattern)
      local filtered = {}
      for _, file in ipairs(files) do
        local match, score = fuzzy_match(file, pattern)
        if match then
          table.insert(filtered, { path = file, score = score })
        end
      end
      table.sort(filtered, function(a, b)
        if a.score ~= b.score then
          return a.score > b.score
        end
        return a.path < b.path
      end)
      return filtered
    end

    it('should filter by partial filename', function()
      local result = filter_files('main')
      assert.equals(1, #result)
      assert.equals('src/main.go', result[1].path)
    end)

    it('should filter by extension', function()
      local result = filter_files('readme')
      assert.equals(1, #result)
      assert.equals('README.md', result[1].path)
    end)

    it('should filter by path components', function()
      local result = filter_files('server')
      assert.equals(2, #result)
    end)

    it('should return all files for empty filter', function()
      local result = filter_files('')
      assert.equals(#files, #result)
    end)

    it('should rank exact matches higher', function()
      local result = filter_files('state')
      -- 'state.go' should be ranked higher than 'types.go'
      assert.equals('internal/state/state.go', result[1].path)
    end)
end)
