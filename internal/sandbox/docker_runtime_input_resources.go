package sandbox

import (
	"context"
	"errors"
	"strconv"
	"time"
)

const (
	DockerRuntimeInputResourceDescriptorProtocolVersion     = "sandbox_docker_runtime_input_resource_descriptor.v1"
	DockerRuntimeInputResourceInspectionProtocolVersion     = "sandbox_docker_runtime_input_resource_inspection.v1"
	DockerRuntimeInputResourceInspectionOperationVersion    = "sandbox_docker_runtime_input_resource_inspection_operation.v1"
	DockerRuntimeInputResourceCleanupIntentProtocolVersion  = "sandbox_docker_runtime_input_resource_cleanup_intent.v1"
	DockerRuntimeInputResourceCleanupResultProtocolVersion  = "sandbox_docker_runtime_input_resource_cleanup_result.v1"
	DockerRuntimeInputResourceCleanupFailureProtocolVersion = "sandbox_docker_runtime_input_resource_cleanup_failure.v1"
	DockerRuntimeInputResourceCleanupOperationVersion       = "sandbox_docker_runtime_input_resource_cleanup_operation.v1"

	DockerRuntimeInputResourceTargetOwned   = "exact_owned_present"
	DockerRuntimeInputResourceTargetAbsent  = "absent"
	DockerRuntimeInputResourceTargetForeign = "foreign_or_changed"

	DockerRuntimeInputResourceInspectionComplete = "exact_owned_resources_present"
	DockerRuntimeInputResourceInspectionPartial  = "exact_owned_resources_partial_or_absent"
	DockerRuntimeInputResourceInspectionUnsafe   = "unsafe_resource_collision"

	DockerRuntimeInputResourceInspectionTrustComplete = "exact_owned_readonly_never_started"
	DockerRuntimeInputResourceInspectionTrustPartial  = "partial_exact_owned_or_absent"
	DockerRuntimeInputResourceInspectionTrustUnsafe   = "foreign_or_changed_resource_detected"

	DockerRuntimeInputResourceCleanupStatusComplete = "exact_owned_resources_absent"
	DockerRuntimeInputResourceCleanupTrustClass     = "exact_owned_cleanup_reverified"
	DockerRuntimeInputResourceCleanupLeaseActive    = "active"
	DockerRuntimeInputResourceCleanupLeaseReleased  = "released"

	DockerRuntimeInputResourceErrorDisabled        = "resource_lifecycle_disabled"
	DockerRuntimeInputResourceErrorUnsupported     = "resource_lifecycle_unsupported"
	DockerRuntimeInputResourceErrorConnection      = "connection_failed"
	DockerRuntimeInputResourceErrorInvalidResponse = "invalid_response"
	DockerRuntimeInputResourceErrorUnsafeCollision = "unsafe_resource_collision"
	DockerRuntimeInputResourceErrorCleanup         = "resource_cleanup_failed"
	DockerRuntimeInputResourceErrorCanceled        = "context_canceled"
	DockerRuntimeInputResourceErrorDeadline        = "deadline_exceeded"

	MaxDockerRuntimeInputResourceCleanupFailures     = 16
	MaxDockerRuntimeInputResourceDaemonReads         = 256
	MaxDockerRuntimeInputResourceDaemonWrites        = MaxDockerRuntimeInputProjections + 1
	DefaultDockerRuntimeInputResourceCleanupLeaseTTL = 10 * time.Minute
	MinDockerRuntimeInputResourceCleanupLeaseTTL     = time.Minute
	MaxDockerRuntimeInputResourceCleanupLeaseTTL     = 30 * time.Minute
)

type DockerRuntimeInputResourceMount struct {
	Ordinal     int
	Item        DockerRuntimeInputProjectionItem
	Target      string
	VolumeName  string
	CarrierName string
}

type DockerRuntimeInputResourceDescriptor struct {
	ProtocolVersion              string
	ApplicationIntentID          string
	ApplicationResultID          string
	RunID                        string
	ManifestFingerprint          string
	IntentFingerprint            string
	ProjectionFingerprint        string
	ApplicationResultFingerprint string
	Spec                         DockerContainerSpec
	WritableMount                DockerHostMount
	WritableMountFingerprint     string
	Mounts                       []DockerRuntimeInputResourceMount
	RequestFingerprint           string
	DescriptorFingerprint        string
}

