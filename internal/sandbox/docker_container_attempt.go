package sandbox

import (
	"errors"
	"strconv"
	"time"
)

const (
	DockerContainerAttemptProtocolVersion   = "sandbox_docker_container_rehearsal_attempt.v1"
	DockerContainerStageProtocolVersion     = "sandbox_docker_container_stage.v1"
	DockerContainerCleanupProtocolVersion   = "sandbox_docker_container_cleanup.v1"
	DockerContainerControlProtocolVersion   = "sandbox_docker_container_control_matrix.v1"
	DockerContainerAttemptStatusPrepared    = "prepared"
	DockerContainerAttemptStatusStaged      = "container_staged_verified"
	DockerContainerAttemptStatusCleaned     = "cleanup_confirmed"
	DockerContainerAttemptStatusCompleted   = "rehearsal_completed"
	DockerContainerAttemptLeaseActive       = "active"
	DockerContainerAttemptLeaseReleased     = "released"
	DockerContainerAttemptFailureStage      = "stage"
	DockerContainerAttemptFailureCleanup    = "cleanup"
	DockerContainerAttemptFailureCompletion = "completion"
	DockerContainerAttemptFailureCanceled   = "context_canceled"
	DockerContainerAttemptFailureDeadline   = "deadline_exceeded"
	DockerContainerAttemptFailureCheckpoint = "checkpoint_failure"
	DockerContainerStageStatusVerified      = "stopped_container_verified"
	DockerContainerCleanupStatusRemoved     = "container_removed"
	DockerContainerCleanupStatusAlreadyGone = "container_already_absent"
	DockerContainerControlStateVerified     = "verified"
	MaxDockerContainerAttemptFailures       = 16
	MaxDockerContainerVerifiedControls      = 19
	DefaultDockerContainerAttemptLeaseTTL   = 2 * time.Minute
	MinDockerContainerAttemptLeaseTTL       = time.Second
	MaxDockerContainerAttemptLeaseTTL       = 10 * time.Minute
)

var dockerContainerVerifiedControlNames = [...]string{
	"image_digest_exact",
	"command_and_workdir_exact",
	"non_root_user",
	"rootfs_read_only",
	"no_new_privileges",
	"capabilities_dropped",
	"init_enabled",
	"network_disabled",
	"environment_empty",
	"secrets_absent",
	"mount_configuration_exact_private",
	"resources_bounded",
	"restart_disabled",
	"logging_disabled",
	"devices_absent",
	"ports_absent",
	"attachments_disabled",
	"authority_labels_exact",
	"container_never_started",
}

type DockerContainerAttemptIntent struct {
	ID                       string
	PlanID                   string
	ObservationID            string
	EvidenceID               string
	OutputSimulationID       string
	PreflightID              string
	ExecutionID              string
	CandidateID              string
	PreparationID            string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	ProtocolVersion          string
	OperationKeyDigest       string
	ManifestFingerprint      string
	AuthorizationFingerprint string
	PolicyFingerprint        string
	MountBindingFingerprint  string
	InputArtifactDigest      string
	ThreatModelFingerprint   string
	OutputPlanFingerprint    string
	ObservationFingerprint   string
	AuthorityFingerprint     string
	SpecFingerprint          string
	PlanFingerprint          string
	ImageDigest              string
	RequestFingerprint       string
	EndpointClass            string
	EndpointFingerprint      string
	NetworkMode              string
	EnvironmentCount         int
	SecretReferenceCount     int
	IntentFingerprint        string
	RequestedBy              string
	CreatedAt                time.Time
}

func NewDockerContainerAttemptIntent(id, operationKeyDigest string, plan DockerContainerPlan,
	request DockerContainerWriteRequest, endpoint DockerObservationEndpoint, requestedBy string,
	createdAt time.Time,
) (DockerContainerAttemptIntent, error) {
	intent := DockerContainerAttemptIntent{
		ID: id, PlanID: plan.ID, ObservationID: plan.ObservationID, EvidenceID: plan.EvidenceID,
		OutputSimulationID: plan.OutputSimulationID, PreflightID: plan.PreflightID,
		ExecutionID: plan.ExecutionID, CandidateID: plan.CandidateID,
		PreparationID: plan.PreparationID, RunID: plan.RunID, MissionID: plan.MissionID,
		WorkspaceID: plan.WorkspaceID, ProtocolVersion: DockerContainerAttemptProtocolVersion,
		OperationKeyDigest: operationKeyDigest, ManifestFingerprint: plan.ManifestFingerprint,
		AuthorizationFingerprint: plan.AuthorizationFingerprint,
		PolicyFingerprint:        plan.PolicyFingerprint,
		MountBindingFingerprint:  plan.MountBindingFingerprint,
		InputArtifactDigest:      plan.InputArtifactDigest,
		ThreatModelFingerprint:   plan.ThreatModelFingerprint,
		OutputPlanFingerprint:    plan.OutputPlanFingerprint,
		ObservationFingerprint:   plan.ObservationFingerprint,
		AuthorityFingerprint:     plan.AuthorityFingerprint, SpecFingerprint: plan.SpecFingerprint,
		PlanFingerprint: plan.PlanFingerprint, ImageDigest: plan.ImageDigest,
		RequestFingerprint: request.RequestFingerprint, EndpointClass: endpoint.Class,
		EndpointFingerprint: endpoint.Fingerprint, NetworkMode: plan.NetworkMode,
		EnvironmentCount: plan.EnvironmentCount, SecretReferenceCount: plan.SecretReferenceCount,
		RequestedBy: requestedBy, CreatedAt: createdAt.UTC(),
	}
	intent.IntentFingerprint = dockerContainerAttemptIntentFingerprint(intent)
	if plan.Validate() != nil || request.Validate() != nil || endpoint.Validate() != nil ||
		DockerContainerPlanMatchesSpec(plan, request.Spec) != nil || intent.Validate() != nil {
		return DockerContainerAttemptIntent{}, errors.New("docker container rehearsal attempt intent is invalid")
	}
	return intent, nil
}

