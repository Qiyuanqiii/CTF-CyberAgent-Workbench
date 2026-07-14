package sandbox

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

type dockerContainerCompilerTransport struct {
	imageDigest string
	pids        bool
	ncpu        int
	memory      int64
}

func (transport dockerContainerCompilerTransport) Endpoint() DockerObservationEndpoint {
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	return endpoint
}

func (dockerContainerCompilerTransport) Ping(context.Context) error { return nil }

func (dockerContainerCompilerTransport) Version(context.Context) (DockerDaemonVersion, error) {
	return DockerDaemonVersion{APIVersion: "1.47", MinAPIVersion: "1.24",
		EngineVersion: "27.5.1", GitCommit: "abc123", OSType: "linux",
		Architecture: "amd64"}, nil
}

func (transport dockerContainerCompilerTransport) Info(context.Context) (DockerDaemonInfo, error) {
	return DockerDaemonInfo{ID: "daemon", Name: "host", DockerRootDir: "/var/lib/docker",
		ServerVersion: "27.5.1", OSType: "linux", Architecture: "amd64",
		Driver: "overlay2", CgroupDriver: "systemd", CgroupVersion: "2",
		DefaultRuntime: "runc", NCPU: transport.ncpu, MemoryBytes: transport.memory,
		PidsLimit: transport.pids, SecurityOptions: []string{"name=rootless"}}, nil
}

func (transport dockerContainerCompilerTransport) InspectImage(context.Context,
	string,
) (DockerImageInspection, error) {
	return DockerImageInspection{ID: "sha256:" + strings.Repeat("f", 64),
		RepoDigests: []string{"example.invalid/compiler@" + transport.imageDigest},
		OSType:      "linux", Architecture: "amd64", SizeBytes: 4096,
		User: "root", RootFSType: "layers", GraphDriver: "overlay2"}, nil
}