func NewDockerRuntimeInputResourceDescriptor(application DockerRuntimeInputApplicationRecord,
	projection DockerRuntimeInputProjectionPlan, writeRequest DockerContainerWriteRequest,
) (DockerRuntimeInputResourceDescriptor, error) {
	if application.Validate() != nil || application.Result == nil || application.Replayed ||
		projection.Validate() != nil || projection.Replayed || writeRequest.Validate() != nil ||
		application.Intent.ProjectionID != projection.ID ||
		application.Intent.ProjectionFingerprint != projection.ProjectionFingerprint ||
		application.Result.ProjectionID != projection.ID ||
		application.Result.ResultFingerprint == "" ||
		writeRequest.Spec.SpecFingerprint != application.Intent.SpecFingerprint ||
		writeRequest.Spec.ManifestFingerprint != application.Intent.ManifestFingerprint {
		return DockerRuntimeInputResourceDescriptor{}, errors.New("docker runtime input resource authority is invalid")
	}
	writable := make([]DockerHostMount, 0, 1)
	readOnlyTargets := make(map[int]string, projection.ReadOnlyMountCount)
	readOnlyOrdinal := 0
	for _, planned := range writeRequest.Spec.Mounts {
		if planned.Access == MountReadOnly {
			readOnlyOrdinal++
			readOnlyTargets[readOnlyOrdinal] = planned.Target
			continue
		}
		for _, host := range writeRequest.HostMounts {
			if !host.ReadOnly && host.Target == planned.Target {
				writable = append(writable, host)
			}
		}
	}
	if len(writable) != 1 || readOnlyOrdinal != projection.ReadOnlyMountCount ||
		len(projection.Items) != projection.ProjectionCount {
		return DockerRuntimeInputResourceDescriptor{}, errors.New("docker runtime input resource mounts are incomplete")
	}
	mounts := make([]DockerRuntimeInputResourceMount, len(projection.Items))
	seenTargets := make(map[string]struct{}, len(mounts))
	seenVolumes := make(map[string]struct{}, len(mounts))
	for index, item := range projection.Items {
		target := ""
		switch item.Kind {
		case DockerRuntimeInputProjectionKindManifestMount:
			target = readOnlyTargets[item.ManifestMountOrdinal]
		case DockerRuntimeInputProjectionKindArtifacts:
			target = DockerRuntimeArtifactTarget
		default:
			return DockerRuntimeInputResourceDescriptor{}, errors.New("docker runtime input resource kind is invalid")
		}
		volumeName := dockerRuntimeInputProjectionVolumeName(projection.ManifestFingerprint,
			projection.HandoffFingerprint, projection.BundleDigest, item.Kind,
			item.ManifestMountOrdinal, target)
		seed := fingerprint(DockerRuntimeInputApplicationRequestProtocolVersion,
			application.Intent.IntentFingerprint, item.ItemFingerprint)
		mount := DockerRuntimeInputResourceMount{Ordinal: index + 1, Item: item,
			Target: target, VolumeName: volumeName,
			CarrierName: "cyberagent-runtime-carrier-" + seed[:24]}
		if mount.Validate() != nil || mount.Item.Ordinal != mount.Ordinal {
			return DockerRuntimeInputResourceDescriptor{}, errors.New("docker runtime input resource mount changed")
		}
		if _, exists := seenTargets[target]; exists {
			return DockerRuntimeInputResourceDescriptor{}, errors.New("docker runtime input resource target is duplicated")
		}
		if _, exists := seenVolumes[volumeName]; exists {
			return DockerRuntimeInputResourceDescriptor{}, errors.New("docker runtime input resource volume is duplicated")
		}
		seenTargets[target], seenVolumes[volumeName] = struct{}{}, struct{}{}
		mounts[index] = mount
	}
	value := DockerRuntimeInputResourceDescriptor{
		ProtocolVersion:     DockerRuntimeInputResourceDescriptorProtocolVersion,
		ApplicationIntentID: application.Intent.ID, ApplicationResultID: application.Result.ID,
		RunID: application.Intent.RunID, ManifestFingerprint: application.Intent.ManifestFingerprint,
		IntentFingerprint:            application.Intent.IntentFingerprint,
		ProjectionFingerprint:        application.Intent.ProjectionFingerprint,
		ApplicationResultFingerprint: application.Result.ResultFingerprint,
		Spec:                         writeRequest.Spec, WritableMount: writable[0], Mounts: mounts,
	}
	value.WritableMountFingerprint = dockerHostMountFingerprint([]DockerHostMount{value.WritableMount})
	value.RequestFingerprint = dockerRuntimeInputResourceRequestFingerprint(value)
	value.DescriptorFingerprint = dockerRuntimeInputResourceDescriptorFingerprint(value)
	if value.RequestFingerprint != application.Result.RequestFingerprint {
		return DockerRuntimeInputResourceDescriptor{}, errors.New("docker runtime input resource request changed after application")
	}
	return value, value.Validate()
}

func (value DockerRuntimeInputResourceMount) Validate() error {
	if value.Ordinal < 1 || value.Ordinal > MaxDockerRuntimeInputProjections ||
		value.Item.Validate() != nil || value.Item.Ordinal != value.Ordinal ||
		validateVirtualPath("Docker runtime input resource target", value.Target) != nil ||
		!validDockerRuntimeInputVolumeName(value.VolumeName) ||
		!validDockerRuntimeInputCarrierName(value.CarrierName) ||
		fingerprint("sandbox_docker_runtime_input_target.v1", value.Target) != value.Item.TargetFingerprint ||
		fingerprint("sandbox_docker_runtime_input_volume_name.v1", value.VolumeName) != value.Item.VolumeNameFingerprint {
		return errors.New("docker runtime input resource mount is invalid")
	}
	return nil
}

func (value DockerRuntimeInputResourceDescriptor) Validate() error {
	for _, identity := range []string{value.ApplicationIntentID, value.ApplicationResultID, value.RunID} {
		if validateStoredIdentity("Docker runtime input resource identity", identity) != nil {
			return errors.New("docker runtime input resource identity is invalid")
		}
	}
	for _, digest := range []string{value.ManifestFingerprint, value.IntentFingerprint,
		value.ProjectionFingerprint, value.ApplicationResultFingerprint,
		value.WritableMountFingerprint, value.RequestFingerprint, value.DescriptorFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker runtime input resource digest is invalid")
		}
	}
	if value.ProtocolVersion != DockerRuntimeInputResourceDescriptorProtocolVersion ||
		ValidateDockerContainerRehearsalProfile(value.Spec) != nil ||
		value.Spec.RunID != value.RunID || value.Spec.ManifestFingerprint != value.ManifestFingerprint ||
		value.WritableMount.Validate() != nil || value.WritableMount.ReadOnly ||
		value.WritableMountFingerprint != dockerHostMountFingerprint([]DockerHostMount{value.WritableMount}) ||
		len(value.Mounts) < 1 || len(value.Mounts) > MaxDockerRuntimeInputProjections {
		return errors.New("docker runtime input resource descriptor is invalid")
	}
	seenTargets := make(map[string]struct{}, len(value.Mounts))
	seenVolumes := make(map[string]struct{}, len(value.Mounts))
	for index, mount := range value.Mounts {
		if mount.Validate() != nil || mount.Ordinal != index+1 {
			return errors.New("docker runtime input resource descriptor mount is invalid")
		}
		if _, exists := seenTargets[mount.Target]; exists {
			return errors.New("docker runtime input resource descriptor target is duplicated")
		}
		if _, exists := seenVolumes[mount.VolumeName]; exists {
			return errors.New("docker runtime input resource descriptor volume is duplicated")
		}
		seenTargets[mount.Target], seenVolumes[mount.VolumeName] = struct{}{}, struct{}{}
	}
	if value.RequestFingerprint != dockerRuntimeInputResourceRequestFingerprint(value) ||
		value.DescriptorFingerprint != dockerRuntimeInputResourceDescriptorFingerprint(value) {
		return errors.New("docker runtime input resource descriptor fingerprint is invalid")
	}
	return nil
}

