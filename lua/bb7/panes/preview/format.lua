-- Formatting helpers for preview rendering
local M = {}

local shared = require('bb7.panes.preview.shared')

-- Format duration in mm:ss
function M.format_duration(seconds)
  if not seconds then return '00:00' end
  local mins = math.floor(seconds / 60)
  local secs = seconds % 60
  return string.format('%02d:%02d', mins, secs)
end

-- Process a line for text style markers (**bold**, *italic*, __underline__)
-- Returns: { display = "processed text", bold_regions = {}, italic_regions = {}, underline_regions = {} }
-- Positions are byte offsets in the display string
function M.process_bold_markers(text)
  local display = ''
  local bold_regions = {}
  local italic_regions = {}
  local underline_regions = {}
  local pos = 1
  local text_len = #text

  while pos <= text_len do
    -- Find the earliest marker of any type
    local bold_start = text:find('%*%*', pos)
    local italic_start = text:find('%*', pos)
    local underline_start = text:find('__', pos)

    -- Skip italic_start if it's actually the start of bold (**)
    if italic_start and bold_start and italic_start == bold_start then
      italic_start = text:find('%*', bold_start + 2)
      -- Also skip if this new position is part of closing **
      if italic_start then
        local next_char = text:sub(italic_start + 1, italic_start + 1)
        if next_char == '*' then
          italic_start = nil
        end
      end
    end

    -- Find earliest marker
    local earliest = nil
    local marker_type = nil

    if bold_start and (not earliest or bold_start < earliest) then
      earliest = bold_start
      marker_type = 'bold'
    end
    if italic_start and (not earliest or italic_start < earliest) then
      earliest = italic_start
      marker_type = 'italic'
    end
    if underline_start and (not earliest or underline_start < earliest) then
      earliest = underline_start
      marker_type = 'underline'
    end

    if not earliest then
      -- No more markers, append rest
      display = display .. text:sub(pos)
      break
    end

    -- Append text before marker
    display = display .. text:sub(pos, earliest - 1)

    if marker_type == 'bold' then
      -- Look for closing **
      local end_marker = text:find('%*%*', earliest + 2)
      if not end_marker then
        display = display .. text:sub(earliest)
        break
      end
      local region_start = #display
      local styled_text = text:sub(earliest + 2, end_marker - 1)
      display = display .. styled_text
      local region_end = #display
      if region_end > region_start then
        table.insert(bold_regions, { region_start, region_end })
      end
      pos = end_marker + 2

    elseif marker_type == 'italic' then
      -- Look for closing * (but not **)
      local search_pos = earliest + 1
      local end_marker = nil
      while true do
        end_marker = text:find('%*', search_pos)
        if not end_marker then break end
        -- Make sure it's not **
        if text:sub(end_marker, end_marker + 1) ~= '**' then
          break
        end
        search_pos = end_marker + 2
      end
      if not end_marker then
        display = display .. text:sub(earliest)
        break
      end
      local region_start = #display
      local styled_text = text:sub(earliest + 1, end_marker - 1)
      display = display .. styled_text
      local region_end = #display
      if region_end > region_start then
        table.insert(italic_regions, { region_start, region_end })
      end
      pos = end_marker + 1

    elseif marker_type == 'underline' then
      -- Look for closing __
      local end_marker = text:find('__', earliest + 2)
      if not end_marker then
        display = display .. text:sub(earliest)
        break
      end
      local region_start = #display
      local styled_text = text:sub(earliest + 2, end_marker - 1)
      display = display .. styled_text
      local region_end = #display
      if region_end > region_start then
        table.insert(underline_regions, { region_start, region_end })
      end
      pos = end_marker + 2
    end
  end

  return {
    display = display,
    bold_regions = bold_regions,
    italic_regions = italic_regions,
    underline_regions = underline_regions,
  }
end