func (intent DockerContainerAttemptIntent) Validate() error {
	for label, value := range map[string]string{
		"attempt id": intent.ID, "attempt plan id": intent.PlanID,
		"attempt observation id": intent.ObservationID, "attempt evidence id": intent.EvidenceID,
		"attempt output simulation id": intent.OutputSimulationID,
		"attempt preflight id":         intent.PreflightID, "attempt execution id": intent.ExecutionID,
		"attempt candidate id": intent.CandidateID, "attempt preparation id": intent.PreparationID,
		"attempt Run id": intent.RunID, "attempt Mission id": intent.MissionID,
		"attempt Workspace id": intent.WorkspaceID, "attempt requester": intent.RequestedBy,
	} {
		if validateStoredIdentity(label, value) != nil {
			return errors.New("docker container rehearsal attempt identity is invalid")
		}
	}
	for _, value := range []string{intent.OperationKeyDigest, intent.ManifestFingerprint,
		intent.AuthorizationFingerprint, intent.PolicyFingerprint,
		intent.MountBindingFingerprint, intent.InputArtifactDigest,
		intent.ThreatModelFingerprint, intent.OutputPlanFingerprint,
		intent.ObservationFingerprint, intent.AuthorityFingerprint, intent.SpecFingerprint,
		intent.PlanFingerprint, intent.RequestFingerprint, intent.EndpointFingerprint,
		intent.IntentFingerprint} {
		if !validDigest(value) {
			return errors.New("docker container rehearsal attempt fingerprint is invalid")
		}
	}
	endpoint, err := NewDockerObservationEndpoint(intent.EndpointClass)
	if err != nil || intent.ProtocolVersion != DockerContainerAttemptProtocolVersion ||
		intent.EndpointClass != DockerObservationEndpointLocalUnix ||
		intent.EndpointFingerprint != endpoint.Fingerprint || !ValidOCIImageDigest(intent.ImageDigest) ||
		intent.NetworkMode != "disabled" || intent.EnvironmentCount != 0 ||
		intent.SecretReferenceCount != 0 || intent.CreatedAt.IsZero() ||
		intent.IntentFingerprint != dockerContainerAttemptIntentFingerprint(intent) {
		return errors.New("docker container rehearsal attempt violates the bounded intent")
	}
	return nil
}

func dockerContainerAttemptIntentFingerprint(intent DockerContainerAttemptIntent) string {
	return fingerprint(DockerContainerAttemptProtocolVersion, intent.OperationKeyDigest,
		intent.PlanID, intent.ObservationID, intent.EvidenceID, intent.OutputSimulationID,
		intent.PreflightID, intent.ExecutionID, intent.CandidateID, intent.PreparationID,
		intent.RunID, intent.MissionID, intent.WorkspaceID, intent.ManifestFingerprint,
		intent.AuthorizationFingerprint, intent.PolicyFingerprint,
		intent.MountBindingFingerprint, intent.InputArtifactDigest,
		intent.ThreatModelFingerprint, intent.OutputPlanFingerprint,
		intent.ObservationFingerprint, intent.AuthorityFingerprint, intent.SpecFingerprint,
		intent.PlanFingerprint, intent.ImageDigest, intent.RequestFingerprint,
		intent.EndpointClass, intent.EndpointFingerprint, intent.NetworkMode,
		strconv.Itoa(intent.EnvironmentCount), strconv.Itoa(intent.SecretReferenceCount),
		intent.RequestedBy)
}

type DockerContainerAttemptLease struct {
	AttemptID  string
	LeaseID    string
	OwnerID    string
	Generation int64
	Status     string
	AcquiredAt time.Time
	ExpiresAt  time.Time
	ReleasedAt *time.Time
}

