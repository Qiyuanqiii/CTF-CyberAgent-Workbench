package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const dockerWriteTestContainerID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

type dockerWriteTestDaemon struct {
	mu                              sync.Mutex
	containerID                     string
	name                            string
	payload                         dockerCreateContainerPayload
	requests                        []string
	creates                         int
	deletes                         int
	afterCreate                     func()
	failCreateResponseAfterMutation bool
	imageVolumes                    bool
	imageEnvironment                bool
	unsafe                          bool
}

func (daemon *dockerWriteTestDaemon) Do(request *http.Request) (*http.Response, error) {
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	daemon.requests = append(daemon.requests, request.Method+" "+request.URL.RequestURI())
	path := request.URL.Path
	switch request.Method {
	case http.MethodGet:
		imagePrefix := "/v" + DockerContainerWriteAPIVersion + "/images/"
		if strings.HasPrefix(path, imagePrefix) && strings.HasSuffix(path, "/json") {
			digest, _ := url.PathUnescape(strings.TrimSuffix(
				strings.TrimPrefix(path, imagePrefix), "/json"))
			volumes := map[string]json.RawMessage(nil)
			if daemon.imageVolumes {
				volumes = map[string]json.RawMessage{
					"/declared-volume": json.RawMessage(`{}`),
				}
			}
			var environment []string
			if daemon.imageEnvironment {
				environment = []string{"PATH=/untrusted/image/default"}
			}
			payload, _ := json.Marshal(map[string]any{
				"Id":          "sha256:" + strings.Repeat("d", 64),
				"RepoDigests": []string{"example.invalid/rehearsal@" + digest},
				"Config":      map[string]any{"Volumes": volumes, "Env": environment},
			})
			return dockerWriteTestResponse(request, http.StatusOK, payload), nil
		}
		if daemon.containerID == "" {
			return dockerWriteTestResponse(request, http.StatusNotFound, nil), nil
		}
		reference := strings.TrimSuffix(strings.TrimPrefix(path,
			"/v"+DockerContainerWriteAPIVersion+"/containers/"), "/json")
		decoded, _ := url.PathUnescape(reference)
		if decoded != daemon.containerID && decoded != daemon.name {
			return dockerWriteTestResponse(request, http.StatusNotFound, nil), nil
		}
		payload := daemon.inspectPayload()
		return dockerWriteTestResponse(request, http.StatusOK, payload), nil
	case http.MethodPost:
		if daemon.containerID != "" {
			return dockerWriteTestResponse(request, http.StatusConflict, []byte(`{"message":"conflict"}`)), nil
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &daemon.payload); err != nil {
			return nil, err
		}
		daemon.containerID = dockerWriteTestContainerID
		daemon.name = request.URL.Query().Get("name")
		daemon.creates++
		response := dockerWriteTestResponse(request, http.StatusCreated,
			[]byte(`{"Id":"`+dockerWriteTestContainerID+`","Warnings":[]}`))
		if daemon.afterCreate != nil {
			daemon.afterCreate()
		}
		if daemon.failCreateResponseAfterMutation {
			return nil, request.Context().Err()
		}
		return response, nil
	case http.MethodDelete:
		if daemon.containerID == "" {
			return dockerWriteTestResponse(request, http.StatusNotFound, nil), nil
		}
		daemon.containerID = ""
		daemon.deletes++
		return dockerWriteTestResponse(request, http.StatusNoContent, nil), nil
	default:
		return dockerWriteTestResponse(request, http.StatusMethodNotAllowed, nil), nil
	}
}

func (daemon *dockerWriteTestDaemon) inspectPayload() []byte {
	labels := daemon.payload.Labels
	if daemon.unsafe {
		labels = map[string]string{"io.cyberagent.managed": "false"}
	}
	mounts := make([]map[string]any, len(daemon.payload.HostConfig.Mounts))
	for index, mount := range daemon.payload.HostConfig.Mounts {
		mounts[index] = map[string]any{"Type": "bind", "Source": mount.Source,
			"Destination": mount.Target, "RW": !mount.ReadOnly,
			"Propagation": mount.BindOptions.Propagation}
	}
	payload := map[string]any{
		"Id": daemon.containerID, "Name": "/" + daemon.name,
		"Created": "2026-07-15T00:00:00Z",
		"Config": map[string]any{
			"Image": daemon.payload.Image, "Entrypoint": daemon.payload.Entrypoint,
			"Cmd": daemon.payload.Cmd, "Env": daemon.payload.Env,
			"WorkingDir": daemon.payload.WorkingDir,
			"User":       daemon.payload.User, "NetworkDisabled": daemon.payload.NetworkDisabled,
			"Labels": labels, "StopSignal": daemon.payload.StopSignal,
		},
		"State": map[string]any{"Status": "created", "Running": false, "Paused": false,
			"Restarting": false, "OOMKilled": false, "Dead": false, "Pid": 0},
		"HostConfig": daemon.payload.HostConfig,
		"Mounts":     mounts,
	}
	data, _ := json.Marshal(payload)
	return data
}

