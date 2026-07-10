package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/toolrun"
)

func TestDurableApprovalDecisionIsIdempotent(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := toolrun.ToolRun{
		ID: "tool-idempotent", ToolName: toolrun.ShellTool, Command: "echo safe",
		Status: toolrun.StatusProposed, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := st.SaveToolRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	pending, err := st.GetApprovalByProposal(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != approval.StatusPending || pending.Version != 1 || pending.RequestFingerprint == "" {
		t.Fatalf("unexpected pending approval: %#v", pending)
	}
	request := approval.DecisionRequest{
		ProposalID: run.ID, IdempotencyKey: "review:shell:tool-idempotent:approve",
		Action: approval.ActionApprove, ReviewedBy: "test_operator",
	}
	first, err := st.DecideApproval(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Approval.Status != approval.StatusApproved || first.Approval.Version != 2 {
		t.Fatalf("unexpected first decision: %#v", first)
	}
	replay, err := st.DecideApproval(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.Approval.Version != first.Approval.Version {
		t.Fatalf("decision did not replay: %#v", replay)
	}
	var operations int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_operations`).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 1 {
		t.Fatalf("expected one immutable operation, got %d", operations)
	}
	var storedOperationKey string
	if err := st.db.QueryRowContext(ctx, `SELECT idempotency_key FROM approval_operations`).Scan(&storedOperationKey); err != nil {
		t.Fatal(err)
	}
	if storedOperationKey == request.IdempotencyKey || storedOperationKey != approval.OperationKeyDigest(request.IdempotencyKey) {
		t.Fatalf("raw operation key was persisted: %q", storedOperationKey)
	}
	conflicting := request
	conflicting.Action = approval.ActionDeny
	conflicting.Reason = "changed intent"
	if _, err := st.DecideApproval(ctx, conflicting); err == nil {
		t.Fatal("expected idempotency-key reuse with a different action to fail")
	}
}

func TestApprovalReadFiltersAreBounded(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if _, err := st.GetApproval(ctx, ""); err == nil {
		t.Fatal("expected empty approval id to fail")
	}
	if _, err := st.GetApprovalByProposal(ctx, strings.Repeat("x", approval.MaxIdentityRunes+1)); err == nil {
		t.Fatal("expected oversized proposal id to fail")
	}
	if _, err := st.ListApprovals(ctx, approval.ListFilter{SessionID: string([]byte{0xff})}); err == nil {
		t.Fatal("expected invalid UTF-8 filter to fail")
	}
	if _, err := st.ListApprovals(ctx, approval.ListFilter{Limit: 501}); err == nil {
		t.Fatal("expected oversized list limit to fail")
	}
}

func TestDurableApprovalBlocksUnapprovedAndTamperedTransitions(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	run := toolrun.ToolRun{
		ID: "tool-guard", ToolName: toolrun.ShellTool, Command: "echo original",
		Status: toolrun.StatusProposed, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := st.SaveToolRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	tampered := run
	tampered.Command = "echo changed"
	tampered.UpdatedAt = time.Now().UTC()
	if _, err := st.SaveToolRun(ctx, tampered); err == nil {
		t.Fatal("expected proposal fingerprint mutation to fail")
	}
	completed := run
	completed.Status = toolrun.StatusCompleted
	completed.UpdatedAt = time.Now().UTC()
	if _, err := st.SaveToolRun(ctx, completed); err == nil {
		t.Fatal("expected completion without durable approval to fail")
	}
	loaded, err := st.GetToolRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != toolrun.StatusProposed || loaded.Command != run.Command {
		t.Fatalf("rejected transition changed the proposal: %#v", loaded)
	}

	edit := fileedit.Edit{
		ID: "edit-guard", WorkspaceID: "ws-test", Path: "README.md", Status: fileedit.StatusApplied,
		OriginalText: "old", ProposedText: "new", Diff: "-old\n+new", OriginalHash: "old",
		ProposedHash: fileedit.HashText("new"), CreatedAt: now, UpdatedAt: now,
	}
	if _, err := st.SaveFileEdit(ctx, edit); err == nil {
		t.Fatal("expected a new applied file edit to be rejected")
	}
	if _, err := st.GetFileEdit(ctx, edit.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rejected file edit was persisted: %v", err)
	}
	if _, err := st.EnsureApproval(ctx, approval.Proposal{
		IdempotencyKey: "proposal:shell:ghost", ProposalID: "ghost", ToolName: "shell", ActionClass: "shell",
		Mode: "per_call", Status: approval.StatusPending, RequestFingerprint: approval.ShellFingerprint("", "", "echo ghost"),
		RequestedBy: "test", CreatedAt: now, UpdatedAt: now,
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ghost proposal created an approval: %v", err)
	}
}

func TestPolicyDeniedApprovalRemainsNeverOnIdempotentSave(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	run := toolrun.ToolRun{
		ID: "tool-policy-denied", ToolName: toolrun.ShellTool, Command: "masscan 0.0.0.0/0",
		Status: toolrun.StatusDenied, Risk: "critical", PolicyReason: "public attack denied",
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := st.SaveToolRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveToolRun(ctx, run); err != nil {
		t.Fatalf("idempotent policy denial failed: %v", err)
	}
	record, err := st.GetApprovalByProposal(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Mode != "never" || record.Status != approval.StatusDenied || record.Version != 1 {
		t.Fatalf("policy denial changed approval semantics: %#v", record)
	}
}

func TestConcurrentApprovalDecisionsHaveOneWinner(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := toolrun.ToolRun{
		ID: "tool-concurrent", ToolName: toolrun.ShellTool, Command: "echo concurrency",
		Status: toolrun.StatusProposed, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := st.SaveToolRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	requests := []approval.DecisionRequest{
		{ProposalID: run.ID, IdempotencyKey: "concurrent-approve", Action: approval.ActionApprove, ReviewedBy: "operator-a"},
		{ProposalID: run.ID, IdempotencyKey: "concurrent-deny", Action: approval.ActionDeny, Reason: "operator-b denied", ReviewedBy: "operator-b"},
	}
	results := make([]error, len(requests))
	var wait sync.WaitGroup
	for index := range requests {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, results[index] = st.DecideApproval(ctx, requests[index])
		}(index)
	}
	wait.Wait()
	successes := 0
	for _, result := range results {
		if result == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("expected one concurrent decision winner, got %d errors=%v", successes, results)
	}
	record, err := st.GetApprovalByProposal(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != approval.StatusApproved && record.Status != approval.StatusDenied {
		t.Fatalf("unexpected concurrent decision state: %#v", record)
	}
	var operations int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_operations WHERE approval_id = ?`, record.ID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 1 {
		t.Fatalf("expected one winning operation, got %d", operations)
	}
}

func TestApprovalDecisionEventFailureRollsBackDecisionAndOperation(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "approval rollback", Profile: "code", WorkspaceID: "ws-rollback", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	tool := toolrun.ToolRun{
		ID: "tool-rollback", SessionID: run.SessionID, WorkspaceID: "ws-rollback",
		ToolName: toolrun.ShellTool, Command: "echo rollback", Status: toolrun.StatusProposed,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := st.SaveToolRun(ctx, tool); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `CREATE TRIGGER fail_approval_decided
		BEFORE INSERT ON run_events WHEN NEW.type = 'approval.decided'
		BEGIN SELECT RAISE(ABORT, 'injected approval event failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DecideApproval(ctx, approval.DecisionRequest{
		ProposalID: tool.ID, IdempotencyKey: "rollback-decision", Action: approval.ActionApprove, ReviewedBy: "test",
	}); err == nil {
		t.Fatal("expected injected event failure")
	}
	record, err := st.GetApprovalByProposal(ctx, tool.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != approval.StatusPending || record.Version != 1 {
		t.Fatalf("failed event did not roll back approval: %#v", record)
	}
	var operations int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_operations WHERE approval_id = ?`, record.ID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatalf("failed event left an operation record: %d", operations)
	}
}

func TestApprovalLazilyBindsWhenLegacySessionGetsRun(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	sess := session.New("ws-late-binding", "legacy", "code")
	if err := st.SaveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	tool := toolrun.ToolRun{
		ID: "tool-late-binding", SessionID: sess.ID, WorkspaceID: sess.WorkspaceID,
		ToolName: toolrun.ShellTool, Command: "echo late", Status: toolrun.StatusProposed,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := st.SaveToolRun(ctx, tool); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetApprovalByProposal(ctx, tool.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.RunID != "" {
		t.Fatalf("legacy approval unexpectedly had a run: %#v", before)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "adopt legacy session", Profile: "code", WorkspaceID: sess.WorkspaceID,
		SessionID: sess.ID, Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := st.EnsureApproval(ctx, approval.Proposal{
		IdempotencyKey: approval.ProposalIdempotencyKey("shell", tool.ID), ProposalID: tool.ID,
		SessionID: tool.SessionID, WorkspaceID: tool.WorkspaceID, ToolName: "shell", ActionClass: "shell",
		Mode: "per_call", Status: approval.StatusPending,
		RequestFingerprint: approval.ShellFingerprint(tool.SessionID, tool.WorkspaceID, tool.Command),
		RequestedBy:        "binding_test", CreatedAt: tool.CreatedAt, UpdatedAt: tool.UpdatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if after.RunID != run.ID || after.Version != before.Version+1 {
		t.Fatalf("approval was not bound to the run: before=%#v after=%#v", before, after)
	}
	runEvents, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	bound := 0
	for _, event := range runEvents {
		if event.Type == events.ApprovalBoundEvent && event.SubjectID == after.ID {
			bound++
		}
	}
	if bound != 1 {
		t.Fatalf("expected one approval.bound event, got %d events=%#v", bound, runEvents)
	}
}

func TestSchemaV10UpgradeCreatesEmptyApprovalLedgerAndPreservesProposal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v10.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	run := toolrun.ToolRun{
		ID: "tool-v10", ToolName: toolrun.ShellTool, Command: "echo legacy",
		Status: toolrun.StatusProposed, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := st.SaveToolRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	removeSchemaV12ForTest(t, st, ctx)
	for _, table := range []string{"approval_operations", "tool_approvals"} {
		if _, err := st.db.ExecContext(ctx, `DROP TABLE `+table); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 11`); err != nil {
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
	if _, err := st.GetToolRun(ctx, run.ID); err != nil {
		t.Fatalf("legacy proposal was not preserved: %v", err)
	}
	if _, err := st.GetApprovalByProposal(ctx, run.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("v10 proposal should be lazily adopted, got %v", err)
	}
	record, err := st.EnsureApproval(ctx, approval.Proposal{
		IdempotencyKey: approval.ProposalIdempotencyKey("shell", run.ID), ProposalID: run.ID,
		ToolName: "shell", ActionClass: "shell", Mode: "per_call", Status: approval.StatusPending,
		RequestFingerprint: approval.ShellFingerprint("", "", run.Command), RequestedBy: "migration_test",
		CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
	})
	if err != nil || record.Status != approval.StatusPending {
		t.Fatalf("legacy proposal could not be adopted: %#v err=%v", record, err)
	}
}

func TestSchemaV11UpgradePreservesApprovalAndEnablesSessionGrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v11.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "preserve v11 approval", Profile: "code", WorkspaceID: "ws-v11",
		Budget: domain.Budget{MaxTurns: 3, MaxToolCalls: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal := toolrun.ToolRun{
		ID: "tool-v11-preserved", SessionID: run.SessionID, WorkspaceID: "ws-v11",
		ToolName: toolrun.ShellTool, Command: "echo preserved", Status: toolrun.StatusProposed,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := st.SaveToolRun(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	removeSchemaV12ForTest(t, st, ctx)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	record, err := st.GetApprovalByProposal(ctx, proposal.ID)
	if err != nil || record.Status != approval.StatusPending || record.RunID != run.ID || record.GrantID != "" {
		t.Fatalf("v11 approval was not preserved: %#v err=%v", record, err)
	}
	grant, err := st.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: run.SessionID, ToolName: "shell", ActionClass: "shell", GrantedBy: "migration_test",
		IdempotencyKey: "v11-upgrade-grant",
	})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := st.AuthorizeApprovalWithSessionGrant(ctx, proposal.ID, grant.Grant.ID)
	if err != nil || decision.Approval.GrantID != grant.Grant.ID || decision.Approval.Status != approval.StatusApproved {
		t.Fatalf("upgraded approval could not use a session grant: %#v err=%v", decision, err)
	}
}
