-- BB7 client: manages communication with the Go backend process
local M = {}

local log = require('bb7.log')

local state = {
  job_id = nil,
  pending_callbacks = {}, -- map of request_id -> callback
  pending_queue = {},     -- request_id FIFO for legacy/no-id responses
  next_request_id = 1,
  stream_handlers = nil,  -- { on_chunk, on_done, on_error } for streaming
  stream_request_id = nil,
  stream_buffer = nil,  -- { content, reasoning, user_message }
  event_handlers = {},    -- { title_updated = fn, ... } for async events
  response_buffer = '',   -- buffer for partial JSON lines
  initialized = false,
  project_root = nil,
}

local function remove_pending_id(id)
  for i, value in ipairs(state.pending_queue) do
    if value == id then
      table.remove(state.pending_queue, i)
      return
    end
  end
end

-- Get path to bb7 binary
local function get_bin_path()
  -- Check if user configured a path
  local config = require('bb7').get_config and require('bb7').get_config()
  if config and config.bin then
    return config.bin
  end
  -- Default: look for bb7 in PATH or same directory as plugin
  local plugin_dir = vim.fn.fnamemodify(debug.getinfo(1, 'S').source:sub(2), ':h:h:h')
  local local_bin = plugin_dir .. '/bb7'
  if vim.fn.executable(local_bin) == 1 then
    return local_bin
  end
  return 'bb7' -- hope it's in PATH
end

-- Handle a line of output from the backend
local function handle_output(line)
  if line == '' then return end

  local ok, data = pcall(vim.json.decode, line)
  if not ok then
    log.error('Failed to parse response: ' .. line)
    return
  end

  local msg_type = data.type
  local resp_id = data.request_id and tostring(data.request_id) or nil

  -- Handle streaming responses
  if msg_type == 'chunk' then
    if resp_id and state.stream_request_id == resp_id then
      if state.stream_buffer then
        state.stream_buffer.content = state.stream_buffer.content .. (data.content or '')
      end
    end
    if state.stream_handlers and state.stream_request_id == resp_id and state.stream_handlers.on_chunk then
      state.stream_handlers.on_chunk(data.content)
    end
    return
  end

  if msg_type == 'thinking' then
    if resp_id and state.stream_request_id == resp_id then
      if state.stream_buffer then
        state.stream_buffer.reasoning = state.stream_buffer.reasoning .. (data.content or '')
      end
    end
    if state.stream_handlers and state.stream_request_id == resp_id and state.stream_handlers.on_reasoning then
      state.stream_handlers.on_reasoning(data.content)
    end
    return
  end

  if msg_type == 'done' then
    if state.stream_handlers and state.stream_request_id == resp_id and state.stream_handlers.on_done then
      state.stream_handlers.on_done(data.output_files or {}, data.usage)
    end
    state.stream_handlers = nil
    state.stream_request_id = nil
    state.stream_buffer = nil
    return
  end

  -- Handle async events (title_updated, etc.)
  if msg_type == 'title_updated' then
    if state.event_handlers.on_title_updated then
      state.event_handlers.on_title_updated(data.chat_id, data.title)
    end
    return
  end

  -- Handle errors
  if msg_type == 'error' then
    if state.stream_handlers and state.stream_request_id == resp_id and state.stream_handlers.on_error then
      state.stream_handlers.on_error(data.message)
      state.stream_handlers = nil
      state.stream_request_id = nil
      state.stream_buffer = nil
    elseif resp_id and state.pending_callbacks[resp_id] then
      local cb = state.pending_callbacks[resp_id]
      state.pending_callbacks[resp_id] = nil
      remove_pending_id(resp_id)
      cb(nil, data.message)
    elseif #state.pending_queue > 0 then
      local legacy_id = table.remove(state.pending_queue, 1)
      local cb = legacy_id and state.pending_callbacks[legacy_id] or nil
      if legacy_id then
        state.pending_callbacks[legacy_id] = nil
      end
      if not cb then
        log.error(data.message)
        return
      end
      cb(nil, data.message)
    else
      log.error(data.message)
    end
    return
  end

  -- Handle regular responses (invoke first pending callback)
  if resp_id and state.pending_callbacks[resp_id] then
    local cb = state.pending_callbacks[resp_id]
    state.pending_callbacks[resp_id] = nil
    remove_pending_id(resp_id)
    cb(data, nil)
  elseif #state.pending_queue > 0 then
    local legacy_id = table.remove(state.pending_queue, 1)
    local cb = legacy_id and state.pending_callbacks[legacy_id] or nil
    if legacy_id then
      state.pending_callbacks[legacy_id] = nil
    end
    if cb then
      cb(data, nil)
    end
  end
end

