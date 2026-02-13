package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/youruser/bb7/internal/logging"
)

var (
	ErrRequestFailed = errors.New("API request failed")
	ErrStreamError   = errors.New("stream error")
	log              = logging.Get()
)

const defaultRequestTimeout = 30 * time.Second

// Client handles communication with the LLM API.
type Client struct {
	baseURL            string
	apiKey             string
	httpClient         *http.Client
	allowTraining      bool
	allowDataRetention bool
}

// NewClient creates a new LLM client.
func NewClient(baseURL, apiKey string, allowTraining, allowDataRetention bool) *Client {
	return &Client{
		baseURL:            strings.TrimSuffix(baseURL, "/"),
		apiKey:             apiKey,
		httpClient:         &http.Client{},
		allowTraining:      allowTraining,
		allowDataRetention: allowDataRetention,
	}
}

// providerPreferences builds the provider preferences for API requests
// based on client privacy settings. Returns nil if no restrictions apply.
func (c *Client) providerPreferences() *ProviderPreferences {
	if c.allowTraining && c.allowDataRetention {
		return nil
	}
	p := &ProviderPreferences{}
	if !c.allowTraining {
		p.DataCollection = "deny"
	}
	if !c.allowDataRetention {
		t := true
		p.ZDR = &t
	}
	return p
}

// StreamCallback is called for each event in the stream.
type StreamCallback func(event StreamEvent)

// ChatStream sends a chat request and streams the response.
// The callback is called for each event (content chunks, tool calls, completion).
// If reasoning is non-nil, extended thinking is enabled with the specified effort level.
func (c *Client) ChatStream(ctx context.Context, model, systemPrompt string, messages []Message, reasoning *ReasoningConfig, callback StreamCallback) error {
	// Prepend system message
	allMessages := make([]Message, 0, len(messages)+1)
	allMessages = append(allMessages, Message{
		Role:    "system",
		Content: systemPrompt,
	})
	allMessages = append(allMessages, messages...)

	reqBody := ChatRequest{
		Model:     model,
		Messages:  allMessages,
		Tools:     DefaultTools(),
		Stream:    true,
		Reasoning: reasoning,
		Provider:  c.providerPreferences(),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	log.Debug("HTTP POST %s/chat/completions (model: %s, messages: %d, tools: %d)",
		c.baseURL, model, len(allMessages), len(reqBody.Tools))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error("HTTP request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	log.Debug("HTTP response status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error("API error %d: %s", resp.StatusCode, string(body))
		return fmt.Errorf("%w: %d - %s", ErrRequestFailed, resp.StatusCode, string(body))
	}

	return c.processStream(ctx, resp.Body, callback)
}

// processStream reads SSE events and calls the callback for each.
func (c *Client) processStream(ctx context.Context, reader io.Reader, callback StreamCallback) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// Track tool calls being built up across multiple deltas
	toolCalls := make(map[int]*ToolCall)
	var lastUsage *Usage
	log.Debug("Starting SSE stream processing")

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()

		// SSE format: "data: {json}"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Stream end marker
		if data == "[DONE]" {
			log.Debug("SSE stream received [DONE], emitting %d tool calls", len(toolCalls))
			// Emit any complete tool calls
			for _, tc := range toolCalls {
				log.Debug("Emitting tool call: %s", tc.Function.Name)
				callback(StreamEvent{
					Type:     "tool_call",
					ToolCall: tc,
				})
			}
			callback(StreamEvent{Type: "done", Usage: lastUsage})
			return nil
		}

		var resp ChatResponse
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue // Skip malformed chunks
		}

		if resp.Error != nil {
			callback(StreamEvent{
				Type:  "error",
				Error: resp.Error.Message,
			})
			return fmt.Errorf("%w: %s", ErrStreamError, resp.Error.Message)
		}

		// Capture usage if present (typically in the final chunk)
		if resp.Usage != nil {
			lastUsage = resp.Usage
			log.Debug("Captured usage: prompt=%d, completion=%d, cached=%d",
				resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.CachedTokens)
		}

		if len(resp.Choices) == 0 {
			continue
		}

		choice := resp.Choices[0]
		delta := choice.Delta
		if delta == nil {
			delta = choice.Message
		}
		if delta == nil {
			continue
		}

		// Handle text content
		if delta.Content != "" {
			callback(StreamEvent{
				Type:    "content",
				Content: delta.Content,
			})
		}

		// Handle reasoning/thinking content
		if delta.Reasoning != "" {
			callback(StreamEvent{
				Type:      "reasoning",
				Reasoning: delta.Reasoning,
			})
		}

		// Handle tool calls (they come in pieces during streaming)
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			if tc.ID != "" {
				// New tool call starting
				toolCalls[idx] = &ToolCall{
					Index: idx,
					ID:    tc.ID,
					Type:  tc.Type,
					Function: ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			} else if existing, ok := toolCalls[idx]; ok {
				// Append to existing tool call
				if tc.Function.Name != "" {
					existing.Function.Name = tc.Function.Name
				}
				existing.Function.Arguments += tc.Function.Arguments
			}
		}
	}

	if err := scanner.Err(); err != nil {
		// When the context is canceled (user abort), the HTTP body closes and
		// the scanner sees an IO error. Return the context error so callers
		// can detect the cancellation and save the partial response.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		log.Error("SSE scanner error: %v", err)
		return err
	}

	// If stream ended without [DONE], still emit any collected tool calls
	log.Debug("SSE stream ended without [DONE], emitting %d tool calls (fallback)", len(toolCalls))
	for _, tc := range toolCalls {
		log.Debug("Emitting tool call (fallback): %s", tc.Function.Name)
		callback(StreamEvent{
			Type:     "tool_call",
			ToolCall: tc,
		})
	}
	callback(StreamEvent{Type: "done", Usage: lastUsage})

	return nil
}

