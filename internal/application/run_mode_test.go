package application_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/store"
)

type delayedRunModeReplayStore struct {
	application.RunStore
	lookupDone chan struct{}
	atGetRun   chan struct{}
	release    chan struct{}
	lookupOnce sync.Once
	getRunOnce sync.Once
}

func (s *delayedRunModeReplayStore) GetRunModeOperation(ctx context.Context,
	keyDigest string,
) (domain.RunModeOperation, bool, error) {
	operation, found, err := s.RunStore.GetRunModeOperation(ctx, keyDigest)
	s.lookupOnce.Do(func() { close(s.lookupDone) })
	return operation, found, err
}

func (s *delayedRunModeReplayStore) GetRun(ctx context.Context,
	id string,
) (domain.Run, error) {
	s.getRunOnce.Do(func() { close(s.atGetRun) })
	select {
	case <-ctx.Done():
		return domain.Run{}, ctx.Err()
	case <-s.release:
		return s.RunStore.GetRun(ctx, id)
	}
}

func TestRunModeSelectionTransitionReplayAndStatusBoundary(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "run-mode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "plan an authorized lab review", Profile: "review", Surface: "cyber",
		Phase: "plan", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := service.Mode(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Surface != domain.ExecutionSurfaceCyber ||
		initial.Phase != domain.ExecutionPhasePlan || initial.Revision != 1 {
		t.Fatalf("unexpected initial mode: %#v", initial)
	}
	request := application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "deliver", OperationKey: "phase-change-0001",
		RequestedBy: "operator", Reason: "API_KEY=sk-" + strings.Repeat("a", 32),
	}
	changed, err := service.ChangePhase(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Replayed || changed.Mode.Revision != 2 ||
		changed.Mode.Phase != domain.ExecutionPhaseDeliver ||
		strings.Contains(changed.Mode.Reason, "sk-") {
		t.Fatalf("unexpected phase transition: %#v", changed)
	}
	replayed, err := service.ChangePhase(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Mode.ID != changed.Mode.ID {
		t.Fatalf("phase replay mismatch: result=%#v err=%v", replayed, err)
	}
	conflict := request
	conflict.Phase = "plan"
	if _, err := service.ChangePhase(ctx, conflict); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed operation intent error = %v", err)
	}
	back, err := service.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "plan", OperationKey: "phase-change-0002",
		RequestedBy: "operator", Reason: "return for review",
	})
	if err != nil || back.Mode.Revision != 3 {
		t.Fatalf("second phase transition result=%#v err=%v", back, err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "deliver", OperationKey: "phase-change-0003",
	}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("running phase transition error = %v", err)
	}
	items, err := service.Events(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.RunModeSelectedEvent) != 1 ||
		countEventType(items, events.RunPhaseChangedEvent) != 2 {
		t.Fatalf("unexpected mode event ledger: %#v", items)
	}
	for _, item := range items {
		if strings.Contains(item.PayloadJSON, "sk-") {
			t.Fatalf("mode event leaked secret-shaped input: %s", item.PayloadJSON)
		}
	}
}

func TestRunModeRejectsInvalidCreateAndNoopTransition(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "run-mode-invalid.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	if _, _, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "bad surface", Surface: "work", Budget: domain.Budget{MaxTurns: 1},
	}); err == nil {
		t.Fatal("invalid create surface was accepted")
	}
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "default mode", Budget: domain.Budget{MaxTurns: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	mode, err := service.Mode(ctx, run.ID)
	if err != nil || mode.Surface != domain.ExecutionSurfaceCode ||
		mode.Phase != domain.ExecutionPhaseDeliver {
		t.Fatalf("default mode=%#v err=%v", mode, err)
	}
	if _, err := service.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "deliver", OperationKey: "phase-noop-key-0001",
	}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("same phase transition error = %v", err)
	}
}

func TestRunPhaseCanChangeWhilePaused(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "run-mode-paused.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "pause before planning", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Pause(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	result, err := service.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "plan", OperationKey: "paused-phase-change-0001",
		RequestedBy: "operator", Reason: "return to planning",
	})
	if err != nil || result.Mode.Phase != domain.ExecutionPhasePlan || result.Mode.Revision != 2 {
		t.Fatalf("paused phase result=%#v err=%v", result, err)
	}
}

