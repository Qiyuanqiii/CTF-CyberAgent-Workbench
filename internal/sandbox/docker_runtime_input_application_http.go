package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	dockerRuntimeInputApplicationCleanupTimeout = 15 * time.Second
	dockerRuntimeInputApplicationLeaseSafety    = 2 * dockerRuntimeInputApplicationCleanupTimeout
	maxDockerRuntimeInputReadbackBytes          = int64(MaxHostInputBundleBytes + 1024*1024)
)

func (transport dockerEngineContainerWriteTransport) Apply(ctx context.Context,
	intent DockerRuntimeInputApplicationIntent, lease DockerRuntimeInputApplicationLease,
	request DockerRuntimeInputApplicationRequest,
) (result DockerRuntimeInputApplicationResult, returnedErr error) {
	if err := ctx.Err(); err != nil {
		return DockerRuntimeInputApplicationResult{}, err
	}
	now := time.Now().UTC()
	if intent.Validate() != nil || lease.Validate() != nil || request.Validate() != nil ||
		lease.IntentID != intent.ID || now.Before(lease.AcquiredAt) || !lease.ActiveAt(now) ||
		request.IntentFingerprint != intent.IntentFingerprint || transport.doer == nil ||
		transport.endpoint.Class != DockerObservationEndpointLocalUnix ||
		transport.endpoint.Fingerprint != intent.EndpointFingerprint {
		return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorUnsupported)
	}
	operationDeadline := lease.ExpiresAt.Add(-dockerRuntimeInputApplicationLeaseSafety)
	if !now.Before(operationDeadline) {
		return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorDeadline)
	}
	operationCtx, cancelOperation := context.WithDeadline(ctx, operationDeadline)
	defer cancelOperation()
	ctx = operationCtx
	cleanupPending := true
	defer func() {
		if !cleanupPending {
			return
		}
		if cleanupErr := transport.cleanupDockerRuntimeInputApplication(request); cleanupErr != nil {
			returnedErr = errors.Join(returnedErr,
				newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorCleanup))
		}
	}()

	reads, writes, reconciled := 0, 0, 0
	if err := transport.verifyImageProfile(ctx, request.Spec.ImageDigest); err != nil {
		return DockerRuntimeInputApplicationResult{}, normalizeDockerRuntimeInputApplicationError(err)
	}
	reads++
	target, found, err := transport.inspectRuntimeInputContainer(ctx, request.Spec.ContainerName)
	reads++
	if err != nil {
		return DockerRuntimeInputApplicationResult{}, err
	}
	if found {
		if verifyDockerRuntimeInputContainer(target, request, nil) != nil {
			return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
				DockerRuntimeInputApplicationErrorUnsafeCollision)
		}
		if err := transport.removeRuntimeInputContainer(ctx, target.ID); err != nil {
			return DockerRuntimeInputApplicationResult{}, err
		}
		writes++
		reconciled++
	}

	for index := range request.Mounts {
		mount := &request.Mounts[index]
		carrier, carrierFound, inspectErr := transport.inspectRuntimeInputContainer(ctx,
			mount.CarrierName)
		reads++
		if inspectErr != nil {
			return DockerRuntimeInputApplicationResult{}, inspectErr
		}
		if carrierFound {
			if verifyDockerRuntimeInputContainer(carrier, request, mount) != nil {
				return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
					DockerRuntimeInputApplicationErrorUnsafeCollision)
			}
			if err := transport.removeRuntimeInputContainer(ctx, carrier.ID); err != nil {
				return DockerRuntimeInputApplicationResult{}, err
			}
			writes++
			reconciled++
		}

		volume, volumeFound, inspectErr := transport.inspectRuntimeInputVolume(ctx, mount.VolumeName)
		reads++
		if inspectErr != nil {
			return DockerRuntimeInputApplicationResult{}, inspectErr
		}
		if volumeFound {
			if verifyDockerRuntimeInputVolume(volume, request, *mount) != nil {
				return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
					DockerRuntimeInputApplicationErrorUnsafeCollision)
			}
			if err := transport.removeRuntimeInputVolume(ctx, mount.VolumeName); err != nil {
				return DockerRuntimeInputApplicationResult{}, err
			}
			writes++
			reconciled++
		}
		if err := transport.createRuntimeInputVolume(ctx, request, *mount); err != nil {
			return DockerRuntimeInputApplicationResult{}, err
		}
		writes++
		volume, volumeFound, inspectErr = transport.inspectRuntimeInputVolume(ctx, mount.VolumeName)
		reads++
		if inspectErr != nil || !volumeFound ||
			verifyDockerRuntimeInputVolume(volume, request, *mount) != nil {
			if inspectErr != nil {
				return DockerRuntimeInputApplicationResult{}, inspectErr
			}
			return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
				DockerRuntimeInputApplicationErrorUnsafeCollision)
		}

		carrierID, createErr := transport.createRuntimeInputContainer(ctx, request, mount)
		if createErr != nil {
			return DockerRuntimeInputApplicationResult{}, createErr
		}
		writes++
		carrier, carrierFound, inspectErr = transport.inspectRuntimeInputContainer(ctx, carrierID)
		reads++
		if inspectErr != nil || !carrierFound || carrier.ID != carrierID ||
			verifyDockerRuntimeInputContainer(carrier, request, mount) != nil {
			if inspectErr != nil {
				return DockerRuntimeInputApplicationResult{}, inspectErr
			}
			return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
				DockerRuntimeInputApplicationErrorUnsafeCollision)
		}
		if err := transport.putRuntimeInputArchive(ctx, carrierID, mount.Archive); err != nil {
			return DockerRuntimeInputApplicationResult{}, err
		}
		writes++
		readback, readErr := transport.getRuntimeInputArchive(ctx, carrierID)
		reads++
		if readErr != nil {
			return DockerRuntimeInputApplicationResult{}, readErr
		}
		if verifyRuntimeInputProjectionReadback(mount.Archive, readback) != nil {
			return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
				DockerRuntimeInputApplicationErrorReadbackMismatch)
		}
		if err := transport.removeRuntimeInputContainer(ctx, carrierID); err != nil {
			return DockerRuntimeInputApplicationResult{}, err
		}
		writes++
	}

	targetID, err := transport.createRuntimeInputContainer(ctx, request, nil)
	if err != nil {
		return DockerRuntimeInputApplicationResult{}, err
	}
	writes++
	target, found, err = transport.inspectRuntimeInputContainer(ctx, targetID)
	reads++
	if err != nil || !found || target.ID != targetID ||
		verifyDockerRuntimeInputContainer(target, request, nil) != nil {
		if err != nil {
			return DockerRuntimeInputApplicationResult{}, err
		}
		return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorConfigMismatch)
	}
	resultID := "runtime-input-result-" + request.RequestFingerprint[:24]
	result, err = NewDockerRuntimeInputApplicationResult(resultID, intent, lease, request,
		targetID, reads, writes, reconciled, time.Now().UTC())
	if err != nil {
		return DockerRuntimeInputApplicationResult{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	cleanupPending = false
	return result, nil
}

func runtimeInputApplicationLabels(request DockerRuntimeInputApplicationRequest,
	role string, mount *DockerRuntimeInputApplicationMount,
) map[string]string {
	labels := make(map[string]string, len(request.Spec.Labels)+4)
	for _, label := range request.Spec.Labels {
		labels[label.Name] = label.Value
	}
	labels["cyberagent.workbench.runtime-input.request"] = request.RequestFingerprint
	labels["cyberagent.workbench.runtime-input.projection"] = request.ProjectionFingerprint
	labels["cyberagent.workbench.runtime-input.role"] = role
	if mount != nil {
		labels["cyberagent.workbench.runtime-input.item"] = mount.Item.ItemFingerprint
	}
	return labels
}

func runtimeInputVolumeLabels(request DockerRuntimeInputApplicationRequest,
	mount DockerRuntimeInputApplicationMount,
) map[string]string {
	return map[string]string{
		"cyberagent.workbench.runtime-input.request":    request.RequestFingerprint,
		"cyberagent.workbench.runtime-input.projection": request.ProjectionFingerprint,
		"cyberagent.workbench.runtime-input.item":       mount.Item.ItemFingerprint,
		"cyberagent.workbench.runtime-input.role":       "projection-volume",
	}
}

func runtimeInputContainerPayload(request DockerRuntimeInputApplicationRequest,
	mount *DockerRuntimeInputApplicationMount,
) dockerCreateContainerPayload {
	baseRequest := DockerContainerWriteRequest{Spec: request.Spec,
		HostMounts: []DockerHostMount{request.WritableMount}}
	payload := dockerCreatePayload(baseRequest)
	role := "never-started-target"
	if mount != nil {
		role = "write-only-carrier"
		payload.HostConfig.Mounts = []dockerCreateMount{{Type: "volume",
			Source: mount.VolumeName, Target: DockerRuntimeInputCarrierDestination,
			VolumeOptions: &dockerCreateVolumeOptions{NoCopy: true}}}
	} else {
		payload.HostConfig.Mounts = []dockerCreateMount{{Type: "bind",
			Source: request.WritableMount.Source, Target: request.WritableMount.Target,
			BindOptions: &dockerCreateBindOptions{Propagation: request.WritableMount.Propagation}}}
		for _, current := range request.Mounts {
			payload.HostConfig.Mounts = append(payload.HostConfig.Mounts, dockerCreateMount{
				Type: "volume", Source: current.VolumeName, Target: current.Target, ReadOnly: true,
				VolumeOptions: &dockerCreateVolumeOptions{NoCopy: true}})
		}
	}
	payload.Labels = runtimeInputApplicationLabels(request, role, mount)
	return payload
}

func verifyDockerRuntimeInputContainer(inspection dockerContainerInspection,
	request DockerRuntimeInputApplicationRequest, mount *DockerRuntimeInputApplicationMount,
) error {
	spec := request.Spec
	name, role, expectedMounts := spec.ContainerName, "never-started-target", len(request.Mounts)+1
	if mount != nil {
		name, role, expectedMounts = mount.CarrierName, "write-only-carrier", 1
	}
	if !validDockerContainerID(inspection.ID) || inspection.Name != "/"+name ||
		inspection.Config.Image != spec.ImageDigest || inspection.Config.User != spec.User ||
		inspection.Config.WorkingDir != spec.WorkingDirectory || !inspection.Config.NetworkDisabled ||
		inspection.Config.StopSignal != DockerTerminationSignalGraceful ||
		inspection.Config.AttachStdin || inspection.Config.AttachStdout ||
		inspection.Config.AttachStderr || inspection.Config.OpenStdin || inspection.Config.StdinOnce ||
		inspection.Config.Tty || len(inspection.Config.Env) != 0 ||
		!equalStrings(inspection.Config.Entrypoint, []string{spec.Executable}) ||
		!equalStrings(inspection.Config.Cmd, spec.Arguments) ||
		!equalStringMap(inspection.Config.Labels,
			runtimeInputApplicationLabels(request, role, mount)) ||
		inspection.State.Status != "created" || inspection.State.Running || inspection.State.Paused ||
		inspection.State.Restarting || inspection.State.OOMKilled || inspection.State.Dead ||
		inspection.State.Pid != 0 || !inspection.HostConfig.ReadonlyRootfs ||
		inspection.HostConfig.Privileged || inspection.HostConfig.AutoRemove ||
		inspection.HostConfig.Init == nil || !*inspection.HostConfig.Init ||
		inspection.HostConfig.NetworkMode != DockerNetworkDriverNone ||
		inspection.HostConfig.NanoCPUs != spec.Resources.NanoCPUs ||
		inspection.HostConfig.Memory != spec.Resources.MemoryBytes ||
		inspection.HostConfig.MemorySwap != spec.Resources.MemoryBytes ||
		inspection.HostConfig.PidsLimit != int64(spec.Resources.PIDs) ||
		inspection.HostConfig.RestartPolicy.Name != "no" ||
		inspection.HostConfig.RestartPolicy.MaximumRetryCount != 0 ||
		inspection.HostConfig.LogConfig.Type != "none" || len(inspection.HostConfig.LogConfig.Config) != 0 ||
		len(inspection.HostConfig.SecurityOpt) != 1 ||
		!containsFold(inspection.HostConfig.SecurityOpt, "no-new-privileges") ||
		len(inspection.HostConfig.CapAdd) != 0 || len(inspection.HostConfig.CapDrop) != 1 ||
		!containsFold(inspection.HostConfig.CapDrop, "ALL") || len(inspection.HostConfig.Binds) != 0 ||
		len(inspection.HostConfig.Devices) != 0 || len(inspection.HostConfig.DeviceRequests) != 0 ||
		len(inspection.HostConfig.PortBindings) != 0 || inspection.HostConfig.PublishAllPorts ||
		len(inspection.HostConfig.Mounts) != expectedMounts ||
		len(inspection.Mounts) != expectedMounts {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorConfigMismatch)
	}
	if verifyDockerRuntimeInputHostConfigMounts(inspection.HostConfig.Mounts,
		request, mount) != nil {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorConfigMismatch)
	}
	expectedVolumes := make(map[string]DockerRuntimeInputApplicationMount, len(request.Mounts))
	if mount != nil {
		expectedVolumes[DockerRuntimeInputCarrierDestination] = *mount
	} else {
		for _, current := range request.Mounts {
			expectedVolumes[current.Target] = current
		}
	}
	writableFound := false
	for _, observed := range inspection.Mounts {
		if mount == nil && observed.Destination == request.WritableMount.Target {
			if writableFound || observed.Type != "bind" ||
				observed.Source != request.WritableMount.Source || !observed.RW ||
				observed.Propagation != request.WritableMount.Propagation {
				return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorConfigMismatch)
			}
			writableFound = true
			continue
		}
		expected, ok := expectedVolumes[observed.Destination]
		expectedRW := mount != nil
		if !ok || observed.Type != "volume" || observed.Name != expected.VolumeName ||
			observed.Driver != "local" || observed.RW != expectedRW {
			return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorConfigMismatch)
		}
		delete(expectedVolumes, observed.Destination)
	}
	if len(expectedVolumes) != 0 || (mount == nil && !writableFound) {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorConfigMismatch)
	}
	return nil
}

