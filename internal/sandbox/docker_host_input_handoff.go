package sandbox

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	DockerHostInputHandoffRequirementProtocolVersion = "sandbox_docker_host_input_handoff_requirement.v1"
	DockerHostInputHandoffIntentProtocolVersion      = "sandbox_docker_host_input_handoff_intent.v1"
	DockerHostInputHandoffProtocolVersion            = "sandbox_docker_host_input_handoff.v1"
	DockerHostInputHandoffRequestProtocolVersion     = "sandbox_docker_host_input_handoff_request.v1"
	DockerHostInputHandoffSourceLocal                = "local_docker_volume_carrier"
	DockerHostInputHandoffTrustClass                 = "daemon_readback_verified_never_started"
	DockerHostInputHandoffStatusComplete             = "daemon_handoff_cleaned"
	DockerHostInputCarrierDestination                = "/cyberagent-input"
	DockerHostInputCarrierArchiveName                = "bundle.tar"
	DockerHostInputHandoffErrorDisabled              = "handoff_disabled"
	DockerHostInputHandoffErrorUnsupported           = "handoff_unsupported"
	DockerHostInputHandoffErrorInvalidBundle         = "invalid_bundle"
	DockerHostInputHandoffErrorUnsafeCollision       = "unsafe_resource_collision"
	DockerHostInputHandoffErrorReadbackMismatch      = "daemon_readback_mismatch"
	DockerHostInputHandoffErrorCleanup               = "handoff_cleanup_failed"
	MaxDockerHostInputHandoffDaemonReads             = 32
	MaxDockerHostInputHandoffDaemonWrites            = 24
)

type DockerHostInputHandoffRequirement struct {
	AttemptID                     string
	PlanID                        string
	RunID                         string
	MissionID                     string
	WorkspaceID                   string
	ProtocolVersion               string
	OperationKeyDigest            string
	AttemptIntentFingerprint      string
	RequestFingerprint            string
	CaptureRequirementFingerprint string
	ManifestFingerprint           string
	MountBindingFingerprint       string
	InputArtifactDigest           string
	AuthorityFingerprint          string
	PlanFingerprint               string
	Required                      bool
	OperatorConfirmed             bool
	ReadOnlyMountCount            int
	InputArtifactCount            int
	RequirementFingerprint        string
	RequestedBy                   string
	CreatedAt                     time.Time
}

func NewDockerHostInputHandoffRequirement(intent DockerContainerAttemptIntent,
	plan DockerContainerPlan, capture DockerHostInputRequirement, required, confirmed bool,
) (DockerHostInputHandoffRequirement, error) {
	requirement := DockerHostInputHandoffRequirement{
		AttemptID: intent.ID, PlanID: plan.ID, RunID: plan.RunID,
		MissionID: plan.MissionID, WorkspaceID: plan.WorkspaceID,
		ProtocolVersion:               DockerHostInputHandoffRequirementProtocolVersion,
		OperationKeyDigest:            intent.OperationKeyDigest,
		AttemptIntentFingerprint:      intent.IntentFingerprint,
		RequestFingerprint:            intent.RequestFingerprint,
		CaptureRequirementFingerprint: capture.RequirementFingerprint,
		ManifestFingerprint:           plan.ManifestFingerprint,
		MountBindingFingerprint:       plan.MountBindingFingerprint,
		InputArtifactDigest:           plan.InputArtifactDigest,
		AuthorityFingerprint:          plan.AuthorityFingerprint,
		PlanFingerprint:               plan.PlanFingerprint, Required: required,
		OperatorConfirmed: confirmed, ReadOnlyMountCount: plan.ReadOnlyMountCount,
		InputArtifactCount: plan.InputArtifactCount, RequestedBy: intent.RequestedBy,
		CreatedAt: intent.CreatedAt,
	}
	requirement.RequirementFingerprint = dockerHostInputHandoffRequirementFingerprint(requirement)
	if intent.Validate() != nil || plan.Validate() != nil || capture.Validate() != nil ||
		capture.AttemptID != intent.ID || capture.PlanID != plan.ID ||
		capture.OperationKeyDigest != intent.OperationKeyDigest ||
		capture.AttemptIntentFingerprint != intent.IntentFingerprint ||
		capture.RequestFingerprint != intent.RequestFingerprint ||
		(required && !capture.Required) ||
		intent.PlanID != plan.ID || intent.RequestedBy != plan.RequestedBy ||
		requirement.Validate() != nil {
		return DockerHostInputHandoffRequirement{}, errors.New("docker host input handoff requirement authority is invalid")
	}
	return requirement, nil
}

