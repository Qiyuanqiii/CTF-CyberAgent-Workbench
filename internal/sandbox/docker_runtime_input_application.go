package sandbox

import (
	"context"
	"errors"
	"strconv"
	"time"
)

const (
	DockerRuntimeInputApplicationIntentProtocolVersion  = "sandbox_docker_runtime_input_application_intent.v1"
	DockerRuntimeInputApplicationRequestProtocolVersion = "sandbox_docker_runtime_input_application_request.v1"
	DockerRuntimeInputApplicationResultProtocolVersion  = "sandbox_docker_runtime_input_application_result.v1"
	DockerRuntimeInputApplicationFailureProtocolVersion = "sandbox_docker_runtime_input_application_failure.v1"
	DockerRuntimeInputApplicationOperationVersion       = "sandbox_docker_runtime_input_application_operation.v1"
	DockerRuntimeInputApplicationSourceLocal            = "local_unix_projection_volume_application"
	DockerRuntimeInputApplicationStatusComplete         = "volumes_applied_target_never_started"
	DockerRuntimeInputApplicationTrustClass             = "daemon_projection_readback_verified_never_started"
	DockerRuntimeInputApplicationLeaseActive            = "active"
	DockerRuntimeInputApplicationLeaseReleased          = "released"
	DockerRuntimeInputApplicationErrorDisabled          = "application_disabled"
	DockerRuntimeInputApplicationErrorUnsupported       = "application_unsupported"
	DockerRuntimeInputApplicationErrorConnection        = "connection_failed"
	DockerRuntimeInputApplicationErrorInvalidResponse   = "invalid_response"
	DockerRuntimeInputApplicationErrorUnsafeCollision   = "unsafe_resource_collision"
	DockerRuntimeInputApplicationErrorReadbackMismatch  = "projection_readback_mismatch"
	DockerRuntimeInputApplicationErrorConfigMismatch    = "target_configuration_mismatch"
	DockerRuntimeInputApplicationErrorCleanup           = "application_cleanup_failed"
	DockerRuntimeInputApplicationErrorCanceled          = "context_canceled"
	DockerRuntimeInputApplicationErrorDeadline          = "deadline_exceeded"
	MaxDockerRuntimeInputApplicationFailures            = 16
	MaxDockerRuntimeInputApplicationDaemonReads         = 256
	MaxDockerRuntimeInputApplicationDaemonWrites        = 256
	DefaultDockerRuntimeInputApplicationLeaseTTL        = 10 * time.Minute
	MinDockerRuntimeInputApplicationLeaseTTL            = time.Minute
	MaxDockerRuntimeInputApplicationLeaseTTL            = 30 * time.Minute
	DockerRuntimeInputCarrierDestination                = "/cyberagent-input"
)

type DockerRuntimeInputApplicationIntent struct {
	ID                         string
	ProjectionID               string
	HandoffID                  string
	HandoffIntentID            string
	AttemptID                  string
	ContainerPlanID            string
	RunID                      string
	MissionID                  string
	WorkspaceID                string
	ProtocolVersion            string
	OperationKeyDigest         string
	ManifestFingerprint        string
	MountBindingFingerprint    string
	InputArtifactDigest        string
	AuthorityFingerprint       string
	SpecFingerprint            string
	ContainerPlanFingerprint   string
	HandoffFingerprint         string
	ProjectionSetFingerprint   string
	ProjectionFingerprint      string
	EndpointClass              string
	EndpointFingerprint        string
	ProjectionCount            int
	ReadOnlyMountCount         int
	InputArtifactCount         int
	TotalEntryCount            int
	TotalContentBytes          int64
	TotalProjectionBytes       int64
	OperatorConfirmed          bool
	DaemonWriteConfirmed       bool
	ContainerStartAuthorized   bool
	ProcessExecutionAuthorized bool
	OutputExportAuthorized     bool
	ArtifactCommitAuthorized   bool
	IntentFingerprint          string
	RequestedBy                string
	CreatedAt                  time.Time
}