func (lease DockerContainerAttemptLease) Validate() error {
	if validateStoredIdentity("Docker attempt lease attempt id", lease.AttemptID) != nil ||
		validateStoredIdentity("Docker attempt lease id", lease.LeaseID) != nil ||
		validateStoredIdentity("Docker attempt lease owner", lease.OwnerID) != nil ||
		lease.Generation < 1 ||
		(lease.Status != DockerContainerAttemptLeaseActive &&
			lease.Status != DockerContainerAttemptLeaseReleased) || lease.AcquiredAt.IsZero() ||
		lease.ExpiresAt.IsZero() || !lease.ExpiresAt.After(lease.AcquiredAt) {
		return errors.New("docker container attempt lease is invalid")
	}
	if lease.Status == DockerContainerAttemptLeaseActive && lease.ReleasedAt != nil {
		return errors.New("active Docker container attempt lease cannot be released")
	}
	if lease.Status == DockerContainerAttemptLeaseReleased &&
		(lease.ReleasedAt == nil || lease.ReleasedAt.Before(lease.AcquiredAt)) {
		return errors.New("released Docker container attempt lease requires a release time")
	}
	return nil
}

func (lease DockerContainerAttemptLease) ActiveAt(now time.Time) bool {
	return lease.Status == DockerContainerAttemptLeaseActive && now.Before(lease.ExpiresAt)
}

func ValidateDockerContainerAttemptLeaseTTL(ttl time.Duration) error {
	if ttl < MinDockerContainerAttemptLeaseTTL || ttl > MaxDockerContainerAttemptLeaseTTL {
		return errors.New("docker container attempt lease TTL is outside the supported range")
	}
	return nil
}

type DockerContainerVerifiedControl struct {
	Ordinal           int
	Name              string
	State             string
	Observed          bool
	Verified          bool
	ExecutionEvidence bool
	ControlDigest     string
}

func (control DockerContainerVerifiedControl) Validate(requestFingerprint string) error {
	if control.Ordinal < 1 || control.Ordinal > len(dockerContainerVerifiedControlNames) ||
		control.Name != dockerContainerVerifiedControlNames[control.Ordinal-1] ||
		control.State != DockerContainerControlStateVerified || !control.Observed ||
		!control.Verified || control.ExecutionEvidence ||
		control.ControlDigest != fingerprint(DockerContainerControlProtocolVersion,
			requestFingerprint, strconv.Itoa(control.Ordinal), control.Name, control.State,
			strconv.FormatBool(control.Observed), strconv.FormatBool(control.Verified),
			strconv.FormatBool(control.ExecutionEvidence)) {
		return errors.New("docker container verified control is invalid")
	}
	return nil
}

func newDockerContainerVerifiedControls(requestFingerprint string) []DockerContainerVerifiedControl {
	controls := make([]DockerContainerVerifiedControl, len(dockerContainerVerifiedControlNames))
	for index, name := range dockerContainerVerifiedControlNames {
		control := DockerContainerVerifiedControl{Ordinal: index + 1, Name: name,
			State: DockerContainerControlStateVerified, Observed: true, Verified: true}
		control.ControlDigest = fingerprint(DockerContainerControlProtocolVersion,
			requestFingerprint, strconv.Itoa(control.Ordinal), control.Name, control.State,
			strconv.FormatBool(control.Observed), strconv.FormatBool(control.Verified),
			strconv.FormatBool(control.ExecutionEvidence))
		controls[index] = control
	}
	return controls
}

func dockerContainerControlMatrixFingerprint(requestFingerprint string,
	controls []DockerContainerVerifiedControl,
) string {
	parts := []string{DockerContainerControlProtocolVersion, requestFingerprint,
		strconv.Itoa(len(controls))}
	for _, control := range controls {
		parts = append(parts, control.ControlDigest)
	}
	return fingerprint(parts...)
}

type DockerContainerStageResult struct {
	ProtocolVersion              string
	Status                       string
	EndpointClass                string
	EndpointFingerprint          string
	RequestFingerprint           string
	SpecFingerprint              string
	ContainerIDFingerprint       string
	InspectionFingerprint        string
	ControlMatrixFingerprint     string
	StageFingerprint             string
	ControlCount                 int
	DaemonReadCount              int
	DaemonWriteCount             int
	ContainerCreatedNow          bool
	ExistingContainerAdopted     bool
	ConfigurationMatched         bool
	ContainerPresent             bool
	ContainerStarted             bool
	ProcessExecuted              bool
	ImagePulled                  bool
	OutputExported               bool
	ProductionExecutionSubmitted bool
	ProductionVerified           bool
	BackendEnabled               bool
	ExecutionAuthorized          bool
	ArtifactCommitAuthorized     bool
	Controls                     []DockerContainerVerifiedControl
}