func (requirement DockerHostInputHandoffRequirement) Validate() error {
	for _, value := range []string{requirement.AttemptID, requirement.PlanID,
		requirement.RunID, requirement.MissionID, requirement.WorkspaceID, requirement.RequestedBy} {
		if validateStoredIdentity("Docker host input handoff requirement identity", value) != nil {
			return errors.New("docker host input handoff requirement identity is invalid")
		}
	}
	for _, value := range []string{requirement.OperationKeyDigest,
		requirement.AttemptIntentFingerprint, requirement.RequestFingerprint,
		requirement.CaptureRequirementFingerprint, requirement.ManifestFingerprint,
		requirement.MountBindingFingerprint, requirement.InputArtifactDigest,
		requirement.AuthorityFingerprint, requirement.PlanFingerprint,
		requirement.RequirementFingerprint} {
		if !validDigest(value) {
			return errors.New("docker host input handoff requirement digest is invalid")
		}
	}
	if requirement.ProtocolVersion != DockerHostInputHandoffRequirementProtocolVersion ||
		requirement.OperatorConfirmed != requirement.Required || requirement.CreatedAt.IsZero() ||
		requirement.ReadOnlyMountCount < 0 || requirement.ReadOnlyMountCount > MaxMounts ||
		requirement.InputArtifactCount < 0 || requirement.InputArtifactCount > MaxInputArtifacts ||
		(requirement.Required && requirement.ReadOnlyMountCount < 1) ||
		requirement.RequirementFingerprint != dockerHostInputHandoffRequirementFingerprint(requirement) {
		return errors.New("docker host input handoff requirement is invalid")
	}
	return nil
}

func dockerHostInputHandoffRequirementFingerprint(value DockerHostInputHandoffRequirement) string {
	return fingerprint(DockerHostInputHandoffRequirementProtocolVersion, value.PlanID,
		value.RunID, value.MissionID, value.WorkspaceID,
		value.OperationKeyDigest, value.AttemptIntentFingerprint, value.RequestFingerprint,
		value.CaptureRequirementFingerprint, value.ManifestFingerprint,
		value.MountBindingFingerprint, value.InputArtifactDigest, value.AuthorityFingerprint,
		value.PlanFingerprint, strconv.FormatBool(value.Required),
		strconv.FormatBool(value.OperatorConfirmed), strconv.Itoa(value.ReadOnlyMountCount),
		strconv.Itoa(value.InputArtifactCount), value.RequestedBy)
}

type DockerHostInputHandoffIntent struct {
	ID                            string
	AttemptID                     string
	StagingIntentID               string
	StagingID                     string
	PlanID                        string
	RunID                         string
	MissionID                     string
	WorkspaceID                   string
	ProtocolVersion               string
	OperationKeyDigest            string
	AttemptIntentFingerprint      string
	ContainerIDFingerprint        string
	CaptureRequirementFingerprint string
	HandoffRequirementFingerprint string
	StagingFingerprint            string
	BundleReportFingerprint       string
	BundleDigest                  string
	BundleBytes                   int64
	AuthorityFingerprint          string
	SpecFingerprint               string
	PlanFingerprint               string
	PreparedGeneration            int64
	IntentFingerprint             string
	RequestedBy                   string
	CreatedAt                     time.Time
}

