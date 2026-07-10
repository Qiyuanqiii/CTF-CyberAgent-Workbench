package llm

import (
	"context"
	"encoding/json"
	"errors"
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