// ParseWriteFileArgs parses the arguments JSON for a write_file tool call.
func ParseWriteFileArgs(argsJSON string) (*WriteFileArgs, error) {
	var args WriteFileArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, err
	}
	if args.Path == "" {
		return nil, errors.New("write_file: missing path")
	}
	return &args, nil
}

// ChatSimple sends a simple chat request without streaming or tools.
// Returns the assistant's response content.
func (c *Client) ChatSimple(model, systemPrompt string, messages []Message) (string, error) {
	// Prepend system message
	allMessages := make([]Message, 0, len(messages)+1)
	allMessages = append(allMessages, Message{
		Role:    "system",
		Content: systemPrompt,
	})
	allMessages = append(allMessages, messages...)

	reqBody := map[string]any{
		"model":       model,
		"messages":    allMessages,
		"stream":      false,
		"temperature": 0.5, // Lower temp for more consistent titles
	}
	if p := c.providerPreferences(); p != nil {
		prov := map[string]any{}
		if p.DataCollection != "" {
			prov["data_collection"] = p.DataCollection
		}
		if p.ZDR != nil && *p.ZDR {
			prov["zdr"] = true
		}
		reqBody["provider"] = prov
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	log.Debug("HTTP POST %s/chat/completions (simple, model: %s)", c.baseURL, model)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error("HTTP request failed: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error("API error %d: %s", resp.StatusCode, string(body))
		return "", fmt.Errorf("%w: %d - %s", ErrRequestFailed, resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", errors.New("no choices in response")
	}

	// Get content from message (non-streaming response)
	msg := chatResp.Choices[0].Message
	if msg == nil {
		return "", errors.New("no message in response")
	}

	return msg.Content, nil
}

// GetBalance fetches the account balance from OpenRouter.
func (c *Client) GetBalance() (*BalanceResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/credits", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	log.Debug("HTTP GET %s/credits", c.baseURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error("HTTP request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error("API error %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("%w: %d - %s", ErrRequestFailed, resp.StatusCode, string(body))
	}

	var balance BalanceResponse
	if err := json.NewDecoder(resp.Body).Decode(&balance); err != nil {
		return nil, err
	}

	return &balance, nil
}

// GetModels fetches the list of available models with pricing.
func (c *Client) GetModels() (*ModelsResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	log.Debug("HTTP GET %s/models", c.baseURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Error("HTTP request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Error("API error %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("%w: %d - %s", ErrRequestFailed, resp.StatusCode, string(body))
	}

	var models ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, err
	}

	return &models, nil
}

// GetModelPricing fetches pricing for a specific model.
func (c *Client) GetModelPricing(modelID string) (*ModelPricing, error) {
	models, err := c.GetModels()
	if err != nil {
		return nil, err
	}

	for _, m := range models.Data {
		if m.ID == modelID {
			return &m.Pricing, nil
		}
	}

	return nil, fmt.Errorf("model not found: %s", modelID)
}
