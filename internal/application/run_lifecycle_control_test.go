package application_test

import (
	"context"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/store"
)

func TestRunLifecycleControlReplaysAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-lifecycle-control.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	_, created, err := application.NewRunService(st).Create(ctx,
		application.CreateRunRequest{Goal: "controlled lifecycle", Profile: "code",
			Budget: domain.Budget{MaxTurns: 8}})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewRunLifecycleControlService(st)
	startRequest := application.ControlRunLifecycleRequest{
		Version: domain.RunLifecycleControlProtocolVersion, RunID: created.ID,
		Action: domain.RunLifecycleStart, OperationKey: "run-lifecycle-start-0001",
		RequestedBy: "http_run_operator",
	}
	started, err := service.Apply(ctx, startRequest)
	if err != nil {
		t.Fatal(err)
	}
	if started.Replayed || started.Run.Status != domain.RunRunning ||
		started.Operation.EventSequenceEnd != started.Operation.EventSequenceStart+1 {
		t.Fatalf("unexpected start result: %#v", started)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service = application.NewRunLifecycleControlService(st)
	replayed, err := service.Apply(ctx, startRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Operation != started.Operation ||
		replayed.Run.Status != domain.RunRunning {
		t.Fatalf("restart replay diverged: first=%#v replay=%#v", started, replayed)
	}
	pauseRequest := application.ControlRunLifecycleRequest{
		Version: domain.RunLifecycleControlProtocolVersion, RunID: created.ID,
		Action: domain.RunLifecyclePause, OperationKey: "run-lifecycle-pause-0001",
		RequestedBy: "http_run_operator",
	}
	paused, err := service.Apply(ctx, pauseRequest)
	if err != nil || paused.Run.Status != domain.RunPaused {
		t.Fatalf("pause failed: result=%#v err=%v", paused, err)
	}
	delayedStartReplay, err := service.Apply(ctx, startRequest)
	if err != nil || !delayedStartReplay.Replayed ||
		delayedStartReplay.Operation != started.Operation ||
		delayedStartReplay.Run.Status != domain.RunPaused {
		t.Fatalf("delayed start replay lost operation or current state: result=%#v err=%v",
			delayedStartReplay, err)
	}
	resumeRequest := application.ControlRunLifecycleRequest{
		Version: domain.RunLifecycleControlProtocolVersion, RunID: created.ID,
		Action: domain.RunLifecycleResume, OperationKey: "run-lifecycle-resume-0001",
		RequestedBy: "http_run_operator",
	}
	resumed, err := service.Apply(ctx, resumeRequest)
	if err != nil || resumed.Run.Status != domain.RunRunning {
		t.Fatalf("resume failed: result=%#v err=%v", resumed, err)
	}
	startRequest.Action = domain.RunLifecyclePause
	if _, err := service.Apply(ctx, startRequest); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed lifecycle intent code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestRunLifecyclePauseRejectsLeaseAndNonQuiescentSupervisor(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "run-lifecycle-guard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, created, err := application.NewRunService(st).Create(ctx,
		application.CreateRunRequest{Goal: "guarded pause", Profile: "code",
			Budget: domain.Budget{MaxTurns: 8}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = acquireTestRunExecutionLease(t, ctx, st, run.ID)
	service := application.NewRunLifecycleControlService(st)
	request := application.ControlRunLifecycleRequest{
		Version: domain.RunLifecycleControlProtocolVersion, RunID: run.ID,
		Action: domain.RunLifecyclePause, OperationKey: "guarded-pause-active-lease-0001",
		RequestedBy: "http_run_operator",
	}
	if _, err := service.Apply(ctx, request); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("active lease pause code=%s err=%v", apperror.CodeOf(err), err)
	}
	releaseTestRunExecutionLease(t, ctx, st, run.ID)
	queued, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "prepared input",
		OperationKey: "guarded-pause-steering-0001", RequestedBy: "test_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	if _, err := st.BeginSupervisorSteeringTurnForMessage(ctx, lease,
		queued.Message.ID); err != nil {
		t.Fatal(err)
	}
	preparedMessage, err := st.GetOperatorSteering(ctx, queued.Message.ID)
	if err != nil || !preparedMessage.Prepared {
		t.Fatalf("prepared steering state was not projected: message=%#v err=%v",
			preparedMessage, err)
	}
	releaseTestRunExecutionLease(t, ctx, st, run.ID)
	request.OperationKey = "guarded-pause-prepared-turn-0001"
	if _, err := service.Apply(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("non-quiescent pause code=%s err=%v", apperror.CodeOf(err), err)
	}
}