func dockerRuntimeInputResourceRequestFingerprint(value DockerRuntimeInputResourceDescriptor) string {
	parts := []string{DockerRuntimeInputApplicationRequestProtocolVersion,
		value.IntentFingerprint, value.ProjectionFingerprint, value.Spec.SpecFingerprint,
		value.WritableMountFingerprint, strconv.Itoa(len(value.Mounts))}
	for _, mount := range value.Mounts {
		parts = append(parts, mount.Item.ItemFingerprint,
			fingerprint("sandbox_docker_runtime_input_target.v1", mount.Target),
			fingerprint("sandbox_docker_runtime_input_volume_name.v1", mount.VolumeName),
			fingerprint("sandbox_docker_runtime_input_carrier_name.v1", mount.CarrierName),
			mount.Item.ProjectionArchiveDigest)
	}
	return fingerprint(parts...)
}

func dockerRuntimeInputResourceDescriptorFingerprint(value DockerRuntimeInputResourceDescriptor) string {
	parts := []string{DockerRuntimeInputResourceDescriptorProtocolVersion,
		value.ApplicationIntentID, value.ApplicationResultID, value.RunID,
		value.ManifestFingerprint, value.IntentFingerprint, value.ProjectionFingerprint,
		value.ApplicationResultFingerprint, value.Spec.SpecFingerprint,
		value.WritableMountFingerprint, value.RequestFingerprint, strconv.Itoa(len(value.Mounts))}
	for _, mount := range value.Mounts {
		parts = append(parts, mount.Item.ItemFingerprint, mount.Item.TargetFingerprint,
			mount.Item.VolumeNameFingerprint,
			fingerprint("sandbox_docker_runtime_input_carrier_name.v1", mount.CarrierName))
	}
	return fingerprint(parts...)
}

func (value DockerRuntimeInputResourceDescriptor) applicationRequestView() DockerRuntimeInputApplicationRequest {
	mounts := make([]DockerRuntimeInputApplicationMount, len(value.Mounts))
	for index, mount := range value.Mounts {
		mounts[index] = DockerRuntimeInputApplicationMount{Ordinal: mount.Ordinal,
			Item: mount.Item, Target: mount.Target, VolumeName: mount.VolumeName,
			CarrierName: mount.CarrierName}
	}
	return DockerRuntimeInputApplicationRequest{
		ProtocolVersion:   DockerRuntimeInputApplicationRequestProtocolVersion,
		IntentFingerprint: value.IntentFingerprint, ProjectionFingerprint: value.ProjectionFingerprint,
		Spec: value.Spec, WritableMount: value.WritableMount,
		WritableMountFingerprint: value.WritableMountFingerprint,
		Mounts:                   mounts, RequestFingerprint: value.RequestFingerprint,
	}
}

type DockerRuntimeInputResourceObservation struct {
	EndpointClass       string
	EndpointFingerprint string
	TargetState         string
	OwnedVolumeCount    int
	AbsentVolumeCount   int
	ForeignVolumeCount  int
	DaemonReadCount     int
	ObservedAt          time.Time
}

func (value DockerRuntimeInputResourceObservation) Validate(
	descriptor DockerRuntimeInputResourceDescriptor,
) error {
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	if err != nil || descriptor.Validate() != nil || value.EndpointClass != DockerObservationEndpointLocalUnix ||
		value.EndpointFingerprint != endpoint.Fingerprint || !validDockerRuntimeInputResourceTargetState(value.TargetState) ||
		value.OwnedVolumeCount < 0 || value.AbsentVolumeCount < 0 || value.ForeignVolumeCount < 0 ||
		value.OwnedVolumeCount+value.AbsentVolumeCount+value.ForeignVolumeCount != len(descriptor.Mounts) ||
		value.DaemonReadCount != len(descriptor.Mounts)+1 ||
		value.DaemonReadCount > MaxDockerRuntimeInputResourceDaemonReads || value.ObservedAt.IsZero() {
		return errors.New("docker runtime input resource observation is invalid")
	}
	return nil
}

func validDockerRuntimeInputResourceTargetState(value string) bool {
	return value == DockerRuntimeInputResourceTargetOwned || value == DockerRuntimeInputResourceTargetAbsent ||
		value == DockerRuntimeInputResourceTargetForeign
}

type DockerRuntimeInputResourceInspection struct {
	ID                           string
	ApplicationIntentID          string
	ApplicationResultID          string
	ProjectionID                 string
	ContainerPlanID              string
	RunID                        string
	ProtocolVersion              string
	OperationKeyDigest           string
	ManifestFingerprint          string
	DescriptorFingerprint        string
	RequestFingerprint           string
	ApplicationResultFingerprint string
	EndpointClass                string
	EndpointFingerprint          string
	Status                       string
	TrustClass                   string
	TargetState                  string
	ProjectionCount              int
	OwnedVolumeCount             int
	AbsentVolumeCount            int
	ForeignVolumeCount           int
	ForeignResourceCount         int
	DaemonReadCount              int
	Complete                     bool
	CleanupEligible              bool
	OwnedTargetNeverStarted      bool
	AllOwnedVolumesReadOnly      bool
	AllOwnedVolumesNoCopy        bool
	ContainerStartAuthorized     bool
	ProcessExecutionAuthorized   bool
	OutputExportAuthorized       bool
	ArtifactCommitAuthorized     bool
	RequestSemanticFingerprint   string
	InspectionFingerprint        string
	RequestedBy                  string
	CreatedAt                    time.Time
	Replayed                     bool
}

