-- Model management and picker
local M = {}

local picker = require('bb7.picker')
local client = require('bb7.client')
local log = require('bb7.log')

local state = {
  models = {},           -- All models from OpenRouter
  models_by_id = {},     -- Lookup by ID for quick access
  favorites = {},        -- Set of favorite model IDs
  current_model = nil,   -- Currently selected model ID
  on_model_changed = nil, -- Callback when model changes
  last_success_at = nil, -- Unix timestamp of last successful refresh
  did_initial_refresh = false,
  initialized = false,
}

local state_path = vim.fn.expand('~/.bb7/state.json')

-- Migrate from old config files to new state file (one-time)
local function migrate_old_config()
  -- Check if new state file already exists
  local new_file = io.open(state_path, 'r')
  if new_file then
    new_file:close()
    return -- Already migrated
  end

  local migrated = {}

  -- Migrate favorites from old location
  local old_favorites_path = vim.fn.expand('~/.config/bb7/favorites.json')
  local fav_file = io.open(old_favorites_path, 'r')
  if fav_file then
    local content = fav_file:read('*a')
    fav_file:close()
    local ok, data = pcall(vim.json.decode, content)
    if ok and data and data.models then
      migrated.favorites = data.models
    end
  end

  -- Migrate last_model from old location
  local old_model_path = vim.fn.expand('~/.config/bb7/last_model.json')
  local model_file = io.open(old_model_path, 'r')
  if model_file then
    local content = model_file:read('*a')
    model_file:close()
    local ok, data = pcall(vim.json.decode, content)
    if ok and data and type(data.model) == 'string' then
      migrated.last_model = data.model
    end
  end

  -- Save migrated data if we found anything
  if migrated.favorites or migrated.last_model then
    local dir = vim.fn.fnamemodify(state_path, ':h')
    vim.fn.mkdir(dir, 'p')
    local json = vim.json.encode(migrated)
    local file = io.open(state_path, 'w')
    if file then
      file:write(json)
      file:close()
    end
  end
end

-- Run migration on module load
migrate_old_config()

-- Load global state from disk
local function load_state()
  local file = io.open(state_path, 'r')
  if not file then
    return {}
  end
  local content = file:read('*a')
  file:close()
  local ok, data = pcall(vim.json.decode, content)
  if ok and type(data) == 'table' then
    return data
  end
  return {}
end

-- Save global state to disk
local function save_state(data)
  local dir = vim.fn.fnamemodify(state_path, ':h')
  vim.fn.mkdir(dir, 'p')
  local json = vim.json.encode(data)
  local file = io.open(state_path, 'w')
  if file then
    file:write(json)
    file:close()
  end
end

-- Get a global setting from state
function M.get_setting(key)
  local data = load_state()
  return data[key]
end

-- Set a global setting in state
function M.set_setting(key, value)
  local data = load_state()
  data[key] = value
  save_state(data)
end

-- Format price for display (input/output per 1M tokens)
-- Always shows 3 decimal places for consistent alignment
local function format_price(price_str)
  local price = tonumber(price_str) or 0
  -- Price is per token, multiply by 1M for display
  local per_million = price * 1000000
  return string.format('$%.3f', per_million)
end

-- Format combined price per 1M tokens for easy comparison
-- Uses weighted average: 80% input, 20% output (typical chat usage)
-- 2 decimal places (approximate value indicated by ~ prefix)
local function format_price_per_1m(prompt_price_str, completion_price_str)
  local prompt_price = tonumber(prompt_price_str) or 0
  local completion_price = tonumber(completion_price_str) or 0
  -- Weighted average per token, then multiply by 1M
  local weighted_per_token = (prompt_price * 0.8) + (completion_price * 0.2)
  local per_1m = weighted_per_token * 1000000
  return string.format('~$%.2f', per_1m)
end

-- Format context length for display
local function format_context(ctx)
  if ctx >= 1000000 then
    return string.format('%.1fM', ctx / 1000000)
  elseif ctx >= 1000 then
    return string.format('%dk', ctx / 1000)
  else
    return tostring(ctx)
  end
