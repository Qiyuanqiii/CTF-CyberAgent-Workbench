package application

import (
	"context"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/sandbox"
)

type countingDockerWriteTransport struct {
	calls  int
	mutate func(*sandbox.DockerContainerWriteResult)
	err    error
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
	if err == nil && transport.mutate != nil {
		transport.mutate(&result)
	}
	return result, err
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
	claiming := &countingDockerWriteTransport{mutate: func(result *sandbox.DockerContainerWriteResult) {
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
