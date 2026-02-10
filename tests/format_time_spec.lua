-- Tests for format_time utility function

local utils = require('bb7.utils')

describe('format_time', function()
  it('should return empty string for nil input', function()
    assert.equals('', utils.format_time(nil))
  end)

  it('should parse ISO 8601 timestamp', function()
    assert.equals('2024-01-15 14:30', utils.format_time('2024-01-15T14:30:00'))
    assert.equals('2024-12-31 23:59', utils.format_time('2024-12-31T23:59:59'))
  end)

  it('should handle timestamps with timezone suffix', function()
    -- The pattern doesn't require timezone, so it should match before Z
    assert.equals('2024-01-15 14:30', utils.format_time('2024-01-15T14:30:00Z'))
    assert.equals('2024-01-15 14:30', utils.format_time('2024-01-15T14:30:00+00:00'))
  end)

  it('should handle timestamps with fractional seconds', function()
    -- Pattern should match up to seconds, ignoring fractions
    assert.equals('2024-01-15 14:30', utils.format_time('2024-01-15T14:30:45.123456Z'))
  end)

  it('should preserve leading zeros', function()
    assert.equals('2024-01-05 09:05', utils.format_time('2024-01-05T09:05:03'))
  end)

  it('should return original string for invalid format', function()
    assert.equals('invalid', utils.format_time('invalid'))
    assert.equals('2024-01-15', utils.format_time('2024-01-15'))
    assert.equals('just text', utils.format_time('just text'))
  end)

  it('should handle midnight timestamps', function()
    assert.equals('2024-01-15 00:00', utils.format_time('2024-01-15T00:00:00'))
  end)

  it('should drop seconds from output', function()
    local result = utils.format_time('2024-01-15T14:30:45')
    assert.equals('2024-01-15 14:30', result)
    -- Verify seconds are not in output
    assert.is_nil(result:match(':45'))
  end)
end)
