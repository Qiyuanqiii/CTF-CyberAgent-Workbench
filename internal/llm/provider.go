package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxProviderToolCalls       = 16
	MaxProviderToolIdentity    = 128
	MaxProviderToolPayloadSize = 256 * 1024
	MaxProviderToolResultSize  = 64 * 1024
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
	Role        string       `json:"role"`
	Content     string       `json:"content,omitempty"`
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults []ToolResult `json:"tool_results,omitempty"`
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
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func NormalizeToolCall(call ToolCall) (ToolCall, error) {
	call.ID = strings.TrimSpace(call.ID)
	call.Name = strings.TrimSpace(call.Name)
	call.Arguments = append(json.RawMessage(nil), bytes.TrimSpace(call.Arguments)...)
	if call.ID == "" || call.Name == "" || !utf8.ValidString(call.ID) || !utf8.ValidString(call.Name) ||
		len([]rune(call.ID)) > MaxProviderToolIdentity || len([]rune(call.Name)) > MaxProviderToolIdentity ||
		strings.ContainsRune(call.ID, 0) || strings.ContainsRune(call.Name, 0) {
		return ToolCall{}, errors.New("provider tool call id and name must be bounded normalized UTF-8")
	}
	for _, current := range call.Name {
		if (current < 'a' || current > 'z') && (current < '0' || current > '9') && current != '_' {
			return ToolCall{}, errors.New("provider tool call name must use lowercase letters, digits, or underscore")
		}
	}
	if len(call.Arguments) == 0 || len(call.Arguments) > MaxProviderToolPayloadSize ||
		!utf8.Valid(call.Arguments) || !json.Valid(call.Arguments) {
		return ToolCall{}, fmt.Errorf("provider tool call arguments must be valid UTF-8 JSON up to %d bytes",
			MaxProviderToolPayloadSize)
	}
	return call, nil
}

func NormalizeToolCalls(calls []ToolCall) ([]ToolCall, error) {
	if len(calls) > MaxProviderToolCalls {
		return nil, fmt.Errorf("provider tool call list exceeds %d items", MaxProviderToolCalls)
	}
	out := make([]ToolCall, len(calls))
	seen := make(map[string]struct{}, len(calls))
	for index, call := range calls {
		normalized, err := NormalizeToolCall(call)
		if err != nil {
			return nil, fmt.Errorf("invalid provider tool call at index %d: %w", index, err)
		}
		if _, exists := seen[normalized.ID]; exists {
			return nil, errors.New("provider tool call ids must be unique")
		}
		seen[normalized.ID] = struct{}{}
		out[index] = normalized
	}
	return out, nil
}

type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

func NormalizeToolResult(result ToolResult) (ToolResult, error) {
	result.ToolCallID = strings.TrimSpace(result.ToolCallID)
	result.Content = strings.TrimSpace(result.Content)
	if result.ToolCallID == "" || !utf8.ValidString(result.ToolCallID) ||
		len([]rune(result.ToolCallID)) > MaxProviderToolIdentity || strings.ContainsRune(result.ToolCallID, 0) {
		return ToolResult{}, errors.New("provider tool result call id must be bounded normalized UTF-8")
	}
	if result.Content == "" || !utf8.ValidString(result.Content) || len(result.Content) > MaxProviderToolResultSize {
		return ToolResult{}, fmt.Errorf("provider tool result content must be valid UTF-8 up to %d bytes",
			MaxProviderToolResultSize)
	}
	return result, nil
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
