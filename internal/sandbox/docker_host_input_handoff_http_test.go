package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type handoffTestBundle struct {
	*bytes.Reader
	report HostInputBundleReport
	closed bool
}

func (bundle *handoffTestBundle) Report() HostInputBundleReport { return bundle.report }
func (bundle *handoffTestBundle) Close() error {
	bundle.closed = true
	return nil
}

type handoffTestContainer struct {
	id      string
	name    string
	payload dockerCreateContainerPayload
}

type dockerHandoffTestDaemon struct {
	mu              sync.Mutex
	containers      map[string]*handoffTestContainer
	volume          *dockerVolumeInspection
	archive         []byte
	requests        []string
	volumeDeletes   int
	containerStarts int
}

func (daemon *dockerHandoffTestDaemon) Do(request *http.Request) (*http.Response, error) {
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	daemon.requests = append(daemon.requests, request.Method+" "+request.URL.RequestURI())
	endpoint := request.URL.Path
	base := "/v" + DockerContainerWriteAPIVersion
	if request.Method == http.MethodGet && strings.HasPrefix(endpoint, base+"/images/") {
		digest, _ := url.PathUnescape(strings.TrimSuffix(
			strings.TrimPrefix(endpoint, base+"/images/"), "/json"))
		return handoffTestJSONResponse(request, http.StatusOK, map[string]any{
			"Id":          "sha256:" + strings.Repeat("d", 64),
			"RepoDigests": []string{"example.invalid/handoff@" + digest},
			"Config":      map[string]any{"Volumes": nil, "Env": nil},
		}), nil
	}
	if strings.HasPrefix(endpoint, base+"/volumes") {
		return daemon.handleVolume(request)
	}
	if request.Method == http.MethodPost && endpoint == base+"/containers/create" {
		name := request.URL.Query().Get("name")
		if daemon.findContainer(name) != nil {
			return handoffTestJSONResponse(request, http.StatusConflict,
				map[string]any{"message": "conflict"}), nil
		}
		var payload dockerCreateContainerPayload
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			return nil, err
		}
		id := strings.Repeat("d", 64)
		if validDockerHostInputCarrierName(name) {
			id = strings.Repeat("c", 64)
		}
		daemon.containers[name] = &handoffTestContainer{id: id, name: name, payload: payload}
		return handoffTestJSONResponse(request, http.StatusCreated,
			map[string]any{"Id": id, "Warnings": []string{}}), nil
	}
	containerPrefix := base + "/containers/"
	if strings.HasPrefix(endpoint, containerPrefix) && strings.HasSuffix(endpoint, "/archive") {
		reference, _ := url.PathUnescape(strings.TrimSuffix(
			strings.TrimPrefix(endpoint, containerPrefix), "/archive"))
		container := daemon.findContainer(reference)
		if container == nil {
			return handoffTestResponse(request, http.StatusNotFound, nil, ""), nil
		}
		switch request.Method {
		case http.MethodPut:
			data, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			daemon.archive = append([]byte(nil), data...)
			return handoffTestResponse(request, http.StatusOK, nil, ""), nil
		case http.MethodGet:
			return handoffTestResponse(request, http.StatusOK, daemon.archive,
				"application/x-tar"), nil
		}
	}
	if request.Method == http.MethodGet && strings.HasPrefix(endpoint, containerPrefix) &&
		strings.HasSuffix(endpoint, "/json") {
		reference, _ := url.PathUnescape(strings.TrimSuffix(
			strings.TrimPrefix(endpoint, containerPrefix), "/json"))
		container := daemon.findContainer(reference)
		if container == nil {
			return handoffTestResponse(request, http.StatusNotFound, nil, ""), nil
		}
		return handoffTestJSONResponse(request, http.StatusOK,
			daemon.inspectPayload(container)), nil
	}
	if request.Method == http.MethodDelete && strings.HasPrefix(endpoint, containerPrefix) {
		reference, _ := url.PathUnescape(strings.TrimPrefix(endpoint, containerPrefix))
		container := daemon.findContainer(reference)
		if container == nil {
			return handoffTestResponse(request, http.StatusNotFound, nil, ""), nil
		}
		delete(daemon.containers, container.name)
		return handoffTestResponse(request, http.StatusNoContent, nil, ""), nil
	}
	if strings.Contains(endpoint, "/start") {
		daemon.containerStarts++
	}
	return handoffTestResponse(request, http.StatusMethodNotAllowed, nil, ""), nil
}

