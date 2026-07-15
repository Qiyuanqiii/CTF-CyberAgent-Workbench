//go:build !windows

package sandbox

import "testing"

func TestLocalDockerRuntimeInputResourceTransportsAreCapabilityNarrow(t *testing.T) {
	inspector := NewLocalDockerRuntimeInputResourceInspector()
	cleanup := NewLocalDockerRuntimeInputResourceCleanupTransport()
	if _, ok := inspector.(DockerRuntimeInputResourceCleanupTransport); ok {
		t.Fatal("resource inspector widened into cleanup authority")
	}
	if _, ok := cleanup.(DockerRuntimeInputResourceInspector); ok {
		t.Fatal("resource cleanup widened into inspection authority")
	}
	for name, value := range map[string]any{"inspector": inspector, "cleanup": cleanup} {
		if _, ok := value.(DockerRuntimeInputApplicationTransport); ok {
			t.Fatalf("%s widened into runtime-input application authority", name)
		}
		if _, ok := value.(DockerContainerWriteTransport); ok {
			t.Fatalf("%s widened into container-write authority", name)
		}
	}
}