func NewDockerRuntimeInputResourceInspection(id, operationKeyDigest, requestedBy string,
	application DockerRuntimeInputApplicationRecord, descriptor DockerRuntimeInputResourceDescriptor,
	observation DockerRuntimeInputResourceObservation,
) (DockerRuntimeInputResourceInspection, error) {
	if application.Validate() != nil || application.Result == nil || descriptor.Validate() != nil ||
		observation.Validate(descriptor) != nil || application.Intent.ID != descriptor.ApplicationIntentID ||
		application.Result.ID != descriptor.ApplicationResultID ||
		application.Result.ResultFingerprint != descriptor.ApplicationResultFingerprint ||
		application.Result.RequestFingerprint != descriptor.RequestFingerprint ||
		observation.ObservedAt.Before(application.Result.CreatedAt) {
		return DockerRuntimeInputResourceInspection{}, errors.New("docker runtime input resource inspection authority is invalid")
	}
	foreign := observation.ForeignVolumeCount + boolCount(observation.TargetState == DockerRuntimeInputResourceTargetForeign)
	complete := observation.TargetState == DockerRuntimeInputResourceTargetOwned &&
		observation.OwnedVolumeCount == len(descriptor.Mounts) && foreign == 0
	status, trust := DockerRuntimeInputResourceInspectionPartial, DockerRuntimeInputResourceInspectionTrustPartial
	if foreign > 0 {
		status, trust = DockerRuntimeInputResourceInspectionUnsafe, DockerRuntimeInputResourceInspectionTrustUnsafe
	} else if complete {
		status, trust = DockerRuntimeInputResourceInspectionComplete, DockerRuntimeInputResourceInspectionTrustComplete
	}
	value := DockerRuntimeInputResourceInspection{
		ID: id, ApplicationIntentID: application.Intent.ID, ApplicationResultID: application.Result.ID,
		ProjectionID: application.Intent.ProjectionID, ContainerPlanID: application.Intent.ContainerPlanID,
		RunID: application.Intent.RunID, ProtocolVersion: DockerRuntimeInputResourceInspectionProtocolVersion,
		OperationKeyDigest: operationKeyDigest, ManifestFingerprint: descriptor.ManifestFingerprint,
		DescriptorFingerprint:        descriptor.DescriptorFingerprint,
		RequestFingerprint:           descriptor.RequestFingerprint,
		ApplicationResultFingerprint: application.Result.ResultFingerprint,
		EndpointClass:                observation.EndpointClass, EndpointFingerprint: observation.EndpointFingerprint,
		Status: status, TrustClass: trust, TargetState: observation.TargetState,
		ProjectionCount: len(descriptor.Mounts), OwnedVolumeCount: observation.OwnedVolumeCount,
		AbsentVolumeCount:  observation.AbsentVolumeCount,
		ForeignVolumeCount: observation.ForeignVolumeCount, ForeignResourceCount: foreign,
		DaemonReadCount: observation.DaemonReadCount, Complete: complete,
		CleanupEligible:         foreign == 0,
		OwnedTargetNeverStarted: observation.TargetState == DockerRuntimeInputResourceTargetOwned,
		AllOwnedVolumesReadOnly: complete,
		AllOwnedVolumesNoCopy:   complete,
		RequestedBy:             requestedBy, CreatedAt: observation.ObservedAt.UTC(),
	}
	value.RequestSemanticFingerprint = dockerRuntimeInputResourceInspectionRequestFingerprint(value)
	value.InspectionFingerprint = dockerRuntimeInputResourceInspectionFingerprint(value)
	return value, value.Validate()
}

func (value DockerRuntimeInputResourceInspection) Validate() error {
	for _, identity := range []string{value.ID, value.ApplicationIntentID, value.ApplicationResultID,
		value.ProjectionID, value.ContainerPlanID, value.RunID, value.RequestedBy} {
		if validateStoredIdentity("Docker runtime input resource inspection identity", identity) != nil {
			return errors.New("docker runtime input resource inspection identity is invalid")
		}
	}
	for _, digest := range []string{value.OperationKeyDigest, value.ManifestFingerprint,
		value.DescriptorFingerprint, value.RequestFingerprint, value.ApplicationResultFingerprint,
		value.EndpointFingerprint, value.RequestSemanticFingerprint, value.InspectionFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker runtime input resource inspection digest is invalid")
		}
	}
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	if err != nil || value.ProtocolVersion != DockerRuntimeInputResourceInspectionProtocolVersion ||
		value.EndpointClass != DockerObservationEndpointLocalUnix || value.EndpointFingerprint != endpoint.Fingerprint ||
		!validDockerRuntimeInputResourceTargetState(value.TargetState) ||
		value.ProjectionCount < 1 || value.ProjectionCount > MaxDockerRuntimeInputProjections ||
		value.OwnedVolumeCount < 0 || value.AbsentVolumeCount < 0 || value.ForeignVolumeCount < 0 ||
		value.OwnedVolumeCount+value.AbsentVolumeCount+value.ForeignVolumeCount != value.ProjectionCount ||
		value.ForeignResourceCount != value.ForeignVolumeCount+boolCount(value.TargetState == DockerRuntimeInputResourceTargetForeign) ||
		value.DaemonReadCount != value.ProjectionCount+1 ||
		value.ContainerStartAuthorized || value.ProcessExecutionAuthorized ||
		value.OutputExportAuthorized || value.ArtifactCommitAuthorized || value.CreatedAt.IsZero() {
		return errors.New("docker runtime input resource inspection widened authority")
	}
	expectedStatus, expectedTrust := DockerRuntimeInputResourceInspectionPartial,
		DockerRuntimeInputResourceInspectionTrustPartial
	expectedComplete := value.TargetState == DockerRuntimeInputResourceTargetOwned &&
		value.OwnedVolumeCount == value.ProjectionCount && value.ForeignResourceCount == 0
	if value.ForeignResourceCount > 0 {
		expectedStatus, expectedTrust = DockerRuntimeInputResourceInspectionUnsafe,
			DockerRuntimeInputResourceInspectionTrustUnsafe
	} else if expectedComplete {
		expectedStatus, expectedTrust = DockerRuntimeInputResourceInspectionComplete,
			DockerRuntimeInputResourceInspectionTrustComplete
	}
	if value.Status != expectedStatus || value.TrustClass != expectedTrust ||
		value.Complete != expectedComplete || value.CleanupEligible != (value.ForeignResourceCount == 0) ||
		value.OwnedTargetNeverStarted != (value.TargetState == DockerRuntimeInputResourceTargetOwned) ||
		value.AllOwnedVolumesReadOnly != expectedComplete ||
		value.AllOwnedVolumesNoCopy != expectedComplete ||
		value.RequestSemanticFingerprint != dockerRuntimeInputResourceInspectionRequestFingerprint(value) ||
		value.InspectionFingerprint != dockerRuntimeInputResourceInspectionFingerprint(value) {
		return errors.New("docker runtime input resource inspection is inconsistent")
	}
	return nil
}

