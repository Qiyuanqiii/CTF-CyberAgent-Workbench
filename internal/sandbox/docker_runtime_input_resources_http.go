package sandbox

import (
	"context"
	"errors"
	"strconv"
	"time"
)

const dockerRuntimeInputResourceCleanupLeaseSafety = 15 * time.Second

type dockerRuntimeInputResourceSnapshot struct {
	observation  DockerRuntimeInputResourceObservation
	targetID     string
	ownedVolumes []bool
}

func (transport dockerEngineContainerWriteTransport) inspectRuntimeInputResources(ctx context.Context,
	descriptor DockerRuntimeInputResourceDescriptor,
) (DockerRuntimeInputResourceObservation, error) {
	if err := ctx.Err(); err != nil {
		return DockerRuntimeInputResourceObservation{}, err
	}
	if descriptor.Validate() != nil || transport.doer == nil ||
		transport.endpoint.Class != DockerObservationEndpointLocalUnix {
		return DockerRuntimeInputResourceObservation{}, newDockerRuntimeInputResourceError(
			DockerRuntimeInputResourceErrorUnsupported)
	}
	snapshot, err := transport.inspectDockerRuntimeInputResourceSnapshot(ctx, descriptor)
	if err != nil {
		return DockerRuntimeInputResourceObservation{}, normalizeDockerRuntimeInputResourceError(err)
	}
	return snapshot.observation, nil
}

func (transport dockerEngineContainerWriteTransport) cleanupRuntimeInputResources(ctx context.Context,
	intent DockerRuntimeInputResourceCleanupIntent,
	lease DockerRuntimeInputResourceCleanupLease,
	descriptor DockerRuntimeInputResourceDescriptor,
) (DockerRuntimeInputResourceCleanupResult, error) {
	if err := ctx.Err(); err != nil {
		return DockerRuntimeInputResourceCleanupResult{}, err
	}
	now := time.Now().UTC()
	if intent.Validate() != nil || lease.Validate() != nil || descriptor.Validate() != nil ||
		lease.IntentID != intent.ID || now.Before(lease.AcquiredAt) || !lease.ActiveAt(now) ||
		intent.DescriptorFingerprint != descriptor.DescriptorFingerprint ||
		intent.RequestFingerprint != descriptor.RequestFingerprint || transport.doer == nil ||
		transport.endpoint.Class != DockerObservationEndpointLocalUnix ||
		transport.endpoint.Fingerprint != intent.EndpointFingerprint {
		return DockerRuntimeInputResourceCleanupResult{}, newDockerRuntimeInputResourceError(
			DockerRuntimeInputResourceErrorUnsupported)
	}
	operationDeadline := lease.ExpiresAt.Add(-dockerRuntimeInputResourceCleanupLeaseSafety)
	if !now.Before(operationDeadline) {
		return DockerRuntimeInputResourceCleanupResult{}, newDockerRuntimeInputResourceError(
			DockerRuntimeInputResourceErrorDeadline)
	}
	operationCtx, cancel := context.WithDeadline(ctx, operationDeadline)
	defer cancel()

	initial, err := transport.inspectDockerRuntimeInputResourceSnapshot(operationCtx, descriptor)
	if err != nil {
		return DockerRuntimeInputResourceCleanupResult{}, normalizeDockerRuntimeInputResourceError(err)
	}
	if initial.observation.TargetState == DockerRuntimeInputResourceTargetForeign ||
		initial.observation.ForeignVolumeCount != 0 {
		return DockerRuntimeInputResourceCleanupResult{}, newDockerRuntimeInputResourceError(
			DockerRuntimeInputResourceErrorUnsafeCollision)
	}
	initialOwned := initial.observation.OwnedVolumeCount +
		boolCount(initial.observation.TargetState == DockerRuntimeInputResourceTargetOwned)
	initialAbsent := initial.observation.AbsentVolumeCount +
		boolCount(initial.observation.TargetState == DockerRuntimeInputResourceTargetAbsent)
	deleteAttempts := 0
	if initial.observation.TargetState == DockerRuntimeInputResourceTargetOwned {
		if err := transport.removeRuntimeInputContainer(operationCtx, initial.targetID); err != nil {
			return DockerRuntimeInputResourceCleanupResult{}, normalizeDockerRuntimeInputResourceError(err)
		}
		deleteAttempts++
	}
	for index, owned := range initial.ownedVolumes {
		if !owned {
			continue
		}
		if err := transport.removeRuntimeInputVolume(operationCtx,
			descriptor.Mounts[index].VolumeName); err != nil {
			return DockerRuntimeInputResourceCleanupResult{}, normalizeDockerRuntimeInputResourceError(err)
		}
		deleteAttempts++
	}
	final, err := transport.inspectDockerRuntimeInputResourceSnapshot(operationCtx, descriptor)
	if err != nil {
		return DockerRuntimeInputResourceCleanupResult{}, normalizeDockerRuntimeInputResourceError(err)
	}
	if final.observation.TargetState != DockerRuntimeInputResourceTargetAbsent ||
		final.observation.AbsentVolumeCount != len(descriptor.Mounts) ||
		final.observation.OwnedVolumeCount != 0 || final.observation.ForeignVolumeCount != 0 {
		return DockerRuntimeInputResourceCleanupResult{}, newDockerRuntimeInputResourceError(
			DockerRuntimeInputResourceErrorCleanup)
	}
	resultIDSeed := fingerprint(DockerRuntimeInputResourceCleanupResultProtocolVersion,
		intent.IntentFingerprint, strconv.FormatInt(lease.Generation, 10))
	return NewDockerRuntimeInputResourceCleanupResult(
		"runtime-input-resource-cleanup-result-"+resultIDSeed[:24], intent, lease, descriptor,
		initialOwned, initialAbsent, deleteAttempts,
		initial.observation.DaemonReadCount+final.observation.DaemonReadCount,
		deleteAttempts, time.Now().UTC())
}