end

-- Load last used model from state
local function load_last_model()
  local data = load_state()
  if type(data.last_model) == 'string' and data.last_model ~= '' then
    return data.last_model
  end
  return nil
end

-- Save last used model to state
local function save_last_model(model_id)
  if not model_id or model_id == '' then
    return
  end
  local data = load_state()
  data.last_model = model_id
  save_state(data)
end

local function model_exists(model_id)
  if not model_id then
    return false
  end
  for _, model in ipairs(state.models) do
    if model.id == model_id then
      return true
    end
  end
  return false
end

-- Load favorites from state
local function load_favorites()
  local data = load_state()
  state.favorites = {}
  if type(data.favorites) == 'table' then
    for _, id in ipairs(data.favorites) do
      state.favorites[id] = true
    end
  end
end

-- Save favorites to state
local function save_favorites()
  local data = load_state()
  -- Convert set to list
  local list = {}
  for id, _ in pairs(state.favorites) do
    table.insert(list, id)
  end
  table.sort(list)
  data.favorites = list
  save_state(data)
end

-- Format a model for the picker display
local function format_model(model, is_favorite)
  local star = is_favorite and '*' or ' '
  local id = model.id
  local price_in = format_price(model.pricing.prompt)
  local price_out = format_price(model.pricing.completion)
  local price_1m = format_price_per_1m(model.pricing.prompt, model.pricing.completion)
  local ctx = format_context(model.context_length)

  -- Truncate ID if too long
  local max_id_len = 40
  if #id > max_id_len then
    id = id:sub(1, max_id_len - 2) .. '..'
  end

  -- Format: "* model-id                 ~$4.80/1M · $3.000↑ · $15.000↓ ·   200k"
  -- All columns right-aligned except model ID (left-aligned)
  return string.format('%s %-42s %9s/1M · %7s↑ · %8s↓ · %6s',
    star, id, price_1m, price_in, price_out, ctx)
end

-- Format a model for the compact picker display (used with detail pane)
local function format_model_compact(model, is_favorite)
  local star = is_favorite and '*' or ' '
  local id = model.id
  local price_1m = format_price_per_1m(model.pricing.prompt, model.pricing.completion)
  local ctx = format_context(model.context_length)

  local max_id_len = 42
  if #id > max_id_len then
    id = id:sub(1, max_id_len - 2) .. '..'
  end

  -- Format: "* model-id                              ~$4.80   200k"
  return string.format('%s %-44s %7s %6s', star, id, price_1m, ctx)
end

-- Wrap text at word boundaries to fit within max_width, returning up to max_lines lines.
-- Truncates with ".." if text doesn't fit.
local function wrap_text(text, max_width, max_lines)
  if not text or text == '' then
    return {}
  end
  local result = {}
  local remaining = text
  for i = 1, max_lines do
    if #remaining <= max_width then
      table.insert(result, remaining)
      return result
    end
    if i == max_lines then
      -- Last allowed line: truncate with ".."
      local truncated = remaining:sub(1, max_width - 2)
      -- Try to break at last space
      local last_space = truncated:match('.*()%s')
      if last_space and last_space > 1 then
        truncated = remaining:sub(1, last_space - 1)
      end
      table.insert(result, truncated .. '..')
      return result
    end
    -- Find word boundary closest to max_width
    local line = remaining:sub(1, max_width)
    if remaining:sub(max_width + 1, max_width + 1):match('%S') then
      -- We're in the middle of a word, break at last space
      local last_space = line:match('.*()%s')
      if last_space and last_space > 1 then
        table.insert(result, remaining:sub(1, last_space - 1))
        remaining = remaining:sub(last_space):match('^%s*(.*)$')
      else
        -- No space found, hard break
        table.insert(result, line)
        remaining = remaining:sub(max_width + 1)
      end
    else
      table.insert(result, line)
      remaining = remaining:sub(max_width + 1):match('^%s*(.*)$')
    end
  end
  return result
