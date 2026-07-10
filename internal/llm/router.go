package llm

import (
	"context"
	"fmt"
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

func (r *Router) Chat(ctx context.Context, route string, req ChatRequest) (*ChatResponse, error) {
	ref := r.Resolve(route)
	return r.ChatModelRef(ctx, ref, req)
}

func (r *Router) ChatModelRef(ctx context.Context, ref ModelRef, req ChatRequest) (*ChatResponse, error) {
	provider, ok := r.providers[ref.Provider]
	if !ok {
		return nil, fmt.Errorf("provider %q is not registered", ref.Provider)
	}
	if req.Model == "" {
		req.Model = ref.Model
	}
	req = redactRequest(req)
	return provider.Chat(ctx, req)
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

func redactRequest(req ChatRequest) ChatRequest {
	req.Messages = append([]Message(nil), req.Messages...)
	for i := range req.Messages {
		req.Messages[i].Content = redact.String(req.Messages[i].Content)
	}
	if len(req.Metadata) > 0 {
		metadata := make(map[string]string, len(req.Metadata))
		for key, value := range req.Metadata {
			metadata[key] = redact.String(value)
		}
		req.Metadata = metadata
	}
	return req
}
