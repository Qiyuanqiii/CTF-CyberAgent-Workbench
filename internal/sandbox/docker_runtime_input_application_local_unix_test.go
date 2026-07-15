//go:build !windows

package sandbox

import "testing"

func TestLocalDockerRuntimeInputApplicationTransportIsCapabilityNarrow(t *testing.T) {
	runtimeInput := NewLocalDockerRuntimeInputApplicationTransport()
	if _, ok := runtimeInput.(DockerContainerWriteTransport); ok {
		t.Fatal("runtime-input transport widened into the container rehearsal transport")
	}
	if _, ok := runtimeInput.(DockerHostInputHandoffTransport); ok {
		t.Fatal("runtime-input transport widened into the host-input handoff transport")
	}
	containerWrite := NewLocalDockerContainerWriteTransport()
	if _, ok := containerWrite.(DockerHostInputHandoffTransport); ok {
		t.Fatal("container rehearsal transport widened into the host-input handoff transport")
	}
	if _, ok := containerWrite.(DockerRuntimeInputApplicationTransport); ok {
		t.Fatal("container rehearsal transport widened into the runtime-input transport")
	}
	handoff := NewLocalDockerHostInputHandoffTransport()
	if _, ok := handoff.(DockerContainerWriteTransport); ok {
		t.Fatal("host-input handoff transport widened into the container rehearsal transport")
	}
	if _, ok := handoff.(DockerRuntimeInputApplicationTransport); ok {
		t.Fatal("host-input handoff transport widened into the runtime-input transport")
	}
}
