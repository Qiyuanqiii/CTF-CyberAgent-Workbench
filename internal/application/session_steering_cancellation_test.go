package application_test

import (
	"context"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/store"
)

func TestSessionSteeringCancellationIsBoundedAndRestartSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, created, err := application.NewRunService(st).Create(ctx,
		application.CreateRunRequest{Goal: "cancel queued steering", Profile: "code",
			Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "queued input",
		OperationKey: "session-steering-queue-0001", RequestedBy: "test_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := application.CancelSessionSteeringRequest{
		Version:   domain.SessionSteeringCancellationProtocolVersion,
		SessionID: run.SessionID, MessageID: queued.Message.ID,
		OperationKey: "session-steering-cancel-0001", RequestedBy: "http_session_operator",
		Reason: "operator withdrew queued input",
	}
	first, err := application.NewSessionSteeringCancellationService(st).Cancel(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Run.ID != run.ID || first.Session.ID != run.SessionID ||
		first.Message.Status != domain.OperatorSteeringCancelled ||
		first.Cancellation.Kind != domain.OperatorSteeringCancellationOperator {
		t.Fatalf("unexpected cancellation: %#v", first)
	}
	history, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil || len(history) != 0 {
		t.Fatalf("cancellation changed Session history: history=%#v err=%v", history, err)
	}
	eventValues, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countEventType(eventValues, events.OperatorSteeringCancelledEvent) != 1 {
		t.Fatalf("cancellation audit event mismatch: events=%#v err=%v", eventValues, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	replayed, err := application.NewSessionSteeringCancellationService(st).Cancel(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Cancellation.ID != first.Cancellation.ID ||
		replayed.Message.Status != domain.OperatorSteeringCancelled {
		t.Fatalf("restart replay diverged: first=%#v replay=%#v", first, replayed)
	}
	request.Reason = "different cancellation intent"
	if _, err := application.NewSessionSteeringCancellationService(st).Cancel(ctx, request); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed intent error=%v code=%s", err, apperror.CodeOf(err))
	}
}

func TestSessionSteeringCancellationRejectsCrossSessionAndNonPendingMessages(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runs := application.NewRunService(st)
	_, firstCreated, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "first Session", Profile: "code", Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := runs.Start(ctx, firstCreated.ID)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: first.ID, SessionID: first.SessionID, Content: "first input",
		OperationKey: "session-steering-cross-queue-0001", RequestedBy: "test_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, secondCreated, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "second Session", Profile: "code", Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := runs.Start(ctx, secondCreated.ID)
	if err != nil {
		t.Fatal(err)
	}
	request := application.CancelSessionSteeringRequest{
		Version:   domain.SessionSteeringCancellationProtocolVersion,
		SessionID: second.SessionID, MessageID: queued.Message.ID,
		OperationKey: "session-steering-cross-cancel-0001",
		RequestedBy:  "http_session_operator", Reason: "cross Session attempt",
	}
	if _, err := application.NewSessionSteeringCancellationService(st).Cancel(ctx, request); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("cross Session error=%v code=%s", err, apperror.CodeOf(err))
	}
	request.SessionID = first.SessionID
	request.OperationKey = "session-steering-cancel-first-0001"
	if _, err := application.NewSessionSteeringCancellationService(st).Cancel(ctx, request); err != nil {
		t.Fatal(err)
	}
	request.OperationKey = "session-steering-cancel-again-0001"
	if _, err := application.NewSessionSteeringCancellationService(st).Cancel(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("second cancellation error=%v code=%s", err, apperror.CodeOf(err))
	}
}
