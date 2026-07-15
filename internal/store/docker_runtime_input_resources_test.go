package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
)

func TestDockerRuntimeInputResourceLedgerIsReplayableFencedAndMetadataOnly(t *testing.T) {
	ctx := context.Background()
	st, run, root := openSandboxManifestStore(t, ctx)
	application, descriptor := prepareCompletedDockerRuntimeInputApplicationForResourceTest(
		t, ctx, st, run.ID, root, "runtime-resource-ledger")
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	observation := sandbox.DockerRuntimeInputResourceObservation{
		EndpointClass: endpoint.Class, EndpointFingerprint: endpoint.Fingerprint,
		TargetState:      sandbox.DockerRuntimeInputResourceTargetOwned,
		OwnedVolumeCount: len(descriptor.Mounts), DaemonReadCount: len(descriptor.Mounts) + 1,
		ObservedAt: time.Now().UTC(),
	}
	inspection, err := sandbox.NewDockerRuntimeInputResourceInspection(
		idgen.New("sandbox-docker-runtime-input-resource-inspection"),
		runmutation.Fingerprint(sandbox.DockerRuntimeInputResourceInspectionOperationVersion,
			application.Intent.ID, "inspect"), application.Intent.RequestedBy,
		application, descriptor, observation)
	if err != nil {
		t.Fatal(err)
	}
	recorded, replayed, err := st.RecordDockerRuntimeInputResourceInspection(ctx, inspection)
	if err != nil || replayed || !recorded.Complete || !recorded.CleanupEligible {
		t.Fatalf("record inspection: %#v replayed=%t err=%v", recorded, replayed, err)
	}
	replayedInspection, replayed, err := st.RecordDockerRuntimeInputResourceInspection(ctx, inspection)
	if err != nil || !replayed || !replayedInspection.Replayed ||
		replayedInspection.InspectionFingerprint != inspection.InspectionFingerprint {
		t.Fatalf("replay inspection: %#v replayed=%t err=%v",
			replayedInspection, replayed, err)
	}
	byOperation, found, err := st.GetDockerRuntimeInputResourceInspectionByOperation(ctx,
		inspection.OperationKeyDigest)
	if err != nil || !found || byOperation.ID != inspection.ID {
		t.Fatalf("load inspection by operation: %#v found=%t err=%v", byOperation, found, err)
	}
	listedInspections, err := st.ListDockerRuntimeInputResourceInspections(ctx, run.ID, 10)
	if err != nil || len(listedInspections) != 1 || listedInspections[0].ID != inspection.ID {
		t.Fatalf("list inspections: %#v err=%v", listedInspections, err)
	}

	cleanupIntent, err := sandbox.NewDockerRuntimeInputResourceCleanupIntent(
		idgen.New("sandbox-docker-runtime-input-resource-cleanup"),
		runmutation.Fingerprint(sandbox.DockerRuntimeInputResourceCleanupOperationVersion,
			inspection.ID, "cleanup"), inspection, descriptor, endpoint, true, true,
		inspection.RequestedBy, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := st.BeginDockerRuntimeInputResourceCleanup(ctx, cleanupIntent,
		"runtime_resource_cleanup_owner", time.Minute)
	if err != nil || acquired.Record.Lease.Generation != 1 || acquired.Replayed || acquired.TookOver {
		t.Fatalf("begin cleanup: %#v err=%v", acquired, err)
	}
	if _, err := st.BeginDockerRuntimeInputResourceCleanup(ctx, cleanupIntent,
		"second_runtime_resource_cleanup_owner", time.Minute); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("active cleanup lease was not exclusive: %v", err)
	}
	if _, err := st.RecordDockerRuntimeInputResourceCleanupFailure(ctx, cleanupIntent.ID,
		acquired.Record.Lease, sandbox.DockerRuntimeInputResourceErrorConnection,
		time.Now().UTC().Add(15*time.Second)); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("future cleanup failure timestamp was not rejected: %v", err)
	}
	failed, err := st.RecordDockerRuntimeInputResourceCleanupFailure(ctx, cleanupIntent.ID,
		acquired.Record.Lease, sandbox.DockerRuntimeInputResourceErrorConnection,
		time.Now().UTC())
	if err != nil || failed.Lease.Status != sandbox.DockerRuntimeInputResourceCleanupLeaseReleased ||
		len(failed.Failures) != 1 {
		t.Fatalf("record cleanup failure: %#v err=%v", failed, err)
	}
	resumed, err := st.AcquireDockerRuntimeInputResourceCleanup(ctx, cleanupIntent.ID,
		cleanupIntent.RequestedBy, "resumed_runtime_resource_cleanup_owner", time.Minute)
	if err != nil || resumed.Record.Lease.Generation != 2 || resumed.TookOver {
		t.Fatalf("resume cleanup: %#v err=%v", resumed, err)
	}
	total := len(descriptor.Mounts) + 1
	staleResult, err := sandbox.NewDockerRuntimeInputResourceCleanupResult(
		"runtime-resource-stale-result", cleanupIntent, acquired.Record.Lease, descriptor,
		total, 0, total, 2*total, total, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompleteDockerRuntimeInputResourceCleanup(ctx, staleResult,
		acquired.Record.Lease); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("stale cleanup worker committed: %v", err)
	}
	futureResult, err := sandbox.NewDockerRuntimeInputResourceCleanupResult(
		"runtime-resource-future-result", cleanupIntent, resumed.Record.Lease, descriptor,
		total, 0, total, 2*total, total, time.Now().UTC().Add(15*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.CompleteDockerRuntimeInputResourceCleanup(ctx, futureResult,
		resumed.Record.Lease); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("future cleanup result timestamp was not rejected: %v", err)
	}
	result, err := sandbox.NewDockerRuntimeInputResourceCleanupResult(
		"runtime-resource-cleanup-result", cleanupIntent, resumed.Record.Lease, descriptor,
		total, 0, total, 2*total, total, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	completed, replayed, err := st.CompleteDockerRuntimeInputResourceCleanup(ctx, result,
		resumed.Record.Lease)
	if err != nil || replayed || completed.Result == nil ||
		completed.Lease.Status != sandbox.DockerRuntimeInputResourceCleanupLeaseReleased {
		t.Fatalf("complete cleanup: %#v replayed=%t err=%v", completed, replayed, err)
	}
	replayedAcquisition, err := st.BeginDockerRuntimeInputResourceCleanup(ctx, cleanupIntent,
		"third_runtime_resource_cleanup_owner", time.Minute)
	if err != nil || !replayedAcquisition.Replayed || replayedAcquisition.Record.Result == nil {
		t.Fatalf("cleanup replay failed: %#v err=%v", replayedAcquisition, err)
	}
	loaded, err := st.GetDockerRuntimeInputResourceCleanup(ctx, cleanupIntent.ID)
	if err != nil || loaded.Result == nil || loaded.Result.ResultFingerprint != result.ResultFingerprint {
		t.Fatalf("load cleanup: %#v err=%v", loaded, err)
	}
	byCleanupOperation, found, err := st.GetDockerRuntimeInputResourceCleanupByOperation(ctx,
		cleanupIntent.OperationKeyDigest)
	if err != nil || !found || byCleanupOperation.Intent.ID != cleanupIntent.ID {
		t.Fatalf("load cleanup by operation: %#v found=%t err=%v",
			byCleanupOperation, found, err)
	}
	listedCleanups, err := st.ListDockerRuntimeInputResourceCleanups(ctx, run.ID, 10)
	if err != nil || len(listedCleanups) != 1 || listedCleanups[0].Intent.ID != cleanupIntent.ID {
		t.Fatalf("list cleanups: %#v err=%v", listedCleanups, err)
	}

	for _, mutation := range []string{
		`UPDATE sandbox_docker_runtime_input_resource_inspections SET cleanup_eligible = 0 WHERE id = ?`,
		`UPDATE sandbox_docker_runtime_input_resource_cleanup_intents SET process_execution_authorized = 1 WHERE id = ?`,
		`DELETE FROM sandbox_docker_runtime_input_resource_cleanup_failures WHERE intent_id = ?`,
		`UPDATE sandbox_docker_runtime_input_resource_cleanup_results SET target_absent = 0 WHERE intent_id = ?`,
	} {
		id := cleanupIntent.ID
		if strings.Contains(mutation, "resource_inspections") {
			id = inspection.ID
		}
		if _, err := st.db.ExecContext(ctx, mutation, id); err == nil ||
			!strings.Contains(err.Error(), "cannot be") {
			t.Fatalf("resource ledger mutation was not rejected: %s: %v", mutation, err)
		}
	}
	for query, id := range map[string]string{
		`DELETE FROM sandbox_docker_runtime_input_resource_cleanup_leases WHERE intent_id = ?`: cleanupIntent.ID,
		`DELETE FROM sandbox_docker_runtime_input_application_leases WHERE intent_id = ?`:      application.Intent.ID,
	} {
		if _, err := st.db.ExecContext(ctx, query, id); err == nil ||
			!strings.Contains(err.Error(), "cannot be deleted") {
			t.Fatalf("lease deletion was not rejected: %s: %v", query, err)
		}
	}
	assertDockerRuntimeInputResourceSchemaPrivacy(t, ctx, st)
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundInspected, foundPrepared, foundFailed, foundCompleted := false, false, false, false
	for _, event := range timeline {
		switch event.Type {
		case events.SandboxDockerRuntimeInputResourceInspectedEvent:
			foundInspected = true
		case events.SandboxDockerRuntimeInputResourceCleanupPreparedEvent:
			foundPrepared = true
		case events.SandboxDockerRuntimeInputResourceCleanupFailedEvent:
			foundFailed = true
		case events.SandboxDockerRuntimeInputResourceCleanupCompletedEvent:
			foundCompleted = true
		}
	}
	if !foundInspected || !foundPrepared || !foundFailed || !foundCompleted {
		t.Fatalf("resource lifecycle events missing: inspected=%t prepared=%t failed=%t completed=%t",
			foundInspected, foundPrepared, foundFailed, foundCompleted)
	}
}

func TestSchemaV62PreservesV61ApplicationWithoutFabricatingResourceRecords(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "docker-runtime-input-resources-v61.db")
	st, run, root := openSandboxManifestStoreAt(t, ctx, databasePath)
	application, _ := prepareCompletedDockerRuntimeInputApplicationForResourceTest(
		t, ctx, st, run.ID, root, "runtime-resource-upgrade")
	for _, statement := range removeSchemaV62ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("remove schema v62 with %q: %v", statement, err)
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
	version, err := upgraded.SchemaVersion(ctx)
	if err != nil || version != LatestSchemaVersion {
		t.Fatalf("schema v62 upgrade version=%d err=%v", version, err)
	}
	if loaded, err := upgraded.GetDockerRuntimeInputApplication(ctx,
		application.Intent.ID); err != nil || loaded.Result == nil {
		t.Fatalf("schema v62 lost v61 application: %#v err=%v", loaded, err)
	}
	inspections, err := upgraded.ListDockerRuntimeInputResourceInspections(ctx, run.ID, 10)
	if err != nil || len(inspections) != 0 {
		t.Fatalf("schema v62 fabricated inspections: %#v err=%v", inspections, err)
	}
	cleanups, err := upgraded.ListDockerRuntimeInputResourceCleanups(ctx, run.ID, 10)
	if err != nil || len(cleanups) != 0 {
		t.Fatalf("schema v62 fabricated cleanups: %#v err=%v", cleanups, err)
	}
}

