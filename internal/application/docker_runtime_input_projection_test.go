package application

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
)

type canonicalRuntimeInputStager struct {
	probeCalls   int
	captureCalls int
	lastBundle   *recordingHostInputBundle
}

func (stager *canonicalRuntimeInputStager) Probe(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stager.probeCalls++
	return nil
}

func (stager *canonicalRuntimeInputStager) Stage(ctx context.Context,
	request sandbox.HostInputBundleRequest,
) (sandbox.HostInputBundleReport, error) {
	bundle, err := stager.Capture(ctx, request)
	if err != nil {
		return sandbox.HostInputBundleReport{}, err
	}
	report := bundle.Report()
	_ = bundle.Close()
	return report, nil
}

func (stager *canonicalRuntimeInputStager) Capture(ctx context.Context,
	request sandbox.HostInputBundleRequest,
) (sandbox.HostInputBundle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stager.captureCalls++
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	sourceParts := []string{"sandbox_host_input_source_snapshot.v1",
		strconv.Itoa(request.ReadOnlyMountCount())}
	mountOrdinal := 0
	for _, mount := range request.Manifest.Mounts {
		if mount.Access != sandbox.MountReadOnly {
			continue
		}
		mountOrdinal++
		name := fmt.Sprintf("mounts/%03d", mountOrdinal)
		header := &tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o555,
			Uid: 65532, Gid: 65532, ModTime: time.Unix(0, 0).UTC(),
			AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(),
			Format: tar.FormatPAX}
		if err := writer.WriteHeader(header); err != nil {
			return nil, err
		}
		sourceParts = append(sourceParts,
			testRuntimeProjectionFingerprint("sandbox_host_input_archive_path.v1", name),
			strconv.Itoa(int(tar.TypeDir)), "0",
			testRuntimeProjectionFingerprint("sandbox_host_input_directory.v1", name))
	}
	var artifactBytes int64
	for index, artifact := range request.Artifacts {
		name := fmt.Sprintf("artifacts/%03d", index+1)
		header := &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o444,
			Size: int64(len(artifact.Content)), Uid: 65532, Gid: 65532,
			ModTime: time.Unix(0, 0).UTC(), AccessTime: time.Unix(0, 0).UTC(),
			ChangeTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX}
		if err := writer.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := writer.Write([]byte(artifact.Content)); err != nil {
			return nil, err
		}
		artifactBytes += int64(len(artifact.Content))
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(output.Bytes())
	report, err := sandbox.NewHostInputBundleReport(sandbox.HostInputBundleMeasurements{
		ReadOnlyMountCount: request.ReadOnlyMountCount(), ArtifactCount: len(request.Artifacts),
		DirectoryCount: request.ReadOnlyMountCount(), ArtifactBytes: artifactBytes,
		BundleBytes:           int64(output.Len()),
		SourceSnapshotDigest:  testRuntimeProjectionFingerprint(sourceParts...),
		ArtifactPayloadDigest: request.ArtifactPayloadDigest(),
		BundleDigest:          hex.EncodeToString(digest[:]),
	}, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	bundle := &recordingHostInputBundle{Reader: bytes.NewReader(output.Bytes()), report: report}
	stager.lastBundle = bundle
	return bundle, nil
}

