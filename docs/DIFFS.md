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

## Future: Diff-Based Approach

Complete files become impractical for large files. The LLM should output just the changes.

### Approach Comparison

#### 1. Search/Replace (Aider, Claude Code)

```
<<<<<<< SEARCH
def hello():
    print("hello")
=======
def hello():
    print("hello, world!")
>>>>>>> REPLACE
```

**Pros:**
- Simple format
- Easy for LLM to generate
- Works for small, targeted changes

**Cons:**
- Ambiguous if search text appears multiple times
- Large search blocks are error-prone (LLM may not reproduce exactly)
- No anchoring mechanism

#### 2. Anchor-Based Replacement (Proposed for BB-7)

Instead of reproducing old code, use small anchors to define boundaries:

```
<<<<<<< BEFORE (4 lines)
    def hello():
        """Say hello."""
        # Original implementation
        pass
======= MODIFIED
        print("hello, world!")
        return True
======= AFTER (4 lines)

    def goodbye():
        """Say goodbye."""
        print("bye")
>>>>>>>
```

**Algorithm:**
1. Find BEFORE lines in file (small anchor, easy exact match)
2. Find AFTER lines in file (small anchor, easy exact match)
3. Delete everything between them (implicit old content)
4. Insert MODIFIED chunk

**Key insight**: The old content is never specified - it's implicitly "whatever is between the anchors." The LLM only outputs:
- Small anchors (4 lines each) → high accuracy, easy to reproduce
- New content → no matching needed, just insertion

**Pros:**
- Small anchors are easy for LLM to reproduce exactly
- Large modifications don't need to match anything
- No ambiguity if anchors are unique in the file
- Scales to any size modification

**Cons:**
- Requires unique anchor sequences (rare edge case)
- Slightly more complex format than search/replace

#### 3. Opencode's Apply Patch Format

Opencode uses a custom patch format with explicit operations:

```
*** Begin Patch
*** Update File: src/main.py
@@ def hello():
-    print("hello")
+    print("hello, world!")
*** End Patch
```

**Format details:**
- `*** Add File: <path>` - Create new file
- `*** Delete File: <path>` - Remove file
- `*** Update File: <path>` - Modify file
- `*** Move to: <new_path>` - Rename (optional, with Update)
- `@@` lines provide context anchors
- `-` prefix: remove line
- `+` prefix: add line
- ` ` (space) prefix: unchanged context line

**Hunks structure:**
```
{
  type: "update",
  path: "src/main.py",
  chunks: [
    {
      old_lines: ["    print(\"hello\")"],
      new_lines: ["    print(\"hello, world!\")"],
      change_context: "def hello():"
    }
  ]
}
```

**Pros:**
- Explicit operation types (add/delete/update/move)
- Context anchors (`@@` lines) for disambiguation
- Supports end-of-file anchoring
- Multiple chunks per file
- File moves/renames

**Cons:**
- More complex format for LLM to generate
- Still requires `old_lines` to be specified (error-prone for large changes)
- Requires robust multi-pass line matching to handle LLM inaccuracies

### Opencode's Robustness Strategy

Rather than requiring exact matches, opencode uses multi-pass matching:

1. **Exact match**: Line equals line
2. **Rstrip**: Ignore trailing whitespace
3. **Full trim**: Ignore leading/trailing whitespace
4. **Unicode normalized**: Smart quotes, dashes normalized
5. **End-of-file anchor**: Match from end if flagged

For the Edit tool (search/replace), even more strategies:
- Context anchors (match by first/last line of block)
- Whitespace normalization (all whitespace to single space)
- Indentation flexibility (ignore indentation)
- Levenshtein similarity (fuzzy matching with threshold)

### Recommendation for BB-7 V2

Use the **anchor-based replacement** approach:

1. **Keep `write_file` for small files** (< 500 lines) and new files
2. **Add `modify_file` tool** using anchor-based replacement:

```
modify_file(path, changes):
  changes: [
    {
      before: ["line1", "line2", "line3", "line4"],  // 4-line anchor
      modified: ["new line 1", "new line 2", ...],   // any size
      after: ["line5", "line6", "line7", "line8"]    // 4-line anchor
    }
  ]
```

3. **Multi-pass anchor matching** for robustness:
   - Exact match first
   - Whitespace-normalized match (trailing whitespace)
   - If still ambiguous, error and fall back to `write_file`

4. **Atomic application**: Verify all anchors found before modifying

**Why this over opencode's approach:**
- Opencode still requires `old_lines` to be specified
- For large changes (40+ lines), reproducing old code is error-prone
- Anchors are small (4 lines) so LLM accuracy is high
- Simpler mental model: "replace what's between these markers"

## Implementation Priority

1. **V1 (Current)**: Complete files, simple conflict UI
2. **V2**: Add `patch_file` tool, keep `write_file` as fallback
3. **V3**: Intelligent tool selection (patch for edits, write for new files)

## Approach Comparison Summary

| Aspect | Search/Replace | Opencode Patch | Anchor-Based (BB7) |
|--------|---------------|----------------|---------------------|
| Old content | Must reproduce exactly | Must reproduce exactly | Not needed (implicit) |
| Anchoring | None (match full block) | Single context line | 4-line before + after |
| Large changes | Error-prone | Error-prone | Reliable (small anchors) |
| Complexity | Simple | Complex | Medium |
| Ambiguity | High (duplicates) | Medium (context helps) | Low (8 lines total) |

**Key difference**: Anchor-based replacement never asks the LLM to reproduce old code. It only needs to reproduce 8 lines of anchors (4 before, 4 after) regardless of how large the modification is.

## References

- Opencode patch implementation: `reference/opencode/packages/opencode/src/patch/`
- Opencode edit tool: `reference/opencode/packages/opencode/src/tool/edit.ts`
- Aider's approach: Search/replace blocks in markdown
- Claude Code: Similar search/replace with `<<<<<<< SEARCH` markers
