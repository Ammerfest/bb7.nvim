-- Mock chats for preview testing
local M = {}

function M.get_mock_chat()
  return {
    id = 'mock-chat-001',
    name = 'Mock Chat for Testing (Long)',
    messages = {
      -- User message 1
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'Can you help me refactor this function to be more efficient? I have a nested loop that iterates over two lists and compares every element, which is really slow when the lists get large.' },
        },
      },
      -- Assistant response 1
      {
        role = 'assistant',
        parts = {
          {
            type = 'thinking',
            content = 'Let me analyze this request step by step. The user has described a classic O(n²) problem where they are comparing every element in one list against every element in another list.\n\nThe key insight here is that we can use a hash-based data structure to reduce the time complexity from O(n²) to O(n).',
          },
          {
            type = 'text',
            content = 'I\'ve analyzed your function and found a way to improve it significantly. By using a hash map, we can reduce this to O(n).',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/utils.py', added = false },
        },
      },
      -- User message 2
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'That looks great! Can you also add comprehensive error handling?' },
        },
      },
      -- Assistant response 2
      {
        role = 'assistant',
        parts = {
          {
            type = 'thinking',
            content = 'The user wants robust error handling which is crucial for production code.\n\nInput validation is the first line of defense. We need to check for None/null inputs before attempting any operations.',
          },
          {
            type = 'text',
            content = 'I\'ve added comprehensive error handling to the function with multiple layers of validation.',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/utils.py', added = false },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'tests/test_utils.py', added = true },
        },
      },
      -- User message 3
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'Now I need to add logging. Can you add proper logging with different log levels?' },
        },
      },
      -- Assistant response 3
      {
        role = 'assistant',
        parts = {
          {
            type = 'thinking',
            content = 'Logging is essential for debugging and monitoring production systems.\n\nWe should use DEBUG for detailed diagnostic information, INFO for general operational messages, WARNING for unexpected situations that aren\'t errors, and ERROR for actual failures.',
          },
          {
            type = 'text',
            content = 'I\'ve added structured logging throughout the function using Python\'s logging module with appropriate log levels.',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/utils.py', added = false },
        },
      },
      -- User message 4
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'Can you add type hints to make the code more maintainable?' },
        },
      },
      -- Assistant response 4
      {
        role = 'assistant',
        parts = {
          {
            type = 'text',
            content = 'I\'ve added comprehensive type hints using Python\'s typing module. This includes:\n\n- Function parameter types\n- Return type annotations\n- Generic types for collections\n- Optional types where appropriate',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/utils.py', added = false },
        },
      },
      -- User message 5
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'Now let\'s add unit tests for all the edge cases we discussed.' },
        },
      },
      -- Assistant response 5
      {
        role = 'assistant',
        parts = {
          {
            type = 'thinking',
            content = 'We need comprehensive test coverage for:\n\n1. Normal operation with valid inputs\n2. Empty list handling\n3. None/null input handling\n4. Invalid type handling\n5. Large dataset performance\n6. Error message correctness',
          },
          {
            type = 'text',
            content = 'I\'ve created a comprehensive test suite using pytest with parametrized tests for edge cases and performance benchmarks.',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'tests/test_utils.py', added = false },
        },
      },
      -- User message 6
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'Great! Can we also add documentation with docstrings?' },
        },
      },
      -- Assistant response 6
      {
        role = 'assistant',
        parts = {
          {
            type = 'text',
            content = 'I\'ve added Google-style docstrings to all functions including:\n\n- Description of what the function does\n- Args section with parameter descriptions\n- Returns section with return value description\n- Raises section listing possible exceptions\n- Examples section with usage examples',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/utils.py', added = false },
        },
      },
      -- User message 7
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'Finally, can you add a CLI interface so this can be run from the command line?' },
        },
      },
      -- Assistant response 7
      {
        role = 'assistant',
        parts = {
          {
            type = 'thinking',
            content = 'For a CLI interface, we should use argparse which is the standard library solution.\n\nWe need:\n- Input file argument\n- Output file argument (optional, default to stdout)\n- Verbosity flag for log level control\n- Version flag\n- Help text',
          },
          {
            type = 'text',
            content = 'I\'ve added a full CLI interface using argparse with support for input/output files, verbosity control, and proper exit codes.',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/cli.py', added = true },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/__main__.py', added = true },
        },
      },
      -- User message 8
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'Perfect! One more thing - can you add a configuration file option?' },
        },
      },
      -- Assistant response 8
      {
        role = 'assistant',
        parts = {
          {
            type = 'text',
            content = 'I\'ve added YAML configuration file support with:\n\n- Default config location at ~/.config/myapp/config.yaml\n- Environment variable override (MYAPP_CONFIG)\n- Command line --config flag\n- Sensible defaults when no config exists',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/config.py', added = true },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'config.example.yaml', added = true },
        },
      },
    },
  }
