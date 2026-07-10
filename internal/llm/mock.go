package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type MockProvider struct{}

func NewMockProvider() MockProvider {
	return MockProvider{}
}

func (MockProvider) Name() string {
	return "mock"
}

func (MockProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return []ModelInfo{
		{ID: "mock-cyber-agent", DisplayName: "Mock Cyber Agent", Provider: "mock", Capabilities: []string{"chat", "tools", "json"}},
		{ID: "mock-fast", DisplayName: "Mock Fast", Provider: "mock", Capabilities: []string{"chat"}},
		{ID: "mock-code", DisplayName: "Mock Code", Provider: "mock", Capabilities: []string{"chat", "code"}},
	}, nil
}

func (MockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	model := req.Model
	if model == "" {
		model = "mock-cyber-agent"
	}
	last := lastUserMessage(req.Messages)
	text := fmt.Sprintf("mock plan [%s]: inspect workspace context, keep actions scoped, produce a safe artifact for %q", model, last)
	if req.JSONMode && req.Metadata["response_schema"] == "root_lifecycle.v1" {
		encoded, _ := json.Marshal(map[string]string{
			"version": "root_lifecycle.v1",
			"action":  "continue",
			"message": text,
		})
		text = string(encoded)
	}
	raw, _ := json.Marshal(map[string]string{"provider": "mock", "model": model, "last_user": last})
	return &ChatResponse{
		Text:     text,
		Raw:      raw,
		Model:    model,
		Provider: "mock",
		Usage: Usage{
			InputTokens:  len(strings.Fields(last)),
			OutputTokens: len(strings.Fields(text)),
			TotalTokens:  len(strings.Fields(last)) + len(strings.Fields(text)),
		},
	}, nil
}

func (m MockProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error) {
	resp, err := m.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan ChatChunk, 1)
	ch <- ChatChunk{Text: resp.Text, Done: true}
	close(ch)
	return ch, nil
}

func (MockProvider) SupportsTools(model string) bool {
	return model == "" || model == "mock-cyber-agent"
}

func (MockProvider) SupportsVision(model string) bool {
	return false
}

func (MockProvider) SupportsJSONMode(model string) bool {
	return model == "" || strings.Contains(model, "mock")
}

func lastUserMessage(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Content
}
