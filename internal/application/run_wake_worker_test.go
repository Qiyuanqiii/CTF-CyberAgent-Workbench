package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

type wakeWorkerDueFake struct {
	mu     sync.Mutex
	values []domain.RunWakeIntent
	limits []int
}

func (f *wakeWorkerDueFake) Due(_ context.Context, limit int) ([]domain.RunWakeIntent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limits = append(f.limits, limit)
	if len(f.values) == 0 {
		return nil, nil
	}
	value := f.values[0]
	f.values = f.values[1:]
	return []domain.RunWakeIntent{value}, nil
}

type wakeWorkerConsumerFake struct {
	mu       sync.Mutex
	requests []ConsumeRunWakeRequest
	active   int
	maximum  int
	block    chan struct{}
}

func (f *wakeWorkerConsumerFake) Consume(ctx context.Context,
	request ConsumeRunWakeRequest,
) (ConsumeRunWakeResult, error) {
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.active++
	if f.active > f.maximum {
		f.maximum = f.active
	}
	f.mu.Unlock()
	if f.block != nil {
		select {
		case <-ctx.Done():
		case <-f.block:
		}
	}
	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	return ConsumeRunWakeResult{}, nil
}

func TestRunWakeWorkerHardBoundsDueAndSupervisorHandoff(t *testing.T) {
	due := &wakeWorkerDueFake{values: []domain.RunWakeIntent{{RunID: "run-worker"}}}
	consumer := &wakeWorkerConsumerFake{}
	worker, err := NewRunWakeWorker(due, consumer, RunWakeWorkerConfig{
		PollInterval: MinRunWakeWorkerPollInterval, OwnerID: "wake-worker-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	worked, err := worker.RunOnce(t.Context())
	if err != nil || !worked {
		t.Fatalf("worker did not consume due intent: worked=%t err=%v", worked, err)
	}
	if len(due.limits) != 1 || due.limits[0] != RunWakeWorkerConcurrency ||
		len(consumer.requests) != 1 ||
		consumer.requests[0].Version != domain.RunWakeConsumerProtocolVersion ||
		consumer.requests[0].MaxSteps != RunWakeWorkerMaxSteps ||
		consumer.maximum != 1 {
		t.Fatalf("worker widened a hard bound: due=%v requests=%#v max=%d",
			due.limits, consumer.requests, consumer.maximum)
	}
}

func TestRunWakeWorkerCancellationStopsSerialLoop(t *testing.T) {
	due := &wakeWorkerDueFake{values: []domain.RunWakeIntent{{RunID: "run-worker-cancel"}}}
	consumer := &wakeWorkerConsumerFake{block: make(chan struct{})}
	worker, err := NewRunWakeWorker(due, consumer, RunWakeWorkerConfig{
		PollInterval: MinRunWakeWorkerPollInterval, OwnerID: "wake-worker-cancel",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	deadline := time.After(2 * time.Second)
	for {
		consumer.mu.Lock()
		started := len(consumer.requests) == 1
		consumer.mu.Unlock()
		if started {
			break
		}
		select {
		case <-deadline:
			t.Fatal("worker did not start")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
}