func (transport dockerEngineContainerWriteTransport) inspectDockerRuntimeInputResourceSnapshot(
	ctx context.Context, descriptor DockerRuntimeInputResourceDescriptor,
) (dockerRuntimeInputResourceSnapshot, error) {
	request := descriptor.applicationRequestView()
	targetState, targetID := DockerRuntimeInputResourceTargetAbsent, ""
	target, found, err := transport.inspectRuntimeInputContainer(ctx, descriptor.Spec.ContainerName)
	if err != nil {
		return dockerRuntimeInputResourceSnapshot{}, err
	}
	if found {
		if verifyDockerRuntimeInputContainer(target, request, nil) == nil {
			targetState, targetID = DockerRuntimeInputResourceTargetOwned, target.ID
		} else {
			targetState = DockerRuntimeInputResourceTargetForeign
		}
	}
	owned, absent, foreign := 0, 0, 0
	ownedVolumes := make([]bool, len(descriptor.Mounts))
	for index, resourceMount := range descriptor.Mounts {
		volume, volumeFound, inspectErr := transport.inspectRuntimeInputVolume(ctx,
			resourceMount.VolumeName)
		if inspectErr != nil {
			return dockerRuntimeInputResourceSnapshot{}, inspectErr
		}
		if !volumeFound {
			absent++
			continue
		}
		mount := request.Mounts[index]
		if verifyDockerRuntimeInputVolume(volume, request, mount) != nil {
			foreign++
			continue
		}
		owned++
		ownedVolumes[index] = true
	}
	observation := DockerRuntimeInputResourceObservation{
		EndpointClass: transport.endpoint.Class, EndpointFingerprint: transport.endpoint.Fingerprint,
		TargetState: targetState, OwnedVolumeCount: owned, AbsentVolumeCount: absent,
		ForeignVolumeCount: foreign, DaemonReadCount: len(descriptor.Mounts) + 1,
		ObservedAt: time.Now().UTC(),
	}
	if err := observation.Validate(descriptor); err != nil {
		return dockerRuntimeInputResourceSnapshot{}, newDockerRuntimeInputResourceError(
			DockerRuntimeInputResourceErrorInvalidResponse)
	}
	return dockerRuntimeInputResourceSnapshot{observation: observation,
		targetID: targetID, ownedVolumes: ownedVolumes}, nil
}

func normalizeDockerRuntimeInputResourceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var resourceErr *DockerRuntimeInputResourceError
	if errors.As(err, &resourceErr) {
		return err
	}
	var applicationErr *DockerRuntimeInputApplicationError
	if errors.As(err, &applicationErr) {
		switch DockerRuntimeInputApplicationErrorCode(err) {
		case DockerRuntimeInputApplicationErrorConnection:
			return newDockerRuntimeInputResourceError(DockerRuntimeInputResourceErrorConnection)
		case DockerRuntimeInputApplicationErrorUnsafeCollision,
			DockerRuntimeInputApplicationErrorConfigMismatch:
			return newDockerRuntimeInputResourceError(DockerRuntimeInputResourceErrorUnsafeCollision)
		case DockerRuntimeInputApplicationErrorCleanup:
			return newDockerRuntimeInputResourceError(DockerRuntimeInputResourceErrorCleanup)
		default:
			return newDockerRuntimeInputResourceError(DockerRuntimeInputResourceErrorInvalidResponse)
		}
	}
	return newDockerRuntimeInputResourceError(DockerRuntimeInputResourceErrorInvalidResponse)
}
