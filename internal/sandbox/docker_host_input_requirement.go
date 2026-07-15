package sandbox

import (
	"errors"
	"strconv"
	"time"
)

const DockerHostInputRequirementProtocolVersion = "sandbox_docker_host_input_requirement.v1"

// DockerHostInputRequirement fixes the operator's host-input choice before the
// first daemon mutation. Row identifiers and timestamps are deliberately not
// part of the semantic fingerprint so independent retries can converge.
type DockerHostInputRequirement struct {
	AttemptID                string
	PlanID                   string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	ProtocolVersion          string
	OperationKeyDigest       string
	AttemptIntentFingerprint string
	RequestFingerprint       string
	ManifestFingerprint      string
	MountBindingFingerprint  string
	InputArtifactDigest      string
	AuthorityFingerprint     string
	PlanFingerprint          string
	Required                 bool
	OperatorConfirmed        bool
	ReadOnlyMountCount       int
	InputArtifactCount       int
	RequirementFingerprint   string
	RequestedBy              string
	CreatedAt                time.Time
}

func NewDockerHostInputRequirement(intent DockerContainerAttemptIntent,
	plan DockerContainerPlan, required, operatorConfirmed bool,
) (DockerHostInputRequirement, error) {
	requirement := DockerHostInputRequirement{
		AttemptID: intent.ID, PlanID: intent.PlanID, RunID: intent.RunID,
		MissionID: intent.MissionID, WorkspaceID: intent.WorkspaceID,
		ProtocolVersion:          DockerHostInputRequirementProtocolVersion,
		OperationKeyDigest:       intent.OperationKeyDigest,
		AttemptIntentFingerprint: intent.IntentFingerprint,
		RequestFingerprint:       intent.RequestFingerprint,
		ManifestFingerprint:      intent.ManifestFingerprint,
		MountBindingFingerprint:  intent.MountBindingFingerprint,
		InputArtifactDigest:      intent.InputArtifactDigest,
		AuthorityFingerprint:     intent.AuthorityFingerprint,
		PlanFingerprint:          intent.PlanFingerprint,
		Required:                 required, OperatorConfirmed: operatorConfirmed,
		ReadOnlyMountCount: plan.ReadOnlyMountCount,
		InputArtifactCount: plan.InputArtifactCount,
		RequestedBy:        intent.RequestedBy, CreatedAt: intent.CreatedAt,
	}
	requirement.RequirementFingerprint = dockerHostInputRequirementFingerprint(requirement)
	if intent.Validate() != nil || plan.Validate() != nil ||
		intent.PlanID != plan.ID || intent.RunID != plan.RunID ||
		intent.MissionID != plan.MissionID || intent.WorkspaceID != plan.WorkspaceID ||
		intent.ManifestFingerprint != plan.ManifestFingerprint ||
		intent.MountBindingFingerprint != plan.MountBindingFingerprint ||
		intent.InputArtifactDigest != plan.InputArtifactDigest ||
		intent.AuthorityFingerprint != plan.AuthorityFingerprint ||
		intent.PlanFingerprint != plan.PlanFingerprint || intent.RequestedBy != plan.RequestedBy ||
		requirement.Validate() != nil {
		return DockerHostInputRequirement{}, errors.New("docker host input requirement is invalid")
	}
	return requirement, nil
}

func (requirement DockerHostInputRequirement) Validate() error {
	for label, value := range map[string]string{
		"host input requirement attempt id":   requirement.AttemptID,
		"host input requirement plan id":      requirement.PlanID,
		"host input requirement Run id":       requirement.RunID,
		"host input requirement Mission id":   requirement.MissionID,
		"host input requirement Workspace id": requirement.WorkspaceID,
		"host input requirement requester":    requirement.RequestedBy,
	} {
		if validateStoredIdentity(label, value) != nil {
			return errors.New("docker host input requirement identity is invalid")
		}
	}
	for _, value := range []string{
		requirement.OperationKeyDigest, requirement.AttemptIntentFingerprint,
		requirement.RequestFingerprint, requirement.ManifestFingerprint,
		requirement.MountBindingFingerprint, requirement.InputArtifactDigest,
		requirement.AuthorityFingerprint, requirement.PlanFingerprint,
		requirement.RequirementFingerprint,
	} {
		if !validDigest(value) {
			return errors.New("docker host input requirement digest is invalid")
		}
	}
	if requirement.ProtocolVersion != DockerHostInputRequirementProtocolVersion ||
		requirement.Required != requirement.OperatorConfirmed ||
		requirement.ReadOnlyMountCount < 0 || requirement.ReadOnlyMountCount > MaxMounts ||
		(requirement.Required && requirement.ReadOnlyMountCount == 0) ||
		requirement.InputArtifactCount < 0 ||
		requirement.InputArtifactCount > MaxInputArtifacts || requirement.CreatedAt.IsZero() ||
		requirement.RequirementFingerprint != dockerHostInputRequirementFingerprint(requirement) {
		return errors.New("docker host input requirement violates the pre-stage boundary")
	}
	return nil
}

func dockerHostInputRequirementFingerprint(requirement DockerHostInputRequirement) string {
	return fingerprint(DockerHostInputRequirementProtocolVersion, requirement.PlanID,
		requirement.RunID, requirement.MissionID, requirement.WorkspaceID,
		requirement.OperationKeyDigest, requirement.AttemptIntentFingerprint,
		requirement.RequestFingerprint, requirement.ManifestFingerprint,
		requirement.MountBindingFingerprint, requirement.InputArtifactDigest,
		requirement.AuthorityFingerprint, requirement.PlanFingerprint,
		strconv.FormatBool(requirement.Required),
		strconv.FormatBool(requirement.OperatorConfirmed),
		strconv.Itoa(requirement.ReadOnlyMountCount),
		strconv.Itoa(requirement.InputArtifactCount), requirement.RequestedBy)
}
