package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
)

type completedProgressTurn struct {
	checkpoint domain.SupervisorCheckpoint
	response   llm.ChatResponse
	action     domain.RootAction
	run        domain.Run
	completed  domain.SupervisorCheckpoint
}

func TestRunProgressGuardPausesRepeatedActionAndRecoversAfterExplicitResume(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, st, "detect repeated root action")
	runs := application.NewRunService(st)
	run, err := runs.Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)

	var last completedProgressTurn
	for turn := 1; turn <= domain.RunProgressRepeatThreshold; turn++ {
		last = completeProgressTurn(t, ctx, st, lease, "same response")
		if turn < domain.RunProgressRepeatThreshold &&
			(last.run.Status != domain.RunRunning || last.completed.Phase != domain.SupervisorIdle) {
			t.Fatalf("turn %d paused too early: run=%s phase=%s", turn, last.run.Status, last.completed.Phase)
		}
	}
	if last.run.Status != domain.RunPaused || last.completed.Phase != domain.SupervisorWaiting {
		t.Fatalf("repeated action did not pause Run: run=%#v checkpoint=%#v", last.run, last.completed)
	}
	guard, found, err := st.GetRunProgressGuard(ctx, run.ID)
	if err != nil || !found || guard.Status != domain.RunProgressDetected ||
		guard.Reason != domain.RunProgressRepeatedAction ||
		guard.RepeatedActionCount != domain.RunProgressRepeatThreshold {
		t.Fatalf("unexpected detected guard: %#v found=%t err=%v", guard, found, err)
	}
	if !strings.HasPrefix(guard.WaitReason(), "livelock_detected:repeated_action") {
		t.Fatalf("unexpected wait reason: %q", guard.WaitReason())
	}

	replayedRun, replayedCheckpoint, replayedMessages, err := st.CompleteSupervisorTurn(ctx,
		last.checkpoint, last.response, last.action, policy.Decision{Allowed: true}, 0)
	if err != nil || replayedRun.Status != domain.RunPaused ||
		replayedCheckpoint.Phase != domain.SupervisorWaiting || replayedMessages.User.ID != 0 {
		t.Fatalf("livelock completion replay drifted: run=%#v checkpoint=%#v messages=%#v err=%v",
			replayedRun, replayedCheckpoint, replayedMessages, err)
	}

	if _, _, err := st.ReleaseRunExecutionLease(ctx, lease); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.Resume(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	lease = acquireTestRunExecutionLease(t, ctx, st, run.ID)
	recovered := completeProgressTurn(t, ctx, st, lease, "same response")
	if recovered.run.Status != domain.RunRunning || recovered.completed.Phase != domain.SupervisorIdle {
		t.Fatalf("first post-resume turn was not recoverable: %#v %#v", recovered.run, recovered.completed)
	}
	guard, found, err = st.GetRunProgressGuard(ctx, run.ID)
	if err != nil || !found || guard.Status != domain.RunProgressObserving ||
		guard.RepeatedActionCount != 1 || guard.StagnantTurnCount != 1 {
		t.Fatalf("post-resume guard was not reset: %#v found=%t err=%v", guard, found, err)
	}

	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countRunEventType(timeline, events.SupervisorProgressObservedEvent) != 4 ||
		countRunEventType(timeline, events.SupervisorLivelockDetectedEvent) != 1 ||
		countRunEventType(timeline, events.SupervisorProgressResetEvent) != 1 {
		t.Fatalf("progress audit timeline is incomplete: events=%#v err=%v", timeline, err)
	}
	for _, event := range timeline {
		if strings.HasPrefix(event.Type, "supervisor.progress_") ||
			event.Type == events.SupervisorLivelockDetectedEvent {
			if strings.Contains(event.PayloadJSON, "same response") {
				t.Fatalf("progress event leaked model text: %s", event.PayloadJSON)
			}
		}
	}
}

func TestRunProgressGuardPausesVaryingActionsWithoutObservableProgress(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	_, created := createWorkItemTestRun(t, ctx, st, "detect no observable progress")
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	var completed completedProgressTurn
	for turn := 1; turn <= domain.RunProgressStagnantThreshold; turn++ {
		completed = completeProgressTurn(t, ctx, st, lease,
			"different response "+strings.Repeat("x", turn))
	}
	guard, found, err := st.GetRunProgressGuard(ctx, run.ID)
	if err != nil || !found || completed.run.Status != domain.RunPaused ||
		guard.Reason != domain.RunProgressNoObservableProgress ||
		guard.StagnantTurnCount != domain.RunProgressStagnantThreshold ||
		guard.RepeatedActionCount != 1 {
		t.Fatalf("varying no-progress loop was not detected: run=%#v guard=%#v found=%t err=%v",
			completed.run, guard, found, err)
	}
}

func TestRunProgressGuardStateMutationResetsCounters(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	mission, created := createWorkItemTestRun(t, ctx, st, "reset on durable progress")
	run, err := application.NewRunService(st).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	completeProgressTurn(t, ctx, st, lease, "same response")
	completeProgressTurn(t, ctx, st, lease, "same response")
	createWorkItemTestItem(t, ctx, st, mission.ID, run.ID, "observable work", nil)
	completed := completeProgressTurn(t, ctx, st, lease, "same response")
	guard, found, err := st.GetRunProgressGuard(ctx, run.ID)
	if err != nil || !found || completed.run.Status != domain.RunRunning ||
		guard.RepeatedActionCount != 1 || guard.StagnantTurnCount != 1 {
		t.Fatalf("durable mutation did not reset progress counters: run=%#v guard=%#v err=%v",
			completed.run, guard, err)
	}
}

func TestSchemaV79UpgradePreservesRunWithoutFabricatingProgressGuard(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v78.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, run := createWorkItemTestRun(t, ctx, st, "preserve Run across v79")
	for _, statement := range removeSchemaV79ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			_ = st.Close()
			t.Fatalf("remove schema v79 with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	if preserved, err := upgraded.GetRun(ctx, run.ID); err != nil || preserved.ID != run.ID {
		t.Fatalf("Run was not preserved: %#v err=%v", preserved, err)
	}
	if guard, found, err := upgraded.GetRunProgressGuard(ctx, run.ID); err != nil || found {
		t.Fatalf("v79 fabricated a progress guard: %#v found=%t err=%v", guard, found, err)
	}
}

func completeProgressTurn(t *testing.T, ctx context.Context, st *SQLiteStore,
	lease domain.RunExecutionLease, text string,
) completedProgressTurn {
	t.Helper()
	turn, err := st.BeginSupervisorTurn(ctx, lease, "continue")
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, response := recordOperatorSteeringModelSuccess(t, ctx, st,
		turn.Checkpoint, text)
	action := domain.RootAction{Version: domain.RootLifecycleVersion,
		Kind: domain.RootActionContinue, Message: text}
	run, completed, _, err := st.CompleteSupervisorTurn(ctx, checkpoint, response,
		action, policy.Decision{Allowed: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return completedProgressTurn{checkpoint: checkpoint, response: response,
		action: action, run: run, completed: completed}
}
