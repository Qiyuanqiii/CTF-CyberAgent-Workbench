package application_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
)

func TestRunSupervisorActiveCallSubscriptionAndAuditedCancellation(t *testing.T) {
	provider := newActiveCallBlockingProvider()
	path, st, run, supervisor := newRetrySupervisor(t, provider)
	_ = path
	registry := application.NewActiveCallRegistry()
	supervisor.WithActiveCalls(registry)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type stepResult struct {
		result application.LifecycleResult
		err    error
	}
	stepDone := make(chan stepResult, 1)
	go func() {
		result, err := supervisor.Step(ctx, run.ID)
		stepDone <- stepResult{result: result, err: err}
	}()

	select {
	case <-provider.entered:
	case <-ctx.Done():
		t.Fatal("provider did not enter its streaming call")
	}
	info, ok := supervisor.ActiveCall(run.ID)
	if !ok || info.ModelAttempt != 1 || info.Provider != provider.Name() {
		t.Fatalf("unexpected active call query: %#v ok=%t", info, ok)
	}
	if calls := supervisor.ActiveCalls(); len(calls) != 1 || calls[0].RunID != run.ID {
		t.Fatalf("unexpected active call list: %#v", calls)
	}
	subscription, err := supervisor.SubscribeActiveCall(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Close()
	if event := receivePublicActiveCallEvent(t, subscription.Events()); event.Type != application.ActiveCallSnapshotEvent {
		t.Fatalf("unexpected initial live event: %#v", event)
	}

	close(provider.releaseChunk)
	progress := receivePublicActiveCallEvent(t, subscription.Events())
	if progress.Type != application.ActiveCallProgressEvent || progress.DeltaBytes == 0 || progress.Call.StreamBytes == 0 {
		t.Fatalf("unexpected live progress event: %#v", progress)
	}
	secret := "s" + "k-" + strings.Repeat("a", 30)
	cancelResult, err := supervisor.CancelActiveCall(context.Background(), application.ActiveCallCancelRequest{
		RunID: run.ID, Reason: "operator stopped test with token=" + secret,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cancelResult.Found || !cancelResult.AuditRecorded || !cancelResult.Signaled || !cancelResult.Call.CancelRequested {
		t.Fatalf("unexpected active cancellation result: %#v", cancelResult)
	}
	secondCancel, err := supervisor.CancelActiveCall(context.Background(), application.ActiveCallCancelRequest{RunID: run.ID})
	if err != nil {
		t.Fatal(err)
	}
	if secondCancel.Found && (!secondCancel.AuditRecorded || !secondCancel.AlreadyRequested) {
		t.Fatalf("repeated cancellation was not idempotent: %#v", secondCancel)
	}

	seenCancel := false
	seenTerminal := false
	for event := range subscription.Events() {
		switch event.Type {
		case application.ActiveCallCancelRequestedEvent:
			seenCancel = true
		case application.ActiveCallFailedEvent:
			seenTerminal = event.Outcome == llm.OutcomeCancelled
		}
	}
	if !seenCancel || !seenTerminal || subscription.Dropped() {
		t.Fatalf("live cancellation sequence incomplete: cancel=%t terminal=%t dropped=%t", seenCancel, seenTerminal, subscription.Dropped())
	}

	var completed stepResult
	select {
	case completed = <-stepDone:
	case <-ctx.Done():
		t.Fatal("supervisor did not stop after active cancellation")
	}
	if apperror.CodeOf(completed.err) != apperror.CodeCancelled {
		t.Fatalf("active cancellation returned %v", completed.err)
	}
	if completed.result.Checkpoint.Phase != domain.SupervisorTurnFailed || completed.result.Checkpoint.PendingInput == "" {
		t.Fatalf("cancelled turn was not left recoverable: %#v", completed.result.Checkpoint)
	}
	if _, ok := supervisor.ActiveCall(run.ID); ok || len(supervisor.ActiveCalls()) != 0 {
		t.Fatal("active call registry retained a terminal call")
	}
	if _, err := supervisor.SubscribeActiveCall(run.ID); apperror.CodeOf(err) != apperror.CodeNotFound {
		t.Fatalf("terminal call remained subscribable: %v", err)
	}

	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	cancelEvents := 0
	failedEvents := 0
	deltaEvents := 0
	for _, event := range items {
		switch event.Type {
		case events.ModelCancelRequestedEvent:
			cancelEvents++
			if strings.Contains(event.PayloadJSON, secret) || !strings.Contains(event.PayloadJSON, "[REDACTED:secret]") {
				t.Fatalf("cancellation reason was not safely redacted: %s", event.PayloadJSON)
			}
		case events.ModelDeltaEvent:
			deltaEvents++
			if strings.Contains(event.PayloadJSON, "partial-live-output") || strings.Contains(event.PayloadJSON, `"text"`) {
				t.Fatalf("model delta persisted live text: %s", event.PayloadJSON)
			}
		case events.ModelFailedEvent:
			failedEvents++
			var payload struct {
				Outcome llm.Outcome `json:"outcome"`
			}
			if err := json.Unmarshal([]byte(event.PayloadJSON), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Outcome != llm.OutcomeCancelled {
				t.Fatalf("unexpected durable cancellation outcome: %s", event.PayloadJSON)
			}
		}
	}
	if cancelEvents != 1 || deltaEvents != 1 || failedEvents != 1 {
		t.Fatalf("unexpected cancellation audit counts: cancel=%d delta=%d failed=%d", cancelEvents, deltaEvents, failedEvents)
	}
}

func TestStoreModelCancellationRequestIsIdempotent(t *testing.T) {
	provider := newActiveCallBlockingProvider()
	_, st, run, _ := newRetrySupervisor(t, provider)
	ctx := context.Background()
	turn, err := st.BeginSupervisorTurn(ctx, run.ID, "cancel ledger")
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: provider.Name(), Model: "model",
	}
	inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt)
	if err != nil || !inserted {
		t.Fatalf("model start inserted=%t err=%v", inserted, err)
	}
	secret := "s" + "k-" + strings.Repeat("b", 30)
	inserted, err = st.RecordSupervisorModelCancelRequested(ctx, turn.Checkpoint, attempt, "token="+secret)
	if err != nil || !inserted {
		t.Fatalf("cancel request inserted=%t err=%v", inserted, err)
	}
	inserted, err = st.RecordSupervisorModelCancelRequested(ctx, turn.Checkpoint, attempt, "different replay reason")
	if err != nil || inserted {
		t.Fatalf("cancel replay inserted=%t err=%v", inserted, err)
	}
	attempt.Outcome = llm.OutcomeCancelled
	attempt.ErrorText = "provider call cancelled"
	if _, err := st.RecordSupervisorModelFailed(ctx, turn.Checkpoint, attempt); err != nil {
		t.Fatal(err)
	}

	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range items {
		if event.Type != events.ModelCancelRequestedEvent {
			continue
		}
		count++
		if strings.Contains(event.PayloadJSON, secret) || !strings.Contains(event.PayloadJSON, "[REDACTED:secret]") {
			t.Fatalf("durable cancellation request leaked its reason: %s", event.PayloadJSON)
		}
	}
	if count != 1 {
		t.Fatalf("expected one durable cancellation request, got %d", count)
	}
}

type activeCallBlockingProvider struct {
	entered      chan struct{}
	releaseChunk chan struct{}
	once         sync.Once
}

func newActiveCallBlockingProvider() *activeCallBlockingProvider {
	return &activeCallBlockingProvider{entered: make(chan struct{}), releaseChunk: make(chan struct{})}
}

func (*activeCallBlockingProvider) Name() string { return "active-call-test" }

func (p *activeCallBlockingProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: p.Name()}}, nil
}

func (*activeCallBlockingProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, apperror.New(apperror.CodeInternal, "active call test requires streaming")
}

func (p *activeCallBlockingProvider) StreamChat(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	chunks := make(chan llm.ChatChunk, 1)
	p.once.Do(func() { close(p.entered) })
	go func() {
		defer close(chunks)
		select {
		case <-p.releaseChunk:
		case <-ctx.Done():
			return
		}
		select {
		case chunks <- llm.ChatChunk{Text: "partial-live-output"}:
		case <-ctx.Done():
			return
		}
		<-ctx.Done()
	}()
	return chunks, nil
}

func (*activeCallBlockingProvider) SupportsTools(string) bool    { return false }
func (*activeCallBlockingProvider) SupportsVision(string) bool   { return false }
func (*activeCallBlockingProvider) SupportsJSONMode(string) bool { return true }

func receivePublicActiveCallEvent(t *testing.T, channel <-chan application.ActiveCallEvent) application.ActiveCallEvent {
	t.Helper()
	select {
	case event, ok := <-channel:
		if !ok {
			t.Fatal("live subscription closed before the expected event")
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a live active-call event")
		return application.ActiveCallEvent{}
	}
}