func NewDockerHostInputHandoffIntent(id, operationKeyDigest string,
	attempt DockerContainerRehearsalAttempt, plan DockerContainerPlan,
	staging DockerHostInputStagingRecord, now time.Time,
) (DockerHostInputHandoffIntent, error) {
	if attempt.Validate() != nil || plan.Validate() != nil || staging.Validate() != nil ||
		attempt.Stage == nil || attempt.Cleanup != nil || attempt.Completion != nil ||
		attempt.HostInputRequirement == nil || attempt.HostInputHandoffRequirement == nil ||
		!attempt.HostInputRequirement.Required || !attempt.HostInputHandoffRequirement.Required ||
		staging.Staging == nil || staging.Intent.AttemptID != attempt.Intent.ID ||
		staging.Intent.PlanID != plan.ID || !attempt.Lease.ActiveAt(now) {
		return DockerHostInputHandoffIntent{}, errors.New("docker host input handoff intent authority is invalid")
	}
	value := DockerHostInputHandoffIntent{
		ID: id, AttemptID: attempt.Intent.ID, StagingIntentID: staging.Intent.ID,
		StagingID: staging.Staging.ID, PlanID: plan.ID, RunID: plan.RunID,
		MissionID: plan.MissionID, WorkspaceID: plan.WorkspaceID,
		ProtocolVersion:               DockerHostInputHandoffIntentProtocolVersion,
		OperationKeyDigest:            operationKeyDigest,
		AttemptIntentFingerprint:      attempt.Intent.IntentFingerprint,
		ContainerIDFingerprint:        attempt.Stage.Result.ContainerIDFingerprint,
		CaptureRequirementFingerprint: attempt.HostInputRequirement.RequirementFingerprint,
		HandoffRequirementFingerprint: attempt.HostInputHandoffRequirement.RequirementFingerprint,
		StagingFingerprint:            staging.Staging.StagingFingerprint,
		BundleReportFingerprint:       staging.Staging.Report.ReportFingerprint,
		BundleDigest:                  staging.Staging.Report.BundleDigest,
		BundleBytes:                   staging.Staging.Report.BundleBytes,
		AuthorityFingerprint:          plan.AuthorityFingerprint,
		SpecFingerprint:               plan.SpecFingerprint, PlanFingerprint: plan.PlanFingerprint,
		PreparedGeneration: attempt.Lease.Generation, RequestedBy: attempt.Intent.RequestedBy,
		CreatedAt: now.UTC(),
	}
	value.IntentFingerprint = dockerHostInputHandoffIntentFingerprint(value)
	return value, value.Validate()
}

func (value DockerHostInputHandoffIntent) Validate() error {
	for _, identity := range []string{value.ID, value.AttemptID, value.StagingIntentID,
		value.StagingID, value.PlanID, value.RunID, value.MissionID, value.WorkspaceID,
		value.RequestedBy} {
		if validateStoredIdentity("Docker host input handoff intent identity", identity) != nil {
			return errors.New("docker host input handoff intent identity is invalid")
		}
	}
	for _, digest := range []string{value.OperationKeyDigest, value.AttemptIntentFingerprint,
		value.ContainerIDFingerprint, value.CaptureRequirementFingerprint,
		value.HandoffRequirementFingerprint, value.StagingFingerprint,
		value.BundleReportFingerprint, value.BundleDigest, value.AuthorityFingerprint,
		value.SpecFingerprint, value.PlanFingerprint, value.IntentFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker host input handoff intent digest is invalid")
		}
	}
	if value.ProtocolVersion != DockerHostInputHandoffIntentProtocolVersion ||
		value.BundleBytes < 1 || value.BundleBytes > MaxHostInputBundleBytes ||
		value.PreparedGeneration < 1 || value.CreatedAt.IsZero() ||
		value.IntentFingerprint != dockerHostInputHandoffIntentFingerprint(value) {
		return errors.New("docker host input handoff intent is invalid")
	}
	return nil
}

func dockerHostInputHandoffIntentFingerprint(value DockerHostInputHandoffIntent) string {
	return fingerprint(DockerHostInputHandoffIntentProtocolVersion, value.AttemptID,
		value.StagingIntentID, value.StagingID, value.PlanID, value.RunID, value.MissionID,
		value.WorkspaceID, value.OperationKeyDigest, value.AttemptIntentFingerprint,
		value.ContainerIDFingerprint, value.CaptureRequirementFingerprint,
		value.HandoffRequirementFingerprint, value.StagingFingerprint,
		value.BundleReportFingerprint, value.BundleDigest, strconv.FormatInt(value.BundleBytes, 10),
		value.AuthorityFingerprint, value.SpecFingerprint, value.PlanFingerprint,
		strconv.FormatInt(value.PreparedGeneration, 10), value.RequestedBy)
}

