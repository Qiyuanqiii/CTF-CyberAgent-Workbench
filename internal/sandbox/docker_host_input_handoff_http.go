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
	dockerHostInputHandoffCleanupTimeout  = 10 * time.Second
	maxDockerHostInputHandoffArchiveBytes = MaxHostInputBundleBytes + 1024*1024
)

type dockerVolumeInspection struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
	Options    map[string]string `json:"Options"`
}

func (transport dockerEngineContainerWriteTransport) Handoff(ctx context.Context,
	request DockerHostInputHandoffRequest, bundle HostInputBundle,
) (result DockerHostInputHandoffResult, returnedErr error) {
	if err := ctx.Err(); err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	if request.Validate() != nil || transport.doer == nil ||
		transport.endpoint.Class != DockerObservationEndpointLocalUnix {
		return DockerHostInputHandoffResult{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorUnsupported)
	}
	cleanupPending := true
	defer func() {
		if !cleanupPending {
			return
		}
		if cleanupErr := transport.cleanupDockerHostInputHandoff(request); cleanupErr != nil {
			returnedErr = errors.Join(returnedErr,
				newDockerHostInputHandoffError(DockerHostInputHandoffErrorCleanup))
		}
	}()
	bundleBytes, err := readExactHostInputBundle(bundle, request.BundleReport)
	if err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	archiveBytes, err := wrapDockerHostInputBundle(bundleBytes)
	if err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	reads, writes, reconciled := 0, 0, 0
	if err := transport.verifyImageProfile(ctx, request.WriteRequest.Spec.ImageDigest); err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	reads++

	target, targetFound, err := transport.inspect(ctx, request.WriteRequest.Spec.ContainerName)
	reads++
	if err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	originalID := ""
	if targetFound {
		switch {
		case verifyDockerContainerInspection(target, request.WriteRequest) == nil &&
			fingerprint("sandbox_docker_container_id.v1", target.ID) ==
				request.Stage.ContainerIDFingerprint:
			originalID = target.ID
		case verifyDockerHostInputContainer(target, request, false) == nil:
			if err := transport.remove(ctx, target.ID, false); err != nil {
				return DockerHostInputHandoffResult{}, err
			}
			writes++
			reconciled++
		default:
			return DockerHostInputHandoffResult{}, newDockerHostInputHandoffError(
				DockerHostInputHandoffErrorUnsafeCollision)
		}
	}

	carrier, carrierFound, err := transport.inspectDockerHostInputContainer(ctx,
		request.CarrierName)
	reads++
	if err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	if carrierFound {
		if verifyDockerHostInputContainer(carrier, request, true) != nil {
			return DockerHostInputHandoffResult{}, newDockerHostInputHandoffError(
				DockerHostInputHandoffErrorUnsafeCollision)
		}
		if err := transport.remove(ctx, carrier.ID, false); err != nil {
			return DockerHostInputHandoffResult{}, err
		}
		writes++
		reconciled++
	}

	volume, volumeFound, err := transport.inspectDockerHostInputVolume(ctx, request)
	reads++
	if err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	if volumeFound {
		if verifyDockerHostInputVolume(volume, request) != nil {
			return DockerHostInputHandoffResult{}, newDockerHostInputHandoffError(
				DockerHostInputHandoffErrorUnsafeCollision)
		}
		if err := transport.removeDockerHostInputVolume(ctx, request.VolumeName); err != nil {
			return DockerHostInputHandoffResult{}, err
		}
		writes++
		reconciled++
	}

	if err := transport.createDockerHostInputVolume(ctx, request); err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	writes++
	volume, volumeFound, err = transport.inspectDockerHostInputVolume(ctx, request)
	reads++
	if err != nil || !volumeFound || verifyDockerHostInputVolume(volume, request) != nil {
		if err != nil {
			return DockerHostInputHandoffResult{}, err
		}
		return DockerHostInputHandoffResult{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorUnsafeCollision)
	}

	carrierID, err := transport.createDockerHostInputContainer(ctx, request, true)
	if err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	writes++
	carrier, carrierFound, err = transport.inspectDockerHostInputContainer(ctx, carrierID)
	reads++
	if err != nil || !carrierFound || carrier.ID != carrierID ||
		verifyDockerHostInputContainer(carrier, request, true) != nil {
		if err != nil {
			return DockerHostInputHandoffResult{}, err
		}
		return DockerHostInputHandoffResult{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorUnsafeCollision)
	}
	if err := transport.putDockerHostInputArchive(ctx, carrierID, archiveBytes); err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	writes++
	readback, err := transport.getDockerHostInputArchive(ctx, carrierID)
	reads++
	if err != nil || hashHostInputBytes(readback) != request.BundleReport.BundleDigest ||
		int64(len(readback)) != request.BundleReport.BundleBytes {
		if err != nil {
			return DockerHostInputHandoffResult{}, err
		}
		return DockerHostInputHandoffResult{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorReadbackMismatch)
	}
	if err := transport.remove(ctx, carrierID, false); err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	writes++
	if originalID != "" {
		if err := transport.remove(ctx, originalID, false); err != nil {
			return DockerHostInputHandoffResult{}, err
		}
		writes++
	}

	finalID, err := transport.createDockerHostInputContainer(ctx, request, false)
	if err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	writes++
	final, finalFound, err := transport.inspect(ctx, finalID)
	reads++
	if err != nil || !finalFound || final.ID != finalID ||
		verifyDockerHostInputContainer(final, request, false) != nil {
		if err != nil {
			return DockerHostInputHandoffResult{}, err
		}
		return DockerHostInputHandoffResult{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorUnsafeCollision)
	}
	if err := transport.remove(ctx, finalID, false); err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	writes++
	if err := transport.removeDockerHostInputVolume(ctx, request.VolumeName); err != nil {
		return DockerHostInputHandoffResult{}, err
	}
	writes++
	cleanupPending = false
	return NewDockerHostInputHandoffResult(transport.endpoint, request, finalID,
		reads, writes, reconciled)
}

