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

func removeSchemaV84ForTestStatements() []string {
	return []string{
		`DROP TRIGGER trg_operator_verification_snapshot_receipt_review_delete_immutable`,
		`DROP TRIGGER trg_operator_verification_snapshot_receipt_review_update_immutable`,
		`DROP TRIGGER trg_operator_verification_snapshot_receipt_review_insert`,
		`DROP INDEX idx_operator_verification_snapshot_receipt_reviews_run_event`,
		`DROP TABLE operator_verification_snapshot_receipt_reviews`,
		`DELETE FROM schema_migrations WHERE version = 84`,
	}
}

func recordSnapshotReceiptFixture(t *testing.T, state *SQLiteStore,
	workspace WorkspaceRecord,
) (domain.Run, verification.SnapshotReceipt) {
	t.Helper()
	ctx := t.Context()
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "review exact snapshot receipt metadata",
			Profile: "code", WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := application.NewVerificationPlanService(state).Record(ctx,
		application.RecordVerificationPlanRequest{Version: verification.PlanProtocolVersion,
			RunID: run.ID, Title: "Receipt review", Summary: "Metadata-only review",
			Items: []application.VerificationPlanItemRequest{{Title: "Focused suite",
				ExpectedObservation: "Observe an explicit result"}},
			OperationKey: "snapshot-review-plan-operation-0001", AuthoredBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	exported, err := application.NewVerificationSnapshotExportService(state).Build(ctx, run.ID,
		plan.Plan.ID, 1, application.VerificationSnapshotExportFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := application.NewVerificationSnapshotReceiptService(state).Record(ctx,
		application.RecordVerificationSnapshotReceiptRequest{
			Version: verification.SnapshotReceiptProtocolVersion, RunID: run.ID,
			PlanID: plan.Plan.ID, PlanItemOrdinal: 1, Format: exported.Format,
			SnapshotHighWaterEventSequence: exported.SnapshotHighWaterEventSequence,
			ContentSHA256:                  exported.ContentSHA256, ConfirmMetadataSnapshot: true,
			OperationKey: "snapshot-review-receipt-operation-0001", RecordedBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	return run, recorded.Receipt
}

func TestVerificationSnapshotReceiptReviewIsImmutableIdempotentAndNonAuthorizing(t *testing.T) {
	ctx := t.Context()
	state, err := Open(filepath.Join(t.TempDir(), "verification-snapshot-receipt-review.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	run, receipt := recordSnapshotReceiptFixture(t, state, WorkspaceRecord{
		ID: "workspace-snapshot-receipt-review", Name: "snapshot-receipt-review",
		RootPath: t.TempDir(),
	})
	service := application.NewVerificationSnapshotReceiptReviewService(state)
	request := application.RecordVerificationSnapshotReceiptReviewRequest{
		Version: verification.SnapshotReceiptReviewProtocolVersion, RunID: run.ID,
		ReceiptID: receipt.ID, ReceiptContentSHA256: receipt.ContentSHA256,
		ReceiptEventSequence:        receipt.EventSequence,
		Decision:                    string(verification.SnapshotReceiptReviewMetadataConfirmed),
		ConfirmNonAuthorizingReview: true,
		OperationKey:                "snapshot-receipt-review-operation-0001", ReviewedBy: "operator",
	}
	recorded, err := service.Record(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if recorded.Replayed || recorded.Review.EventSequence <= receipt.EventSequence {
		t.Fatalf("receipt review lost causality: %#v", recorded)
	}
	replayed, err := service.Record(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Review.ID != recorded.Review.ID {
		t.Fatalf("receipt review replay diverged: %#v err=%v", replayed, err)
	}
	changed := request
	changed.Decision = string(verification.SnapshotReceiptReviewMetadataDisputed)
	if _, err := service.Record(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("same key accepted changed review intent: %v", err)
	}
	duplicate := request
	duplicate.OperationKey = "snapshot-receipt-review-operation-0002"
	if _, err := service.Record(ctx, duplicate); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("receipt accepted a second immutable review: %v", err)
	}
	mismatched := duplicate
	mismatched.ReceiptContentSHA256 = strings.Repeat("f", 64)
	if _, err := service.Record(ctx, mismatched); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("receipt review accepted a mismatched digest: %v", err)
	}
	inventory, err := service.Inventory(ctx, run.ID)
	if err != nil || len(inventory.Items) != 1 || !inventory.MetadataOnly ||
		!inventory.ReadOnly || !inventory.ReviewNonAuthorizing || inventory.SnapshotAccepted ||
		inventory.ResultAccepted || inventory.ResultInferred || inventory.RecordRewritten ||
		inventory.Approval || inventory.AuthorityGranted || inventory.ExecutionStarted {
		t.Fatalf("receipt review inventory widened authority: %#v err=%v", inventory, err)
	}
	if _, err := state.db.ExecContext(ctx,
		`UPDATE operator_verification_snapshot_receipt_reviews
		SET decision = 'metadata_disputed' WHERE id = ?`, recorded.Review.ID); err == nil {
		t.Fatal("receipt review update was accepted")
	}
	if _, err := state.db.ExecContext(ctx,
		`DELETE FROM operator_verification_snapshot_receipt_reviews WHERE id = ?`,
		recorded.Review.ID); err == nil {
		t.Fatal("receipt review delete was accepted")
	}
	timeline, err := state.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range timeline {
		if event.Type != events.VerificationSnapshotReviewRecordedEvent {
			continue
		}
		found = true
		if strings.Contains(event.PayloadJSON, `"reviewed_by"`) ||
			strings.Contains(event.PayloadJSON, `"operator"`) ||
			!strings.Contains(event.PayloadJSON, `"review_non_authorizing":true`) ||
			!strings.Contains(event.PayloadJSON, `"snapshot_accepted":false`) ||
			!strings.Contains(event.PayloadJSON, `"result_accepted":false`) ||
			!strings.Contains(event.PayloadJSON, `"authority_granted":false`) ||
			!strings.Contains(event.PayloadJSON, `"execution_started":false`) {
			t.Fatalf("receipt review event exposed identity or authority: %s", event.PayloadJSON)
		}
	}
	if !found {
		t.Fatal("receipt review event was not recorded")
	}
}

func TestSchemaV84UpgradeFabricatesNoSnapshotReceiptReview(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "v83-snapshot-receipt-review.db")
	state, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	run, _ := recordSnapshotReceiptFixture(t, state, WorkspaceRecord{
		ID: "workspace-v84-upgrade", Name: "v84-upgrade", RootPath: t.TempDir(),
	})
	for _, statement := range removeSchemaV84ForTestStatements() {
		if _, err := state.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("remove v84 with %q: %v", statement, err)
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
	values, err := state.ListVerificationSnapshotReceiptReviews(ctx, run.ID, 1)
	if err != nil || len(values) != 0 {
		t.Fatalf("v84 upgrade fabricated a snapshot receipt review: %#v err=%v", values, err)
	}
	if version, err := state.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("v84 upgrade version=%d err=%v", version, err)
	}
}
