package application_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/store"
)

func TestControlledRunCreationIsClosedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	state := openRunCreationStore(t, filepath.Join(t.TempDir(), "state.db"))
	workspace := saveRunCreationWorkspace(t, state)
	service := application.NewControlledRunCreationService(state)
	request := application.ControlledRunCreationRequest{
		Version: domain.RunCreationProtocolVersion, Goal: "Implement the parser",
		WorkspaceID: workspace.ID, Profile: "code", Surface: "code", Phase: "plan",
		OperationKey: "run-create-operation-0001", RequestedBy: "http_control",
	}
	created, err := service.Create(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if created.Replayed || created.Run.Status != domain.RunCreated ||
		!created.Run.Config.Interactive || created.Run.Config.ModelRoute != "code" ||
		created.Run.Budget != domain.DefaultBudget() || created.Mission.Scope.NetworkMode != "disabled" ||
		len(created.Mission.Scope.AllowedTargets) != 0 || created.Session.ID != created.Run.SessionID ||
		created.Session.Route != "code" || created.Mode.RequestedBy != "http_control" ||
		created.Mode.Phase != domain.ExecutionPhasePlan {
		t.Fatalf("unexpected controlled creation: %#v", created)
	}
	operation, found, err := state.GetRunCreationOperation(ctx,
		runmutation.RunCreationOperationDigest(request.OperationKey))
	if err != nil || !found || operation.RunID != created.Run.ID ||
		operation.SessionID != created.Session.ID {
		t.Fatalf("creation operation was not persisted: %#v found=%t err=%v", operation, found, err)
	}
	replayed, err := service.Create(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Run.ID != created.Run.ID ||
		replayed.Mission.ID != created.Mission.ID || replayed.Session.ID != created.Session.ID {
		t.Fatalf("idempotent replay changed identities: %#v", replayed)
	}
	changed := request
	changed.Goal = "Implement a different parser"
	if _, err := service.Create(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed intent code=%s err=%v", apperror.CodeOf(err), err)
	}
	runs, err := state.ListRuns(ctx, domain.RunFilter{Limit: 10})
	if err != nil || len(runs) != 1 {
		t.Fatalf("replay created extra Runs: count=%d err=%v", len(runs), err)
	}
	sessions, err := state.ListSessions(ctx)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("replay created extra Sessions: count=%d err=%v", len(sessions), err)
	}
}

func TestControlledRunCreationConvergesAcrossSQLiteConnections(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shared.db")
	firstStore := openRunCreationStore(t, path)
	workspace := saveRunCreationWorkspace(t, firstStore)
	secondStore := openRunCreationStore(t, path)
	request := application.ControlledRunCreationRequest{
		Version: domain.RunCreationProtocolVersion, Goal: "Concurrent creation",
		WorkspaceID: workspace.ID, Profile: "review", Surface: "code", Phase: "deliver",
		OperationKey: "run-create-operation-0002", RequestedBy: "http_control",
	}
	services := []*application.ControlledRunCreationService{
		application.NewControlledRunCreationService(firstStore),
		application.NewControlledRunCreationService(secondStore),
	}
	start := make(chan struct{})
	results := make(chan application.ControlledRunCreationResult, 2)
	errorsFound := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, service := range services {
		go func(current *application.ControlledRunCreationService) {
			ready.Done()
			<-start
			result, err := current.Create(ctx, request)
			if err != nil {
				errorsFound <- err
				return
			}
			results <- result
		}(service)
	}
	ready.Wait()
	close(start)
	var collected []application.ControlledRunCreationResult
	for range 2 {
		select {
		case err := <-errorsFound:
			t.Fatal(err)
		case result := <-results:
			collected = append(collected, result)
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent Run creation timed out")
		}
	}
	if collected[0].Run.ID != collected[1].Run.ID ||
		collected[0].Session.ID != collected[1].Session.ID ||
		collected[0].Replayed == collected[1].Replayed {
		t.Fatalf("concurrent requests did not converge: %#v", collected)
	}
	runs, err := firstStore.ListRuns(ctx, domain.RunFilter{Limit: 10})
	if err != nil || len(runs) != 1 {
		t.Fatalf("concurrent creation persisted %d Runs: %v", len(runs), err)
	}
}

func TestControlledRunCreationReplayRejectsRunThatLeftInitialState(t *testing.T) {
	ctx := context.Background()
	state := openRunCreationStore(t, filepath.Join(t.TempDir(), "state.db"))
	workspace := saveRunCreationWorkspace(t, state)
	service := application.NewControlledRunCreationService(state)
	request := application.ControlledRunCreationRequest{
		Version: domain.RunCreationProtocolVersion, Goal: "Initial-only replay",
		WorkspaceID: workspace.ID, OperationKey: "run-create-operation-initial-state",
		RequestedBy: "http_control",
	}
	created, err := service.Create(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewRunService(state).Start(ctx, created.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, request); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("replay after Run start code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestControlledRunCreationRejectsInvalidBoundaryValues(t *testing.T) {
	state := openRunCreationStore(t, filepath.Join(t.TempDir(), "state.db"))
	workspace := saveRunCreationWorkspace(t, state)
	service := application.NewControlledRunCreationService(state)
	base := application.ControlledRunCreationRequest{
		Version: domain.RunCreationProtocolVersion, Goal: "Valid goal", WorkspaceID: workspace.ID,
		OperationKey: "run-create-operation-0003", RequestedBy: "http_control",
	}
	cases := []application.ControlledRunCreationRequest{
		{Version: "run_creation.v2", Goal: base.Goal, WorkspaceID: base.WorkspaceID,
			OperationKey: base.OperationKey, RequestedBy: base.RequestedBy},
		{Version: base.Version, Goal: string([]byte{0xff}), WorkspaceID: base.WorkspaceID,
			OperationKey: base.OperationKey, RequestedBy: base.RequestedBy},
		{Version: base.Version, Goal: base.Goal, WorkspaceID: "missing-workspace",
			OperationKey: base.OperationKey, RequestedBy: base.RequestedBy},
		{Version: base.Version, Goal: base.Goal, WorkspaceID: base.WorkspaceID,
			OperationKey: "contains whitespace key", RequestedBy: base.RequestedBy},
		{Version: base.Version, Goal: base.Goal, WorkspaceID: " " + base.WorkspaceID,
			OperationKey: base.OperationKey, RequestedBy: base.RequestedBy},
		{Version: base.Version, Goal: base.Goal, WorkspaceID: base.WorkspaceID,
			Profile: "Code", OperationKey: base.OperationKey, RequestedBy: base.RequestedBy},
	}
	for index, request := range cases {
		if _, err := service.Create(context.Background(), request); err == nil {
			t.Fatalf("invalid request %d was accepted", index)
		}
	}
}

func openRunCreationStore(t *testing.T, path string) *store.SQLiteStore {
	t.Helper()
	state, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	return state
}

func saveRunCreationWorkspace(t *testing.T, state *store.SQLiteStore) store.WorkspaceRecord {
	t.Helper()
	record := store.WorkspaceRecord{ID: "workspace-controlled-create", Name: "controlled-create",
		RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := state.SaveWorkspace(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	return record
}