func NewDockerContainerStageResult(endpoint DockerObservationEndpoint,
	request DockerContainerWriteRequest, containerID string, adopted bool,
) (DockerContainerStageResult, error) {
	if endpoint.Validate() != nil || endpoint.Class != DockerObservationEndpointLocalUnix ||
		request.Validate() != nil || !validDockerContainerID(containerID) {
		return DockerContainerStageResult{}, errors.New("docker container stage result input is invalid")
	}
	controls := newDockerContainerVerifiedControls(request.RequestFingerprint)
	result := DockerContainerStageResult{
		ProtocolVersion: DockerContainerStageProtocolVersion,
		Status:          DockerContainerStageStatusVerified, EndpointClass: endpoint.Class,
		EndpointFingerprint:    endpoint.Fingerprint,
		RequestFingerprint:     request.RequestFingerprint,
		SpecFingerprint:        request.Spec.SpecFingerprint,
		ContainerIDFingerprint: fingerprint("sandbox_docker_container_id.v1", containerID),
		InspectionFingerprint: fingerprint("sandbox_docker_container_inspection.v1", containerID,
			request.Spec.SpecFingerprint, request.MountFingerprint),
		ControlCount: len(controls), DaemonReadCount: 3,
		DaemonWriteCount: boolIntLocal(!adopted), ContainerCreatedNow: !adopted,
		ExistingContainerAdopted: adopted, ConfigurationMatched: true,
		ContainerPresent: true, Controls: controls,
	}
	result.ControlMatrixFingerprint = dockerContainerControlMatrixFingerprint(
		request.RequestFingerprint, controls)
	result.StageFingerprint = dockerContainerStageResultFingerprint(result)
	if err := result.Validate(); err != nil {
		return DockerContainerStageResult{}, err
	}
	return result, nil
}

func (result DockerContainerStageResult) Validate() error {
	endpoint, err := NewDockerObservationEndpoint(result.EndpointClass)
	if err != nil || result.ProtocolVersion != DockerContainerStageProtocolVersion ||
		result.Status != DockerContainerStageStatusVerified ||
		result.EndpointClass != DockerObservationEndpointLocalUnix ||
		result.EndpointFingerprint != endpoint.Fingerprint ||
		!validDigest(result.RequestFingerprint) || !validDigest(result.SpecFingerprint) ||
		!validDigest(result.ContainerIDFingerprint) || !validDigest(result.InspectionFingerprint) ||
		!validDigest(result.ControlMatrixFingerprint) || !validDigest(result.StageFingerprint) ||
		result.ControlCount != len(dockerContainerVerifiedControlNames) ||
		len(result.Controls) != result.ControlCount || result.ControlCount > MaxDockerContainerVerifiedControls ||
		result.DaemonReadCount != 3 || result.DaemonWriteCount != boolIntLocal(result.ContainerCreatedNow) ||
		result.ContainerCreatedNow == result.ExistingContainerAdopted ||
		!result.ConfigurationMatched || !result.ContainerPresent || result.ContainerStarted ||
		result.ProcessExecuted || result.ImagePulled || result.OutputExported ||
		result.ProductionExecutionSubmitted || result.ProductionVerified || result.BackendEnabled ||
		result.ExecutionAuthorized || result.ArtifactCommitAuthorized {
		return errors.New("docker container stage result violates the non-execution boundary")
	}
	for index, control := range result.Controls {
		if control.Ordinal != index+1 || control.Validate(result.RequestFingerprint) != nil {
			return errors.New("docker container stage control matrix is invalid")
		}
	}
	if result.ControlMatrixFingerprint != dockerContainerControlMatrixFingerprint(
		result.RequestFingerprint, result.Controls) ||
		result.StageFingerprint != dockerContainerStageResultFingerprint(result) {
		return errors.New("docker container stage fingerprint is invalid")
	}
	return nil
}

func dockerContainerStageResultFingerprint(result DockerContainerStageResult) string {
	return fingerprint(DockerContainerStageProtocolVersion, result.Status, result.EndpointClass,
		result.EndpointFingerprint, result.RequestFingerprint, result.SpecFingerprint,
		result.ContainerIDFingerprint, result.InspectionFingerprint,
		result.ControlMatrixFingerprint, strconv.Itoa(result.ControlCount),
		strconv.Itoa(result.DaemonReadCount), strconv.Itoa(result.DaemonWriteCount),
		strconv.FormatBool(result.ContainerCreatedNow),
		strconv.FormatBool(result.ExistingContainerAdopted),
		strconv.FormatBool(result.ConfigurationMatched),
		strconv.FormatBool(result.ContainerPresent), strconv.FormatBool(result.ContainerStarted),
		strconv.FormatBool(result.ProcessExecuted), strconv.FormatBool(result.ImagePulled),
		strconv.FormatBool(result.OutputExported),
		strconv.FormatBool(result.ProductionExecutionSubmitted),
		strconv.FormatBool(result.ProductionVerified), strconv.FormatBool(result.BackendEnabled),
		strconv.FormatBool(result.ExecutionAuthorized),
		strconv.FormatBool(result.ArtifactCommitAuthorized))
}

