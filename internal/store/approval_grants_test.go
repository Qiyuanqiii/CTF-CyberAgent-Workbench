package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/toolbudget"
	"cyberagent-workbench/internal/toolrun"
)

func TestSessionGrantLifecycleIsIdempotentRevocableAndRunScoped(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "grants.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "grant lifecycle", Profile: "code", WorkspaceID: "ws-grant",
		Budget: domain.Budget{MaxTurns: 3, MaxToolCalls: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := approval.CreateGrantRequest{
		SessionID: run.SessionID, ToolName: "shell", ActionClass: "shell", Reason: "trusted build commands",
		GrantedBy: "operator", IdempotencyKey: "grant-create-once",
	}
	created, err := st.CreateSessionGrant(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if created.Replayed || created.Grant.RunID != run.ID || created.Grant.WorkspaceID != "ws-grant" ||
		created.Grant.Status != approval.GrantActive {
		t.Fatalf("unexpected created grant: %#v", created)
	}
	replayed, err := st.CreateSessionGrant(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Grant.ID != created.Grant.ID {
		t.Fatalf("grant create did not replay: %#v", replayed)
	}
	conflict := request
	conflict.ToolName = "replace_file"
	conflict.ActionClass = "workspace_write"
	if _, err := st.CreateSessionGrant(ctx, conflict); err == nil {
		t.Fatal("expected conflicting grant idempotency key to fail")
	}
	matched, found, err := st.FindActiveSessionGrant(ctx, approval.GrantQuery{
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-grant", ToolName: "shell", ActionClass: "shell",
	})
	if err != nil || !found || matched.ID != created.Grant.ID {
		t.Fatalf("active grant was not found: %#v found=%t err=%v", matched, found, err)
	}
	if _, found, err := st.FindActiveSessionGrant(ctx, approval.GrantQuery{
		SessionID: run.SessionID, WorkspaceID: "ws-other", ToolName: "shell", ActionClass: "shell",
	}); err != nil || found {
		t.Fatalf("cross-workspace grant lookup escaped scope: found=%t err=%v", found, err)
	}
	revoke := approval.RevokeGrantRequest{
		GrantID: created.Grant.ID, Reason: "command phase complete", RevokedBy: "operator",
		IdempotencyKey: "grant-revoke-once",
	}
	revoked, err := st.RevokeSessionGrant(ctx, revoke)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Replayed || revoked.Grant.Status != approval.GrantRevoked || revoked.Grant.Version != 2 {
		t.Fatalf("unexpected revoked grant: %#v", revoked)
	}
	revokedReplay, err := st.RevokeSessionGrant(ctx, revoke)
	if err != nil || !revokedReplay.Replayed || revokedReplay.Grant.Version != 2 {
		t.Fatalf("grant revoke did not replay: %#v err=%v", revokedReplay, err)
	}
	if _, found, err := st.FindActiveSessionGrant(ctx, approval.GrantQuery{
		SessionID: run.SessionID, WorkspaceID: "ws-grant", ToolName: "shell", ActionClass: "shell",
	}); err != nil || found {
		t.Fatalf("revoked grant remained active: found=%t err=%v", found, err)
	}
	request.IdempotencyKey = "grant-create-again"
	recreated, err := st.CreateSessionGrant(ctx, request)
	if err != nil || recreated.Grant.ID == created.Grant.ID || recreated.Grant.Status != approval.GrantActive {
		t.Fatalf("scope could not be granted again after revocation: %#v err=%v", recreated, err)
	}
	var rawKeys int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM approval_grant_operations
		WHERE operation_key IN (?, ?)`, request.IdempotencyKey, revoke.IdempotencyKey).Scan(&rawKeys); err != nil {
		t.Fatal(err)
	}
	if rawKeys != 0 {
		t.Fatal("raw grant operation keys were persisted")
	}
	runEvents, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	createdEvents, revokedEvents := 0, 0
	for _, event := range runEvents {
		switch event.Type {
		case events.ApprovalGrantCreatedEvent:
			createdEvents++
		case events.ApprovalGrantRevokedEvent:
			revokedEvents++
		}
	}
	if createdEvents != 2 || revokedEvents != 1 {
		t.Fatalf("unexpected grant event counts: created=%d revoked=%d", createdEvents, revokedEvents)
	}
}

func TestSessionGrantAuthorizationCannotBypassScopeOrRevocation(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "grant-authorization.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "grant authorization", Profile: "code", WorkspaceID: "ws-auth",
		Budget: domain.Budget{MaxTurns: 3, MaxToolCalls: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	grantResult, err := st.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: run.SessionID, ToolName: "shell", ActionClass: "shell", GrantedBy: "operator",
		IdempotencyKey: "authorize-grant",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	proposal := toolrun.ToolRun{
		ID: "tool-grant-authorized", SessionID: run.SessionID, WorkspaceID: "ws-auth",
		ToolName: toolrun.ShellTool, Command: "echo safe", Status: toolrun.StatusProposed,
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := st.SaveToolRun(ctx, proposal); err != nil {
		t.Fatal(err)
	}
	decision, err := st.AuthorizeApprovalWithSessionGrant(ctx, proposal.ID, grantResult.Grant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Approval.Status != approval.StatusApproved || decision.Approval.GrantID != grantResult.Grant.ID ||
		decision.Approval.ReviewedBy != "session_grant" {
		t.Fatalf("grant authorization was not recorded: %#v", decision)
	}
	replay, err := st.AuthorizeApprovalWithSessionGrant(ctx, proposal.ID, grantResult.Grant.ID)
	if err != nil || !replay.Replayed {
		t.Fatalf("grant authorization did not replay: %#v err=%v", replay, err)
	}
	other := proposal
	other.ID = "tool-grant-revoked"
	other.CreatedAt = time.Now().UTC()
	other.UpdatedAt = other.CreatedAt
	if _, err := st.SaveToolRun(ctx, other); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RevokeSessionGrant(ctx, approval.RevokeGrantRequest{
		GrantID: grantResult.Grant.ID, RevokedBy: "operator", IdempotencyKey: "revoke-before-use",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AuthorizeApprovalWithSessionGrant(ctx, other.ID, grantResult.Grant.ID); err == nil {
		t.Fatal("revoked grant authorized a new proposal")
	}
	pending, err := st.GetApprovalByProposal(ctx, other.ID)
	if err != nil || pending.Status != approval.StatusPending || pending.GrantID != "" {
		t.Fatalf("failed authorization changed pending approval: %#v err=%v", pending, err)
	}
}

func TestToolCallBudgetIsAtomicAndBoundToRunScope(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "tool-budget.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "bounded tools", Profile: "code", WorkspaceID: "ws-budget",
		Budget: domain.Budget{MaxTurns: 3, MaxToolCalls: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := toolbudget.ChargeRequest{
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-budget",
		ToolName: "read_file", ActionClass: "workspace_read",
	}
	for expected := int64(1); expected <= 2; expected++ {
		usage, err := st.ChargeToolCall(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		if usage.Consumed != expected || usage.Remaining != 2-expected || !usage.Tracked {
			t.Fatalf("unexpected tool usage after charge %d: %#v", expected, usage)
		}
	}
	if _, err := st.ChargeToolCall(ctx, request); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("expected resource exhaustion, got %v (%s)", err, apperror.CodeOf(err))
	}
	if _, err := st.ChargeToolCall(ctx, request); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("expected repeated resource exhaustion, got %v (%s)", err, apperror.CodeOf(err))
	}
	usage, err := st.GetToolCallUsage(ctx, run.ID)
	if err != nil || usage.Consumed != 2 || usage.Limit != 2 || usage.Remaining != 0 || usage.ExhaustedAt == nil {
		t.Fatalf("unexpected persisted tool usage: %#v err=%v", usage, err)
	}
	mismatch := request
	mismatch.WorkspaceID = "ws-other"
	if _, err := st.ChargeToolCall(ctx, mismatch); err == nil {
		t.Fatal("cross-workspace tool call consumed a Run budget")
	}
	legacy := session.New("ws-budget", "legacy", "code")
	if err := st.SaveSession(ctx, legacy); err != nil {
		t.Fatal(err)
	}
	untracked, err := st.ChargeToolCall(ctx, toolbudget.ChargeRequest{
		SessionID: legacy.ID, WorkspaceID: legacy.WorkspaceID, ToolName: "read_file", ActionClass: "workspace_read",
	})
	if err != nil || untracked.Tracked || untracked.Remaining != -1 {
		t.Fatalf("legacy session should be untracked: %#v err=%v", untracked, err)
	}
	runEvents, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	charges, exhaustedEvents := 0, 0
	for _, event := range runEvents {
		switch event.Type {
		case events.ToolBudgetChargedEvent:
			charges++
		case events.ToolBudgetExhaustedEvent:
			exhaustedEvents++
		}
	}
	if charges != 2 || exhaustedEvents != 1 {
		t.Fatalf("unexpected durable tool budget events: charged=%d exhausted=%d", charges, exhaustedEvents)
	}
}

func TestConcurrentToolCallBudgetNeverOverspends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool-budget-concurrent.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "concurrent budget", Profile: "code", WorkspaceID: "ws-concurrent",
		Budget: domain.Budget{MaxTurns: 3, MaxToolCalls: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := toolbudget.ChargeRequest{
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-concurrent",
		ToolName: "read_file", ActionClass: "workspace_read",
	}
	results := make([]error, 16)
	stores := []*SQLiteStore{st, other}
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, results[index] = stores[index%len(stores)].ChargeToolCall(ctx, request)
		}(index)
	}
	wait.Wait()
	successes, exhausted := 0, 0
	for _, result := range results {
		switch {
		case result == nil:
			successes++
		case apperror.CodeOf(result) == apperror.CodeResourceExhausted:
			exhausted++
		default:
			t.Fatalf("unexpected concurrent charge error: %v", result)
		}
	}
	if successes != 8 || exhausted != 8 {
		t.Fatalf("budget was not enforced atomically: success=%d exhausted=%d", successes, exhausted)
	}
	usage, err := st.GetToolCallUsage(ctx, run.ID)
	if err != nil || usage.Consumed != 8 {
		t.Fatalf("concurrent budget counter is wrong: %#v err=%v", usage, err)
	}
}

func TestTerminalRunCannotCreateOrUseSessionGrant(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "terminal-grant.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "terminal grants", Profile: "code", Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Cancel(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: run.SessionID, ToolName: "shell", ActionClass: "shell", GrantedBy: "operator",
		IdempotencyKey: "terminal-grant",
	})
	if err == nil {
		t.Fatal("terminal Run accepted a new session grant")
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("unexpected terminal grant error: %v", err)
	}
}

func TestArchivedSessionCannotCreateOrConsumeGrant(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "archived-session-grant.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "archived session grants", Profile: "code", WorkspaceID: "ws-archived",
		Budget: domain.Budget{MaxTurns: 2, MaxToolCalls: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: run.SessionID, ToolName: "shell", ActionClass: "shell", GrantedBy: "operator",
		IdempotencyKey: "archived-session-grant",
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	sess.Status = session.StatusArchived
	sess.UpdatedAt = time.Now().UTC()
	if err := st.SaveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if _, found, err := st.FindActiveSessionGrant(ctx, approval.GrantQuery{
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-archived", ToolName: "shell", ActionClass: "shell",
	}); err != nil || found {
		t.Fatalf("archived session exposed an active grant: id=%s found=%t err=%v", created.Grant.ID, found, err)
	}
	_, err = st.CreateSessionGrant(ctx, approval.CreateGrantRequest{
		SessionID: run.SessionID, ToolName: "replace_file", ActionClass: "workspace_write", GrantedBy: "operator",
		IdempotencyKey: "archived-session-new-grant",
	})
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("archived session accepted a new grant: %v", err)
	}
}
