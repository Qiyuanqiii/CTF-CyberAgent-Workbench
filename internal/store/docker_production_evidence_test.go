package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
)

func TestDockerProductionEvidencePersistsReplaysAndRemainsImmutable(t *testing.T) {
	ctx := context.Background()
	st, run, root := openSandboxManifestStore(t, ctx)
	value, operation := prepareDockerProductionEvidenceStoreFixture(
		t, ctx, st, run.ID, root, "production-evidence-store")
	assertDockerProductionEvidenceOperationKeyBoundBySQL(t, ctx, st, value, operation)
	created, replayed, err := st.CreateDockerProductionEvidence(ctx, value, operation)
	if err != nil || replayed {
		t.Fatalf("create Docker production evidence: replayed=%t err=%v", replayed, err)
	}
	if created.StartGatePassed || created.ProcessExecutionAuthorized ||
		created.ArtifactCommitAuthorized || created.RequiredCheckCount != sandbox.MaxBackendChecks {
		t.Fatalf("stored evidence widened authority: %#v", created)
	}
	loaded, err := st.GetDockerProductionEvidence(ctx, created.ID)
	if err != nil || loaded.CaptureFingerprint != created.CaptureFingerprint ||
		len(loaded.Items) != sandbox.MaxBackendChecks {
		t.Fatalf("load Docker production evidence: %#v err=%v", loaded, err)
	}
	listed, err := st.ListDockerProductionEvidence(ctx, run.ID, 10)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("list Docker production evidence: %#v err=%v", listed, err)
	}
	replayedValue, replayed, err := st.CreateDockerProductionEvidence(ctx, value, operation)
	if err != nil || !replayed || replayedValue.ID != created.ID {
		t.Fatalf("replay Docker production evidence: %#v replayed=%t err=%v",
			replayedValue, replayed, err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE sandbox_docker_production_evidence
		SET start_gate_passed = 1 WHERE id = ?`, created.ID); err == nil {
		t.Fatal("Docker production evidence was mutable")
	}
	if _, err := st.db.ExecContext(ctx, `DELETE FROM sandbox_docker_production_evidence_items
		WHERE evidence_id = ? AND ordinal = 1`, created.ID); err == nil {
		t.Fatal("Docker production evidence item was mutable")
	}
	eventsForRun, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range eventsForRun {
		if event.SubjectID == created.ID && event.Type ==
			"sandbox.docker_production_evidence_captured" {
			found = true
		}
	}
	if !found {
		t.Fatal("Docker production evidence event was not recorded")
	}
}

func assertDockerProductionEvidenceOperationKeyBoundBySQL(t *testing.T, ctx context.Context,
	st *SQLiteStore, value sandbox.DockerProductionEvidence,
	operation sandbox.DockerProductionEvidenceOperation,
) {
	t.Helper()
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertDockerProductionEvidenceTx(ctx, tx, value); err != nil {
		t.Fatal(err)
	}
	for _, item := range value.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_items
			(evidence_id, ordinal, name, probe_code, state, observed,
			production_verified, sufficient_for_start, blocker_code, evidence_digest)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, item.Ordinal, item.Name,
			item.ProbeCode, item.State, boolInt(item.Observed),
			boolInt(item.ProductionVerified), boolInt(item.SufficientForStart),
			item.BlockerCode, item.EvidenceDigest); err != nil {
			t.Fatal(err)
		}
	}
	mismatchedKey := runmutation.Fingerprint(
		sandbox.DockerProductionEvidenceOperationVersion, value.ReviewID, "mismatch")
	if mismatchedKey == value.OperationKeyDigest {
		t.Fatal("mismatched operation key fixture collided")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_docker_production_evidence_operations
		(key_digest, request_fingerprint, evidence_id, review_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, mismatchedKey,
		operation.RequestFingerprint, operation.EvidenceID, operation.ReviewID,
		operation.RunID, operation.RequestedBy, ts(operation.CreatedAt)); err == nil {
		t.Fatal("SQLite accepted an operation key that was not bound to its evidence")
	}
}

func TestSchemaV65PreservesReviewWithoutFabricatingProductionEvidence(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "docker-production-evidence-v64.db")
	st, run, root := openSandboxManifestStoreAt(t, ctx, databasePath)
	value, _ := prepareDockerProductionEvidenceStoreFixture(
		t, ctx, st, run.ID, root, "production-evidence-upgrade")
	for _, statement := range removeSchemaV65ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("remove schema v65 with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	if version, err := upgraded.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v65 upgrade version=%d err=%v", version, err)
	}
	if _, err := upgraded.GetDockerStartGateReview(ctx, value.ReviewID); err != nil {
		t.Fatalf("schema v65 lost start-gate review: %v", err)
	}
	values, err := upgraded.ListDockerProductionEvidence(ctx, run.ID, 10)
	if err != nil || len(values) != 0 {
		t.Fatalf("schema v65 fabricated production evidence: %#v err=%v", values, err)
	}
}

func prepareDockerProductionEvidenceStoreFixture(t *testing.T, ctx context.Context,
	st *SQLiteStore, runID, root, key string,
) (sandbox.DockerProductionEvidence, sandbox.DockerProductionEvidenceOperation) {
	t.Helper()
	cleanup, application := prepareCompletedDockerRuntimeInputResourceCleanupForReviewTest(
		t, ctx, st, runID, root, key)
	projection, err := st.GetDockerRuntimeInputProjectionPlan(ctx,
		application.Intent.ProjectionID)
	if err != nil {
		t.Fatal(err)
	}
	containerPlan, err := st.GetDockerContainerPlan(ctx, application.Intent.ContainerPlanID)
	if err != nil {
		t.Fatal(err)
	}
	preflight, err := st.GetSandboxDisabledPreflight(ctx, containerPlan.PreflightID)
	if err != nil {
		t.Fatal(err)
	}
	reviewKey := runmutation.Fingerprint(sandbox.DockerStartGateReviewOperationVersion,
		cleanup.Intent.ID, key+"-review")
	review, err := sandbox.NewDockerStartGateReview(
		idgen.New("sandbox-docker-start-gate-review"), reviewKey,
		cleanup.Intent.RequestedBy, sandbox.DockerStartGateReviewBinding{
			CleanupIntentID: cleanup.Intent.ID, CleanupResultID: cleanup.Result.ID,
			ApplicationIntentID: application.Intent.ID,
			ApplicationResultID: application.Result.ID, ProjectionID: projection.ID,
			ContainerPlanID: containerPlan.ID, PreflightID: preflight.ID,
			RunID: application.Intent.RunID, MissionID: application.Intent.MissionID,
			WorkspaceID:              application.Intent.WorkspaceID,
			ManifestFingerprint:      application.Intent.ManifestFingerprint,
			ThreatModelFingerprint:   preflight.Handshake.ThreatModelFingerprint,
			CleanupResultFingerprint: cleanup.Result.ResultFingerprint,
			MaxLogBytes:              preflight.OutputPlan.MaxOutputBytes,
		}, true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	reviewOperation, err := sandbox.NewDockerStartGateReviewOperation(reviewKey, review)
	if err != nil {
		t.Fatal(err)
	}
	review, _, err = st.CreateDockerStartGateReview(ctx, review, reviewOperation)
	if err != nil {
		t.Fatal(err)
	}
	collector := sandbox.NewLocalDockerProductionEvidenceCollector()
	observation, err := collector.Capture(ctx, sandbox.DockerProductionEvidenceCaptureRequest{
		ReviewID: review.ID, RunID: review.RunID,
		AuthorityFingerprint: review.AuthorityFingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	operationKey := runmutation.Fingerprint(sandbox.DockerProductionEvidenceOperationVersion,
		review.ID, key+"-capture")
	value, err := sandbox.NewDockerProductionEvidence(
		idgen.New("sandbox-docker-production-evidence"), operationKey, review.RequestedBy,
		review, observation, true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	operation, err := sandbox.NewDockerProductionEvidenceOperation(operationKey, value)
	if err != nil {
		t.Fatal(err)
	}
	return value, operation
}
