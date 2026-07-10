package toolgateway_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
)

func TestGatewayApprovalRecoversAcrossStoreRestart(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "approval.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveWorkspace(ctx, store.WorkspaceRecord{ID: "ws-approval", Name: "approval", RootPath: root}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "verify approval recovery", Profile: "code", WorkspaceID: "ws-approval",
		Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := toolgateway.New(st, policy.NewDefaultChecker())
	outcome, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "echo safe"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-approval", RequestedBy: "integration_test",
	})
	if err != nil || outcome.Proposal == nil {
		t.Fatalf("shell proposal failed: %#v err=%v", outcome, err)
	}
	record, err := st.GetApprovalByProposal(ctx, outcome.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.RunID != run.ID || record.SessionID != run.SessionID || record.Status != approval.StatusPending {
		t.Fatalf("approval was not bound to the run: %#v", record)
	}
	reviewKey := approval.ReviewIdempotencyKey("shell", outcome.Proposal.ID, approval.ActionApprove)
	if _, err := st.DecideApproval(ctx, approval.DecisionRequest{
		ProposalID: outcome.Proposal.ID, IdempotencyKey: reviewKey,
		Action: approval.ActionApprove, ReviewedBy: "integration_operator",
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	gateway = toolgateway.New(st, policy.NewDefaultChecker())
	request := toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, Tool: toolgateway.ShellTool, ProposalID: outcome.Proposal.ID,
		IdempotencyKey: reviewKey, ReviewedBy: "integration_operator",
	}
	recovered, err := gateway.Review(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Proposal == nil || recovered.Proposal.Status != toolgateway.StatusCompleted || recovered.Result == nil || recovered.Result.Status != toolgateway.StatusCompleted {
		t.Fatalf("approval did not converge after restart: %#v", recovered)
	}
	if _, err := gateway.Review(ctx, request); err != nil {
		t.Fatalf("repeated review was not idempotent: %v", err)
	}
	runEvents, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	requested, decided := 0, 0
	for _, event := range runEvents {
		switch event.Type {
		case events.ApprovalRequestedEvent:
			requested++
		case events.ApprovalDecidedEvent:
			decided++
		}
	}
	if requested != 1 || decided != 1 {
		t.Fatalf("approval events were duplicated: requested=%d decided=%d", requested, decided)
	}
}

func TestGatewayFileApprovalAppliesOnceAfterDurableDecision(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "approval.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.SaveWorkspace(ctx, store.WorkspaceRecord{ID: "ws-file", Name: "file", RootPath: root}); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "apply a reviewed file", Profile: "code", WorkspaceID: "ws-file", Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	gateway := toolgateway.New(st, policy.NewDefaultChecker()).WithWorkspaceRootResolver(func(context.Context, string) (string, error) {
		return root, nil
	})
	outcome, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ReplaceFileTool, Arguments: map[string]string{"path": "reviewed.txt", "content": "approved\n"},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: "ws-file", RequestedBy: "integration_test",
	})
	if err != nil || outcome.Proposal == nil {
		t.Fatalf("file proposal failed: %#v err=%v", outcome, err)
	}
	request := toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, Tool: toolgateway.ReplaceFileTool, ProposalID: outcome.Proposal.ID,
		WorkspaceRoot: root, IdempotencyKey: "review:file:apply-once", ReviewedBy: "integration_operator",
	}
	if _, err := gateway.Review(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.Review(ctx, request); err != nil {
		t.Fatalf("repeated file approval failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "reviewed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "approved\n" {
		t.Fatalf("unexpected applied file: %q", data)
	}
	record, err := st.GetApprovalByProposal(ctx, outcome.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != approval.StatusApproved || record.Version != 2 {
		t.Fatalf("repeated file approval changed the decision: %#v", record)
	}
}