func testRuntimeProjectionFingerprint(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		value := []byte(part)
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = hash.Write(value)
		_, _ = hash.Write([]byte{'|'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func TestDockerRuntimeInputProjectionPlansPersistsReplaysAndDoesNotWidenAuthority(
	t *testing.T,
) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, observation := prepareDockerContainerPlanAuthority(t, ctx, service, run.ID,
		root, "docker-runtime-input", "runtime_input_operator")
	containerPlan, err := service.CompileDockerContainerPlan(ctx,
		CompileDockerContainerPlanRequest{ObservationID: observation.ID, Manifest: manifest,
			OperationKey: "docker-runtime-input-container-plan",
			RequestedBy:  "runtime_input_operator"})
	if err != nil {
		t.Fatal(err)
	}
	writer := &countingDockerWriteTransport{}
	stager := &canonicalRuntimeInputStager{}
	handoffTransport := &recordingDockerHostInputHandoffTransport{}
	service.WithDockerContainerWriteTransport(writer).
		WithDockerHostInputStager(stager).
		WithDockerHostInputHandoffTransport(handoffTransport)
	if _, err := service.RehearseDockerContainer(ctx, RehearseDockerContainerRequest{
		PlanID: containerPlan.ID, Manifest: manifest,
		OperationKey: "docker-runtime-input-rehearsal",
		RequestedBy:  "runtime_input_operator", OperatorConfirmed: true,
		StageHostInputs: true, OperatorConfirmedHostInputStaging: true,
		HandoffHostInputs: true, OperatorConfirmedHostInputHandoff: true,
	}); err != nil {
		t.Fatal(err)
	}
	handoffs, err := service.ListDockerHostInputHandoffs(ctx, run.ID, 10)
	if err != nil || len(handoffs) != 1 || handoffs[0].Handoff == nil {
		t.Fatalf("completed handoff missing: %#v err=%v", handoffs, err)
	}
	request := PlanDockerRuntimeInputsRequest{HandoffIntentID: handoffs[0].Intent.ID,
		Manifest: manifest, OperationKey: "docker-runtime-input-projection",
		RequestedBy: "runtime_input_operator"}
	if _, err := service.PlanDockerRuntimeInputs(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition || stager.captureCalls != 1 {
		t.Fatalf("runtime projection skipped confirmation or recaptured input: %v", err)
	}
	request.OperatorConfirmed = true
	plan, err := service.PlanDockerRuntimeInputs(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Replayed || plan.Status != sandbox.DockerRuntimeInputProjectionStatusCompiled ||
		plan.ProjectionCount != 1 || plan.DirectoryRootCount != 1 ||
		plan.FileRootCount != 0 || plan.TotalEntryCount != 1 || len(plan.Items) != 1 ||
		!plan.OperatorConfirmed || !plan.ExactTargetBinding ||
		!plan.AllVolumesReadOnly || !plan.AllVolumesNoCopy ||
		!plan.BundleRecaptured || !plan.BundleDigestMatched || plan.DaemonContacted ||
		plan.DaemonApplied || plan.ContainerStarted || plan.ProcessExecuted ||
		plan.OutputExported || plan.ProductionExecutionSubmitted ||
		plan.ProductionVerified || plan.BackendEnabled || plan.ExecutionAuthorized ||
		plan.ArtifactCommitAuthorized || stager.captureCalls != 2 ||
		stager.lastBundle == nil || !stager.lastBundle.closed {
		t.Fatalf("runtime projection widened authority: plan=%#v stager=%#v", plan, stager)
	}
	loaded, err := service.GetDockerRuntimeInputProjectionPlan(ctx, plan.ID)
	if err != nil || loaded.ProjectionFingerprint != plan.ProjectionFingerprint {
		t.Fatalf("load runtime projection: %#v err=%v", loaded, err)
	}
	listed, err := service.ListDockerRuntimeInputProjectionPlans(ctx, run.ID, 10)
	if err != nil || len(listed) != 1 || listed[0].ID != plan.ID {
		t.Fatalf("list runtime projections: %#v err=%v", listed, err)
	}
	replayed, err := service.PlanDockerRuntimeInputs(ctx, request)
	if err != nil || !replayed.Replayed || replayed.ID != plan.ID ||
		stager.captureCalls != 2 {
		t.Fatalf("runtime projection replay recaptured input: %#v err=%v", replayed, err)
	}
	changed := request
	changed.Manifest.TimeoutSeconds++
	if _, err := service.PlanDockerRuntimeInputs(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict || stager.captureCalls != 2 {
		t.Fatalf("changed Manifest reused runtime projection operation: %v", err)
	}
	otherOperation := request
	otherOperation.OperationKey = "docker-runtime-input-projection-other"
	if _, err := service.PlanDockerRuntimeInputs(ctx, otherOperation); apperror.CodeOf(err) != apperror.CodeConflict || stager.captureCalls != 2 {
		t.Fatalf("handoff accepted a second runtime projection: %v", err)
	}
	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(event.PayloadJSON, root) ||
			strings.Contains(event.PayloadJSON, "/workspace") ||
			strings.Contains(event.PayloadJSON, "cyberagent-runtime-") {
			t.Fatalf("runtime projection event leaked transient input data: %#v", event)
		}
	}
}
