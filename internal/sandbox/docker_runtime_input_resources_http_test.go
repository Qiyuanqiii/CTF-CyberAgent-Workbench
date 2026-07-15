package sandbox

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDockerRuntimeInputResourceInspectionAndExactCleanup(t *testing.T) {
	application, descriptor, transport, daemon := newDockerRuntimeInputResourceFixture(t)
	observation, err := transport.inspectRuntimeInputResources(context.Background(), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := NewDockerRuntimeInputResourceInspection("runtime-resource-inspection",
		strings.Repeat("2", 64), application.Intent.RequestedBy, application, descriptor, observation)
	if err != nil || !inspection.Complete || !inspection.CleanupEligible ||
		inspection.Status != DockerRuntimeInputResourceInspectionComplete {
		t.Fatalf("exact resources were not recognized: inspection=%#v err=%v", inspection, err)
	}

	intent, lease := newDockerRuntimeInputResourceCleanupFixture(t, inspection, descriptor,
		transport.Endpoint())
	daemon.requests = nil
	result, err := transport.cleanupRuntimeInputResources(context.Background(), intent, lease, descriptor)
	if err != nil || result.Validate() != nil || result.InitialOwnedResourceCount != len(descriptor.Mounts)+1 ||
		result.DeleteAttemptCount != len(descriptor.Mounts)+1 || len(daemon.containers) != 0 ||
		len(daemon.volumes) != 0 || daemon.containerStarts != 0 {
		t.Fatalf("exact cleanup failed: result=%#v containers=%d volumes=%d err=%v",
			result, len(daemon.containers), len(daemon.volumes), err)
	}
	firstDelete := ""
	for _, request := range daemon.requests {
		if strings.HasPrefix(request, http.MethodDelete+" ") {
			firstDelete = request
			break
		}
	}
	if !strings.Contains(firstDelete, "/containers/") {
		t.Fatalf("cleanup did not remove the never-started target before volumes: %v", daemon.requests)
	}
	assertNoRuntimeInputExecutionEndpoint(t, daemon.requests)
}

func TestDockerRuntimeInputResourceCleanupAcceptsAbsentResources(t *testing.T) {
	application, descriptor, transport, daemon := newDockerRuntimeInputResourceFixture(t)
	delete(daemon.volumes, descriptor.Mounts[0].VolumeName)
	observation, err := transport.inspectRuntimeInputResources(context.Background(), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := NewDockerRuntimeInputResourceInspection("runtime-resource-partial",
		strings.Repeat("3", 64), application.Intent.RequestedBy, application, descriptor, observation)
	if err != nil || inspection.Complete || !inspection.CleanupEligible ||
		inspection.AllOwnedVolumesReadOnly || inspection.AllOwnedVolumesNoCopy ||
		inspection.Status != DockerRuntimeInputResourceInspectionPartial || inspection.AbsentVolumeCount != 1 {
		t.Fatalf("partial exact state was not cleanup eligible: inspection=%#v err=%v", inspection, err)
	}
	intent, lease := newDockerRuntimeInputResourceCleanupFixture(t, inspection, descriptor,
		transport.Endpoint())
	result, err := transport.cleanupRuntimeInputResources(context.Background(), intent, lease, descriptor)
	if err != nil || result.InitialAbsentResourceCount != 1 ||
		result.InitialOwnedResourceCount != len(descriptor.Mounts) || len(daemon.containers) != 0 ||
		len(daemon.volumes) != 0 {
		t.Fatalf("partial cleanup failed: result=%#v containers=%d volumes=%d err=%v",
			result, len(daemon.containers), len(daemon.volumes), err)
	}
}

func TestDockerRuntimeInputResourceCleanupPreflightsAllResourcesBeforeDelete(t *testing.T) {
	application, descriptor, transport, daemon := newDockerRuntimeInputResourceFixture(t)
	observation, err := transport.inspectRuntimeInputResources(context.Background(), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := NewDockerRuntimeInputResourceInspection("runtime-resource-stale-inspection",
		strings.Repeat("4", 64), application.Intent.RequestedBy, application, descriptor, observation)
	if err != nil {
		t.Fatal(err)
	}
	intent, lease := newDockerRuntimeInputResourceCleanupFixture(t, inspection, descriptor,
		transport.Endpoint())
	foreign := daemon.volumes[descriptor.Mounts[len(descriptor.Mounts)-1].VolumeName]
	foreign.Labels = map[string]string{"owner": "foreign"}
	daemon.requests = nil
	_, err = transport.cleanupRuntimeInputResources(context.Background(), intent, lease, descriptor)
	for _, request := range daemon.requests {
		if strings.HasPrefix(request, http.MethodDelete+" ") {
			t.Fatalf("foreign collision caused a partial delete: requests=%v", daemon.requests)
		}
	}
	if DockerRuntimeInputResourceErrorCode(err) != DockerRuntimeInputResourceErrorUnsafeCollision ||
		len(daemon.containers) != 1 || len(daemon.volumes) != len(descriptor.Mounts) ||
		daemon.foreignDeletes != 0 {
		t.Fatalf("foreign collision was not fail-closed: daemon=%#v err=%v", daemon, err)
	}
}

func TestDockerRuntimeInputResourceInspectionReportsForeignWithoutAuthority(t *testing.T) {
	application, descriptor, transport, daemon := newDockerRuntimeInputResourceFixture(t)
	foreign := daemon.volumes[descriptor.Mounts[0].VolumeName]
	foreign.Labels = map[string]string{"owner": "foreign"}
	observation, err := transport.inspectRuntimeInputResources(context.Background(), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := NewDockerRuntimeInputResourceInspection("runtime-resource-unsafe",
		strings.Repeat("5", 64), application.Intent.RequestedBy, application, descriptor, observation)
	if err != nil || inspection.Status != DockerRuntimeInputResourceInspectionUnsafe ||
		inspection.CleanupEligible || inspection.ForeignResourceCount != 1 ||
		inspection.AllOwnedVolumesReadOnly || inspection.AllOwnedVolumesNoCopy ||
		inspection.ContainerStartAuthorized || inspection.ProcessExecutionAuthorized ||
		inspection.OutputExportAuthorized || inspection.ArtifactCommitAuthorized {
		t.Fatalf("foreign inspection widened authority: inspection=%#v err=%v", inspection, err)
	}
}

func newDockerRuntimeInputResourceFixture(t *testing.T) (
	DockerRuntimeInputApplicationRecord, DockerRuntimeInputResourceDescriptor,
	dockerEngineContainerWriteTransport, *runtimeInputTestDaemon,
) {
	t.Helper()
	intent, lease, request, projection, writeRequest :=
		newDockerRuntimeInputApplicationTransportFixtureFull(t)
	daemon := &runtimeInputTestDaemon{containers: map[string]*handoffTestContainer{},
		volumes: map[string]*dockerVolumeInspection{}, archives: map[string][]byte{}}
	endpoint, _ := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	transport, err := newDockerEngineContainerWriteTransport(daemon, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	result, err := transport.Apply(context.Background(), intent, lease, request)
	if err != nil {
		t.Fatal(err)
	}
	releasedAt := result.CreatedAt
	lease.Status, lease.ReleasedAt = DockerRuntimeInputApplicationLeaseReleased, &releasedAt
	application := DockerRuntimeInputApplicationRecord{Intent: intent, Lease: lease, Result: &result}
	if err := application.Validate(); err != nil {
		t.Fatal(err)
	}
	descriptor, err := NewDockerRuntimeInputResourceDescriptor(application, projection, writeRequest)
	if err != nil || descriptor.RequestFingerprint != result.RequestFingerprint {
		t.Fatalf("resource descriptor did not reconstruct v61 request: descriptor=%#v err=%v",
			descriptor, err)
	}
	return application, descriptor, transport, daemon
}

func newDockerRuntimeInputResourceCleanupFixture(t *testing.T,
	inspection DockerRuntimeInputResourceInspection, descriptor DockerRuntimeInputResourceDescriptor,
	endpoint DockerObservationEndpoint,
) (DockerRuntimeInputResourceCleanupIntent, DockerRuntimeInputResourceCleanupLease) {
	t.Helper()
	now := time.Now().UTC()
	if now.Before(inspection.CreatedAt) {
		now = inspection.CreatedAt
	}
	intent, err := NewDockerRuntimeInputResourceCleanupIntent("runtime-resource-cleanup",
		strings.Repeat("6", 64), inspection, descriptor, endpoint, true, true,
		inspection.RequestedBy, now)
	if err != nil {
		t.Fatal(err)
	}
	lease := DockerRuntimeInputResourceCleanupLease{IntentID: intent.ID,
		LeaseID: "runtime-resource-cleanup-lease", OwnerID: "runtime_resource_cleanup_owner",
		Generation: 1, Status: DockerRuntimeInputResourceCleanupLeaseActive,
		AcquiredAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := lease.Validate(); err != nil {
		t.Fatal(err)
	}
	return intent, lease
}

func assertNoRuntimeInputExecutionEndpoint(t *testing.T, requests []string) {
	t.Helper()
	for _, request := range requests {
		if strings.Contains(request, "/start") || strings.Contains(request, "/exec") ||
			strings.Contains(request, "/export") || strings.Contains(request, "/attach") ||
			strings.Contains(request, "/networks/") {
			t.Fatalf("resource lifecycle issued forbidden endpoint: %s", request)
		}
	}
}