type DockerHostInputHandoffRequest struct {
	ProtocolVersion    string
	IntentFingerprint  string
	WriteRequest       DockerContainerWriteRequest
	Stage              DockerContainerStageResult
	BundleReport       HostInputBundleReport
	CarrierName        string
	VolumeName         string
	RequestFingerprint string
}

func NewDockerHostInputHandoffRequest(intent DockerHostInputHandoffIntent,
	writeRequest DockerContainerWriteRequest, stage DockerContainerStageResult,
	report HostInputBundleReport,
) (DockerHostInputHandoffRequest, error) {
	seed := fingerprint(DockerHostInputHandoffRequestProtocolVersion, intent.IntentFingerprint,
		report.ReportFingerprint, stage.ContainerIDFingerprint)
	value := DockerHostInputHandoffRequest{
		ProtocolVersion:   DockerHostInputHandoffRequestProtocolVersion,
		IntentFingerprint: intent.IntentFingerprint, WriteRequest: writeRequest, Stage: stage,
		BundleReport: report, CarrierName: "cyberagent-carrier-" + seed[:20],
		VolumeName: "cyberagent-input-" + seed[:24],
	}
	value.RequestFingerprint = dockerHostInputHandoffRequestFingerprint(value)
	if intent.Validate() != nil || intent.ContainerIDFingerprint != stage.ContainerIDFingerprint ||
		intent.SpecFingerprint != writeRequest.Spec.SpecFingerprint ||
		intent.BundleReportFingerprint != report.ReportFingerprint ||
		intent.BundleDigest != report.BundleDigest || intent.BundleBytes != report.BundleBytes ||
		value.Validate() != nil {
		return DockerHostInputHandoffRequest{}, errors.New("docker host input handoff request authority is invalid")
	}
	return value, nil
}

func (value DockerHostInputHandoffRequest) Validate() error {
	if value.ProtocolVersion != DockerHostInputHandoffRequestProtocolVersion ||
		!validDigest(value.IntentFingerprint) || value.WriteRequest.Validate() != nil ||
		value.Stage.Validate() != nil || value.BundleReport.Validate() != nil ||
		value.Stage.RequestFingerprint != value.WriteRequest.RequestFingerprint ||
		value.Stage.SpecFingerprint != value.WriteRequest.Spec.SpecFingerprint ||
		dockerHostInputDestinationConflicts(value.WriteRequest.HostMounts) ||
		!validDockerHostInputCarrierName(value.CarrierName) ||
		!validDockerHostInputVolumeName(value.VolumeName) ||
		!validDigest(value.RequestFingerprint) ||
		value.RequestFingerprint != dockerHostInputHandoffRequestFingerprint(value) {
		return errors.New("docker host input handoff request is invalid")
	}
	return nil
}

func dockerHostInputDestinationConflicts(mounts []DockerHostMount) bool {
	for _, mount := range mounts {
		if mount.Target == "/" ||
			pathWithin(mount.Target, DockerHostInputCarrierDestination) ||
			pathWithin(DockerHostInputCarrierDestination, mount.Target) {
			return true
		}
	}
	return false
}

func dockerHostInputHandoffRequestFingerprint(value DockerHostInputHandoffRequest) string {
	return fingerprint(DockerHostInputHandoffRequestProtocolVersion, value.IntentFingerprint,
		value.WriteRequest.RequestFingerprint, value.Stage.ContainerIDFingerprint,
		value.BundleReport.ReportFingerprint, value.BundleReport.BundleDigest,
		strconv.FormatInt(value.BundleReport.BundleBytes, 10), value.CarrierName, value.VolumeName,
		DockerHostInputCarrierDestination, DockerHostInputCarrierArchiveName)
}

type DockerHostInputHandoffResult struct {
	ProtocolVersion              string
	Source                       string
	TrustClass                   string
	Status                       string
	EndpointClass                string
	EndpointFingerprint          string
	RequestFingerprint           string
	IntentFingerprint            string
	BundleReportFingerprint      string
	BundleDigest                 string
	ReadbackDigest               string
	CarrierNameFingerprint       string
	VolumeNameFingerprint        string
	FinalContainerIDFingerprint  string
	TransportFingerprint         string
	DaemonReadCount              int
	DaemonWriteCount             int
	ReconciledResourceCount      int
	DaemonConsumed               bool
	ReadbackVerified             bool
	FinalMountReadOnly           bool
	CarrierRemoved               bool
	FinalContainerRemoved        bool
	VolumeRemoved                bool
	CleanupConfirmed             bool
	ContainerStarted             bool
	ProcessExecuted              bool
	OutputExported               bool
	RawContentRetained           bool
	ProductionExecutionSubmitted bool
	ProductionVerified           bool
	BackendEnabled               bool
	ExecutionAuthorized          bool
	ArtifactCommitAuthorized     bool
}