func (daemon *dockerHandoffTestDaemon) handleVolume(request *http.Request) (*http.Response, error) {
	base := "/v" + DockerContainerWriteAPIVersion
	if request.Method == http.MethodPost && request.URL.Path == base+"/volumes/create" {
		var payload struct {
			Name   string            `json:"Name"`
			Labels map[string]string `json:"Labels"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if daemon.volume == nil {
			daemon.volume = &dockerVolumeInspection{Name: payload.Name, Driver: "local",
				Mountpoint: "/var/lib/docker/volumes/" + payload.Name + "/_data",
				Labels:     payload.Labels, Scope: "local", Options: map[string]string{}}
		}
		return handoffTestJSONResponse(request, http.StatusCreated, daemon.volume), nil
	}
	name, _ := url.PathUnescape(strings.TrimPrefix(request.URL.Path, base+"/volumes/"))
	if daemon.volume == nil || daemon.volume.Name != name {
		return handoffTestResponse(request, http.StatusNotFound, nil, ""), nil
	}
	if request.Method == http.MethodGet {
		return handoffTestJSONResponse(request, http.StatusOK, daemon.volume), nil
	}
	if request.Method == http.MethodDelete {
		daemon.volume = nil
		daemon.volumeDeletes++
		return handoffTestResponse(request, http.StatusNoContent, nil, ""), nil
	}
	return handoffTestResponse(request, http.StatusMethodNotAllowed, nil, ""), nil
}

func (daemon *dockerHandoffTestDaemon) findContainer(reference string) *handoffTestContainer {
	if value := daemon.containers[reference]; value != nil {
		return value
	}
	for _, value := range daemon.containers {
		if value.id == reference {
			return value
		}
	}
	return nil
}

func (daemon *dockerHandoffTestDaemon) inspectPayload(container *handoffTestContainer) any {
	mounts := make([]map[string]any, 0, len(container.payload.HostConfig.Mounts))
	for _, mount := range container.payload.HostConfig.Mounts {
		if mount.Type == "volume" {
			mode := "ro"
			if !mount.ReadOnly {
				mode = "rw"
			}
			mounts = append(mounts, map[string]any{"Type": "volume", "Name": mount.Source,
				"Source":      "/var/lib/docker/volumes/" + mount.Source + "/_data",
				"Destination": mount.Target, "Driver": "local", "Mode": mode,
				"RW": !mount.ReadOnly, "Propagation": ""})
			continue
		}
		propagation := ""
		if mount.BindOptions != nil {
			propagation = mount.BindOptions.Propagation
		}
		mounts = append(mounts, map[string]any{"Type": "bind", "Source": mount.Source,
			"Destination": mount.Target, "RW": !mount.ReadOnly,
			"Propagation": propagation})
	}
	return map[string]any{
		"Id": container.id, "Name": "/" + container.name,
		"Created": "2026-07-15T00:00:00Z",
		"Config": map[string]any{"Image": container.payload.Image,
			"Entrypoint": container.payload.Entrypoint, "Cmd": container.payload.Cmd,
			"Env": container.payload.Env, "WorkingDir": container.payload.WorkingDir,
			"User":            container.payload.User,
			"NetworkDisabled": container.payload.NetworkDisabled,
			"Labels":          container.payload.Labels, "StopSignal": container.payload.StopSignal},
		"State": map[string]any{"Status": "created", "Running": false, "Paused": false,
			"Restarting": false, "OOMKilled": false, "Dead": false, "Pid": 0},
		"HostConfig": container.payload.HostConfig, "Mounts": mounts,
	}
}

func handoffTestJSONResponse(request *http.Request, status int, payload any) *http.Response {
	body, _ := json.Marshal(payload)
	return handoffTestResponse(request, status, body, "application/json")
}

func handoffTestResponse(request *http.Request, status int, body []byte,
	contentType string,
) *http.Response {
	response := &http.Response{StatusCode: status, Header: make(http.Header),
		Body: io.NopCloser(bytes.NewReader(body)), Request: request}
	if contentType != "" {
		response.Header.Set("Content-Type", contentType)
	}
	return response
}

func TestDockerHostInputHandoffUsesOnlyStoppedFixedEndpointsAndCleansEverything(t *testing.T) {
	request, bundle := newDockerHostInputHandoffTransportFixture(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerHandoffTestDaemon{containers: map[string]*handoffTestContainer{
		request.WriteRequest.Spec.ContainerName: {id: dockerWriteTestContainerID,
			name:    request.WriteRequest.Spec.ContainerName,
			payload: dockerCreatePayload(request.WriteRequest)},
	}}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	result, err := transport.Handoff(context.Background(), request, bundle)
	if err != nil {
		t.Fatal(err)
	}
	if result.Validate() != nil || !result.DaemonConsumed || !result.ReadbackVerified ||
		!result.FinalMountReadOnly || !result.CleanupConfirmed || result.ContainerStarted ||
		result.ProcessExecuted || len(daemon.containers) != 0 || daemon.volume != nil ||
		daemon.containerStarts != 0 || bundle.closed {
		t.Fatalf("handoff escaped its boundary: result=%#v daemon=%#v", result, daemon)
	}
	for _, call := range daemon.requests {
		if strings.Contains(call, "/start") || strings.Contains(call, "/exec") ||
			strings.Contains(call, "/attach") || strings.Contains(call, "/export") ||
			strings.Contains(call, "/images/create") || strings.Contains(call, "/networks/") {
			t.Fatalf("handoff issued a forbidden endpoint: %s", call)
		}
	}
	joined := strings.Join(daemon.requests, "\n")
	for _, required := range []string{"POST /v1.40/volumes/create",
		"PUT /v1.40/containers/" + strings.Repeat("c", 64) + "/archive",
		"GET /v1.40/containers/" + strings.Repeat("c", 64) + "/archive",
		"DELETE /v1.40/volumes/" + request.VolumeName + "?force=0"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("handoff omitted fixed operation %q:\n%s", required, joined)
		}
	}
}

func TestDockerHostInputHandoffRejectsForeignVolumeWithoutDeletingIt(t *testing.T) {
	request, bundle := newDockerHostInputHandoffTransportFixture(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerHandoffTestDaemon{containers: map[string]*handoffTestContainer{
		request.WriteRequest.Spec.ContainerName: {id: dockerWriteTestContainerID,
			name:    request.WriteRequest.Spec.ContainerName,
			payload: dockerCreatePayload(request.WriteRequest)},
	}, volume: &dockerVolumeInspection{Name: request.VolumeName, Driver: "local",
		Mountpoint: "/foreign", Scope: "local", Options: map[string]string{},
		Labels: map[string]string{"owner": "foreign"}}}
	transport, _ := newDockerEngineContainerWriteTransport(daemon, endpoint)
	_, err := transport.Handoff(context.Background(), request, bundle)
	if DockerHostInputHandoffErrorCode(err) != DockerHostInputHandoffErrorUnsafeCollision ||
		daemon.volume == nil || daemon.volumeDeletes != 0 ||
		daemon.findContainer(request.WriteRequest.Spec.ContainerName) != nil {
		t.Fatalf("foreign volume collision was modified: daemon=%#v err=%v", daemon, err)
	}
}

func TestDockerHostInputHandoffInvalidBundleCleansOnlyExactOriginal(t *testing.T) {
	request, bundle := newDockerHostInputHandoffTransportFixture(t)
	bundle.Reader = bytes.NewReader([]byte("changed after capture"))
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerHandoffTestDaemon{containers: map[string]*handoffTestContainer{
		request.WriteRequest.Spec.ContainerName: {id: dockerWriteTestContainerID,
			name:    request.WriteRequest.Spec.ContainerName,
			payload: dockerCreatePayload(request.WriteRequest)},
	}}
	transport, _ := newDockerEngineContainerWriteTransport(daemon, endpoint)
	_, err := transport.Handoff(context.Background(), request, bundle)
	if DockerHostInputHandoffErrorCode(err) != DockerHostInputHandoffErrorInvalidBundle ||
		len(daemon.containers) != 0 || daemon.volume != nil || daemon.volumeDeletes != 0 ||
		daemon.containerStarts != 0 {
		t.Fatalf("invalid bundle left owned resources or widened authority: daemon=%#v err=%v",
			daemon, err)
	}
	for _, call := range daemon.requests {
		if strings.Contains(call, "/archive") || strings.Contains(call, "/start") ||
			strings.Contains(call, "/exec") || strings.Contains(call, "/volumes/create") {
			t.Fatalf("invalid bundle reached a forbidden side effect: %s", call)
		}
	}
}

func TestDockerHostInputHandoffReconcilesOnlyExactCrashResidue(t *testing.T) {
	request, bundle := newDockerHostInputHandoffTransportFixture(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerHandoffTestDaemon{containers: map[string]*handoffTestContainer{
		request.WriteRequest.Spec.ContainerName: {id: strings.Repeat("d", 64),
			name:    request.WriteRequest.Spec.ContainerName,
			payload: dockerHostInputContainerPayload(request, false)},
		request.CarrierName: {id: strings.Repeat("c", 64), name: request.CarrierName,
			payload: dockerHostInputContainerPayload(request, true)},
	}, volume: &dockerVolumeInspection{Name: request.VolumeName, Driver: "local",
		Mountpoint: "/var/lib/docker/volumes/" + request.VolumeName + "/_data",
		Scope:      "local", Options: map[string]string{},
		Labels: dockerHostInputVolumeLabels(request)}}
	transport, _ := newDockerEngineContainerWriteTransport(daemon, endpoint)
	result, err := transport.Handoff(context.Background(), request, bundle)
	if err != nil || result.Validate() != nil || result.ReconciledResourceCount != 3 ||
		len(daemon.containers) != 0 || daemon.volume != nil || daemon.volumeDeletes != 2 ||
		daemon.containerStarts != 0 {
		t.Fatalf("exact crash residue did not converge: result=%#v daemon=%#v err=%v",
			result, daemon, err)
	}
}

func TestDockerHostInputHandoffAllowlistRejectsExecutionAndArbitraryPaths(t *testing.T) {
	id := strings.Repeat("c", 64)
	for _, operation := range []struct {
		method, path, query, contentType string
		body                             []byte
	}{
		{http.MethodPost, "/v1.40/containers/" + id + "/start", "", "", nil},
		{http.MethodPost, "/v1.40/containers/" + id + "/exec", "", "application/json", []byte(`{}`)},
		{http.MethodPut, "/v1.40/containers/" + id + "/archive", "path=%2Fetc", "application/x-tar", []byte("tar")},
		{http.MethodDelete, "/v1.40/volumes/foreign", "force=1", "", nil},
		{http.MethodGet, "/v1.40/networks", "", "", nil},
	} {
		if validDockerHostInputHandoffOperation(operation.method, operation.path,
			operation.query, operation.body, operation.contentType) {
			t.Fatalf("handoff allowlist accepted %s %s?%s", operation.method,
				operation.path, operation.query)
		}
	}
}

func TestDockerHostInputHandoffReservesItsDestinationTree(t *testing.T) {
	for _, target := range []string{"/", "/cyberagent-input", "/cyberagent-input/nested"} {
		if !dockerHostInputDestinationConflicts([]DockerHostMount{{Target: target}}) {
			t.Fatalf("handoff accepted overlapping mount target %q", target)
		}
	}
	for _, target := range []string{"/workspace", "/output", "/cyberagent-input-other"} {
		if dockerHostInputDestinationConflicts([]DockerHostMount{{Target: target}}) {
			t.Fatalf("handoff rejected independent mount target %q", target)
		}
	}
}

func newDockerHostInputHandoffTransportFixture(t *testing.T) (
	DockerHostInputHandoffRequest, *handoffTestBundle,
) {
	t.Helper()
	ctx := context.Background()
	manifest := dockerContainerCompilerManifest()
	manifest.Network = NetworkScope{Mode: "disabled"}
	manifest.Environment = nil
	manifest.InputArtifactIDs = nil
	observation := dockerContainerCompilerObservation(t, ctx, manifest, true, 8,
		8*1024*1024*1024)
	spec, err := CompileDockerContainerSpec(ctx, observation, manifest)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := NewInMemoryDockerWriteTransaction().Simulate(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewDockerContainerPlan("docker-handoff-plan", observation, spec,
		transaction, observation.RequestedBy, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, name := range []string{"output", "src"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeRequest, err := NewDockerContainerWriteRequest(ctx, root, spec)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	attemptIntent, err := NewDockerContainerAttemptIntent("docker-handoff-attempt",
		strings.Repeat("a", 64), plan, writeRequest, endpoint, plan.RequestedBy,
		time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	captureRequirement, err := NewDockerHostInputRequirement(attemptIntent, plan, true, true)
	if err != nil {
		t.Fatal(err)
	}
	handoffRequirement, err := NewDockerHostInputHandoffRequirement(attemptIntent, plan,
		captureRequirement, true, true)
	if err != nil {
		t.Fatal(err)
	}
	lease := DockerContainerAttemptLease{AttemptID: attemptIntent.ID,
		LeaseID: "docker-handoff-lease", OwnerID: "docker_handoff_owner", Generation: 1,
		Status: DockerContainerAttemptLeaseActive, AcquiredAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(time.Minute)}
	stageResult, err := NewDockerContainerStageResult(endpoint, writeRequest,
		dockerWriteTestContainerID, false)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := NewDockerContainerAttemptStage(attemptIntent.ID, 1, stageResult,
		time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	attempt := DockerContainerRehearsalAttempt{Intent: attemptIntent,
		HostInputRequirement:        &captureRequirement,
		HostInputHandoffRequirement: &handoffRequirement,
		Status:                      DockerContainerAttemptStatusStaged, Lease: lease, Stage: &stage}
	if err := attempt.Validate(); err != nil {
		t.Fatal(err)
	}
	stagingIntent, err := NewDockerHostInputStagingIntent("docker-handoff-staging-intent",
		strings.Repeat("b", 64), attempt, plan, manifest, plan.RequestedBy, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	inner := deterministicHandoffInnerArchive(t)
	report, err := NewHostInputBundleReport(HostInputBundleMeasurements{
		ReadOnlyMountCount: plan.ReadOnlyMountCount, RegularFileCount: 1,
		BundleBytes: int64(len(inner)), SourceSnapshotDigest: strings.Repeat("e", 64),
		ArtifactPayloadDigest: hostInputArtifactPayloadDigest(nil),
		BundleDigest:          hashHostInputBytes(inner)}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	stagingValue, err := NewDockerHostInputStaging("docker-handoff-staging",
		stagingIntent, 1, report, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	staging := DockerHostInputStagingRecord{Intent: stagingIntent, Staging: &stagingValue}
	handoffIntent, err := NewDockerHostInputHandoffIntent("docker-handoff-intent",
		strings.Repeat("f", 64), attempt, plan, staging, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	request, err := NewDockerHostInputHandoffRequest(handoffIntent, writeRequest,
		stageResult, report)
	if err != nil {
		t.Fatal(err)
	}
	return request, &handoffTestBundle{Reader: bytes.NewReader(inner), report: report}
}

func deterministicHandoffInnerArchive(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	content := []byte("sealed host input")
	if err := writer.WriteHeader(&tar.Header{Name: "mounts/001/input.txt",
		Typeflag: tar.TypeReg, Mode: 0o444, Size: int64(len(content)),
		ModTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
