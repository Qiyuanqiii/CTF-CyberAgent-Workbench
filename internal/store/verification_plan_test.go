package store

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestOperatorVerificationPlanIsImmutableRedactedAndNeverBecomesAResult(t *testing.T) {
	ctx := t.Context()
	state, err := Open(filepath.Join(t.TempDir(), "verification-plan.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	workspace := WorkspaceRecord{ID: "workspace-verification-plan", Name: "verification-plan",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "plan operator verification", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewVerificationPlanService(state)
	secret := "sk-123456789012345678901234567890"
	request := application.RecordVerificationPlanRequest{
		Version: verification.PlanProtocolVersion, RunID: run.ID,
		Title: "Release verification", Summary: "Operator-authored checks only",
		Items: []application.VerificationPlanItemRequest{
			{Title: "Focused tests", ExpectedObservation: "Observe passing focused tests"},
			{Title: "Credential boundary", ExpectedObservation: "API_KEY=" + secret + " remains absent"},
		},
		OperationKey: "operator-verification-plan-operation-0001", AuthoredBy: "operator",
	}
	result, err := service.Record(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Replayed || !result.Plan.Redacted || len(result.Plan.Items) != 2 ||
		strings.Contains(result.Plan.Items[1].ExpectedObservation, secret) ||
		result.Plan.PlanSHA256 != verification.PlanDigest(result.Plan.Title,
			result.Plan.Summary, result.Plan.Items) {
		t.Fatalf("verification plan was not safely stored: %#v", result)
	}
	replayed, err := service.Record(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Plan.ID != result.Plan.ID {
		t.Fatalf("verification plan replay did not converge: %#v err=%v", replayed, err)
	}
	changed := request
	changed.Items = append([]application.VerificationPlanItemRequest{}, request.Items...)
	changed.Items[0].ExpectedObservation = "Different observation"
	if _, err := service.Record(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("same operation key accepted a changed plan: %v", err)
	}
	inventory, err := service.Inventory(ctx, run.ID)
	if err != nil || inventory.ProtocolVersion != verification.PlanInventoryProtocolVersion ||
		inventory.RunID != run.ID || inventory.SessionID != run.SessionID ||
		inventory.WorkspaceID != workspace.ID || inventory.Truncated ||
		len(inventory.Items) != 1 {
		t.Fatalf("unexpected verification plan inventory: %#v err=%v", inventory, err)
	}
	evidence, err := state.ListVerificationEvidence(ctx, run.ID, 1)
	if err != nil || len(evidence) != 0 {
		t.Fatalf("verification plan fabricated a verification result: %#v err=%v", evidence, err)
	}
	handoff, err := application.NewCodeHandoffService(state).Build(ctx, run.ID)
	if err != nil || handoff.SourceEventSequence <= 0 ||
		handoff.VerificationPlans.ReturnedCount != 1 ||
		len(handoff.VerificationPlans.References) != 1 ||
		handoff.VerificationPlans.References[0].ID != result.Plan.ID {
		t.Fatalf("verification plan was not safely referenced by handoff: %#v err=%v", handoff, err)
	}
	latestSequence, err := state.LatestRunEventSequence(ctx, run.ID)
	if err != nil || handoff.SourceEventSequence != latestSequence {
		t.Fatalf("handoff high-water=%d latest=%d err=%v",
			handoff.SourceEventSequence, latestSequence, err)
	}
	for _, format := range []string{application.CodeHandoffExportFormatMarkdown,
		application.CodeHandoffExportFormatJSON} {
		exported, err := application.NewCodeHandoffExportService(state).Build(ctx, run.ID, format)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256([]byte(exported.Content))
		if exported.ProtocolVersion != application.CodeHandoffExportProtocolVersion ||
			exported.RunID != run.ID || exported.SourceEventSequence != latestSequence ||
			exported.ContentBytes != len([]byte(exported.Content)) ||
			exported.ContentSHA256 != hex.EncodeToString(digest[:]) ||
			exported.ContentBytes <= 0 ||
			exported.ContentBytes > application.MaxCodeHandoffExportBytes ||
			!exported.ReadOnly || !exported.DownloadOnly || exported.PrivateBodies ||
			exported.ResumeAuthorized || exported.MutationSupported ||
			exported.ReportAcceptance || exported.ExecutionStarted ||
			strings.Contains(exported.Content, secret) ||
			strings.Contains(exported.Content, "Operator-authored checks only") ||
			strings.Contains(exported.Content, "Focused tests") {
			t.Fatalf("unsafe %s handoff export: %#v", format, exported)
		}
		if format == application.CodeHandoffExportFormatMarkdown &&
			!strings.Contains(exported.Content, "Source event high-water") {
			t.Fatalf("Markdown export omitted high-water: %s", exported.Content)
		}
		if format == application.CodeHandoffExportFormatJSON &&
			!strings.Contains(exported.Content, `"source_event_sequence"`) {
			t.Fatalf("JSON export omitted high-water: %s", exported.Content)
		}
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE operator_verification_plans
		SET title = 'changed' WHERE id = ?`, result.Plan.ID); err == nil {
		t.Fatal("verification plan update was accepted")
	}
	if _, err := state.db.ExecContext(ctx, `DELETE FROM operator_verification_plan_items
		WHERE plan_id = ? AND ordinal = 1`, result.Plan.ID); err == nil {
		t.Fatal("verification plan item delete was accepted")
	}
	if _, err := state.db.ExecContext(ctx, `DELETE FROM operator_verification_plans
		WHERE id = ?`, result.Plan.ID); err == nil {
		t.Fatal("verification plan delete was accepted")
	}
	timeline, err := state.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range timeline {
		if event.Type != events.VerificationPlanRecordedEvent {
			continue
		}
		count++
		if strings.Contains(event.PayloadJSON, "Focused tests") ||
			strings.Contains(event.PayloadJSON, "Observe passing") ||
			!strings.Contains(event.PayloadJSON, `"guidance_only":true`) ||
			!strings.Contains(event.PayloadJSON, `"result_inferred":false`) ||
			!strings.Contains(event.PayloadJSON, `"command_executed":false`) ||
			!strings.Contains(event.PayloadJSON, `"authority_granted":false`) {
			t.Fatalf("verification plan event leaked content or authority: %s", event.PayloadJSON)
		}
	}
	if count != 1 {
		t.Fatalf("verification plan event count = %d, want 1", count)
	}
}

func TestRecordVerificationPlanRechecksActiveCodeSessionInsideTransaction(t *testing.T) {
	ctx := t.Context()
	state, err := Open(filepath.Join(t.TempDir(), "verification-plan-session.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	workspace := WorkspaceRecord{ID: "workspace-verification-plan-session", Name: "session",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "reject archived verification plan", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := state.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	linked.Status = session.StatusArchived
	linked.UpdatedAt = time.Now().UTC()
	if err := state.SaveSession(ctx, linked); err != nil {
		t.Fatal(err)
	}
	items := []verification.PlanItem{{Ordinal: 1, Title: "Focused tests",
		ExpectedObservation: "Observe a deterministic pass"}}
	items[0].ItemSHA256 = verification.PlanItemDigest(items[0].Title,
		items[0].ExpectedObservation)
	planSHA := verification.PlanDigest("Release checks", "Operator guidance", items)
	plan := verification.Plan{
		ID: "verification-plan-archived", ProtocolVersion: verification.PlanProtocolVersion,
		OperationKeyDigest: runmutation.VerificationPlanOperationDigest(run.ID,
			"verification-plan-archived-operation"),
		RequestFingerprint: runmutation.VerificationPlanRequestFingerprint(run.ID,
			run.SessionID, workspace.ID, planSHA, "operator"),
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: workspace.ID,
		Title: "Release checks", Summary: "Operator guidance", PlanSHA256: planSHA,
		AuthoredBy: "operator", CreatedAt: time.Now().UTC(), Items: items,
	}
	if _, _, err := state.RecordVerificationPlan(ctx, plan); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("archived Session verification plan was accepted: %v", err)
	}
	plans, err := state.ListVerificationPlans(ctx, run.ID, 1)
	if err != nil || len(plans) != 0 {
		t.Fatalf("archived Session left a verification plan: %#v err=%v", plans, err)
	}
}

func TestSchemaV80UpgradePreservesRunWithoutFabricatingVerificationPlan(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "v79.db")
	state, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	workspace := WorkspaceRecord{ID: "workspace-v80-upgrade", Name: "v80-upgrade",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		_ = state.Close()
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "preserve Run across v80", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		_ = state.Close()
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV80ForTestStatements() {
		if _, err := state.db.ExecContext(ctx, statement); err != nil {
			_ = state.Close()
			t.Fatalf("remove schema v80 with %q: %v", statement, err)
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
	plans, err := upgraded.ListVerificationPlans(ctx, run.ID, 1)
	if err != nil || len(plans) != 0 {
		t.Fatalf("v80 migration fabricated verification plans: %#v err=%v", plans, err)
	}
}
