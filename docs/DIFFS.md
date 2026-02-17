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

## Approach Comparison

### Prior Art

**Search/Replace** (Aider, Claude Code): LLM outputs old and new blocks. Simple but requires exact reproduction of old code — error-prone for large changes, ambiguous when search text appears multiple times.

**Opencode Patch**: Custom format with `@@` context anchors, `-`/`+` line prefixes. Still requires `old_lines` to be specified. Uses multi-pass fuzzy matching (exact → rstrip → trim → unicode normalize) plus 9-pass matching for their edit tool (Levenshtein, indentation flexibility, etc.).

**BB-7 Region-Based**: Anchors locate the region, `content` replaces it entirely. Old code is never reproduced (only small anchors). 3-pass matching (exact + trailing trim + full trim). Single unified concept for insert, edit, replace, and delete.

### Summary

| Aspect | Search/Replace | Opencode Patch | BB-7 Region |
|--------|---------------|----------------|-------------|
| Old content | Must reproduce exactly | Must reproduce exactly | Not needed |
| Anchor size | Full old block | 1 context line + old lines | 1-4 lines start + end |
| Large changes | Error-prone | Error-prone | Reliable |
| Matching | Exact (fragile) | 4-pass + 9-pass fuzzy | 3-pass (exact + rstrip + trim) |
| Mental model | Find old, put new | Patch format with prefixes | Locate region, replace |
| Complexity | Simple | Complex | Medium |

## Implementation Priority

1. **V1 (Current)**: Complete files via `write_file`, conflict UI.
2. **V2**: Add `modify_file` tool, keep `write_file` for new files and full rewrites.

## References

- Opencode patch implementation: `reference/opencode/packages/opencode/src/patch/`
- Opencode edit tool: `reference/opencode/packages/opencode/src/tool/edit.ts`
- Aider's approach: Search/replace blocks in markdown
- Claude Code: Similar search/replace with `<<<<<<< SEARCH` markers
