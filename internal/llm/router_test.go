package llm

import (
	"context"
	"sync"
	"testing"
)

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
