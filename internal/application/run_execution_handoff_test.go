package application_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/store"
)

func TestRunExecutionHandoffExecutesOnlyFrozenSelectionAndReplays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-execution-handoff.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionContinue, "selected second", "", ""),
	}}
	_, created, err := application.NewRunService(st).Create(ctx,
		application.CreateRunRequest{Goal: "bounded execution handoff", Profile: "code",
			ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 8}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	messages := make([]domain.OperatorSteeringMessage, 0, 3)
	for index, content := range []string{"cancel selected first", "execute selected second",
		"leave later third"} {
		queued, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
			RunID: run.ID, SessionID: run.SessionID, Content: content,
			OperationKey: fmt.Sprintf("handoff-steering-%04d", index+1),
			RequestedBy:  "test_operator",
		})
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, queued.Message)
	}
	request := application.ExecuteRunHandoffRequest{
		Version: domain.RunExecutionHandoffProtocolVersion, RunID: run.ID,
		MaxSteps: 2, OperationKey: "run-execution-handoff-0001",
		RequestedBy: "http_run_operator",
	}
	keyDigest := runmutation.RunExecutionHandoffOperationDigest(request.RunID,
		request.OperationKey)
	fingerprint := runmutation.RunExecutionHandoffRequestFingerprint(request.RunID,
		request.RequestedBy, request.MaxSteps)
	prepared, replayed, err := st.PrepareRunExecutionHandoff(ctx,
		domain.RunExecutionHandoffOperation{
			ID:              idgen.New("run-handoff"),
			ProtocolVersion: domain.RunExecutionHandoffProtocolVersion,
			KeyDigest:       keyDigest, RequestFingerprint: fingerprint,
			RunID: run.ID, SessionID: run.SessionID, RequestedBy: request.RequestedBy,
			MaxSteps: request.MaxSteps, CreatedAt: time.Now().UTC(),
		})
	if err != nil || replayed || len(prepared.Items) != 2 ||
		prepared.Items[0].MessageID != messages[0].ID ||
		prepared.Items[1].MessageID != messages[1].ID {
		t.Fatalf("selection was not frozen in order: handoff=%#v replayed=%t err=%v",
			prepared, replayed, err)
	}
	if _, err := st.CancelOperatorSteering(ctx, domain.CancelOperatorSteeringRequest{
		MessageID: messages[0].ID, OperationKey: "handoff-cancel-selected-0001",
		RequestedBy: "test_operator", Reason: "cancel before lease acquisition",
	}); err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	service := application.NewRunExecutionHandoffService(st, router,
		policy.NewDefaultChecker())
	result, err := service.Execute(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Replayed || result.Handoff.Result == nil ||
		result.Handoff.Result.Status != domain.RunExecutionHandoffCompleted ||
		result.Handoff.Result.StepsCompleted != 1 || provider.calls != 1 {
		t.Fatalf("handoff result drifted: result=%#v calls=%d", result, provider.calls)
	}
	first, _ := st.GetOperatorSteering(ctx, messages[0].ID)
	second, _ := st.GetOperatorSteering(ctx, messages[1].ID)
	third, _ := st.GetOperatorSteering(ctx, messages[2].ID)
	if first.Status != domain.OperatorSteeringCancelled ||
		second.Status != domain.OperatorSteeringCommitted ||
		third.Status != domain.OperatorSteeringPending {
		t.Fatalf("exact selection boundary failed: first=%s second=%s third=%s",
			first.Status, second.Status, third.Status)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	service = application.NewRunExecutionHandoffService(st, router,
		policy.NewDefaultChecker())
	again, err := service.Execute(ctx, request)
	if err != nil || !again.Replayed || provider.calls != 1 ||
		again.Handoff.Result == nil ||
		again.Handoff.Result.CompletionEventSequence !=
			result.Handoff.Result.CompletionEventSequence {
		t.Fatalf("restart replay called model or drifted: result=%#v calls=%d err=%v",
			again, provider.calls, err)
	}
	request.MaxSteps = 1
	if _, err := service.Execute(ctx, request); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed handoff intent code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestRunExecutionHandoffCompletesEmptyQueueWithoutLeaseOrModel(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "empty-handoff.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	provider := &lifecycleProvider{}
	_, created, err := application.NewRunService(st).Create(ctx,
		application.CreateRunRequest{Goal: "empty handoff", Profile: "code",
			ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewRunExecutionHandoffService(st, router,
		policy.NewDefaultChecker()).Execute(ctx, application.ExecuteRunHandoffRequest{
		Version: domain.RunExecutionHandoffProtocolVersion, RunID: run.ID,
		MaxSteps: 4, OperationKey: "empty-run-handoff-0001",
		RequestedBy: "http_run_operator",
	})
	if err != nil || result.Handoff.Result == nil ||
		result.Handoff.Result.StopReason != "queue_empty" ||
		result.Handoff.Result.LeaseID != "" || provider.calls != 0 {
		t.Fatalf("empty handoff acquired work: result=%#v calls=%d err=%v",
			result, provider.calls, err)
	}
}
