package waitgraph

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestGraphRejectsDirectAndIndirectCycles(t *testing.T) {
	graph := New()
	if _, err := graph.Acquire(Agent("root"), Agent("root")); !errors.Is(err, ErrCycle) {
		t.Fatalf("expected self-cycle rejection, got %v", err)
	}
	releaseAB, err := graph.Acquire(Agent("a"), Tool("b"))
	if err != nil {
		t.Fatal(err)
	}
	defer releaseAB()
	releaseBC, err := graph.Acquire(Tool("b"), Retriever("c"))
	if err != nil {
		t.Fatal(err)
	}
	defer releaseBC()
	if _, err := graph.Acquire(Retriever("c"), Agent("a")); !errors.Is(err, ErrReverseAgentWait) {
		t.Fatalf("expected reverse Agent wait rejection, got %v", err)
	}
	if _, err := graph.Acquire(Retriever("c"), External("a")); err != nil {
		t.Fatalf("unrelated edge should remain valid: %v", err)
	}
	if _, err := graph.Acquire(Retriever("c"), Tool("b")); !errors.Is(err, ErrCycle) {
		t.Fatalf("expected indirect cycle rejection, got %v", err)
	}
}

func TestGraphReleaseIsIdempotentAndRemovesCyclePath(t *testing.T) {
	graph := New()
	release, err := graph.Acquire(Agent("a"), Tool("b"))
	if err != nil {
		t.Fatal(err)
	}
	release()
	release()
	reverse, err := graph.Acquire(Tool("b"), External("a"))
	if err != nil {
		t.Fatal(err)
	}
	reverse()
}

func TestGraphRejectsLowerLayerAgentCallbacks(t *testing.T) {
	graph := New()
	for _, source := range []Node{Tool("tool"), Retriever("rag"), Store("sqlite"), Runner("docker")} {
		if _, err := graph.Acquire(source, Agent("root")); !errors.Is(err, ErrReverseAgentWait) {
			t.Fatalf("%s callback was not rejected: %v", source.Kind, err)
		}
	}
}

func TestGraphConcurrentAcquireRelease(t *testing.T) {
	graph := New()
	var group sync.WaitGroup
	for index := 0; index < 64; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			release, err := graph.Acquire(Agent("root"), Tool("shared"))
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			release()
		}()
	}
	group.Wait()
}

func TestEnterPropagatesCurrentNode(t *testing.T) {
	graph := New()
	ctx := WithCurrent(context.Background(), Agent("root"))
	ctx, release, err := Enter(ctx, graph, External("fallback"), Tool("read"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	current, ok := Current(ctx)
	if !ok || current != Tool("read") {
		t.Fatalf("unexpected current node: %#v found=%t", current, ok)
	}
}
