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

// DefaultTools returns the tools to include in every request.
func DefaultTools() []Tool {
	return []Tool{WriteFileTool}
}
