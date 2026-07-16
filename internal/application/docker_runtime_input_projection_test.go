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

type recordingDockerRuntimeInputApplicationTransport struct {
	calls   int
	cancel  context.CancelFunc
	request sandbox.DockerRuntimeInputApplicationRequest
}

type recordingDockerRuntimeInputResourceInspector struct {
	calls              int
	targetState        string
	foreignVolumeCount int
}

func (transport *recordingDockerRuntimeInputResourceInspector) Endpoint() sandbox.DockerObservationEndpoint {
	value, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return value
}

func (transport *recordingDockerRuntimeInputResourceInspector) Inspect(ctx context.Context,
	descriptor sandbox.DockerRuntimeInputResourceDescriptor,
) (sandbox.DockerRuntimeInputResourceObservation, error) {
	if err := ctx.Err(); err != nil {
		return sandbox.DockerRuntimeInputResourceObservation{}, err
	}
	transport.calls++
	targetState := transport.targetState
	if targetState == "" {
		targetState = sandbox.DockerRuntimeInputResourceTargetOwned
	}
	owned := len(descriptor.Mounts) - transport.foreignVolumeCount
	return sandbox.DockerRuntimeInputResourceObservation{
		EndpointClass:       transport.Endpoint().Class,
		EndpointFingerprint: transport.Endpoint().Fingerprint,
		TargetState:         targetState,
		OwnedVolumeCount:    owned,
		ForeignVolumeCount:  transport.foreignVolumeCount,
		DaemonReadCount:     len(descriptor.Mounts) + 1,
		ObservedAt:          time.Now().UTC(),
	}, nil
}

type recordingDockerRuntimeInputResourceCleanupTransport struct {
	calls     int
	cancel    bool
	onCleanup func(sandbox.DockerRuntimeInputResourceCleanupIntent)
}

type recordingDockerProductionEvidenceCollector struct {
	calls         int
	forceComplete bool
	delegate      sandbox.LocalDockerProductionEvidenceCollector
	before        func(sandbox.DockerProductionEvidenceCaptureRequest)
}

type recordingDockerProductionEvidenceHarness struct {
	reconcileCalls  int
	captureCalls    int
	beforeReconcile func(sandbox.DockerProductionEvidenceHarnessRequest)
	beforeCapture   func(sandbox.DockerProductionEvidenceHarnessCaptureRequest)
}

func (*recordingDockerProductionEvidenceHarness) Capture(context.Context,
	sandbox.DockerProductionEvidenceCaptureRequest,
) (sandbox.DockerProductionEvidenceObservation, error) {
	return sandbox.DockerProductionEvidenceObservation{},
		fmt.Errorf("inert collector path must not run for an enabled harness")
}

func (*recordingDockerProductionEvidenceHarness) HarnessEnabled() bool { return true }

func (collector *recordingDockerProductionEvidenceHarness) ReconcileHarness(
	_ context.Context, request sandbox.DockerProductionEvidenceHarnessRequest,
) (sandbox.DockerProductionEvidenceHarnessInventory, error) {
	collector.reconcileCalls++
	if collector.beforeReconcile != nil {
		collector.beforeReconcile(request)
	}
	endpoint, err := sandbox.NewDockerObservationEndpoint(
		sandbox.DockerObservationEndpointLocalUnix)
	if err != nil {
		return sandbox.DockerProductionEvidenceHarnessInventory{}, err
	}
	return sandbox.NewDockerProductionEvidenceHarnessInventory(endpoint, nil)
}

func (collector *recordingDockerProductionEvidenceHarness) CaptureHarness(
	_ context.Context, request sandbox.DockerProductionEvidenceHarnessCaptureRequest,
) (sandbox.DockerProductionEvidenceObservation, error) {
	collector.captureCalls++
	if collector.beforeCapture != nil {
		collector.beforeCapture(request)
	}
	return sandbox.NewDockerProductionEvidenceHarnessObservation(
		request.AuthorityFingerprint, strings.Repeat("9", 64))
}

