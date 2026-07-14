package application_test

import (
	"context"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
)

func TestOperatorSteeringDrainWakesPausedRunAndExecutesOnlyQueuedTurns(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "steering-drain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runs := application.NewRunService(st)
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionContinue, "first drained", "", ""),
		rootActionResponse(domain.RootActionContinue, "second drained", "", ""),
	}}
	_, created, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "drain only queued operator guidance", Profile: "review",
		ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err = runs.Pause(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index, content := range []string{"first queued drain", "second queued drain"} {
		if _, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
			RunID: run.ID, SessionID: run.SessionID, Content: content,
			OperationKey: "operator-steering-drain-enqueue-000" + string(rune('1'+index)),
			RequestedBy:  "operator",
		}); err != nil {
			t.Fatal(err)
		}
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	drain := application.NewOperatorSteeringDrainService(st, router, policy.NewDefaultChecker())
	result, err := drain.Drain(ctx, application.DrainOperatorSteeringRequest{
		RunID: run.ID, MaxSteps: 4,
	})
	if err != nil || !result.Woke || len(result.Execution.Steps) != 2 ||
		result.Execution.StopReason != "steering_drained" || provider.calls != 2 ||
		result.Before.Pending != 2 || result.After.Pending != 0 ||
		result.After.Prepared != 0 || result.After.Committed != 2 {
		t.Fatalf("paused steering drain drifted: result=%#v calls=%d err=%v",
			result, provider.calls, err)
	}
	persisted, err := st.GetRun(ctx, run.ID)
	if err != nil || persisted.Status != domain.RunRunning {
		t.Fatalf("drained Run lifecycle drifted: run=%#v err=%v", persisted, err)
	}
	history, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil || len(history) != 4 || history[0].Content != "first queued drain" ||
		history[2].Content != "second queued drain" {
		t.Fatalf("drained Session history is not ordered exactly once: %#v err=%v",
			history, err)
	}
	if _, err := runs.Pause(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	empty, err := drain.Drain(ctx, application.DrainOperatorSteeringRequest{
		RunID: run.ID, MaxSteps: 4,
	})
	if err != nil || empty.Woke || empty.Execution.StopReason != "queue_empty" ||
		provider.calls != 2 {
		t.Fatalf("empty drain woke or called the model: result=%#v calls=%d err=%v",
			empty, provider.calls, err)
	}
	persisted, err = st.GetRun(ctx, run.ID)
	if err != nil || persisted.Status != domain.RunPaused {
		t.Fatalf("empty drain changed paused Run: run=%#v err=%v", persisted, err)
	}
}

func TestOperatorSteeringDrainDoesNotWakePausedRunUnderAnotherExecutionLease(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "steering-drain-lease.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runs := application.NewRunService(st)
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionContinue, "drained after release", "", ""),
	}}
	_, created, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "do not wake while another worker owns the Run", Profile: "review",
		ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	run, err = runs.Pause(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "wait for the active worker",
		OperationKey: "operator-steering-drain-active-lease-0001", RequestedBy: "operator",
	}); err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	drain := application.NewOperatorSteeringDrainService(st, router, policy.NewDefaultChecker())
	blocked, err := drain.Drain(ctx, application.DrainOperatorSteeringRequest{
		RunID: run.ID, MaxSteps: 1,
	})
	if apperror.CodeOf(err) != apperror.CodeConflict || blocked.Woke || provider.calls != 0 ||
		blocked.After.Pending != 1 {
		t.Fatalf("lease-conflicted drain changed execution: result=%#v calls=%d code=%s err=%v",
			blocked, provider.calls, apperror.CodeOf(err), err)
	}
	persisted, getErr := st.GetRun(ctx, run.ID)
	if getErr != nil || persisted.Status != domain.RunPaused {
		t.Fatalf("lease-conflicted drain woke paused Run: run=%#v err=%v", persisted, getErr)
	}
	if _, _, err := st.ReleaseRunExecutionLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
	drained, err := drain.Drain(ctx, application.DrainOperatorSteeringRequest{
		RunID: run.ID, MaxSteps: 1,
	})
	if err != nil || !drained.Woke || len(drained.Execution.Steps) != 1 ||
		provider.calls != 1 || drained.After.Pending != 0 {
		t.Fatalf("released drain did not resume normally: result=%#v calls=%d err=%v",
			drained, provider.calls, err)
	}
}

