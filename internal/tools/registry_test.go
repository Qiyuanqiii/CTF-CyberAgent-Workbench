package tools

import (
	"context"
	"errors"
	"testing"
	"time"
)

type blockingTool struct {
	release <-chan struct{}
}

func (blockingTool) Name() string { return "blocking" }

func (blockingTool) Schema() Schema { return Schema{} }

func (t blockingTool) Run(context.Context, Call) (Result, error) {
	<-t.release
	return Result{Stdout: "released"}, nil
}

func TestRegistryEnforcesHardExecutionTimeout(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	registry := NewRegistry().WithExecutionTimeout(25 * time.Millisecond)
	registry.Register(blockingTool{release: release})

	started := time.Now()
	result, err := registry.Run(context.Background(), Call{Name: "blocking"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got result=%#v err=%v", result, err)
	}
	if result.ExitCode != 124 || time.Since(started) > time.Second {
		t.Fatalf("hard timeout was not enforced: result=%#v elapsed=%s", result, time.Since(started))
	}
}

func TestRegistryPropagatesCallerCancellation(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	registry := NewRegistry()
	registry.Register(blockingTool{release: release})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := registry.Run(ctx, Call{Name: "blocking"})
	if !errors.Is(err, context.Canceled) || result.ExitCode != 130 {
		t.Fatalf("expected cancellation result, got result=%#v err=%v", result, err)
	}
}

func TestRegistryRecoversToolPanic(t *testing.T) {
	registry := NewRegistry()
	registry.Register(panicTool{})
	result, err := registry.Run(context.Background(), Call{Name: "panic"})
	if err == nil || result.ExitCode != 1 {
		t.Fatalf("expected recovered panic, got result=%#v err=%v", result, err)
	}
}

type panicTool struct{}

func (panicTool) Name() string                              { return "panic" }
func (panicTool) Schema() Schema                            { return Schema{} }
func (panicTool) Run(context.Context, Call) (Result, error) { panic("test panic") }
