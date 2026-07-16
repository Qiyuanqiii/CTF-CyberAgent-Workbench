package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
)

func TestRunExecutionProfileSelectionIsImmutableIdempotentAndLeaseGuarded(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "run-execution-profile.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runs := application.NewRunService(st)
	_, run, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "select a bounded execution environment", Profile: "code",
		Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	profiles := application.NewRunExecutionProfileService(st)
	initial, err := profiles.Current(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Profile != domain.RunExecutionProfilePreview || initial.Revision != 1 ||
		initial.ProcessEnabled || initial.ExecutionAuthorized || initial.CapabilityGrant {
		t.Fatalf("unexpected default profile: %#v", initial)
	}
	request := application.ChangeRunExecutionProfileRequest{
		RunID: run.ID, Profile: "docker",
		OperationKey: "execution-profile-operation-0001",
		RequestedBy:  "test_operator", Reason: "prefer isolated execution",
	}
	selected, err := profiles.Change(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Replayed || selected.Profile.Revision != 2 ||
		selected.Profile.Profile != domain.RunExecutionProfileDocker ||
		selected.Profile.RequiredGate != domain.ExecutionGateDockerProductionStart ||
		selected.Profile.ProcessEnabled || selected.Profile.ExecutionAuthorized ||
		selected.Profile.CapabilityGrant {
		t.Fatalf("unexpected Docker selection: %#v", selected)
	}
	replayed, err := profiles.Change(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Profile.ID != selected.Profile.ID {
		t.Fatalf("selection replay changed result: %#v err=%v", replayed, err)
	}
	request.Profile = "local"
	if _, err := profiles.Change(ctx, request); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("reused key error=%v", err)
	}
	for _, statement := range []string{
		`UPDATE run_execution_profile_snapshots SET process_enabled = 1 WHERE id = ?`,
		`DELETE FROM run_execution_profile_snapshots WHERE id = ?`,
	} {
		if _, err := st.db.ExecContext(ctx, statement, selected.Profile.ID); err == nil {
			t.Fatalf("immutable profile statement succeeded: %s", statement)
		}
	}
	eventsList, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || len(eventsList) == 0 ||
		eventsList[len(eventsList)-1].Type != events.RunExecutionProfileSelectedEvent {
		t.Fatalf("selection event missing: events=%#v err=%v", eventsList, err)
	}
	if _, err := runs.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	_ = acquireTestRunExecutionLease(t, ctx, st, run.ID)
	if _, err := runs.Pause(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	_, err = profiles.Change(ctx, application.ChangeRunExecutionProfileRequest{
		RunID: run.ID, Profile: "local",
		OperationKey: "execution-profile-operation-0002",
		RequestedBy:  "test_operator", Reason: "lease must block selection",
	})
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		!strings.Contains(err.Error(), "lease") {
		t.Fatalf("active lease profile error=%v", err)
	}
}

func TestSchemaV64BackfillsPreviewExecutionProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema-v63-execution-profile.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "legacy v63 Run", Profile: "review", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV64ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("downgrade v64 fixture with %q: %v", statement, err)
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
	profile, err := upgraded.GetRunExecutionProfile(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Profile != domain.RunExecutionProfilePreview || profile.Revision != 1 ||
		profile.RequestedBy != "schema_v64" || profile.Backend != domain.ExecutionBackendNoop ||
		profile.ProcessEnabled || profile.ExecutionAuthorized || profile.CapabilityGrant {
		t.Fatalf("unexpected v64 compatibility profile: %#v", profile)
	}
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
}
