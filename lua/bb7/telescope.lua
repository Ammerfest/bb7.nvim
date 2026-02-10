-- Telescope integration for BB7 chat search
-- This module is only loaded when Telescope is available

local M = {}

-- Check if Telescope is available
local has_telescope, telescope = pcall(require, 'telescope')
if not has_telescope then
  return M
end

local pickers = require('telescope.pickers')
local finders = require('telescope.finders')
local conf = require('telescope.config').values
local actions = require('telescope.actions')
local action_state = require('telescope.actions.state')
local previewers = require('telescope.previewers')
local sorters = require('telescope.sorters')

-- Format a chat for display in the results list
local function format_entry(entry)
  local display = entry.value.name
  if entry.value.match_type == 'content' and entry.value.excerpt then
    display = display .. ' | ' .. entry.value.excerpt
  end
  return display
end

-- Format chat messages for the preview pane
local function format_chat_preview(chat)
  local lines = {}

  -- Header
  table.insert(lines, 'Chat: ' .. (chat.name or 'Untitled'))
  table.insert(lines, 'Created: ' .. (chat.created or 'Unknown'))
  table.insert(lines, string.rep('-', 60))
  table.insert(lines, '')

  -- Messages
  if chat.messages then
    for _, msg in ipairs(chat.messages) do
      local role = msg.role == 'user' and 'USER' or 'ASSISTANT'
      table.insert(lines, '[' .. role .. ']')

      -- Get message text
      local text = msg.content or ''
      if msg.parts and #msg.parts > 0 then
        local parts_text = {}
        for _, part in ipairs(msg.parts) do
          if part.content and part.content ~= '' then
            table.insert(parts_text, part.content)
          end
        end
        text = table.concat(parts_text, '\n')
      end

      -- Split into lines and add
      for line in text:gmatch('[^\n]*') do
        table.insert(lines, line)
      end
      table.insert(lines, '')
    end
  else
    table.insert(lines, '(No messages)')
  end

  return lines
end

-- Custom sorter that prioritizes title matches over content matches
-- Title matches (from backend) appear at bottom (lower score = higher priority)
-- Content matches appear at top (higher score = lower priority)
local function chat_sorter()
  return sorters.Sorter:new({
    scoring_function = function(_, prompt, line, entry)
      -- Backend already filtered results, just order by match type
      if entry.value.match_type == 'title' then
        -- Title match: low score = appears at bottom (near input)
        return 1 + (entry.index or 0) * 0.001
      else
        -- Content match: higher score = appears at top
        return 10 + (entry.index or 0) * 0.001
      end
    end,

    -- No highlighting needed since backend does the filtering
    highlighter = function() return {} end,
  })
end

-- Create the chat previewer
local function chat_previewer()
  return previewers.new_buffer_previewer({
    title = 'Chat Preview',

    define_preview = function(self, entry, status)
      local client = require('bb7.client')

      -- Clear buffer first
      vim.api.nvim_buf_set_lines(self.state.bufnr, 0, -1, false, { 'Loading...' })

      -- Select and get the full chat
      client.request({ action = 'chat_select', id = entry.value.id }, function(_, select_err)
        if select_err then
          vim.schedule(function()
            if vim.api.nvim_buf_is_valid(self.state.bufnr) then
              vim.api.nvim_buf_set_lines(self.state.bufnr, 0, -1, false, { 'Error: ' .. select_err })
            end
          end)
          return
        end

        client.request({ action = 'chat_get' }, function(chat_data, get_err)
          vim.schedule(function()
            if not vim.api.nvim_buf_is_valid(self.state.bufnr) then
              return
            end

            if get_err then
              vim.api.nvim_buf_set_lines(self.state.bufnr, 0, -1, false, { 'Error: ' .. get_err })
              return
            end

            local lines = format_chat_preview(chat_data)
            vim.api.nvim_buf_set_lines(self.state.bufnr, 0, -1, false, lines)
          end)
        end)
      end)
    end,
  })
end

-- Create entry maker for search results
local function make_entry_maker()
  return function(chat)
    return {
      value = chat,
      display = format_entry,
      ordinal = chat.name,
    }
  end
end

-- Search chats using Telescope
function M.search_chats(opts)
  opts = opts or {}

  local client = require('bb7.client')
  local ui = require('bb7.ui')
  local log = require('bb7.log')

  -- Ensure backend is initialized
  if not client.is_initialized() then
    log.warn('BB-7 not initialized - open BB7 first')
    return
  end

  -- State for dynamic searching
  local current_picker = nil
  local last_query = nil
  local debounce_timer = nil

  -- Function to refresh results based on query
  local function refresh_results(prompt)
    -- Don't re-query if prompt hasn't changed
    if prompt == last_query then
      return
    end
    last_query = prompt

    client.request({ action = 'search_chats', query = prompt or '' }, function(response, err)
      if err then
        log.error('Search failed: ' .. err)
        return
      end

      local results = response.results or {}

      vim.schedule(function()
        if current_picker then
          -- Refresh the picker with new results
          current_picker:refresh(finders.new_table({
            results = results,
            entry_maker = make_entry_maker(),
          }), { reset_prompt = false })
        end
      end)
    end)
  end

  -- Get initial chat list (all chats)
  client.request({ action = 'search_chats', query = '' }, function(response, err)
    if err then
      log.error('Failed to search chats: ' .. err)
      return
    end

    local results = response.results or {}
    last_query = ''

    vim.schedule(function()
      current_picker = pickers.new(opts, {
        prompt_title = 'Search BB-7 Chats',
        finder = finders.new_table({
          results = results,
          entry_maker = make_entry_maker(),
        }),
        sorter = chat_sorter(),
        previewer = chat_previewer(),
        attach_mappings = function(prompt_bufnr, map)
          -- Watch for prompt changes and refresh results
          local prompt_buf = vim.api.nvim_get_current_buf()

          vim.api.nvim_create_autocmd({ 'TextChanged', 'TextChangedI' }, {
            buffer = prompt_bufnr,
            callback = function()
              -- Debounce: wait 150ms after typing stops before searching
              if debounce_timer then
                vim.fn.timer_stop(debounce_timer)
              end
              debounce_timer = vim.fn.timer_start(150, function()
                vim.schedule(function()
                  local picker = action_state.get_current_picker(prompt_bufnr)
                  if picker then
                    local prompt = picker:_get_prompt()
                    refresh_results(prompt)
                  end
                end)
              end)
            end,
          })

          actions.select_default:replace(function()
            local entry = action_state.get_selected_entry()
            actions.close(prompt_bufnr)

            if entry then
              -- Open BB7 and switch to selected chat
              ui.open()
              client.request({ action = 'chat_select', id = entry.value.id }, function(_, select_err)
                if select_err then
                  log.error('Failed to select chat: ' .. select_err)
                  return
                end

                -- Refresh the chats pane to show the selection
                vim.schedule(function()
                  local chats_pane = require('bb7.panes.chats')
                  chats_pane.refresh()
                end)
              end)
            end
          end)

          return true
        end,
      })

      current_picker:find()
    end)
  end)
end

-- Check if Telescope is available
function M.is_available()
  return has_telescope
end

return M
