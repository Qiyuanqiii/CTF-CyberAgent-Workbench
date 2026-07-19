package llm

import (
	"context"
	"sync"
	"testing"
)

type generationProvider struct {
	name    string
	value   string
	started chan struct{}
	release chan struct{}
}

func (p *generationProvider) Name() string { return p.name }
func (p *generationProvider) ListModels(context.Context) ([]ModelInfo, error) {
	return []ModelInfo{{ID: "model", Provider: p.name}}, nil
}
func (p *generationProvider) Chat(ctx context.Context, _ ChatRequest) (*ChatResponse, error) {
	if p.started != nil {
		close(p.started)
	}
	if p.release != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-p.release:
		}
	}
	return &ChatResponse{Text: p.value, Provider: p.name, Model: "model"}, nil
}
func (p *generationProvider) StreamChat(context.Context, ChatRequest) (<-chan ChatChunk, error) {
	values := make(chan ChatChunk)
	close(values)
	return values, nil
}
func (*generationProvider) SupportsTools(string) bool    { return false }
func (*generationProvider) SupportsVision(string) bool   { return false }
func (*generationProvider) SupportsJSONMode(string) bool { return false }

func TestRouterSupportsConcurrentRouteSelectionAndResolution(t *testing.T) {
	router := NewDefaultRouter()
	refs := []ModelRef{
		{Provider: "mock", Model: "mock-code"},
		{Provider: "mock", Model: "mock-fast"},
	}
	var workers sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		workers.Add(1)
		go func(offset int) {
			defer workers.Done()
			for index := 0; index < 100; index++ {
				router.SetRoute("code", refs[(index+offset)%len(refs)])
				ref := router.Resolve("code")
				if ref.Provider != "mock" {
					t.Errorf("resolved unexpected Provider: %#v", ref)
					return
				}
				_ = router.Routes()
				_ = router.ProviderNames()
			}
		}(worker)
	}
	workers.Wait()
	if _, err := router.Chat(context.Background(), "code", ChatRequest{
		Messages: []Message{{Role: "user", Content: "concurrency check"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRouterGenerationSwapDoesNotCancelActiveProviderCall(t *testing.T) {
	oldProvider := &generationProvider{name: "generation", value: "old",
		started: make(chan struct{}), release: make(chan struct{})}
	router := NewRouter(ModelRef{Provider: "generation", Model: "model"})
	router.RegisterProvider(oldProvider)
	router.SetRoute("code", ModelRef{Provider: "generation", Model: "model"})
	oldResult := make(chan *ChatResponse, 1)
	oldError := make(chan error, 1)
	go func() {
		response, err := router.Chat(t.Context(), "code", ChatRequest{
			Messages: []Message{{Role: "user", Content: "old generation"}},
		})
		oldResult <- response
		oldError <- err
	}()
	<-oldProvider.started

	next := NewRouter(ModelRef{Provider: "generation", Model: "model"})
	next.RegisterProvider(&generationProvider{name: "generation", value: "new"})
	next.SetRoute("code", ModelRef{Provider: "generation", Model: "model"})
	if err := router.ReplaceConfiguration(next); err != nil {
		t.Fatal(err)
	}
	current, err := router.Chat(t.Context(), "code", ChatRequest{
		Messages: []Message{{Role: "user", Content: "new generation"}},
	})
	if err != nil || current == nil || current.Text != "new" {
		t.Fatalf("new call did not use the replacement generation: %#v err=%v", current, err)
	}
	close(oldProvider.release)
	if err := <-oldError; err != nil {
		t.Fatal(err)
	}
	if old := <-oldResult; old == nil || old.Text != "old" {
		t.Fatalf("active call was not retained on its original Provider: %#v", old)
	}
}
