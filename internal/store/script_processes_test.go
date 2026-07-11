package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/scriptprocess"
	"cyberagent-workbench/internal/toolgateway"
)

func TestSQLiteScriptProcessRunIsAtomicAndIdempotent(t *testing.T) {
	st := openScriptProcessTestStore(t)
	ctx := context.Background()
	service, _ := newScriptProcessTestService(t, st)
	request := scriptProcessTestRequest("atomic-script-request", "alpha")

	first, err := service.Create(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Process.Status != scriptprocess.StatusProposed ||
		first.Process.RunID != first.Run.ID || first.Run.SessionID != first.Process.SessionID {
		t.Fatalf("unexpected first script Run: %#v", first)
	}
	ledger, err := st.GetApprovalByProposal(ctx, first.Process.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ledger.RunID != first.Run.ID || ledger.Status != approval.StatusPending ||
		ledger.ToolName != string(toolgateway.ScriptProcessTool) {
		t.Fatalf("typed process approval was not bound atomically: %#v", ledger)
	}
	usage, err := st.GetToolCallUsage(ctx, first.Run.ID)
	if err != nil || usage.Consumed != 1 || usage.Limit != 3 || usage.Remaining != 2 {
		t.Fatalf("initial process budget charge is wrong: %#v err=%v", usage, err)
	}

	replayed, err := service.Create(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Process.ID != first.Process.ID || replayed.Run.ID != first.Run.ID ||
		replayed.Run.SessionID != first.Run.SessionID || replayed.Mission.ID != first.Mission.ID {
		t.Fatalf("idempotent replay changed identities: first=%#v replay=%#v", first, replayed)
	}
	for table, want := range map[string]int{
		"missions": 1, "runs": 1, "sessions": 1, "script_process_proposals": 1,
		"tool_approvals": 1, "run_tool_usage": 1, "run_tool_calls": 1,
	} {
		if got := countScriptProcessTestRows(t, st, table); got != want {
			t.Fatalf("%s row count = %d, want %d", table, got, want)
		}
	}
	timeline, err := st.ListRunEvents(ctx, first.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for eventType, want := range map[string]int{
		events.RunCreatedEvent: 1, events.SessionAttachedEvent: 1, events.ToolBudgetChargedEvent: 1,
		events.PolicyDecisionEvent: 1, events.ToolProposedEvent: 1, events.ApprovalRequestedEvent: 1,
	} {
		if got := countRunEventType(timeline, eventType); got != want {
			t.Fatalf("event %s count = %d, want %d", eventType, got, want)
		}
	}

	conflict := scriptProcessTestRequest("atomic-script-request", "different")
	if _, err := service.Create(ctx, conflict); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed idempotent request code = %s, err=%v", apperror.CodeOf(err), err)
	}
	if got := countScriptProcessTestRows(t, st, "runs"); got != 1 {
		t.Fatalf("idempotency conflict left %d runs", got)
	}
}

func TestSQLiteScriptProcessConcurrentReplayCreatesOneRun(t *testing.T) {
	st := openScriptProcessTestStore(t)
	service, _ := newScriptProcessTestService(t, st)
	ctx := context.Background()
	const workers = 12
	results := make(chan toolgateway.ScriptRunCreateResult, workers)
	errorsOut := make(chan error, workers)
	start := make(chan struct{})
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			result, err := service.Create(ctx, scriptProcessTestRequest("concurrent-script-request", "same"))
			if err != nil {
				errorsOut <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsOut)
	for err := range errorsOut {
		t.Fatalf("concurrent replay failed: %v", err)
	}
	processIDs := map[string]bool{}
	runIDs := map[string]bool{}
	count := 0
	for result := range results {
		count++
		processIDs[result.Process.ID] = true
		runIDs[result.Run.ID] = true
	}
	if count != workers || len(processIDs) != 1 || len(runIDs) != 1 {
		t.Fatalf("concurrent replay count=%d process_ids=%v run_ids=%v", count, processIDs, runIDs)
	}
	if countScriptProcessTestRows(t, st, "runs") != 1 ||
		countScriptProcessTestRows(t, st, "script_process_proposals") != 1 ||
		countScriptProcessTestRows(t, st, "run_tool_calls") != 1 {
		t.Fatal("concurrent replay duplicated durable state")
	}
}

func TestSQLiteScriptProcessEventFailureRollsBackEntireRun(t *testing.T) {
	st := openScriptProcessTestStore(t)
	ctx := context.Background()
	service, _ := newScriptProcessTestService(t, st)
	if _, err := st.db.ExecContext(ctx, `CREATE TRIGGER fail_script_tool_proposed
		BEFORE INSERT ON run_events WHEN NEW.type = 'tool.proposed'
		BEGIN SELECT RAISE(ABORT, 'injected script event failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, scriptProcessTestRequest("rollback-script-request", "value")); err == nil {
		t.Fatal("expected injected script event failure")
	}
	for _, table := range []string{
		"missions", "runs", "sessions", "run_events", "run_tool_usage", "run_tool_calls",
		"script_process_proposals", "tool_approvals",
	} {
		if got := countScriptProcessTestRows(t, st, table); got != 0 {
			t.Fatalf("failed atomic creation left %d rows in %s", got, table)
		}
	}
}

func TestSQLiteScriptProcessCannotBypassApprovalLedger(t *testing.T) {
	st := openScriptProcessTestStore(t)
	ctx := context.Background()
	service, gateway := newScriptProcessTestService(t, st)
	created, err := service.Create(ctx, scriptProcessTestRequest("approval-gate-script", "value"))
	if err != nil {
		t.Fatal(err)
	}
	bypass := created.Process
	bypass.Status = scriptprocess.StatusApproved
	bypass.Version++
	bypass.UpdatedAt = time.Now().UTC().Add(time.Second)
	if _, err := st.SaveScriptProcess(ctx, bypass); err == nil {
		t.Fatal("expected direct process approval to require an approved ledger")
	}
	persisted, err := st.GetScriptProcess(ctx, created.Process.ID)
	if err != nil || persisted.Status != scriptprocess.StatusProposed || persisted.Version != 1 {
		t.Fatalf("failed approval bypass changed process: %#v err=%v", persisted, err)
	}
	ledger, err := st.GetApprovalByProposal(ctx, created.Process.ID)
	if err != nil || ledger.Status != approval.StatusPending || ledger.Version != 1 {
		t.Fatalf("failed approval bypass changed ledger: %#v err=%v", ledger, err)
	}

	reviewed, err := gateway.Review(ctx, toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, Tool: toolgateway.ScriptProcessTool,
		ProposalID: created.Process.ID, ReviewedBy: "test_operator",
	})
	if err != nil || reviewed.Proposal == nil || reviewed.Proposal.Status != toolgateway.StatusCompleted ||
		reviewed.Execution == nil || reviewed.Execution.Backend != "dry_run" {
		t.Fatalf("ledger-backed process approval failed: %#v err=%v", reviewed, err)
	}
	ledger, err = st.GetApprovalByProposal(ctx, created.Process.ID)
	if err != nil || ledger.Status != approval.StatusApproved {
		t.Fatalf("approved process ledger is wrong: %#v err=%v", ledger, err)
	}
}

func TestSQLiteScriptProcessAllowsMultipleCallsPerRunAndRejectsCrossRunBinding(t *testing.T) {
	st := openScriptProcessTestStore(t)
	ctx := context.Background()
	service, gateway := newScriptProcessTestService(t, st)
	created, err := service.Create(ctx, scriptProcessTestRequest("multi-process-run", "first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := gateway.ProposeScriptProcess(ctx, toolgateway.ToolCall{
		Name: toolgateway.ScriptProcessTool, RunID: created.Run.ID, SessionID: created.Run.SessionID,
		WorkspaceID: created.Mission.WorkspaceID, RequestedBy: "multi_process_test",
	}, toolgateway.ScriptProcessProposal{
		Executable: "python", Arguments: []string{"scripts/second.py"}, RequestedBackend: "sandbox",
	})
	if err != nil || second.Proposal == nil || second.Proposal.ID == created.Process.ID {
		t.Fatalf("second process in one Run failed: %#v err=%v", second, err)
	}
	processes, err := st.ListScriptProcesses(ctx, scriptprocess.ListFilter{RunID: created.Run.ID})
	if err != nil || len(processes) != 2 {
		t.Fatalf("one Run should contain two process proposals: %#v err=%v", processes, err)
	}
	usage, err := st.GetToolCallUsage(ctx, created.Run.ID)
	if err != nil || usage.Consumed != 2 {
		t.Fatalf("multiple process calls did not share Run budget: %#v err=%v", usage, err)
	}

	_, otherRun, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "other run", Profile: "script", WorkspaceID: created.Mission.WorkspaceID,
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	forged := created.Process
	forged.ID = "process-cross-run-binding"
	forged.OperationKeyDigest = scriptprocess.Fingerprint("cross-run-operation")
	forged.RequestFingerprint = scriptprocess.Fingerprint("cross-run-request")
	forged.ApprovalFingerprint = scriptprocess.Fingerprint("cross-run-approval")
	forged.RunID = otherRun.ID
	forged.Version = 1
	forged.CreatedAt = time.Now().UTC()
	forged.UpdatedAt = forged.CreatedAt
	if _, err := st.SaveScriptProcess(ctx, forged); err == nil {
		t.Fatal("expected process Run/Session binding mismatch rejection")
	}
	if _, err := st.GetScriptProcess(ctx, forged.ID); err == nil {
		t.Fatal("cross-Run process survived transaction rollback")
	}
}

func TestSQLiteUpgradesSchemaV12ToTypedScriptProcessesWithoutLosingRunOrGrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v12.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "preserve v12 run and grant", Profile: "code", WorkspaceID: "ws-v12",
		Budget: domain.Budget{MaxTurns: 3, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	grantResult, err := st.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: run.SessionID, WorkspaceID: "ws-v12", ToolName: string(toolgateway.ShellTool),
		ActionClass: string(toolgateway.ClassShell), Reason: "preserve migration grant",
		GrantedBy: "migration_test", IdempotencyKey: "migration-grant",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TABLE structured_tool_operations`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 15`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TABLE run_artifacts`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 14`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DROP TABLE script_process_proposals`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 13`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	loadedRun, err := st.GetRun(ctx, run.ID)
	if err != nil || loadedRun.ID != run.ID || loadedRun.SessionID != run.SessionID {
		t.Fatalf("v12 Run was not preserved: %#v err=%v", loadedRun, err)
	}
	loadedGrant, err := st.GetSessionGrant(ctx, grantResult.Grant.ID)
	if err != nil || loadedGrant.ID != grantResult.Grant.ID || loadedGrant.Status != approval.GrantActive {
		t.Fatalf("v12 Grant was not preserved: %#v err=%v", loadedGrant, err)
	}
	version, err := st.SchemaVersion(ctx)
	if err != nil || version != LatestSchemaVersion {
		t.Fatalf("v12 database did not migrate to latest: version=%d err=%v", version, err)
	}
	service, _ := newScriptProcessTestService(t, st)
	created, err := service.Create(ctx, scriptProcessTestRequest("post-migration-script", "value"))
	if err != nil || created.Process.ID == "" {
		t.Fatalf("migrated typed process store is unusable: %#v err=%v", created, err)
	}
}

func openScriptProcessTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "script-process.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newScriptProcessTestService(t *testing.T, st *SQLiteStore) (*application.ScriptProcessService, *toolgateway.Gateway) {
	t.Helper()
	root := t.TempDir()
	gateway := toolgateway.New(st, policy.NewDefaultChecker()).WithWorkspaceRootResolver(
		func(context.Context, string) (string, error) { return root, nil },
	)
	return application.NewScriptProcessService(st, gateway), gateway
}

func scriptProcessTestRequest(operationKey string, value string) application.CreateScriptProcessRunRequest {
	return application.CreateScriptProcessRunRequest{
		Run: application.CreateRunRequest{
			Goal: "review typed script process", Profile: "script", WorkspaceID: "ws-script",
			Interactive: true, Budget: domain.Budget{MaxTurns: 5, MaxToolCalls: 3},
		},
		OperationKey: operationKey, RequestedBy: "script_process_test",
		Process: toolgateway.ScriptProcessProposal{
			Executable: "python", Arguments: []string{"scripts/noop.py", value}, RequestedBackend: "sandbox",
		},
	}
}

func countScriptProcessTestRows(t *testing.T, st *SQLiteStore, table string) int {
	t.Helper()
	allowed := map[string]bool{
		"missions": true, "runs": true, "sessions": true, "run_events": true,
		"run_tool_usage": true, "run_tool_calls": true, "script_process_proposals": true,
		"tool_approvals": true,
	}
	if !allowed[table] {
		t.Fatalf("test attempted to count unsupported table %q", table)
	}
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