func dockerRuntimeInputResourceInspectionRequestFingerprint(value DockerRuntimeInputResourceInspection) string {
	return fingerprint(DockerRuntimeInputResourceInspectionProtocolVersion, value.ApplicationIntentID,
		value.ApplicationResultID, value.ProjectionID, value.ContainerPlanID, value.RunID,
		value.OperationKeyDigest, value.ManifestFingerprint, value.DescriptorFingerprint,
		value.RequestFingerprint, value.ApplicationResultFingerprint, value.EndpointClass,
		value.EndpointFingerprint, value.RequestedBy)
}

func dockerRuntimeInputResourceInspectionFingerprint(value DockerRuntimeInputResourceInspection) string {
	return fingerprint(DockerRuntimeInputResourceInspectionProtocolVersion,
		value.RequestSemanticFingerprint, value.Status, value.TrustClass, value.TargetState,
		strconv.Itoa(value.ProjectionCount), strconv.Itoa(value.OwnedVolumeCount),
		strconv.Itoa(value.AbsentVolumeCount), strconv.Itoa(value.ForeignVolumeCount),
		strconv.Itoa(value.ForeignResourceCount), strconv.Itoa(value.DaemonReadCount),
		strconv.FormatBool(value.Complete), strconv.FormatBool(value.CleanupEligible),
		strconv.FormatBool(value.OwnedTargetNeverStarted),
		strconv.FormatBool(value.AllOwnedVolumesReadOnly),
		strconv.FormatBool(value.AllOwnedVolumesNoCopy),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized))
}

type DockerRuntimeInputResourceCleanupIntent struct {
	ID                           string
	InspectionID                 string
	ApplicationIntentID          string
	ApplicationResultID          string
	ProjectionID                 string
	ContainerPlanID              string
	RunID                        string
	ProtocolVersion              string
	OperationKeyDigest           string
	ManifestFingerprint          string
	DescriptorFingerprint        string
	RequestFingerprint           string
	InspectionFingerprint        string
	ApplicationResultFingerprint string
	EndpointClass                string
	EndpointFingerprint          string
	ProjectionCount              int
	OperatorConfirmed            bool
	DaemonWriteConfirmed         bool
	ContainerStartAuthorized     bool
	ProcessExecutionAuthorized   bool
	OutputExportAuthorized       bool
	ArtifactCommitAuthorized     bool
	IntentFingerprint            string
	RequestedBy                  string
	CreatedAt                    time.Time
}

func NewDockerRuntimeInputResourceCleanupIntent(id, operationKeyDigest string,
	inspection DockerRuntimeInputResourceInspection, descriptor DockerRuntimeInputResourceDescriptor,
	endpoint DockerObservationEndpoint, operatorConfirmed, daemonWriteConfirmed bool,
	requestedBy string, now time.Time,
) (DockerRuntimeInputResourceCleanupIntent, error) {
	if inspection.Validate() != nil || descriptor.Validate() != nil || endpoint.Validate() != nil ||
		!inspection.CleanupEligible || inspection.ApplicationIntentID != descriptor.ApplicationIntentID ||
		inspection.ApplicationResultID != descriptor.ApplicationResultID ||
		inspection.DescriptorFingerprint != descriptor.DescriptorFingerprint ||
		inspection.RequestFingerprint != descriptor.RequestFingerprint ||
		inspection.RequestedBy != requestedBy || !operatorConfirmed || !daemonWriteConfirmed ||
		endpoint.Class != DockerObservationEndpointLocalUnix || now.Before(inspection.CreatedAt) {
		return DockerRuntimeInputResourceCleanupIntent{}, errors.New("docker runtime input resource cleanup authority is invalid")
	}
	value := DockerRuntimeInputResourceCleanupIntent{
		ID: id, InspectionID: inspection.ID, ApplicationIntentID: inspection.ApplicationIntentID,
		ApplicationResultID: inspection.ApplicationResultID, ProjectionID: inspection.ProjectionID,
		ContainerPlanID: inspection.ContainerPlanID, RunID: inspection.RunID,
		ProtocolVersion:    DockerRuntimeInputResourceCleanupIntentProtocolVersion,
		OperationKeyDigest: operationKeyDigest, ManifestFingerprint: inspection.ManifestFingerprint,
		DescriptorFingerprint:        descriptor.DescriptorFingerprint,
		RequestFingerprint:           descriptor.RequestFingerprint,
		InspectionFingerprint:        inspection.InspectionFingerprint,
		ApplicationResultFingerprint: inspection.ApplicationResultFingerprint,
		EndpointClass:                endpoint.Class, EndpointFingerprint: endpoint.Fingerprint,
		ProjectionCount: len(descriptor.Mounts), OperatorConfirmed: operatorConfirmed,
		DaemonWriteConfirmed: daemonWriteConfirmed, RequestedBy: requestedBy, CreatedAt: now.UTC(),
	}
	value.IntentFingerprint = dockerRuntimeInputResourceCleanupIntentFingerprint(value)
	return value, value.Validate()
}

func (value DockerRuntimeInputResourceCleanupIntent) Validate() error {
	for _, identity := range []string{value.ID, value.InspectionID, value.ApplicationIntentID,
		value.ApplicationResultID, value.ProjectionID, value.ContainerPlanID, value.RunID, value.RequestedBy} {
		if validateStoredIdentity("Docker runtime input resource cleanup identity", identity) != nil {
			return errors.New("docker runtime input resource cleanup identity is invalid")
		}
	}
	for _, digest := range []string{value.OperationKeyDigest, value.ManifestFingerprint,
		value.DescriptorFingerprint, value.RequestFingerprint, value.InspectionFingerprint,
		value.ApplicationResultFingerprint, value.EndpointFingerprint, value.IntentFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker runtime input resource cleanup digest is invalid")
		}
	}
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	if err != nil || value.ProtocolVersion != DockerRuntimeInputResourceCleanupIntentProtocolVersion ||
		value.EndpointClass != DockerObservationEndpointLocalUnix || value.EndpointFingerprint != endpoint.Fingerprint ||
		value.ProjectionCount < 1 || value.ProjectionCount > MaxDockerRuntimeInputProjections ||
		!value.OperatorConfirmed || !value.DaemonWriteConfirmed || value.ContainerStartAuthorized ||
		value.ProcessExecutionAuthorized || value.OutputExportAuthorized ||
		value.ArtifactCommitAuthorized || value.CreatedAt.IsZero() ||
		value.IntentFingerprint != dockerRuntimeInputResourceCleanupIntentFingerprint(value) {
		return errors.New("docker runtime input resource cleanup intent widened authority")
	}
	return nil
}

