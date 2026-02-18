package llm

// WriteFileTool writes content to a file for user review.
var WriteFileTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "write_file",
		Description: "Write content to a file in the output directory for user review.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative file path (e.g., 'math.cs' or 'src/utils/helper.go')",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Complete file content",
				},
			},
			"required": []string{"path", "content"},
		},
	},
}

// EditFileAnchoredTool is the anchor-based edit tool exposed as "edit_file".
var EditFileAnchoredTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "edit_file",
		Description: "Apply targeted changes to an existing file using anchor-based regions.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative file path of the existing file to modify",
				},
				"file_id": map[string]any{
					"type":        "string",
					"description": "Optional file identifier from the @file id=... listing for the exact base version to edit.",
				},
				"changes": map[string]any{
					"type":        "array",
					"description": "List of changes to apply to the file",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"start": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"minItems":    1,
								"maxItems":    10,
								"description": "1-4 consecutive lines that uniquely mark the start of the region (up to 10 allowed if needed for disambiguation)",
							},
							"end": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"minItems":    1,
								"maxItems":    10,
								"description": "1-4 consecutive lines that mark the end of the region (up to 10 allowed; optional — omit for small edits where start lines are the entire region)",
							},
							"content": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Exact replacement lines for the matched region",
							},
						},
						"required": []string{"start", "content"},
					},
				},
			},
			"required": []string{"path", "changes"},
		},
	},
}

// EditFileSRTool is the search/replace edit tool exposed as "edit_file".
var EditFileSRTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "edit_file",
		Description: "Apply a search/replace edit to an existing file.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative file path of the existing file to modify",
				},
				"file_id": map[string]any{
					"type":        "string",
					"description": "Optional file identifier from the @file id=... listing for the exact base version to edit.",
				},
				"old_string": map[string]any{
					"type":        "string",
					"description": "The exact text to find in the file. Must match exactly once (unless replace_all is true).",
				},
				"new_string": map[string]any{
					"type":        "string",
					"description": "The replacement text. May be empty string to delete the matched text.",
				},
				"replace_all": map[string]any{
					"type":        "boolean",
					"description": "Replace all occurrences of old_string (default false).",
				},
			},
			"required": []string{"path", "old_string", "new_string"},
		},
	},
}

// EditFileSRMultiTool is the batched search/replace edit tool exposed as "edit_file".
// Accepts an array of edits applied sequentially, allowing multiple changes in one call.
var EditFileSRMultiTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "edit_file",
		Description: "Apply search/replace edits to one or more files. Edits are applied sequentially.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"edits": map[string]any{
					"type":        "array",
					"description": "List of search/replace edits to apply sequentially",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{
								"type":        "string",
								"description": "Relative file path of the existing file to modify",
							},
							"file_id": map[string]any{
								"type":        "string",
								"description": "Optional file identifier from the @file id=... listing for the exact base version to edit.",
							},
							"old_string": map[string]any{
								"type":        "string",
								"description": "The exact text to find in the file. Must match exactly once (unless replace_all is true).",
							},
							"new_string": map[string]any{
								"type":        "string",
								"description": "The replacement text. May be empty string to delete the matched text.",
							},
							"replace_all": map[string]any{
								"type":        "boolean",
								"description": "Replace all occurrences of old_string (default false).",
							},
						},
						"required": []string{"path", "old_string", "new_string"},
					},
				},
			},
			"required": []string{"edits"},
		},
	},
}

// DefaultTools returns the tools to include in every request.
// diffMode controls which tools are exposed:
//   - "search_replace": write_file + edit_file (search/replace schema)
//   - "search_replace_multi": write_file + edit_file (batched search/replace schema)
//   - "anchored": write_file + edit_file (anchor schema)
//   - "off": write_file only
func DefaultTools(diffMode string) []Tool {
	switch diffMode {
	case "search_replace":
		return []Tool{WriteFileTool, EditFileSRTool}
	case "search_replace_multi":
		return []Tool{WriteFileTool, EditFileSRMultiTool}
	case "anchored":
		return []Tool{WriteFileTool, EditFileAnchoredTool}
	default:
		return []Tool{WriteFileTool}
	}
}
