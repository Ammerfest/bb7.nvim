-- Render pipeline for preview pane
local M = {}

local shared = require('bb7.panes.preview.shared')
local format = require('bb7.panes.preview.format')
local highlight = require('bb7.panes.preview.highlight')
local syntax = require('bb7.panes.preview.syntax')
local utils = require('bb7.utils')

-- Apply extmarks (virtual text for bars + padding, highlights for text/bg)
-- Note: text_hl is used for both text foreground AND line background
local function apply_extmarks(buf)
  for _, em in ipairs(shared.extmarks) do
    local bar_char = shared.config.bar_char
    local bar_width = vim.fn.strwidth(bar_char)

    if em.has_bar then
      -- Build virtual text components
      local virt_text = {}
      -- For code blocks: special handling
      -- - indent before bar (Normal bg)
      -- - bar with Normal bg (just fg color from bar_hl)
      -- - padding with code bg
      -- - buffer content with code bg (via hl_eol extmark, not line_hl_group)
      if em.code_block then
        -- Add indent before bar (Normal bg)
        if em.indent and #em.indent > 0 then
          table.insert(virt_text, { em.indent, 'Normal' })
        end

        -- Bar with Normal bg (get fg from bar_hl)
        if em.show_bar then
          local bar_hl_def = vim.api.nvim_get_hl(0, { name = em.bar_hl, link = false })
          local normal_hl = vim.api.nvim_get_hl(0, { name = 'Normal', link = false })
          local bar_with_normal_bg = 'BB7CodeBar_NormalBg'
          if not shared.hl_cache[bar_with_normal_bg] then
            vim.api.nvim_set_hl(0, bar_with_normal_bg, { fg = bar_hl_def.fg, bg = normal_hl.bg })
            shared.hl_cache[bar_with_normal_bg] = true
          end
          table.insert(virt_text, { bar_char, bar_with_normal_bg })
        else
          table.insert(virt_text, { string.rep(' ', bar_width), 'Normal' })
        end

        -- Padding with code bg (from text_hl)
        local text_hl_resolved = vim.api.nvim_get_hl(0, { name = em.text_hl or 'Normal', link = false })
        local content_bg = text_hl_resolved.bg
        local padding_bg_hl = 'BB7CodePadding_' .. tostring(content_bg)
        if not shared.hl_cache[padding_bg_hl] then
          vim.api.nvim_set_hl(0, padding_bg_hl, { bg = content_bg })
          shared.hl_cache[padding_bg_hl] = true
        end
        table.insert(virt_text, { string.rep(' ', shared.config.style.bar_padding), padding_bg_hl })

        vim.api.nvim_buf_set_extmark(buf, shared.ns_id, em.line, 0, {
          virt_text = virt_text,
          virt_text_pos = 'inline',
        })

        -- Background for buffer content: use line_hl_group to extend to end of screen
        -- Virtual text has explicit bg set, so it will override line_hl_group in those areas
        -- Use bg-only highlight for the line and a low-priority fg-only highlight
        -- so syntax highlights can override foreground.
        if em.text_hl then
          vim.api.nvim_buf_set_extmark(buf, shared.ns_id, em.line, 0, {
            line_hl_group = highlight.get_hl_bg_only(em.text_hl),
            priority = 50,  -- Low priority so syntax highlights override
          })
          if em.text_len and em.text_len > 0 then
            vim.api.nvim_buf_set_extmark(buf, shared.ns_id, em.line, 0, {
              end_col = em.text_len,
              hl_group = highlight.get_hl_fg_only(em.text_hl),
              priority = 50,
            })
          end
        end
      else
        -- Non-code block: existing behavior
        -- Use get_hl_with_bg to ensure virtual text doesn't inherit line_hl_group's bg
        if em.show_bar then
          -- Show colored bar (ensure it has Normal bg, not line bg)
          table.insert(virt_text, { bar_char, highlight.get_hl_with_bg(em.bar_hl) })
        else
          -- Show invisible spacer for bar
          table.insert(virt_text, { string.rep(' ', bar_width), 'Normal' })
        end

        -- Add icon or padding (uses text's bg for content area)
        local text_hl_resolved = vim.api.nvim_get_hl(0, { name = em.text_hl or 'Normal', link = false })
        local content_bg = text_hl_resolved.bg

        -- Get or create padding highlight (text's bg, no fg needed)
        local padding_hl = em.text_hl or 'Normal'

        if em.icon then
          -- Determine icon highlight group (icon fg + text's bg)
          local hl_name
          local icon_fg = em.icon_fg or text_hl_resolved.fg
          local cache_key = 'BB7Icon_' .. tostring(icon_fg) .. '_' .. tostring(content_bg)
          if not shared.hl_cache[cache_key] then
            vim.api.nvim_set_hl(0, cache_key, { fg = icon_fg, bg = content_bg })
            shared.hl_cache[cache_key] = true
          end
          hl_name = cache_key

          table.insert(virt_text, { em.icon, hl_name })
          -- Remaining padding (text's bg)
          local icon_width = vim.fn.strwidth(em.icon)
          local remaining_padding = shared.config.style.bar_padding - icon_width
          if remaining_padding > 0 then
            table.insert(virt_text, { string.rep(' ', remaining_padding), padding_hl })
          end
        else
          -- Just padding (text's bg)
          table.insert(virt_text, { string.rep(' ', shared.config.style.bar_padding), padding_hl })
        end

        vim.api.nvim_buf_set_extmark(buf, shared.ns_id, em.line, 0, {
          virt_text = virt_text,
          virt_text_pos = 'inline',
        })

        -- Text highlight: fg for text, bg for full line background
        -- Use low priority so bold highlights (priority 4200) can override
        if em.text_hl then
          vim.api.nvim_buf_set_extmark(buf, shared.ns_id, em.line, 0, {
            line_hl_group = em.text_hl,
            priority = 50,
          })
        end
      end
    else
      -- Header or other non-styled lines - just text highlight
      if em.text_hl then
        vim.api.nvim_buf_add_highlight(buf, shared.ns_id, em.text_hl, em.line, 0, -1)
      end
    end
  end
end

-- TEMPORARY: Render placeholder with all chunk types for style testing
local function render_placeholder()
  if not shared.state.buf or not vim.api.nvim_buf_is_valid(shared.state.buf) then
    return
  end

  local lines = {}
  shared.extmarks = {}  -- Reset extmarks

  -- Tango Light palette (hardcoded, bright colors made pastel)
  local tango_colors = {
    [0]  = '#000000',  -- black
    [1]  = '#cc0000',  -- red
    [2]  = '#4e9a06',  -- green
    [3]  = '#c4a000',  -- yellow
    [4]  = '#3465a4',  -- blue
    [5]  = '#75507b',  -- magenta
    [6]  = '#06989a',  -- cyan
    [7]  = '#b9bdb5',  -- white (light gray)
    [8]  = '#a8a8a6',  -- bright black (lighter gray)
    [9]  = '#ffb3b3',  -- bright red (pastel)
    [10] = '#c8f0a0',  -- bright green (pastel)
    [11] = '#fff0a0',  -- bright yellow (pastel)
    [12] = '#a8d0ff',  -- bright blue (pastel)
    [13] = '#e0c0e0',  -- bright magenta (pastel)
    [14] = '#a0f0f0',  -- bright cyan (pastel)
    [15] = '#ffffff',  -- bright white
  }
  local function get_term_color(n)
    return tango_colors[n]
  end

  -- Create highlight groups for all 16x16 color combinations
  local color_ns = vim.api.nvim_create_namespace('bb7_color_grid')
  for bg = 0, 15 do
    for fg = 0, 15 do
      local hl_name = string.format('BB7Color_%d_%d', bg, fg)
      local fg_color = get_term_color(fg)
      local bg_color = get_term_color(bg)
      vim.api.nvim_set_hl(0, hl_name, {
        fg = fg_color,
        bg = bg_color,
        ctermfg = fg,
        ctermbg = bg,
      })
    end
  end

  -- Header with column numbers (5 chars per column to match cells)
  local header = '      '  -- 6 chars for row label column
  for col = 0, 15 do
    header = header .. string.format('%3d  ', col)
  end
  table.insert(lines, header)
  table.insert(shared.extmarks, { line = 0, text_hl = 'Comment' })

  -- Grid rows
  local cell_text = 'qYw'
  local grid_highlights = {}  -- { line, col_start, col_end, hl_group }

  for row = 0, 15 do
    -- Row label + cells
    local line = string.format('  %2d  ', row)
    local col_positions = {}  -- track where each cell starts

    for col = 0, 15 do
      col_positions[col] = #line
      line = line .. cell_text .. '  '  -- cell + 2 space gap
    end

    table.insert(lines, line)
    local line_idx = #lines - 1

    -- Store highlight info for each cell (row=fg, col=bg)
    for col = 0, 15 do
      local hl_name = string.format('BB7Color_%d_%d', col, row)
      local start_col = col_positions[col]
      local end_col = start_col + #cell_text
      table.insert(grid_highlights, { line_idx, start_col, end_col, hl_name })
    end

    -- Row label highlight
    table.insert(shared.extmarks, { line = line_idx, text_hl = 'Comment' })
  end

  -- Legend
  table.insert(lines, '')
  table.insert(lines, '  Columns = background color (0-15)')
  table.insert(lines, '  Rows = foreground color (0-15)')
  table.insert(lines, '')
  table.insert(lines, '  Toggle: :lua require("bb7.panes.preview").toggle_placeholder()')

  local legend_start = 17
  for i = legend_start, #lines - 1 do
    table.insert(shared.extmarks, { line = i, text_hl = 'Comment' })
  end

  -- Update buffer
  vim.bo[shared.state.buf].modifiable = true
  vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, lines)
  vim.bo[shared.state.buf].modifiable = false

  -- Clear old extmarks and apply new ones
  vim.api.nvim_buf_clear_namespace(shared.state.buf, shared.ns_id, 0, -1)
  vim.api.nvim_buf_clear_namespace(shared.state.buf, color_ns, 0, -1)

  -- Apply grid cell highlights
  for _, hl in ipairs(grid_highlights) do
    vim.api.nvim_buf_add_highlight(shared.state.buf, color_ns, hl[4], hl[1], hl[2], hl[3])
  end

  -- Apply extmarks for labels
  for _, em in ipairs(shared.extmarks) do
    if em.text_hl then
      vim.api.nvim_buf_add_highlight(shared.state.buf, shared.ns_id, em.text_hl, em.line, 0, 6)  -- Just the label portion
    end
  end
end

-- Render the chat header
local function render_header(lines)
  if not shared.state.chat then
    return
  end

  -- Chat started line - styled as user action
  local created = utils.format_time(shared.state.chat.created)
  local user_icon, user_icon_fg = format.get_prefix_icon('BB7UserAction')
  format.add_styled_line(lines, 'Chat started ' .. created, 'BB7UserActionBar', 'BB7UserActionText', true, user_icon, user_icon_fg)
  shared.state.last_rendered_type = 'context_event'
  shared.state.last_rendered_role = 'user'

  -- Instructions info (show which instruction files are loaded)
  local instructions_info = shared.state.chat.instructions_info
  if instructions_info then
    local instr_parts = {}
    if instructions_info.global_exists then
      table.insert(instr_parts, 'global')
    end
    if instructions_info.project_exists then
      if instructions_info.project_error then
        table.insert(instr_parts, 'project (!)')
      else
        table.insert(instr_parts, 'project')
      end
    end
    if #instr_parts > 0 then
      format.add_styled_line(lines, 'Instructions: ' .. table.concat(instr_parts, ', '), 'BB7UserActionBar', 'BB7UserActionText', true, nil, nil)
    end
  end
end

-- Render a text segment (regular text, no code blocks)
-- first_line_of_part indicates if this is the first line of the text part (for icon)
local function render_text_segment(content, lines, role, first_line_of_part)
  local prefix = role == 'user' and 'BB7UserMessage' or 'BB7AssistantMessage'
  local icon, icon_fg = format.get_prefix_icon(prefix)
  local text_width = format.get_text_width(0)
  local text_hl = prefix .. 'Text'
  local bold_hl = highlight.get_bold_hl(text_hl)
  local italic_hl = highlight.get_italic_hl(text_hl)
  local underline_hl = highlight.get_underline_hl(text_hl)
  local first_line = first_line_of_part

  for _, line in ipairs(vim.split(content, '\n', { plain = true })) do
    -- Process style markers to get display text and styled regions
    local processed = format.process_bold_markers(line)
    local display_text = processed.display
    local bold_regions = processed.bold_regions
    local italic_regions = processed.italic_regions
    local underline_regions = processed.underline_regions

    -- Wrap the display text
    local wrapped = format.wrap_text(display_text, text_width)

    -- Track cumulative position for mapping regions to wrapped lines
    local cum_pos = 0

    for _, wrapped_line in ipairs(wrapped) do
      local line_idx = #lines  -- 0-indexed line number (before adding)
      local wrapped_len = #wrapped_line

      local line_icon = first_line and icon or nil
      local line_icon_fg = first_line and icon_fg or nil
      format.add_styled_line(lines, wrapped_line, prefix .. 'Bar', text_hl, true, line_icon, line_icon_fg)
      first_line = false

      -- Helper to apply style regions
      local function apply_style_regions(regions, hl_group)
        for _, region in ipairs(regions) do
          local region_start, region_end = region[1], region[2]
          local line_start = cum_pos
          local line_end = cum_pos + wrapped_len

          if region_start < line_end and region_end > line_start then
            local hl_start = math.max(0, region_start - line_start)
            local hl_end = math.min(wrapped_len, region_end - line_start)
            if hl_end > hl_start then
              table.insert(shared.syntax_highlights, { line_idx, hl_start, hl_end, hl_group })
            end
          end
        end
      end

      apply_style_regions(bold_regions, bold_hl)
      apply_style_regions(italic_regions, italic_hl)
      apply_style_regions(underline_regions, underline_hl)

      -- Update cumulative position (account for trimmed trailing space in wrap)
      cum_pos = cum_pos + wrapped_len
      -- If there was a space separator that got trimmed, account for it
      if cum_pos < #display_text then
        cum_pos = cum_pos + 1  -- Skip the space that was trimmed
      end
    end
  end

  return first_line  -- Returns false if any lines were rendered
end

-- Render a code block segment
-- Returns the updated first_line state
local function render_code_segment(content, lang, lines, role, first_line_of_part)
  local msg_prefix = role == 'user' and 'BB7UserMessage' or 'BB7AssistantMessage'
  local code_prefix = role == 'user' and 'BB7UserCode' or 'BB7AssistantCode'
  local icon, icon_fg = format.get_prefix_icon(msg_prefix)
  local indent_width = vim.fn.strwidth(shared.config.code_indent)
  local code_width = format.get_text_width(indent_width)

  -- Get syntax highlights if language is available
  local highlights = syntax.get_code_highlights(content, lang)
  local has_syntax_hl = #highlights > 0

  -- Highlight groups
  local bar_hl = code_prefix .. 'Bar'
  local lang_hl = code_prefix .. 'Lang'
  local text_hl = code_prefix .. 'Text'

  -- Render each line of code
  local code_lines = vim.split(content, '\n', { plain = true })
  local first_line = first_line_of_part

  -- Add language label line first (code_block=true, indent as virtual text)
  local lang_label = lang or 'text'
  local lang_line_icon = first_line and icon or nil
  local lang_line_icon_fg = first_line and icon_fg or nil
  -- Language label: buffer content is just the label, indent is virtual text
  format.add_styled_line(lines, lang_label, bar_hl, lang_hl, true, lang_line_icon, lang_line_icon_fg, shared.config.code_indent, true)
  first_line = false

  for row_idx, code_line in ipairs(code_lines) do
    -- Wrap long code lines
    local wrapped = format.wrap_text(code_line, code_width)
    for wrap_idx, wrapped_line in ipairs(wrapped) do
      local line_idx = #lines  -- 0-indexed line number in buffer (before adding this line)

      -- Use code styling with visible bar
      -- Buffer content is just the code, indent is virtual text
      format.add_styled_line(lines, wrapped_line, bar_hl, text_hl, true, nil, nil, shared.config.code_indent, true)

      -- Apply syntax highlights for this line (only first wrap segment gets highlights)
      -- Note: indent is now virtual text, so columns in buffer start at 0
      if wrap_idx == 1 and has_syntax_hl then
        local row = row_idx - 1  -- 0-indexed row in code block
        for _, hl in ipairs(highlights) do
          if hl[1] == row then
            -- hl format: { row, col_start, col_end, hl_group }
            -- No indent adjustment needed - buffer content starts at col 0
            local col_start = hl[2]
            local col_end = hl[3]
            -- Clamp to line length
            local line_len = #wrapped_line
            if col_start < line_len then
              col_end = math.min(col_end, line_len)
              table.insert(shared.syntax_highlights, { line_idx, col_start, col_end, hl[4] })
            end
          end
        end
      end
    end
  end

  return first_line
end

-- Render a text part (may contain code blocks)
-- role is 'user' or 'assistant' to determine styling
local function render_text_part(part, lines, role)
  local content = part.content or ''

  -- Parse content into text and code segments
  local segments = format.parse_text_segments(content)

  -- If no segments (empty content), return
  if #segments == 0 then
    return
  end

  local first_line = true

  for _, segment in ipairs(segments) do
    if segment.type == 'text' then
      first_line = render_text_segment(segment.content, lines, role, first_line)
    elseif segment.type == 'code' then
      first_line = render_code_segment(segment.content, segment.lang, lines, role, first_line)
    end
  end
end

function M.render_text_part(lines, role, content)
  if not content or content == '' then
    return
  end
  render_text_part({ content = content }, lines, role)
end

local function render_meta_line(text, lines)
  local icon, icon_fg = format.get_prefix_icon('BB7UserAction')
  format.add_styled_line(lines, text, 'BB7UserActionBar', 'BB7UserActionText', true, icon, icon_fg)
end

-- Render a context event (file writes, user actions, etc.)
-- Returns true if something was rendered, false if the event should be hidden
local function render_context_event(part, lines)
  local action = part.action or ''
  local path = part.path or 'unknown'

  -- Get icons for action types
  local assistant_icon, assistant_icon_fg = format.get_prefix_icon('BB7AssistantAction')
  local user_icon, user_icon_fg = format.get_prefix_icon('BB7UserAction')

  if action == 'AssistantWriteFile' then
    -- LLM wrote a file
    local label = part.added and 'Assistant added' or 'Assistant modified'
    format.add_styled_line(lines, label .. ': ' .. path, 'BB7AssistantActionBar', 'BB7AssistantActionText', true, assistant_icon, assistant_icon_fg)
    return true

  elseif action == 'UserApplyFile' then
    -- User applied output to context
    format.add_styled_line(lines, 'Applied: ' .. path, 'BB7UserActionBar', 'BB7UserActionText', true, user_icon, user_icon_fg)
    return true

  elseif action == 'UserSaveAs' then
    -- User saved output to different path
    local orig = part.original_path or path
    format.add_styled_line(lines, 'Saved: ' .. orig .. ' → ' .. path, 'BB7UserActionBar', 'BB7UserActionText', true, user_icon, user_icon_fg)
    return true

  elseif action == 'UserRejectOutput' then
    -- User rejected/deleted output
    format.add_styled_line(lines, 'Rejected: ' .. path, 'BB7UserActionBar', 'BB7UserActionText', true, user_icon, user_icon_fg)
    return true

  elseif action == 'UserAddFile' then
    -- User added file to context
    format.add_styled_line(lines, 'Context added: ' .. path, 'BB7UserActionBar', 'BB7UserActionText', true, user_icon, user_icon_fg)
    return true

  elseif action == 'UserRemoveFile' then
    -- User removed file from context
    format.add_styled_line(lines, 'Context removed: ' .. path, 'BB7UserActionBar', 'BB7UserActionText', true, user_icon, user_icon_fg)
    return true

  elseif action == 'UserWriteFile' or action == 'UserUpdateFile' then
    -- User re-snapshotted file
    format.add_styled_line(lines, 'Context updated: ' .. path, 'BB7UserActionBar', 'BB7UserActionText', true, user_icon, user_icon_fg)
    return true
  end

  -- Unknown action - don't render
  return false
end

-- Render a reasoning part (LLM reasoning output)
-- reasoning_id is used for collapse state tracking
local function render_reasoning_part(part, lines, collapsed, reasoning_id, msg_idx)
  local content = part.content or ''
  -- Trim trailing whitespace/newlines to avoid empty lines at end
  content = content:gsub('%s+$', '')

  local thinking_icon, thinking_icon_fg = format.get_prefix_icon('BB7Thinking')

  -- Record line position for this reasoning block (header line)
  local header_line = #lines + 1  -- 1-indexed line number

  -- Track as anchor for navigation (reasoning blocks are anchors too)
  table.insert(shared.state.anchor_lines, header_line)
  if msg_idx then
    shared.state.anchor_msg_idx[header_line] = msg_idx
  end

  -- Add header line with collapse indicator
  local header_icon = collapsed and '▶' or '▼'
  format.add_styled_line(lines, header_icon .. ' Reasoning', 'BB7ThinkingBar', 'BB7ThinkingText', true, thinking_icon, thinking_icon_fg)

  -- Record this line as belonging to this reasoning block
  shared.state.reasoning_line_map[header_line] = reasoning_id

  -- Only show content if not collapsed
  if not collapsed then
    -- Empty line after header for spacing
    local spacer_line = #lines + 1
    format.add_styled_line(lines, '', 'BB7ThinkingBar', 'BB7ThinkingText', true, nil, nil)
    shared.state.reasoning_line_map[spacer_line] = reasoning_id

    local indent = 2  -- '  ' prefix
    local text_width = format.get_text_width(indent)
    local text_hl = 'BB7ThinkingText'
    local bold_hl = highlight.get_bold_hl(text_hl)
    local italic_hl = highlight.get_italic_hl(text_hl)
    local underline_hl = highlight.get_underline_hl(text_hl)

    for _, line in ipairs(vim.split(content, '\n', { plain = true })) do
      -- Process style markers (**bold**, *italic*, __underline__)
      local processed = format.process_bold_markers(line)
      local display_text = processed.display

      -- Wrap the display text
      local wrapped = format.wrap_text(display_text, text_width)
      local cum_pos = 0

      for _, wrapped_line in ipairs(wrapped) do
        local line_idx = #lines  -- 0-indexed for highlights
        local wrapped_len = #wrapped_line
        local content_line = #lines + 1
        -- No icon on content lines (header already has it)
        format.add_styled_line(lines, '  ' .. wrapped_line, 'BB7ThinkingBar', 'BB7ThinkingText', true, nil, nil)
        -- All content lines also map to the same reasoning block
        shared.state.reasoning_line_map[content_line] = reasoning_id

        -- Apply style regions (offset by 2 for indent)
        local function apply_style_regions(regions, hl_group)
          for _, region in ipairs(regions) do
            local region_start, region_end = region[1], region[2]
            local line_start = cum_pos
            local line_end = cum_pos + wrapped_len
            if region_start < line_end and region_end > line_start then
              local hl_start = math.max(0, region_start - line_start)
              local hl_end = math.min(wrapped_len, region_end - line_start)
              if hl_end > hl_start then
                table.insert(shared.syntax_highlights, { line_idx, hl_start + indent, hl_end + indent, hl_group })
              end
            end
          end
        end

        apply_style_regions(processed.bold_regions, bold_hl)
        apply_style_regions(processed.italic_regions, italic_hl)
        apply_style_regions(processed.underline_regions, underline_hl)

        cum_pos = cum_pos + wrapped_len
        if cum_pos < #display_text then
          cum_pos = cum_pos + 1
        end
      end
    end
  end
end

-- Render message parts (structured format)
-- msg_idx is used to create unique IDs for thinking blocks
-- role is 'user' or 'assistant' for text styling
-- Returns the last rendered part type ('text', 'context_event', 'thinking', or nil)
local function render_parts(parts, lines, msg_idx, role)
  local prev_type = shared.state.last_rendered_type  -- Continue from previous message

  -- Role change: add blank line to separate user/assistant content
  -- (but not for first content after header)
  if shared.state.last_rendered_role and role ~= shared.state.last_rendered_role then
    format.add_empty_line(lines)
    prev_type = nil  -- Reset so first part doesn't add another blank
  end

  -- Add user anchor AFTER blank line so navigation lands on actual content
  if role == 'user' then
    local anchor_line = #lines + 1
    table.insert(shared.state.anchor_lines, anchor_line)
    shared.state.anchor_msg_idx[anchor_line] = msg_idx
    table.insert(shared.state.user_anchor_lines, anchor_line)
    shared.state.user_anchor_msg_idx[anchor_line] = msg_idx
  end

  local last_type = nil
  local had_reasoning = false  -- Track if we've had a reasoning block
  local assistant_anchor_added = false  -- Track if we've added an anchor for assistant content
  for part_idx, part in ipairs(parts) do
    local part_type = part.type or 'text'
    local rendered = false
    if part_type == 'text' then
      -- Empty line before text (separates from previous content within same role)
      if prev_type then format.add_empty_line(lines) end
      -- For assistant: add anchor for first text OR text that follows reasoning
      if role == 'assistant' and (not assistant_anchor_added or had_reasoning) then
        local text_anchor = #lines + 1
        table.insert(shared.state.anchor_lines, text_anchor)
        shared.state.anchor_msg_idx[text_anchor] = msg_idx
        assistant_anchor_added = true
        had_reasoning = false  -- Reset so we only add one anchor per reasoning->text transition
      end
      render_text_part(part, lines, role)
      rendered = true
    elseif part_type == 'context_event' then
      -- Only empty line if previous was different type (group consecutive actions)
      if prev_type and prev_type ~= 'context_event' then format.add_empty_line(lines) end
      rendered = render_context_event(part, lines)
    elseif part_type == 'thinking' then
      -- Empty line before thinking (separates from previous content)
      if prev_type then format.add_empty_line(lines) end
      -- Create unique ID for this reasoning block
      local reasoning_id = tostring(msg_idx) .. ':' .. tostring(part_idx)
      -- Default to collapsed (true), unless explicitly expanded (set to false)
      local collapsed = shared.state.collapsed_reasoning[reasoning_id] ~= false
      render_reasoning_part(part, lines, collapsed, reasoning_id, msg_idx)
      had_reasoning = true
      assistant_anchor_added = true  -- reasoning block adds its own anchor
      rendered = true
    end

    if rendered then
      prev_type = part_type
      last_type = part_type
    end
  end
  -- Update tracking state (no trailing blank - leading blanks handle separation)
  shared.state.last_rendered_type = last_type
  shared.state.last_rendered_role = role
  return last_type
end

-- Render a system message (fork warnings, etc.)
-- System messages are not sent to LLM, just displayed to user
local function render_system_message(msg, lines)
  -- Add blank line to separate from previous content
  if shared.state.last_rendered_type then
    format.add_empty_line(lines)
  end

  local icon, icon_fg = format.get_prefix_icon('BB7SystemMessage')

  -- Render fork warnings with header + individual file lines
  if msg.parts and #msg.parts > 0 then
    -- Collect fork warnings
    local fork_warnings = {}
    for _, part in ipairs(msg.parts) do
      if part.type == 'context_event' and (part.action == 'ForkWarningModified' or part.action == 'ForkWarningDeleted') then
        table.insert(fork_warnings, part)
      end
    end

    if #fork_warnings > 0 then
      -- Render header line with icon
      format.add_styled_line(lines, 'Fork warning:', 'BB7SystemMessageBar', 'BB7SystemMessageText', true, icon, icon_fg)

      -- Render each file warning on its own line (no icon)
      for _, part in ipairs(fork_warnings) do
        local path = part.path or 'unknown file'
        local text
        if part.action == 'ForkWarningDeleted' then
          text = path .. ' no longer exists'
        else
          text = path .. ' differs from the forked chat state'
        end
        format.add_styled_line(lines, text, 'BB7SystemMessageBar', 'BB7SystemMessageText', true, nil, nil)
      end
    end
  elseif msg.content and msg.content ~= '' then
    -- Legacy content format
    local text_width = format.get_text_width(0)
    local first_line = true
    for _, line in ipairs(vim.split(msg.content, '\n', { plain = true })) do
      local wrapped = format.wrap_text(line, text_width)
      for _, wrapped_line in ipairs(wrapped) do
        local line_icon = first_line and icon or nil
        local line_icon_fg = first_line and icon_fg or nil
        format.add_styled_line(lines, wrapped_line, 'BB7SystemMessageBar', 'BB7SystemMessageText', true, line_icon, line_icon_fg)
        first_line = false
      end
    end
  end

  shared.state.last_rendered_type = 'system'
  shared.state.last_rendered_role = 'system'
end

-- Render a single message
-- msg_idx is used for creating unique IDs for collapsible elements
local function render_message(msg, lines, msg_idx)
  local role = msg.role or 'unknown'

  -- Handle system messages separately (fork warnings, etc.)
  -- System messages are not navigable and not sent to LLM
  if role == 'system' then
    render_system_message(msg, lines)
    return
  end

  local prefix = role == 'user' and 'BB7UserMessage' or 'BB7AssistantMessage'
  local bar_hl = prefix .. 'Bar'
  local text_hl = prefix .. 'Text'
  local icon, icon_fg = format.get_prefix_icon(prefix)

  -- Check for new structured parts format
  if msg.parts and #msg.parts > 0 then
    render_parts(msg.parts, lines, msg_idx, role)
  else
    -- Legacy format: render content as plain text
    -- Role change: add blank line to separate user/assistant content
    local need_blank = false
    if shared.state.last_rendered_role and role ~= shared.state.last_rendered_role then
      need_blank = true
    elseif shared.state.last_rendered_type then
      -- Same role but previous content exists
      need_blank = true
    end
    if need_blank then
      format.add_empty_line(lines)
    end
    -- Add anchor AFTER blank line so navigation lands on actual content
    local anchor_line = #lines + 1
    table.insert(shared.state.anchor_lines, anchor_line)
    shared.state.anchor_msg_idx[anchor_line] = msg_idx
    if role == 'user' then
      table.insert(shared.state.user_anchor_lines, anchor_line)
      shared.state.user_anchor_msg_idx[anchor_line] = msg_idx
    end
    local content = msg.content or ''
    local text_width = format.get_text_width(0)
    local first_line = true
    for _, line in ipairs(vim.split(content, '\n', { plain = true })) do
      local wrapped = format.wrap_text(line, text_width)
      for _, wrapped_line in ipairs(wrapped) do
        -- Icon only on first line
        local line_icon = first_line and icon or nil
        local line_icon_fg = first_line and icon_fg or nil
        format.add_styled_line(lines, wrapped_line, bar_hl, text_hl, true, line_icon, line_icon_fg)
        first_line = false
      end
    end
    -- Update tracking (no trailing blank - leading blanks handle separation)
    shared.state.last_rendered_type = 'text'
    shared.state.last_rendered_role = role
  end
end

-- Render the full chat view
function M.render()
  -- TEMPORARY: Show placeholder for style testing
  if shared.state.show_placeholder then
    render_placeholder()
    return
  end

  if not shared.state.buf or not vim.api.nvim_buf_is_valid(shared.state.buf) then
    return
  end

  local should_autoscroll = shared.state.autoscroll
  local prior_view = nil
  if shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
    prior_view = vim.fn.winsaveview()
    local line_count = vim.api.nvim_buf_line_count(shared.state.buf)
    if prior_view.lnum and prior_view.lnum < line_count then
      should_autoscroll = false
      shared.state.autoscroll = false
    end
  end

  if not shared.state.chat then
    local lines = {}
    shared.extmarks = {}
    shared.state.reasoning_line_map = {}
    shared.state.anchor_lines = {}
    shared.state.anchor_msg_idx = {}
    shared.state.user_anchor_lines = {}
    shared.state.user_anchor_msg_idx = {}
    local centered = format.center_text('No chat selected', format.get_text_width(0))
    local win_height = 10
    if shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
      win_height = vim.api.nvim_win_get_height(shared.state.win)
    end
    local message_lines = 1
    local top_pad = math.max(0, math.floor((win_height - message_lines) / 2))
    format.add_vertical_padding(lines, top_pad)
    format.add_styled_line(lines, centered, nil, 'Comment', false)
    format.add_vertical_padding(lines, 1)
    vim.bo[shared.state.buf].modifiable = true
    vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, lines)
    vim.bo[shared.state.buf].modifiable = false
    vim.api.nvim_buf_clear_namespace(shared.state.buf, shared.ns_id, 0, -1)
    apply_extmarks(shared.state.buf)
    if should_autoscroll and shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
      vim.api.nvim_win_set_cursor(shared.state.win, { #lines, 0 })
    elseif prior_view and shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
      vim.fn.winrestview(prior_view)
    end
    return
  end

  local lines = {}
  shared.extmarks = {}  -- Reset extmarks
  shared.syntax_highlights = {}  -- Reset syntax highlights
  shared.state.reasoning_line_map = {}  -- Reset line map
  shared.state.anchor_lines = {}        -- Reset anchor tracking
  shared.state.anchor_msg_idx = {}      -- Reset anchor -> msg_idx map
  shared.state.user_anchor_lines = {}   -- Reset user anchor tracking
  shared.state.user_anchor_msg_idx = {} -- Reset user anchor -> msg_idx map
  shared.state.last_rendered_type = nil -- Reset content type tracking
  shared.state.last_rendered_role = nil -- Reset role tracking

  -- Header
  render_header(lines)

  -- Send error (shown at top so it's always visible)
  if shared.state.send_error then
    if shared.state.last_rendered_type then
      format.add_empty_line(lines)
    end
    local error_icon, error_icon_fg = format.get_prefix_icon('BB7Error')
    local text_width = format.get_text_width(0)
    local error_text = shared.state.send_error
    local first_line = true
    for _, line in ipairs(vim.split(error_text, '\n', { plain = true })) do
      local wrapped = format.wrap_text(line, text_width)
      for _, wrapped_line in ipairs(wrapped) do
        local line_icon = first_line and error_icon or nil
        local line_icon_fg = first_line and error_icon_fg or nil
        format.add_styled_line(lines, wrapped_line, 'BB7ErrorBar', 'BB7ErrorText', true, line_icon, line_icon_fg)
        first_line = false
      end
    end
    shared.state.last_rendered_type = 'error'
    shared.state.last_rendered_role = nil
  end

  -- Messages
  if shared.state.chat and shared.state.chat.messages then
    local last_user_model = nil
    for msg_idx, msg in ipairs(shared.state.chat.messages) do
      if msg.role == 'user' and msg.model and msg.model ~= '' then
        if not last_user_model or last_user_model ~= msg.model then
          -- Add empty line before Model if there's previous content
          -- (handles role change and content type transitions)
          if shared.state.last_rendered_type then
            format.add_empty_line(lines)
          end
          render_meta_line('Model: ' .. msg.model, lines)
          shared.state.last_rendered_type = 'meta'
          shared.state.last_rendered_role = 'user'  -- Model line is part of user message
        end
        last_user_model = msg.model
      end
      render_message(msg, lines, msg_idx)
    end
  end

  -- Streaming: show user message first, then assistant response
  if shared.state.streaming then
    -- Show the user's message that's being responded to
    if shared.state.pending_user_message then
      M.render_text_part(lines, 'user', shared.state.pending_user_message)
    end

    format.add_empty_line(lines)

    -- Show reasoning content first (if any)
    if #shared.state.stream_reasoning_lines > 0 then
      local thinking_icon, thinking_icon_fg = format.get_prefix_icon('BB7Thinking')
      local reasoning_indent = 2
      local reasoning_text_width = format.get_text_width(reasoning_indent)

      -- Reasoning header (with icon)
      format.add_styled_line(lines, '▼ Reasoning', 'BB7ThinkingBar', 'BB7ThinkingText', true, thinking_icon, thinking_icon_fg)
      -- Empty line after header for spacing
      format.add_styled_line(lines, '', 'BB7ThinkingBar', 'BB7ThinkingText', true, nil, nil)
      -- Content lines with text formatting (no icon)
      local text_hl = 'BB7ThinkingText'
      local bold_hl = highlight.get_bold_hl(text_hl)
      local italic_hl = highlight.get_italic_hl(text_hl)
      local underline_hl = highlight.get_underline_hl(text_hl)
      for _, line in ipairs(shared.state.stream_reasoning_lines) do
        local processed = format.process_bold_markers(line)
        local display_text = processed.display
        local wrapped = format.wrap_text(display_text, reasoning_text_width)
        local cum_pos = 0
        for _, wrapped_line in ipairs(wrapped) do
          local line_idx = #lines
          local wrapped_len = #wrapped_line
          format.add_styled_line(lines, '  ' .. wrapped_line, 'BB7ThinkingBar', 'BB7ThinkingText', true, nil, nil)

          local function apply_style_regions(regions, hl_group)
            for _, region in ipairs(regions) do
              local region_start, region_end = region[1], region[2]
              local line_start = cum_pos
              local line_end = cum_pos + wrapped_len
              if region_start < line_end and region_end > line_start then
                local hl_start = math.max(0, region_start - line_start)
                local hl_end = math.min(wrapped_len, region_end - line_start)
                if hl_end > hl_start then
                  table.insert(shared.syntax_highlights, { line_idx, hl_start + reasoning_indent, hl_end + reasoning_indent, hl_group })
                end
              end
            end
          end

          apply_style_regions(processed.bold_regions, bold_hl)
          apply_style_regions(processed.italic_regions, italic_hl)
          apply_style_regions(processed.underline_regions, underline_hl)

          cum_pos = cum_pos + wrapped_len
          if cum_pos < #display_text then
            cum_pos = cum_pos + 1
          end
        end
      end
      format.add_empty_line(lines)
    end

    -- Show assistant response (streaming)
    local stream_content = table.concat(shared.state.stream_lines, '\n')
    if stream_content ~= '' then
      M.render_text_part(lines, 'assistant', stream_content)
    end

    -- Animated spinner at the bottom with duration
    format.add_empty_line(lines)
    local spinner = shared.config.spinner_frames[shared.state.spinner_frame] or shared.config.spinner_frames[1]
    local duration_str = ''
    if shared.persistent.stream_start_time then
      local elapsed = os.time() - shared.persistent.stream_start_time
      duration_str = ' ' .. format.format_duration(elapsed)
    end
    format.add_styled_line(lines, spinner .. ' Generating...' .. duration_str, nil, 'BB7Spinner', false)
  end

  -- Persistent usage line: find last assistant message with usage data
  if not shared.state.streaming and shared.state.chat and shared.state.chat.messages then
    local last_usage_msg = nil
    for i = #shared.state.chat.messages, 1, -1 do
      local msg = shared.state.chat.messages[i]
      if msg.role == 'assistant' and msg.usage then
        last_usage_msg = msg
        break
      end
    end
    if last_usage_msg then
      local usage = last_usage_msg.usage
      local usage_parts = {}
      -- Include duration only for the chat where the stream just completed
      if shared.persistent.last_duration
        and shared.persistent.last_stream_chat_id
        and shared.state.chat.id == shared.persistent.last_stream_chat_id then
        table.insert(usage_parts, format.format_duration(shared.persistent.last_duration))
      end
      local input_str = format.format_tokens_short(usage.prompt_tokens or 0)
      local output_str = format.format_tokens_short(usage.completion_tokens or 0)
      table.insert(usage_parts, input_str .. ' in / ' .. output_str .. ' out')
      if usage.cost and usage.cost > 0 then
        table.insert(usage_parts, string.format('$%.3f', usage.cost))
      end
      format.add_empty_line(lines)
      format.add_styled_line(lines, '✓ ' .. table.concat(usage_parts, ' · '), nil, 'Comment', false)
    end
  end

  -- Update buffer
  vim.bo[shared.state.buf].modifiable = true
  vim.api.nvim_buf_set_lines(shared.state.buf, 0, -1, false, lines)
  vim.bo[shared.state.buf].modifiable = false

  -- Apply extmarks (virtual text bars + highlights)
  vim.api.nvim_buf_clear_namespace(shared.state.buf, shared.ns_id, 0, -1)
  vim.api.nvim_buf_clear_namespace(shared.state.buf, shared.syntax_ns, 0, -1)
  apply_extmarks(shared.state.buf)

  -- Apply syntax highlights for code blocks (separate namespace, higher priority)
  -- Use fg-only variants to preserve code block background
  for _, hl in ipairs(shared.syntax_highlights) do
    -- hl format: { line, col_start, col_end, hl_group }
    local orig_hl = hl[4]
    local fg_only_hl = orig_hl .. '_FgOnly'

    -- Create fg-only variant if not cached
    if not shared.hl_cache[fg_only_hl] then
      -- Resolve the highlight, following links to get actual colors
      local hl_def = vim.api.nvim_get_hl(0, { name = orig_hl, link = false })

      -- If it's a link, follow it to get the actual fg
      local fg = hl_def.fg
      if not fg and hl_def.link then
        local linked_def = vim.api.nvim_get_hl(0, { name = hl_def.link, link = false })
        fg = linked_def.fg
      end

      -- If still no fg, try getting the resolved highlight (without link=false)
      if not fg then
        -- This follows all links automatically
        local resolved = vim.api.nvim_get_hl(0, { name = orig_hl })
        if type(resolved) == 'table' then
          fg = resolved.fg
        end
      end

      if fg then
        -- Preserve bold/italic attributes from original highlight
        local new_hl = { fg = fg }
        if hl_def.bold then new_hl.bold = true end
        if hl_def.italic then new_hl.italic = true end
        if hl_def.underline then new_hl.underline = true end
        vim.api.nvim_set_hl(0, fg_only_hl, new_hl)
        shared.hl_cache[fg_only_hl] = true
      else
        -- No fg defined, skip this highlight
        shared.hl_cache[fg_only_hl] = 'skip'
      end
    end

    if shared.hl_cache[fg_only_hl] ~= 'skip' then
      vim.api.nvim_buf_set_extmark(shared.state.buf, shared.syntax_ns, hl[1], hl[2], {
        end_col = hl[3],
        hl_group = fg_only_hl,
        priority = 4200,  -- Higher than line_hl_group default (4096)
      })
    end
  end

  -- Scroll to bottom
  if should_autoscroll and shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
    local line_count = vim.api.nvim_buf_line_count(shared.state.buf)
    vim.api.nvim_win_set_cursor(shared.state.win, { line_count, 0 })
  elseif prior_view and shared.state.win and vim.api.nvim_win_is_valid(shared.state.win) then
    vim.fn.winrestview(prior_view)
  end
end

return M