func dockerRuntimeInputResourceCleanupIntentFingerprint(value DockerRuntimeInputResourceCleanupIntent) string {
	return fingerprint(DockerRuntimeInputResourceCleanupIntentProtocolVersion, value.InspectionID,
		value.ApplicationIntentID, value.ApplicationResultID, value.ProjectionID,
		value.ContainerPlanID, value.RunID, value.OperationKeyDigest, value.ManifestFingerprint,
		value.DescriptorFingerprint, value.RequestFingerprint, value.InspectionFingerprint,
		value.ApplicationResultFingerprint, value.EndpointClass, value.EndpointFingerprint,
		strconv.Itoa(value.ProjectionCount), strconv.FormatBool(value.OperatorConfirmed),
		strconv.FormatBool(value.DaemonWriteConfirmed),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized), value.RequestedBy)
}

type DockerRuntimeInputResourceCleanupLease struct {
	IntentID   string
	LeaseID    string
	OwnerID    string
	Generation int64
	Status     string
	AcquiredAt time.Time
	ExpiresAt  time.Time
	ReleasedAt *time.Time
}

func (value DockerRuntimeInputResourceCleanupLease) Validate() error {
	if validateStoredIdentity("Docker runtime input resource cleanup lease intent", value.IntentID) != nil ||
		validateStoredIdentity("Docker runtime input resource cleanup lease id", value.LeaseID) != nil ||
		validateStoredIdentity("Docker runtime input resource cleanup lease owner", value.OwnerID) != nil ||
		value.Generation < 1 || (value.Status != DockerRuntimeInputResourceCleanupLeaseActive &&
		value.Status != DockerRuntimeInputResourceCleanupLeaseReleased) || value.AcquiredAt.IsZero() ||
		value.ExpiresAt.IsZero() || !value.ExpiresAt.After(value.AcquiredAt) {
		return errors.New("docker runtime input resource cleanup lease is invalid")
	}
	if value.Status == DockerRuntimeInputResourceCleanupLeaseActive && value.ReleasedAt != nil {
		return errors.New("active Docker runtime input resource cleanup lease cannot be released")
	}
	if value.Status == DockerRuntimeInputResourceCleanupLeaseReleased &&
		(value.ReleasedAt == nil || value.ReleasedAt.Before(value.AcquiredAt)) {
		return errors.New("released Docker runtime input resource cleanup lease requires a release time")
	}
	return nil
}

func (value DockerRuntimeInputResourceCleanupLease) ActiveAt(now time.Time) bool {
	return value.Status == DockerRuntimeInputResourceCleanupLeaseActive && now.Before(value.ExpiresAt)
}

func ValidateDockerRuntimeInputResourceCleanupLeaseTTL(value time.Duration) error {
	if value < MinDockerRuntimeInputResourceCleanupLeaseTTL || value > MaxDockerRuntimeInputResourceCleanupLeaseTTL {
		return errors.New("docker runtime input resource cleanup lease TTL is outside the supported range")
	}
	return nil
}

type DockerRuntimeInputResourceCleanupResult struct {
	ID                           string
	IntentID                     string
	InspectionID                 string
	ApplicationIntentID          string
	ApplicationResultID          string
	RunID                        string
	ProtocolVersion              string
	Status                       string
	TrustClass                   string
	LeaseGeneration              int64
	EndpointClass                string
	EndpointFingerprint          string
	DescriptorFingerprint        string
	RequestFingerprint           string
	ApplicationResultFingerprint string
	ProjectionCount              int
	TotalResourceCount           int
	InitialOwnedResourceCount    int
	InitialAbsentResourceCount   int
	DeleteAttemptCount           int
	FinalAbsentResourceCount     int
	DaemonReadCount              int
	DaemonWriteCount             int
	TargetAbsent                 bool
	AllVolumesAbsent             bool
	ForeignResourceDetected      bool
	ContainerStartAuthorized     bool
	ProcessExecutionAuthorized   bool
	OutputExportAuthorized       bool
	ArtifactCommitAuthorized     bool
	ResultFingerprint            string
	CreatedAt                    time.Time
}

func NewDockerRuntimeInputResourceCleanupResult(id string,
	intent DockerRuntimeInputResourceCleanupIntent, lease DockerRuntimeInputResourceCleanupLease,
	descriptor DockerRuntimeInputResourceDescriptor, initialOwned, initialAbsent,
	deleteAttempts, daemonReads, daemonWrites int, now time.Time,
) (DockerRuntimeInputResourceCleanupResult, error) {
	total := len(descriptor.Mounts) + 1
	if intent.Validate() != nil || lease.Validate() != nil || descriptor.Validate() != nil ||
		lease.IntentID != intent.ID || !lease.ActiveAt(now) ||
		intent.DescriptorFingerprint != descriptor.DescriptorFingerprint ||
		intent.RequestFingerprint != descriptor.RequestFingerprint ||
		initialOwned < 0 || initialAbsent < 0 || initialOwned+initialAbsent != total ||
		deleteAttempts != initialOwned || daemonWrites != deleteAttempts ||
		daemonReads != 2*total || now.Before(intent.CreatedAt) || now.Before(lease.AcquiredAt) {
		return DockerRuntimeInputResourceCleanupResult{}, errors.New("docker runtime input resource cleanup result authority is invalid")
	}
	value := DockerRuntimeInputResourceCleanupResult{
		ID: id, IntentID: intent.ID, InspectionID: intent.InspectionID,
		ApplicationIntentID: intent.ApplicationIntentID,
		ApplicationResultID: intent.ApplicationResultID, RunID: intent.RunID,
		ProtocolVersion: DockerRuntimeInputResourceCleanupResultProtocolVersion,
		Status:          DockerRuntimeInputResourceCleanupStatusComplete,
		TrustClass:      DockerRuntimeInputResourceCleanupTrustClass,
		LeaseGeneration: lease.Generation, EndpointClass: intent.EndpointClass,
		EndpointFingerprint:          intent.EndpointFingerprint,
		DescriptorFingerprint:        intent.DescriptorFingerprint,
		RequestFingerprint:           intent.RequestFingerprint,
		ApplicationResultFingerprint: intent.ApplicationResultFingerprint,
		ProjectionCount:              len(descriptor.Mounts), TotalResourceCount: total,
		InitialOwnedResourceCount: initialOwned, InitialAbsentResourceCount: initialAbsent,
		DeleteAttemptCount: deleteAttempts, FinalAbsentResourceCount: total,
		DaemonReadCount: daemonReads, DaemonWriteCount: daemonWrites,
		TargetAbsent: true, AllVolumesAbsent: true, CreatedAt: now.UTC(),
	}
	value.ResultFingerprint = dockerRuntimeInputResourceCleanupResultFingerprint(value)
	return value, value.Validate()
}