-- Parse text content into segments (text vs code blocks)
-- Returns array of { type = 'text'|'code', content = string, lang = string|nil }
function M.parse_text_segments(content)
  local segments = {}
  local pos = 1
  local len = #content

  while pos <= len do
    -- Look for code block start
    local block_start, block_end, lang = content:find('^```(%w*)\n?', pos)
    if block_start == pos then
      -- Found code block start
      -- Find the closing ```
      local close_start, close_end = content:find('\n```', block_end + 1)
      if close_start then
        -- Extract code content (between opening ``` and closing ```)
        local code = content:sub(block_end + 1, close_start - 1)
        -- Remove trailing newline if present
        code = code:gsub('\n$', '')
        table.insert(segments, { type = 'code', content = code, lang = lang ~= '' and lang or nil })
        pos = close_end + 1
        -- Skip newline after closing ```
        if content:sub(pos, pos) == '\n' then
          pos = pos + 1
        end
      else
        -- No closing ```, treat rest as text
        table.insert(segments, { type = 'text', content = content:sub(pos) })
        break
      end
    else
      -- Look for next code block or end of content
      local next_block = content:find('\n```', pos)
      if not next_block then
        next_block = content:find('^```', pos)
      end
      if next_block then
        -- Text before next code block
        local text = content:sub(pos, next_block - 1)
        if text ~= '' then
          table.insert(segments, { type = 'text', content = text })
        end
        pos = next_block
        -- Handle newline before ```
        if content:sub(pos, pos) == '\n' then
          pos = pos + 1
        end
      else
        -- No more code blocks, rest is text
        local text = content:sub(pos)
        if text ~= '' then
          table.insert(segments, { type = 'text', content = text })
        end
        break
      end
    end
  end

  return segments
end

-- Wrap text to fit within a given width
-- Returns a list of lines, each fitting within max_width
-- Respects word boundaries when possible
function M.wrap_text(text, max_width)
  if max_width <= 0 then
    return { text }
  end

  local result = {}
  local text_width = vim.fn.strwidth(text)

  if text_width <= max_width then
    return { text }
  end

  -- Wrap the text
  local current_line = ''
  local current_width = 0

  -- Split by words (keeping spaces with words)
  for word in text:gmatch('%S+%s*') do
    local word_width = vim.fn.strwidth(word)

    if current_width + word_width <= max_width then
      current_line = current_line .. word
      current_width = current_width + word_width
    else
      -- Word doesn't fit on current line
      if current_line ~= '' then
        -- Trim trailing space and save current line
        -- Note: wrap gsub in parens to discard the count return value
        table.insert(result, (current_line:gsub('%s+$', '')))
      end

      -- If word itself is longer than max_width, break it
      if word_width > max_width then
        local remaining = word
        while vim.fn.strwidth(remaining) > max_width do
          -- Find how many chars fit
          local fit_len = 0
          local fit_width = 0
          for i = 1, #remaining do
            local char = remaining:sub(i, i)
            local char_width = vim.fn.strwidth(char)
            if fit_width + char_width > max_width then
              break
            end
            fit_len = i
            fit_width = fit_width + char_width
          end
          if fit_len == 0 then fit_len = 1 end  -- At least one char
          table.insert(result, remaining:sub(1, fit_len))
          remaining = remaining:sub(fit_len + 1)
        end
        current_line = remaining
        current_width = vim.fn.strwidth(remaining)
      else
        current_line = word
        current_width = word_width
      end
    end
  end

  -- Don't forget the last line
  if current_line ~= '' then
    table.insert(result, (current_line:gsub('%s+$', '')))
  end

  return result
end

-- Get available text width for content with a bar
-- Accounts for bar char, padding, and optional indent
function M.get_text_width(indent)
  indent = indent or 0
  local win_width = 80  -- Default fallback
  if shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
    win_width = vim.api.nvim_win_get_width(shared.state.win)
  end

  local bar_width = vim.fn.strwidth(shared.config.bar_char)
  -- Available = window - bar - padding - indent
  return win_width - bar_width - shared.config.style.bar_padding - indent
end

function M.center_text(text, width)
  local text_width = vim.fn.strwidth(text)
  if text_width >= width then
    return text
  end
  local left_pad = math.floor((width - text_width) / 2)
  return string.rep(' ', left_pad) .. text
end

-- Helper: add a line with text + styling
-- Bar + padding are virtual text (not yankable, cursor can't go there)
-- Buffer contains only the actual text content
-- show_bar: if true, shows colored bar; if false, shows invisible spacer for alignment
-- icon: optional icon character to show in padding area
-- icon_fg: foreground color for the icon (number or string, nil = use bar's fg)
-- indent: optional string to show BEFORE the bar (with Normal bg)
-- code_block: if true, use special rendering (bar has Normal bg, content bg starts after bar)
-- Note: text_hl is used for both text foreground AND line background (via line_hl_group)
function M.add_styled_line(lines, text, bar_hl, text_hl, show_bar, icon, icon_fg, indent, code_block)
  -- Default show_bar to true if bar_hl is provided
  if show_bar == nil then
    show_bar = (bar_hl ~= nil)
  end

  -- Actual buffer content: use space for empty lines so cursor has a position
  local content = text == '' and ' ' or text
  table.insert(lines, content)
  local line_idx = #lines - 1

  -- Store extmark info for this line
  table.insert(shared.extmarks, {
    line = line_idx,
    bar_hl = show_bar and bar_hl or 'Normal',  -- Use Normal (invisible) if no bar
    text_hl = text_hl,
    text_len = #content,
    has_bar = true,  -- Always reserve space for alignment
    show_bar = show_bar,
    icon = icon,
    icon_fg = icon_fg,
    indent = indent,        -- Virtual text before bar (Normal bg)
    code_block = code_block, -- Special code block handling
  })
end

-- Helper: add an empty separator line (no bar, no styling, just spacing)
function M.add_empty_line(lines)
  -- Use space so cursor has a position (not truly empty)
  table.insert(lines, ' ')
  local line_idx = #lines - 1

  -- Add extmark with spacer for alignment but no visible bar
  table.insert(shared.extmarks, {
    line = line_idx,
    has_bar = true,  -- Reserve space for alignment
    show_bar = false,
    bar_hl = 'Normal',
    text_hl = nil,
  })
end

function M.add_vertical_padding(lines, pad_lines)
  for _ = 1, pad_lines do
    M.add_empty_line(lines)
  end
end

-- Convert highlight prefix to vim.g variable name
-- e.g., 'BB7UserMessage' -> 'bb7_user_message'
function M.prefix_to_var(prefix)
  -- Remove BB7 prefix and convert CamelCase to snake_case
  local name = prefix:gsub('^BB7', '')
  return 'bb7_' .. name:gsub('(%u)', function(c) return '_' .. c:lower() end):gsub('^_', '')
end

-- Get icon configuration for a highlight prefix
-- Returns icon, icon_fg (both may be nil)
function M.get_prefix_icon(prefix)
  local var_name = M.prefix_to_var(prefix)
  return vim.g[var_name .. '_icon'], vim.g[var_name .. '_icon_fg']
end

-- Helper: add a styled line using highlight group prefix
-- prefix should be like 'BB7UserMessage', 'BB7AssistantAction', etc.
-- Looks up optional icon from vim.g.{prefix}_icon (e.g., vim.g.bb7_user_message_icon)
function M.add_chunk_line(lines, text, prefix)
  local bar_hl = prefix .. 'Bar'
  local text_hl = prefix .. 'Text'
  local icon, icon_fg = M.get_prefix_icon(prefix)
  M.add_styled_line(lines, text, bar_hl, text_hl, true, icon, icon_fg)
end

-- Format token count for display
function M.format_tokens_short(count)
  if not count or count == 0 then return '0' end
  if count >= 1000 then
    return string.format('%.1fk', count / 1000)
  end
  return tostring(count)
end

return M
