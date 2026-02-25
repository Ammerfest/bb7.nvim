package llm

// Request types for OpenRouter/OpenAI-compatible API

// ReasoningConfig controls extended thinking/reasoning for supported models.
type ReasoningConfig struct {
	Effort string `json:"effort,omitempty"` // "low", "medium", "high", or empty to disable
}

type ProviderPreferences struct {
	DataCollection string `json:"data_collection,omitempty"` // "allow" or "deny"
	ZDR            *bool  `json:"zdr,omitempty"`             // zero data retention
}

type ChatRequest struct {
	Model          string               `json:"model"`
	Messages       []APIMessage            `json:"messages"`
	Tools          []Tool               `json:"tools,omitempty"`
	Stream         bool                 `json:"stream"`
	Reasoning      *ReasoningConfig     `json:"reasoning,omitempty"`
	Provider       *ProviderPreferences `json:"provider,omitempty"`
	PromptCacheKey string               `json:"prompt_cache_key,omitempty"`
}

type APIMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ToolCall struct {
	Index    int              `json:"index"`
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Response types

type ChatResponse struct {
	ID      string    `json:"id"`
	Choices []Choice  `json:"choices"`
	Usage   *Usage    `json:"usage,omitempty"`
	Error   *APIError `json:"error,omitempty"`
}

// Usage contains token usage and cost information from the API response.
type Usage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	CachedTokens        int                  `json:"cached_tokens,omitempty"` // legacy flat field
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	Cost                float64              `json:"cost,omitempty"` // In USD, if provided by API
}

// PromptTokensDetails contains cache-specific token details for prompt tokens.
type PromptTokensDetails struct {
	CachedTokens     int `json:"cached_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

type Choice struct {
	Index        int    `json:"index"`
	Delta        *Delta `json:"delta,omitempty"`
	Message      *Delta `json:"message,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
}

type Delta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	Reasoning string     `json:"reasoning,omitempty"` // For thinking/reasoning models
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// StreamEvent represents a parsed event from the SSE stream.
type StreamEvent struct {
	Type      string    // "raw", "content", "reasoning", "tool_call", "done", "error"
	Raw       string    // For "raw" events (verbatim SSE data payload, including "[DONE]")
	Content   string    // For "content" events
	Reasoning string    // For "reasoning" events (thinking models)
	ToolCall  *ToolCall // For "tool_call" events
	Error     string    // For "error" events
	Usage     *Usage    // For "done" events, if available
}

// WriteFileArgs is the parsed arguments for the write_file tool.
type WriteFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// AnchoredEditChange represents a single change within an anchored edit_file tool call.
type AnchoredEditChange struct {
	Start   []string `json:"start"`
	End     []string `json:"end,omitempty"`
	Content []string `json:"content"`
}

// AnchoredEditArgs is the parsed arguments for anchored edit_file calls.
type AnchoredEditArgs struct {
	Path    string               `json:"path"`
	FileID  string               `json:"file_id,omitempty"`
	Changes []AnchoredEditChange `json:"changes"`
}

// EditFileArgs is the parsed arguments for the edit_file tool (search/replace mode).
type EditFileArgs struct {
	FileID     string `json:"file_id,omitempty"`
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// EditFileMultiArgs is the parsed arguments for the edit_file tool (search/replace multi mode).
// Contains an array of edits applied sequentially.
type EditFileMultiArgs struct {
	Edits []EditFileArgs `json:"edits"`
}

// BalanceResponse from /api/v1/credits endpoint.
type BalanceResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"`
		TotalUsage   float64 `json:"total_usage"`
	} `json:"data"`
}

// TopProvider contains provider-level limits from the OpenRouter API.
type TopProvider struct {
	MaxCompletionTokens int `json:"max_completion_tokens"`
}

// ModelInfo from /api/v1/models endpoint.
type ModelInfo struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Created             int64        `json:"created"`
	ExpirationDate      *string      `json:"expiration_date"`
	ContextLength       int          `json:"context_length"`
	Pricing             ModelPricing `json:"pricing"`
	TopProvider         TopProvider  `json:"top_provider"`
	SupportedParameters []string     `json:"supported_parameters,omitempty"`
}

// ModelPricing contains per-token prices in USD.
type ModelPricing struct {
	Prompt            string  `json:"prompt"`             // Price per input token
	Completion        string  `json:"completion"`         // Price per output token
	InputCacheRead    string  `json:"input_cache_read"`   // Price per cached input token
	InputCacheWrite   string  `json:"input_cache_write"`  // Price per cache write token
	InternalReasoning string  `json:"internal_reasoning"` // Price per reasoning token
	Discount          float64 `json:"discount"`           // Discount factor (0-1)
}

// ModelsResponse from /api/v1/models endpoint.
type ModelsResponse struct {
	Data []ModelInfo `json:"data"`
}
