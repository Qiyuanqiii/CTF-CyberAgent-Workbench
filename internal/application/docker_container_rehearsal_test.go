package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
)

type countingDockerWriteTransport struct {
	calls        int
	cleanupCalls int
	mutate       func(*sandbox.DockerContainerStageResult)
	err          error
}

type recoveringDockerWriteTransport struct {
	stageCalls    int
	createWrites  int
	cleanupWrites int
}

type recordingDockerHostInputStager struct {
	probeCalls int
	stageCalls int
	request    sandbox.HostInputBundleRequest
	probeErr   error
	stageErr   error
	mutate     func(*sandbox.HostInputBundleReport)
}

func (stager *recordingDockerHostInputStager) Probe(_ context.Context, _ string) error {
	stager.probeCalls++
	return stager.probeErr
}

func (stager *recordingDockerHostInputStager) Stage(_ context.Context,
	request sandbox.HostInputBundleRequest,
) (sandbox.HostInputBundleReport, error) {
	stager.stageCalls++
	stager.request = request
	if stager.stageErr != nil {
		return sandbox.HostInputBundleReport{}, stager.stageErr
	}
	report, err := sandbox.NewHostInputBundleReport(sandbox.HostInputBundleMeasurements{
		ReadOnlyMountCount: request.ReadOnlyMountCount(), ArtifactCount: len(request.Artifacts),
		DirectoryCount: request.ReadOnlyMountCount(), ArtifactBytes: request.ArtifactBytes(),
		BundleBytes: 4096, SourceSnapshotDigest: strings.Repeat("a", 64),
		ArtifactPayloadDigest: request.ArtifactPayloadDigest(),
		BundleDigest:          strings.Repeat("b", 64),
	}, time.Now().UTC())
	if err == nil && stager.mutate != nil {
		stager.mutate(&report)
	}
	return report, err
}

func (transport *recoveringDockerWriteTransport) Endpoint() sandbox.DockerObservationEndpoint {
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return endpoint
}

func (transport *recoveringDockerWriteTransport) Rehearse(_ context.Context,
	request sandbox.DockerContainerWriteRequest,
) (sandbox.DockerContainerWriteResult, error) {
	return sandbox.NewDockerContainerWriteResult(transport.Endpoint(), request,
		strings.Repeat("d", 64), 0)
}

func (transport *recoveringDockerWriteTransport) Stage(_ context.Context,
	request sandbox.DockerContainerWriteRequest,
) (sandbox.DockerContainerStageResult, error) {
	transport.stageCalls++
	if transport.stageCalls == 1 {
		transport.createWrites++
		return sandbox.DockerContainerStageResult{}, context.Canceled
	}
	return sandbox.NewDockerContainerStageResult(transport.Endpoint(), request,
		strings.Repeat("d", 64), true)
}

func (transport *recoveringDockerWriteTransport) Cleanup(_ context.Context,
	request sandbox.DockerContainerWriteRequest, stage sandbox.DockerContainerStageResult,
) (sandbox.DockerContainerCleanupResult, error) {
	transport.cleanupWrites++
	return sandbox.NewDockerContainerCleanupResult(transport.Endpoint(), request, stage, true)
}

func (transport *countingDockerWriteTransport) Endpoint() sandbox.DockerObservationEndpoint {
	endpoint, _ := sandbox.NewDockerObservationEndpoint(sandbox.DockerObservationEndpointLocalUnix)
	return endpoint
}

func (transport *countingDockerWriteTransport) Rehearse(_ context.Context,
	request sandbox.DockerContainerWriteRequest,
) (sandbox.DockerContainerWriteResult, error) {
	transport.calls++
	if transport.err != nil {
		return sandbox.DockerContainerWriteResult{}, transport.err
	}
	result, err := sandbox.NewDockerContainerWriteResult(transport.Endpoint(), request,
		strings.Repeat("c", 64), 0)
	return result, err
}

func (transport *countingDockerWriteTransport) Stage(_ context.Context,
	request sandbox.DockerContainerWriteRequest,
) (sandbox.DockerContainerStageResult, error) {
	transport.calls++
	if transport.err != nil {
		return sandbox.DockerContainerStageResult{}, transport.err
	}
	result, err := sandbox.NewDockerContainerStageResult(transport.Endpoint(), request,
		strings.Repeat("c", 64), false)
	if err == nil && transport.mutate != nil {
		transport.mutate(&result)
	}
	return result, err
}