func dockerWriteTestResponse(request *http.Request, status int, body []byte) *http.Response {
	response := &http.Response{StatusCode: status, Header: make(http.Header),
		Body: io.NopCloser(bytes.NewReader(body)), Request: request}
	if len(body) > 0 {
		response.Header.Set("Content-Type", "application/json")
	}
	return response
}

func TestDockerContainerWriteTransportRehearsesAndReconcilesWithoutStarting(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerWriteTestDaemon{}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}

	first, err := transport.Rehearse(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ReconciledContainerCount != 0 || first.DaemonWriteCount != 2 ||
		first.ContainerStarted || first.ProcessExecuted || daemon.creates != 1 ||
		daemon.deletes != 1 || daemon.containerID != "" {
		t.Fatalf("Docker write rehearsal escaped its boundary: result=%#v daemon=%#v", first, daemon)
	}

	// A matching, unstarted orphan is inspected and removed before a new rehearsal.
	daemon.payload = dockerCreatePayload(request)
	daemon.containerID = dockerWriteTestContainerID
	daemon.name = request.Spec.ContainerName
	second, err := transport.Rehearse(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.ReconciledContainerCount != 1 || second.DaemonWriteCount != 3 ||
		daemon.creates != 2 || daemon.deletes != 3 || daemon.containerID != "" {
		t.Fatalf("Docker orphan reconciliation is invalid: result=%#v daemon=%#v", second, daemon)
	}
	for _, call := range daemon.requests {
		if strings.Contains(call, "/start") || strings.Contains(call, "/exec") ||
			strings.Contains(call, "/attach") || strings.Contains(call, "/images/create") {
			t.Fatalf("write rehearsal issued a forbidden Docker endpoint: %s", call)
		}
	}
}

func TestDockerContainerWriteTransportRejectsUnsafeExistingContainer(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerWriteTestDaemon{containerID: dockerWriteTestContainerID,
		name: request.Spec.ContainerName, payload: dockerCreatePayload(request), unsafe: true}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Rehearse(context.Background(), request)
	if DockerContainerWriteErrorCode(err) != DockerContainerWriteFailureUnsafeExisting ||
		daemon.creates != 0 || daemon.deletes != 0 || daemon.containerID == "" {
		t.Fatalf("unsafe name collision was modified: daemon=%#v err=%v", daemon, err)
	}
}

func TestDockerContainerWriteTransportRejectsImageDeclaredVolumesBeforeCreate(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerWriteTestDaemon{imageVolumes: true}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Rehearse(context.Background(), request)
	if DockerContainerWriteErrorCode(err) != DockerContainerWriteFailureUnsafeImage ||
		daemon.creates != 0 || daemon.deletes != 0 || daemon.containerID != "" {
		t.Fatalf("image-declared volume reached Docker create: daemon=%#v err=%v", daemon, err)
	}
}

func TestDockerContainerWriteTransportRejectsImageEnvironmentBeforeCreate(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerWriteTestDaemon{imageEnvironment: true}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Stage(context.Background(), request)
	if DockerContainerWriteErrorCode(err) != DockerContainerWriteFailureUnsafeImage ||
		daemon.creates != 0 || daemon.deletes != 0 || daemon.containerID != "" {
		t.Fatalf("image environment reached Docker create: daemon=%#v err=%v", daemon, err)
	}
}

func TestDockerContainerStageAndCleanupKeepStoppedRecoveryEvidence(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerWriteTestDaemon{}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := transport.Stage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !stage.ContainerCreatedNow || stage.ExistingContainerAdopted ||
		stage.ControlCount != 19 || len(stage.Controls) != 19 || daemon.creates != 1 ||
		daemon.deletes != 0 || daemon.containerID == "" {
		t.Fatalf("Docker stage did not preserve bounded recovery evidence: %#v daemon=%#v",
			stage, daemon)
	}
	cleanup, err := transport.Cleanup(context.Background(), request, stage)
	if err != nil {
		t.Fatal(err)
	}
	if !cleanup.ContainerRemovedNow || cleanup.ContainerAlreadyAbsent ||
		daemon.creates != 1 || daemon.deletes != 1 || daemon.containerID != "" {
		t.Fatalf("Docker exact cleanup is invalid: %#v daemon=%#v", cleanup, daemon)
	}
}

func TestDockerContainerStageAdoptsUncertainCreateWithoutCreatingTwice(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	ctx, cancel := context.WithCancel(context.Background())
	daemon := &dockerWriteTestDaemon{afterCreate: cancel,
		failCreateResponseAfterMutation: true}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Stage(ctx, request); !errorsIsContextCanceled(err) ||
		daemon.creates != 1 || daemon.deletes != 0 || daemon.containerID == "" {
		t.Fatalf("uncertain stage did not leave exact recovery evidence: daemon=%#v err=%v",
			daemon, err)
	}
	stage, err := transport.Stage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if stage.ContainerCreatedNow || !stage.ExistingContainerAdopted ||
		daemon.creates != 1 || daemon.deletes != 0 {
		t.Fatalf("recovery stage created twice: stage=%#v daemon=%#v", stage, daemon)
	}
	if _, err := transport.Cleanup(context.Background(), request, stage); err != nil ||
		daemon.deletes != 1 || daemon.containerID != "" {
		t.Fatalf("recovered stage cleanup failed: daemon=%#v err=%v", daemon, err)
	}
}

func TestDockerContainerCleanupIsIdempotentAndNeverDeletesUnrelatedContainer(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	daemon := &dockerWriteTestDaemon{}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := transport.Stage(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	daemon.containerID = ""
	cleanup, err := transport.Cleanup(context.Background(), request, stage)
	if err != nil || cleanup.ContainerRemovedNow || !cleanup.ContainerAlreadyAbsent ||
		daemon.deletes != 0 {
		t.Fatalf("already-absent cleanup was not idempotent: %#v daemon=%#v err=%v",
			cleanup, daemon, err)
	}
	daemon.containerID, daemon.name = dockerWriteTestContainerID, request.Spec.ContainerName
	daemon.payload, daemon.unsafe = dockerCreatePayload(request), true
	if _, err := transport.Cleanup(context.Background(), request, stage); DockerContainerWriteErrorCode(err) != DockerContainerWriteFailureUnsafeExisting ||
		daemon.deletes != 0 || daemon.containerID == "" {
		t.Fatalf("cleanup deleted an unrelated same-name container: daemon=%#v err=%v",
			daemon, err)
	}
}

func TestDockerContainerWriteTransportCleansUpAfterCancellation(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	ctx, cancel := context.WithCancel(context.Background())
	daemon := &dockerWriteTestDaemon{afterCreate: cancel}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Rehearse(ctx, request); !errorsIsContextCanceled(err) {
		t.Fatalf("Docker rehearsal did not surface cancellation: %v", err)
	}
	if daemon.creates != 1 || daemon.deletes != 1 || daemon.containerID != "" {
		t.Fatalf("Docker rehearsal cancellation left an orphan: %#v", daemon)
	}
}

func TestDockerContainerWriteTransportReconcilesUnknownCreateOutcome(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	ctx, cancel := context.WithCancel(context.Background())
	daemon := &dockerWriteTestDaemon{afterCreate: cancel,
		failCreateResponseAfterMutation: true}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Rehearse(ctx, request); !errorsIsContextCanceled(err) {
		t.Fatalf("Docker rehearsal did not surface the uncertain create cancellation: %v", err)
	}
	if daemon.creates != 1 || daemon.deletes != 1 || daemon.containerID != "" {
		t.Fatalf("uncertain Docker create outcome left an orphan: %#v", daemon)
	}
}

func TestDockerContainerWriteTransportNeverBlindDeletesAfterFailure(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	ctx, cancel := context.WithCancel(context.Background())
	daemon := &dockerWriteTestDaemon{}
	daemon.afterCreate = func() {
		daemon.unsafe = true
		cancel()
	}
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	_, err = transport.Rehearse(ctx, request)
	if !errorsIsContextCanceled(err) ||
		DockerContainerWriteErrorCode(err) != DockerContainerWriteFailureCleanup {
		t.Fatalf("unsafe cleanup did not return cancellation plus cleanup failure: %v", err)
	}
	if daemon.creates != 1 || daemon.deletes != 0 || daemon.containerID == "" {
		t.Fatalf("failure cleanup blindly deleted a mismatched container: %#v", daemon)
	}
}

func TestDockerContainerWriteRequestRejectsWiderProfilesAndSymlinks(t *testing.T) {
	request := newDockerContainerWriteTestRequest(t)
	changed := request.Spec
	changed.Network.Mode = "allowlist"
	changed.Network.Driver = DockerNetworkDriverManagedEgress
	changed.Network.AllowedTargets = []string{"example.invalid:443"}
	changed.Network.ExactAllowlist = true
	changed.Network.GuardRequired = true
	finalizeDockerContainerSpec(&changed)
	if err := ValidateDockerContainerRehearsalProfile(changed); err == nil {
		t.Fatal("Docker rehearsal accepted an allowlisted network profile")
	}

	changed = request.Spec
	changed.Environment = []DockerContainerEnvironmentSpec{{Name: "MODE",
		Source: EnvironmentLiteral, LiteralValue: "test"}}
	finalizeDockerContainerSpec(&changed)
	if err := ValidateDockerContainerRehearsalProfile(changed); err == nil {
		t.Fatal("Docker rehearsal accepted environment material")
	}

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "real"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "linked")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("Windows symlink privilege unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := resolveDockerRehearsalMountSource(root, "linked"); err == nil {
		t.Fatal("Docker rehearsal followed a symlinked mount source")
	}
}

func TestDockerContainerWriteHTTPAllowlistIsClosed(t *testing.T) {
	name := "cyberagent-" + strings.Repeat("a", 24)
	id := strings.Repeat("b", 64)
	digest := "sha256:" + strings.Repeat("c", 64)
	validBody := []byte(`{"Image":"sha256:` + strings.Repeat("c", 64) + `"}`)
	if !validDockerContainerWriteOperation(http.MethodGet,
		"/v1.40/containers/"+name+"/json", "", nil) ||
		!validDockerContainerWriteOperation(http.MethodGet,
			"/v1.40/images/"+digest+"/json", "", nil) ||
		!validDockerContainerWriteOperation(http.MethodDelete,
			"/v1.40/containers/"+id, "v=1", nil) ||
		!validDockerContainerWriteOperation(http.MethodPost,
			"/v1.40/containers/create", "name="+name, validBody) {
		t.Fatal("Docker write allowlist rejected a fixed rehearsal endpoint")
	}
	for _, test := range []struct {
		method, path, query string
		body                []byte
	}{
		{http.MethodPost, "/v1.40/containers/" + id + "/start", "", nil},
		{http.MethodPost, "/v1.40/containers/" + id + "/exec", "", validBody},
		{http.MethodPost, "/v1.40/images/create", "fromImage=alpine", validBody},
		{http.MethodDelete, "/v1.40/containers/" + name, "force=1", nil},
		{http.MethodDelete, "/v1.40/containers/" + id, "", nil},
		{http.MethodPost, "/v1.40/containers/create", "name=caller-name", validBody},
		{http.MethodGet, "/version", "", nil},
	} {
		if validDockerContainerWriteOperation(test.method, test.path, test.query, test.body) {
			t.Fatalf("Docker write allowlist accepted %s %s?%s", test.method, test.path, test.query)
		}
	}
}

func newDockerContainerWriteTestRequest(t *testing.T) DockerContainerWriteRequest {
	t.Helper()
	manifest := dockerContainerCompilerManifest()
	manifest.Network = NetworkScope{Mode: "disabled"}
	manifest.Environment = nil
	manifest.InputArtifactIDs = nil
	observation := dockerContainerCompilerObservation(t, context.Background(), manifest,
		true, 8, 8*1024*1024*1024)
	spec, err := CompileDockerContainerSpec(context.Background(), observation, manifest)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, path := range []string{"output", "src"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	request, err := NewDockerContainerWriteRequest(context.Background(), root, spec)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func errorsIsContextCanceled(err error) bool {
	return err != nil && (err == context.Canceled || strings.Contains(err.Error(), context.Canceled.Error()))
}
