package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cyberagent-workbench/internal/redact"
)

const AnthropicVersion = "2023-06-01"

type AnthropicCompatibleConfig struct {
	Name         string
	BaseURL      string
	APIKey       string
	DefaultModel string
	HTTPClient   *http.Client
}

type AnthropicCompatibleProvider struct {
	name         string
	baseURL      string
	apiKey       string
	defaultModel string
	client       *http.Client
}

func NewAnthropicCompatibleProvider(config AnthropicCompatibleConfig) (*AnthropicCompatibleProvider, error) {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		name = "anthropic_compatible"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("base url is required for provider %s", name)
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid base url for provider %s: %w", name, err)
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("api key is required for provider %s", name)
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	defaultModel := strings.TrimSpace(config.DefaultModel)
	if defaultModel == "" {
		defaultModel = "claude-3-5-sonnet-latest"
	}
	return &AnthropicCompatibleProvider{
		name:         name,
		baseURL:      baseURL,
		apiKey:       config.APIKey,
		defaultModel: defaultModel,
		client:       client,
	}, nil
}

func (p *AnthropicCompatibleProvider) Name() string {
	return p.name
}

func (p *AnthropicCompatibleProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint("/v1/models"), nil)
	if err != nil {
		return nil, err
	}
	p.addHeaders(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return fallbackModelInfo(p.name, p.defaultModel), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fallbackModelInfo(p.name, p.defaultModel), nil
	}
	var payload struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fallbackModelInfo(p.name, p.defaultModel), nil
	}
	if len(payload.Data) == 0 {
		return fallbackModelInfo(p.name, p.defaultModel), nil
	}
	models := make([]ModelInfo, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.ID == "" {
			continue
		}
		display := item.DisplayName
		if display == "" {
			display = item.ID
		}
		models = append(models, ModelInfo{
			ID:           item.ID,
			DisplayName:  display,
			Provider:     p.name,
			Capabilities: []string{"chat"},
		})
	}
	if len(models) == 0 {
		return fallbackModelInfo(p.name, p.defaultModel), nil
	}
	return models, nil
}

func (p *AnthropicCompatibleProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.defaultModel
	}
	body := p.toRequest(model, req)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint("/v1/messages"), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	p.addHeaders(httpReq)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider %s returned HTTP %d: %s", p.name, resp.StatusCode, trimForError(redact.String(string(raw)), 700))
	}
	var parsed anthropicMessageResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	text := parsed.Text()
	return &ChatResponse{
		Text:     text,
		Raw:      raw,
		Model:    parsed.ModelOrDefault(model),
		Provider: p.name,
		Usage: Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
			TotalTokens:  parsed.Usage.InputTokens + parsed.Usage.OutputTokens,
		},
	}, nil
}

func (p *AnthropicCompatibleProvider) StreamChat(ctx context.Context, req ChatRequest) (<-chan ChatChunk, error) {
	resp, err := p.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan ChatChunk, 1)
	ch <- ChatChunk{Text: resp.Text, Done: true}
	close(ch)
	return ch, nil
}

func (p *AnthropicCompatibleProvider) SupportsTools(model string) bool {
	return false
}

func (p *AnthropicCompatibleProvider) SupportsVision(model string) bool {
	return false
}

func (p *AnthropicCompatibleProvider) SupportsJSONMode(model string) bool {
	return false
}

func (p *AnthropicCompatibleProvider) toRequest(model string, req ChatRequest) anthropicMessageRequest {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}
	out := anthropicMessageRequest{
		Model:     model,
		MaxTokens: maxTokens,
		Messages:  make([]anthropicMessage, 0, len(req.Messages)),
	}
	if req.Temperature > 0 {
		out.Temperature = &req.Temperature
	}
	var systemParts []string
	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		switch role {
		case "system":
			systemParts = append(systemParts, content)
		case "assistant":
			out.Messages = append(out.Messages, anthropicMessage{Role: "assistant", Content: content})
		default:
			out.Messages = append(out.Messages, anthropicMessage{Role: "user", Content: content})
		}
	}
	if len(systemParts) > 0 {
		out.System = strings.Join(systemParts, "\n\n")
	}
	if len(out.Messages) == 0 {
		out.Messages = append(out.Messages, anthropicMessage{Role: "user", Content: "Hello"})
	}
	return out
}

func (p *AnthropicCompatibleProvider) endpoint(path string) string {
	if strings.HasSuffix(p.baseURL, path) {
		return p.baseURL
	}
	if strings.HasSuffix(p.baseURL, "/v1") && strings.HasPrefix(path, "/v1/") {
		return p.baseURL + strings.TrimPrefix(path, "/v1")
	}
	return p.baseURL + path
}

func (p *AnthropicCompatibleProvider) addHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", AnthropicVersion)
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
}

type anthropicMessageRequest struct {
	Model       string             `json:"model"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicMessageResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (r anthropicMessageResponse) Text() string {
	var parts []string
	for _, item := range r.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func (r anthropicMessageResponse) ModelOrDefault(model string) string {
	if strings.TrimSpace(r.Model) == "" {
		return model
	}
	return r.Model
}

func fallbackModelInfo(provider string, model string) []ModelInfo {
	if model == "" {
		model = "claude-3-5-sonnet-latest"
	}
	return []ModelInfo{{
		ID:           model,
		DisplayName:  model,
		Provider:     provider,
		Capabilities: []string{"chat"},
	}}
}

func trimForError(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
