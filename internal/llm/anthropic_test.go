package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestAnthropicCompatibleProviderStreamsSSEWithFinalUsage(t *testing.T) {
	var captured anthropicMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("missing streaming accept header: %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("test server does not support flushing")
		}
		for _, payload := range []string{
			`{"type":"message_start","message":{"model":"stream-model","usage":{"input_tokens":7,"output_tokens":0}}}`,
			`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello "}}`,
			`{"type":"content_block_delta","delta":{"type":"text_delta","text":"world"}}`,
			`{"type":"message_delta","usage":{"output_tokens":2}}`,
			`{"type":"message_stop"}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}))
	defer server.Close()
	provider, err := NewAnthropicCompatibleProvider(AnthropicCompatibleConfig{
		Name: "test", BaseURL: server.URL, APIKey: "secret", DefaultModel: "fallback-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := provider.StreamChat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	var final ChatChunk
	for chunk := range chunks {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		text.WriteString(chunk.Text)
		if chunk.Done {
			final = chunk
		}
	}
	if !captured.Stream || text.String() != "hello world" || !final.Done || final.Usage == nil ||
		final.Usage.InputTokens != 7 || final.Usage.OutputTokens != 2 || final.Usage.TotalTokens != 9 ||
		final.Provider != "test" || final.Model != "stream-model" {
		t.Fatalf("unexpected stream captured=%#v text=%q final=%#v", captured, text.String(), final)
	}
}

func TestAnthropicCompatibleProviderMapsToolsAndToolResults(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_tool","type":"message","role":"assistant","model":"tool-model",
			"content":[{"type":"tool_use","id":"provider-tool-2","name":"note_create",
				"input":{"title":"Decision","content":"Use strict JSON"}}],
			"usage":{"input_tokens":9,"output_tokens":4}
		}`))
	}))
	defer server.Close()
	provider, err := NewAnthropicCompatibleProvider(AnthropicCompatibleConfig{
		Name: "test", BaseURL: server.URL, APIKey: "secret", DefaultModel: "tool-model",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := provider.Chat(t.Context(), ChatRequest{
		Messages: []Message{
			{Role: "user", Content: "plan"},
			{Role: "assistant", ToolCalls: []ToolCall{{
				ID: "toolu_0123456789abcdef01234567", Name: "work_item_create",
				Arguments: json.RawMessage(`{"title":"Inspect parser"}`),
			}}},
			{Role: "user", ToolResults: []ToolResult{{
				ToolCallID: "toolu_0123456789abcdef01234567", Content: `{"status":"completed"}`,
			}}},
		},
		Tools: []ToolSpec{{
			Name: "note_create", Description: "Create a Note",
			Parameters: json.RawMessage(`{"type":"object","required":["title","content"]}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tools, ok := captured["tools"].([]any)
	messages, messagesOK := captured["messages"].([]any)
	if !ok || len(tools) != 1 || !messagesOK || len(messages) != 3 || !provider.SupportsTools("tool-model") {
		t.Fatalf("Anthropic tool request was not encoded: %#v", captured)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].ID != "provider-tool-2" ||
		response.ToolCalls[0].Name != "note_create" || !json.Valid(response.ToolCalls[0].Arguments) ||
		response.Usage.TotalTokens != 13 || response.Text != "" {
		t.Fatalf("Anthropic tool response was not decoded: %#v", response)
	}
}

func TestAnthropicCompatibleProviderStreamsToolUseJSONDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, payload := range []string{
			`{"type":"message_start","message":{"model":"tool-stream","usage":{"input_tokens":6,"output_tokens":0}}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"provider-stream-1","name":"work_item_create","input":{}}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"title\":"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Stream plan\"}"}}`,
			`{"type":"content_block_stop","index":0}`,
			`{"type":"message_delta","usage":{"output_tokens":3}}`,
			`{"type":"message_stop"}`,
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}))
	defer server.Close()
	provider, err := NewAnthropicCompatibleProvider(AnthropicCompatibleConfig{
		Name: "test", BaseURL: server.URL, APIKey: "secret", DefaultModel: "tool-stream",
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := provider.StreamChat(t.Context(), ChatRequest{Messages: []Message{{Role: "user", Content: "plan"}}})
	if err != nil {
		t.Fatal(err)
	}
	var final ChatChunk
	for chunk := range chunks {
		if chunk.Err != nil {
			t.Fatal(chunk.Err)
		}
		if chunk.Done {
			final = chunk
		}
	}
	if !final.Done || final.Usage == nil || final.Usage.TotalTokens != 9 || len(final.ToolCalls) != 1 ||
		final.ToolCalls[0].ID != "provider-stream-1" ||
		string(final.ToolCalls[0].Arguments) != `{"title":"Stream plan"}` {
		t.Fatalf("unexpected streamed tool call: %#v", final)
	}
}

func TestAnthropicCompatibleProviderReportsMalformedStreamEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: not-json\n\n"))
	}))
	defer server.Close()
	provider, err := NewAnthropicCompatibleProvider(AnthropicCompatibleConfig{
		Name: "test", BaseURL: server.URL, APIKey: "secret", DefaultModel: "model",
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := provider.StreamChat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	chunk, ok := <-chunks
	if !ok || ProviderErrorKind(chunk.Err) != OutcomeInvalidResponse {
		t.Fatalf("malformed SSE was not typed: ok=%t chunk=%#v", ok, chunk)
	}
}

func TestAnthropicCompatibleProviderClassifiesHTTPFailures(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		retryAfter string
		want       Outcome
	}{
		{name: "rate limit", statusCode: http.StatusTooManyRequests, retryAfter: "7", want: OutcomeRateLimited},
		{name: "unavailable", statusCode: http.StatusServiceUnavailable, want: OutcomeRetryable},
		{name: "overloaded", statusCode: 529, want: OutcomeRetryable},
		{name: "auth", statusCode: http.StatusUnauthorized, want: OutcomePermanent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if test.retryAfter != "" {
					w.Header().Set("Retry-After", test.retryAfter)
				}
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(`{"error":"temporary"}`))
			}))
			defer server.Close()
			provider, err := NewAnthropicCompatibleProvider(AnthropicCompatibleConfig{
				Name: "test", BaseURL: server.URL, APIKey: "secret", DefaultModel: "model",
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = provider.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
			var providerErr *ProviderError
			if !errors.As(err, &providerErr) || providerErr.Kind != test.want || providerErr.StatusCode != test.statusCode {
				t.Fatalf("unexpected provider error: %#v err=%v", providerErr, err)
			}
			if test.retryAfter != "" && providerErr.RetryAfter != 7*time.Second {
				t.Fatalf("retry after = %s, want 7s", providerErr.RetryAfter)
			}
		})
	}
}

func TestAnthropicCompatibleProviderRejectsMalformedAndEmptyResponses(t *testing.T) {
	for _, body := range []string{`not-json`, `{"model":"model","content":[]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		provider, err := NewAnthropicCompatibleProvider(AnthropicCompatibleConfig{
			Name: "test", BaseURL: server.URL, APIKey: "secret", DefaultModel: "model",
		})
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		_, err = provider.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hello"}}})
		server.Close()
		if ProviderErrorKind(err) != OutcomeInvalidResponse {
			t.Fatalf("body %q outcome=%s err=%v", body, ProviderErrorKind(err), err)
		}
	}
}

func TestAnthropicHTTPErrorRedactsResponseBody(t *testing.T) {
	token := "t" + "p-" + strings.Repeat("b", 40)
	err := anthropicHTTPError("mimo", http.StatusTooManyRequests, "", []byte("MIMO_API_KEY="+token))
	if strings.Contains(err.Error(), token[:12]) || !strings.Contains(err.Error(), "[REDACTED:") {
		t.Fatalf("HTTP error leaked response secret: %q", err.Error())
	}
}

func TestParseRetryAfterSupportsSecondsAndHTTPDate(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	if got := parseRetryAfter("4", now); got != 4*time.Second {
		t.Fatalf("seconds retry-after = %s", got)
	}
	when := now.Add(5 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(when, now); got != 5*time.Second {
		t.Fatalf("date retry-after = %s", got)
	}
	if got := parseRetryAfter("9223372036854775807", now); got <= 0 {
		t.Fatalf("overflow retry-after = %s", got)
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