func NewDockerHostInputHandoffResult(endpoint DockerObservationEndpoint,
	request DockerHostInputHandoffRequest, finalContainerID string, reads, writes, reconciled int,
) (DockerHostInputHandoffResult, error) {
	value := DockerHostInputHandoffResult{
		ProtocolVersion: DockerHostInputHandoffProtocolVersion,
		Source:          DockerHostInputHandoffSourceLocal, TrustClass: DockerHostInputHandoffTrustClass,
		Status: DockerHostInputHandoffStatusComplete, EndpointClass: endpoint.Class,
		EndpointFingerprint: endpoint.Fingerprint, RequestFingerprint: request.RequestFingerprint,
		IntentFingerprint:           request.IntentFingerprint,
		BundleReportFingerprint:     request.BundleReport.ReportFingerprint,
		BundleDigest:                request.BundleReport.BundleDigest,
		ReadbackDigest:              request.BundleReport.BundleDigest,
		CarrierNameFingerprint:      fingerprint("sandbox_docker_host_input_carrier_name.v1", request.CarrierName),
		VolumeNameFingerprint:       fingerprint("sandbox_docker_host_input_volume_name.v1", request.VolumeName),
		FinalContainerIDFingerprint: fingerprint("sandbox_docker_container_id.v1", finalContainerID),
		DaemonReadCount:             reads, DaemonWriteCount: writes,
		ReconciledResourceCount: reconciled, DaemonConsumed: true, ReadbackVerified: true,
		FinalMountReadOnly: true, CarrierRemoved: true, FinalContainerRemoved: true,
		VolumeRemoved: true, CleanupConfirmed: true,
	}
	value.TransportFingerprint = dockerHostInputHandoffResultFingerprint(value)
	if endpoint.Validate() != nil || request.Validate() != nil || !validDockerContainerID(finalContainerID) ||
		value.Validate() != nil {
		return DockerHostInputHandoffResult{}, errors.New("docker host input handoff result input is invalid")
	}
	return value, nil
}

func (value DockerHostInputHandoffResult) Validate() error {
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	if err != nil || value.ProtocolVersion != DockerHostInputHandoffProtocolVersion ||
		value.Source != DockerHostInputHandoffSourceLocal ||
		value.TrustClass != DockerHostInputHandoffTrustClass ||
		value.Status != DockerHostInputHandoffStatusComplete ||
		value.EndpointClass != DockerObservationEndpointLocalUnix ||
		value.EndpointFingerprint != endpoint.Fingerprint ||
		value.DaemonReadCount < 6 || value.DaemonReadCount > MaxDockerHostInputHandoffDaemonReads ||
		value.DaemonWriteCount < 7 || value.DaemonWriteCount > MaxDockerHostInputHandoffDaemonWrites ||
		value.ReconciledResourceCount < 0 || value.ReconciledResourceCount > 3 ||
		!value.DaemonConsumed || !value.ReadbackVerified || !value.FinalMountReadOnly ||
		!value.CarrierRemoved || !value.FinalContainerRemoved || !value.VolumeRemoved ||
		!value.CleanupConfirmed || value.ContainerStarted || value.ProcessExecuted ||
		value.OutputExported || value.RawContentRetained || value.ProductionExecutionSubmitted ||
		value.ProductionVerified || value.BackendEnabled || value.ExecutionAuthorized ||
		value.ArtifactCommitAuthorized || value.ReadbackDigest != value.BundleDigest {
		return errors.New("docker host input handoff result widened execution authority")
	}
	for _, digest := range []string{value.RequestFingerprint, value.IntentFingerprint,
		value.BundleReportFingerprint, value.BundleDigest, value.ReadbackDigest,
		value.CarrierNameFingerprint, value.VolumeNameFingerprint,
		value.FinalContainerIDFingerprint, value.TransportFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker host input handoff result digest is invalid")
		}
	}
	if value.TransportFingerprint != dockerHostInputHandoffResultFingerprint(value) {
		return errors.New("docker host input handoff result fingerprint is invalid")
	}
	return nil
}