func (value DockerRuntimeInputResourceCleanupResult) Validate() error {
	for _, identity := range []string{value.ID, value.IntentID, value.InspectionID,
		value.ApplicationIntentID, value.ApplicationResultID, value.RunID} {
		if validateStoredIdentity("Docker runtime input resource cleanup result identity", identity) != nil {
			return errors.New("docker runtime input resource cleanup result identity is invalid")
		}
	}
	for _, digest := range []string{value.EndpointFingerprint, value.DescriptorFingerprint,
		value.RequestFingerprint, value.ApplicationResultFingerprint, value.ResultFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker runtime input resource cleanup result digest is invalid")
		}
	}
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	expectedTotal := value.ProjectionCount + 1
	if err != nil || value.ProtocolVersion != DockerRuntimeInputResourceCleanupResultProtocolVersion ||
		value.Status != DockerRuntimeInputResourceCleanupStatusComplete ||
		value.TrustClass != DockerRuntimeInputResourceCleanupTrustClass || value.LeaseGeneration < 1 ||
		value.EndpointClass != DockerObservationEndpointLocalUnix || value.EndpointFingerprint != endpoint.Fingerprint ||
		value.ProjectionCount < 1 || value.ProjectionCount > MaxDockerRuntimeInputProjections ||
		value.TotalResourceCount != expectedTotal || value.InitialOwnedResourceCount < 0 ||
		value.InitialAbsentResourceCount < 0 ||
		value.InitialOwnedResourceCount+value.InitialAbsentResourceCount != expectedTotal ||
		value.DeleteAttemptCount != value.InitialOwnedResourceCount ||
		value.FinalAbsentResourceCount != expectedTotal ||
		value.DaemonReadCount != 2*expectedTotal || value.DaemonReadCount > MaxDockerRuntimeInputResourceDaemonReads ||
		value.DaemonWriteCount != value.DeleteAttemptCount ||
		value.DaemonWriteCount > MaxDockerRuntimeInputResourceDaemonWrites ||
		!value.TargetAbsent || !value.AllVolumesAbsent || value.ForeignResourceDetected ||
		value.ContainerStartAuthorized || value.ProcessExecutionAuthorized ||
		value.OutputExportAuthorized || value.ArtifactCommitAuthorized || value.CreatedAt.IsZero() ||
		value.ResultFingerprint != dockerRuntimeInputResourceCleanupResultFingerprint(value) {
		return errors.New("docker runtime input resource cleanup result widened authority")
	}
	return nil
}

func dockerRuntimeInputResourceCleanupResultFingerprint(value DockerRuntimeInputResourceCleanupResult) string {
	return fingerprint(DockerRuntimeInputResourceCleanupResultProtocolVersion, value.IntentID,
		value.InspectionID, value.ApplicationIntentID, value.ApplicationResultID, value.RunID,
		value.Status, value.TrustClass, strconv.FormatInt(value.LeaseGeneration, 10),
		value.EndpointClass, value.EndpointFingerprint, value.DescriptorFingerprint,
		value.RequestFingerprint, value.ApplicationResultFingerprint,
		strconv.Itoa(value.ProjectionCount), strconv.Itoa(value.TotalResourceCount),
		strconv.Itoa(value.InitialOwnedResourceCount), strconv.Itoa(value.InitialAbsentResourceCount),
		strconv.Itoa(value.DeleteAttemptCount), strconv.Itoa(value.FinalAbsentResourceCount),
		strconv.Itoa(value.DaemonReadCount), strconv.Itoa(value.DaemonWriteCount),
		strconv.FormatBool(value.TargetAbsent), strconv.FormatBool(value.AllVolumesAbsent),
		strconv.FormatBool(value.ForeignResourceDetected),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized))
}

type DockerRuntimeInputResourceCleanupFailure struct {
	IntentID           string
	Sequence           int
	Generation         int64
	ProtocolVersion    string
	Code               string
	FailureFingerprint string
	CreatedAt          time.Time
}

func NewDockerRuntimeInputResourceCleanupFailure(intentID string, sequence int,
	generation int64, code string, now time.Time,
) (DockerRuntimeInputResourceCleanupFailure, error) {
	value := DockerRuntimeInputResourceCleanupFailure{IntentID: intentID, Sequence: sequence,
		Generation: generation, ProtocolVersion: DockerRuntimeInputResourceCleanupFailureProtocolVersion,
		Code: code, CreatedAt: now.UTC()}
	value.FailureFingerprint = fingerprint(DockerRuntimeInputResourceCleanupFailureProtocolVersion,
		intentID, strconv.Itoa(sequence), strconv.FormatInt(generation, 10), code)
	return value, value.Validate()
}

func (value DockerRuntimeInputResourceCleanupFailure) Validate() error {
	if validateStoredIdentity("Docker runtime input resource cleanup failure intent", value.IntentID) != nil ||
		value.Sequence < 1 || value.Sequence > MaxDockerRuntimeInputResourceCleanupFailures ||
		value.Generation < 1 || value.ProtocolVersion != DockerRuntimeInputResourceCleanupFailureProtocolVersion ||
		!validDockerRuntimeInputResourceErrorCode(value.Code) || !validDigest(value.FailureFingerprint) ||
		value.CreatedAt.IsZero() || value.FailureFingerprint != fingerprint(
		DockerRuntimeInputResourceCleanupFailureProtocolVersion, value.IntentID,
		strconv.Itoa(value.Sequence), strconv.FormatInt(value.Generation, 10), value.Code) {
		return errors.New("docker runtime input resource cleanup failure is invalid")
	}
	return nil
}

