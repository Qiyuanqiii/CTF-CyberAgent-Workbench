package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
)

func TestActiveCallRegistryLifecycleAndIdempotentCancellation(t *testing.T) {
	registry := NewActiveCallRegistry()
	checkpoint, attempt := activeCallTestIdentity()
	lease, err := registry.reserve(context.Background(), checkpoint, attempt, "session-live")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Lookup(checkpoint.RunID); ok {
		t.Fatal("reserved call became visible before its durable start")
	}
	if err := lease.Activate(); err != nil {
		t.Fatal(err)
	}
	info, ok := registry.Lookup(checkpoint.RunID)
	if !ok || info.SessionID != "session-live" || info.ModelAttempt != 1 || info.StreamBytes != 0 {
		t.Fatalf("unexpected active call info: %#v ok=%t", info, ok)
	}
	if bySession, ok := registry.LookupSession("session-live"); !ok || bySession.RunID != checkpoint.RunID {
		t.Fatalf("unexpected session active call lookup: %#v ok=%t", bySession, ok)
	}
	if listed := registry.List(); len(listed) != 1 || listed[0].RunID != checkpoint.RunID {
		t.Fatalf("unexpected active call list: %#v", listed)
	}

	subscription, err := registry.Subscribe(checkpoint.RunID)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	snapshot := receiveActiveCallEvent(t, subscription.Events())
	if snapshot.Type != ActiveCallSnapshotEvent || snapshot.Sequence != 1 {
		t.Fatalf("unexpected subscription snapshot: %#v", snapshot)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("invalid subscription snapshot: %v", err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"text"`) {
		t.Fatalf("live envelope unexpectedly exposed model text: %s", encoded)
	}

	if err := lease.PublishProgress(7, 7); err != nil {
		t.Fatal(err)
	}
	progress := receiveActiveCallEvent(t, subscription.Events())
	if progress.Type != ActiveCallProgressEvent || progress.Sequence != 2 || progress.DeltaBytes != 7 || progress.Call.StreamChunks != 1 {
		t.Fatalf("unexpected progress event: %#v", progress)
	}
	if err := progress.Validate(); err != nil {
		t.Fatalf("invalid progress event: %v", err)
	}

	target, ok := registry.cancellationTarget(checkpoint.RunID)
	if !ok {
		t.Fatal("active call cancellation target was not found")
	}
	updated, signalled, already := registry.signalCancel(target.key)
	if !signalled || already || !updated.CancelRequested {
		t.Fatalf("unexpected first cancellation result: info=%#v signalled=%t already=%t", updated, signalled, already)
	}
	select {
	case <-lease.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("active call context was not cancelled")
	}
	cancelEvent := receiveActiveCallEvent(t, subscription.Events())
	if cancelEvent.Type != ActiveCallCancelRequestedEvent || cancelEvent.Sequence != 3 {
		t.Fatalf("unexpected cancellation event: %#v", cancelEvent)
	}
	_, signalled, already = registry.signalCancel(target.key)
	if !signalled || !already {
		t.Fatalf("repeated cancellation was not idempotent: signalled=%t already=%t", signalled, already)
	}

	lease.Finish(llm.OutcomeCancelled)
	terminal := receiveActiveCallEvent(t, subscription.Events())
	if terminal.Type != ActiveCallFailedEvent || terminal.Sequence != 4 || terminal.Outcome != llm.OutcomeCancelled {
		t.Fatalf("unexpected active call terminal event: %#v", terminal)
	}
	if err := terminal.Validate(); err != nil {
		t.Fatalf("invalid terminal event: %v", err)
	}
	if _, open := <-subscription.Events(); open {
		t.Fatal("active call subscription stayed open after terminal event")
	}
	if _, ok := registry.Lookup(checkpoint.RunID); ok || len(registry.List()) != 0 {
		t.Fatal("active call was not removed after terminal event")
	}
}

func TestActiveCallRegistryRejectsDuplicateRunAndDropsSlowSubscriber(t *testing.T) {
	registry := newActiveCallRegistry(1)
	checkpoint, attempt := activeCallTestIdentity()
	lease, err := registry.reserve(context.Background(), checkpoint, attempt, "session-live")
	if err != nil {
		t.Fatal(err)
	}
	duplicate := attempt
	duplicate.Number = 2
	duplicate.TransportAttempt = 2
	if _, err := registry.reserve(context.Background(), checkpoint, duplicate, "session-live"); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("duplicate active run should conflict, got %v", err)
	}
	if err := lease.Activate(); err != nil {
		t.Fatal(err)
	}
	subscription, err := registry.Subscribe(checkpoint.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.PublishProgress(1, 1); err != nil {
		t.Fatal(err)
	}
	if !subscription.Dropped() {
		t.Fatal("full subscriber buffer did not trigger the slow-consumer policy")
	}
	if snapshot, open := <-subscription.Events(); !open || snapshot.Type != ActiveCallSnapshotEvent {
		t.Fatalf("unexpected buffered snapshot after subscriber drop: %#v open=%t", snapshot, open)
	}
	if _, open := <-subscription.Events(); open {
		t.Fatal("dropped subscriber channel remained open")
	}
	lease.Finish(llm.OutcomeSuccess)

	newCheckpoint := checkpoint
	newCheckpoint.AttemptID = "attempt-live-2"
	newAttempt := attempt
	newAttempt.Number = 2
	newAttempt.TransportAttempt = 2
	replacement, err := registry.reserve(context.Background(), newCheckpoint, newAttempt, "session-live")
	if err != nil {
		t.Fatalf("completed call prevented a later call for the same run: %v", err)
	}
	replacement.Abort()
}

func activeCallTestIdentity() (domain.SupervisorCheckpoint, llm.ModelAttempt) {
	return domain.SupervisorCheckpoint{
			RunID: "run-live", NextTurn: 1, Phase: domain.SupervisorTurnStarted,
			AttemptID: "attempt-live", PendingInput: "test", UpdatedAt: time.Now().UTC(),
		}, llm.ModelAttempt{
			Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "provider", Model: "model",
		}
}

func receiveActiveCallEvent(t *testing.T, events <-chan ActiveCallEvent) ActiveCallEvent {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("active call subscription closed before the expected event")
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for active call event")
		return ActiveCallEvent{}
	}
}
