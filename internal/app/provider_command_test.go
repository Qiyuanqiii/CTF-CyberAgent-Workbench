package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeepSeekProviderRegistersFromEnvironmentAndUsesDefaults(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-api-key")
	t.Setenv("DEEPSEEK_MODEL", "")
	requestSeen := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		if request.URL.Path != "/v1/messages" {
			t.Errorf("request path = %q, want /v1/messages", request.URL.Path)
		}
		if request.Header.Get("x-api-key") != "test-deepseek-api-key" {
			t.Error("DeepSeek API key header was not populated")
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != defaultDeepSeekModel {
			t.Errorf("request model = %q, want %q", body.Model, defaultDeepSeekModel)
		}
		requestSeen <- struct{}{}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg-deepseek-test","type":"message","role":"assistant",
			"content":[{"type":"text","text":"provider healthy"}],
			"model":"deepseek-v4-flash","stop_reason":"end_turn",
			"usage":{"input_tokens":3,"output_tokens":2}
		}`))
	}))
	defer server.Close()
	t.Setenv("DEEPSEEK_BASE_URL", server.URL)

	listed, stderr, code := executeTestCommand(t, "provider", "list")
	if code != 0 || stderr != "" || !strings.Contains(listed, "deepseek") {
		t.Fatalf("DeepSeek provider was not listed: output=%q stderr=%q code=%d", listed, stderr, code)
	}
	output, stderr, code := executeTestCommand(t, "provider", "test", "deepseek/"+defaultDeepSeekModel)
	if code != 0 || stderr != "" || !strings.Contains(output, "provider: deepseek") ||
		!strings.Contains(output, "model: deepseek-v4-flash") ||
		!strings.Contains(output, "status: reachable") ||
		!strings.Contains(output, "response_content_returned: false") {
		t.Fatalf("unexpected DeepSeek test output=%q stderr=%q code=%d", output, stderr, code)
	}
	if strings.Contains(output, "provider healthy") || strings.Contains(output, "response:") {
		t.Fatal("Provider diagnostic exposed model response content")
	}
	if strings.Contains(output, "test-deepseek-api-key") || strings.Contains(stderr, "test-deepseek-api-key") {
		t.Fatal("DeepSeek API key leaked into CLI output")
	}
	select {
	case <-requestSeen:
	default:
		t.Fatal("DeepSeek-compatible endpoint was not called")
	}
}

func TestDeepSeekProviderRequiresEnvironmentKey(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	listed, stderr, code := executeTestCommand(t, "provider", "list")
	if code != 0 || stderr != "" {
		t.Fatalf("provider list failed: output=%q stderr=%q code=%d", listed, stderr, code)
	}
	for _, name := range strings.Fields(listed) {
		if name == "deepseek" {
			t.Fatal("DeepSeek provider was registered without an API key")
		}
	}
}
