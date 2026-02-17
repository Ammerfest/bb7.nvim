# File Changes and Conflict Resolution

This document describes how BB-7 handles LLM-generated file changes and conflicts.

## V1: Complete Files (Current)

In V1, the LLM always outputs complete files via the `write_file` tool. This is simple and predictable:

- **Output**: Complete file content goes to `.bb7/chats/{id}/output/`
- **Context**: Original file snapshot in `.bb7/chats/{id}/context/`
- **Local**: Current file in the project (may have unsaved buffer changes)

### Status Indicators

| Status | Meaning |
|--------|---------|
| (blank) | File unchanged (in context, no output) |
| `M` | Modified by LLM (output differs from context) |
| `A` | Added by LLM (new file, not in context) |
| `~` | Out of sync (local differs from context, no LLM output) |
| `~M` | Conflict (local differs from context AND LLM has output) |

### Conflict Detection

A conflict (`~M`) occurs when:
1. LLM modified a file (created output)
2. User also modified the file locally (buffer differs from context snapshot)

Detection: Compare current buffer content (not disk) against context snapshot. This catches unsaved changes.

### Recommended Workflow

To avoid conflicts, work from a **clean git state**:
1. Commit or stash local changes before adding files to context
2. Either edit locally OR prompt the LLM to change files - not both at once
3. Apply or discard LLM changes before making further local edits

### Conflict Resolution

When `~M` occurs (user modified locally while LLM also modified):

| Action | Key | Effect |
|--------|-----|--------|
| **Apply LLM output** | `p` | Overwrite local with LLM version |
| **Discard LLM output** | `x` | Keep local, remove LLM's changes |
| **Discard local changes** | git | Use `git checkout` then `u` to resync context |
| **Manual merge** | `:BB7DiffLocal` | Open vim diff for cherry-picking |

