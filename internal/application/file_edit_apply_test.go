package application_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/tools"
)

type fileEditPolicyFlipChecker struct{ calls int }

func (c *fileEditPolicyFlipChecker) CheckText(string, string) policy.Decision {
	return policy.Decision{Allowed: true, Reason: "fixture text allowed"}
}

func (c *fileEditPolicyFlipChecker) CheckToolCall(tools.Call) policy.Decision {
	c.calls++
	if c.calls == 1 {
		return policy.Decision{Allowed: true, Reason: "prepare allowed"}
	}
	return policy.Decision{Allowed: false, Risk: "high", Reason: "write authority revoked"}
}

func TestFileEditApplyRequiresReviewWritesOnceAndReplaysAcrossRestart(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	databasePath := filepath.Join(home, "file-edit-apply.db")
	workspaceRoot := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	state, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	workspace := store.WorkspaceRecord{ID: "workspace-file-apply",
		Name: "file-apply", RootPath: workspaceRoot}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, created, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "apply reviewed diff", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.NewRunService(state).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	edit, err := fileedit.NewManager(state).Propose(ctx, fileedit.Proposal{
		SessionID: run.SessionID, WorkspaceID: workspace.ID,
		WorkspaceRoot: workspaceRoot, Path: "README.md", ProposedText: "reviewed\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewFileEditReviewService(state).Review(ctx,
		application.ReviewFileEditRequest{
			Version: application.FileEditReviewProtocolVersion, RunID: run.ID,
			EditID: edit.ID, Action: application.FileEditApproveIntent,
		}); err != nil {
		t.Fatal(err)
	}
	request := application.ApplyFileEditRequest{
		Version: fileedit.FileEditApplyProtocolVersion, RunID: run.ID, EditID: edit.ID,
		OperationKey: "file-edit-apply-operation-0001", AppliedBy: "test_operator",
	}
	result, err := application.NewFileEditApplyService(state,
		policy.NewDefaultChecker()).Apply(ctx, request)
	if err != nil || result.Result.Status != fileedit.ApplyCompleted ||
		result.Edit.Status != fileedit.StatusApplied || !result.FileWritten {
		t.Fatalf("apply result=%#v err=%v", result, err)
	}
	written, err := os.ReadFile(filepath.Join(workspaceRoot, "README.md"))
	if err != nil || string(written) != "reviewed\n" {
		t.Fatalf("written=%q err=%v", written, err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	replay, err := application.NewFileEditApplyService(state,
		policy.NewDefaultChecker()).Apply(ctx, request)
	if err != nil || !replay.Replayed || replay.FileWritten ||
		replay.Result.EventSequence != result.Result.EventSequence {
		t.Fatalf("restart replay=%#v err=%v", replay, err)
	}
}

func TestFileEditApplyRechecksPolicyAndRunStateAfterDurablePreparation(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	workspaceRoot := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	state, err := store.Open(filepath.Join(home, "file-edit-recovery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	workspace := store.WorkspaceRecord{ID: "workspace-file-recovery",
		Name: "file-recovery", RootPath: workspaceRoot}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, created, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "recover reviewed diff", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	runService := application.NewRunService(state)
	run, err := runService.Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	edit, err := fileedit.NewManager(state).Propose(ctx, fileedit.Proposal{
		SessionID: run.SessionID, WorkspaceID: workspace.ID,
		WorkspaceRoot: workspaceRoot, Path: "RECOVERY.md", ProposedText: "bounded\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewFileEditReviewService(state).Review(ctx,
		application.ReviewFileEditRequest{
			Version: application.FileEditReviewProtocolVersion, RunID: run.ID,
			EditID: edit.ID, Action: application.FileEditApproveIntent,
		}); err != nil {
		t.Fatal(err)
	}
	request := application.ApplyFileEditRequest{
		Version: fileedit.FileEditApplyProtocolVersion, RunID: run.ID, EditID: edit.ID,
		OperationKey: "file-edit-policy-recovery-0001", AppliedBy: "test_operator",
	}
	checker := &fileEditPolicyFlipChecker{}
	_, err = application.NewFileEditApplyService(state, checker).Apply(ctx, request)
	if apperror.CodeOf(err) != apperror.CodePolicyDenied || checker.calls != 2 {
		t.Fatalf("write-boundary Policy recheck calls=%d code=%s err=%v",
			checker.calls, apperror.CodeOf(err), err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "RECOVERY.md")); !os.IsNotExist(err) {
		t.Fatalf("denied recovery wrote workspace file: %v", err)
	}
	conflicting := request
	conflicting.OperationKey = "file-edit-policy-recovery-0002"
	_, err = application.NewFileEditApplyService(state,
		policy.NewDefaultChecker()).Apply(ctx, conflicting)
	if apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("second apply operation code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := runService.Pause(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	_, err = application.NewFileEditApplyService(state,
		policy.NewDefaultChecker()).Apply(ctx, request)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("paused recovery code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := os.Stat(filepath.Join(workspaceRoot, "RECOVERY.md")); !os.IsNotExist(err) {
		t.Fatalf("paused recovery wrote workspace file: %v", err)
	}
	if _, err := runService.Resume(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	result, err := application.NewFileEditApplyService(state,
		policy.NewDefaultChecker()).Apply(ctx, request)
	if err != nil || !result.Replayed || !result.FileWritten ||
		result.Result.Status != fileedit.ApplyCompleted {
		t.Fatalf("authorized recovery result=%#v err=%v", result, err)
	}
}
