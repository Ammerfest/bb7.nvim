package state

import (
	"strconv"
	"strings"
	"time"
)

// MessagePart represents a structured piece of message content.
type MessagePart struct {
	Type         string `json:"type"`                    // "text", "code", "raw", "thinking", "context_event"
	Content      string `json:"content,omitempty"`       // content for text/code/raw/thinking
	Language     string `json:"language,omitempty"`      // for "code" type
	Path         string `json:"path,omitempty"`          // for "context_event" type
	Added        bool   `json:"added,omitempty"`         // for "context_event" type: true if new file (AssistantWriteFile)
	Action       string `json:"action,omitempty"`        // for "context_event" type
	ReadOnly     *bool  `json:"readonly,omitempty"`      // for "context_event" type
	External     *bool  `json:"external,omitempty"`      // for "context_event" type
	Version      string `json:"version,omitempty"`       // for "context_event" type
	PrevVersion  string `json:"prev_version,omitempty"`  // for "context_event" type
	OriginalPath string `json:"original_path,omitempty"` // for "context_event" type: original path when saved elsewhere
	StartLine    int    `json:"start_line,omitempty"`    // for "context_event" type: section start line
	EndLine      int    `json:"end_line,omitempty"`      // for "context_event" type: section end line
}

// MessageUsage contains token counts and cost for a message.
type MessageUsage struct {
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	CachedTokens     int     `json:"cached_tokens,omitempty"`
	TotalTokens      int     `json:"total_tokens,omitempty"`
	Cost             float64 `json:"cost,omitempty"`
	Duration         float64 `json:"duration,omitempty"` // seconds
}

// Message represents a single message in a chat conversation.
type Message struct {
	Role            string           `json:"role"`                       // "user", "assistant", or "system"
	Model           string           `json:"model,omitempty"`            // model used (assistant) or requested (user)
	Parts           []MessagePart    `json:"parts,omitempty"`            // structured content (new)
	Content         string           `json:"content,omitempty"`          // raw content (backward compat)
	Timestamp       time.Time        `json:"timestamp"`
	OutputFiles     []string         `json:"output_files,omitempty"`     // assistant only
	Usage           *MessageUsage    `json:"usage,omitempty"`            // token usage and cost (assistant only)
	ReasoningEffort string           `json:"reasoning_effort,omitempty"` // "low", "medium", "high" (assistant only)
	ContextSnapshot []ContextFileRef `json:"context_snapshot,omitempty"` // context state at send time (user messages only)
}

// HasParts returns true if the message uses structured parts.
func (m *Message) HasParts() bool {
	return len(m.Parts) > 0
}

// MessageText returns a text representation of a message.
// Prefers Content; otherwise concatenates relevant parts.
func MessageText(m Message) string {
	if m.Content != "" {
		return m.Content
	}
	if len(m.Parts) == 0 {
		return ""
	}

	var b strings.Builder
	for _, part := range m.Parts {
		switch part.Type {
		case "text", "thinking", "code", "raw":
			if part.Content == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(part.Content)
		case "context_event":
			formatted := formatContextEvent(part)
			if formatted == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(formatted)
		}
	}

	return b.String()
}

func formatContextEvent(part MessagePart) string {
	if part.Action == "" && part.Path == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("[context_event")
	if part.Action != "" {
		b.WriteString(" action=")
		b.WriteString(part.Action)
	}
	if part.Path != "" {
		b.WriteString(" path=")
		b.WriteString(strconv.Quote(part.Path))
	}
	if part.StartLine > 0 && part.EndLine > 0 {
		b.WriteString(" lines=")
		b.WriteString(strconv.Itoa(part.StartLine))
		b.WriteString("-")
		b.WriteString(strconv.Itoa(part.EndLine))
	}
	if part.ReadOnly != nil {
		b.WriteString(" readonly=")
		b.WriteString(strconv.FormatBool(*part.ReadOnly))
	}
	if part.External != nil {
		b.WriteString(" external=")
		b.WriteString(strconv.FormatBool(*part.External))
	}
	if part.PrevVersion != "" {
		b.WriteString(" prev_version=")
		b.WriteString(part.PrevVersion)
	}
	if part.Version != "" {
		b.WriteString(" version=")
		b.WriteString(part.Version)
	}
	b.WriteString("]")
	return b.String()
}

// ContextFileRef is a lightweight reference to a context file version.
// Used to snapshot context state at the time of a user message.
type ContextFileRef struct {
	Path      string `json:"path"`
	FileID    string `json:"file_id"`
	StartLine int    `json:"start_line,omitempty"` // 1-indexed start line for sections, 0 = full file
	EndLine   int    `json:"end_line,omitempty"`   // 1-indexed inclusive end line for sections, 0 = full file
}

// ContextFile represents a file in the chat's context.
type ContextFile struct {
	Path      string `json:"path"`                 // Relative path (internal) or absolute path (external)
	ReadOnly  bool   `json:"readonly"`             // If true, LLM cannot write to this file
	External  bool   `json:"external"`             // If true, file is outside project directory
	Version   string `json:"version,omitempty"`    // Hash of context snapshot content
	StartLine int    `json:"start_line,omitempty"` // 1-indexed start line for sections, 0 = full file
	EndLine   int    `json:"end_line,omitempty"`   // 1-indexed inclusive end line for sections, 0 = full file
}

// Chat represents a single chat session with its messages and context.
type Chat struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Created         time.Time     `json:"created"`
	Model           string        `json:"model"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	Global          bool          `json:"global,omitempty"` // If true, this is a global chat (stored in ~/.bb7/chats/)
	Draft           string        `json:"draft,omitempty"`  // Unsent message draft
	ContextFiles    []ContextFile `json:"context_files"`
	Messages        []Message     `json:"messages"`
}

// ChatSummary is a lightweight representation for listing chats.
type ChatSummary struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Created time.Time `json:"created"`
	Global  bool      `json:"global,omitempty"` // If true, this is a global chat
	Locked  bool      `json:"locked,omitempty"` // If true, chat is locked by another process
}

// ChatSearchResult represents a chat that matched a search query.
type ChatSearchResult struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Created   time.Time `json:"created"`
	MatchType string    `json:"match_type"`          // "title" or "content"
	Excerpt   string    `json:"excerpt,omitempty"`   // For content matches, snippet with context
}