func (transport *countingDockerWriteTransport) Cleanup(_ context.Context,
	request sandbox.DockerContainerWriteRequest, stage sandbox.DockerContainerStageResult,
) (sandbox.DockerContainerCleanupResult, error) {
	transport.cleanupCalls++
	return sandbox.NewDockerContainerCleanupResult(transport.Endpoint(), request, stage, true)
}

func TestDockerContainerRehearsalRequiresConfirmationRevalidatesAndReplays(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, observation := prepareDockerContainerPlanAuthority(t, ctx, service, run.ID,
		root, "docker-rehearsal", "docker_rehearsal_operator")
	plan, err := service.CompileDockerContainerPlan(ctx, CompileDockerContainerPlanRequest{
		ObservationID: observation.ID, Manifest: manifest,
		OperationKey: "docker-rehearsal-plan", RequestedBy: "docker_rehearsal_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := RehearseDockerContainerRequest{PlanID: plan.ID, Manifest: manifest,
		OperationKey: "docker-rehearsal-operation", RequestedBy: "docker_rehearsal_operator"}
	if _, err := service.RehearseDockerContainer(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("Docker rehearsal did not require explicit confirmation: %v", err)
	}
	request.OperatorConfirmed = true
	if _, err := service.RehearseDockerContainer(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		sandbox.DockerContainerWriteErrorCode(err) != sandbox.DockerContainerWriteFailureDisabled {
		t.Fatalf("default-disabled Docker transport was not fail-closed: %v", err)
	}
	values, err := service.ListDockerContainerRehearsals(ctx, run.ID, 10)
	if err != nil || len(values) != 0 {
		t.Fatalf("disabled transport left a durable rehearsal: %#v err=%v", values, err)
	}

	transport := &countingDockerWriteTransport{}
	service.WithDockerContainerWriteTransport(transport)
	rehearsal, err := service.RehearseDockerContainer(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if rehearsal.Replayed || transport.calls != 1 || rehearsal.DaemonWriteCount != 2 ||
		!rehearsal.ContainerNeverStarted || !rehearsal.ProcessNeverExecuted ||
		!rehearsal.ImageNeverPulled || !rehearsal.OutputNeverExported ||
		rehearsal.ProductionExecutionSubmitted || rehearsal.ProductionVerified ||
		rehearsal.BackendEnabled || rehearsal.ExecutionAuthorized ||
		rehearsal.ArtifactCommitAuthorized {
		t.Fatalf("Docker rehearsal widened authority: value=%#v calls=%d", rehearsal, transport.calls)
	}
	replayed, err := service.RehearseDockerContainer(ctx, request)
	if err != nil || !replayed.Replayed || replayed.ID != rehearsal.ID || transport.calls != 1 {
		t.Fatalf("Docker rehearsal replay contacted transport: value=%#v calls=%d err=%v",
			replayed, transport.calls, err)
	}
	loaded, err := service.GetDockerContainerRehearsal(ctx, rehearsal.ID)
	if err != nil || loaded.RehearsalFingerprint != rehearsal.RehearsalFingerprint {
		t.Fatalf("load Docker rehearsal: %#v err=%v", loaded, err)
	}
	values, err = service.ListDockerContainerRehearsals(ctx, run.ID, 10)
	if err != nil || len(values) != 1 || values[0].ID != rehearsal.ID {
		t.Fatalf("list Docker rehearsals: %#v err=%v", values, err)
	}
	changed := request
	changed.Manifest.TimeoutSeconds++
	if _, err := service.RehearseDockerContainer(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict ||
		transport.calls != 1 {
		t.Fatalf("changed Manifest reused rehearsal operation: calls=%d err=%v", transport.calls, err)
	}
}

func TestDockerContainerRehearsalRejectsTransportClaimsAndCancelledAuthority(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, observation := prepareDockerContainerPlanAuthority(t, ctx, service, run.ID,
		root, "docker-rehearsal-claim", "docker_rehearsal_operator")
	plan, err := service.CompileDockerContainerPlan(ctx, CompileDockerContainerPlanRequest{
		ObservationID: observation.ID, Manifest: manifest,
		OperationKey: "docker-rehearsal-claim-plan", RequestedBy: "docker_rehearsal_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	claiming := &countingDockerWriteTransport{mutate: func(result *sandbox.DockerContainerStageResult) {
		result.ProcessExecuted = true
	}}
	service.WithDockerContainerWriteTransport(claiming)
	request := RehearseDockerContainerRequest{PlanID: plan.ID, Manifest: manifest,
		OperationKey: "docker-rehearsal-claim-operation", RequestedBy: "docker_rehearsal_operator",
		OperatorConfirmed: true}
	if _, err := service.RehearseDockerContainer(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		claiming.calls != 1 {
		t.Fatalf("unsupported transport claim was accepted: calls=%d err=%v", claiming.calls, err)
	}
	values, err := service.ListDockerContainerRehearsals(ctx, run.ID, 10)
	if err != nil || len(values) != 0 {
		t.Fatalf("unsupported claim left a durable rehearsal: %#v err=%v", values, err)
	}

	if _, err := service.CancelDisabledExecution(ctx, CancelSandboxExecutionRequest{
		ExecutionID: observation.ExecutionID, OperationKey: "docker-rehearsal-cancel",
		RequestedBy: "docker_rehearsal_operator",
	}); err != nil {
		t.Fatal(err)
	}
	safe := &countingDockerWriteTransport{}
	service.WithDockerContainerWriteTransport(safe)
	request.OperationKey = "docker-rehearsal-after-cancel"
	if _, err := service.RehearseDockerContainer(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		safe.calls != 0 {
		t.Fatalf("cancelled authority reached Docker write transport: calls=%d err=%v", safe.calls, err)
	}
}

func TestDockerContainerRehearsalRecoversUncertainCreateWithoutCreatingTwice(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, observation := prepareDockerContainerPlanAuthority(t, ctx, service, run.ID,
		root, "docker-rehearsal-recovery", "docker_rehearsal_operator")
	plan, err := service.CompileDockerContainerPlan(ctx, CompileDockerContainerPlanRequest{
		ObservationID: observation.ID, Manifest: manifest,
		OperationKey: "docker-rehearsal-recovery-plan",
		RequestedBy:  "docker_rehearsal_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &recoveringDockerWriteTransport{}
	service.WithDockerContainerWriteTransport(transport)
	request := RehearseDockerContainerRequest{PlanID: plan.ID, Manifest: manifest,
		OperationKey: "docker-rehearsal-recovery-operation",
		RequestedBy:  "docker_rehearsal_operator", OperatorConfirmed: true}
	if _, err := service.RehearseDockerContainer(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition ||
		!errors.Is(err, context.Canceled) {
		t.Fatalf("uncertain Docker create did not remain resumable: %v", err)
	}
	pending, err := service.ListDockerContainerRehearsalAttempts(ctx, run.ID, 10)
	if err != nil || len(pending) != 1 || len(pending[0].Failures) != 1 ||
		pending[0].Lease.Status != sandbox.DockerContainerAttemptLeaseReleased {
		t.Fatalf("uncertain Docker create was not durably recoverable: %#v err=%v", pending, err)
	}
	rehearsal, err := service.ResumeDockerContainerRehearsal(ctx,
		ResumeDockerContainerRequest{AttemptID: pending[0].Intent.ID, Manifest: manifest,
			RequestedBy: "docker_rehearsal_operator", OperatorConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	if transport.stageCalls != 2 || transport.createWrites != 1 ||
		transport.cleanupWrites != 1 || rehearsal.Replayed ||
		rehearsal.DaemonWriteCount != 2 || !rehearsal.ContainerNeverStarted ||
		!rehearsal.ProcessNeverExecuted {
		t.Fatalf("Docker recovery duplicated mutation or widened authority: rehearsal=%#v transport=%#v",
			rehearsal, transport)
	}
	attempts, err := service.ListDockerContainerRehearsalAttempts(ctx, run.ID, 10)
	if err != nil || len(attempts) != 1 || attempts[0].Lease.Generation != 2 ||
		attempts[0].Status != sandbox.DockerContainerAttemptStatusCompleted ||
		len(attempts[0].Failures) != 1 || attempts[0].Stage == nil ||
		!attempts[0].Stage.Result.ExistingContainerAdopted ||
		attempts[0].Stage.Result.ContainerCreatedNow {
		t.Fatalf("Docker recovery ledger is invalid: attempts=%#v err=%v", attempts, err)
	}
}

func TestDockerHostInputStagingRequiresSeparateConfirmationAndPersistsMetadataOnly(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, observation := prepareDockerContainerPlanAuthority(t, ctx, service, run.ID,
		root, "docker-host-input", "docker_rehearsal_operator")
	plan, err := service.CompileDockerContainerPlan(ctx, CompileDockerContainerPlanRequest{
		ObservationID: observation.ID, Manifest: manifest,
		OperationKey: "docker-host-input-plan", RequestedBy: "docker_rehearsal_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &countingDockerWriteTransport{}
	stager := &recordingDockerHostInputStager{}
	service.WithDockerContainerWriteTransport(transport).WithDockerHostInputStager(stager)
	request := RehearseDockerContainerRequest{PlanID: plan.ID, Manifest: manifest,
		OperationKey: "docker-host-input-operation", RequestedBy: "docker_rehearsal_operator",
		OperatorConfirmed: true, StageHostInputs: true}
	if _, err := service.RehearseDockerContainer(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition || transport.calls != 0 ||
		stager.probeCalls != 0 {
		t.Fatalf("host input staging did not require independent confirmation: %v", err)
	}
	request.OperatorConfirmedHostInputStaging = true
	rehearsal, err := service.RehearseDockerContainer(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if rehearsal.Replayed || transport.calls != 1 || transport.cleanupCalls != 1 ||
		stager.probeCalls != 1 || stager.stageCalls != 1 ||
		stager.request.WorkspaceRoot != root || len(stager.request.Artifacts) != 0 {
		t.Fatalf("host input staging execution diverged: rehearsal=%#v transport=%#v stager=%#v",
			rehearsal, transport, stager)
	}
	records, err := service.ListDockerHostInputStagings(ctx, run.ID, 10)
	if err != nil || len(records) != 1 || records[0].Staging == nil ||
		records[0].Intent.PlanID != plan.ID || records[0].Staging.Report.DaemonConsumed ||
		records[0].Staging.Report.ContainerStarted || records[0].Staging.Report.ProcessExecuted ||
		records[0].Staging.ExecutionAuthorized {
		t.Fatalf("host input staging ledger widened authority: %#v err=%v", records, err)
	}
	loaded, err := service.GetDockerHostInputStaging(ctx, records[0].Intent.ID)
	if err != nil || loaded.Staging == nil ||
		loaded.Staging.StagingFingerprint != records[0].Staging.StagingFingerprint {
		t.Fatalf("load host input staging: %#v err=%v", loaded, err)
	}
	replayRequest := request
	replayRequest.StageHostInputs = false
	replayRequest.OperatorConfirmedHostInputStaging = false
	replayed, err := service.RehearseDockerContainer(ctx, replayRequest)
	if err != nil || !replayed.Replayed || transport.calls != 1 || stager.stageCalls != 1 {
		t.Fatalf("durable host input requirement replay repeated side effects: %#v err=%v",
			replayed, err)
	}
	events, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if strings.Contains(event.PayloadJSON, root) ||
			strings.Contains(event.PayloadJSON, stager.request.WorkspaceRoot) {
			t.Fatalf("host input staging event leaked a host path: %#v", event)
		}
	}
}

func TestDockerHostInputRequirementCannotWidenCompletedAttempt(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, observation := prepareDockerContainerPlanAuthority(t, ctx, service, run.ID,
		root, "docker-host-input-no-widen", "docker_rehearsal_operator")
	plan, err := service.CompileDockerContainerPlan(ctx, CompileDockerContainerPlanRequest{
		ObservationID: observation.ID, Manifest: manifest,
		OperationKey: "docker-host-input-no-widen-plan",
		RequestedBy:  "docker_rehearsal_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &countingDockerWriteTransport{}
	stager := &recordingDockerHostInputStager{}
	service.WithDockerContainerWriteTransport(transport).WithDockerHostInputStager(stager)
	request := RehearseDockerContainerRequest{PlanID: plan.ID, Manifest: manifest,
		OperationKey: "docker-host-input-no-widen-operation",
		RequestedBy:  "docker_rehearsal_operator", OperatorConfirmed: true}
	if _, err := service.RehearseDockerContainer(ctx, request); err != nil {
		t.Fatal(err)
	}
	request.StageHostInputs = true
	request.OperatorConfirmedHostInputStaging = true
	if _, err := service.RehearseDockerContainer(ctx, request); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("completed attempt widened a durable false host input requirement: %v", err)
	}
	if transport.calls != 1 || transport.cleanupCalls != 1 ||
		stager.probeCalls != 0 || stager.stageCalls != 0 {
		t.Fatalf("requirement widening reached a side effect: transport=%#v stager=%#v",
			transport, stager)
	}
	attempts, err := service.ListDockerContainerRehearsalAttempts(ctx, run.ID, 10)
	if err != nil || len(attempts) != 1 || attempts[0].HostInputRequirement == nil ||
		attempts[0].HostInputRequirement.Required ||
		attempts[0].HostInputRequirement.OperatorConfirmed {
		t.Fatalf("durable false host input requirement changed: %#v err=%v", attempts, err)
	}
}

func TestDockerHostInputStagingMismatchCleansAndResumesByOperationWithoutAnotherCreate(t *testing.T) {
	ctx := context.Background()
	st, run, root := newSandboxManifestTestRuntime(t, ctx)
	service := NewSandboxManifestService(st, policy.NewDefaultChecker())
	manifest, observation := prepareDockerContainerPlanAuthority(t, ctx, service, run.ID,
		root, "docker-host-input-recovery", "docker_rehearsal_operator")
	plan, err := service.CompileDockerContainerPlan(ctx, CompileDockerContainerPlanRequest{
		ObservationID: observation.ID, Manifest: manifest,
		OperationKey: "docker-host-input-recovery-plan",
		RequestedBy:  "docker_rehearsal_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	transport := &countingDockerWriteTransport{}
	stager := &recordingDockerHostInputStager{mutate: func(report *sandbox.HostInputBundleReport) {
		report.ArtifactBytes++
	}}
	service.WithDockerContainerWriteTransport(transport).WithDockerHostInputStager(stager)
	request := RehearseDockerContainerRequest{PlanID: plan.ID, Manifest: manifest,
		OperationKey: "docker-host-input-recovery-operation",
		RequestedBy:  "docker_rehearsal_operator", OperatorConfirmed: true,
		StageHostInputs: true, OperatorConfirmedHostInputStaging: true}
	if _, err := service.RehearseDockerContainer(ctx, request); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("mismatched host input evidence was accepted: %v", err)
	}
	attempts, err := service.ListDockerContainerRehearsalAttempts(ctx, run.ID, 10)
	if err != nil || len(attempts) != 1 || attempts[0].Stage == nil ||
		attempts[0].Cleanup == nil || len(attempts[0].Failures) != 1 ||
		attempts[0].HostInputRequirement == nil ||
		!attempts[0].HostInputRequirement.Required ||
		!attempts[0].HostInputRequirement.OperatorConfirmed ||
		attempts[0].Lease.Status != sandbox.DockerContainerAttemptLeaseReleased ||
		transport.calls != 1 || transport.cleanupCalls != 1 {
		t.Fatalf("failed host input staging was not safely recoverable: %#v err=%v", attempts, err)
	}
	records, err := service.ListDockerHostInputStagings(ctx, run.ID, 10)
	if err != nil || len(records) != 1 || records[0].Staging != nil {
		t.Fatalf("failed host input staging did not preserve pending intent: %#v err=%v",
			records, err)
	}
	stager.mutate = nil
	ambiguous := request
	ambiguous.OperatorConfirmedHostInputStaging = false
	if _, err := service.RehearseDockerContainer(ctx, ambiguous); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("durable requirement accepted an unmatched staging flag pair: %v", err)
	}
	unchanged, err := service.ListDockerContainerRehearsalAttempts(ctx, run.ID, 10)
	if err != nil || len(unchanged) != 1 || unchanged[0].Lease.Generation != 1 ||
		len(unchanged[0].Failures) != 1 || transport.calls != 1 || stager.stageCalls != 1 {
		t.Fatalf("ambiguous replay consumed durable recovery state: %#v err=%v", unchanged, err)
	}
	recovery := request
	recovery.StageHostInputs = false
	recovery.OperatorConfirmedHostInputStaging = false
	rehearsal, err := service.RehearseDockerContainer(ctx, recovery)
	if err != nil {
		t.Fatal(err)
	}
	if rehearsal.Replayed || transport.calls != 1 || transport.cleanupCalls != 1 ||
		stager.stageCalls != 2 {
		t.Fatalf("operation-key host input recovery repeated Docker mutation: rehearsal=%#v transport=%#v stager=%#v",
			rehearsal, transport, stager)
	}
	resumedAttempts, err := service.ListDockerContainerRehearsalAttempts(ctx, run.ID, 10)
	if err != nil || len(resumedAttempts) != 1 || len(resumedAttempts[0].Failures) != 1 ||
		resumedAttempts[0].Lease.Generation != 2 ||
		resumedAttempts[0].Status != sandbox.DockerContainerAttemptStatusCompleted {
		t.Fatalf("durable host input requirement did not drive recovery: %#v err=%v",
			resumedAttempts, err)
	}
	records, err = service.ListDockerHostInputStagings(ctx, run.ID, 10)
	if err != nil || len(records) != 1 || records[0].Staging == nil ||
		records[0].Staging.LeaseGeneration != 2 {
		t.Fatalf("host input staging recovery did not commit on takeover: %#v err=%v", records, err)
	}
}
