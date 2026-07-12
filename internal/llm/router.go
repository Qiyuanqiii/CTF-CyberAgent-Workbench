package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"cyberagent-workbench/internal/redact"
)

type ModelRef struct {
	Provider string
	Model    string
}

type Router struct {
	providers  map[string]Provider
	routes     map[string]ModelRef
	defaultRef ModelRef
}

func NewRouter(defaultRef ModelRef) *Router {
	return &Router{
		providers:  map[string]Provider{},
		routes:     map[string]ModelRef{},
		defaultRef: defaultRef,
	}
}

func NewDefaultRouter() *Router {
	r := NewRouter(ModelRef{Provider: "mock", Model: "mock-cyber-agent"})
	r.RegisterProvider(NewMockProvider())
	r.SetRoute("ctf", ModelRef{Provider: "mock", Model: "mock-cyber-agent"})
	r.SetRoute("script", ModelRef{Provider: "mock", Model: "mock-code"})
	r.SetRoute("learn", ModelRef{Provider: "mock", Model: "mock-fast"})
	r.SetRoute("code", ModelRef{Provider: "mock", Model: "mock-code"})
	r.SetRoute("review", ModelRef{Provider: "mock", Model: "mock-cyber-agent"})
	return r
}

func (r *Router) RegisterProvider(provider Provider) {
	r.providers[provider.Name()] = provider
}

func (r *Router) ProviderNames() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Router) Routes() map[string]ModelRef {
	out := make(map[string]ModelRef, len(r.routes))
	for k, v := range r.routes {
		out[k] = v
	}
	return out
}

func (r *Router) SetRoute(name string, ref ModelRef) {
	r.routes[name] = ref
}

func (r *Router) Resolve(route string) ModelRef {
	if ref, ok := r.routes[route]; ok {
		return ref
	}
	return r.defaultRef
}

func (r *Router) SupportsJSONMode(ref ModelRef) bool {
	provider, ok := r.providers[strings.TrimSpace(ref.Provider)]
	return ok && provider.SupportsJSONMode(strings.TrimSpace(ref.Model))
}

func (r *Router) Chat(ctx context.Context, route string, req ChatRequest) (*ChatResponse, error) {
	ref := r.Resolve(route)
	return r.ChatModelRef(ctx, ref, req)
}

func (r *Router) ChatModelRef(ctx context.Context, ref ModelRef, req ChatRequest) (*ChatResponse, error) {
	provider, ok := r.providers[ref.Provider]
	if !ok {
		return nil, NewProviderError(OutcomePermanent, ref.Provider, fmt.Sprintf("provider %q is not registered", ref.Provider), nil)
	}
	if req.Model == "" {
		req.Model = ref.Model
	}
	req, err := redactRequest(req)
	if err != nil {
		return nil, NewProviderError(OutcomePermanent, ref.Provider, "invalid model request", err)
	}
	response, err := provider.Chat(ctx, req)
	if err != nil {
		return nil, NormalizeProviderError(ref.Provider, err)
	}
	return response, nil
}

func (r *Router) StreamChat(ctx context.Context, route string, req ChatRequest) (<-chan ChatChunk, error) {
	return r.StreamChatModelRef(ctx, r.Resolve(route), req)
}

func (r *Router) StreamChatModelRef(ctx context.Context, ref ModelRef, req ChatRequest) (<-chan ChatChunk, error) {
	provider, ok := r.providers[ref.Provider]
	if !ok {
		return nil, NewProviderError(OutcomePermanent, ref.Provider, fmt.Sprintf("provider %q is not registered", ref.Provider), nil)
	}
	if req.Model == "" {
		req.Model = ref.Model
	}
	var err error
	req, err = redactRequest(req)
	if err != nil {
		return nil, NewProviderError(OutcomePermanent, ref.Provider, "invalid model request", err)
	}
	chunks, err := provider.StreamChat(ctx, req)
	if err != nil {
		return nil, NormalizeProviderError(ref.Provider, err)
	}
	if chunks == nil {
		return nil, NewProviderError(OutcomeInvalidResponse, ref.Provider, "returned a nil stream", nil)
	}
	return chunks, nil
}

func (r *Router) ListModels(ctx context.Context) ([]ModelInfo, error) {
	var all []ModelInfo
	for _, name := range r.ProviderNames() {
		models, err := r.providers[name].ListModels(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, models...)
	}
	return all, nil
}

func ParseModelRef(value string) (ModelRef, error) {
	parts := strings.SplitN(value, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ModelRef{}, fmt.Errorf("model ref must use provider/model format")
	}
	return ModelRef{Provider: parts[0], Model: parts[1]}, nil
}

func redactRequest(req ChatRequest) (ChatRequest, error) {
	req.Messages = append([]Message(nil), req.Messages...)
	for i := range req.Messages {
		req.Messages[i].Content = redact.String(req.Messages[i].Content)
		calls, err := NormalizeToolCalls(req.Messages[i].ToolCalls)
		if err != nil {
			return ChatRequest{}, err
		}
		for index := range calls {
			calls[index].Arguments, err = redactModelJSON(calls[index].Arguments)
			if err != nil {
				return ChatRequest{}, err
			}
		}
		req.Messages[i].ToolCalls = calls
		results := make([]ToolResult, len(req.Messages[i].ToolResults))
		for index, result := range req.Messages[i].ToolResults {
			normalized, err := NormalizeToolResult(result)
			if err != nil {
				return ChatRequest{}, err
			}
			normalized.Content = redact.String(normalized.Content)
			results[index] = normalized
		}
		req.Messages[i].ToolResults = results
	}
	req.Tools = append([]ToolSpec(nil), req.Tools...)
	for index := range req.Tools {
		req.Tools[index].Name = strings.TrimSpace(req.Tools[index].Name)
		req.Tools[index].Description = redact.String(strings.TrimSpace(req.Tools[index].Description))
		req.Tools[index].Parameters = append(json.RawMessage(nil), bytes.TrimSpace(req.Tools[index].Parameters)...)
		if req.Tools[index].Name == "" || len(req.Tools[index].Parameters) == 0 || !json.Valid(req.Tools[index].Parameters) {
			return ChatRequest{}, fmt.Errorf("model tool specification at index %d is invalid", index)
		}
	}
	if len(req.Metadata) > 0 {
		metadata := make(map[string]string, len(req.Metadata))
		for key, value := range req.Metadata {
			metadata[key] = redact.String(value)
		}
		req.Metadata = metadata
	}
	return req, nil
}

func redactModelJSON(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := ensureModelJSONEOF(decoder); err != nil {
		return nil, err
	}
	nodes := 0
	safe, err := redactModelJSONValue(value, 0, &nodes)
	if err != nil {
		return nil, err
	}
	return json.Marshal(safe)
}

func ensureModelJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return fmt.Errorf("model tool JSON contains trailing data")
}

func redactModelJSONValue(value any, depth int, nodes *int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("model tool JSON exceeds depth limit")
	}
	(*nodes)++
	if *nodes > 100000 {
		return nil, fmt.Errorf("model tool JSON exceeds node limit")
	}
	switch current := value.(type) {
	case string:
		return redact.String(current), nil
	case []any:
		out := make([]any, len(current))
		for index, item := range current {
			redacted, err := redactModelJSONValue(item, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			out[index] = redacted
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(current))
		for key, item := range current {
			redacted, err := redactModelJSONValue(item, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			out[key] = redacted
		}
		return out, nil
	default:
		return value, nil
	}
}