type DockerContainerAttemptStage struct {
	AttemptID             string
	LeaseGeneration       int64
	Result                DockerContainerStageResult
	CheckpointFingerprint string
	RecordedAt            time.Time
}

func NewDockerContainerAttemptStage(attemptID string, generation int64,
	result DockerContainerStageResult, recordedAt time.Time,
) (DockerContainerAttemptStage, error) {
	stage := DockerContainerAttemptStage{AttemptID: attemptID, LeaseGeneration: generation,
		Result: result, RecordedAt: recordedAt.UTC()}
	stage.CheckpointFingerprint = fingerprint("sandbox_docker_container_stage_checkpoint.v1",
		stage.AttemptID, strconv.FormatInt(stage.LeaseGeneration, 10), result.StageFingerprint,
		stage.RecordedAt.Format(time.RFC3339Nano))
	if err := stage.Validate(); err != nil {
		return DockerContainerAttemptStage{}, err
	}
	return stage, nil
}

func (stage DockerContainerAttemptStage) Validate() error {
	if validateStoredIdentity("Docker attempt stage id", stage.AttemptID) != nil ||
		stage.LeaseGeneration < 1 || stage.Result.Validate() != nil || stage.RecordedAt.IsZero() ||
		stage.CheckpointFingerprint != fingerprint("sandbox_docker_container_stage_checkpoint.v1",
			stage.AttemptID, strconv.FormatInt(stage.LeaseGeneration, 10),
			stage.Result.StageFingerprint, stage.RecordedAt.Format(time.RFC3339Nano)) {
		return errors.New("docker container attempt stage checkpoint is invalid")
	}
	return nil
}

type DockerContainerCleanupResult struct {
	ProtocolVersion          string
	Status                   string
	EndpointClass            string
	EndpointFingerprint      string
	RequestFingerprint       string
	ContainerIDFingerprint   string
	CleanupFingerprint       string
	DaemonReadCount          int
	DaemonWriteCount         int
	ContainerRemovedNow      bool
	ContainerAlreadyAbsent   bool
	CleanupConfirmed         bool
	ContainerStarted         bool
	ProcessExecuted          bool
	OutputExported           bool
	ExecutionAuthorized      bool
	ArtifactCommitAuthorized bool
}

func NewDockerContainerCleanupResult(endpoint DockerObservationEndpoint,
	request DockerContainerWriteRequest, stage DockerContainerStageResult, removedNow bool,
) (DockerContainerCleanupResult, error) {
	status := DockerContainerCleanupStatusAlreadyGone
	if removedNow {
		status = DockerContainerCleanupStatusRemoved
	}
	result := DockerContainerCleanupResult{
		ProtocolVersion: DockerContainerCleanupProtocolVersion, Status: status,
		EndpointClass: endpoint.Class, EndpointFingerprint: endpoint.Fingerprint,
		RequestFingerprint:     request.RequestFingerprint,
		ContainerIDFingerprint: stage.ContainerIDFingerprint, DaemonReadCount: 1,
		DaemonWriteCount: boolIntLocal(removedNow), ContainerRemovedNow: removedNow,
		ContainerAlreadyAbsent: !removedNow, CleanupConfirmed: true,
	}
	result.CleanupFingerprint = dockerContainerCleanupResultFingerprint(result)
	if endpoint.Validate() != nil || request.Validate() != nil || stage.Validate() != nil ||
		stage.RequestFingerprint != request.RequestFingerprint || result.Validate() != nil {
		return DockerContainerCleanupResult{}, errors.New("docker container cleanup result input is invalid")
	}
	return result, nil
}

func (result DockerContainerCleanupResult) Validate() error {
	endpoint, err := NewDockerObservationEndpoint(result.EndpointClass)
	if err != nil || result.ProtocolVersion != DockerContainerCleanupProtocolVersion ||
		(result.Status != DockerContainerCleanupStatusRemoved &&
			result.Status != DockerContainerCleanupStatusAlreadyGone) ||
		result.EndpointClass != DockerObservationEndpointLocalUnix ||
		result.EndpointFingerprint != endpoint.Fingerprint ||
		!validDigest(result.RequestFingerprint) || !validDigest(result.ContainerIDFingerprint) ||
		!validDigest(result.CleanupFingerprint) || result.DaemonReadCount != 1 ||
		result.DaemonWriteCount != boolIntLocal(result.ContainerRemovedNow) ||
		result.ContainerRemovedNow == result.ContainerAlreadyAbsent ||
		(result.Status == DockerContainerCleanupStatusRemoved) != result.ContainerRemovedNow ||
		!result.CleanupConfirmed || result.ContainerStarted || result.ProcessExecuted ||
		result.OutputExported || result.ExecutionAuthorized || result.ArtifactCommitAuthorized ||
		result.CleanupFingerprint != dockerContainerCleanupResultFingerprint(result) {
		return errors.New("docker container cleanup result violates the exact cleanup boundary")
	}
	return nil
}

