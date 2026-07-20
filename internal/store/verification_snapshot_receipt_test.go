package store

import (
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/verification"
)

func removeSchemaV83ForTestStatements() []string {
	return append(removeSchemaV84ForTestStatements(), []string{
		`DROP TRIGGER trg_operator_verification_snapshot_receipt_delete_immutable`,
		`DROP TRIGGER trg_operator_verification_snapshot_receipt_update_immutable`,
		`DROP TRIGGER trg_operator_verification_snapshot_receipt_insert`,
		`DROP INDEX idx_operator_verification_snapshot_receipts_plan_item`,
		`DROP INDEX idx_operator_verification_snapshot_receipts_run_event`,
		`DROP TABLE operator_verification_snapshot_receipts`,
		`DELETE FROM schema_migrations WHERE version = 83`,
	}...)
}

func TestVerificationSnapshotReceiptIsImmutableIdempotentAndRejectsStaleSnapshot(t *testing.T) {
	ctx := t.Context()
	state, err := Open(filepath.Join(t.TempDir(), "verification-snapshot-receipt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	workspace := WorkspaceRecord{ID: "workspace-snapshot-receipt", Name: "snapshot-receipt",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "retain an exact metadata snapshot", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := application.NewVerificationPlanService(state).Record(ctx,
		application.RecordVerificationPlanRequest{Version: verification.PlanProtocolVersion,
			RunID: run.ID, Title: "Release checks", Summary: "Operator guidance",
			Items: []application.VerificationPlanItemRequest{{Title: "Focused suite",
				ExpectedObservation: "Observe an explicit result"}},
			OperationKey: "snapshot-receipt-plan-operation-0001", AuthoredBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	evidenceService := application.NewVerificationEvidenceService(state)
	evidence, err := evidenceService.Record(ctx, application.RecordVerificationEvidenceRequest{
		Version: verification.EvidenceProtocolVersion, RunID: run.ID,
		Outcome: string(verification.OutcomePass), Title: "Focused suite",
		Summary: "Observed a passing run", OperationKey: "snapshot-receipt-evidence-0001",
		RecordedBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewVerificationAssociationService(state).Record(ctx,
		application.RecordVerificationAssociationRequest{
			Version: verification.PlanEvidenceAssociationProtocolVersion, RunID: run.ID,
			PlanID: plan.Plan.ID, PlanItemOrdinal: 1, EvidenceID: evidence.Evidence.ID,
			OperationKey: "snapshot-receipt-association-0001", AssociatedBy: "operator",
		}); err != nil {
		t.Fatal(err)
	}
	exported, err := application.NewVerificationSnapshotExportService(state).Build(ctx, run.ID,
		plan.Plan.ID, 1, application.VerificationSnapshotExportFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewVerificationSnapshotReceiptService(state)
	request := application.RecordVerificationSnapshotReceiptRequest{
		Version: verification.SnapshotReceiptProtocolVersion, RunID: run.ID,
		PlanID: plan.Plan.ID, PlanItemOrdinal: 1, Format: exported.Format,
		SnapshotHighWaterEventSequence: exported.SnapshotHighWaterEventSequence,
		ContentSHA256:                  exported.ContentSHA256, ConfirmMetadataSnapshot: true,
		OperationKey: "snapshot-receipt-operation-0001", RecordedBy: "operator",
	}
	recorded, err := service.Record(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if recorded.Replayed || recorded.Receipt.EventSequence <=
		recorded.Receipt.SnapshotHighWaterEventSequence ||
		recorded.Receipt.ContentBytes != exported.ContentBytes {
		t.Fatalf("snapshot receipt lost causality or content metadata: %#v", recorded)
	}
	replayed, err := service.Record(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Receipt.ID != recorded.Receipt.ID {
		t.Fatalf("snapshot receipt replay diverged: %#v err=%v", replayed, err)
	}
	changed := request
	changed.Format = application.VerificationSnapshotExportFormatMarkdown
	if _, err := service.Record(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("same key accepted changed snapshot intent: %v", err)
	}
	inventory, err := service.Inventory(ctx, run.ID)
	if err != nil || len(inventory.Items) != 1 || !inventory.MetadataOnly ||
		!inventory.ReadOnly || inventory.SnapshotAccepted || inventory.ResultAccepted ||
		inventory.ResultInferred || inventory.RecordRewritten || inventory.Approval ||
		inventory.AuthorityGranted || inventory.ExecutionStarted {
		t.Fatalf("snapshot receipt inventory widened authority: %#v err=%v", inventory, err)
	}
	if _, err := state.db.ExecContext(ctx,
		`UPDATE operator_verification_snapshot_receipts SET format = 'markdown' WHERE id = ?`,
		recorded.Receipt.ID); err == nil {
		t.Fatal("snapshot receipt update was accepted")
	}
	if _, err := state.db.ExecContext(ctx,
		`DELETE FROM operator_verification_snapshot_receipts WHERE id = ?`,
		recorded.Receipt.ID); err == nil {
		t.Fatal("snapshot receipt delete was accepted")
	}
	timeline, err := state.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range timeline {
		if event.Type != events.VerificationSnapshotReceiptRecordedEvent {
			continue
		}
		found = true
		if strings.Contains(event.PayloadJSON, `"recorded_by"`) ||
			strings.Contains(event.PayloadJSON, `"operator"`) ||
			!strings.Contains(event.PayloadJSON, `"snapshot_accepted":false`) ||
			!strings.Contains(event.PayloadJSON, `"result_accepted":false`) ||
			!strings.Contains(event.PayloadJSON, `"private_bodies_included":false`) ||
			!strings.Contains(event.PayloadJSON, `"execution_started":false`) {
			t.Fatalf("snapshot receipt event exposed identity or authority: %s", event.PayloadJSON)
		}
	}
	if !found {
		t.Fatal("snapshot receipt event was not recorded")
	}
	laterEvidence, err := evidenceService.Record(ctx,
		application.RecordVerificationEvidenceRequest{Version: verification.EvidenceProtocolVersion,
			RunID: run.ID, Outcome: string(verification.OutcomeFail), Title: "Later observation",
			Summary:      "A later result changed the snapshot",
			OperationKey: "snapshot-receipt-evidence-0002", RecordedBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewVerificationAssociationService(state).Record(ctx,
		application.RecordVerificationAssociationRequest{
			Version: verification.PlanEvidenceAssociationProtocolVersion, RunID: run.ID,
			PlanID: plan.Plan.ID, PlanItemOrdinal: 1, EvidenceID: laterEvidence.Evidence.ID,
			OperationKey: "snapshot-receipt-association-0002", AssociatedBy: "operator",
		}); err != nil {
		t.Fatal(err)
	}
	replayedAfterAppend, err := service.Record(ctx, request)
	if err != nil || !replayedAfterAppend.Replayed ||
		replayedAfterAppend.Receipt.ID != recorded.Receipt.ID {
		t.Fatalf("committed receipt did not replay after later association: %#v err=%v",
			replayedAfterAppend, err)
	}
	request.OperationKey = "snapshot-receipt-operation-0002"
	if _, err := service.Record(ctx, request); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("stale snapshot receipt returned code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestSchemaV83UpgradeFabricatesNoSnapshotReceipt(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "v82-snapshot-receipt.db")
	state, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	workspace := WorkspaceRecord{ID: "workspace-v83-upgrade", Name: "v83-upgrade",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "preserve v82 facts", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range removeSchemaV83ForTestStatements() {
		if _, err := state.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("remove v83 with %q: %v", statement, err)
		}
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	values, err := state.ListVerificationSnapshotReceipts(ctx, run.ID, 1)
	if err != nil || len(values) != 0 {
		t.Fatalf("v83 upgrade fabricated a snapshot receipt: %#v err=%v", values, err)
	}
	if version, err := state.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("v83 upgrade version=%d err=%v", version, err)
	}
}
