package sandbox

import (
	"strings"
	"testing"
	"time"
)

func TestDisabledExecutionProtocolIsDeterministicAndFailClosed(t *testing.T) {
	manifest, err := NormalizeManifest(validManifest())
	if err != nil {
		t.Fatal(err)
	}
	plan := NewOutputCapturePlan(manifest)
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	inputs := []InputArtifactBinding{{
		ExecutionID: "execution-one", Ordinal: 1, ArtifactID: "artifact-one",
		SHA256: strings.Repeat("a", 64), SizeBytes: 12, MIME: "text/plain; charset=utf-8",
		Stream: "stdout", SourceID: "source-one", Redacted: true,
	}}
	if err := inputs[0].Validate(); err != nil {
		t.Fatal(err)
	}
	digest := InputArtifactBindingsDigest(inputs)
	if digest != InputArtifactBindingsDigest(append([]InputArtifactBinding(nil), inputs...)) {
		t.Fatal("input Artifact binding digest is not deterministic")
	}
	now := time.Now().UTC()
	execution := DisabledExecution{
		ID: "execution-one", CandidateID: "candidate-one", PreparationID: "preparation-one",
		RunID: "run-one", MissionID: "mission-one", WorkspaceID: "workspace-one",
		CancellationID: "cancellation-one", ProtocolVersion: DisabledExecutionProtocolVersion,
		ManifestFingerprint:      strings.Repeat("1", 64),
		AuthorizationFingerprint: strings.Repeat("2", 64),
		PolicyFingerprint:        strings.Repeat("3", 64),
		MountBindingFingerprint:  strings.Repeat("4", 64),
		InputArtifactCount:       1, InputArtifactBytes: 12, InputArtifactDigest: digest,
		OutputPlan: plan, InitialLeaseID: "lease-one", InitialLeaseGeneration: 1,
		RequestedBy: "operator-one", CreatedAt: now,
	}
	if err := execution.Validate(); err != nil {
		t.Fatal(err)
	}
	execution.BackendStarted = true
	if err := execution.Validate(); err == nil {
		t.Fatal("disabled Sandbox execution accepted a started backend")
	}
	if err := ValidateExecutionLeaseTTL(MinExecutionLeaseTTL - time.Nanosecond); err == nil {
		t.Fatal("Sandbox execution lease accepted a sub-minimum TTL")
	}
}

func TestCleanupResultCannotClaimBackendOrOutputWork(t *testing.T) {
	now := time.Now().UTC()
	result := CleanupResult{
		ID: "cleanup-one", ExecutionID: "execution-one", RunID: "run-one",
		ProtocolVersion: CleanupProtocolVersion, LeaseID: "lease-one", LeaseGeneration: 2,
		InputArtifactsVerified: true, CleanupComplete: true, Outcome: "backend_disabled",
		ReconciledBy: "operator-one", CompletedAt: now,
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.OutputArtifactCount = 1
	if err := result.Validate(); err == nil {
		t.Fatal("disabled cleanup claimed an output Artifact")
	}
	result.OutputArtifactCount = 0
	result.OrphanDetected = true
	if err := result.Validate(); err == nil {
		t.Fatal("disabled cleanup claimed a backend orphan")
	}
}
