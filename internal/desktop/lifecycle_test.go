package desktop

import (
	"context"
	"sync"
	"testing"
	"time"
)

type recordingWindowRestorer struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingWindowRestorer) Unminimise(context.Context) {
	r.mu.Lock()
	r.calls = append(r.calls, "unminimise")
	r.mu.Unlock()
}

func (r *recordingWindowRestorer) Show(context.Context) {
	r.mu.Lock()
	r.calls = append(r.calls, "show")
	r.mu.Unlock()
}

func (r *recordingWindowRestorer) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func TestLifecycleCoalescesEarlyRestoreAndStopsPermanently(t *testing.T) {
	restorer := &recordingWindowRestorer{}
	lifecycle := NewLifecycle(restorer)
	lifecycle.RequestRestore()
	lifecycle.RequestRestore()
	if len(restorer.snapshot()) != 0 {
		t.Fatal("restore ran before the native lifecycle was ready")
	}

	lifecycle.Start(context.Background())
	if got := restorer.snapshot(); len(got) != 2 || got[0] != "unminimise" || got[1] != "show" {
		t.Fatalf("early restore was not coalesced in order: %#v", got)
	}
	if lifecycle.Context() == nil {
		t.Fatal("lifecycle context is unavailable after startup")
	}
	lifecycle.RequestRestore()
	if got := restorer.snapshot(); len(got) != 4 || got[2] != "unminimise" || got[3] != "show" {
		t.Fatalf("active restore is invalid: %#v", got)
	}

	lifecycle.Stop()
	lifecycle.Stop()
	lifecycle.RequestRestore()
	lifecycle.Start(context.Background())
	if lifecycle.Context() != nil || len(restorer.snapshot()) != 4 {
		t.Fatal("stopped lifecycle was restarted or restored")
	}
}

func TestLifecycleRejectsCancelledContexts(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	restorer := &recordingWindowRestorer{}
	lifecycle := NewLifecycle(restorer)
	lifecycle.Start(parent)
	cancel()
	if lifecycle.Context() != nil {
		t.Fatal("cancelled lifecycle context remained available")
	}
	lifecycle.RequestRestore()
	if len(restorer.snapshot()) != 0 {
		t.Fatal("cancelled lifecycle restored a native window")
	}
	lifecycle.Stop()
}

func TestLifecycleConcurrentRestoreAndStopIsRaceFree(t *testing.T) {
	restorer := &recordingWindowRestorer{}
	lifecycle := NewLifecycle(restorer)
	lifecycle.Start(context.Background())
	var wait sync.WaitGroup
	for index := 0; index < 32; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			lifecycle.RequestRestore()
		}()
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		lifecycle.Stop()
	}()
	wait.Wait()
	if lifecycle.Context() != nil {
		t.Fatal("concurrent stop left the lifecycle active")
	}
}

type blockingWindowRestorer struct {
	entered chan struct{}
	release chan struct{}
}

func (r *blockingWindowRestorer) Unminimise(context.Context) {
	close(r.entered)
	<-r.release
}

func (*blockingWindowRestorer) Show(context.Context) {}

func TestLifecycleStopWaitsForAnActiveNativeRestore(t *testing.T) {
	restorer := &blockingWindowRestorer{entered: make(chan struct{}), release: make(chan struct{})}
	lifecycle := NewLifecycle(restorer)
	lifecycle.Start(context.Background())
	restoreDone := make(chan struct{})
	go func() {
		lifecycle.RequestRestore()
		close(restoreDone)
	}()
	<-restorer.entered

	stopDone := make(chan struct{})
	stopStarted := make(chan struct{})
	go func() {
		close(stopStarted)
		lifecycle.Stop()
		close(stopDone)
	}()
	<-stopStarted
	select {
	case <-stopDone:
		t.Fatal("lifecycle stopped while a native restore was still active")
	case <-time.After(25 * time.Millisecond):
	}
	close(restorer.release)
	<-restoreDone
	<-stopDone
	if lifecycle.Context() != nil {
		t.Fatal("stopped lifecycle retained a context")
	}
}