func NewDockerRuntimeInputApplicationIntent(id, operationKeyDigest string,
	projection DockerRuntimeInputProjectionPlan, endpoint DockerObservationEndpoint,
	operatorConfirmed, daemonWriteConfirmed bool, requestedBy string, now time.Time,
) (DockerRuntimeInputApplicationIntent, error) {
	if projection.Validate() != nil || projection.Replayed || endpoint.Validate() != nil ||
		endpoint.Class != DockerObservationEndpointLocalUnix ||
		projection.Status != DockerRuntimeInputProjectionStatusCompiled ||
		!operatorConfirmed || !daemonWriteConfirmed || requestedBy != projection.RequestedBy ||
		now.Before(projection.CreatedAt) {
		return DockerRuntimeInputApplicationIntent{}, errors.New("docker runtime input application authority is invalid")
	}
	value := DockerRuntimeInputApplicationIntent{
		ID: id, ProjectionID: projection.ID, HandoffID: projection.HandoffID,
		HandoffIntentID: projection.HandoffIntentID, AttemptID: projection.AttemptID,
		ContainerPlanID: projection.ContainerPlanID, RunID: projection.RunID,
		MissionID: projection.MissionID, WorkspaceID: projection.WorkspaceID,
		ProtocolVersion:          DockerRuntimeInputApplicationIntentProtocolVersion,
		OperationKeyDigest:       operationKeyDigest,
		ManifestFingerprint:      projection.ManifestFingerprint,
		MountBindingFingerprint:  projection.MountBindingFingerprint,
		InputArtifactDigest:      projection.InputArtifactDigest,
		AuthorityFingerprint:     projection.AuthorityFingerprint,
		SpecFingerprint:          projection.SpecFingerprint,
		ContainerPlanFingerprint: projection.ContainerPlanFingerprint,
		HandoffFingerprint:       projection.HandoffFingerprint,
		ProjectionSetFingerprint: projection.ProjectionSetFingerprint,
		ProjectionFingerprint:    projection.ProjectionFingerprint,
		EndpointClass:            endpoint.Class, EndpointFingerprint: endpoint.Fingerprint,
		ProjectionCount:      projection.ProjectionCount,
		ReadOnlyMountCount:   projection.ReadOnlyMountCount,
		InputArtifactCount:   projection.InputArtifactCount,
		TotalEntryCount:      projection.TotalEntryCount,
		TotalContentBytes:    projection.TotalContentBytes,
		TotalProjectionBytes: projection.TotalProjectionBytes,
		OperatorConfirmed:    operatorConfirmed, DaemonWriteConfirmed: daemonWriteConfirmed,
		RequestedBy: requestedBy, CreatedAt: now.UTC(),
	}
	value.IntentFingerprint = dockerRuntimeInputApplicationIntentFingerprint(value)
	return value, value.Validate()
}

func (value DockerRuntimeInputApplicationIntent) Validate() error {
	for _, identity := range []string{value.ID, value.ProjectionID, value.HandoffID,
		value.HandoffIntentID, value.AttemptID, value.ContainerPlanID, value.RunID,
		value.MissionID, value.WorkspaceID, value.RequestedBy} {
		if validateStoredIdentity("Docker runtime input application identity", identity) != nil {
			return errors.New("docker runtime input application identity is invalid")
		}
	}
	for _, digest := range []string{value.OperationKeyDigest, value.ManifestFingerprint,
		value.MountBindingFingerprint, value.InputArtifactDigest, value.AuthorityFingerprint,
		value.SpecFingerprint, value.ContainerPlanFingerprint, value.HandoffFingerprint,
		value.ProjectionSetFingerprint, value.ProjectionFingerprint,
		value.EndpointFingerprint, value.IntentFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker runtime input application digest is invalid")
		}
	}
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	if err != nil || value.ProtocolVersion != DockerRuntimeInputApplicationIntentProtocolVersion ||
		value.EndpointClass != DockerObservationEndpointLocalUnix ||
		value.EndpointFingerprint != endpoint.Fingerprint || value.ProjectionCount < 1 ||
		value.ProjectionCount > MaxDockerRuntimeInputProjections ||
		value.ReadOnlyMountCount < 1 || value.ReadOnlyMountCount > MaxMounts ||
		value.InputArtifactCount < 0 || value.InputArtifactCount > MaxInputArtifacts ||
		value.ProjectionCount != value.ReadOnlyMountCount+boolCount(value.InputArtifactCount > 0) ||
		value.TotalEntryCount < value.ReadOnlyMountCount ||
		value.TotalEntryCount > MaxHostInputBundleEntries || value.TotalContentBytes < 0 ||
		value.TotalContentBytes > MaxHostInputSourceBytes+MaxInputArtifactTotalBytes ||
		value.TotalProjectionBytes < 1 ||
		value.TotalProjectionBytes > int64(MaxDockerRuntimeInputProjections)*MaxHostInputBundleBytes ||
		!value.OperatorConfirmed || !value.DaemonWriteConfirmed || value.ContainerStartAuthorized ||
		value.ProcessExecutionAuthorized || value.OutputExportAuthorized ||
		value.ArtifactCommitAuthorized || value.CreatedAt.IsZero() ||
		value.IntentFingerprint != dockerRuntimeInputApplicationIntentFingerprint(value) {
		return errors.New("docker runtime input application intent widened authority")
	}
	return nil
}

