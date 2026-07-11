package application_test

import (
	"context"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/store"
)

func TestRunSupervisorConsumesCrossProcessModelCancellation(t *testing.T) {
	provider := newActiveCallBlockingProvider()
	path, workerStore, run, supervisor := newRetrySupervisor(t, provider)
	supervisor.WithModelCancellationPollInterval(10 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := supervisor.Step(ctx, run.ID)
		done <- err
	}()
	select {
	case <-provider.entered:
	case <-ctx.Done():
		t.Fatal("provider did not enter before cross-process cancellation")
	}
	active, found := supervisor.ActiveCall(run.ID)
	if !found {
		t.Fatal("worker did not publish its active model call")
	}
	controlStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer controlStore.Close()
	requested, err := controlStore.RequestSupervisorModelCancellation(ctx, domain.RequestModelCancellation{
		RunID: run.ID, AttemptID: active.AttemptID, ModelAttempt: active.ModelAttempt,
		IdempotencyKey: "cross-process-cancel-0123456789", Reason: "integration operator stop",
		RequestedBy: "http_control",
	})
	if err != nil || requested.Replayed {
		t.Fatalf("cross-process cancellation request failed: %#v err=%v", requested, err)
	}
	select {
	case stepErr := <-done:
		if apperror.CodeOf(stepErr) != apperror.CodeCancelled {
			t.Fatalf("worker returned code=%s err=%v", apperror.CodeOf(stepErr), stepErr)
		}
	case <-ctx.Done():
		t.Fatal("worker did not consume the cross-process cancellation")
	}
	resolved, err := controlStore.GetModelCancellation(ctx, requested.Cancellation.ID)
	if err != nil || resolved.Status != domain.ModelCancellationResolved || resolved.Resolution != "cancelled" {
		t.Fatalf("cross-process cancellation did not resolve: %#v err=%v", resolved, err)
	}
	items, err := workerStore.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	requestedEvents, observedEvents, failedEvents := 0, 0, 0
	for _, event := range items {
		switch event.Type {
		case events.ModelCancelRequestedEvent:
			requestedEvents++
		case events.ModelCancelObservedEvent:
			observedEvents++
		case events.ModelFailedEvent:
			failedEvents++
		}
	}
	if requestedEvents != 1 || observedEvents != 1 || failedEvents != 1 {
		t.Fatalf("cross-process audit is incomplete: requested=%d observed=%d failed=%d",
			requestedEvents, observedEvents, failedEvents)
	}
}