func prepareCompletedDockerRuntimeInputApplicationForResourceTest(t *testing.T,
	ctx context.Context, st *SQLiteStore, runID, root, key string,
) (sandbox.DockerRuntimeInputApplicationRecord, sandbox.DockerRuntimeInputResourceDescriptor) {
	t.Helper()
	projection, operation := prepareDockerRuntimeInputProjectionStoreFixture(
		t, ctx, st, runID, root, key)
	projection, replayed, err := st.CreateDockerRuntimeInputProjectionPlan(ctx, projection, operation)
	if err != nil || replayed {
		t.Fatalf("create projection: replayed=%t err=%v", replayed, err)
	}
	intent, request, writeRequest := newDockerRuntimeInputApplicationStoreFixtureFull(
		t, ctx, st, root, projection)
	acquired, err := st.BeginDockerRuntimeInputApplication(ctx, intent,
		"runtime_resource_application_owner", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	result, err := sandbox.NewDockerRuntimeInputApplicationResult(
		idgen.New("sandbox-docker-runtime-input-result"), intent, acquired.Record.Lease, request,
		strings.Repeat("d", 64), 3+5*len(request.Mounts), 1+4*len(request.Mounts), 0,
		time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	application, replayed, err := st.CompleteDockerRuntimeInputApplication(
		ctx, result, acquired.Record.Lease)
	if err != nil || replayed {
		t.Fatalf("complete application: replayed=%t err=%v", replayed, err)
	}
	descriptor, err := sandbox.NewDockerRuntimeInputResourceDescriptor(
		application, projection, writeRequest)
	if err != nil {
		t.Fatal(err)
	}
	return application, descriptor
}

func assertDockerRuntimeInputResourceSchemaPrivacy(t *testing.T, ctx context.Context,
	st *SQLiteStore,
) {
	t.Helper()
	for _, table := range []string{"sandbox_docker_runtime_input_resource_inspections",
		"sandbox_docker_runtime_input_resource_cleanup_intents",
		"sandbox_docker_runtime_input_resource_cleanup_leases",
		"sandbox_docker_runtime_input_resource_cleanup_failures",
		"sandbox_docker_runtime_input_resource_cleanup_results"} {
		rows, err := st.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue,
				&primaryKey); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			switch name {
			case "host_path", "workspace_root", "mount_source", "mount_target", "content",
				"raw_content", "container_id", "container_name", "volume_name", "carrier_name",
				"manifest_json", "archive_blob", "operation_key":
				_ = rows.Close()
				t.Fatalf("schema v62 persists private material in %s.%s", table, name)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
