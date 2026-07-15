//go:build windows

package sandbox

import (
	"context"
	"testing"
)

func TestLocalDockerRuntimeInputApplicationTransportIsUnsupportedOnWindows(t *testing.T) {
	intent, lease, request := newDockerRuntimeInputApplicationTransportFixture(t)
	transport := NewLocalDockerRuntimeInputApplicationTransport()
	if _, ok := transport.(DockerContainerWriteTransport); ok {
		t.Fatal("Windows runtime-input transport widened into the container rehearsal transport")
	}
	if _, ok := transport.(DockerHostInputHandoffTransport); ok {
		t.Fatal("Windows runtime-input transport widened into the host-input handoff transport")
	}
	containerWrite := NewLocalDockerContainerWriteTransport()
	if _, ok := containerWrite.(DockerHostInputHandoffTransport); ok {
		t.Fatal("Windows container rehearsal transport widened into the handoff transport")
	}
	if _, ok := containerWrite.(DockerRuntimeInputApplicationTransport); ok {
		t.Fatal("Windows container rehearsal transport widened into the runtime-input transport")
	}
	handoff := NewLocalDockerHostInputHandoffTransport()
	if _, ok := handoff.(DockerContainerWriteTransport); ok {
		t.Fatal("Windows handoff transport widened into the container rehearsal transport")
	}
	if _, ok := handoff.(DockerRuntimeInputApplicationTransport); ok {
		t.Fatal("Windows handoff transport widened into the runtime-input transport")
	}
	_, err := transport.Apply(
		context.Background(), intent, lease, request)
	if DockerRuntimeInputApplicationErrorCode(err) != DockerRuntimeInputApplicationErrorUnsupported {
		t.Fatalf("local Windows transport error = %v", err)
	}
}
