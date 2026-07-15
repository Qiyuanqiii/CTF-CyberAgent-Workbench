package sandbox

import (
	"strings"
	"testing"
	"time"
)

func TestDockerHostInputRequirementValidationAndSemanticFingerprint(t *testing.T) {
	digest := strings.Repeat("a", 64)
	requirement := DockerHostInputRequirement{
		AttemptID: "attempt-one", PlanID: "plan-one", RunID: "run-one",
		MissionID: "mission-one", WorkspaceID: "workspace-one",
		ProtocolVersion:          DockerHostInputRequirementProtocolVersion,
		OperationKeyDigest:       digest,
		AttemptIntentFingerprint: digest,
		RequestFingerprint:       digest,
		ManifestFingerprint:      digest,
		MountBindingFingerprint:  digest,
		InputArtifactDigest:      digest,
		AuthorityFingerprint:     digest,
		PlanFingerprint:          digest,
		ReadOnlyMountCount:       0,
		InputArtifactCount:       0,
		RequestedBy:              "operator-one",
		CreatedAt:                time.Now().UTC(),
	}
	requirement.RequirementFingerprint = dockerHostInputRequirementFingerprint(requirement)
	if err := requirement.Validate(); err != nil {
		t.Fatalf("optional zero-input requirement was rejected: %v", err)
	}
	second := requirement
	second.AttemptID = "attempt-two"
	second.CreatedAt = second.CreatedAt.Add(time.Second)
	if second.RequirementFingerprint != dockerHostInputRequirementFingerprint(second) ||
		second.Validate() != nil {
		t.Fatal("candidate row identity or timestamp changed the semantic requirement")
	}
	required := requirement
	required.Required, required.OperatorConfirmed = true, true
	required.RequirementFingerprint = dockerHostInputRequirementFingerprint(required)
	if err := required.Validate(); err == nil {
		t.Fatal("required host input staging accepted a zero-input plan")
	}
	mismatched := requirement
	mismatched.OperatorConfirmed = true
	mismatched.RequirementFingerprint = dockerHostInputRequirementFingerprint(mismatched)
	if err := mismatched.Validate(); err == nil {
		t.Fatal("host input requirement accepted a confirmation mismatch")
	}
}
