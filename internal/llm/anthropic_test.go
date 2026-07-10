package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnthropicCompatibleProviderChat(t *testing.T) {
	var captured struct {
		Path             string
		APIKey           string
		AnthropicVersion string
		Body             anthropicMessageRequest
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Path = r.URL.Path
		captured.APIKey = r.Header.Get("x-api-key")
		captured.AnthropicVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&captured.Body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_test",
			"type":"message",
			"role":"assistant",
			"model":"test-model",
			"content":[{"type":"text","text":"hello from provider"}],
			"usage":{"input_tokens":7,"output_tokens":3}
		}`))
	}))
	defer server.Close()

	provider, err := NewAnthropicCompatibleProvider(AnthropicCompatibleConfig{
		Name:         "test",
		BaseURL:      server.URL,
		APIKey:       "secret",
		DefaultModel: "test-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "say hello"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Path != "/v1/messages" {
		t.Fatalf("unexpected path: %s", captured.Path)
	}
	if captured.APIKey != "secret" {
		t.Fatalf("missing api key header")
	}
	if captured.AnthropicVersion != AnthropicVersion {
		t.Fatalf("unexpected anthropic version: %s", captured.AnthropicVersion)
	}
	if captured.Body.System != "be concise" {
		t.Fatalf("system prompt not lifted: %#v", captured.Body)
	}
	if len(captured.Body.Messages) != 1 || captured.Body.Messages[0].Role != "user" || captured.Body.Messages[0].Content != "say hello" {
		t.Fatalf("unexpected messages: %#v", captured.Body.Messages)
	}
	if resp.Text != "hello from provider" || resp.Usage.TotalTokens != 10 {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestAnthropicCompatibleProviderListModelsFallback(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	provider, err := NewAnthropicCompatibleProvider(AnthropicCompatibleConfig{
		Name:         "test",
		BaseURL:      server.URL,
		APIKey:       "secret",
		DefaultModel: "fallback-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "fallback-model" {
		t.Fatalf("unexpected fallback models: %#v", models)
	}
}
