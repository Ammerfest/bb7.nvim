package state

import (
	"github.com/youruser/bb7/internal/llm"
)

// FileTokenInfo contains token information for a single file.
type FileTokenInfo struct {
	Path           string `json:"path"`
	Tokens         int    `json:"tokens"`           // Total tokens (original + output if has_output)
	HasOutput      bool   `json:"has_output"`       // Whether this file has LLM output
	OriginalTokens int    `json:"original_tokens"`  // Tokens in original context file
	OutputTokens   int    `json:"output_tokens"`    // Tokens in output file (if any)
}

// TokenEstimate contains the full token estimate for the current context.
type TokenEstimate struct {
	Total            int             `json:"total"`             // Total tokens that will be sent
	ContextFiles     int             `json:"context_files"`     // Tokens from context files
	History          int             `json:"history"`           // Tokens from chat history
	Instructions     int             `json:"instructions"`      // Tokens from instruction files
	SystemPrompt     int             `json:"system_prompt"`     // Tokens from system prompt
	InputText        int             `json:"input_text"`        // Tokens from current input text
	Files            []FileTokenInfo `json:"files"`             // Per-file breakdown
	PotentialSavings int             `json:"potential_savings"` // Tokens saved by applying M files
}

// EstimateTokens calculates token estimates for the current chat context.
func (s *State) EstimateTokens(systemPrompt string, inputText string) (*TokenEstimate, error) {
	if err := s.requireActiveChat(); err != nil {
		return nil, err
	}

	estimate := &TokenEstimate{}

	// Estimate system prompt tokens
	estimate.SystemPrompt = llm.EstimateTokensSimple(systemPrompt)

	// Estimate instruction files
	instructionsBlock, err := s.BuildInstructionsBlock()
	if err == nil && instructionsBlock != "" {
		estimate.Instructions = llm.EstimateTokensSimple(instructionsBlock)
	}

	// Estimate chat history
	for _, msg := range s.ActiveChat.Messages {
		estimate.History += llm.EstimateTokensSimple(MessageText(msg))
	}

	// Estimate context files
	for _, cf := range s.ActiveChat.ContextFiles {
		fileInfo := FileTokenInfo{Path: cf.Path}

		// Get original context content
		originalContent, err := s.GetContextFile(cf.Path)
		if err != nil {
			continue
		}
		fileInfo.OriginalTokens = llm.EstimateTokensSimple(originalContent)

		// Check if there's an output file (external files can't have output)
		var outputContent string
		if !cf.External {
			outputContent, err = s.GetOutputFile(cf.Path)
		}
		if err == nil && outputContent != "" {
			fileInfo.HasOutput = true
			fileInfo.OutputTokens = llm.EstimateTokensSimple(outputContent)
			// When sending both versions, total = original + output
			fileInfo.Tokens = fileInfo.OriginalTokens + fileInfo.OutputTokens
			// Potential savings = original tokens (after applying, we only send the new version)
			estimate.PotentialSavings += fileInfo.OriginalTokens
		} else {
			fileInfo.Tokens = fileInfo.OriginalTokens
		}

		estimate.ContextFiles += fileInfo.Tokens
		estimate.Files = append(estimate.Files, fileInfo)
	}

	// Estimate input text tokens
	if inputText != "" {
		estimate.InputText = llm.EstimateTokensSimple(inputText)
	}

	// Calculate total
	estimate.Total = estimate.SystemPrompt + estimate.Instructions + estimate.ContextFiles + estimate.History + estimate.InputText

	return estimate, nil
}
