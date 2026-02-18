# File Changes and Conflict Resolution

This document describes how BB-7 handles LLM-generated file changes, conflicts, and retries.

## Supported File Edit Modes

BB-7 supports one write tool and three `edit_file` schemas.
`diff_mode` selects which `edit_file` schema is exposed.

### User-Facing File Tools (Current)

| Tool/Schema | Purpose | Status |
|-------------|---------|--------|
| `write_file(path, content)` | Create new files or full-file rewrites | Supported |
| `edit_file(edits[])` (search/replace multi) | Batched search/replace edits across one or more files | **Default** |
| `edit_file(path, old_string, new_string, replace_all?)` (search/replace single) | Single search/replace edit per call | Supported |
| `edit_file(path, changes[])` (anchored) | Region-based anchor edits | **Experimental** |

Note:
- The tool name is always `edit_file`; only the schema changes by mode.

### Configuration

```lua
require('bb7').setup({
  diff_mode = "search_replace_multi",  -- default
  -- diff_mode = "search_replace",     -- one search/replace edit per call
  -- diff_mode = "anchored",           -- experimental anchor schema
  -- diff_mode = "off",                -- write_file only
})
```

| `diff_mode` | Tools exposed |
|-------------|---------------|
| `"search_replace_multi"` | `write_file` + `edit_file(edits[])` |
| `"search_replace"` | `write_file` + `edit_file(path, old_string, new_string, replace_all?)` |
| `"anchored"` | `write_file` + `edit_file(path, changes[])` |
| `"off"` | `write_file` only |

## Tool Semantics

### `write_file`

Use for:
- New files
- True full rewrites

Rules:
- Must contain complete file content.
- Do not call it multiple times for the same path in one response.
- Do not mix `write_file` and `edit_file` for the same path in one response.

### `edit_file` (Search/Replace Multi, Default)

Schema:

```json
{
  "edits": [
    {
      "path": "src/main.go",
      "file_id": "optional-id",
      "old_string": "...",
      "new_string": "...",
      "replace_all": false
    }
  ]
}
```

Properties:
- All edits are sent in one tool call.
- Edits apply sequentially; later edits see earlier results.
- Can edit multiple files in one call.

Strengths:
- Best model reliability in benchmark.
- Fits models that prefer one tool call per response.

Tradeoffs:
- Still requires reproducing `old_string` from the current file state.

### `edit_file` (Search/Replace Single)

Schema:

```json
{
  "path": "src/main.go",
  "file_id": "optional-id",
  "old_string": "...",
  "new_string": "...",
  "replace_all": false
}
```

Strengths:
- Very simple mental model.

Tradeoffs:
- Multi-edit tasks need multiple tool calls; some models stop after first call.

### `edit_file` (Anchored, Experimental)

Schema:

```json
{
  "path": "src/main.go",
  "file_id": "optional-id",
  "changes": [
    {
      "start": ["line 1", "line 2"],
      "end": ["line 3"],
      "content": ["replacement line"]
    }
  ]
}
```

Semantics:
- `start` is required.
- `end` is optional. If omitted, region is exactly `start`.
- `content` fully replaces the matched region.

Strengths:
- Efficient for large region edits without copying large `old_string` blocks.

Tradeoffs:
- Anchor selection/uniqueness is cognitively harder for models.
- More frequent failures on weaker models.

## Base Version Resolution and `file_id`

When applying edits for a path, BB-7 resolves base content in this order:
1. Pending writes from earlier tool calls in the same response
2. Output file (`status=pending_output`)
3. Context snapshot (`status=original`)

If `file_id` is provided, BB-7 validates it against the resolved base version.
A mismatch returns a `diff_error` (no partial write commit).

Why this matters:
- The same path can appear as both original and pending output.
- `file_id` disambiguates exactly which version the model intends to edit.

## Atomic Write Behavior

All file writes from one assistant response are buffered first.

- On success: all pending writes commit together.
- On diff failure: all pending writes are discarded.

This is all-or-nothing per response.

## Error Handling and Retry UX

On diff errors (e.g., `old_string not found`, not unique match, `file_id mismatch`):

1. Assistant text/thinking is preserved.
2. Pending file writes are discarded.
3. A `diff_error` is emitted.
4. Retry context is attached in the next request (`@retry_context`) so the model can repair tool calls.

Retry guidance now explicitly tells models to:
- Use writable `@file ... mode=rw status=pending_output` content as base.
- Use the matching writable `file_id` when path is ambiguous.

## Status Indicators

| Status | Meaning |
|--------|---------|
| (blank) | File unchanged (in context, no output) |
| `M` | Modified by LLM (output differs from context) |
| `A` | Added by LLM (new file, not in context) |
| `~` | Out of sync (local differs from context, no LLM output) |
| `~M` | Conflict (local differs from context and LLM has output) |

## Conflict Detection

A conflict (`~M`) occurs when:
1. LLM has pending output for a file, and
2. Local file/buffer diverged from the context snapshot.

Resolution options:
- Apply LLM output
- Discard LLM output
- Manually merge via diff

## Benchmark Summary (2026-02-17)

### Anchored (experimental)

| Model | Score |
|-------|-------|
| google/gemini-2.5-pro | 6/6 |
| anthropic/claude-opus-4.5 | 3/6 |
| anthropic/claude-sonnet-4 | 2/6 |
| anthropic/claude-sonnet-4.5 | 2/6 |
| z-ai/glm-5 | 1/6 |
| openai/gpt-5.2-codex | 0/6 |

Common failures:
- Non-unique anchors
- Anchoring against imagined post-edit state
- Reorder operations needing precise delete+insert anchors

### Search/Replace Single

| Model | Score | Notes |
|-------|-------|-------|
| z-ai/glm-5 | 6/6 | |
| anthropic/claude-sonnet-4.5 | 5/6 | Failed reorder |
| google/gemini-2.5-pro | 5/6 | API issue in one run |
| anthropic/claude-sonnet-4 | 2/6 | |
| openai/gpt-5.2-codex | 2/6 | |

Main limitation:
- Some models emit only one tool call per response.

### Search/Replace Multi (default)

| Model | Score |
|-------|-------|
| anthropic/claude-opus-4.5 | 7/7 |
| anthropic/claude-sonnet-4.5 | 7/7* |
| anthropic/claude-sonnet-4 | 7/7* |
| openai/gpt-5.2-codex | 7/7* |
| z-ai/glm-5 | 7/7* |
| google/gemini-2.5-pro | API issues (intermittent) |

\* Tests 1-6 as suite; test 7 confirmed individually.

## Matching Pipelines

### Search/Replace Modes

Matching pipeline:
1. Exact line match
2. Trailing whitespace trimmed
3. Full trim with indentation adjustment
4. Boundary prefix matching (first/last lines may be truncated prefixes)
5. Raw substring fallback

### Anchored Mode

Matching pipeline:
1. Exact
2. Trim trailing whitespace
3. Trim all surrounding whitespace

## Approach Comparison

| Aspect | SR Multi (Default) | SR Single | Anchored (Experimental) |
|--------|--------------------|-----------|--------------------------|
| Reliability | Highest in benchmark | Medium | Lowest on most models |
| Calls per complex task | One | Multiple | One |
| Old text reproduction | Required | Required | Not required |
| Multi-file in one call | Yes | No | No |
| Cognitive load for model | Low | Low | High |

## References

- OpenCode patch implementation: `reference/opencode/packages/opencode/src/patch/`
- OpenCode edit tool: `reference/opencode/packages/opencode/src/tool/edit.ts`
- Aider: search/replace blocks
- Claude Code: search/replace style markers