func dockerHostInputHandoffResultFingerprint(value DockerHostInputHandoffResult) string {
	return fingerprint(DockerHostInputHandoffProtocolVersion, value.Source, value.TrustClass,
		value.Status, value.EndpointClass, value.EndpointFingerprint, value.RequestFingerprint,
		value.IntentFingerprint, value.BundleReportFingerprint, value.BundleDigest,
		value.ReadbackDigest, value.CarrierNameFingerprint, value.VolumeNameFingerprint,
		value.FinalContainerIDFingerprint, strconv.Itoa(value.DaemonReadCount),
		strconv.Itoa(value.DaemonWriteCount), strconv.Itoa(value.ReconciledResourceCount),
		strconv.FormatBool(value.DaemonConsumed), strconv.FormatBool(value.ReadbackVerified),
		strconv.FormatBool(value.FinalMountReadOnly), strconv.FormatBool(value.CarrierRemoved),
		strconv.FormatBool(value.FinalContainerRemoved), strconv.FormatBool(value.VolumeRemoved),
		strconv.FormatBool(value.CleanupConfirmed), strconv.FormatBool(value.ContainerStarted),
		strconv.FormatBool(value.ProcessExecuted), strconv.FormatBool(value.OutputExported),
		strconv.FormatBool(value.RawContentRetained),
		strconv.FormatBool(value.ProductionExecutionSubmitted),
		strconv.FormatBool(value.ProductionVerified), strconv.FormatBool(value.BackendEnabled),
		strconv.FormatBool(value.ExecutionAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized))
}

type DockerHostInputHandoff struct {
	ID                 string
	IntentID           string
	AttemptID          string
	PlanID             string
	RunID              string
	ProtocolVersion    string
	LeaseGeneration    int64
	HandoffFingerprint string
	Result             DockerHostInputHandoffResult
	CreatedAt          time.Time
}

func NewDockerHostInputHandoff(id string, intent DockerHostInputHandoffIntent,
	leaseGeneration int64, result DockerHostInputHandoffResult, now time.Time,
) (DockerHostInputHandoff, error) {
	value := DockerHostInputHandoff{ID: id, IntentID: intent.ID, AttemptID: intent.AttemptID,
		PlanID: intent.PlanID, RunID: intent.RunID,
		ProtocolVersion: DockerHostInputHandoffProtocolVersion,
		LeaseGeneration: leaseGeneration, Result: result, CreatedAt: now.UTC()}
	value.HandoffFingerprint = dockerHostInputHandoffFingerprint(value)
	if intent.Validate() != nil || result.Validate() != nil ||
		result.IntentFingerprint != intent.IntentFingerprint ||
		result.BundleReportFingerprint != intent.BundleReportFingerprint ||
		result.BundleDigest != intent.BundleDigest || value.Validate() != nil {
		return DockerHostInputHandoff{}, errors.New("docker host input handoff authority is invalid")
	}
	return value, nil
}

func (value DockerHostInputHandoff) Validate() error {
	for _, identity := range []string{value.ID, value.IntentID, value.AttemptID,
		value.PlanID, value.RunID} {
		if validateStoredIdentity("Docker host input handoff identity", identity) != nil {
			return errors.New("docker host input handoff identity is invalid")
		}
	}
	if value.ProtocolVersion != DockerHostInputHandoffProtocolVersion ||
		value.LeaseGeneration < 1 || value.Result.Validate() != nil || value.CreatedAt.IsZero() ||
		!validDigest(value.HandoffFingerprint) ||
		value.HandoffFingerprint != dockerHostInputHandoffFingerprint(value) {
		return errors.New("docker host input handoff is invalid")
	}
	return nil
}

func dockerHostInputHandoffFingerprint(value DockerHostInputHandoff) string {
	return fingerprint(DockerHostInputHandoffProtocolVersion, value.IntentID, value.AttemptID,
		value.PlanID, value.RunID, strconv.FormatInt(value.LeaseGeneration, 10),
		value.Result.TransportFingerprint)
}