func dockerContainerCleanupResultFingerprint(result DockerContainerCleanupResult) string {
	return fingerprint(DockerContainerCleanupProtocolVersion, result.Status, result.EndpointClass,
		result.EndpointFingerprint, result.RequestFingerprint, result.ContainerIDFingerprint,
		strconv.Itoa(result.DaemonReadCount), strconv.Itoa(result.DaemonWriteCount),
		strconv.FormatBool(result.ContainerRemovedNow),
		strconv.FormatBool(result.ContainerAlreadyAbsent),
		strconv.FormatBool(result.CleanupConfirmed), strconv.FormatBool(result.ContainerStarted),
		strconv.FormatBool(result.ProcessExecuted), strconv.FormatBool(result.OutputExported),
		strconv.FormatBool(result.ExecutionAuthorized),
		strconv.FormatBool(result.ArtifactCommitAuthorized))
}

type DockerContainerAttemptCleanup struct {
	AttemptID             string
	LeaseGeneration       int64
	Result                DockerContainerCleanupResult
	CheckpointFingerprint string
	RecordedAt            time.Time
}

func NewDockerContainerAttemptCleanup(attemptID string, generation int64,
	result DockerContainerCleanupResult, recordedAt time.Time,
) (DockerContainerAttemptCleanup, error) {
	cleanup := DockerContainerAttemptCleanup{AttemptID: attemptID, LeaseGeneration: generation,
		Result: result, RecordedAt: recordedAt.UTC()}
	cleanup.CheckpointFingerprint = fingerprint("sandbox_docker_container_cleanup_checkpoint.v1",
		cleanup.AttemptID, strconv.FormatInt(cleanup.LeaseGeneration, 10),
		result.CleanupFingerprint, cleanup.RecordedAt.Format(time.RFC3339Nano))
	if err := cleanup.Validate(); err != nil {
		return DockerContainerAttemptCleanup{}, err
	}
	return cleanup, nil
}

func (cleanup DockerContainerAttemptCleanup) Validate() error {
	if validateStoredIdentity("Docker attempt cleanup id", cleanup.AttemptID) != nil ||
		cleanup.LeaseGeneration < 1 || cleanup.Result.Validate() != nil ||
		cleanup.RecordedAt.IsZero() || cleanup.CheckpointFingerprint != fingerprint(
		"sandbox_docker_container_cleanup_checkpoint.v1", cleanup.AttemptID,
		strconv.FormatInt(cleanup.LeaseGeneration, 10), cleanup.Result.CleanupFingerprint,
		cleanup.RecordedAt.Format(time.RFC3339Nano)) {
		return errors.New("docker container attempt cleanup checkpoint is invalid")
	}
	return nil
}

type DockerContainerAttemptFailure struct {
	AttemptID          string
	Ordinal            int
	LeaseGeneration    int64
	Phase              string
	Code               string
	Retryable          bool
	FailureFingerprint string
	CreatedAt          time.Time
}

func NewDockerContainerAttemptFailure(attemptID string, ordinal int, generation int64,
	phase, code string, retryable bool, createdAt time.Time,
) (DockerContainerAttemptFailure, error) {
	failure := DockerContainerAttemptFailure{AttemptID: attemptID, Ordinal: ordinal,
		LeaseGeneration: generation, Phase: phase, Code: code, Retryable: retryable,
		CreatedAt: createdAt.UTC()}
	failure.FailureFingerprint = fingerprint("sandbox_docker_container_attempt_failure.v1",
		failure.AttemptID, strconv.Itoa(failure.Ordinal),
		strconv.FormatInt(failure.LeaseGeneration, 10), failure.Phase, failure.Code,
		strconv.FormatBool(failure.Retryable), failure.CreatedAt.Format(time.RFC3339Nano))
	if err := failure.Validate(); err != nil {
		return DockerContainerAttemptFailure{}, err
	}
	return failure, nil
}

func (failure DockerContainerAttemptFailure) Validate() error {
	if validateStoredIdentity("Docker attempt failure id", failure.AttemptID) != nil ||
		failure.Ordinal < 1 || failure.Ordinal > MaxDockerContainerAttemptFailures ||
		failure.LeaseGeneration < 1 ||
		(failure.Phase != DockerContainerAttemptFailureStage &&
			failure.Phase != DockerContainerAttemptFailureCleanup &&
			failure.Phase != DockerContainerAttemptFailureCompletion) ||
		!validDockerContainerAttemptFailureCode(failure.Code) ||
		failure.CreatedAt.IsZero() || failure.FailureFingerprint != fingerprint(
		"sandbox_docker_container_attempt_failure.v1", failure.AttemptID,
		strconv.Itoa(failure.Ordinal), strconv.FormatInt(failure.LeaseGeneration, 10),
		failure.Phase, failure.Code, strconv.FormatBool(failure.Retryable),
		failure.CreatedAt.Format(time.RFC3339Nano)) {
		return errors.New("docker container attempt failure is invalid")
	}
	return nil
}

