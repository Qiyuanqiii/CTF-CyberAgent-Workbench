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

func TestRouterRedactsStreamingRequestBeforeProvider(t *testing.T) {
	provider := &capturingProvider{name: "capture"}
	router := NewRouter(ModelRef{Provider: "capture", Model: "capture-model"})
	router.RegisterProvider(provider)
	token := "t" + "p-" + strings.Repeat("s", 40)
	chunks, err := router.StreamChat(context.Background(), "learn", ChatRequest{
		Messages: []Message{{Role: "user", Content: "MIMO_API_KEY=" + token}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for chunk := range chunks {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
	}
	if provider.last.Model != "capture-model" || strings.Contains(provider.last.Messages[0].Content, token[:12]) ||
		!strings.Contains(provider.last.Messages[0].Content, "[REDACTED:secret]") {
		t.Fatalf("streaming request bypassed router normalization: %#v", provider.last)
	}
}

func TestRouterRedactsStructuredToolTranscriptBeforeProvider(t *testing.T) {
	provider := &capturingProvider{name: "capture"}
	router := NewRouter(ModelRef{Provider: "capture", Model: "capture-model"})
	router.RegisterProvider(provider)
	token := "s" + "k-" + strings.Repeat("r", 28)
	_, err := router.ChatModelRef(t.Context(), ModelRef{Provider: "capture", Model: "capture-model"}, ChatRequest{
		Messages: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{{
				ID: "toolu_0123456789abcdef01234567", Name: "note_create",
				Arguments: json.RawMessage(`{"title":"Provider","content":"token=` + token + `"}`),
			}}},
			{Role: "user", ToolResults: []ToolResult{{
				ToolCallID: "toolu_0123456789abcdef01234567", Content: `{"message":"token=` + token + `"}`,
			}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(provider.last.Messages)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), token) || !strings.Contains(string(encoded), "[REDACTED:") {
		t.Fatalf("structured tool transcript was not redacted: %s", encoded)
	}
}

func TestModelDeltaValidation(t *testing.T) {
	valid := ModelDelta{Sequence: 1, ChunkCount: 2, ByteCount: 10, TotalBytes: 10}
	if err := valid.Validate(MaxModelDeltaEvents, MaxModelOutputBytes); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []ModelDelta{
		{Sequence: 0, ChunkCount: 1, ByteCount: 1, TotalBytes: 1},
		{Sequence: MaxModelDeltaEvents + 1, ChunkCount: 1, ByteCount: 1, TotalBytes: 1},
		{Sequence: 1, ChunkCount: 1, ByteCount: 2, TotalBytes: 1},
		{Sequence: 1, ChunkCount: 1, ByteCount: 1, TotalBytes: MaxModelOutputBytes + 1},
		{Sequence: 1},
	} {
		if err := invalid.Validate(MaxModelDeltaEvents, MaxModelOutputBytes); err == nil {
			t.Fatalf("invalid model delta accepted: %#v", invalid)
		}
	}
}

func TestUsageValidationRejectsNegativeAndOverflow(t *testing.T) {
	if err := (Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3}).Validate(); err != nil {
		t.Fatal(err)
	}
	maxInt := int(^uint(0) >> 1)
	for _, usage := range []Usage{
		{InputTokens: -1},
		{OutputTokens: -1},
		{TotalTokens: -1},
		{InputTokens: maxInt, OutputTokens: 1, TotalTokens: maxInt},
	} {
		if err := usage.Validate(); err == nil {
			t.Fatalf("invalid usage accepted: %#v", usage)
		}
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
	p.last = req
	response := &ChatResponse{Text: "ok", Provider: p.name, Model: req.Model}
	ch := make(chan ChatChunk, 2)
	ch <- ChatChunk{Text: response.Text}
	ch <- FinalChatChunk(response)
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