func dockerRuntimeInputApplicationIntentFingerprint(value DockerRuntimeInputApplicationIntent) string {
	return fingerprint(DockerRuntimeInputApplicationIntentProtocolVersion,
		value.ProjectionID, value.HandoffID, value.HandoffIntentID, value.AttemptID,
		value.ContainerPlanID, value.RunID, value.MissionID, value.WorkspaceID,
		value.OperationKeyDigest, value.ManifestFingerprint, value.MountBindingFingerprint,
		value.InputArtifactDigest, value.AuthorityFingerprint, value.SpecFingerprint,
		value.ContainerPlanFingerprint, value.HandoffFingerprint,
		value.ProjectionSetFingerprint, value.ProjectionFingerprint,
		value.EndpointClass, value.EndpointFingerprint, strconv.Itoa(value.ProjectionCount),
		strconv.Itoa(value.ReadOnlyMountCount), strconv.Itoa(value.InputArtifactCount),
		strconv.Itoa(value.TotalEntryCount), strconv.FormatInt(value.TotalContentBytes, 10),
		strconv.FormatInt(value.TotalProjectionBytes, 10),
		strconv.FormatBool(value.OperatorConfirmed), strconv.FormatBool(value.DaemonWriteConfirmed),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized), value.RequestedBy)
}

type DockerRuntimeInputApplicationLease struct {
	IntentID   string
	LeaseID    string
	OwnerID    string
	Generation int64
	Status     string
	AcquiredAt time.Time
	ExpiresAt  time.Time
	ReleasedAt *time.Time
}

func (value DockerRuntimeInputApplicationLease) Validate() error {
	if validateStoredIdentity("Docker runtime input application lease intent", value.IntentID) != nil ||
		validateStoredIdentity("Docker runtime input application lease id", value.LeaseID) != nil ||
		validateStoredIdentity("Docker runtime input application lease owner", value.OwnerID) != nil ||
		value.Generation < 1 || (value.Status != DockerRuntimeInputApplicationLeaseActive &&
		value.Status != DockerRuntimeInputApplicationLeaseReleased) || value.AcquiredAt.IsZero() ||
		value.ExpiresAt.IsZero() || !value.ExpiresAt.After(value.AcquiredAt) {
		return errors.New("docker runtime input application lease is invalid")
	}
	if value.Status == DockerRuntimeInputApplicationLeaseActive && value.ReleasedAt != nil {
		return errors.New("active Docker runtime input application lease cannot be released")
	}
	if value.Status == DockerRuntimeInputApplicationLeaseReleased &&
		(value.ReleasedAt == nil || value.ReleasedAt.Before(value.AcquiredAt)) {
		return errors.New("released Docker runtime input application lease requires a release time")
	}
	return nil
}

func (value DockerRuntimeInputApplicationLease) ActiveAt(now time.Time) bool {
	return value.Status == DockerRuntimeInputApplicationLeaseActive && now.Before(value.ExpiresAt)
}

func ValidateDockerRuntimeInputApplicationLeaseTTL(value time.Duration) error {
	if value < MinDockerRuntimeInputApplicationLeaseTTL || value > MaxDockerRuntimeInputApplicationLeaseTTL {
		return errors.New("docker runtime input application lease TTL is outside the supported range")
	}
	return nil
}

type DockerRuntimeInputApplicationMount struct {
	Ordinal     int
	Item        DockerRuntimeInputProjectionItem
	Target      string
	VolumeName  string
	CarrierName string
	Archive     []byte
}

type DockerRuntimeInputApplicationRequest struct {
	ProtocolVersion          string
	IntentFingerprint        string
	ProjectionFingerprint    string
	Spec                     DockerContainerSpec
	WritableMount            DockerHostMount
	WritableMountFingerprint string
	Mounts                   []DockerRuntimeInputApplicationMount
	RequestFingerprint       string
}

