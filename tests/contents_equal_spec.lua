-- Tests for contents_equal utility function (used by is_out_of_sync)

local utils = require('bb7.utils')

describe('contents_equal', function()
  it('should return true for identical content', function()
    assert.is_true(utils.contents_equal('hello', 'hello'))
    assert.is_true(utils.contents_equal('line1\nline2', 'line1\nline2'))
  end)

  it('should return false for different content', function()
    assert.is_false(utils.contents_equal('hello', 'world'))
    assert.is_false(utils.contents_equal('line1', 'line2'))
  end)

  it('should handle empty strings', function()
    assert.is_true(utils.contents_equal('', ''))
    assert.is_false(utils.contents_equal('', 'content'))
    assert.is_false(utils.contents_equal('content', ''))
  end)

  it('should handle nil values', function()
    assert.is_true(utils.contents_equal(nil, nil))
    assert.is_true(utils.contents_equal(nil, ''))
    assert.is_true(utils.contents_equal('', nil))
    assert.is_false(utils.contents_equal(nil, 'content'))
    assert.is_false(utils.contents_equal('content', nil))
  end)

  it('should normalize \\r\\n to \\n', function()
    assert.is_true(utils.contents_equal('line1\r\nline2', 'line1\nline2'))
    assert.is_true(utils.contents_equal('line1\nline2', 'line1\r\nline2'))
    assert.is_true(utils.contents_equal('a\r\nb\r\nc', 'a\nb\nc'))
  end)

  it('should normalize \\r to \\n', function()
    assert.is_true(utils.contents_equal('line1\rline2', 'line1\nline2'))
    assert.is_true(utils.contents_equal('line1\nline2', 'line1\rline2'))
  end)

  it('should handle mixed line endings', function()
    assert.is_true(utils.contents_equal('a\r\nb\rc\n', 'a\nb\nc\n'))
    assert.is_true(utils.contents_equal('a\nb\nc', 'a\r\nb\rc'))
  end)

  it('should detect actual content differences with different line endings', function()
    -- Content differs, but both use different line endings
    assert.is_false(utils.contents_equal('hello\r\nworld', 'hello\nplanet'))
    assert.is_false(utils.contents_equal('line1\nline2', 'line1\r\nline3'))
  end)
end)

describe('normalize_line_endings', function()
  it('should convert \\r\\n to \\n', function()
    assert.equals('a\nb\nc', utils.normalize_line_endings('a\r\nb\r\nc'))
  end)

  it('should convert \\r to \\n', function()
    assert.equals('a\nb\nc', utils.normalize_line_endings('a\rb\rc'))
  end)

  it('should handle mixed line endings', function()
    assert.equals('a\nb\nc\nd', utils.normalize_line_endings('a\r\nb\rc\nd'))
  end)

  it('should leave \\n unchanged', function()
    assert.equals('a\nb\nc', utils.normalize_line_endings('a\nb\nc'))
  end)

  it('should handle nil input', function()
    assert.equals('', utils.normalize_line_endings(nil))
  end)

  it('should handle empty string', function()
    assert.equals('', utils.normalize_line_endings(''))
  end)
end)
