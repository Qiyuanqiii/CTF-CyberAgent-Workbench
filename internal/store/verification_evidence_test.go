package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/verification"
)

type retryingCodeHandoffStore struct {
	*SQLiteStore
	calls       int
	neverStable bool
}

func (s *retryingCodeHandoffStore) LatestRunEventSequence(ctx context.Context,
	runID string,
) (int64, error) {
	value, err := s.SQLiteStore.LatestRunEventSequence(ctx, runID)
	if err != nil {
		return 0, err
	}
	s.calls++
	if s.neverStable {
		return value + int64(s.calls), nil
	}
	if s.calls == 2 {
		return value + 1, nil
	}
	return value, nil
}

func TestOperatorVerificationEvidenceIsImmutableRedactedAndReplayable(t *testing.T) {
	ctx := t.Context()
	state, err := Open(filepath.Join(t.TempDir(), "verification.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	workspace := WorkspaceRecord{ID: "workspace-verification", Name: "verification",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "record operator verification", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewVerificationEvidenceService(state)
	key := "operator-verification-operation-0001"
	secret := "sk-123456789012345678901234567890"
	request := application.RecordVerificationEvidenceRequest{
		Version: verification.EvidenceProtocolVersion, RunID: run.ID,
		Outcome: string(verification.OutcomePass), Title: "Focused tests",
		Summary:      "API_KEY=" + secret + "\ngo test ./internal/... passed",
		OperationKey: key, RecordedBy: "operator",
	}
	result, err := service.Record(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || !result.Evidence.Redacted ||
		strings.Contains(result.Evidence.Summary, secret) ||
		result.Evidence.SummarySHA256 != verification.SummaryDigest(result.Evidence.Summary) {
		t.Fatalf("verification evidence was not safely stored: %#v", result)
	}
	replayed, err := service.Record(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Evidence.ID != result.Evidence.ID {
		t.Fatalf("verification replay did not converge: %#v err=%v", replayed, err)
	}
	changed := request
	changed.Outcome = string(verification.OutcomeFail)
	if _, err := service.Record(ctx, changed); err == nil {
		t.Fatal("same verification operation key accepted a different outcome")
	}

	inventory, err := service.Inventory(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.ProtocolVersion != verification.InventoryProtocolVersion ||
		inventory.RunID != run.ID || inventory.SessionID != run.SessionID ||
		inventory.WorkspaceID != workspace.ID || inventory.PassCount != 1 ||
		inventory.FailCount != 0 || inventory.UnknownCount != 0 ||
		len(inventory.Items) != 1 || inventory.Truncated {
		t.Fatalf("unexpected verification inventory: %#v", inventory)
	}
	handoff, err := application.NewCodeHandoffService(state).Build(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if handoff.ProtocolVersion != application.CodeHandoffProtocolVersion ||
		handoff.RunID != run.ID || handoff.SessionID != run.SessionID ||
		handoff.WorkspaceID != workspace.ID || handoff.Surface != domain.ExecutionSurfaceCode ||
		handoff.Verification.PassCount != 1 ||
		len(handoff.Verification.References) != 1 || !handoff.Regenerable ||
		!handoff.DurableSources || handoff.PrivateBodiesIncluded ||
		handoff.CompositeMutation || handoff.ResumeAuthorized || handoff.ExecutionStarted {
		t.Fatalf("unexpected Code handoff: %#v", handoff)
	}
	retrying := &retryingCodeHandoffStore{SQLiteStore: state}
	if _, err := application.NewCodeHandoffService(retrying).Build(ctx, run.ID); err != nil ||
		retrying.calls != 4 {
		t.Fatalf("Code handoff did not recover a changing event tail: calls=%d err=%v",
			retrying.calls, err)
	}
	unstable := &retryingCodeHandoffStore{SQLiteStore: state, neverStable: true}
	if _, err := application.NewCodeHandoffService(unstable).Build(ctx, run.ID); apperror.CodeOf(err) != apperror.CodeConflict || unstable.calls != 8 {
		t.Fatalf("Code handoff accepted an unstable event tail: calls=%d err=%v",
			unstable.calls, err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE operator_verification_evidence
		SET outcome = 'fail' WHERE id = ?`, result.Evidence.ID); err == nil {
		t.Fatal("verification evidence update was accepted")
	}
	if _, err := state.db.ExecContext(ctx, `DELETE FROM operator_verification_evidence
		WHERE id = ?`, result.Evidence.ID); err == nil {
		t.Fatal("verification evidence delete was accepted")
	}
	timeline, err := state.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range timeline {
		if event.Type != events.VerificationEvidenceRecordedEvent {
			continue
		}
		count++
		if strings.Contains(event.PayloadJSON, "Focused tests") ||
			strings.Contains(event.PayloadJSON, "go test") ||
			!strings.Contains(event.PayloadJSON, `"command_executed":false`) ||
			!strings.Contains(event.PayloadJSON, `"authority_granted":false`) {
			t.Fatalf("verification event leaked content or authority: %s", event.PayloadJSON)
		}
	}
	if count != 1 {
		t.Fatalf("verification event count = %d, want 1", count)
	}
}

func TestRecordVerificationEvidenceRechecksActiveSessionInsideTransaction(t *testing.T) {
	ctx := t.Context()
	state, err := Open(filepath.Join(t.TempDir(), "verification-session.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	workspace := WorkspaceRecord{ID: "workspace-verification-session", Name: "session",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "reject archived verification", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	linkedSession, err := state.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	linkedSession.Status = session.StatusArchived
	linkedSession.UpdatedAt = time.Now().UTC()
	if err := state.SaveSession(ctx, linkedSession); err != nil {
		t.Fatal(err)
	}
	summary := "focused verification passed"
	title := "Focused verification"
	recordedBy := "operator"
	evidence := verification.Evidence{
		ID: "verification-archived-session", ProtocolVersion: verification.EvidenceProtocolVersion,
		OperationKeyDigest: runmutation.VerificationEvidenceOperationDigest(run.ID,
			"verification-archived-session-operation"),
		RequestFingerprint: runmutation.VerificationEvidenceRequestFingerprint(run.ID,
			run.SessionID, workspace.ID, string(verification.OutcomePass), title, summary,
			recordedBy),
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: workspace.ID,
		Outcome: verification.OutcomePass, Title: title, Summary: summary,
		SummarySHA256: verification.SummaryDigest(summary), RecordedBy: recordedBy,
		CreatedAt: time.Now().UTC(),
	}
	if _, _, err := state.RecordVerificationEvidence(ctx, evidence); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("archived Session verification evidence was accepted: %v", err)
	}
	items, err := state.ListVerificationEvidence(ctx, run.ID, 1)
	if err != nil || len(items) != 0 {
		t.Fatalf("archived Session verification left evidence: %#v err=%v", items, err)
	}
}

func TestSchemaV78UpgradePreservesRunWithoutFabricatingVerificationEvidence(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "v77.db")
	state, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	workspace := WorkspaceRecord{ID: "workspace-v78-upgrade", Name: "v78-upgrade",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		_ = state.Close()
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "preserve Run across v78", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		_ = state.Close()
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV78ForTestStatements() {
		if _, err := state.db.ExecContext(ctx, statement); err != nil {
			_ = state.Close()
			t.Fatalf("remove schema v78 with %q: %v", statement, err)
		}
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}

	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema version = %d, err=%v", version, err)
	}
	if preserved, err := upgraded.GetRun(ctx, run.ID); err != nil || preserved.ID != run.ID {
		t.Fatalf("preserved Run = %#v, err=%v", preserved, err)
	}
	registered, err := upgraded.GetWorkspaceByID(ctx, workspace.ID)
	if err != nil || registered.ID != workspace.ID {
		t.Fatalf("preserved Workspace = %#v, err=%v", registered, err)
	}
	items, err := upgraded.ListVerificationEvidence(ctx, run.ID, 1)
	if err != nil || len(items) != 0 {
		t.Fatalf("v78 migration fabricated verification evidence: %#v, err=%v", items, err)
	}
}