func TestConcurrentRunPhaseReplayConvergesAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-mode-concurrent.db")
	firstStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer firstStore.Close()
	secondStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondStore.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(firstStore).Create(ctx,
		application.CreateRunRequest{
			Goal: "concurrent mode", Phase: "plan", Budget: domain.Budget{MaxTurns: 2},
		})
	if err != nil {
		t.Fatal(err)
	}
	request := application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "deliver", OperationKey: "concurrent-phase-key-0001",
		RequestedBy: "operator", Reason: "approved",
	}
	services := []*application.RunService{
		application.NewRunService(firstStore), application.NewRunService(secondStore),
	}
	results := make([]application.ChangeRunPhaseResult, len(services))
	errorsFound := make([]error, len(services))
	var group sync.WaitGroup
	for index := range services {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errorsFound[index] = services[index].ChangePhase(ctx, request)
		}(index)
	}
	group.Wait()
	for index, found := range errorsFound {
		if found != nil {
			t.Fatalf("concurrent transition %d failed: %v", index, found)
		}
	}
	if results[0].Mode.ID != results[1].Mode.ID ||
		results[0].Mode.Revision != 2 || results[1].Mode.Revision != 2 ||
		results[0].Replayed == results[1].Replayed {
		t.Fatalf("concurrent transition did not converge: %#v", results)
	}
}

func TestRunPhaseReplayRechecksOperationAfterConcurrentCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-mode-recheck.db")
	primaryStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer primaryStore.Close()
	delayedStore, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer delayedStore.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(primaryStore).Create(ctx,
		application.CreateRunRequest{
			Goal: "deterministic concurrent mode", Phase: "plan",
			Budget: domain.Budget{MaxTurns: 2},
		})
	if err != nil {
		t.Fatal(err)
	}
	request := application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "deliver", OperationKey: "phase-recheck-key-0001",
		RequestedBy: "operator", Reason: "approved",
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseNow := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseNow()
	delayed := &delayedRunModeReplayStore{
		RunStore: delayedStore, lookupDone: make(chan struct{}),
		atGetRun: make(chan struct{}), release: release,
	}
	type outcome struct {
		result application.ChangeRunPhaseResult
		err    error
	}
	delayedResult := make(chan outcome, 1)
	go func() {
		result, err := application.NewRunService(delayed).ChangePhase(ctx, request)
		delayedResult <- outcome{result: result, err: err}
	}()
	select {
	case <-delayed.lookupDone:
	case <-time.After(5 * time.Second):
		t.Fatal("delayed operation lookup did not complete")
	}
	select {
	case <-delayed.atGetRun:
	case <-time.After(5 * time.Second):
		t.Fatal("delayed phase change did not reach the stale-read window")
	}
	primary, err := application.NewRunService(primaryStore).ChangePhase(ctx, request)
	if err != nil || primary.Replayed {
		t.Fatalf("primary phase transition failed: result=%#v err=%v", primary, err)
	}
	releaseNow()
	select {
	case recovered := <-delayedResult:
		if recovered.err != nil || !recovered.result.Replayed ||
			recovered.result.Mode.ID != primary.Mode.ID {
			t.Fatalf("delayed phase replay did not recover: primary=%#v delayed=%#v err=%v",
				primary, recovered.result, recovered.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delayed phase replay did not finish")
	}
}

func TestRunPhaseRejectsPausedRunWithActiveExecutionLease(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "run-mode-leased.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "leased phase boundary", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	_ = acquireTestRunExecutionLease(t, ctx, st, run.ID)
	if _, err := service.Pause(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	_, err = service.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "plan", OperationKey: "leased-phase-change-0001",
		RequestedBy: "operator", Reason: "must wait for worker",
	})
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		!strings.Contains(err.Error(), "execution lease") {
		t.Fatalf("active lease phase transition error = %v", err)
	}
}

func TestPlanCompletionIsRejectedAtStoreBoundary(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "run-mode-finalize.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "plan cannot complete", Phase: "plan", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	if _, _, err := st.FinalizeSupervisorRun(ctx, lease, domain.RunCompleted,
		"bypass application check"); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		!strings.Contains(err.Error(), "persistence boundary") {
		t.Fatalf("store plan completion error = %v", err)
	}
}