func validDockerRuntimeInputResourceErrorCode(value string) bool {
	switch value {
	case DockerRuntimeInputResourceErrorDisabled, DockerRuntimeInputResourceErrorUnsupported,
		DockerRuntimeInputResourceErrorConnection, DockerRuntimeInputResourceErrorInvalidResponse,
		DockerRuntimeInputResourceErrorUnsafeCollision, DockerRuntimeInputResourceErrorCleanup,
		DockerRuntimeInputResourceErrorCanceled, DockerRuntimeInputResourceErrorDeadline:
		return true
	default:
		return false
	}
}

type DockerRuntimeInputResourceCleanupRecord struct {
	Intent   DockerRuntimeInputResourceCleanupIntent
	Lease    DockerRuntimeInputResourceCleanupLease
	Result   *DockerRuntimeInputResourceCleanupResult
	Failures []DockerRuntimeInputResourceCleanupFailure
	Replayed bool
	TookOver bool
}

func (value DockerRuntimeInputResourceCleanupRecord) Validate() error {
	if value.Intent.Validate() != nil || value.Lease.Validate() != nil ||
		value.Lease.IntentID != value.Intent.ID || len(value.Failures) > MaxDockerRuntimeInputResourceCleanupFailures {
		return errors.New("docker runtime input resource cleanup record is invalid")
	}
	for index, failure := range value.Failures {
		if failure.Validate() != nil || failure.IntentID != value.Intent.ID || failure.Sequence != index+1 {
			return errors.New("docker runtime input resource cleanup failure sequence is invalid")
		}
	}
	if value.Result == nil {
		return nil
	}
	if value.Result.Validate() != nil || value.Result.IntentID != value.Intent.ID ||
		value.Result.InspectionID != value.Intent.InspectionID ||
		value.Result.ApplicationIntentID != value.Intent.ApplicationIntentID ||
		value.Result.ApplicationResultID != value.Intent.ApplicationResultID ||
		value.Result.RunID != value.Intent.RunID ||
		value.Result.DescriptorFingerprint != value.Intent.DescriptorFingerprint ||
		value.Result.RequestFingerprint != value.Intent.RequestFingerprint ||
		value.Result.LeaseGeneration != value.Lease.Generation ||
		value.Lease.Status != DockerRuntimeInputResourceCleanupLeaseReleased ||
		value.Result.CreatedAt.Before(value.Intent.CreatedAt) {
		return errors.New("docker runtime input resource cleanup result binding is invalid")
	}
	return nil
}

type DockerRuntimeInputResourceCleanupAcquisition struct {
	Record   DockerRuntimeInputResourceCleanupRecord
	Replayed bool
	TookOver bool
}

type DockerRuntimeInputResourceError struct{ code string }

func (value *DockerRuntimeInputResourceError) Error() string {
	return "docker runtime input resource lifecycle failed: " + value.code
}

func newDockerRuntimeInputResourceError(code string) error {
	return &DockerRuntimeInputResourceError{code: code}
}

func DockerRuntimeInputResourceErrorCode(err error) string {
	var value *DockerRuntimeInputResourceError
	if errors.As(err, &value) {
		return value.code
	}
	if errors.Is(err, context.Canceled) {
		return DockerRuntimeInputResourceErrorCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return DockerRuntimeInputResourceErrorDeadline
	}
	return DockerRuntimeInputResourceErrorInvalidResponse
}

type DockerRuntimeInputResourceInspector interface {
	Endpoint() DockerObservationEndpoint
	Inspect(ctx context.Context, descriptor DockerRuntimeInputResourceDescriptor) (
		DockerRuntimeInputResourceObservation, error)
}

type DockerRuntimeInputResourceCleanupTransport interface {
	Endpoint() DockerObservationEndpoint
	Cleanup(ctx context.Context, intent DockerRuntimeInputResourceCleanupIntent,
		lease DockerRuntimeInputResourceCleanupLease,
		descriptor DockerRuntimeInputResourceDescriptor) (DockerRuntimeInputResourceCleanupResult, error)
}

type UnavailableDockerRuntimeInputResourceTransport struct {
	endpoint DockerObservationEndpoint
	code     string
}

func NewUnavailableDockerRuntimeInputResourceTransport() UnavailableDockerRuntimeInputResourceTransport {
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	return UnavailableDockerRuntimeInputResourceTransport{endpoint: endpoint,
		code: DockerRuntimeInputResourceErrorDisabled}
}

func newUnsupportedDockerRuntimeInputResourceTransport() UnavailableDockerRuntimeInputResourceTransport {
	value := NewUnavailableDockerRuntimeInputResourceTransport()
	value.code = DockerRuntimeInputResourceErrorUnsupported
	return value
}

func (value UnavailableDockerRuntimeInputResourceTransport) Endpoint() DockerObservationEndpoint {
	return value.endpoint
}

func (value UnavailableDockerRuntimeInputResourceTransport) Inspect(ctx context.Context,
	_ DockerRuntimeInputResourceDescriptor,
) (DockerRuntimeInputResourceObservation, error) {
	if err := ctx.Err(); err != nil {
		return DockerRuntimeInputResourceObservation{}, err
	}
	return DockerRuntimeInputResourceObservation{}, newDockerRuntimeInputResourceError(value.normalizedCode())
}

func (value UnavailableDockerRuntimeInputResourceTransport) Cleanup(ctx context.Context,
	_ DockerRuntimeInputResourceCleanupIntent, _ DockerRuntimeInputResourceCleanupLease,
	_ DockerRuntimeInputResourceDescriptor,
) (DockerRuntimeInputResourceCleanupResult, error) {
	if err := ctx.Err(); err != nil {
		return DockerRuntimeInputResourceCleanupResult{}, err
	}
	return DockerRuntimeInputResourceCleanupResult{}, newDockerRuntimeInputResourceError(value.normalizedCode())
}

func (value UnavailableDockerRuntimeInputResourceTransport) normalizedCode() string {
	if value.code == DockerRuntimeInputResourceErrorUnsupported {
		return value.code
	}
	return DockerRuntimeInputResourceErrorDisabled
}
