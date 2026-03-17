-- Snacks.nvim picker integration for BB7 chat search
-- This module is only loaded when snacks.nvim is available

local M = {}

local has_snacks, _ = pcall(require, 'snacks')
if not has_snacks then
  return M
end

-- Format a chat for the results list
local function format_entry(item)
  local chat = item.chat
  local ret = {}
  ret[#ret + 1] = { chat.name or 'Untitled' }
  if chat.match_type == 'content' and chat.excerpt then
    ret[#ret + 1] = { ' | ' .. chat.excerpt, 'SnacksPickerComment' }
  end
  if chat.global then
    ret[#ret + 1] = { ' (Global)', 'SnacksPickerComment' }
  end
  return ret
end

-- Format chat messages for the preview buffer
local function format_chat_preview(chat)
  local lines = {}

  table.insert(lines, 'Chat: ' .. (chat.name or 'Untitled'))
  table.insert(lines, 'Created: ' .. (chat.created or 'Unknown'))
  table.insert(lines, string.rep('-', 60))
  table.insert(lines, '')

  if chat.messages then
    for _, msg in ipairs(chat.messages) do
      local role = msg.role == 'user' and 'USER' or 'ASSISTANT'
      table.insert(lines, '[' .. role .. ']')

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

-- Open the picker (called after backend is initialized)
local function open_picker()
  local client = require('bb7.client')
  local ui = require('bb7.ui')
  local log = require('bb7.log')

  Snacks.picker({
    title = 'Search BB-7 Chats',
    live = true,

    finder = function(opts, ctx)
      local search = ctx.filter and ctx.filter.search or ''

      -- Return async function: receives `add` callback for each item
      return function(add)
        local async = ctx.async
        local results = {}
        local pending = 2

        vim.schedule(function()
          -- Search project chats
          client.request({ action = 'search_chats', query = search }, function(response, err)
            if not err and type(response.results) == 'table' then
              for _, chat in ipairs(response.results) do
                table.insert(results, chat)
              end
            end
            pending = pending - 1
            if pending == 0 then async:resume() end
          end)

          -- Search global chats
          client.request({ action = 'search_chats', query = search, global = true }, function(response, err)
            if not err and type(response.results) == 'table' then
              for _, chat in ipairs(response.results) do
                chat.global = true
                table.insert(results, chat)
              end
            end
            pending = pending - 1
            if pending == 0 then async:resume() end
          end)
        end)
        async:suspend()

        for _, chat in ipairs(results) do
          add({
            text = chat.name or 'Untitled',
            chat = chat,
          })
        end
      end
    end,

    format = format_entry,

    preview = function(ctx)
      local item = ctx.item
      if not item or not item.chat then return end

      local client = require('bb7.client')
      local buf = ctx.buf
      local is_global = item.chat.global

      local function set_lines(lines)
        vim.bo[buf].modifiable = true
        vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
        vim.bo[buf].modifiable = false
      end

      set_lines({ 'Loading...' })

      local select_req = { action = 'chat_select', id = item.chat.id }
      if is_global then select_req.global = true end

      client.request(select_req, function(_, select_err)
        if select_err then
          vim.schedule(function()
            if vim.api.nvim_buf_is_valid(buf) then
              set_lines({ 'Error: ' .. select_err })
            end
          end)
          return
        end

        client.request({ action = 'chat_get' }, function(chat_data, get_err)
          vim.schedule(function()
            if not vim.api.nvim_buf_is_valid(buf) then return end

            if get_err then
              set_lines({ 'Error: ' .. get_err })
              return
            end

            set_lines(format_chat_preview(chat_data))
          end)
        end)
      end)
    end,

    confirm = function(picker, item)
      picker:close()
      if not item or not item.chat then return end

      local client = require('bb7.client')
      local log = require('bb7.log')
      local is_global = item.chat.global

      ui.open()
      local select_req = { action = 'chat_select', id = item.chat.id }
      if is_global then select_req.global = true end

      client.request(select_req, function(_, select_err)
        if select_err then
          log.error('Failed to select chat: ' .. select_err)
          return
        end

        vim.schedule(function()
          require('bb7.panes.chats').refresh()
        end)
      end)
    end,
  })
end

-- Search chats using snacks.nvim picker (auto-initializes backend)
function M.search_chats()
  local client = require('bb7.client')

  if client.is_initialized() then
    open_picker()
    return
  end

  -- Auto-initialize before opening picker
  local project_root = vim.fn.getcwd()
  client.init(project_root, function(resp, init_err)
    if init_err then
      require('bb7.log').error('Failed to initialize: ' .. init_err)
      return
    end
    vim.schedule(function()
      open_picker()
    end)
  end)
end

function M.is_available()
  return has_snacks
end

return M
