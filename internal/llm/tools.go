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

// ModifyFileTool applies region-based diffs to an existing file.
var ModifyFileTool = Tool{
	Type: "function",
	Function: ToolFunction{
		Name:        "modify_file",
		Description: "Apply targeted changes to an existing file using anchor-based regions.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Relative file path of the existing file to modify",
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

// DefaultTools returns the tools to include in every request.
// When diffMode is true, the modify_file tool is included alongside write_file.
func DefaultTools(diffMode bool) []Tool {
	if diffMode {
		return []Tool{WriteFileTool, ModifyFileTool}
	}
	return []Tool{WriteFileTool}
}
