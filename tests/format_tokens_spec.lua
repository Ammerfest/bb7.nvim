-- Tests for format_tokens utility function

local utils = require('bb7.utils')

describe('format_tokens', function()
  it('should return "0" for nil input', function()
    assert.equals('0', utils.format_tokens(nil))
  end)

  it('should return "0" for zero', function()
    assert.equals('0', utils.format_tokens(0))
  end)

  it('should return string for values under 1000', function()
    assert.equals('1', utils.format_tokens(1))
    assert.equals('500', utils.format_tokens(500))
    assert.equals('999', utils.format_tokens(999))
  end)

  it('should return "1.0k" for exactly 1000', function()
    assert.equals('1.0k', utils.format_tokens(1000))
  end)

  it('should format values over 1000 with k suffix', function()
    assert.equals('1.5k', utils.format_tokens(1500))
    assert.equals('2.0k', utils.format_tokens(2000))
    assert.equals('10.0k', utils.format_tokens(10000))
    assert.equals('100.5k', utils.format_tokens(100500))
  end)

  it('should handle edge case at 1000 boundary', function()
    assert.equals('999', utils.format_tokens(999))
    assert.equals('1.0k', utils.format_tokens(1000))
    assert.equals('1.0k', utils.format_tokens(1001))
  end)

  it('should format with one decimal place', function()
    assert.equals('1.1k', utils.format_tokens(1100))
    assert.equals('1.2k', utils.format_tokens(1234))
    assert.equals('1.9k', utils.format_tokens(1950))
  end)
end)
