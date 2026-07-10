package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMockProviderDeterministicResponse(t *testing.T) {
	provider := NewMockProvider()
	req := ChatRequest{Model: "mock-code", Messages: []Message{{Role: "user", Content: "build parser"}}}

	first, err := provider.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := provider.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if first.Text != second.Text {
		t.Fatalf("mock response should be deterministic: %q != %q", first.Text, second.Text)
	}
	if first.Model != "mock-code" || first.Provider != "mock" {
		t.Fatalf("unexpected model/provider: %s/%s", first.Provider, first.Model)
	}
}

func TestMockProviderRootLifecycleResponse(t *testing.T) {
	provider := NewMockProvider()
	response, err := provider.Chat(context.Background(), ChatRequest{
		Model: "mock-code", JSONMode: true,
		Messages: []Message{{Role: "user", Content: "review code"}},
		Metadata: map[string]string{"response_schema": "root_lifecycle.v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var action struct {
		Version string `json:"version"`
		Action  string `json:"action"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(response.Text), &action); err != nil {
		t.Fatalf("mock lifecycle response is not JSON: %v", err)
	}
	if action.Version != "root_lifecycle.v1" || action.Action != "continue" || action.Message == "" {
		t.Fatalf("unexpected mock lifecycle response: %#v", action)
	}
}

func TestRouterDefaultRoutes(t *testing.T) {
	router := NewDefaultRouter()
	resp, err := router.Chat(context.Background(), "script", ChatRequest{
		Messages: []Message{{Role: "user", Content: "write script"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Model != "mock-code" {
		t.Fatalf("script route should use mock-code, got %s", resp.Model)
	}
}

func TestRouterRedactsRequestBeforeProvider(t *testing.T) {
	provider := &capturingProvider{name: "capture"}
	router := NewRouter(ModelRef{Provider: "capture", Model: "capture-model"})
	router.RegisterProvider(provider)
	mimoToken := "t" + "p-" + strings.Repeat("a", 40)
	openAIToken := "s" + "k-" + "abcdefghijklmnopqrstuvwxyz123456"
	openAIPrefix := "s" + "k-" + "abcdefghijklmnopqrstuvwxyz"
	_, err := router.Chat(context.Background(), "learn", ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "MIMO_API_KEY=" + mimoToken},
		},
		Metadata: map[string]string{"note": "OPENAI_API_KEY=" + openAIToken},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.last.Messages) != 1 {
		t.Fatalf("provider did not receive message: %#v", provider.last)
	}
	if strings.Contains(provider.last.Messages[0].Content, mimoToken[:11]) {
		t.Fatalf("provider received raw token: %#v", provider.last.Messages)
	}
	if strings.Contains(provider.last.Metadata["note"], openAIPrefix) {
		t.Fatalf("provider received raw metadata secret: %#v", provider.last.Metadata)
	}
	if !strings.Contains(provider.last.Messages[0].Content, "[REDACTED:secret]") {
		t.Fatalf("provider request missing redaction marker: %#v", provider.last.Messages)
	}
}

type capturingProvider struct {
	name string
	last ChatRequest
}

func (p *capturingProvider) Name() string {
	return p.name
}

func (p *capturingProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	return []ModelInfo{{Provider: p.name, ID: "capture-model"}}, nil
}

func (p *capturingProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	p.last = req
	return &ChatResponse{Text: "ok", Provider: p.name, Model: req.Model}, nil
}

func (p *capturingProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error) {
	ch := make(chan ChatChunk, 1)
	ch <- ChatChunk{Text: "ok", Done: true}
	close(ch)
	return ch, nil
}

func (p *capturingProvider) SupportsTools(model string) bool {
	return false
}

func (p *capturingProvider) SupportsVision(model string) bool {
	return false
}

func (p *capturingProvider) SupportsJSONMode(model string) bool {
	return false
}