func validDockerContainerAttemptFailureCode(code string) bool {
	switch code {
	case DockerContainerWriteFailureDisabled,
		DockerContainerWriteFailureUnsupported,
		DockerContainerWriteFailureConnection,
		DockerContainerWriteFailureInvalidResponse,
		DockerContainerWriteFailureUnsafeExisting,
		DockerContainerWriteFailureUnsafeImage,
		DockerContainerWriteFailureCreateConflict,
		DockerContainerWriteFailureConfigMismatch,
		DockerContainerWriteFailureCleanup,
		DockerContainerAttemptFailureCanceled,
		DockerContainerAttemptFailureDeadline,
		DockerContainerAttemptFailureCheckpoint:
		return true
	default:
		return false
	}
}

type DockerContainerAttemptCompletion struct {
	AttemptID             string
	RehearsalID           string
	LeaseGeneration       int64
	CompletionFingerprint string
	CompletedAt           time.Time
}

func NewDockerContainerAttemptCompletion(attemptID, rehearsalID string, generation int64,
	completedAt time.Time,
) (DockerContainerAttemptCompletion, error) {
	completion := DockerContainerAttemptCompletion{AttemptID: attemptID,
		RehearsalID: rehearsalID, LeaseGeneration: generation, CompletedAt: completedAt.UTC()}
	completion.CompletionFingerprint = fingerprint(
		"sandbox_docker_container_attempt_completion.v1", completion.AttemptID,
		completion.RehearsalID, strconv.FormatInt(completion.LeaseGeneration, 10),
		completion.CompletedAt.Format(time.RFC3339Nano))
	if err := completion.Validate(); err != nil {
		return DockerContainerAttemptCompletion{}, err
	}
	return completion, nil
}

func (completion DockerContainerAttemptCompletion) Validate() error {
	if validateStoredIdentity("Docker attempt completion id", completion.AttemptID) != nil ||
		validateStoredIdentity("Docker attempt rehearsal id", completion.RehearsalID) != nil ||
		completion.LeaseGeneration < 1 || completion.CompletedAt.IsZero() ||
		completion.CompletionFingerprint != fingerprint(
			"sandbox_docker_container_attempt_completion.v1", completion.AttemptID,
			completion.RehearsalID, strconv.FormatInt(completion.LeaseGeneration, 10),
			completion.CompletedAt.Format(time.RFC3339Nano)) {
		return errors.New("docker container attempt completion is invalid")
	}
	return nil
}

type DockerContainerRehearsalAttempt struct {
	Intent                      DockerContainerAttemptIntent
	HostInputRequirement        *DockerHostInputRequirement
	HostInputHandoffRequirement *DockerHostInputHandoffRequirement
	Status                      string
	Lease                       DockerContainerAttemptLease
	Stage                       *DockerContainerAttemptStage
	Cleanup                     *DockerContainerAttemptCleanup
	Failures                    []DockerContainerAttemptFailure
	Completion                  *DockerContainerAttemptCompletion
	Replayed                    bool
	TookOver                    bool
}