func verifyDockerRuntimeInputHostConfigMounts(observed []dockerCreateMount,
	request DockerRuntimeInputApplicationRequest, carrier *DockerRuntimeInputApplicationMount,
) error {
	expectedVolumes := make(map[string]DockerRuntimeInputApplicationMount, len(request.Mounts))
	if carrier != nil {
		expectedVolumes[DockerRuntimeInputCarrierDestination] = *carrier
	} else {
		for _, mount := range request.Mounts {
			expectedVolumes[mount.Target] = mount
		}
	}
	writableFound := false
	for _, mount := range observed {
		if carrier == nil && mount.Target == request.WritableMount.Target {
			if writableFound || mount.Type != "bind" ||
				mount.Source != request.WritableMount.Source || mount.ReadOnly ||
				mount.BindOptions == nil ||
				mount.BindOptions.Propagation != request.WritableMount.Propagation ||
				mount.VolumeOptions != nil {
				return errors.New("docker runtime input writable mount changed")
			}
			writableFound = true
			continue
		}
		expected, ok := expectedVolumes[mount.Target]
		expectedReadOnly := carrier == nil
		if !ok || mount.Type != "volume" || mount.Source != expected.VolumeName ||
			mount.ReadOnly != expectedReadOnly || mount.BindOptions != nil ||
			mount.VolumeOptions == nil || !mount.VolumeOptions.NoCopy {
			return errors.New("docker runtime input volume mount changed")
		}
		delete(expectedVolumes, mount.Target)
	}
	if len(expectedVolumes) != 0 || (carrier == nil && !writableFound) {
		return errors.New("docker runtime input mounts are incomplete")
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) inspectRuntimeInputContainer(ctx context.Context,
	reference string,
) (dockerContainerInspection, bool, error) {
	response, err := transport.doRuntimeInputApplication(ctx, http.MethodGet,
		"/v"+DockerContainerWriteAPIVersion+"/containers/"+url.PathEscape(reference)+"/json",
		"", nil, "", true)
	if err != nil {
		return dockerContainerInspection{}, false, err
	}
	if response.status == http.StatusNotFound {
		return dockerContainerInspection{}, false, nil
	}
	var value dockerContainerInspection
	if decodeDockerContainerWriteJSON(response.body, &value) != nil {
		return dockerContainerInspection{}, false,
			newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	return value, true, nil
}

func (transport dockerEngineContainerWriteTransport) createRuntimeInputContainer(ctx context.Context,
	request DockerRuntimeInputApplicationRequest, mount *DockerRuntimeInputApplicationMount,
) (string, error) {
	name := request.Spec.ContainerName
	if mount != nil {
		name = mount.CarrierName
	}
	body, err := json.Marshal(runtimeInputContainerPayload(request, mount))
	if err != nil || len(body) == 0 || len(body) > maxDockerContainerWriteRequestBytes {
		return "", newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	response, err := transport.doRuntimeInputApplication(ctx, http.MethodPost,
		"/v"+DockerContainerWriteAPIVersion+"/containers/create",
		"name="+url.QueryEscape(name), body, "application/json", true)
	if err != nil {
		return "", err
	}
	if response.status == http.StatusConflict {
		return "", newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorUnsafeCollision)
	}
	var payload struct {
		ID       string   `json:"Id"`
		Warnings []string `json:"Warnings"`
	}
	if decodeDockerContainerWriteJSON(response.body, &payload) != nil ||
		!validDockerContainerID(payload.ID) || len(payload.Warnings) != 0 {
		return "", newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	return payload.ID, nil
}

func (transport dockerEngineContainerWriteTransport) removeRuntimeInputContainer(ctx context.Context,
	containerID string,
) error {
	response, err := transport.doRuntimeInputApplication(ctx, http.MethodDelete,
		"/v"+DockerContainerWriteAPIVersion+"/containers/"+url.PathEscape(containerID),
		"v=1", nil, "", false)
	if err != nil {
		return err
	}
	if response.status != http.StatusNoContent && response.status != http.StatusNotFound {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorCleanup)
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) inspectRuntimeInputVolume(ctx context.Context,
	name string,
) (dockerVolumeInspection, bool, error) {
	response, err := transport.doRuntimeInputApplication(ctx, http.MethodGet,
		"/v"+DockerContainerWriteAPIVersion+"/volumes/"+url.PathEscape(name),
		"", nil, "", true)
	if err != nil {
		return dockerVolumeInspection{}, false, err
	}
	if response.status == http.StatusNotFound {
		return dockerVolumeInspection{}, false, nil
	}
	var value dockerVolumeInspection
	if decodeDockerContainerWriteJSON(response.body, &value) != nil {
		return dockerVolumeInspection{}, false,
			newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	return value, true, nil
}

func verifyDockerRuntimeInputVolume(value dockerVolumeInspection,
	request DockerRuntimeInputApplicationRequest, mount DockerRuntimeInputApplicationMount,
) error {
	if value.Name != mount.VolumeName || value.Driver != "local" || value.Scope != "local" ||
		value.Mountpoint == "" || strings.ContainsRune(value.Mountpoint, 0) ||
		len(value.Options) != 0 ||
		!equalStringMap(value.Labels, runtimeInputVolumeLabels(request, mount)) {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorUnsafeCollision)
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) createRuntimeInputVolume(ctx context.Context,
	request DockerRuntimeInputApplicationRequest, mount DockerRuntimeInputApplicationMount,
) error {
	body, err := json.Marshal(map[string]any{"Name": mount.VolumeName, "Driver": "local",
		"DriverOpts": map[string]string{}, "Labels": runtimeInputVolumeLabels(request, mount)})
	if err != nil {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	response, err := transport.doRuntimeInputApplication(ctx, http.MethodPost,
		"/v"+DockerContainerWriteAPIVersion+"/volumes/create", "", body,
		"application/json", true)
	if err != nil {
		return err
	}
	var value dockerVolumeInspection
	if response.status != http.StatusCreated ||
		decodeDockerContainerWriteJSON(response.body, &value) != nil ||
		verifyDockerRuntimeInputVolume(value, request, mount) != nil {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorUnsafeCollision)
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) removeRuntimeInputVolume(ctx context.Context,
	name string,
) error {
	response, err := transport.doRuntimeInputApplication(ctx, http.MethodDelete,
		"/v"+DockerContainerWriteAPIVersion+"/volumes/"+url.PathEscape(name),
		"force=0", nil, "", false)
	if err != nil {
		return err
	}
	if response.status != http.StatusNoContent && response.status != http.StatusNotFound {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorCleanup)
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) putRuntimeInputArchive(ctx context.Context,
	containerID string, archive []byte,
) error {
	response, err := transport.doRuntimeInputApplication(ctx, http.MethodPut,
		"/v"+DockerContainerWriteAPIVersion+"/containers/"+url.PathEscape(containerID)+"/archive",
		"noOverwriteDirNonDir=1&path="+url.QueryEscape(DockerRuntimeInputCarrierDestination),
		archive, "application/x-tar", false)
	if err != nil {
		return err
	}
	if response.status != http.StatusOK {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) getRuntimeInputArchive(ctx context.Context,
	containerID string,
) ([]byte, error) {
	response, err := transport.doRuntimeInputApplication(ctx, http.MethodGet,
		"/v"+DockerContainerWriteAPIVersion+"/containers/"+url.PathEscape(containerID)+"/archive",
		"path="+url.QueryEscape(DockerRuntimeInputCarrierDestination), nil,
		"application/x-tar", false)
	if err != nil {
		return nil, err
	}
	if response.status != http.StatusOK || len(response.body) == 0 {
		return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	return response.body, nil
}

type runtimeInputReadbackEntry struct {
	kind    byte
	mode    int64
	content []byte
}

func verifyRuntimeInputProjectionReadback(expectedArchive, actualArchive []byte) error {
	expected, err := parseRuntimeInputProjectionArchive(expectedArchive, false)
	if err != nil {
		return err
	}
	actual, err := parseRuntimeInputProjectionArchive(actualArchive, true)
	if err != nil || len(actual) != len(expected) {
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
	}
	for name, wanted := range expected {
		observed, ok := actual[name]
		if !ok || observed.kind != wanted.kind || observed.mode != wanted.mode ||
			!bytes.Equal(observed.content, wanted.content) {
			return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
		}
	}
	return nil
}

func parseRuntimeInputProjectionArchive(data []byte,
	stripCarrierRoot bool,
) (map[string]runtimeInputReadbackEntry, error) {
	if len(data) == 0 || int64(len(data)) > maxDockerRuntimeInputReadbackBytes {
		return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
	}
	reader := tar.NewReader(bytes.NewReader(data))
	entries := make(map[string]runtimeInputReadbackEntry)
	rootSeen := !stripCarrierRoot
	var totalBytes int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || header == nil || len(entries) >= MaxHostInputBundleEntries {
			return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
		}
		name := strings.TrimSuffix(strings.TrimPrefix(header.Name, "./"), "/")
		if stripCarrierRoot {
			if name == path.Base(DockerRuntimeInputCarrierDestination) {
				if rootSeen || header.Typeflag != tar.TypeDir {
					return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
				}
				rootSeen = true
				continue
			}
			prefix := path.Base(DockerRuntimeInputCarrierDestination) + "/"
			if !rootSeen || !strings.HasPrefix(name, prefix) {
				return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
			}
			name = strings.TrimPrefix(name, prefix)
		}
		if validateRuntimeInputArchiveName(name) != nil || strings.Contains(name, "\\") {
			return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
		}
		if _, duplicate := entries[name]; duplicate {
			return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
		}
		entry := runtimeInputReadbackEntry{kind: header.Typeflag, mode: header.Mode & 0o777}
		switch header.Typeflag {
		case tar.TypeDir:
			if header.Size != 0 || entry.mode != 0o555 {
				return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
			}
		case tar.TypeReg:
			if header.Size < 0 || header.Size > MaxHostInputSourceBytes+MaxInputArtifactTotalBytes ||
				entry.mode != 0o444 {
				return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
			}
			entry.content, err = io.ReadAll(io.LimitReader(reader, header.Size+1))
			if err != nil || int64(len(entry.content)) != header.Size {
				return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
			}
			totalBytes += int64(len(entry.content))
			if totalBytes > MaxHostInputSourceBytes+MaxInputArtifactTotalBytes {
				return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
			}
		default:
			return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
		}
		entries[name] = entry
	}
	if !rootSeen {
		return nil, newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorReadbackMismatch)
	}
	return entries, nil
}

type dockerRuntimeInputApplicationHTTPResponse struct {
	status int
	body   []byte
}

func (transport dockerEngineContainerWriteTransport) doRuntimeInputApplication(ctx context.Context,
	method, endpointPath, rawQuery string, body []byte, contentType string, wantJSON bool,
) (dockerRuntimeInputApplicationHTTPResponse, error) {
	if err := ctx.Err(); err != nil {
		return dockerRuntimeInputApplicationHTTPResponse{}, err
	}
	if !validDockerRuntimeInputApplicationOperation(method, endpointPath, rawQuery, body, contentType) {
		return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	requestURL := "http://docker" + endpointPath
	if rawQuery != "" {
		requestURL += "?" + rawQuery
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
	if err != nil {
		return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	request.Header.Set("Accept", map[bool]string{true: "application/json", false: "*/*"}[wantJSON])
	request.Header.Set("User-Agent", "cyberagent-workbench/docker-runtime-input-application-v1")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := transport.doer.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return dockerRuntimeInputApplicationHTTPResponse{}, ctx.Err()
		}
		return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorConnection)
	}
	if response == nil || response.Body == nil {
		return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil || response.Request.Method != method ||
		response.Request.URL.Scheme != "http" || response.Request.URL.Host != "docker" ||
		response.Request.URL.Path != endpointPath || response.Request.URL.RawQuery != rawQuery {
		return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	allowed := response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated ||
		response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotFound ||
		response.StatusCode == http.StatusConflict
	if !allowed {
		return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	limit := int64(maxDockerContainerWriteResponseBytes)
	if method == http.MethodGet && strings.HasSuffix(endpointPath, "/archive") {
		limit = maxDockerRuntimeInputReadbackBytes
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if readErr != nil || int64(len(data)) > limit {
		return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotFound ||
		response.StatusCode == http.StatusConflict {
		return dockerRuntimeInputApplicationHTTPResponse{status: response.StatusCode}, nil
	}
	if method == http.MethodGet && strings.HasSuffix(endpointPath, "/archive") {
		mediaType, _, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if parseErr != nil || !strings.EqualFold(mediaType, "application/x-tar") {
			return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
				DockerRuntimeInputApplicationErrorInvalidResponse)
		}
	} else if wantJSON {
		mediaType, _, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if parseErr != nil || !strings.EqualFold(mediaType, "application/json") {
			return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
				DockerRuntimeInputApplicationErrorInvalidResponse)
		}
	}
	if len(data) == 0 && method != http.MethodPut {
		return dockerRuntimeInputApplicationHTTPResponse{}, newDockerRuntimeInputApplicationError(
			DockerRuntimeInputApplicationErrorInvalidResponse)
	}
	return dockerRuntimeInputApplicationHTTPResponse{status: response.StatusCode, body: data}, nil
}

func validDockerRuntimeInputApplicationOperation(method, endpointPath, rawQuery string,
	body []byte, contentType string,
) bool {
	base := "/v" + DockerContainerWriteAPIVersion
	containerPrefix := base + "/containers/"
	volumePrefix := base + "/volumes/"
	switch {
	case method == http.MethodPost && endpointPath == base+"/containers/create":
		values, err := url.ParseQuery(rawQuery)
		name := values.Get("name")
		return err == nil && len(values) == 1 && len(values["name"]) == 1 &&
			(validDockerContainerName(name) || validDockerRuntimeInputCarrierName(name)) &&
			rawQuery == "name="+url.QueryEscape(name) && len(body) > 0 &&
			len(body) <= maxDockerContainerWriteRequestBytes && json.Valid(body) &&
			contentType == "application/json"
	case method == http.MethodPost && endpointPath == base+"/volumes/create":
		return rawQuery == "" && len(body) > 0 && len(body) <= maxDockerContainerWriteRequestBytes &&
			json.Valid(body) && contentType == "application/json"
	case method == http.MethodGet && strings.HasPrefix(endpointPath, volumePrefix):
		name, err := url.PathUnescape(strings.TrimPrefix(endpointPath, volumePrefix))
		return err == nil && endpointPath == volumePrefix+url.PathEscape(name) &&
			validDockerRuntimeInputVolumeName(name) && rawQuery == "" && len(body) == 0
	case method == http.MethodDelete && strings.HasPrefix(endpointPath, volumePrefix):
		name, err := url.PathUnescape(strings.TrimPrefix(endpointPath, volumePrefix))
		return err == nil && endpointPath == volumePrefix+url.PathEscape(name) &&
			validDockerRuntimeInputVolumeName(name) && rawQuery == "force=0" && len(body) == 0
	case method == http.MethodGet && strings.HasPrefix(endpointPath, containerPrefix) &&
		strings.HasSuffix(endpointPath, "/json"):
		reference, err := url.PathUnescape(strings.TrimSuffix(
			strings.TrimPrefix(endpointPath, containerPrefix), "/json"))
		return err == nil && endpointPath == containerPrefix+url.PathEscape(reference)+"/json" &&
			(validDockerContainerID(reference) || validDockerContainerName(reference) ||
				validDockerRuntimeInputCarrierName(reference)) && rawQuery == "" && len(body) == 0
	case method == http.MethodDelete && strings.HasPrefix(endpointPath, containerPrefix):
		reference, err := url.PathUnescape(strings.TrimPrefix(endpointPath, containerPrefix))
		return err == nil && endpointPath == containerPrefix+url.PathEscape(reference) &&
			validDockerContainerID(reference) && rawQuery == "v=1" && len(body) == 0
	case (method == http.MethodPut || method == http.MethodGet) &&
		strings.HasPrefix(endpointPath, containerPrefix) && strings.HasSuffix(endpointPath, "/archive"):
		encoded := strings.TrimSuffix(strings.TrimPrefix(endpointPath, containerPrefix), "/archive")
		containerID, err := url.PathUnescape(encoded)
		if err != nil || endpointPath != containerPrefix+url.PathEscape(containerID)+"/archive" ||
			!validDockerContainerID(containerID) {
			return false
		}
		values, err := url.ParseQuery(rawQuery)
		if err != nil {
			return false
		}
		if method == http.MethodPut {
			return len(values) == 2 && values.Get("path") == DockerRuntimeInputCarrierDestination &&
				values.Get("noOverwriteDirNonDir") == "1" &&
				rawQuery == "noOverwriteDirNonDir=1&path="+
					url.QueryEscape(DockerRuntimeInputCarrierDestination) && len(body) > 0 &&
				int64(len(body)) <= MaxHostInputBundleBytes && contentType == "application/x-tar"
		}
		return len(values) == 1 && values.Get("path") == DockerRuntimeInputCarrierDestination &&
			rawQuery == "path="+url.QueryEscape(DockerRuntimeInputCarrierDestination) && len(body) == 0
	default:
		return false
	}
}

func (transport dockerEngineContainerWriteTransport) cleanupDockerRuntimeInputApplication(
	request DockerRuntimeInputApplicationRequest,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), dockerRuntimeInputApplicationCleanupTimeout)
	defer cancel()
	var cleanupErrors []error
	if value, found, err := transport.inspectRuntimeInputContainer(ctx,
		request.Spec.ContainerName); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	} else if found {
		if verifyDockerRuntimeInputContainer(value, request, nil) != nil {
			cleanupErrors = append(cleanupErrors,
				newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorUnsafeCollision))
		} else if err := transport.removeRuntimeInputContainer(ctx, value.ID); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	for index := range request.Mounts {
		mount := &request.Mounts[index]
		if value, found, err := transport.inspectRuntimeInputContainer(ctx,
			mount.CarrierName); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else if found {
			if verifyDockerRuntimeInputContainer(value, request, mount) != nil {
				cleanupErrors = append(cleanupErrors,
					newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorUnsafeCollision))
			} else if err := transport.removeRuntimeInputContainer(ctx, value.ID); err != nil {
				cleanupErrors = append(cleanupErrors, err)
			}
		}
		if value, found, err := transport.inspectRuntimeInputVolume(ctx,
			mount.VolumeName); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		} else if found {
			if verifyDockerRuntimeInputVolume(value, request, *mount) != nil {
				cleanupErrors = append(cleanupErrors,
					newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorUnsafeCollision))
			} else if err := transport.removeRuntimeInputVolume(ctx, mount.VolumeName); err != nil {
				cleanupErrors = append(cleanupErrors, err)
			}
		}
	}
	return errors.Join(cleanupErrors...)
}

func normalizeDockerRuntimeInputApplicationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var applicationErr *DockerRuntimeInputApplicationError
	if errors.As(err, &applicationErr) {
		return err
	}
	switch DockerContainerWriteErrorCode(err) {
	case DockerContainerWriteFailureConnection:
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorConnection)
	case DockerContainerWriteFailureUnsafeImage:
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorConfigMismatch)
	default:
		return newDockerRuntimeInputApplicationError(DockerRuntimeInputApplicationErrorInvalidResponse)
	}
}