end

-- Format model detail pane content
-- Returns (lines, highlights) for the detail buffer
local function format_model_detail(model, _is_fav)
  local lines = {}
  local highlights = {}
  local content_width = 32

  local function add_line(text)
    table.insert(lines, text)
  end

  local function add_hl(group, col_start, col_end)
    table.insert(highlights, {
      line = #lines - 1,
      group = group,
      col_start = col_start or 0,
      col_end = col_end or -1,
    })
  end

  local function add_row(label, value)
    add_line(string.format('  %-14s%18s', label, value))
    add_hl('Comment', 2, 2 + #label)
  end

  -- Top padding
  add_line('')

  -- Header: 3 content lines + 1 spacer (fixed 4-line block)
  -- 1-line name: name, id, blank, blank
  -- 2-line name: name1, name2, id, blank
  local name = model.name or model.id
  local name_lines = wrap_text(name, content_width, 2)
  for _, nl in ipairs(name_lines) do
    add_line('  ' .. nl)
    add_hl('BB7Bold', 2, 2 + #nl)
  end

  -- ID (truncate if longer than content_width)
  local id = model.id
  if #id > content_width then
    id = id:sub(1, content_width - 2) .. '..'
  end
  add_line('  ' .. id)
  add_hl('Comment', 2, 2 + #id)

  -- Pad to 3 content lines + 1 spacer = line 5 (after top padding on line 1)
  -- Lines so far: 1 (padding) + #name_lines + 1 (id) = 2 + #name_lines
  -- We want 5 lines total before pricing, so add blanks to reach that
  local header_lines = 1 + #name_lines + 1  -- padding + name + id
  for _ = header_lines + 1, 5 do
    add_line('')
  end

  -- Pricing
  add_row('Price in', format_price(model.pricing.prompt) .. '/1M')
  add_row('Price out', format_price(model.pricing.completion) .. '/1M')

  -- Reasoning pricing (only if present and non-zero)
  local reasoning_price = model.pricing.internal_reasoning
  if reasoning_price and reasoning_price ~= '' and tonumber(reasoning_price) and tonumber(reasoning_price) ~= 0 then
    add_row('Reasoning', format_price(reasoning_price) .. '/1M')
  end

  -- Cache read pricing (only if non-zero)
  local cache_read = model.pricing.input_cache_read
  if cache_read and cache_read ~= '' and tonumber(cache_read) and tonumber(cache_read) ~= 0 then
    add_row('Cache read', format_price(cache_read) .. '/1M')
  end

  -- Cache write pricing (only if present and non-zero)
  local cache_write = model.pricing.input_cache_write
  if cache_write and cache_write ~= '' and tonumber(cache_write) and tonumber(cache_write) ~= 0 then
    add_row('Cache write', format_price(cache_write) .. '/1M')
  end

  add_row('BB7 Estimate', format_price_per_1m(model.pricing.prompt, model.pricing.completion) .. '/1M')

  add_line('')

  -- Context & max output
  add_row('Context', format_context(model.context_length))
  local max_out = model.max_completion_tokens
  if max_out and max_out > 0 then
    add_row('Max output', format_context(max_out))
  end

  add_line('')

  -- Capabilities
  add_row('Reasoning', model.supports_reasoning and 'yes' or 'no')
  add_row('Tools', model.supports_tools ~= false and 'yes' or 'no')

  add_line('')

  -- Created date
  local created = model.created
  if created and created > 0 then
    add_row('Created', os.date('%b %Y', created))
  end

  -- Expiration date (only if present)
  local expires = model.expiration_date
  if expires and expires ~= vim.NIL then
    add_row('Expires', expires)
  end

  -- Discount (only if non-zero)
  local discount = model.pricing.discount
  if discount and discount ~= 0 then
    add_row('Discount', math.floor(discount * 100) .. '%')
  end

  return lines, highlights
end

-- Check if a model is favorite
local function is_favorite(model)
  return state.favorites[model.id] == true
end

-- Toggle favorite status
local function toggle_favorite(model)
  if state.favorites[model.id] then
    state.favorites[model.id] = nil
  else
    state.favorites[model.id] = true
  end
  save_favorites()
end

-- Fetch models from backend
function M.refresh(callback)
  client.request({ action = 'get_models' }, function(response, err)
    if err then
      log.error('Failed to fetch models: ' .. tostring(err))
      if callback then callback(false) end
      return
    end

    -- Filter to models that support tool calling (required for write_file)
    local all_models = response.models or {}
    local filtered = {}
    for _, model in ipairs(all_models) do
      if model.supports_tools ~= false then
        table.insert(filtered, model)
      end
    end
    state.models = filtered
    state.last_success_at = os.time()
    state.did_initial_refresh = true

    -- Build lookup table
    state.models_by_id = {}
    for _, model in ipairs(state.models) do
      state.models_by_id[model.id] = model
    end

    -- Ensure current model is valid, otherwise pick a default.
    if #state.models > 0 and not model_exists(state.current_model) then
      local default_model = nil

      -- Prefer last used model if available.
      local last_model = load_last_model()
      if model_exists(last_model) then
        default_model = last_model
      end

      -- Otherwise prefer first favorite, then first model.
      if not default_model then
        for _, model in ipairs(state.models) do
          if state.favorites[model.id] then
            default_model = model.id
            break
          end
        end
      end
      if not default_model then
        default_model = state.models[1].id
      end

      state.current_model = default_model
      if state.on_model_changed then
        state.on_model_changed(default_model)
      end
    end

    if callback then callback(true) end
  end)
end

-- Refresh models if stale (default: 24h)
function M.refresh_if_stale(callback)
  if not state.last_success_at then
    M.refresh(callback)
    return
  end

  local age = os.time() - state.last_success_at
  if age >= 24 * 60 * 60 then
    M.refresh(callback)
  elseif callback then
    callback(true)
  end
end

-- Open the model picker
function M.open_picker()
  if #state.models == 0 then
    log.error('No models loaded. Open BB7 to load models.')
    return
  end

  picker.open({
    items = state.models,
    title = 'Select Model',
    format_item = format_model,
    format_item_compact = format_model_compact,
    format_detail = format_model_detail,
    get_filter_text = function(model)
      -- Only match against ID to avoid false positives from name
      return model.id
    end,
    get_id = function(model)
      return model.id
    end,
    selected_id = state.current_model,
    is_favorite = is_favorite,
    on_toggle_favorite = toggle_favorite,
    on_select = function(model)
      M.set_current(model.id, { persist = true })
    end,
  })
end

-- Get current model ID
function M.get_current()
  return state.current_model
end

-- Set current model
function M.set_current(model_id, opts)
  opts = opts or {}
  state.current_model = model_id
  if opts.persist ~= false then
    save_last_model(model_id)
  end
  if opts.notify ~= false and state.on_model_changed then
    state.on_model_changed(model_id)
  end
end

-- Persist the current model (used when sending a message)
function M.persist_current()
  save_last_model(state.current_model)
end

-- Set callback for model changes
function M.set_callbacks(callbacks)
  state.on_model_changed = callbacks.on_model_changed
end

-- Get model count
function M.get_count()
  return #state.models
end

-- Get full model info by ID (defaults to current model)
function M.get_model_info(model_id)
  return state.models_by_id[model_id or state.current_model]
end

-- Check if a model supports reasoning
function M.supports_reasoning(model_id)
  if not model_id then return false end
  local model = state.models_by_id[model_id]
  if not model then return false end
  return model.supports_reasoning == true
end

function M.get_last_refresh()
  return state.last_success_at
end

function M.did_refresh_once()
  return state.did_initial_refresh
end

-- Initialize
function M.init()
  if state.initialized then
    return
  end
  load_favorites()
  local last_model = load_last_model()
  if last_model then
    state.current_model = last_model
  end
  state.initialized = true
end

return M