func NewDockerRuntimeInputApplicationRequest(intent DockerRuntimeInputApplicationIntent,
	projection DockerRuntimeInputProjectionPlan, compilation DockerRuntimeInputProjectionCompilation,
	writeRequest DockerContainerWriteRequest,
) (DockerRuntimeInputApplicationRequest, error) {
	if intent.Validate() != nil || projection.Validate() != nil || compilation.Validate() != nil ||
		writeRequest.Validate() != nil || intent.ProjectionID != projection.ID ||
		intent.ProjectionFingerprint != projection.ProjectionFingerprint ||
		intent.IntentFingerprint == "" || writeRequest.Spec.SpecFingerprint != intent.SpecFingerprint ||
		!dockerRuntimeInputCompilationMatchesPlan(compilation, projection) {
		return DockerRuntimeInputApplicationRequest{}, errors.New("docker runtime input application request authority is invalid")
	}
	writable := make([]DockerHostMount, 0, 1)
	for _, mount := range writeRequest.HostMounts {
		if !mount.ReadOnly {
			writable = append(writable, mount)
		}
	}
	if len(writable) != 1 {
		return DockerRuntimeInputApplicationRequest{}, errors.New("docker runtime input application requires one writable output mount")
	}
	mounts := make([]DockerRuntimeInputApplicationMount, len(compilation.Archives))
	for index, archive := range compilation.Archives {
		item := compilation.Items[index]
		seed := fingerprint(DockerRuntimeInputApplicationRequestProtocolVersion,
			intent.IntentFingerprint, item.ItemFingerprint)
		mounts[index] = DockerRuntimeInputApplicationMount{Ordinal: index + 1, Item: item,
			Target: archive.Target, VolumeName: archive.VolumeName,
			CarrierName: "cyberagent-runtime-carrier-" + seed[:24],
			Archive:     append([]byte(nil), archive.Data...)}
	}
	value := DockerRuntimeInputApplicationRequest{
		ProtocolVersion:       DockerRuntimeInputApplicationRequestProtocolVersion,
		IntentFingerprint:     intent.IntentFingerprint,
		ProjectionFingerprint: projection.ProjectionFingerprint,
		Spec:                  writeRequest.Spec, WritableMount: writable[0], Mounts: mounts,
	}
	value.WritableMountFingerprint = dockerHostMountFingerprint([]DockerHostMount{value.WritableMount})
	value.RequestFingerprint = dockerRuntimeInputApplicationRequestFingerprint(value)
	return value, value.Validate()
}

func (value DockerRuntimeInputApplicationRequest) Validate() error {
	if value.ProtocolVersion != DockerRuntimeInputApplicationRequestProtocolVersion ||
		!validDigest(value.IntentFingerprint) || !validDigest(value.ProjectionFingerprint) ||
		!validDigest(value.WritableMountFingerprint) || !validDigest(value.RequestFingerprint) ||
		ValidateDockerContainerRehearsalProfile(value.Spec) != nil ||
		value.WritableMount.Validate() != nil || value.WritableMount.ReadOnly ||
		value.WritableMountFingerprint != dockerHostMountFingerprint([]DockerHostMount{value.WritableMount}) ||
		len(value.Mounts) < 1 || len(value.Mounts) > MaxDockerRuntimeInputProjections {
		return errors.New("docker runtime input application request is invalid")
	}
	readOnlyTargets := make(map[string]int)
	readOnlyOrdinal := 0
	for _, planned := range value.Spec.Mounts {
		if planned.Access == MountReadOnly {
			readOnlyOrdinal++
			readOnlyTargets[planned.Target] = readOnlyOrdinal
		} else if planned.Target != value.WritableMount.Target ||
			planned.Propagation != value.WritableMount.Propagation {
			return errors.New("docker runtime input writable mount changed")
		}
	}
	seenTargets := make(map[string]struct{}, len(value.Mounts))
	seenVolumes := make(map[string]struct{}, len(value.Mounts))
	seenCarriers := make(map[string]struct{}, len(value.Mounts))
	for index, mount := range value.Mounts {
		if mount.Ordinal != index+1 || mount.Item.Validate() != nil ||
			mount.Item.Ordinal != mount.Ordinal || validateVirtualPath("Docker runtime input target", mount.Target) != nil ||
			!validDockerRuntimeInputVolumeName(mount.VolumeName) ||
			!validDockerRuntimeInputCarrierName(mount.CarrierName) || len(mount.Archive) == 0 ||
			int64(len(mount.Archive)) != mount.Item.ProjectionArchiveBytes ||
			hashHostInputBytes(mount.Archive) != mount.Item.ProjectionArchiveDigest ||
			fingerprint("sandbox_docker_runtime_input_target.v1", mount.Target) != mount.Item.TargetFingerprint ||
			fingerprint("sandbox_docker_runtime_input_volume_name.v1", mount.VolumeName) != mount.Item.VolumeNameFingerprint {
			return errors.New("docker runtime input application mount is invalid")
		}
		if _, exists := seenTargets[mount.Target]; exists {
			return errors.New("docker runtime input application target is duplicated")
		}
		if _, exists := seenVolumes[mount.VolumeName]; exists {
			return errors.New("docker runtime input application volume is duplicated")
		}
		if _, exists := seenCarriers[mount.CarrierName]; exists {
			return errors.New("docker runtime input application carrier is duplicated")
		}
		seenTargets[mount.Target], seenVolumes[mount.VolumeName], seenCarriers[mount.CarrierName] = struct{}{}, struct{}{}, struct{}{}
		switch mount.Item.Kind {
		case DockerRuntimeInputProjectionKindManifestMount:
			ordinal, ok := readOnlyTargets[mount.Target]
			if !ok || ordinal != mount.Item.ManifestMountOrdinal {
				return errors.New("docker runtime input application Manifest target changed")
			}
			delete(readOnlyTargets, mount.Target)
		case DockerRuntimeInputProjectionKindArtifacts:
			if mount.Target != DockerRuntimeArtifactTarget {
				return errors.New("docker runtime input Artifact target changed")
			}
		default:
			return errors.New("docker runtime input application kind is invalid")
		}
		if pathWithin(mount.Target, value.WritableMount.Target) ||
			pathWithin(value.WritableMount.Target, mount.Target) {
			return errors.New("docker runtime input application target overlaps writable output")
		}
	}
	if len(readOnlyTargets) != 0 || value.RequestFingerprint != dockerRuntimeInputApplicationRequestFingerprint(value) {
		return errors.New("docker runtime input application request fingerprint is invalid")
	}
	return nil
}