type DockerHostInputHandoffRecord struct {
	Intent   DockerHostInputHandoffIntent
	Handoff  *DockerHostInputHandoff
	Replayed bool
}

func (value DockerHostInputHandoffRecord) Validate() error {
	if value.Intent.Validate() != nil {
		return errors.New("docker host input handoff record intent is invalid")
	}
	if value.Handoff != nil && (value.Handoff.Validate() != nil ||
		value.Handoff.IntentID != value.Intent.ID ||
		value.Handoff.AttemptID != value.Intent.AttemptID ||
		value.Handoff.PlanID != value.Intent.PlanID || value.Handoff.RunID != value.Intent.RunID ||
		value.Handoff.Result.IntentFingerprint != value.Intent.IntentFingerprint ||
		value.Handoff.LeaseGeneration < value.Intent.PreparedGeneration) {
		return errors.New("docker host input handoff record result is invalid")
	}
	return nil
}

type DockerHostInputHandoffError struct{ code string }

func (err *DockerHostInputHandoffError) Error() string {
	return "docker host input handoff failed: " + err.code
}

func newDockerHostInputHandoffError(code string) error {
	return &DockerHostInputHandoffError{code: code}
}

func DockerHostInputHandoffErrorCode(err error) string {
	var value *DockerHostInputHandoffError
	if errors.As(err, &value) {
		return value.code
	}
	return ""
}

type DockerHostInputHandoffTransport interface {
	Endpoint() DockerObservationEndpoint
	Handoff(ctx context.Context, request DockerHostInputHandoffRequest,
		bundle HostInputBundle) (DockerHostInputHandoffResult, error)
}

type UnavailableDockerHostInputHandoffTransport struct {
	endpoint DockerObservationEndpoint
	code     string
}

func NewUnavailableDockerHostInputHandoffTransport() UnavailableDockerHostInputHandoffTransport {
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	return UnavailableDockerHostInputHandoffTransport{endpoint: endpoint,
		code: DockerHostInputHandoffErrorDisabled}
}

func newUnsupportedDockerHostInputHandoffTransport() UnavailableDockerHostInputHandoffTransport {
	value := NewUnavailableDockerHostInputHandoffTransport()
	value.code = DockerHostInputHandoffErrorUnsupported
	return value
}

func (value UnavailableDockerHostInputHandoffTransport) Endpoint() DockerObservationEndpoint {
	return value.endpoint
}

func (value UnavailableDockerHostInputHandoffTransport) Handoff(ctx context.Context,
	_ DockerHostInputHandoffRequest, _ HostInputBundle,
) (DockerHostInputHandoffResult, error) {
	if err := ctx.Err(); err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	code := value.code
	if code != DockerHostInputHandoffErrorUnsupported {
		code = DockerHostInputHandoffErrorDisabled
	}
	return DockerHostInputHandoffResult{}, newDockerHostInputHandoffError(code)
}

func validDockerHostInputCarrierName(value string) bool {
	const prefix = "cyberagent-carrier-"
	return len(value) == len(prefix)+20 && strings.HasPrefix(value, prefix) &&
		validLowerHex(strings.TrimPrefix(value, prefix), 20)
}

func validDockerHostInputVolumeName(value string) bool {
	const prefix = "cyberagent-input-"
	return len(value) == len(prefix)+24 && strings.HasPrefix(value, prefix) &&
		validLowerHex(strings.TrimPrefix(value, prefix), 24)
}

func validLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func readExactHostInputBundle(bundle HostInputBundle, report HostInputBundleReport) ([]byte, error) {
	if bundle == nil || report.Validate() != nil {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	bundleReport := bundle.Report()
	if bundleReport.Validate() != nil ||
		bundleReport.ReportFingerprint != report.ReportFingerprint {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	if _, err := bundle.Seek(0, io.SeekStart); err != nil {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	data, err := io.ReadAll(io.LimitReader(bundle, MaxHostInputBundleBytes+1))
	if err != nil || int64(len(data)) != report.BundleBytes ||
		int64(len(data)) > MaxHostInputBundleBytes || hashHostInputBytes(data) != report.BundleDigest {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	return data, nil
}