**Diff view** (`gd`): Shows local file → LLM output (- lines are local, + lines are LLM's version).

### Preview Behavior

| Status | Preview shows |
|--------|---------------|
| `M`, `A` | Output file (LLM's changes) |
| `~` | Context file with warning |
| `~M` | Output file with conflict warning |
| (blank) | Context file |

## V2: Region-Based Diffs (Planned)

Complete files become impractical for large files. V2 adds a `modify_file` tool that specifies only the changed regions.

### Design Principles

1. **One mental model**: A change locates a region of lines in the file and replaces it. `content` is exactly what will be in the file at that location.
2. **Anchors are small**: 1-4 lines to locate the region. The LLM never reproduces large blocks of old code.
3. **No flags**: Anchors are always part of the replaced region. Include them in `content` to keep them, modify them in `content` to change them, omit them to delete them.
4. **Fail fast**: If anchors don't match, return an error. The LLM retries or falls back to `write_file`.

### Configuration

```lua
require('bb7').setup({
  diff_mode = true,  -- default: true; set false to use only write_file
})
```

When `diff_mode = false`, the backend does not expose the `modify_file` tool. The LLM only sees `write_file` (V1 behavior). This is a safety valve in case diffs prove unreliable with certain models or workflows.

### Tools

When `diff_mode = true`, two tools are available:

- **`write_file(path, content)`** — New files and full rewrites.
- **`modify_file(path, changes[])`** — Everything else.

### modify_file Format

```
modify_file(path, changes):
  changes: [
    {
      start: ["line1", "line2"],    // 1-4 lines, matched in file
      end:   ["line3", "line4"],    // 1-4 lines, matched in file (optional)
      content: ["new1", "new2"]     // exact replacement for the matched region
    }
  ]
```

- **`start`**: 1-4 lines that mark the beginning of the region.
- **`end`**: 1-4 lines that mark the end of the region. Optional — when omitted, the region is exactly the `start` lines.
- **`content`**: The complete replacement. What you put here is exactly what will be in the file.

### Use Cases

**Small surgical edit** (end omitted — region is just the start lines):
```json
{
  "start": ["    print('hello')"],
  "content": ["    print('hello world')"]
}
```

**Insert code after a line**:
```json
{
  "start": ["import os"],
  "content": ["import os", "import sys"]
}
```
The anchor is preserved by including it in content.

**Replace function body, keep signature**:
```json
{
  "start": ["def hello():"],
  "end": ["def goodbye():"],
  "content": ["def hello():", "    print('new body')", "", "def goodbye():"]
}
```
Both anchors repeated in content — preserved.

**Change function signature + body**:
```json
{
  "start": ["def hello():"],
  "end": ["def goodbye():"],
  "content": ["def hello(name):", "    print(name)", "", "def goodbye():"]
}
```
Top anchor changed in content, bottom preserved. Same format, no special flag.

**Delete a block**:
```json
{
  "start": ["# BEGIN DEBUG"],
  "end": ["# END DEBUG"],
  "content": []
}
```

**Multiple changes in one file**:
```json
{
  "path": "src/main.py",
  "changes": [
    {
      "start": ["import os"],
      "content": ["import os", "import sys"]
    },
    {
      "start": ["def hello():"],
      "end": ["def goodbye():"],
      "content": ["def hello(name):", "    print(name)", "", "def goodbye():"]
    }
  ]
}
```

### Application Algorithm

1. Parse all changes for the file.
2. For each change, locate the region:
   - Find `start` lines in the file (consecutive match).
   - If `end` is present, find `end` lines after `start` (consecutive match).
   - Region = first line of `start` through last line of `end` (or last line of `start` if no `end`).
3. Verify no regions overlap. Error if they do.
4. Apply all changes bottom-to-top (so line numbers don't shift).

### Anchor Matching

Three-pass matching per line:

1. **Exact match**: `file_line == anchor_line`
2. **Trailing whitespace trimmed**: `trimRight(file_line) == trimRight(anchor_line)`
3. **All whitespace trimmed**: `trimSpace(file_line) == trimSpace(anchor_line)`

Pass 3 tolerates leading indentation errors (models occasionally miscount nesting depth). No further fuzzy matching (no Levenshtein, no unicode normalization). If anchors don't match after these three passes, the change fails.

**Uniqueness**: Multi-line anchors (2-4 lines) make false matches extremely unlikely even with trailing whitespace trimmed. If ambiguity occurs, error and let the LLM retry with more anchor lines.

### Error Handling and Retries

When a change fails (anchor not found, ambiguous match, overlapping regions):

1. **No changes are applied** (atomic — all or nothing per file).
2. The error is returned to the LLM as a tool result error with specific details:
   - Which file and which change failed
   - Why: `"anchor not found"`, `"anchor not unique (lines 42, 187)"`, `"regions overlap"`
   - The broken diff is included so the LLM can fix it without re-reasoning
3. No automatic retry loop — the user controls the retry.

#### Atomic File Writes

All file writes in a single response are buffered and committed together:

- During streaming, file writes go to a pending buffer (not to disk).
- **On success** (no diff errors): all pending writes are committed to output.
- **On diff failure**: all pending writes are discarded. No files are written.
- This is "all or nothing" per response — even if some `write_file` calls succeeded, a single `modify_file` failure discards everything.

#### Retry UX

When a diff fails:

1. **Assistant text is preserved**: The assistant's text and thinking content is saved as a normal message (without file write events). It may contain valuable explanations.
2. **Non-persistent warning**: A user-friendly error message appears at the bottom of the chat preview. Raw error details are only in debug logs (`BB7_DEBUG=1`).
3. **Input prepopulated**: The input field is prepopulated with "Please retry the file changes."
4. **Hidden retry context**: The frontend stores the error details and failed tool calls. When the user sends the retry message, this context is sent as a separate `retry_context` field — not part of the saved message. The backend injects it as a `@retry_context` block in the LLM request. The chat history stays clean.
5. The user can edit the prepopulated text to add context, or just send as-is.

#### User Actions on Diff Failure

| Action | Key | Effect |
|--------|-----|--------|
| **Retry** | `<CR>` / `<S-CR>` | Send the prepopulated message (retry context injected automatically) |
| **Abort** | `<C-x>` | Add system message "File changes failed to apply.", clear retry state, continue conversation |
| **Fork** | `<C-e>` | Existing fork mechanism to discard and redo the entire exchange |

#### What the LLM sees on retry

The backend injects a `@retry_context` / `@end retry_context` block after the `@latest` block in the user message. This contains:
- The specific error messages (anchor not found, not unique, etc.)
- The original tool calls (JSON) so the LLM can see what it tried
- A prompt to fix the anchors and retry

The system prompt tells the LLM to fix errors in `@retry_context` while also following any additional instructions in `@latest`. The retry context is never saved to chat history — it exists only in the LLM request.

### Context Lock During Requests

All context-modifying operations are blocked while a request is active:

| Operation | Locked? | Reason |
|-----------|---------|--------|
| `u` (update context) | Yes | Changes the base that diffs are applied against |
| Add file | Yes | LLM doesn't know about it |
| Remove file | Yes | LLM might be about to modify it |
| `p` (apply output) | Yes | Moves output → context and deletes output; confusing if new output arrives |
| `x` (discard output) | Yes | Removes output while LLM might produce more changes to the same file |
| View file/diff (`gf`, `gd`) | No | Read-only, harmless |

**Why lock `p`?** Technically safe — `ApplyFile` updates context to match the output and deletes the output, so the diff base is preserved and anchors still match. But allowing it invites users to modify context state during requests, creating confusing intermediate states (applied version on disk, new unapplied version arriving). Locking keeps the workflow clean: request running → read-only; request done → review and act.

**Implementation**: Check `is_streaming()` at the top of each operation. Show a message like "Cannot modify context while a request is active" and return.

### LLM Guidance (for system prompt)

The following guidance goes in the system prompt or tool description:

- Use `modify_file` for editing existing files. Use `write_file` for new files or full rewrites.
- `content` is exactly what will be in the file at that location. Include anchor lines in `content` to keep them.
- `start` and `end` are anchors that locate the region to replace. Use 1-4 lines each.
- Prefer unique lines as anchors: function/method signatures, class declarations, import statements, unique comments.
- Omit `end` for small, localized edits where `start` lines are sufficient.
- Use `start` + `end` for large replacements to avoid reproducing old code.
- Changes within one file must not overlap.
- If two changes are close together (within ~5 lines), merge them into one change to reduce matching errors.
- Do not mix `modify_file` and `write_file` for the same file in one response.

## Benchmark Results (2026-02-17)

We built a benchmark (`cmd/bench`) that sends real editing tasks to real models via OpenRouter and measures whether the model's tool call produces the correct output. Each test is single-shot (no retries). The benchmark logs full tool call arguments to `cmd/bench/logs/` for post-mortem analysis.

### Test Suite

1. **Combined common task** — rename function + rewrite body + add import (3 changes in 1 call)
2. **Reorder functions** — move a function from position 2 to position 4 (delete + insert)
3. **Multiple scattered edits** — 4 small independent changes across a ~250-line file
4. **Edit near duplicates** — add code to one of 4 near-identical CRUD handlers
5. **Deeply nested code** — change a value inside `for` → `switch` → `if` nesting
6. **Large region replacement** — rewrite a ~35-line function with provided replacement code
7. **Multi-file coordinated edit** — add a parameter to a function and update all call sites in a test file

### Anchored Mode Results (`modify_file`)

| Model | Score |
|-------|-------|
| google/gemini-2.5-pro | 6/6 |
| anthropic/claude-opus-4.5 | 3/6 |
| anthropic/claude-sonnet-4 | 2/6 |
| anthropic/claude-sonnet-4.5 | 2/6 |
| z-ai/glm-5 | 1/6 |
| openai/gpt-5.2-codex | 0/6 |

**Dominant failure modes:**

- **Non-unique end anchors**: Models use `}` as an end anchor, which matches dozens of lines.
- **Anchoring on post-edit state**: Models mentally apply a rename, then anchor on the new name that doesn't exist in the original file.
- **Function reordering**: Requires two non-overlapping changes (delete + insert). Failed for all models except Gemini.
- **Anchor selection as cognitive task**: The core problem — models can't "grep" the file to verify uniqueness.

### Search/Replace Mode (`edit_file` — single call per edit)

Added as an alternative to anchored mode. Each `edit_file(path, old_string, new_string)` call applies one edit and requires a separate round-trip.

| Model | Score | Notes |
|-------|-------|-------|
| z-ai/glm-5 | 6/6 | |
| anthropic/claude-sonnet-4.5 | 5/6 | Failed: reorder |
| google/gemini-2.5-pro | 5/6 | 1 API error; tasks that ran all passed |
| anthropic/claude-sonnet-4 | 2/6 | |
| openai/gpt-5.2-codex | 2/6 | |

**Key insight — single-call limitation**: Anthropic models and GPT-5.2-Codex strongly prefer making one tool call per response, then waiting for the result before making the next. Since `modify_file` batches N changes in one `changes[]` array, this wasn't a problem before. But SR mode requires N separate tool calls for N edits, and models simply don't emit the second call. The reorder task (which requires a delete + insert = 2 calls) reliably fails for these models.

### Search/Replace Multi Mode (`edit_file` — batched `edits[]` array)

To solve the single-call limitation, we added a third mode where all edits are batched in one `edit_file(edits: [{path, old_string, new_string, replace_all}, ...])` call. This gives models the search/replace simplicity they handle well, with the batching they need.

| Model | Score |
|-------|-------|
| **anthropic/claude-opus-4.5** | **7/7** |
| **anthropic/claude-sonnet-4.5** | **7/7** * |
| **anthropic/claude-sonnet-4** | **7/7** * |
| **openai/gpt-5.2-codex** | **7/7** * |
| **z-ai/glm-5** | **7/7** * |
| google/gemini-2.5-pro | API issues (intermittent failures) |

\* Tests 1-6 run as full suite; test 7 confirmed individually.

This is the clear winner — 7/7 for every model with working API access.

### Matching Improvements

Three matching improvements were needed to get Opus 4.5 from 3/6 to 6/6 in sr_multi mode:

1. **Pass 3 indent skip**: When old_string and new_string have different leading whitespace on their first non-empty lines, skip indentation adjustment. The model likely got new_string's indentation right but old_string's wrong.

2. **Pass 4 boundary prefix matching**: New matching pass (between TrimSpace and raw substring) that allows the first/last lines of old_string to be truncated prefixes of the file lines. Models sometimes use context lines as anchors without copying the full text (e.g., `// ParseConfig` matching `// ParseConfig reads a configuration file...`). When matched, truncated lines in new_string are automatically expanded to the file's full lines. Minimum 8 characters after trimming to avoid false positives.

3. **Per-line indent delta**: Instead of computing a single indent delta from old_string's first line and applying it to ALL new_string lines, compute per-line deltas. Only adjust new_string lines where the corresponding old_string line actually has an indentation mismatch with the file. This prevents over-correction when only some lines have wrong indentation.

### Full Matching Pipeline (search/replace modes)

1. **Exact**: line-by-line consecutive character-for-character match
2. **Trailing whitespace trimmed**: per-line `TrimRight(" \t\r")`
3. **All whitespace trimmed**: per-line `TrimSpace()` with per-line indentation adjustment
4. **Boundary prefix**: first/last lines can be truncated prefixes (min 8 chars), interior lines TrimSpace equality
5. **Raw substring**: `strings.Index(content, oldString)` as last resort

### Conclusion

`search_replace_multi` is the default diff mode. It achieves 6/6 single-shot across all tested models. The key design insight: models work best with search/replace semantics (copy exact text to match) delivered in a batched format (all edits in one tool call).

## V3: Search/Replace `edit_file` (Current Default)

Based on benchmark results, we added search/replace modes as the default. The LLM always sees the tool as `edit_file`, regardless of mode. The schema changes based on config.

### Configuration

```lua
require('bb7').setup({
  diff_mode = "search_replace_multi",  -- default: batched search/replace
  -- diff_mode = "search_replace",     -- single-call search/replace (one edit per call)
  -- diff_mode = "anchored",           -- anchor-based edit_file (same as old modify_file)
  -- diff_mode = "off",                -- write_file only
})
```

| Value | Tools exposed | Default? |
|-------|--------------|----------|
| `"search_replace_multi"` | `write_file` + `edit_file` (batched search/replace) | Yes |
| `"search_replace"` | `write_file` + `edit_file` (single search/replace) | |
| `"anchored"` | `write_file` + `edit_file` (anchor schema) | |
| `"off"` | `write_file` only | |

### Batched Search/Replace `edit_file` (search_replace_multi)

```
edit_file(edits: [{path, old_string, new_string, replace_all?}, ...])
```

All edits in a single tool call. Edits are applied sequentially (later edits see the result of earlier ones). This is the recommended mode — models strongly prefer making one tool call per response.

### Single Search/Replace `edit_file` (search_replace)

```
edit_file(path, old_string, new_string, replace_all?)
```

One edit per tool call. Multiple calls allowed per response. Kept as a fallback for models that handle multi-turn tool calling well (e.g., GLM-5).

### Error Handling

Same atomic write pattern as anchored mode:
- Parse error → terminal stream error (cancel stream)
- `old_string` not found → diff error (buffered, discards all pending writes)
- `old_string` not unique (when `replace_all=false`) → diff error
- No-op (`old_string == new_string`) → parse error (rejected before matching)

Existing retry UX works unchanged.

## Approach Comparison

### Prior Art

**Search/Replace** (Aider, Claude Code): LLM outputs old and new blocks. Simple, but requires exact reproduction of old code.

**Opencode Patch**: Custom format with `@@` context anchors, `-`/`+` line prefixes. Uses multi-pass fuzzy matching (9-pass for their edit tool).

**BB-7 Region-Based**: Anchors locate the region, `content` replaces it entirely. Old code is never reproduced. Elegant but cognitively hard for models (anchor selection requires "grepping" the file mentally).

**BB-7 Search/Replace Multi**: Batched search/replace with 5-pass matching. Combines the simplicity of search/replace with the efficiency of batched tool calls. All edits in one `edits[]` array.

### Summary

| Aspect | BB-7 SR Multi | Aider/Claude Code SR | Opencode Patch | BB-7 Anchored |
|--------|--------------|---------------------|----------------|---------------|
| Benchmark | **6/6 all models** | — | — | 0-6/6 varies |
| Old content | Must reproduce | Must reproduce | Must reproduce | Not needed |
| Matching | 5-pass tolerant | Exact | 9-pass fuzzy | 3-pass |
| Batching | `edits[]` array | One per call | One per call | `changes[]` array |
| Multi-file | Yes (per edit) | Yes (per call) | Yes (per call) | No |
| Complexity | Simple | Simple | Complex | Medium |

## References

- Opencode patch implementation: `reference/opencode/packages/opencode/src/patch/`
- Opencode edit tool: `reference/opencode/packages/opencode/src/tool/edit.ts`
- Aider's approach: Search/replace blocks in markdown
- Claude Code: Similar search/replace with `<<<<<<< SEARCH` markers