-- Start the backend process
function M.start()
  if state.job_id then
    return true -- already running
  end

  local bin_path = get_bin_path()
  if vim.fn.executable(bin_path) ~= 1 then
    log.error('Binary not found: ' .. bin_path)
    return false
  end
  local stat = vim.loop.fs_stat(bin_path)
  if stat and stat.mtime then
    local mtime = os.date('%Y-%m-%d %H:%M:%S', stat.mtime.sec)
    log.debug('Starting backend: ' .. bin_path .. ' (mtime ' .. mtime .. ')')
  else
    log.debug('Starting backend: ' .. bin_path)
  end

  -- Build environment, inheriting from parent and ensuring BB7_DEBUG is passed
  local env = vim.fn.environ()
  if env.BB7_DEBUG == '1' then
    log.info('Debug logging enabled')
  end

  state.job_id = vim.fn.jobstart({ bin_path }, {
    env = env,
    on_stdout = function(_, data, _)
      for _, line in ipairs(data) do
        -- Handle partial lines
        if line ~= '' then
          state.response_buffer = state.response_buffer .. line
        end
        -- Try to parse complete lines
        while true do
          local newline_pos = state.response_buffer:find('\n')
          if not newline_pos then
            -- Check if buffer is a complete JSON object
            if state.response_buffer ~= '' and state.response_buffer:sub(1, 1) == '{' then
              local ok = pcall(vim.json.decode, state.response_buffer)
              if ok then
                handle_output(state.response_buffer)
                state.response_buffer = ''
              end
            end
            break
          end
          local complete_line = state.response_buffer:sub(1, newline_pos - 1)
          state.response_buffer = state.response_buffer:sub(newline_pos + 1)
          handle_output(complete_line)
        end
      end
    end,
    on_stderr = function(_, data, _)
      for _, line in ipairs(data) do
        if line ~= '' then
          log.warn('stderr: ' .. line)
        end
      end
    end,
    on_exit = function(_, code, _)
      state.job_id = nil
      state.initialized = false
      state.pending_callbacks = {}
      state.pending_queue = {}
      state.stream_handlers = nil
      state.stream_request_id = nil
      if code ~= 0 then
        log.warn('Process exited with code ' .. code)
      end
    end,
  })

  return state.job_id ~= nil and state.job_id > 0
end

-- Stop the backend process
function M.stop()
  if state.job_id then
    M.send({ action = 'shutdown' })
    vim.fn.jobstop(state.job_id)
    state.job_id = nil
    state.initialized = false
  end
end

-- Send a request to the backend
function M.send(request, callback)
  if not state.job_id then
    if callback then
      callback(nil, 'BB-7 process not running')
    end
    return
  end

  if not request.request_id then
    request.request_id = tostring(state.next_request_id)
    state.next_request_id = state.next_request_id + 1
  end

  local json = vim.json.encode(request) .. '\n'
  vim.fn.chansend(state.job_id, json)

  if callback then
    state.pending_callbacks[request.request_id] = callback
    table.insert(state.pending_queue, request.request_id)
  end
  return request.request_id
end

-- Send a request and handle the response (convenience wrapper)
function M.request(request, callback)
  return M.send(request, callback)
end

-- Send a streaming request (for "send" action)
function M.stream(request, handlers)
  if not state.job_id then
    if handlers and handlers.on_error then
      handlers.on_error('BB-7 process not running')
    end
    return
  end
  state.stream_handlers = handlers
  state.stream_request_id = M.send(request)
  state.stream_buffer = {
    content = '',
    reasoning = '',
    user_message = request.content,
  }
end

-- Initialize the backend with project root
function M.init(project_root, callback)
  if not M.start() then
    if callback then
      callback(nil, 'Failed to start BB-7 process')
    end
    return
  end

  state.project_root = project_root
  M.send({ action = 'init', project_root = project_root }, function(response, err)
    if err then
      if callback then callback(nil, err) end
      return
    end
    state.initialized = true
    if callback then callback(response, nil) end
  end)
end

-- Check if initialized
function M.is_initialized()
  return state.initialized
end

-- Check if running
function M.is_running()
  return state.job_id ~= nil
end

-- Get project root
function M.get_project_root()
  return state.project_root
end

-- Register event handlers for async events (title_updated, etc.)
function M.set_event_handlers(handlers)
  state.event_handlers = handlers or {}
end

-- Request title generation for a chat (async, result comes via title_updated event)
function M.generate_title(chat_id, content, callback)
  M.send({ action = 'generate_title', chat_id = chat_id, content = content }, callback)
end

-- Check if a stream is currently active
function M.has_active_stream()
  return state.stream_handlers ~= nil
end

function M.set_stream_handlers(handlers)
  if not state.stream_request_id then
    return false
  end
  state.stream_handlers = handlers
  return true
end

function M.get_stream_buffer()
  return state.stream_buffer
end

-- Cancel the active streaming request, if any
function M.cancel_active_stream()
  if not state.stream_request_id then
    return
  end
  M.send({ action = 'cancel', target_request_id = state.stream_request_id })
end

-- Get the backend version
function M.get_version(callback)
  M.send({ action = 'version' }, function(response, err)
    if err then
      callback(nil, err)
    else
      callback(response.version, nil)
    end
  end)
end

return M
