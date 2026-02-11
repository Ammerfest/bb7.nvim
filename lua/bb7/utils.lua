-- Shared utility functions for BB7
local M = {}

-- Format token count for display
-- Returns "0" for 0/nil, "999" for under 1000, "1.5k" for 1000+
function M.format_tokens(count)
  if not count or count == 0 then return '0' end
  if count >= 1000 then
    return string.format('%.1fk', count / 1000)
  end
  return tostring(count)
end

-- Normalize line endings to \n
function M.normalize_line_endings(content)
  if not content then return '' end
  return content:gsub('\r\n', '\n'):gsub('\r', '\n')
end

-- Compare two content strings with line ending normalization
-- Returns true if they are the same (ignoring line ending differences and trailing newlines)
function M.contents_equal(a, b)
  local normalized_a = M.normalize_line_endings(a)
  local normalized_b = M.normalize_line_endings(b)
  -- Strip trailing newlines (context uses readfile+concat which omits trailing newline,
  -- but io.open read('*a') preserves it)
  normalized_a = normalized_a:gsub('\n+$', '')
  normalized_b = normalized_b:gsub('\n+$', '')
  return normalized_a == normalized_b
end

-- Format ISO 8601 timestamp for display
-- Input: "2024-01-15T14:30:00Z" or similar
-- Output: "2024-01-15 14:30"
function M.format_time(timestamp)
  if not timestamp then return '' end
  -- Handle ISO 8601 format from Go
  local pattern = '(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)'
  local year, month, day, hour, min, _ = timestamp:match(pattern)
  if year then
    return string.format('%s-%s-%s %s:%s', year, month, day, hour, min)
  end
  return timestamp
end

return M
