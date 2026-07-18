package application_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
)

func TestSessionMessageSubmissionIsDurableRedactedAndRestartSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	runs := application.NewRunService(st)
	_, created, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "durable Session submission", Profile: "review",
		Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	secret := "sk-" + strings.Repeat("s", 32)
	request := application.SubmitSessionMessageRequest{
		Version:   domain.SessionMessageSubmissionProtocolVersion,
		SessionID: run.SessionID, Content: "review this token=" + secret,
		OperationKey: "session-message-restart-safe-0001",
		RequestedBy:  "http_session_operator",
	}
	first, err := application.NewSessionMessageSubmissionService(st).Submit(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Run.ID != run.ID || first.Session.ID != run.SessionID ||
		first.Message.Status != domain.OperatorSteeringPending ||
		first.Message.Sequence != 1 || strings.Contains(first.Message.Content, secret) ||
		!strings.Contains(first.Message.Content, "[REDACTED:") {
		t.Fatalf("unexpected first submission: %#v", first)
	}
	history, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil || len(history) != 0 {
		t.Fatalf("submission wrote Session history early: history=%#v err=%v", history, err)
	}
	before, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countEventType(before, events.OperatorSteeringQueuedEvent) != 1 ||
		countEventType(before, events.SessionMessageEvent) != 0 {
		t.Fatalf("submission events are incorrect: events=%#v err=%v", before, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	replayed, err := application.NewSessionMessageSubmissionService(st).Submit(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Message.ID != first.Message.ID ||
		replayed.Message.Sequence != first.Message.Sequence {
		t.Fatalf("restart replay diverged: first=%#v replay=%#v", first, replayed)
	}
	request.Content = "different intent"
	if _, err := application.NewSessionMessageSubmissionService(st).Submit(ctx, request); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed intent error=%v code=%s", err, apperror.CodeOf(err))
	}
	queue, err := st.GetOperatorSteeringQueueSummary(ctx, run.ID)
	if err != nil || queue.Pending != 1 || queue.Prepared != 0 || queue.Committed != 0 {
		t.Fatalf("replay changed queue: queue=%#v err=%v", queue, err)
	}
}

func TestSessionMessageSubmissionRejectsUnboundAndNonRunningSessions(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewSessionMessageSubmissionService(st)
	legacy := session.New("", "legacy", "learn")
	if err := st.SaveSession(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	base := application.SubmitSessionMessageRequest{
		Version:   domain.SessionMessageSubmissionProtocolVersion,
		SessionID: legacy.ID, Content: "bounded input",
		OperationKey: "session-message-precondition-0001",
		RequestedBy:  "http_session_operator",
	}
	if _, err := service.Submit(ctx, base); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("unbound Session error=%v code=%s", err, apperror.CodeOf(err))
	}
	_, created, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "created Run cannot queue", Profile: "code", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	base.SessionID = created.SessionID
	base.OperationKey = "session-message-precondition-0002"
	if _, err := service.Submit(ctx, base); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("created Run error=%v code=%s", err, apperror.CodeOf(err))
	}
}

func TestSessionMessageSubmissionValidatesProtocolBeforePersistence(t *testing.T) {
	valid := application.SubmitSessionMessageRequest{
		Version:   domain.SessionMessageSubmissionProtocolVersion,
		SessionID: "sess-valid", Content: "valid content",
		OperationKey: "session-message-validation-0001",
		RequestedBy:  "http_session_operator",
	}
	if _, err := application.NewSessionMessageSubmissionService(nil).
		Submit(context.Background(), valid); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("nil dependency error=%v code=%s", err, apperror.CodeOf(err))
	}
	tests := []struct {
		name   string
		change func(*application.SubmitSessionMessageRequest)
	}{
		{name: "version", change: func(r *application.SubmitSessionMessageRequest) { r.Version = "other.v1" }},
		{name: "session", change: func(r *application.SubmitSessionMessageRequest) { r.SessionID = " session " }},
		{name: "content", change: func(r *application.SubmitSessionMessageRequest) { r.Content = "\x00" }},
		{name: "operation", change: func(r *application.SubmitSessionMessageRequest) { r.OperationKey = "short" }},
		{name: "requester", change: func(r *application.SubmitSessionMessageRequest) { r.RequestedBy = " requester " }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.change(&request)
			st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			if _, err := application.NewSessionMessageSubmissionService(st).
				Submit(context.Background(), request); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
				t.Fatalf("validation error=%v code=%s", err, apperror.CodeOf(err))
			}
		})
	}
}