end

function M.get_format_test_chat()
  return {
    id = 'format-test-001',
    name = 'Format Test Chat',
    messages = {
      -- Message 1: Multiple consecutive context adds (should be grouped)
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'src/main.py' },
        },
      },
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'src/utils.py' },
        },
      },
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'src/config.py' },
        },
      },
      -- Message 2: User text with model change (should have empty line before)
      {
        role = 'user',
        model = 'anthropic/claude-sonnet-4',
        parts = {
          { type = 'text', content = 'Please review these files and suggest improvements for error handling.' },
        },
      },
      -- Message 3: Assistant with thinking + text + multiple file writes
      {
        role = 'assistant',
        parts = {
          {
            type = 'thinking',
            content = 'Let me analyze the code structure.\n\nThe main.py file lacks proper exception handling.\nThe utils.py could benefit from input validation.\nThe config.py should handle missing config files gracefully.',
          },
          {
            type = 'text',
            content = 'I\'ve reviewed your files and found several areas for improvement. Here are my changes:',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/main.py', added = false },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/utils.py', added = false },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'src/config.py', added = false },
        },
      },
      -- Message 4: User applies files (consecutive actions)
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserApplyFile', path = 'src/main.py' },
        },
      },
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserApplyFile', path = 'src/utils.py' },
        },
      },
      -- Message 5: User adds more context
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'tests/test_main.py' },
        },
      },
      -- Message 6: User text (should have empty line before, after actions)
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'Great! Now please add unit tests for the error handling.' },
        },
      },
      -- Message 7: Assistant response with code blocks
      {
        role = 'assistant',
        parts = {
          {
            type = 'text',
            content = 'I\'ll add **comprehensive unit tests**. Here\'s an example:\n\n```lua\nlocal function test_error_handling()\n  local ok, err = pcall(function()\n    error("test error")\n  end)\n  assert(not ok)\n  assert(err:match("test error"))\nend\n```\n\nAnd here\'s a **bash script** to run them:\n\n```bash\n#!/bin/bash\nfor file in tests/*.lua; do\n  echo "Running $file"\n  lua "$file"\ndone\n```\n\nThe tests will cover:\n- **Invalid input types** (bold)\n- *Missing files* (italic)\n- __Network errors__ (underline)\n\nYou can also combine them: **bold**, *italic*, and __underline__ in the same line.',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'tests/test_main.py', added = false },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'tests/test_utils.py', added = true },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'tests/test_config.py', added = true },
        },
      },
      -- Message 8: User rejects one file
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserRejectOutput', path = 'tests/test_config.py' },
        },
      },
      -- Message 9: User text feedback
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'The config tests are too verbose. Can you simplify them?' },
        },
      },
      -- Message 10: Assistant with thinking
      {
        role = 'assistant',
        parts = {
          {
            type = 'thinking',
            content = 'The user found the config tests too verbose. I should:\n1. Remove redundant test cases\n2. Use parametrized tests\n3. Consolidate setup code',
          },
          {
            type = 'text',
            content = 'I\'ve simplified the config tests using parametrized testing.',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'tests/test_config.py', added = true },
        },
      },
      -- Message 11: More file additions
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'docs/README.md' },
        },
      },
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'docs/API.md' },
        },
      },
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'docs/CONTRIBUTING.md' },
        },
      },
      -- Message 12: Model change + user text
      {
        role = 'user',
        model = 'openai/gpt-4o',
        parts = {
          { type = 'text', content = 'Switching to GPT-4. Can you improve the documentation?' },
        },
      },
      -- Message 13: Assistant response
      {
        role = 'assistant',
        parts = {
          {
            type = 'text',
            content = 'I\'ll enhance all the documentation files with better examples and clearer explanations.',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'docs/README.md', added = false },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'docs/API.md', added = false },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'docs/CONTRIBUTING.md', added = false },
        },
      },
      -- Message 14: User applies all
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserApplyFile', path = 'docs/README.md' },
        },
      },
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserApplyFile', path = 'docs/API.md' },
        },
      },
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserApplyFile', path = 'docs/CONTRIBUTING.md' },
        },
      },
      -- Message 15: Markup stress test
      {
        role = 'user',
        parts = {
          { type = 'text', content = 'Show me all the formatting edge cases.' },
        },
      },
      -- Message 16: Markup stress test response
      {
        role = 'assistant',
        parts = {
          {
            type = 'text',
            content = '**Bold alone** and *italic alone* and __underline alone__.\n\nMixed on one line: **bold** then *italic* then __underline__ done.\n\nAdjacent: **bold***italic*__underline__ no gaps.\n\nList with formatting:\n- **bold item**\n- *italic item*\n- __underline item__\n- item with **bold** in the middle\n- item with *italic* in the middle\n- item with __underline__ in the middle\n- **bold** and *italic* and __underline__ all in one item\n\nStar-prefixed list (markdown unordered):\n* plain item\n* **bold item**\n* *italic item*\n* item with **bold** word\n* item with *italic* word\n* **bold** and *italic* together\n\nUnclosed markers: **bold without close\nUnclosed italic: *italic without close\nUnclosed underline: __underline without close\n\nEmpty markers: **** and ** and __ __\n\nMultiple on one word: **bold1** normal **bold2**\n\nCode block should not format:\n```\n**not bold** and *not italic* and __not underline__\n```\n\nBack to formatting: **this should be bold** after code block.',
          },
        },
      },
    },
  }
