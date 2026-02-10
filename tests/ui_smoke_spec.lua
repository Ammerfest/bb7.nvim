local function stub_client()
  return {
    is_running = function() return true end,
    is_initialized = function() return true end,
    init = function(_, cb)
      if cb then cb({}, nil) end
    end,
    request = function(req, cb)
      if not cb then return end
      if req.action == 'chat_list' then
        cb({ chats = {} }, nil)
      elseif req.action == 'chat_get' then
        cb(nil, 'No active chat')
      elseif req.action == 'get_balance' then
        cb({ total_credits = 0, total_usage = 0 }, nil)
      elseif req.action == 'get_models' then
        cb({ models = {} }, nil)
      elseif req.action == 'get_file_statuses' then
        cb({ files = {} }, nil)
      elseif req.action == 'estimate_tokens' then
        cb({ total = 0, potential_savings = 0 }, nil)
      else
        cb({}, nil)
      end
    end,
    set_event_handlers = function() end,
    get_version = function(cb)
      if cb then cb('test-version', nil) end
    end,
    generate_title = function(_, _, cb)
      if cb then cb({}, nil) end
    end,
    has_active_stream = function() return false end,
  }
end

local function reset_modules()
  for _, mod in ipairs({
    'bb7',
    'bb7.ui',
    'bb7.client',
    'bb7.panes.chats',
    'bb7.panes.context',
    'bb7.panes.provider',
    'bb7.panes.preview',
    'bb7.panes.input',
    'bb7.models',
    'bb7.picker',
  }) do
    package.loaded[mod] = nil
  end
end

describe('ui smoke', function()
  before_each(function()
    reset_modules()
    package.loaded['bb7.client'] = stub_client()
    require('bb7').setup({})
  end)

  after_each(function()
    local ok, ui = pcall(require, 'bb7.ui')
    if ok and ui.is_open() then
      ui.close()
    end
    reset_modules()
  end)

  it('opens ui and creates panes', function()
    local ui = require('bb7.ui')
    ui.open()
    vim.wait(50)
    assert.is_true(ui.is_open())

    local win_count = #vim.api.nvim_list_wins()
    assert.is_true(win_count >= 6)
  end)

  it('preview hints omit mode prefixes', function()
    local ui = require('bb7.ui')
    ui.open()
    vim.wait(50)

    local preview = require('bb7.panes.preview')
    local hints = preview.get_hints()
    assert.is_not_nil(hints:match('Next anchor'))
    assert.is_nil(hints:match('%[Chat%]'))
  end)
end)