func (collector *recordingDockerProductionEvidenceCollector) Capture(ctx context.Context,
	request sandbox.DockerProductionEvidenceCaptureRequest,
) (sandbox.DockerProductionEvidenceObservation, error) {
	collector.calls++
	if collector.before != nil {
		collector.before(request)
	}
	value, err := collector.delegate.Capture(ctx, request)
	if err == nil && collector.forceComplete {
		value.Status = sandbox.DockerProductionEvidenceStatusComplete
		value.PlatformClass = sandbox.DockerProductionEvidencePlatformLinux
		value.EndpointClass = sandbox.DockerObservationEndpointLocalUnix
		value.RealDaemonContacted = true
	}
	return value, err
}

func (transport *recordingDockerRuntimeInputResourceCleanupTransport) Endpoint() sandbox.DockerObservationEndpoint {
	value, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return value
}

func (transport *recordingDockerRuntimeInputResourceCleanupTransport) Cleanup(ctx context.Context,
	intent sandbox.DockerRuntimeInputResourceCleanupIntent,
	lease sandbox.DockerRuntimeInputResourceCleanupLease,
	descriptor sandbox.DockerRuntimeInputResourceDescriptor,
) (sandbox.DockerRuntimeInputResourceCleanupResult, error) {
	if err := ctx.Err(); err != nil {
		return sandbox.DockerRuntimeInputResourceCleanupResult{}, err
	}
	transport.calls++
	if transport.onCleanup != nil {
		transport.onCleanup(intent)
	}
	if transport.cancel {
		transport.cancel = false
		return sandbox.DockerRuntimeInputResourceCleanupResult{}, context.Canceled
	}
	total := len(descriptor.Mounts) + 1
	return sandbox.NewDockerRuntimeInputResourceCleanupResult(
		fmt.Sprintf("runtime-input-cleanup-result-%d", transport.calls), intent, lease,
		descriptor, total, 0, total, 2*total, total, time.Now().UTC())
}

func (transport *recordingDockerRuntimeInputApplicationTransport) Endpoint() sandbox.DockerObservationEndpoint {
	value, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return value
}

