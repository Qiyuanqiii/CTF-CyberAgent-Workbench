package llm

import (
	"context"
	"encoding/json"
)

type Provider interface {
	Name() string
	ListModels(ctx context.Context) ([]ModelInfo, error)
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	StreamChat(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error)
	SupportsTools(model string) bool
	SupportsVision(model string) bool
	SupportsJSONMode(model string) bool
}

type ModelInfo struct {
	ID           string
	DisplayName  string
	Provider     string
	Capabilities []string
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type ChatRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolSpec
	Temperature float64
	MaxTokens   int
	JSONMode    bool
	Metadata    map[string]string
}

type ChatResponse struct {
	Text      string
	ToolCalls []ToolCall
	Usage     Usage
	Raw       json.RawMessage
	Model     string
	Provider  string
}

type ChatChunk struct {
	Text      string
	Done      bool
	ToolCalls []ToolCall
	Usage     *Usage
	Model     string
	Provider  string
	Err       error
}

func FinalChatChunk(response *ChatResponse) ChatChunk {
	if response == nil {
		return ChatChunk{Done: true}
	}
	usage := response.Usage
	return ChatChunk{
		Done: true, ToolCalls: response.ToolCalls, Usage: &usage, Model: response.Model, Provider: response.Provider,
	}
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