func dockerRuntimeInputApplicationRequestFingerprint(value DockerRuntimeInputApplicationRequest) string {
	parts := []string{DockerRuntimeInputApplicationRequestProtocolVersion,
		value.IntentFingerprint, value.ProjectionFingerprint, value.Spec.SpecFingerprint,
		value.WritableMountFingerprint, strconv.Itoa(len(value.Mounts))}
	for _, mount := range value.Mounts {
		parts = append(parts, mount.Item.ItemFingerprint,
			fingerprint("sandbox_docker_runtime_input_target.v1", mount.Target),
			fingerprint("sandbox_docker_runtime_input_volume_name.v1", mount.VolumeName),
			fingerprint("sandbox_docker_runtime_input_carrier_name.v1", mount.CarrierName),
			hashHostInputBytes(mount.Archive))
	}
	return fingerprint(parts...)
}

func dockerRuntimeInputCompilationMatchesPlan(compilation DockerRuntimeInputProjectionCompilation,
	plan DockerRuntimeInputProjectionPlan,
) bool {
	if compilation.ManifestFingerprint != plan.ManifestFingerprint ||
		compilation.RuntimeBindingFingerprint != plan.HandoffFingerprint ||
		compilation.BundleReportFingerprint != plan.BundleReportFingerprint ||
		compilation.BundleDigest != plan.BundleDigest || compilation.BundleBytes != plan.BundleBytes ||
		compilation.ReadOnlyMountCount != plan.ReadOnlyMountCount ||
		compilation.InputArtifactCount != plan.InputArtifactCount ||
		compilation.DirectoryRootCount != plan.DirectoryRootCount ||
		compilation.FileRootCount != plan.FileRootCount ||
		compilation.TotalEntryCount != plan.TotalEntryCount ||
		compilation.TotalContentBytes != plan.TotalContentBytes ||
		compilation.TotalProjectionBytes != plan.TotalProjectionBytes ||
		compilation.ProjectionSetFingerprint != plan.ProjectionSetFingerprint ||
		len(compilation.Items) != len(plan.Items) {
		return false
	}
	for index := range compilation.Items {
		if compilation.Items[index].ItemFingerprint != plan.Items[index].ItemFingerprint {
			return false
		}
	}
	return true
}

type DockerRuntimeInputApplicationResult struct {
	ID                           string
	IntentID                     string
	ProjectionID                 string
	ContainerPlanID              string
	RunID                        string
	ProtocolVersion              string
	Source                       string
	Status                       string
	TrustClass                   string
	LeaseGeneration              int64
	EndpointClass                string
	EndpointFingerprint          string
	RequestFingerprint           string
	ProjectionFingerprint        string
	TargetContainerFingerprint   string
	TargetInspectionFingerprint  string
	TransportFingerprint         string
	ProjectionCount              int
	VolumeCreatedCount           int
	VolumePresentCount           int
	CarrierCreatedCount          int
	CarrierRemovedCount          int
	ReadbackVerifiedCount        int
	DaemonReadCount              int
	DaemonWriteCount             int
	ReconciledResourceCount      int
	AllVolumesReadOnly           bool
	AllVolumesNoCopy             bool
	AllProjectionBytesVerified   bool
	TargetConfigurationMatched   bool
	TargetContainerPresent       bool
	ContainerStarted             bool
	ProcessExecuted              bool
	OutputExported               bool
	ProductionExecutionSubmitted bool
	ProductionVerified           bool
	BackendEnabled               bool
	ExecutionAuthorized          bool
	ArtifactCommitAuthorized     bool
	ResultFingerprint            string
	CreatedAt                    time.Time
}