func dockerHostInputLabels(request DockerHostInputHandoffRequest, role string) map[string]string {
	labels := make(map[string]string, len(request.WriteRequest.Spec.Labels)+3)
	for _, label := range request.WriteRequest.Spec.Labels {
		labels[label.Name] = label.Value
	}
	labels["cyberagent.workbench.handoff.request"] = request.RequestFingerprint
	labels["cyberagent.workbench.handoff.bundle"] = request.BundleReport.BundleDigest
	labels["cyberagent.workbench.handoff.role"] = role
	return labels
}

func dockerHostInputVolumeLabels(request DockerHostInputHandoffRequest) map[string]string {
	return map[string]string{
		"cyberagent.workbench.handoff.request": request.RequestFingerprint,
		"cyberagent.workbench.handoff.bundle":  request.BundleReport.BundleDigest,
		"cyberagent.workbench.handoff.role":    "host-input-volume",
	}
}

func dockerHostInputContainerPayload(request DockerHostInputHandoffRequest,
	carrier bool,
) dockerCreateContainerPayload {
	payload := dockerCreatePayload(request.WriteRequest)
	role := "read-only-target"
	mounts := append([]dockerCreateMount(nil), payload.HostConfig.Mounts...)
	if carrier {
		role = "write-only-carrier"
		mounts = nil
	}
	mounts = append(mounts, dockerCreateMount{Type: "volume", Source: request.VolumeName,
		Target: DockerHostInputCarrierDestination, ReadOnly: !carrier,
		VolumeOptions: &dockerCreateVolumeOptions{NoCopy: true}})
	payload.Labels = dockerHostInputLabels(request, role)
	payload.HostConfig.Mounts = mounts
	return payload
}

func (transport dockerEngineContainerWriteTransport) createDockerHostInputContainer(
	ctx context.Context, request DockerHostInputHandoffRequest, carrier bool,
) (string, error) {
	name := request.WriteRequest.Spec.ContainerName
	if carrier {
		name = request.CarrierName
	}
	body, err := json.Marshal(dockerHostInputContainerPayload(request, carrier))
	if err != nil || len(body) == 0 || len(body) > maxDockerContainerWriteRequestBytes {
		return "", newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	response, err := transport.doDockerHostInputHandoff(ctx, http.MethodPost,
		"/v"+DockerContainerWriteAPIVersion+"/containers/create",
		"name="+url.QueryEscape(name), body, "application/json", true)
	if err != nil {
		return "", err
	}
	if response.status == http.StatusConflict {
		return "", newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision)
	}
	var payload struct {
		ID       string   `json:"Id"`
		Warnings []string `json:"Warnings"`
	}
	if decodeDockerContainerWriteJSON(response.body, &payload) != nil ||
		!validDockerContainerID(payload.ID) || len(payload.Warnings) != 0 {
		return "", newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	return payload.ID, nil
}

func (transport dockerEngineContainerWriteTransport) inspectDockerHostInputContainer(
	ctx context.Context, reference string,
) (dockerContainerInspection, bool, error) {
	response, err := transport.doDockerHostInputHandoff(ctx, http.MethodGet,
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
			newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision)
	}
	return value, true, nil
}

