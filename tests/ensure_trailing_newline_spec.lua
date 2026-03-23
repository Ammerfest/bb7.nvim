-- Tests for ensure_trailing_newline utility function

local utils = require('bb7.utils')

describe('ensure_trailing_newline', function()
  it('should add newline to content missing one', function()
    assert.equals('hello\n', utils.ensure_trailing_newline('hello'))
    assert.equals('line1\nline2\n', utils.ensure_trailing_newline('line1\nline2'))
  end)

  it('should not double a trailing newline', function()
    assert.equals('hello\n', utils.ensure_trailing_newline('hello\n'))
    assert.equals('line1\nline2\n', utils.ensure_trailing_newline('line1\nline2\n'))
  end)

  it('should return empty string for empty input', function()
    assert.equals('', utils.ensure_trailing_newline(''))
  end)

  it('should return empty string for nil input', function()
    assert.equals('', utils.ensure_trailing_newline(nil))
  end)

  it('should handle single newline', function()
    assert.equals('\n', utils.ensure_trailing_newline('\n'))
  end)

  it('should preserve multiple trailing newlines', function()
    assert.equals('hello\n\n', utils.ensure_trailing_newline('hello\n\n'))
  end)
end)
