package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type runtimeInputTestDaemon struct {
	mu              sync.Mutex
	containers      map[string]*handoffTestContainer
	volumes         map[string]*dockerVolumeInspection
	archives        map[string][]byte
	requests        []string
	sequence        int
	foreignDeletes  int
	containerStarts int
	corruptReadback bool
}

func (daemon *runtimeInputTestDaemon) Do(request *http.Request) (*http.Response, error) {
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	daemon.requests = append(daemon.requests, request.Method+" "+request.URL.RequestURI())
	base := "/v" + DockerContainerWriteAPIVersion
	endpoint := request.URL.Path
	if request.Method == http.MethodGet && strings.HasPrefix(endpoint, base+"/images/") {
		digest, _ := url.PathUnescape(strings.TrimSuffix(
			strings.TrimPrefix(endpoint, base+"/images/"), "/json"))
		return handoffTestJSONResponse(request, http.StatusOK, map[string]any{
			"Id":          "sha256:" + strings.Repeat("d", 64),
			"RepoDigests": []string{"example.invalid/runtime-input@" + digest},
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
		daemon.sequence++
		id := fmt.Sprintf("%064x", daemon.sequence)
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
		volumeName := container.payload.HostConfig.Mounts[0].Source
		switch request.Method {
		case http.MethodPut:
			data, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			daemon.archives[volumeName] = append([]byte(nil), data...)
			return handoffTestResponse(request, http.StatusOK, nil, ""), nil
		case http.MethodGet:
			archive := wrapRuntimeInputTestReadback(daemon.archives[volumeName])
			if daemon.corruptReadback {
				archive = wrapRuntimeInputTestReadback(runtimeInputTestArchive("changed.txt", "changed"))
			}
			return handoffTestResponse(request, http.StatusOK, archive, "application/x-tar"), nil
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
		payload := (&dockerHandoffTestDaemon{}).inspectPayload(container).(map[string]any)
		for _, mount := range payload["Mounts"].([]map[string]any) {
			if mount["Type"] == "volume" {
				mount["Mode"] = ""
			}
		}
		return handoffTestJSONResponse(request, http.StatusOK, payload), nil
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

func (daemon *runtimeInputTestDaemon) handleVolume(request *http.Request) (*http.Response, error) {
	base := "/v" + DockerContainerWriteAPIVersion
	if request.Method == http.MethodPost && request.URL.Path == base+"/volumes/create" {
		var payload struct {
			Name   string            `json:"Name"`
			Labels map[string]string `json:"Labels"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if existing := daemon.volumes[payload.Name]; existing != nil {
			return handoffTestJSONResponse(request, http.StatusCreated, existing), nil
		}
		value := &dockerVolumeInspection{Name: payload.Name, Driver: "local",
			Mountpoint: "/var/lib/docker/volumes/" + payload.Name + "/_data",
			Labels:     payload.Labels, Scope: "local", Options: map[string]string{}}
		daemon.volumes[payload.Name] = value
		return handoffTestJSONResponse(request, http.StatusCreated, value), nil
	}
	name, _ := url.PathUnescape(strings.TrimPrefix(request.URL.Path, base+"/volumes/"))
	value := daemon.volumes[name]
	if value == nil {
		return handoffTestResponse(request, http.StatusNotFound, nil, ""), nil
	}
	if request.Method == http.MethodGet {
		return handoffTestJSONResponse(request, http.StatusOK, value), nil
	}
	if request.Method == http.MethodDelete {
		if value.Labels["owner"] == "foreign" {
			daemon.foreignDeletes++
		}
		delete(daemon.volumes, name)
		delete(daemon.archives, name)
		return handoffTestResponse(request, http.StatusNoContent, nil, ""), nil
	}
	return handoffTestResponse(request, http.StatusMethodNotAllowed, nil, ""), nil
}

func (daemon *runtimeInputTestDaemon) findContainer(reference string) *handoffTestContainer {
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

func TestDockerRuntimeInputApplicationLeavesOnlyNeverStartedTargetAndReadOnlyVolumes(t *testing.T) {
	intent, lease, request := newDockerRuntimeInputApplicationTransportFixture(t)
	daemon := &runtimeInputTestDaemon{containers: map[string]*handoffTestContainer{},
		volumes: map[string]*dockerVolumeInspection{}, archives: map[string][]byte{}}
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	transport, _ := newDockerEngineContainerWriteTransport(daemon, endpoint)
	result, err := transport.Apply(context.Background(), intent, lease, request)
	if err != nil || result.Validate() != nil || !result.TargetContainerPresent ||
		result.ContainerStarted || result.ProcessExecuted || daemon.containerStarts != 0 ||
		len(daemon.containers) != 1 || len(daemon.volumes) != len(request.Mounts) {
		t.Fatalf("runtime input application escaped boundary: result=%#v daemon=%#v err=%v",
			result, daemon, err)
	}
	target := daemon.findContainer(request.Spec.ContainerName)
	if target == nil || verifyDockerRuntimeInputContainer(
		decodeRuntimeInputTestInspection(t, target), request, nil) != nil {
		t.Fatal("final target configuration was not exact")
	}
	for _, call := range daemon.requests {
		if strings.Contains(call, "/start") || strings.Contains(call, "/exec") ||
			strings.Contains(call, "/export") || strings.Contains(call, "/attach") ||
			strings.Contains(call, "/networks/") {
			t.Fatalf("runtime input application issued forbidden endpoint: %s", call)
		}
	}
}

func TestDockerRuntimeInputApplicationRequiresLeaseCleanupWindow(t *testing.T) {
	intent, lease, request := newDockerRuntimeInputApplicationTransportFixture(t)
	lease.ExpiresAt = time.Now().UTC().Add(dockerRuntimeInputApplicationLeaseSafety / 2)
	daemon := &runtimeInputTestDaemon{containers: map[string]*handoffTestContainer{},
		volumes: map[string]*dockerVolumeInspection{}, archives: map[string][]byte{}}
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	transport, _ := newDockerEngineContainerWriteTransport(daemon, endpoint)
	_, err := transport.Apply(context.Background(), intent, lease, request)
	if DockerRuntimeInputApplicationErrorCode(err) != DockerRuntimeInputApplicationErrorDeadline ||
		len(daemon.requests) != 0 {
		t.Fatalf("short lease reached Docker daemon: requests=%v err=%v", daemon.requests, err)
	}
}

func TestDockerRuntimeInputApplicationLeaseTTLReservesCleanupTime(t *testing.T) {
	if ValidateDockerRuntimeInputApplicationLeaseTTL(30*time.Second) == nil ||
		ValidateDockerRuntimeInputApplicationLeaseTTL(time.Minute) != nil {
		t.Fatal("runtime-input application lease TTL did not reserve cleanup time")
	}
}

func TestDockerRuntimeInputApplicationRejectsFutureLeaseBeforeDaemon(t *testing.T) {
	intent, lease, request := newDockerRuntimeInputApplicationTransportFixture(t)
	lease.AcquiredAt = time.Now().UTC().Add(time.Minute)
	lease.ExpiresAt = lease.AcquiredAt.Add(time.Hour)
	daemon := &runtimeInputTestDaemon{containers: map[string]*handoffTestContainer{},
		volumes: map[string]*dockerVolumeInspection{}, archives: map[string][]byte{}}
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	transport, _ := newDockerEngineContainerWriteTransport(daemon, endpoint)
	_, err := transport.Apply(context.Background(), intent, lease, request)
	if DockerRuntimeInputApplicationErrorCode(err) != DockerRuntimeInputApplicationErrorUnsupported ||
		len(daemon.requests) != 0 {
		t.Fatalf("future lease reached Docker daemon: requests=%v err=%v", daemon.requests, err)
	}
}

func TestDockerRuntimeInputApplicationRejectsForeignVolumeWithoutDeletingIt(t *testing.T) {
	intent, lease, request := newDockerRuntimeInputApplicationTransportFixture(t)
	foreign := &dockerVolumeInspection{Name: request.Mounts[0].VolumeName, Driver: "local",
		Mountpoint: "/foreign", Scope: "local", Options: map[string]string{},
		Labels: map[string]string{"owner": "foreign"}}
	daemon := &runtimeInputTestDaemon{containers: map[string]*handoffTestContainer{},
		volumes:  map[string]*dockerVolumeInspection{foreign.Name: foreign},
		archives: map[string][]byte{}}
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	transport, _ := newDockerEngineContainerWriteTransport(daemon, endpoint)
	_, err := transport.Apply(context.Background(), intent, lease, request)
	if DockerRuntimeInputApplicationErrorCode(err) != DockerRuntimeInputApplicationErrorUnsafeCollision ||
		daemon.volumes[foreign.Name] == nil || daemon.foreignDeletes != 0 {
		t.Fatalf("foreign runtime volume was modified: daemon=%#v err=%v", daemon, err)
	}
}

func TestDockerRuntimeInputApplicationRejectsMissingNoCopyEvidence(t *testing.T) {
	_, _, request := newDockerRuntimeInputApplicationTransportFixture(t)
	payload := runtimeInputContainerPayload(request, nil)
	for index := range payload.HostConfig.Mounts {
		if payload.HostConfig.Mounts[index].Type == "volume" {
			payload.HostConfig.Mounts[index].VolumeOptions = &dockerCreateVolumeOptions{NoCopy: false}
		}
	}
	inspection := decodeRuntimeInputTestInspection(t, &handoffTestContainer{
		id: strings.Repeat("e", 64), name: request.Spec.ContainerName, payload: payload,
	})
	if DockerRuntimeInputApplicationErrorCode(
		verifyDockerRuntimeInputContainer(inspection, request, nil)) !=
		DockerRuntimeInputApplicationErrorConfigMismatch {
		t.Fatal("runtime-input target accepted a volume without NoCopy evidence")
	}
}

func TestDockerRuntimeInputApplicationReadbackMismatchCleansExactResources(t *testing.T) {
	intent, lease, request := newDockerRuntimeInputApplicationTransportFixture(t)
	daemon := &runtimeInputTestDaemon{containers: map[string]*handoffTestContainer{},
		volumes: map[string]*dockerVolumeInspection{}, archives: map[string][]byte{},
		corruptReadback: true}
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	transport, _ := newDockerEngineContainerWriteTransport(daemon, endpoint)
	_, err := transport.Apply(context.Background(), intent, lease, request)
	if DockerRuntimeInputApplicationErrorCode(err) != DockerRuntimeInputApplicationErrorReadbackMismatch ||
		len(daemon.containers) != 0 || len(daemon.volumes) != 0 || daemon.containerStarts != 0 {
		t.Fatalf("readback mismatch left owned resources: daemon=%#v err=%v", daemon, err)
	}
}

func TestDockerRuntimeInputApplicationAllowlistRejectsExecutionAndArbitraryPaths(t *testing.T) {
	id := strings.Repeat("a", 64)
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
		if validDockerRuntimeInputApplicationOperation(operation.method, operation.path,
			operation.query, operation.body, operation.contentType) {
			t.Fatalf("runtime input allowlist accepted %s %s?%s", operation.method,
				operation.path, operation.query)
		}
	}
}

func newDockerRuntimeInputApplicationTransportFixture(t *testing.T) (
	DockerRuntimeInputApplicationIntent, DockerRuntimeInputApplicationLease,
	DockerRuntimeInputApplicationRequest,
) {
	t.Helper()
	intent, lease, request, _, _ := newDockerRuntimeInputApplicationTransportFixtureFull(t)
	return intent, lease, request
}

func newDockerRuntimeInputApplicationTransportFixtureFull(t *testing.T) (
	DockerRuntimeInputApplicationIntent, DockerRuntimeInputApplicationLease,
	DockerRuntimeInputApplicationRequest, DockerRuntimeInputProjectionPlan,
	DockerContainerWriteRequest,
) {
	t.Helper()
	ctx := context.Background()
	manifest := dockerContainerCompilerManifest()
	manifest.Network = NetworkScope{Mode: "disabled"}
	manifest.Environment = nil
	observation := dockerContainerCompilerObservation(t, ctx, manifest, true, 8,
		8*1024*1024*1024)
	spec, err := CompileDockerContainerSpec(ctx, observation, manifest)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, name := range []string{"output", "src"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRequest, err := NewDockerContainerWriteRequest(ctx, root, spec)
	if err != nil {
		t.Fatal(err)
	}
	bundle := newRuntimeProjectionTestBundle(t, []runtimeProjectionTestEntry{
		{name: "mounts/001", kind: tar.TypeDir},
		{name: "mounts/001/main.go", kind: tar.TypeReg, content: "package main\n"},
		{name: "artifacts/001", kind: tar.TypeReg, content: "sealed artifact\n"},
	}, 1, 1)
	compilation, err := CompileDockerRuntimeInputProjectionBundle(ctx, manifest, bundle,
		runtimeProjectionTestBinding)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute)
	digest := strings.Repeat("b", 64)
	plan := DockerRuntimeInputProjectionPlan{
		ID: "runtime-projection", HandoffID: "runtime-handoff",
		HandoffIntentID: "runtime-handoff-intent", AttemptID: "runtime-attempt",
		ContainerPlanID: "runtime-container-plan", RunID: spec.RunID,
		MissionID: "runtime-mission", WorkspaceID: "runtime-workspace",
		ProtocolVersion:    DockerRuntimeInputProjectionPlanProtocolVersion,
		Status:             DockerRuntimeInputProjectionStatusCompiled,
		TrustClass:         DockerRuntimeInputProjectionTrustClass,
		OperationKeyDigest: digest, ManifestFingerprint: compilation.ManifestFingerprint,
		MountBindingFingerprint: writeRequest.MountFingerprint,
		InputArtifactDigest:     digest, AuthorityFingerprint: digest,
		SpecFingerprint: spec.SpecFingerprint, ContainerPlanFingerprint: digest,
		HandoffFingerprint:          runtimeProjectionTestBinding,
		HandoffTransportFingerprint: digest,
		BundleReportFingerprint:     compilation.BundleReportFingerprint,
		BundleDigest:                compilation.BundleDigest, BundleBytes: compilation.BundleBytes,
		ReadOnlyMountCount: compilation.ReadOnlyMountCount,
		InputArtifactCount: compilation.InputArtifactCount,
		ProjectionCount:    len(compilation.Items), DirectoryRootCount: compilation.DirectoryRootCount,
		FileRootCount: compilation.FileRootCount, TotalEntryCount: compilation.TotalEntryCount,
		TotalContentBytes:        compilation.TotalContentBytes,
		TotalProjectionBytes:     compilation.TotalProjectionBytes,
		ProjectionSetFingerprint: compilation.ProjectionSetFingerprint,
		OperatorConfirmed:        true, ExactTargetBinding: true, AllVolumesReadOnly: true,
		AllVolumesNoCopy: true, BundleRecaptured: true, BundleDigestMatched: true,
		Items:       append([]DockerRuntimeInputProjectionItem(nil), compilation.Items...),
		RequestedBy: observation.RequestedBy, CreatedAt: now,
	}
	plan.RequestFingerprint = dockerRuntimeInputProjectionRequestFingerprint(plan)
	plan.ProjectionFingerprint = dockerRuntimeInputProjectionPlanFingerprint(plan)
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	intent, err := NewDockerRuntimeInputApplicationIntent("runtime-application", digest,
		plan, endpoint, true, true, plan.RequestedBy, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	lease := DockerRuntimeInputApplicationLease{IntentID: intent.ID,
		LeaseID: "runtime-application-lease", OwnerID: "runtime_application_owner",
		Generation: 1, Status: DockerRuntimeInputApplicationLeaseActive,
		AcquiredAt: now.Add(2 * time.Second), ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}
	request, err := NewDockerRuntimeInputApplicationRequest(intent, plan, compilation, writeRequest)
	if err != nil {
		t.Fatal(err)
	}
	return intent, lease, request, plan, writeRequest
}

func wrapRuntimeInputTestReadback(inner []byte) []byte {
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	_ = writer.WriteHeader(&tar.Header{Name: "cyberagent-input/", Typeflag: tar.TypeDir,
		Mode: 0o555})
	reader := tar.NewReader(bytes.NewReader(inner))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil
		}
		copyHeader := *header
		copyHeader.Name = "cyberagent-input/" + header.Name
		_ = writer.WriteHeader(&copyHeader)
		if header.Typeflag == tar.TypeReg {
			_, _ = io.Copy(writer, reader)
		}
	}
	_ = writer.Close()
	return output.Bytes()
}

func runtimeInputTestArchive(name, content string) []byte {
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	_ = writer.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg,
		Mode: 0o444, Size: int64(len(content))})
	_, _ = io.WriteString(writer, content)
	_ = writer.Close()
	return output.Bytes()
}

func decodeRuntimeInputTestInspection(t *testing.T, container *handoffTestContainer,
) dockerContainerInspection {
	t.Helper()
	body, err := json.Marshal((&dockerHandoffTestDaemon{}).inspectPayload(container))
	if err != nil {
		t.Fatal(err)
	}
	var value dockerContainerInspection
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