func NewDockerRuntimeInputApplicationResult(id string, intent DockerRuntimeInputApplicationIntent,
	lease DockerRuntimeInputApplicationLease, request DockerRuntimeInputApplicationRequest,
	targetContainerID string, daemonReads, daemonWrites, reconciled int, now time.Time,
) (DockerRuntimeInputApplicationResult, error) {
	if intent.Validate() != nil || lease.Validate() != nil || request.Validate() != nil ||
		lease.IntentID != intent.ID || lease.Status != DockerRuntimeInputApplicationLeaseActive ||
		request.IntentFingerprint != intent.IntentFingerprint || !validDockerContainerID(targetContainerID) ||
		now.Before(intent.CreatedAt) || now.Before(lease.AcquiredAt) {
		return DockerRuntimeInputApplicationResult{}, errors.New("docker runtime input application result authority is invalid")
	}
	count := len(request.Mounts)
	value := DockerRuntimeInputApplicationResult{
		ID: id, IntentID: intent.ID, ProjectionID: intent.ProjectionID,
		ContainerPlanID: intent.ContainerPlanID, RunID: intent.RunID,
		ProtocolVersion: DockerRuntimeInputApplicationResultProtocolVersion,
		Source:          DockerRuntimeInputApplicationSourceLocal,
		Status:          DockerRuntimeInputApplicationStatusComplete,
		TrustClass:      DockerRuntimeInputApplicationTrustClass,
		LeaseGeneration: lease.Generation, EndpointClass: intent.EndpointClass,
		EndpointFingerprint:        intent.EndpointFingerprint,
		RequestFingerprint:         request.RequestFingerprint,
		ProjectionFingerprint:      intent.ProjectionFingerprint,
		TargetContainerFingerprint: fingerprint("sandbox_docker_runtime_input_target_container.v1", targetContainerID),
		TargetInspectionFingerprint: fingerprint("sandbox_docker_runtime_input_target_inspection.v1",
			targetContainerID, request.RequestFingerprint),
		ProjectionCount: count, VolumeCreatedCount: count, VolumePresentCount: count,
		CarrierCreatedCount: count, CarrierRemovedCount: count,
		ReadbackVerifiedCount: count, DaemonReadCount: daemonReads,
		DaemonWriteCount: daemonWrites, ReconciledResourceCount: reconciled,
		AllVolumesReadOnly: true, AllVolumesNoCopy: true,
		AllProjectionBytesVerified: true, TargetConfigurationMatched: true,
		TargetContainerPresent: true, CreatedAt: now.UTC(),
	}
	value.TransportFingerprint = dockerRuntimeInputApplicationTransportFingerprint(value)
	value.ResultFingerprint = dockerRuntimeInputApplicationResultFingerprint(value)
	return value, value.Validate()
}

func (value DockerRuntimeInputApplicationResult) Validate() error {
	for _, identity := range []string{value.ID, value.IntentID, value.ProjectionID,
		value.ContainerPlanID, value.RunID} {
		if validateStoredIdentity("Docker runtime input application result identity", identity) != nil {
			return errors.New("docker runtime input application result identity is invalid")
		}
	}
	for _, digest := range []string{value.EndpointFingerprint, value.RequestFingerprint,
		value.ProjectionFingerprint, value.TargetContainerFingerprint,
		value.TargetInspectionFingerprint, value.TransportFingerprint, value.ResultFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker runtime input application result digest is invalid")
		}
	}
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	expectedReads := 3 + 5*value.ProjectionCount
	expectedWrites := 1 + 4*value.ProjectionCount + value.ReconciledResourceCount
	if err != nil || value.ProtocolVersion != DockerRuntimeInputApplicationResultProtocolVersion ||
		value.Source != DockerRuntimeInputApplicationSourceLocal ||
		value.Status != DockerRuntimeInputApplicationStatusComplete ||
		value.TrustClass != DockerRuntimeInputApplicationTrustClass || value.LeaseGeneration < 1 ||
		value.EndpointClass != DockerObservationEndpointLocalUnix ||
		value.EndpointFingerprint != endpoint.Fingerprint || value.ProjectionCount < 1 ||
		value.ProjectionCount > MaxDockerRuntimeInputProjections ||
		value.VolumeCreatedCount != value.ProjectionCount ||
		value.VolumePresentCount != value.ProjectionCount ||
		value.CarrierCreatedCount != value.ProjectionCount ||
		value.CarrierRemovedCount != value.ProjectionCount ||
		value.ReadbackVerifiedCount != value.ProjectionCount ||
		value.DaemonReadCount != expectedReads ||
		value.DaemonReadCount > MaxDockerRuntimeInputApplicationDaemonReads ||
		value.ReconciledResourceCount < 0 ||
		value.ReconciledResourceCount > 1+2*value.ProjectionCount ||
		value.DaemonWriteCount != expectedWrites ||
		value.DaemonWriteCount > MaxDockerRuntimeInputApplicationDaemonWrites ||
		!value.AllVolumesReadOnly || !value.AllVolumesNoCopy ||
		!value.AllProjectionBytesVerified || !value.TargetConfigurationMatched ||
		!value.TargetContainerPresent || value.ContainerStarted || value.ProcessExecuted ||
		value.OutputExported || value.ProductionExecutionSubmitted || value.ProductionVerified ||
		value.BackendEnabled || value.ExecutionAuthorized || value.ArtifactCommitAuthorized ||
		value.CreatedAt.IsZero() ||
		value.TransportFingerprint != dockerRuntimeInputApplicationTransportFingerprint(value) ||
		value.ResultFingerprint != dockerRuntimeInputApplicationResultFingerprint(value) {
		return errors.New("docker runtime input application result widened execution authority")
	}
	return nil
}

