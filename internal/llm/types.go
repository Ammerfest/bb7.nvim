package llm

// Request types for OpenRouter/OpenAI-compatible API

// ReasoningConfig controls extended thinking/reasoning for supported models.
type ReasoningConfig struct {
	Effort string `json:"effort,omitempty"` // "low", "medium", "high", or empty to disable
}

type ChatRequest struct {
	Model     string           `json:"model"`
	Messages  []Message        `json:"messages"`
	Tools     []Tool           `json:"tools,omitempty"`
	Stream    bool             `json:"stream"`
	Reasoning *ReasoningConfig `json:"reasoning,omitempty"`
}

type Message struct {
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
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	CachedTokens     int     `json:"cached_tokens,omitempty"`
	Cost             float64 `json:"cost,omitempty"` // In USD, if provided by API
}

type Choice struct {
	Index        int     `json:"index"`
	Delta        *Delta  `json:"delta,omitempty"`
	Message      *Delta  `json:"message,omitempty"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

type Delta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	Reasoning string     `json:"reasoning,omitempty"`          // For thinking/reasoning models
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type APIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// StreamEvent represents a parsed event from the SSE stream.
type StreamEvent struct {
	Type      string    // "content", "reasoning", "tool_call", "done", "error"
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

// BalanceResponse from /api/v1/credits endpoint.
type BalanceResponse struct {
	Data struct {
		TotalCredits float64 `json:"total_credits"`
		TotalUsage   float64 `json:"total_usage"`
	} `json:"data"`
}

// ModelInfo from /api/v1/models endpoint.
type ModelInfo struct {
	ID                  string       `json:"id"`
	Name                string       `json:"name"`
	ContextLength       int          `json:"context_length"`
	Pricing             ModelPricing `json:"pricing"`
	SupportedParameters []string     `json:"supported_parameters,omitempty"`
}

// ModelPricing contains per-token prices in USD.
type ModelPricing struct {
	Prompt         string `json:"prompt"`          // Price per input token
	Completion     string `json:"completion"`      // Price per output token
	InputCacheRead string `json:"input_cache_read"` // Price per cached input token
}

// ModelsResponse from /api/v1/models endpoint.
type ModelsResponse struct {
	Data []ModelInfo `json:"data"`
}