func TestOperatorSteeringDrainDoesNotRecoverFailedNonSteeringTurn(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "steering-drain-failed-input.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runs := application.NewRunService(st)
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionContinue, "must not execute", "", ""),
	}}
	_, created, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "preserve an unrelated failed root turn", Profile: "review",
		ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	turn, err := st.BeginSupervisorTurn(ctx, lease, "recover this through the ordinary path")
	if err != nil {
		t.Fatal(err)
	}
	failed, err := st.FailSupervisorTurn(ctx, turn.Checkpoint, "simulated ordinary failure", 0)
	if err != nil || failed.Phase != domain.SupervisorTurnFailed {
		t.Fatalf("ordinary turn did not enter failed state: checkpoint=%#v err=%v", failed, err)
	}
	if _, _, err := st.ReleaseRunExecutionLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "remain queued behind ordinary recovery",
		OperationKey: "operator-steering-drain-failed-input-0001", RequestedBy: "operator",
	}); err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	result, err := application.NewOperatorSteeringDrainService(st, router,
		policy.NewDefaultChecker()).Drain(ctx,
		application.DrainOperatorSteeringRequest{RunID: run.ID, MaxSteps: 1})
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition || provider.calls != 0 ||
		result.After.Pending != 1 || len(result.Execution.Steps) != 0 {
		t.Fatalf("steering drain recovered unrelated turn: result=%#v calls=%d code=%s err=%v",
			result, provider.calls, apperror.CodeOf(err), err)
	}
	persisted, found, getErr := st.GetSupervisorCheckpoint(ctx, run.ID)
	if getErr != nil || !found || persisted.Phase != domain.SupervisorTurnFailed ||
		persisted.PendingInput != "recover this through the ordinary path" {
		t.Fatalf("failed ordinary checkpoint changed: value=%#v found=%t err=%v",
			persisted, found, getErr)
	}
}

func TestSessionSteeringOperationKeyReplaysAcrossRestartAndAfterDelivery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-steering-replay.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	runs := application.NewRunService(st)
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionFinish, "delivered once", "done", ""),
	}}
	_, created, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "durable ordinary Session steering", Profile: "review",
		ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	manager := session.NewManager(st, router, policy.NewDefaultChecker()).WithRunChatExecutor(
		application.NewSessionRunChatExecutor(st, router, policy.NewDefaultChecker()),
	)
	const operationKey = "session-steering-stable-retry-0001"
	first, err := manager.SendWithOptions(ctx, run.SessionID, "deliver this once",
		session.SendOptions{OperationKey: operationKey})
	if err != nil || !first.Queued || first.SteeringReplayed ||
		first.SteeringStatus != string(domain.OperatorSteeringPending) || provider.calls != 0 {
		t.Fatalf("idempotent Session send did not queue: result=%#v calls=%d err=%v",
			first, provider.calls, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	manager = session.NewManager(st, router, policy.NewDefaultChecker()).WithRunChatExecutor(
		application.NewSessionRunChatExecutor(st, router, policy.NewDefaultChecker()),
	)
	replayed, err := manager.SendWithOptions(ctx, run.SessionID, "deliver this once",
		session.SendOptions{OperationKey: operationKey})
	if err != nil || !replayed.Queued || !replayed.SteeringReplayed ||
		replayed.SteeringID != first.SteeringID || provider.calls != 0 {
		t.Fatalf("Session retry did not converge after restart: result=%#v calls=%d err=%v",
			replayed, provider.calls, err)
	}
	if _, err := manager.SendWithOptions(ctx, run.SessionID, "changed intent",
		session.SendOptions{OperationKey: operationKey}); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed Session retry intent was not rejected: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	drained, err := application.NewOperatorSteeringDrainService(st, router,
		policy.NewDefaultChecker()).Drain(ctx,
		application.DrainOperatorSteeringRequest{RunID: run.ID, MaxSteps: 2})
	if err != nil || len(drained.Execution.Steps) != 1 ||
		drained.Execution.StopReason != "root_finish" || provider.calls != 1 ||
		drained.After.Committed != 1 {
		t.Fatalf("queued Session input was not delivered once: result=%#v calls=%d err=%v",
			drained, provider.calls, err)
	}
	afterDelivery, err := manager.SendWithOptions(ctx, run.SessionID, "deliver this once",
		session.SendOptions{OperationKey: operationKey})
	if err != nil || !afterDelivery.SteeringReplayed ||
		afterDelivery.SteeringStatus != string(domain.OperatorSteeringCommitted) ||
		afterDelivery.SteeringID != first.SteeringID || provider.calls != 1 {
		t.Fatalf("committed Session retry was not replayed: result=%#v calls=%d err=%v",
			afterDelivery, provider.calls, err)
	}
	history, err := st.ListSessionMessages(ctx, run.SessionID, true)
	if err != nil || len(history) != 2 || history[0].Content != "deliver this once" {
		t.Fatalf("idempotent Session delivery duplicated history: %#v err=%v", history, err)
	}
}

func TestSessionSteeringOperationKeyRejectsUnboundAndSlashInput(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "session-steering-boundary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	router := llm.NewDefaultRouter()
	manager := session.NewManager(st, router, policy.NewDefaultChecker()).WithRunChatExecutor(
		application.NewSessionRunChatExecutor(st, router, policy.NewDefaultChecker()),
	)
	sess, err := manager.Create(ctx, "", "unbound retry identity", "learn")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.SendWithOptions(ctx, sess.ID, "do not imply idempotency",
		session.SendOptions{OperationKey: "unbound-session-operation-0001"}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("unbound Session accepted retry identity: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	if _, err := manager.SendWithOptions(ctx, sess.ID, "/help",
		session.SendOptions{OperationKey: "slash-session-operation-0001"}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("slash command accepted retry identity: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	history, err := st.ListSessionMessages(ctx, sess.ID, true)
	if err != nil || len(history) != 0 {
		t.Fatalf("rejected idempotent input changed Session history: %#v err=%v", history, err)
	}
}