func (transport *recordingDockerRuntimeInputApplicationTransport) Apply(ctx context.Context,
	intent sandbox.DockerRuntimeInputApplicationIntent,
	lease sandbox.DockerRuntimeInputApplicationLease,
	request sandbox.DockerRuntimeInputApplicationRequest,
) (sandbox.DockerRuntimeInputApplicationResult, error) {
	if err := ctx.Err(); err != nil {
		return sandbox.DockerRuntimeInputApplicationResult{}, err
	}
	transport.calls++
	transport.request = request
	if transport.cancel != nil {
		transport.cancel()
		return sandbox.DockerRuntimeInputApplicationResult{}, context.Canceled
	}
	count := len(request.Mounts)
	return sandbox.NewDockerRuntimeInputApplicationResult(
		fmt.Sprintf("runtime-input-result-%d", transport.calls), intent, lease, request,
		strings.Repeat("d", 64), 3+5*count, 1+4*count, 0, time.Now().UTC())
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
	runtimeTransport := &recordingDockerRuntimeInputApplicationTransport{}
	service.WithDockerRuntimeInputApplicationTransport(runtimeTransport)
	applyRequest := ApplyDockerRuntimeInputsRequest{ProjectionID: plan.ID,
		Manifest: manifest, OperationKey: "docker-runtime-input-application",
		RequestedBy: "runtime_input_operator"}
	if _, err := service.ApplyDockerRuntimeInputs(ctx, applyRequest); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		runtimeTransport.calls != 0 || stager.captureCalls != 2 {
		t.Fatalf("runtime input application skipped dual confirmation: calls=%d captures=%d err=%v",
			runtimeTransport.calls, stager.captureCalls, err)
	}
	applyRequest.OperatorConfirmed, applyRequest.DaemonWriteConfirmed = true, true
	applyCtx, cancelApply := context.WithCancel(ctx)
	runtimeTransport.cancel = cancelApply
	failed, err := service.ApplyDockerRuntimeInputs(applyCtx, applyRequest)
	if apperror.CodeOf(err) != apperror.CodeCancelled ||
		failed.Lease.Status != sandbox.DockerRuntimeInputApplicationLeaseReleased ||
		len(failed.Failures) != 1 || failed.Result != nil || runtimeTransport.calls != 1 ||
		stager.captureCalls != 3 || failed.Failures[0].Code !=
		sandbox.DockerRuntimeInputApplicationErrorCanceled {
		t.Fatalf("runtime input failure was not durable: record=%#v calls=%d captures=%d err=%v",
			failed, runtimeTransport.calls, stager.captureCalls, err)
	}
	runtimeTransport.cancel = nil
	resumeRequest := ResumeDockerRuntimeInputsRequest{
		IntentID: failed.Intent.ID, Manifest: manifest, RequestedBy: "runtime_input_operator",
	}
	if _, err := service.ResumeDockerRuntimeInputs(ctx, resumeRequest); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		runtimeTransport.calls != 1 || stager.captureCalls != 3 {
		t.Fatalf("runtime input resume skipped dual confirmation: calls=%d captures=%d err=%v",
			runtimeTransport.calls, stager.captureCalls, err)
	}
	notAcquired, err := service.GetDockerRuntimeInputApplication(ctx, failed.Intent.ID)
	if err != nil || notAcquired.Lease.Generation != 1 ||
		notAcquired.Lease.Status != sandbox.DockerRuntimeInputApplicationLeaseReleased {
		t.Fatalf("unconfirmed resume changed lease: record=%#v err=%v", notAcquired, err)
	}
	resumeRequest.OperatorConfirmed, resumeRequest.DaemonWriteConfirmed = true, true
	completed, err := service.ResumeDockerRuntimeInputs(ctx, resumeRequest)
	if err != nil || completed.Result == nil || completed.Lease.Generation != 2 ||
		completed.Result.ContainerStarted || completed.Result.ProcessExecuted ||
		!completed.Result.TargetContainerPresent || runtimeTransport.calls != 2 ||
		stager.captureCalls != 4 || len(runtimeTransport.request.Mounts) != plan.ProjectionCount ||
		runtimeTransport.request.WritableMount.ReadOnly {
		t.Fatalf("runtime input resume did not converge: record=%#v calls=%d captures=%d err=%v",
			completed, runtimeTransport.calls, stager.captureCalls, err)
	}
	replayedApplication, err := service.ApplyDockerRuntimeInputs(ctx, applyRequest)
	if err != nil || !replayedApplication.Replayed || replayedApplication.Result == nil ||
		runtimeTransport.calls != 2 || stager.captureCalls != 4 {
		t.Fatalf("runtime input application replay touched inputs or daemon: %#v err=%v",
			replayedApplication, err)
	}
	resourceInspector := &recordingDockerRuntimeInputResourceInspector{}
	resourceCleanup := &recordingDockerRuntimeInputResourceCleanupTransport{cancel: true}
	service.WithDockerRuntimeInputResourceInspector(resourceInspector).
		WithDockerRuntimeInputResourceCleanupTransport(resourceCleanup)
	inspectionRequest := InspectDockerRuntimeInputResourcesRequest{
		ApplicationIntentID: completed.Intent.ID, Manifest: manifest,
		OperationKey: "docker-runtime-input-resource-inspection",
		RequestedBy:  "runtime_input_operator",
	}
	if _, err := service.InspectDockerRuntimeInputResources(ctx,
		inspectionRequest); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		resourceInspector.calls != 0 || stager.captureCalls != 4 {
		t.Fatalf("runtime resource inspection skipped confirmation: calls=%d captures=%d err=%v",
			resourceInspector.calls, stager.captureCalls, err)
	}
	inspectionRequest.OperatorConfirmed = true
	inspection, err := service.InspectDockerRuntimeInputResources(ctx, inspectionRequest)
	if err != nil || inspection.Replayed || !inspection.Complete ||
		!inspection.CleanupEligible || !inspection.OwnedTargetNeverStarted ||
		!inspection.AllOwnedVolumesReadOnly || !inspection.AllOwnedVolumesNoCopy ||
		inspection.ContainerStartAuthorized || inspection.ProcessExecutionAuthorized ||
		inspection.OutputExportAuthorized || inspection.ArtifactCommitAuthorized ||
		resourceInspector.calls != 1 || stager.captureCalls != 4 {
		t.Fatalf("runtime resource inspection widened authority: value=%#v calls=%d captures=%d err=%v",
			inspection, resourceInspector.calls, stager.captureCalls, err)
	}
	replayedInspection, err := service.InspectDockerRuntimeInputResources(ctx, inspectionRequest)
	if err != nil || !replayedInspection.Replayed || replayedInspection.ID != inspection.ID ||
		resourceInspector.calls != 1 || stager.captureCalls != 4 {
		t.Fatalf("runtime resource inspection replay touched inputs or daemon: %#v err=%v",
			replayedInspection, err)
	}
	changedInspection := inspectionRequest
	changedInspection.Manifest.TimeoutSeconds++
	if _, err := service.InspectDockerRuntimeInputResources(ctx,
		changedInspection); apperror.CodeOf(err) != apperror.CodeConflict ||
		resourceInspector.calls != 1 || stager.captureCalls != 4 {
		t.Fatalf("changed Manifest reused resource inspection operation: %v", err)
	}
	cleanupRequest := CleanupDockerRuntimeInputResourcesRequest{
		InspectionID: inspection.ID, Manifest: manifest,
		OperationKey: "docker-runtime-input-resource-cleanup",
		RequestedBy:  "runtime_input_operator", OwnerID: "runtime_resource_owner",
	}
	if _, err := service.CleanupDockerRuntimeInputResources(ctx,
		cleanupRequest); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		resourceCleanup.calls != 0 || stager.captureCalls != 4 {
		t.Fatalf("runtime resource cleanup skipped confirmation: calls=%d captures=%d err=%v",
			resourceCleanup.calls, stager.captureCalls, err)
	}
	intentVisibleBeforeTransport := false
	resourceCleanup.onCleanup = func(intent sandbox.DockerRuntimeInputResourceCleanupIntent) {
		durable, lookupErr := st.GetDockerRuntimeInputResourceCleanup(ctx, intent.ID)
		intentVisibleBeforeTransport = lookupErr == nil && durable.Result == nil &&
			durable.Lease.Status == sandbox.DockerRuntimeInputResourceCleanupLeaseActive
	}
	cleanupRequest.OperatorConfirmed, cleanupRequest.DaemonWriteConfirmed = true, true
	failedCleanup, err := service.CleanupDockerRuntimeInputResources(ctx, cleanupRequest)
	if apperror.CodeOf(err) != apperror.CodeCancelled || !intentVisibleBeforeTransport ||
		failedCleanup.Result != nil || len(failedCleanup.Failures) != 1 ||
		failedCleanup.Lease.Status != sandbox.DockerRuntimeInputResourceCleanupLeaseReleased ||
		failedCleanup.Failures[0].Code != sandbox.DockerRuntimeInputResourceErrorCanceled ||
		resourceCleanup.calls != 1 || stager.captureCalls != 4 {
		t.Fatalf("runtime resource cleanup failure was not recoverable: record=%#v calls=%d captures=%d err=%v",
			failedCleanup, resourceCleanup.calls, stager.captureCalls, err)
	}
	resumeCleanup := ResumeDockerRuntimeInputResourceCleanupRequest{
		IntentID: failedCleanup.Intent.ID, Manifest: manifest,
		RequestedBy: "runtime_input_operator", OwnerID: "runtime_resource_owner_2",
	}
	if _, err := service.ResumeDockerRuntimeInputResourceCleanup(ctx,
		resumeCleanup); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		resourceCleanup.calls != 1 {
		t.Fatalf("runtime resource cleanup resume skipped confirmation: %v", err)
	}
	resumeCleanup.OperatorConfirmed, resumeCleanup.DaemonWriteConfirmed = true, true
	cleaned, err := service.ResumeDockerRuntimeInputResourceCleanup(ctx, resumeCleanup)
	if err != nil || cleaned.Result == nil || cleaned.Lease.Generation != 2 ||
		cleaned.Lease.Status != sandbox.DockerRuntimeInputResourceCleanupLeaseReleased ||
		!cleaned.Result.TargetAbsent || !cleaned.Result.AllVolumesAbsent ||
		cleaned.Result.ContainerStartAuthorized || cleaned.Result.ProcessExecutionAuthorized ||
		cleaned.Result.OutputExportAuthorized || cleaned.Result.ArtifactCommitAuthorized ||
		resourceCleanup.calls != 2 || stager.captureCalls != 4 {
		t.Fatalf("runtime resource cleanup resume did not converge: record=%#v calls=%d captures=%d err=%v",
			cleaned, resourceCleanup.calls, stager.captureCalls, err)
	}
	replayedCleanup, err := service.CleanupDockerRuntimeInputResources(ctx, cleanupRequest)
	if err != nil || !replayedCleanup.Replayed || replayedCleanup.Result == nil ||
		resourceCleanup.calls != 2 || stager.captureCalls != 4 {
		t.Fatalf("runtime resource cleanup replay touched inputs or daemon: %#v err=%v",
			replayedCleanup, err)
	}
	startGateRequest := ReviewDockerStartGateRequest{
		CleanupIntentID: cleaned.Intent.ID, Manifest: manifest,
		OperationKey: "docker-start-gate-review", RequestedBy: "runtime_input_operator",
	}
	if _, err := service.ReviewDockerStartGate(ctx, startGateRequest); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		resourceCleanup.calls != 2 || resourceInspector.calls != 1 || stager.captureCalls != 4 {
		t.Fatalf("start-gate review skipped confirmation or contacted a transport: %v", err)
	}
	startGateRequest.OperatorConfirmed = true
	startGateReview, err := service.ReviewDockerStartGate(ctx, startGateRequest)
	if err != nil || startGateReview.Replayed ||
		startGateReview.Status != sandbox.DockerStartGateReviewStatusBlocked ||
		startGateReview.Decision != sandbox.DockerStartGateReviewDecisionDeny ||
		startGateReview.StartGatePassed || startGateReview.RealDaemonChainVerified ||
		startGateReview.StartImplementationPresent ||
		startGateReview.ContainerStartAuthorized || startGateReview.ProcessExecutionAuthorized ||
		startGateReview.OutputExportAuthorized || startGateReview.ArtifactCommitAuthorized ||
		len(startGateReview.Checks) != sandbox.MaxBackendChecks ||
		len(startGateReview.Lifecycle.Transitions) != sandbox.DockerStartGateLifecycleTransitionCount ||
		startGateReview.Lifecycle.ImplementationPresent ||
		startGateReview.Lifecycle.DaemonMutationEnabled ||
		resourceCleanup.calls != 2 || resourceInspector.calls != 1 || stager.captureCalls != 4 {
		t.Fatalf("start-gate review widened authority or contacted a transport: %#v err=%v",
			startGateReview, err)
	}
	replayedStartGate, err := service.ReviewDockerStartGate(ctx, startGateRequest)
	if err != nil || !replayedStartGate.Replayed || replayedStartGate.ID != startGateReview.ID ||
		resourceCleanup.calls != 2 || resourceInspector.calls != 1 || stager.captureCalls != 4 {
		t.Fatalf("start-gate replay contacted a transport: %#v err=%v", replayedStartGate, err)
	}
	changedStartGate := startGateRequest
	changedStartGate.Manifest.TimeoutSeconds++
	if _, err := service.ReviewDockerStartGate(ctx, changedStartGate); apperror.CodeOf(err) != apperror.CodeConflict ||
		resourceCleanup.calls != 2 || resourceInspector.calls != 1 || stager.captureCalls != 4 {
		t.Fatalf("changed Manifest reused start-gate operation: %v", err)
	}
	secondStartGate := startGateRequest
	secondStartGate.OperationKey = "docker-start-gate-review-second"
	if _, err := service.ReviewDockerStartGate(ctx, secondStartGate); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("cleanup accepted a second start-gate review: %v", err)
	}
	loadedStartGate, err := service.GetDockerStartGateReview(ctx, startGateReview.ID)
	if err != nil || loadedStartGate.ReviewFingerprint != startGateReview.ReviewFingerprint {
		t.Fatalf("load start-gate review: %#v err=%v", loadedStartGate, err)
	}
	listedStartGates, err := service.ListDockerStartGateReviews(ctx, run.ID, 10)
	if err != nil || len(listedStartGates) != 1 || listedStartGates[0].ID != startGateReview.ID {
		t.Fatalf("list start-gate reviews: %#v err=%v", listedStartGates, err)
	}
	productionCollector := &recordingDockerProductionEvidenceCollector{
		delegate: sandbox.NewLocalDockerProductionEvidenceCollector(),
	}
	productionCollector.before = func(request sandbox.DockerProductionEvidenceCaptureRequest) {
		record, loadErr := st.GetDockerProductionEvidenceAttempt(ctx, request.AttemptID)
		if loadErr != nil || record.Lease.Generation != request.LeaseGeneration ||
			len(record.Reconciliations) == 0 ||
			record.Reconciliations[len(record.Reconciliations)-1].Generation != request.LeaseGeneration {
			t.Fatalf("collector ran before durable generation reconciliation: %#v err=%v",
				record, loadErr)
		}
	}
	service.WithDockerProductionEvidenceCollector(productionCollector)
	productionRequest := CaptureDockerProductionEvidenceRequest{
		ReviewID: startGateReview.ID, OperationKey: "docker-production-evidence",
		RequestedBy: "runtime_input_operator",
	}
	if _, err := service.CaptureDockerProductionEvidence(ctx,
		productionRequest); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		productionCollector.calls != 0 {
		t.Fatalf("production evidence skipped confirmation: calls=%d err=%v",
			productionCollector.calls, err)
	}
	productionRequest.OperatorConfirmed = true
	productionEvidence, err := service.CaptureDockerProductionEvidence(ctx, productionRequest)
	if err != nil || productionEvidence.Replayed || productionCollector.calls != 1 ||
		productionEvidence.RequiredCheckCount != sandbox.MaxBackendChecks ||
		productionEvidence.SufficientCheckCount != 0 ||
		productionEvidence.BlockerCount != sandbox.MaxBackendChecks ||
		productionEvidence.StartGatePassed || productionEvidence.ContainerStartAuthorized ||
		productionEvidence.ProcessExecutionAuthorized ||
		productionEvidence.OutputExportAuthorized ||
		productionEvidence.ArtifactCommitAuthorized || productionEvidence.Attempt.Result == nil ||
		productionEvidence.Attempt.Lease.Generation != 1 ||
		len(productionEvidence.Attempt.Reconciliations) != 1 {
		t.Fatalf("production evidence widened authority: %#v calls=%d err=%v",
			productionEvidence, productionCollector.calls, err)
	}
	replayedProductionEvidence, err := service.CaptureDockerProductionEvidence(ctx,
		productionRequest)
	if err != nil || !replayedProductionEvidence.Replayed ||
		replayedProductionEvidence.ID != productionEvidence.ID || productionCollector.calls != 1 ||
		replayedProductionEvidence.Attempt.Result == nil {
		t.Fatalf("production evidence replay recollected: %#v calls=%d err=%v",
			replayedProductionEvidence, productionCollector.calls, err)
	}
	loadedProductionEvidence, err := service.GetDockerProductionEvidence(ctx,
		productionEvidence.ID)
	if err != nil || loadedProductionEvidence.CaptureFingerprint !=
		productionEvidence.CaptureFingerprint {
		t.Fatalf("load production evidence: %#v err=%v", loadedProductionEvidence, err)
	}
	listedProductionEvidence, err := service.ListDockerProductionEvidence(ctx, run.ID, 10)
	if err != nil || len(listedProductionEvidence) != 1 ||
		listedProductionEvidence[0].ID != productionEvidence.ID {
		t.Fatalf("list production evidence: %#v err=%v", listedProductionEvidence, err)
	}
	productionCollector.forceComplete = true
	unsafeProductionRequest := productionRequest
	unsafeProductionRequest.OperationKey = "docker-production-evidence-real-daemon"
	if _, err := service.CaptureDockerProductionEvidence(ctx,
		unsafeProductionRequest); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		productionCollector.calls != 2 {
		t.Fatalf("production evidence bypassed the write-ahead harness gate: calls=%d err=%v",
			productionCollector.calls, err)
	}
	listedProductionEvidence, err = service.ListDockerProductionEvidence(ctx, run.ID, 10)
	if err != nil || len(listedProductionEvidence) != 1 {
		t.Fatalf("rejected real-daemon evidence left state: %#v err=%v",
			listedProductionEvidence, err)
	}
	productionAttempts, err := service.ListDockerProductionEvidenceAttempts(ctx, run.ID, 10)
	if err != nil || len(productionAttempts) != 2 ||
		len(productionAttempts[1].Failures) != 1 ||
		productionAttempts[1].Failures[0].Code !=
			sandbox.DockerProductionEvidenceAttemptErrorUnsafeContact ||
		productionAttempts[1].Result != nil ||
		productionAttempts[1].Lease.Status != sandbox.DockerProductionEvidenceAttemptLeaseReleased {
		t.Fatalf("unsafe collector attempt was not durably failed: %#v err=%v",
			productionAttempts, err)
	}
	productionCollector.forceComplete = false
	resumedProductionEvidence, err := service.ResumeDockerProductionEvidence(ctx,
		ResumeDockerProductionEvidenceRequest{AttemptID: productionAttempts[1].Attempt.ID,
			RequestedBy: "runtime_input_operator", OwnerID: "recovery-worker",
			OperatorConfirmed: true})
	if err != nil || resumedProductionEvidence.Attempt.Result == nil ||
		resumedProductionEvidence.Attempt.Lease.Generation != 2 ||
		len(resumedProductionEvidence.Attempt.Reconciliations) != 2 ||
		resumedProductionEvidence.Attempt.Reconciliations[1].Status !=
			sandbox.DockerProductionEvidenceReconciliationRestart ||
		productionCollector.calls != 3 {
		t.Fatalf("production evidence attempt did not recover: %#v calls=%d err=%v",
			resumedProductionEvidence, productionCollector.calls, err)
	}
	productionHarness := &recordingDockerProductionEvidenceHarness{}
	productionHarness.beforeReconcile = func(
		request sandbox.DockerProductionEvidenceHarnessRequest,
	) {
		record, loadErr := st.GetDockerProductionEvidenceAttempt(ctx, request.AttemptID)
		if loadErr != nil || record.HarnessIntent == nil ||
			record.HarnessIntent.IntentFingerprint != request.IntentFingerprint ||
			len(record.Reconciliations) != 1 ||
			len(record.HarnessReconciliations) != 0 {
			t.Fatalf("harness reconciled before durable intent: %#v err=%v", record, loadErr)
		}
	}
	productionHarness.beforeCapture = func(
		request sandbox.DockerProductionEvidenceHarnessCaptureRequest,
	) {
		record, loadErr := st.GetDockerProductionEvidenceAttempt(ctx, request.AttemptID)
		current, found := record.CurrentHarnessReconciliation()
		if loadErr != nil || !found ||
			current.ReconciliationFingerprint != request.HarnessReconciliationFingerprint ||
			!current.RealDaemonContacted || current.DaemonReadCount != 1 ||
			current.OwnedResourceCount != 0 {
			t.Fatalf("harness captured before daemon-aware reconciliation: %#v err=%v",
				record, loadErr)
		}
	}
	service.WithDockerProductionEvidenceCollector(productionHarness)
	harnessRequest := productionRequest
	harnessRequest.OperationKey = "docker-production-evidence-linux-harness"
	harnessEvidence, err := service.CaptureDockerProductionEvidence(ctx, harnessRequest)
	if err != nil || !harnessEvidence.RealDaemonContacted ||
		harnessEvidence.Status != sandbox.DockerProductionEvidenceStatusComplete ||
		harnessEvidence.ObservedCount != sandbox.MaxBackendChecks ||
		harnessEvidence.ProductionVerifiedCount != 0 ||
		harnessEvidence.StartGatePassed || harnessEvidence.ContainerStartAuthorized ||
		harnessEvidence.ProcessExecutionAuthorized || harnessEvidence.OutputExportAuthorized ||
		harnessEvidence.ArtifactCommitAuthorized || harnessEvidence.Attempt.Result != nil ||
		harnessEvidence.Attempt.HarnessIntent == nil ||
		harnessEvidence.Attempt.HarnessResult == nil ||
		len(harnessEvidence.Attempt.HarnessReconciliations) != 1 ||
		productionHarness.reconcileCalls != 1 || productionHarness.captureCalls != 1 {
		t.Fatalf("v67 harness widened authority or skipped checkpoints: %#v reconcile=%d capture=%d err=%v",
			harnessEvidence, productionHarness.reconcileCalls,
			productionHarness.captureCalls, err)
	}
	replayedHarnessEvidence, err := service.CaptureDockerProductionEvidence(ctx,
		harnessRequest)
	if err != nil || !replayedHarnessEvidence.Replayed ||
		replayedHarnessEvidence.ID != harnessEvidence.ID ||
		productionHarness.reconcileCalls != 1 || productionHarness.captureCalls != 1 {
		t.Fatalf("v67 harness replay contacted daemon: %#v reconcile=%d capture=%d err=%v",
			replayedHarnessEvidence, productionHarness.reconcileCalls,
			productionHarness.captureCalls, err)
	}
	resourceInspector.targetState = sandbox.DockerRuntimeInputResourceTargetForeign
	unsafeRequest := inspectionRequest
	unsafeRequest.OperationKey = "docker-runtime-input-resource-inspection-after-cleanup"
	unsafeInspection, err := service.InspectDockerRuntimeInputResources(ctx, unsafeRequest)
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		unsafeInspection.Status != sandbox.DockerRuntimeInputResourceInspectionUnsafe ||
		unsafeInspection.CleanupEligible || unsafeInspection.ForeignResourceCount != 1 ||
		resourceInspector.calls != 2 || stager.captureCalls != 4 {
		t.Fatalf("unsafe runtime resource collision was not persisted and rejected: %#v err=%v",
			unsafeInspection, err)
	}
	loadedInspection, err := service.GetDockerRuntimeInputResourceInspection(ctx, inspection.ID)
	if err != nil || loadedInspection.InspectionFingerprint != inspection.InspectionFingerprint {
		t.Fatalf("load runtime resource inspection: %#v err=%v", loadedInspection, err)
	}
	listedInspections, err := service.ListDockerRuntimeInputResourceInspections(ctx, run.ID, 10)
	if err != nil || len(listedInspections) != 2 {
		t.Fatalf("list runtime resource inspections: %#v err=%v", listedInspections, err)
	}
	loadedCleanup, err := service.GetDockerRuntimeInputResourceCleanup(ctx, cleaned.Intent.ID)
	if err != nil || loadedCleanup.Result == nil {
		t.Fatalf("load runtime resource cleanup: %#v err=%v", loadedCleanup, err)
	}
	listedCleanups, err := service.ListDockerRuntimeInputResourceCleanups(ctx, run.ID, 10)
	if err != nil || len(listedCleanups) != 1 || listedCleanups[0].Intent.ID != cleaned.Intent.ID {
		t.Fatalf("list runtime resource cleanups: %#v err=%v", listedCleanups, err)
	}
	loadedApplication, err := service.GetDockerRuntimeInputApplication(ctx, failed.Intent.ID)
	if err != nil || loadedApplication.Result == nil {
		t.Fatalf("load runtime input application: %#v err=%v", loadedApplication, err)
	}
	listedApplications, err := service.ListDockerRuntimeInputApplications(ctx, run.ID, 10)
	if err != nil || len(listedApplications) != 1 ||
		listedApplications[0].Intent.ID != failed.Intent.ID {
		t.Fatalf("list runtime input applications: %#v err=%v", listedApplications, err)
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
		stager.captureCalls != 4 {
		t.Fatalf("runtime projection replay recaptured input: %#v err=%v", replayed, err)
	}
	changed := request
	changed.Manifest.TimeoutSeconds++
	if _, err := service.PlanDockerRuntimeInputs(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict || stager.captureCalls != 4 {
		t.Fatalf("changed Manifest reused runtime projection operation: %v", err)
	}
	otherOperation := request
	otherOperation.OperationKey = "docker-runtime-input-projection-other"
	if _, err := service.PlanDockerRuntimeInputs(ctx, otherOperation); apperror.CodeOf(err) != apperror.CodeConflict || stager.captureCalls != 4 {
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