func (attempt DockerContainerRehearsalAttempt) Validate() error {
	if attempt.Intent.Validate() != nil || attempt.Lease.Validate() != nil ||
		attempt.Lease.AttemptID != attempt.Intent.ID ||
		attempt.Lease.AcquiredAt.Before(attempt.Intent.CreatedAt) ||
		len(attempt.Failures) > MaxDockerContainerAttemptFailures {
		return errors.New("docker container rehearsal attempt is invalid")
	}
	if attempt.HostInputRequirement != nil {
		requirement := attempt.HostInputRequirement
		if requirement.Validate() != nil || requirement.AttemptID != attempt.Intent.ID ||
			requirement.PlanID != attempt.Intent.PlanID ||
			requirement.RunID != attempt.Intent.RunID ||
			requirement.MissionID != attempt.Intent.MissionID ||
			requirement.WorkspaceID != attempt.Intent.WorkspaceID ||
			requirement.OperationKeyDigest != attempt.Intent.OperationKeyDigest ||
			requirement.AttemptIntentFingerprint != attempt.Intent.IntentFingerprint ||
			requirement.RequestFingerprint != attempt.Intent.RequestFingerprint ||
			requirement.ManifestFingerprint != attempt.Intent.ManifestFingerprint ||
			requirement.MountBindingFingerprint != attempt.Intent.MountBindingFingerprint ||
			requirement.InputArtifactDigest != attempt.Intent.InputArtifactDigest ||
			requirement.AuthorityFingerprint != attempt.Intent.AuthorityFingerprint ||
			requirement.PlanFingerprint != attempt.Intent.PlanFingerprint ||
			requirement.RequestedBy != attempt.Intent.RequestedBy ||
			!requirement.CreatedAt.Equal(attempt.Intent.CreatedAt) {
			return errors.New("docker container host input requirement is invalid")
		}
	}
	if attempt.HostInputHandoffRequirement != nil {
		requirement := attempt.HostInputHandoffRequirement
		if requirement.Validate() != nil || attempt.HostInputRequirement == nil ||
			requirement.AttemptID != attempt.Intent.ID ||
			requirement.PlanID != attempt.Intent.PlanID ||
			requirement.RunID != attempt.Intent.RunID ||
			requirement.MissionID != attempt.Intent.MissionID ||
			requirement.WorkspaceID != attempt.Intent.WorkspaceID ||
			requirement.OperationKeyDigest != attempt.Intent.OperationKeyDigest ||
			requirement.AttemptIntentFingerprint != attempt.Intent.IntentFingerprint ||
			requirement.RequestFingerprint != attempt.Intent.RequestFingerprint ||
			requirement.CaptureRequirementFingerprint !=
				attempt.HostInputRequirement.RequirementFingerprint ||
			requirement.ManifestFingerprint != attempt.Intent.ManifestFingerprint ||
			requirement.MountBindingFingerprint != attempt.Intent.MountBindingFingerprint ||
			requirement.InputArtifactDigest != attempt.Intent.InputArtifactDigest ||
			requirement.AuthorityFingerprint != attempt.Intent.AuthorityFingerprint ||
			requirement.PlanFingerprint != attempt.Intent.PlanFingerprint ||
			requirement.RequestedBy != attempt.Intent.RequestedBy ||
			requirement.Required && !attempt.HostInputRequirement.Required ||
			!requirement.CreatedAt.Equal(attempt.Intent.CreatedAt) {
			return errors.New("docker container host input handoff requirement is invalid")
		}
	}
	for index, failure := range attempt.Failures {
		if failure.Validate() != nil || failure.AttemptID != attempt.Intent.ID ||
			failure.Ordinal != index+1 || failure.LeaseGeneration > attempt.Lease.Generation ||
			failure.CreatedAt.Before(attempt.Intent.CreatedAt) ||
			(index > 0 && failure.CreatedAt.Before(attempt.Failures[index-1].CreatedAt)) {
			return errors.New("docker container rehearsal attempt failures are invalid")
		}
	}
	expectedStatus := DockerContainerAttemptStatusPrepared
	if attempt.Stage != nil {
		if attempt.Stage.Validate() != nil || attempt.Stage.AttemptID != attempt.Intent.ID ||
			attempt.Stage.LeaseGeneration > attempt.Lease.Generation ||
			attempt.Stage.RecordedAt.Before(attempt.Intent.CreatedAt) ||
			attempt.Stage.Result.RequestFingerprint != attempt.Intent.RequestFingerprint ||
			attempt.Stage.Result.SpecFingerprint != attempt.Intent.SpecFingerprint ||
			attempt.Stage.Result.EndpointFingerprint != attempt.Intent.EndpointFingerprint {
			return errors.New("docker container rehearsal attempt stage is invalid")
		}
		expectedStatus = DockerContainerAttemptStatusStaged
	}
	if attempt.Cleanup != nil {
		if attempt.Stage == nil || attempt.Cleanup.Validate() != nil ||
			attempt.Cleanup.AttemptID != attempt.Intent.ID ||
			attempt.Cleanup.LeaseGeneration < attempt.Stage.LeaseGeneration ||
			attempt.Cleanup.LeaseGeneration > attempt.Lease.Generation ||
			attempt.Cleanup.RecordedAt.Before(attempt.Stage.RecordedAt) ||
			attempt.Cleanup.Result.RequestFingerprint != attempt.Intent.RequestFingerprint ||
			attempt.Cleanup.Result.ContainerIDFingerprint !=
				attempt.Stage.Result.ContainerIDFingerprint {
			return errors.New("docker container rehearsal attempt cleanup is invalid")
		}
		expectedStatus = DockerContainerAttemptStatusCleaned
	}
	if attempt.Completion != nil {
		if attempt.Cleanup == nil || attempt.Completion.Validate() != nil ||
			attempt.Completion.AttemptID != attempt.Intent.ID ||
			attempt.Completion.LeaseGeneration != attempt.Lease.Generation ||
			attempt.Completion.CompletedAt.Before(attempt.Cleanup.RecordedAt) ||
			attempt.Lease.Status != DockerContainerAttemptLeaseReleased {
			return errors.New("docker container rehearsal attempt completion is invalid")
		}
		expectedStatus = DockerContainerAttemptStatusCompleted
	}
	if attempt.Status != expectedStatus {
		return errors.New("docker container rehearsal attempt status is invalid")
	}
	return nil
}

type DockerContainerAttemptAcquisition struct {
	Attempt  DockerContainerRehearsalAttempt
	Replayed bool
	TookOver bool
}

func boolIntLocal(value bool) int {
	if value {
		return 1
	}
	return 0
}