func verifyDockerHostInputContainer(inspection dockerContainerInspection,
	request DockerHostInputHandoffRequest, carrier bool,
) error {
	spec := request.WriteRequest.Spec
	name, role, bindCount := spec.ContainerName, "read-only-target", len(request.WriteRequest.HostMounts)
	if carrier {
		name, role, bindCount = request.CarrierName, "write-only-carrier", 0
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
		!equalStringMap(inspection.Config.Labels, dockerHostInputLabels(request, role)) ||
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
		inspection.HostConfig.LogConfig.Type != "none" ||
		len(inspection.HostConfig.LogConfig.Config) != 0 ||
		len(inspection.HostConfig.SecurityOpt) != 1 ||
		!containsFold(inspection.HostConfig.SecurityOpt, "no-new-privileges") ||
		len(inspection.HostConfig.CapAdd) != 0 || len(inspection.HostConfig.CapDrop) != 1 ||
		!containsFold(inspection.HostConfig.CapDrop, "ALL") ||
		len(inspection.HostConfig.Binds) != 0 || len(inspection.HostConfig.Devices) != 0 ||
		len(inspection.HostConfig.DeviceRequests) != 0 ||
		len(inspection.HostConfig.PortBindings) != 0 || inspection.HostConfig.PublishAllPorts ||
		len(inspection.Mounts) != bindCount+1 {
		return newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision)
	}
	binds := make(map[string]DockerHostMount, bindCount)
	if !carrier {
		for _, mount := range request.WriteRequest.HostMounts {
			binds[mount.Target] = mount
		}
	}
	volumeFound := false
	for _, observed := range inspection.Mounts {
		if observed.Destination == DockerHostInputCarrierDestination {
			expectedMode := "ro"
			if carrier {
				expectedMode = "rw"
			}
			if volumeFound || observed.Type != "volume" || observed.Name != request.VolumeName ||
				observed.Driver != "local" || observed.RW != carrier ||
				observed.Mode != expectedMode {
				return newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision)
			}
			volumeFound = true
			continue
		}
		expected, ok := binds[observed.Destination]
		if !ok || observed.Type != "bind" || observed.Source != expected.Source ||
			observed.RW == expected.ReadOnly || observed.Propagation != expected.Propagation {
			return newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision)
		}
		delete(binds, observed.Destination)
	}
	if !volumeFound || len(binds) != 0 {
		return newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision)
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) inspectDockerHostInputVolume(ctx context.Context,
	request DockerHostInputHandoffRequest,
) (dockerVolumeInspection, bool, error) {
	response, err := transport.doDockerHostInputHandoff(ctx, http.MethodGet,
		"/v"+DockerContainerWriteAPIVersion+"/volumes/"+url.PathEscape(request.VolumeName),
		"", nil, "application/json", true)
	if err != nil {
		return dockerVolumeInspection{}, false, err
	}
	if response.status == http.StatusNotFound {
		return dockerVolumeInspection{}, false, nil
	}
	var value dockerVolumeInspection
	if decodeDockerContainerWriteJSON(response.body, &value) != nil {
		return dockerVolumeInspection{}, false,
			newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision)
	}
	return value, true, nil
}