func dockerRuntimeInputApplicationTransportFingerprint(value DockerRuntimeInputApplicationResult) string {
	return fingerprint(DockerRuntimeInputApplicationResultProtocolVersion, value.Source,
		value.EndpointClass, value.EndpointFingerprint, value.RequestFingerprint,
		value.ProjectionFingerprint, value.TargetContainerFingerprint,
		value.TargetInspectionFingerprint, strconv.Itoa(value.ProjectionCount),
		strconv.Itoa(value.VolumeCreatedCount), strconv.Itoa(value.VolumePresentCount),
		strconv.Itoa(value.CarrierCreatedCount), strconv.Itoa(value.CarrierRemovedCount),
		strconv.Itoa(value.ReadbackVerifiedCount), strconv.Itoa(value.DaemonReadCount),
		strconv.Itoa(value.DaemonWriteCount), strconv.Itoa(value.ReconciledResourceCount))
}

func dockerRuntimeInputApplicationResultFingerprint(value DockerRuntimeInputApplicationResult) string {
	return fingerprint(DockerRuntimeInputApplicationResultProtocolVersion, value.IntentID,
		value.ProjectionID, value.ContainerPlanID, value.RunID, value.Status, value.TrustClass,
		strconv.FormatInt(value.LeaseGeneration, 10), value.TransportFingerprint,
		strconv.FormatBool(value.AllVolumesReadOnly), strconv.FormatBool(value.AllVolumesNoCopy),
		strconv.FormatBool(value.AllProjectionBytesVerified),
		strconv.FormatBool(value.TargetConfigurationMatched),
		strconv.FormatBool(value.TargetContainerPresent), strconv.FormatBool(value.ContainerStarted),
		strconv.FormatBool(value.ProcessExecuted), strconv.FormatBool(value.OutputExported),
		strconv.FormatBool(value.ProductionExecutionSubmitted),
		strconv.FormatBool(value.ProductionVerified), strconv.FormatBool(value.BackendEnabled),
		strconv.FormatBool(value.ExecutionAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized))
}

type DockerRuntimeInputApplicationFailure struct {
	IntentID           string
	Sequence           int
	Generation         int64
	ProtocolVersion    string
	Code               string
	FailureFingerprint string
	CreatedAt          time.Time
}

func NewDockerRuntimeInputApplicationFailure(intentID string, sequence int, generation int64,
	code string, now time.Time,
) (DockerRuntimeInputApplicationFailure, error) {
	value := DockerRuntimeInputApplicationFailure{IntentID: intentID, Sequence: sequence,
		Generation: generation, ProtocolVersion: DockerRuntimeInputApplicationFailureProtocolVersion,
		Code: code, CreatedAt: now.UTC()}
	value.FailureFingerprint = fingerprint(DockerRuntimeInputApplicationFailureProtocolVersion,
		intentID, strconv.Itoa(sequence), strconv.FormatInt(generation, 10), code)
	return value, value.Validate()
}

func (value DockerRuntimeInputApplicationFailure) Validate() error {
	if validateStoredIdentity("Docker runtime input application failure intent", value.IntentID) != nil ||
		value.Sequence < 1 || value.Sequence > MaxDockerRuntimeInputApplicationFailures ||
		value.Generation < 1 || value.ProtocolVersion != DockerRuntimeInputApplicationFailureProtocolVersion ||
		!validDockerRuntimeInputApplicationFailureCode(value.Code) ||
		!validDigest(value.FailureFingerprint) || value.CreatedAt.IsZero() ||
		value.FailureFingerprint != fingerprint(DockerRuntimeInputApplicationFailureProtocolVersion,
			value.IntentID, strconv.Itoa(value.Sequence), strconv.FormatInt(value.Generation, 10), value.Code) {
		return errors.New("docker runtime input application failure is invalid")
	}
	return nil
}

func validDockerRuntimeInputApplicationFailureCode(value string) bool {
	switch value {
	case DockerRuntimeInputApplicationErrorDisabled,
		DockerRuntimeInputApplicationErrorUnsupported,
		DockerRuntimeInputApplicationErrorConnection,
		DockerRuntimeInputApplicationErrorInvalidResponse,
		DockerRuntimeInputApplicationErrorUnsafeCollision,
		DockerRuntimeInputApplicationErrorReadbackMismatch,
		DockerRuntimeInputApplicationErrorConfigMismatch,
		DockerRuntimeInputApplicationErrorCleanup,
		DockerRuntimeInputApplicationErrorCanceled,
		DockerRuntimeInputApplicationErrorDeadline:
		return true
	default:
		return false
	}
}