func TestDockerContainerCompilerIsDeterministicAndFixesSecurityControls(t *testing.T) {
	ctx := context.Background()
	manifest := dockerContainerCompilerManifest()
	observation := dockerContainerCompilerObservation(t, ctx, manifest, true, 8,
		8*1024*1024*1024)
	first, err := CompileDockerContainerSpec(ctx, observation, manifest)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileDockerContainerSpec(ctx, observation, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.SpecFingerprint == "" {
		t.Fatalf("Docker compiler is not deterministic: first=%#v second=%#v", first, second)
	}
	if first.User != DockerContainerFixedUser || !first.ReadOnlyRootFS ||
		!first.NoNewPrivileges || !first.DropAllCapabilities || !first.InitEnabled ||
		len(first.Mounts) != 2 || first.Mounts[0].Access != MountReadWrite ||
		!first.Mounts[0].DedicatedOutput || first.Mounts[1].Access != MountReadOnly ||
		!first.Mounts[1].InputReadOnly || first.Mounts[0].Propagation != DockerMountPropagationPrivate ||
		first.Network.Driver != DockerNetworkDriverManagedEgress ||
		!first.Network.DefaultDeny || !first.Network.ExactAllowlist ||
		!first.Network.GuardRequired || first.SecretMountTarget != DockerSecretMountTarget ||
		!first.SecretsEphemeral || !first.SecretsMetadataExcluded ||
		first.Termination.GracefulSignal != DockerTerminationSignalGraceful ||
		first.Termination.ForcedSignal != DockerTerminationSignalForced ||
		!first.ReconcileBeforeCreate || !first.RemoveOnRollback {
		t.Fatalf("Docker compiler omitted a fixed security control: %#v", first)
	}
	if first.Environment[1].Source != EnvironmentSecretRef ||
		first.Environment[1].LiteralValue != "" ||
		first.Environment[1].SecretPath != DockerSecretMountTarget+"/001" {
		t.Fatalf("Docker secret was not converted to an ephemeral file plan: %#v", first.Environment)
	}
	if first.Resources.NanoCPUs != 1_000_000_000 || first.InputArtifactCount != 1 ||
		first.OutputCount != 3 {
		t.Fatalf("Docker compiler metadata changed: %#v", first)
	}

	transaction := NewInMemoryDockerWriteTransaction()
	result, err := transaction.Simulate(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewDockerContainerPlan("docker-plan-one", observation, first, result,
		"compiler_operator", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Controls) != MaxBackendChecks || len(plan.Transaction.Steps) != MaxDockerWriteSteps ||
		!plan.SimulationOnly || plan.ProductionSubmitted || plan.ProductionVerified ||
		plan.BackendAvailable || plan.BackendEnabled || plan.ExecutionAuthorized ||
		plan.ArtifactCommitAuthorized {
		t.Fatalf("Docker plan widened fake transaction authority: %#v", plan)
	}
}

func TestDockerContainerCompilerRejectsUnsafeOrUnsupportedPlans(t *testing.T) {
	ctx := context.Background()
	manifest := dockerContainerCompilerManifest()
	observation := dockerContainerCompilerObservation(t, ctx, manifest, true, 8,
		8*1024*1024*1024)

	withoutOutputMount := manifest
	withoutOutputMount.Mounts = withoutOutputMount.Mounts[1:]
	withoutOutputMount.Output.Paths = nil
	observationForChanged := dockerContainerCompilerObservation(t, ctx, withoutOutputMount, true, 8,
		8*1024*1024*1024)
	if _, err := CompileDockerContainerSpec(ctx, observationForChanged, withoutOutputMount); err == nil ||
		!strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("Docker compiler accepted no dedicated output mount: %v", err)
	}

	twoOutputs := manifest
	twoOutputs.Mounts = append(append([]Mount(nil), manifest.Mounts...), Mount{
		Source: "more-output", Target: "/more-output", Access: MountReadWrite,
	})
	twoOutputs.Output.Paths = append(twoOutputs.Output.Paths, "/more-output/result")
	if _, err := NormalizeManifest(twoOutputs); err == nil {
		changedObservation := dockerContainerCompilerObservation(t, ctx, twoOutputs, true, 8,
			8*1024*1024*1024)
		if _, err := CompileDockerContainerSpec(ctx, changedObservation, twoOutputs); err == nil {
			t.Fatal("Docker compiler accepted two writable output mounts")
		}
	}

	unsupported := dockerContainerCompilerObservation(t, ctx, manifest, false, 8,
		8*1024*1024*1024)
	if _, err := CompileDockerContainerSpec(ctx, unsupported, manifest); err == nil {
		t.Fatal("Docker compiler accepted a daemon without PID limits")
	}

	overCapacity := manifest
	overCapacity.Resources.CPUQuotaMillis = 2000
	overCapacityObservation := dockerContainerCompilerObservation(t, ctx, overCapacity, true, 1,
		8*1024*1024*1024)
	if _, err := CompileDockerContainerSpec(ctx, overCapacityObservation, overCapacity); err == nil ||
		!strings.Contains(err.Error(), "CPU") {
		t.Fatalf("Docker compiler accepted an over-capacity CPU plan: %v", err)
	}

	tampered := observation
	tampered.Report.ProductionVerified = true
	if _, err := CompileDockerContainerSpec(ctx, tampered, manifest); err == nil {
		t.Fatal("Docker compiler accepted a v53 authority claim")
	}
	changedManifest := manifest
	changedManifest.TimeoutSeconds++
	if _, err := CompileDockerContainerSpec(ctx, observation, changedManifest); err == nil {
		t.Fatal("Docker compiler accepted a changed Manifest")
	}
}

func TestInMemoryDockerWriteTransactionRollsBackFailureCrashAndCancellation(t *testing.T) {
	ctx := context.Background()
	manifest := dockerContainerCompilerManifest()
	observation := dockerContainerCompilerObservation(t, ctx, manifest, true, 8,
		8*1024*1024*1024)
	spec, err := CompileDockerContainerSpec(ctx, observation, manifest)
	if err != nil {
		t.Fatal(err)
	}

	failing := NewInMemoryDockerWriteTransaction()
	failing.FailAtOrdinal = 3
	if _, err := failing.Simulate(ctx, spec); err == nil || len(failing.Snapshot()) != 0 {
		t.Fatalf("failed fake transaction published staged steps: snapshot=%#v err=%v",
			failing.Snapshot(), err)
	}
	crashing := NewInMemoryDockerWriteTransaction()
	crashing.CrashAtOrdinal = 5
	if _, err := crashing.Simulate(ctx, spec); err == nil || len(crashing.Snapshot()) != 0 {
		t.Fatalf("crashed fake transaction published staged steps: snapshot=%#v err=%v",
			crashing.Snapshot(), err)
	}
	cancelled := NewInMemoryDockerWriteTransaction()
	cancelCtx, cancel := context.WithCancel(ctx)
	cancelled.beforeStep = func(ordinal int) {
		if ordinal == 4 {
			cancel()
		}
	}
	if _, err := cancelled.Simulate(cancelCtx, spec); err == nil ||
		len(cancelled.Snapshot()) != 0 {
		t.Fatalf("cancelled fake transaction published staged steps: snapshot=%#v err=%v",
			cancelled.Snapshot(), err)
	}

	success := NewInMemoryDockerWriteTransaction()
	result, err := success.Simulate(ctx, spec)
	if err != nil || len(success.Snapshot()) != 1 || result.CommittedStepCount != 7 ||
		result.DaemonWriteCount != 0 || result.BackendTouched || result.ProductionSubmitted {
		t.Fatalf("successful fake transaction is invalid: result=%#v snapshot=%#v err=%v",
			result, success.Snapshot(), err)
	}
	snapshot := success.Snapshot()
	snapshot[0].Steps[0].State = "mutated_by_caller"
	if success.Snapshot()[0].Steps[0].State == "mutated_by_caller" {
		t.Fatal("fake transaction snapshot exposes mutable committed steps")
	}
}

func dockerContainerCompilerManifest() Manifest {
	return Manifest{
		ProtocolVersion: ManifestProtocolVersion, Backend: BackendDocker,
		Command: CommandSpec{Executable: "go", Arguments: []string{"test", "./..."},
			WorkingDirectory: "/workspace"},
		Mounts: []Mount{
			{Source: "output", Target: "/output", Access: MountReadWrite},
			{Source: "src", Target: "/workspace", Access: MountReadOnly},
		},
		Network: NetworkScope{Mode: "allowlist",
			AllowedTargets: []string{"198.51.100.8:443", "example.invalid:443"}},
		Resources: ResourceLimits{CPUQuotaMillis: 1000, MemoryBytes: 256 * 1024 * 1024,
			PIDs: 64, MaxOutputBytes: 4 * 1024 * 1024},
		Environment: []EnvironmentBinding{
			{Name: "GOFLAGS", Source: EnvironmentLiteral, Value: "-mod=readonly"},
			{Name: "SERVICE_TOKEN", Source: EnvironmentSecretRef, Value: "service-token"},
		},
		InputArtifactIDs: []string{"artifact-input-one"},
		Output: OutputSpec{CaptureStdout: true, CaptureStderr: true,
			Paths: []string{"/output/report.json"}},
		TimeoutSeconds: 300, Cancellation: CancellationSpec{GracePeriodMillis: 2000},
	}
}

func dockerContainerCompilerObservation(t *testing.T, ctx context.Context, manifest Manifest,
	pids bool, ncpu int, memory int64,
) DockerObservation {
	t.Helper()
	normalized, err := NormalizeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestFingerprint, err := normalized.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	outputPlan, err := NewOutputExportPlan(normalized)
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	imageDigest := "sha256:" + strings.Repeat("d", 64)
	observation := DockerObservation{
		ID: "docker-observation-one", EvidenceID: "docker-evidence-one",
		OutputSimulationID: "docker-output-simulation-one", PreflightID: "docker-preflight-one",
		ExecutionID: "docker-execution-one", CandidateID: "docker-candidate-one",
		PreparationID: "docker-preparation-one", RunID: "docker-run-one",
		MissionID: "docker-mission-one", WorkspaceID: "docker-workspace-one",
		ManifestFingerprint: manifestFingerprint, AuthorizationFingerprint: digest,
		PolicyFingerprint: digest, MountBindingFingerprint: digest,
		InputArtifactDigest: digest, ThreatModelFingerprint: digest,
		OutputPlanFingerprint: outputPlan.Fingerprint,
		Report:                DockerObservationReport{ImageDigest: imageDigest},
		RequestedBy:           "compiler_operator", CreatedAt: time.Now().UTC(),
	}
	observer := NewReadOnlyDockerProductionObserver(dockerContainerCompilerTransport{
		imageDigest: imageDigest, pids: pids, ncpu: ncpu, memory: memory,
	})
	report, err := observer.Observe(ctx, DockerObservationProbeRequest{
		BindingFingerprint: DockerObservationBindingFingerprint(observation),
		ImageDigest:        imageDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation.Report = report
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	return observation
}