func verifyDockerHostInputVolume(value dockerVolumeInspection,
	request DockerHostInputHandoffRequest,
) error {
	if value.Name != request.VolumeName || value.Driver != "local" || value.Scope != "local" ||
		value.Mountpoint == "" || strings.ContainsRune(value.Mountpoint, 0) ||
		len(value.Options) != 0 || !equalStringMap(value.Labels, dockerHostInputVolumeLabels(request)) {
		return newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision)
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) createDockerHostInputVolume(ctx context.Context,
	request DockerHostInputHandoffRequest,
) error {
	body, err := json.Marshal(map[string]any{"Name": request.VolumeName, "Driver": "local",
		"DriverOpts": map[string]string{}, "Labels": dockerHostInputVolumeLabels(request)})
	if err != nil {
		return newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	response, err := transport.doDockerHostInputHandoff(ctx, http.MethodPost,
		"/v"+DockerContainerWriteAPIVersion+"/volumes/create", "", body,
		"application/json", true)
	if err != nil {
		return err
	}
	var value dockerVolumeInspection
	if response.status != http.StatusCreated ||
		decodeDockerContainerWriteJSON(response.body, &value) != nil ||
		verifyDockerHostInputVolume(value, request) != nil {
		return newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision)
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) removeDockerHostInputVolume(ctx context.Context,
	name string,
) error {
	response, err := transport.doDockerHostInputHandoff(ctx, http.MethodDelete,
		"/v"+DockerContainerWriteAPIVersion+"/volumes/"+url.PathEscape(name),
		"force=0", nil, "", false)
	if err != nil {
		return err
	}
	if response.status != http.StatusNoContent && response.status != http.StatusNotFound {
		return newDockerHostInputHandoffError(DockerHostInputHandoffErrorCleanup)
	}
	return nil
}

func wrapDockerHostInputBundle(bundle []byte) ([]byte, error) {
	if len(bundle) == 0 || int64(len(bundle)) > MaxHostInputBundleBytes {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	header := &tar.Header{Name: DockerHostInputCarrierArchiveName, Typeflag: tar.TypeReg,
		Mode: 0o444, Size: int64(len(bundle)), ModTime: time.Unix(0, 0).UTC(),
		AccessTime: time.Unix(0, 0).UTC(), ChangeTime: time.Unix(0, 0).UTC(),
		Uid: 65532, Gid: 65532, Format: tar.FormatPAX}
	if err := writer.WriteHeader(header); err != nil {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	if _, err := writer.Write(bundle); err != nil {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	if err := writer.Close(); err != nil || int64(output.Len()) > maxDockerHostInputHandoffArchiveBytes {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	return output.Bytes(), nil
}

func (transport dockerEngineContainerWriteTransport) putDockerHostInputArchive(ctx context.Context,
	containerID string, archive []byte,
) error {
	response, err := transport.doDockerHostInputHandoff(ctx, http.MethodPut,
		"/v"+DockerContainerWriteAPIVersion+"/containers/"+url.PathEscape(containerID)+"/archive",
		"noOverwriteDirNonDir=1&path="+url.QueryEscape(DockerHostInputCarrierDestination),
		archive, "application/x-tar", false)
	if err != nil {
		return err
	}
	if response.status != http.StatusOK {
		return newDockerHostInputHandoffError(DockerHostInputHandoffErrorInvalidBundle)
	}
	return nil
}

func (transport dockerEngineContainerWriteTransport) getDockerHostInputArchive(ctx context.Context,
	containerID string,
) ([]byte, error) {
	response, err := transport.doDockerHostInputHandoff(ctx, http.MethodGet,
		"/v"+DockerContainerWriteAPIVersion+"/containers/"+url.PathEscape(containerID)+"/archive",
		"path="+url.QueryEscape(path.Join(DockerHostInputCarrierDestination,
			DockerHostInputCarrierArchiveName)), nil, "application/x-tar", false)
	if err != nil {
		return nil, err
	}
	reader := tar.NewReader(bytes.NewReader(response.body))
	header, err := reader.Next()
	if err != nil || header == nil || header.Typeflag != tar.TypeReg ||
		path.Base(header.Name) != DockerHostInputCarrierArchiveName ||
		header.Size < 1 || header.Size > MaxHostInputBundleBytes {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorReadbackMismatch)
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxHostInputBundleBytes+1))
	if err != nil || int64(len(data)) != header.Size {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorReadbackMismatch)
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		return nil, newDockerHostInputHandoffError(DockerHostInputHandoffErrorReadbackMismatch)
	}
	return data, nil
}

type dockerHostInputHTTPResponse struct {
	status int
	body   []byte
}

func (transport dockerEngineContainerWriteTransport) doDockerHostInputHandoff(ctx context.Context,
	method, endpointPath, rawQuery string, body []byte, contentType string, wantJSON bool,
) (dockerHostInputHTTPResponse, error) {
	if err := ctx.Err(); err != nil {
		return dockerHostInputHTTPResponse{}, err
	}
	if !validDockerHostInputHandoffOperation(method, endpointPath, rawQuery, body, contentType) {
		return dockerHostInputHTTPResponse{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorInvalidBundle)
	}
	requestURL := "http://docker" + endpointPath
	if rawQuery != "" {
		requestURL += "?" + rawQuery
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
	if err != nil {
		return dockerHostInputHTTPResponse{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorInvalidBundle)
	}
	httpRequest.Header.Set("Accept", map[bool]string{true: "application/json", false: "*/*"}[wantJSON])
	httpRequest.Header.Set("User-Agent", "cyberagent-workbench/docker-host-input-handoff-v1")
	if contentType != "" {
		httpRequest.Header.Set("Content-Type", contentType)
	}
	response, err := transport.doer.Do(httpRequest)
	if err != nil {
		if ctx.Err() != nil {
			return dockerHostInputHTTPResponse{}, ctx.Err()
		}
		return dockerHostInputHTTPResponse{}, newDockerContainerWriteError(
			DockerContainerWriteFailureConnection)
	}
	if response == nil || response.Body == nil {
		return dockerHostInputHTTPResponse{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorInvalidBundle)
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil ||
		response.Request.Method != method || response.Request.URL.Scheme != "http" ||
		response.Request.URL.Host != "docker" || response.Request.URL.Path != endpointPath ||
		response.Request.URL.RawQuery != rawQuery {
		return dockerHostInputHTTPResponse{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorInvalidBundle)
	}
	allowed := response.StatusCode == http.StatusOK || response.StatusCode == http.StatusCreated ||
		response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotFound ||
		response.StatusCode == http.StatusConflict || response.StatusCode == http.StatusForbidden
	if !allowed {
		return dockerHostInputHTTPResponse{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorInvalidBundle)
	}
	limit := int64(maxDockerContainerWriteResponseBytes)
	if method == http.MethodGet && strings.HasSuffix(endpointPath, "/archive") {
		limit = maxDockerHostInputHandoffArchiveBytes
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if readErr != nil || int64(len(data)) > limit {
		return dockerHostInputHTTPResponse{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorInvalidBundle)
	}
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusNotFound ||
		response.StatusCode == http.StatusConflict || response.StatusCode == http.StatusForbidden {
		return dockerHostInputHTTPResponse{status: response.StatusCode}, nil
	}
	if method == http.MethodGet && strings.HasSuffix(endpointPath, "/archive") {
		mediaType, _, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if parseErr != nil || !strings.EqualFold(mediaType, "application/x-tar") {
			return dockerHostInputHTTPResponse{}, newDockerHostInputHandoffError(
				DockerHostInputHandoffErrorInvalidBundle)
		}
	}
	if len(data) == 0 && method != http.MethodPut {
		return dockerHostInputHTTPResponse{}, newDockerHostInputHandoffError(
			DockerHostInputHandoffErrorInvalidBundle)
	}
	if wantJSON {
		mediaType, _, parseErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if parseErr != nil || !strings.EqualFold(mediaType, "application/json") {
			return dockerHostInputHTTPResponse{}, newDockerHostInputHandoffError(
				DockerHostInputHandoffErrorInvalidBundle)
		}
	}
	return dockerHostInputHTTPResponse{status: response.StatusCode, body: data}, nil
}

func validDockerHostInputHandoffOperation(method, endpointPath, rawQuery string,
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
			(validDockerContainerName(name) || validDockerHostInputCarrierName(name)) &&
			rawQuery == "name="+url.QueryEscape(name) && len(body) > 0 &&
			len(body) <= maxDockerContainerWriteRequestBytes && json.Valid(body) &&
			contentType == "application/json"
	case method == http.MethodPost && endpointPath == base+"/volumes/create":
		return rawQuery == "" && len(body) > 0 && len(body) <= maxDockerContainerWriteRequestBytes &&
			json.Valid(body) && contentType == "application/json"
	case method == http.MethodGet && strings.HasPrefix(endpointPath, volumePrefix):
		name, err := url.PathUnescape(strings.TrimPrefix(endpointPath, volumePrefix))
		return err == nil && endpointPath == volumePrefix+url.PathEscape(name) &&
			validDockerHostInputVolumeName(name) && rawQuery == "" && len(body) == 0
	case method == http.MethodGet && strings.HasPrefix(endpointPath, containerPrefix) &&
		strings.HasSuffix(endpointPath, "/json"):
		reference, err := url.PathUnescape(strings.TrimSuffix(
			strings.TrimPrefix(endpointPath, containerPrefix), "/json"))
		return err == nil && endpointPath == containerPrefix+url.PathEscape(reference)+"/json" &&
			(validDockerContainerID(reference) || validDockerContainerName(reference) ||
				validDockerHostInputCarrierName(reference)) && rawQuery == "" && len(body) == 0
	case method == http.MethodDelete && strings.HasPrefix(endpointPath, volumePrefix):
		name, err := url.PathUnescape(strings.TrimPrefix(endpointPath, volumePrefix))
		return err == nil && endpointPath == volumePrefix+url.PathEscape(name) &&
			validDockerHostInputVolumeName(name) && rawQuery == "force=0" && len(body) == 0
	case (method == http.MethodPut || method == http.MethodGet) &&
		strings.HasPrefix(endpointPath, containerPrefix) && strings.HasSuffix(endpointPath, "/archive"):
		encodedID := strings.TrimSuffix(strings.TrimPrefix(endpointPath, containerPrefix), "/archive")
		containerID, err := url.PathUnescape(encodedID)
		if err != nil || endpointPath != containerPrefix+url.PathEscape(containerID)+"/archive" ||
			!validDockerContainerID(containerID) {
			return false
		}
		values, err := url.ParseQuery(rawQuery)
		if err != nil {
			return false
		}
		if method == http.MethodPut {
			return len(values) == 2 && values.Get("path") == DockerHostInputCarrierDestination &&
				values.Get("noOverwriteDirNonDir") == "1" &&
				rawQuery == "noOverwriteDirNonDir=1&path="+
					url.QueryEscape(DockerHostInputCarrierDestination) &&
				len(body) > 0 && int64(len(body)) <= maxDockerHostInputHandoffArchiveBytes &&
				contentType == "application/x-tar"
		}
		expectedPath := path.Join(DockerHostInputCarrierDestination,
			DockerHostInputCarrierArchiveName)
		return len(values) == 1 && values.Get("path") == expectedPath &&
			rawQuery == "path="+url.QueryEscape(expectedPath) && len(body) == 0
	default:
		return false
	}
}

func equalStringMap(first, second map[string]string) bool {
	if len(first) != len(second) {
		return false
	}
	for key, value := range first {
		if second[key] != value {
			return false
		}
	}
	return true
}

func (transport dockerEngineContainerWriteTransport) cleanupDockerHostInputHandoff(
	request DockerHostInputHandoffRequest,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), dockerHostInputHandoffCleanupTimeout)
	defer cancel()
	var cleanupErrors []error
	if value, found, err := transport.inspectDockerHostInputContainer(ctx,
		request.CarrierName); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	} else if found {
		if verifyDockerHostInputContainer(value, request, true) != nil {
			cleanupErrors = append(cleanupErrors,
				newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision))
		} else if err := transport.remove(ctx, value.ID, false); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if value, found, err := transport.inspect(ctx, request.WriteRequest.Spec.ContainerName); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	} else if found {
		finalOwned := verifyDockerHostInputContainer(value, request, false) == nil
		originalOwned := verifyDockerContainerInspection(value, request.WriteRequest) == nil &&
			fingerprint("sandbox_docker_container_id.v1", value.ID) ==
				request.Stage.ContainerIDFingerprint
		if !finalOwned && !originalOwned {
			cleanupErrors = append(cleanupErrors,
				newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision))
		} else if err := transport.remove(ctx, value.ID, false); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if value, found, err := transport.inspectDockerHostInputVolume(ctx, request); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	} else if found {
		if verifyDockerHostInputVolume(value, request) != nil {
			cleanupErrors = append(cleanupErrors,
				newDockerHostInputHandoffError(DockerHostInputHandoffErrorUnsafeCollision))
		} else if err := transport.removeDockerHostInputVolume(ctx, request.VolumeName); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}