type DockerRuntimeInputApplicationRecord struct {
	Intent   DockerRuntimeInputApplicationIntent
	Lease    DockerRuntimeInputApplicationLease
	Result   *DockerRuntimeInputApplicationResult
	Failures []DockerRuntimeInputApplicationFailure
	Replayed bool
	TookOver bool
}

func (value DockerRuntimeInputApplicationRecord) Validate() error {
	if value.Intent.Validate() != nil || value.Lease.Validate() != nil ||
		value.Lease.IntentID != value.Intent.ID || len(value.Failures) > MaxDockerRuntimeInputApplicationFailures {
		return errors.New("docker runtime input application record is invalid")
	}
	for index, failure := range value.Failures {
		if failure.Validate() != nil || failure.IntentID != value.Intent.ID || failure.Sequence != index+1 {
			return errors.New("docker runtime input application failure sequence is invalid")
		}
	}
	if value.Result == nil {
		return nil
	}
	if value.Result.Validate() != nil || value.Result.IntentID != value.Intent.ID ||
		value.Result.ProjectionID != value.Intent.ProjectionID ||
		value.Result.ContainerPlanID != value.Intent.ContainerPlanID ||
		value.Result.RunID != value.Intent.RunID ||
		value.Result.ProjectionFingerprint != value.Intent.ProjectionFingerprint ||
		value.Result.LeaseGeneration != value.Lease.Generation ||
		value.Lease.Status != DockerRuntimeInputApplicationLeaseReleased ||
		value.Result.CreatedAt.Before(value.Intent.CreatedAt) {
		return errors.New("docker runtime input application result binding is invalid")
	}
	return nil
}

type DockerRuntimeInputApplicationAcquisition struct {
	Record   DockerRuntimeInputApplicationRecord
	Replayed bool
	TookOver bool
}

type DockerRuntimeInputApplicationError struct{ code string }

func (value *DockerRuntimeInputApplicationError) Error() string {
	return "docker runtime input application failed: " + value.code
}

func newDockerRuntimeInputApplicationError(code string) error {
	return &DockerRuntimeInputApplicationError{code: code}
}

func DockerRuntimeInputApplicationErrorCode(err error) string {
	var value *DockerRuntimeInputApplicationError
	if errors.As(err, &value) {
		return value.code
	}
	if errors.Is(err, context.Canceled) {
		return DockerRuntimeInputApplicationErrorCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return DockerRuntimeInputApplicationErrorDeadline
	}
	return DockerRuntimeInputApplicationErrorInvalidResponse
}

type DockerRuntimeInputApplicationTransport interface {
	Endpoint() DockerObservationEndpoint
	Apply(ctx context.Context, intent DockerRuntimeInputApplicationIntent,
		lease DockerRuntimeInputApplicationLease,
		request DockerRuntimeInputApplicationRequest) (DockerRuntimeInputApplicationResult, error)
}

type UnavailableDockerRuntimeInputApplicationTransport struct {
	endpoint DockerObservationEndpoint
	code     string
}

func NewUnavailableDockerRuntimeInputApplicationTransport() UnavailableDockerRuntimeInputApplicationTransport {
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	return UnavailableDockerRuntimeInputApplicationTransport{endpoint: endpoint,
		code: DockerRuntimeInputApplicationErrorDisabled}
}

func newUnsupportedDockerRuntimeInputApplicationTransport() UnavailableDockerRuntimeInputApplicationTransport {
	value := NewUnavailableDockerRuntimeInputApplicationTransport()
	value.code = DockerRuntimeInputApplicationErrorUnsupported
	return value
}

func (value UnavailableDockerRuntimeInputApplicationTransport) Endpoint() DockerObservationEndpoint {
	return value.endpoint
}

func (value UnavailableDockerRuntimeInputApplicationTransport) Apply(ctx context.Context,
	_ DockerRuntimeInputApplicationIntent, _ DockerRuntimeInputApplicationLease,
	_ DockerRuntimeInputApplicationRequest,
) (DockerRuntimeInputApplicationResult, error) {
	if err := ctx.Err(); err != nil {
		return DockerRuntimeInputApplicationResult{}, err
	}
	code := value.code
	if code != DockerRuntimeInputApplicationErrorUnsupported {
		code = DockerRuntimeInputApplicationErrorDisabled
	}
	return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(code)
}

func validDockerRuntimeInputCarrierName(value string) bool {
	const prefix = "cyberagent-runtime-carrier-"
	if len(value) != len(prefix)+24 || len(value) > 63 || value[:len(prefix)] != prefix {
		return false
	}
	return validLowerHex(value[len(prefix):], 24)
}