end

function M.get_screenshot_chat()
  return {
    id = 'screenshot-chat-003',
    name = 'Understanding signals vs direct refs',
    created = '2026-02-09T14:32:00Z',
    instructions_info = {
      global_exists = true,
      project_exists = true,
    },
    messages = {
      -- User adds context files
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'player_controller.gd' },
        },
      },
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'main_scene.tscn' },
        },
      },
      {
        role = 'user',
        parts = {
          { type = 'context_event', action = 'UserAddFile', path = 'signal_bus.gd' },
        },
      },
      -- User message 1
      {
        role = 'user',
        model = 'claude-sonnet-4-5-20250929',
        parts = {
          { type = 'text', content = 'My player controller calls get_node() on the health bar, score label, and inventory panel directly. It works but feels wrong \u{2014} everything breaks when I rearrange the scene tree. What\'s the Godot way to handle this?' },
        },
      },
      -- Assistant response 1
      {
        role = 'assistant',
        parts = {
          {
            type = 'thinking',
            content = 'Classic beginner coupling problem. The user is using direct node references which creates tight coupling between the player controller and the UI nodes. In Godot, the idiomatic solution is the signal pattern \u{2014} either direct signals or an autoloaded signal bus for cross-scene communication.',
          },
          {
            type = 'text',
            content = 'You\'ve hit the most common Godot architecture mistake \u{2014} tight coupling through direct node paths.\n\nThe problem with `get_node("../UI/HealthBar")` is that it creates an **implicit dependency** on your scene tree layout. Move one node and everything breaks silently.\n\nGodot\'s answer is **signals**. There are two patterns:\n\n1. **Direct signals** \u{2014} for parent-child communication within a scene\n2. **Signal bus** (autoload) \u{2014} for cross-scene communication\n\nFor your case, a signal bus is the right call. Your player emits `health_changed`, `score_changed`, etc., and any UI node connects to those signals independently.\n\nI\'ve rewritten your `signal_bus.gd` as an autoload that acts as a central event hub:',
          },
          { type = 'context_event', action = 'AssistantWriteFile', path = 'signal_bus.gd', added = false },
        },
        usage = {
          prompt_tokens = 3847,
          completion_tokens = 1024,
          cached_tokens = 1200,
          cost = 0.0214,
        },
      },
    },
  }
end

return M
