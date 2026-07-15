//go:build windows

package sandbox

import (
	"context"
	"testing"
)

func TestLocalDockerRuntimeInputResourceTransportsAreUnsupportedAndNarrowOnWindows(t *testing.T) {
	application, descriptor, _, _ := newDockerRuntimeInputResourceFixture(t)
	inspector := NewLocalDockerRuntimeInputResourceInspector()
	cleanup := NewLocalDockerRuntimeInputResourceCleanupTransport()
	if _, ok := inspector.(DockerRuntimeInputResourceCleanupTransport); ok {
		t.Fatal("resource inspector widened into cleanup authority")
	}
	if _, ok := cleanup.(DockerRuntimeInputResourceInspector); ok {
		t.Fatal("resource cleanup widened into inspection authority")
	}
	if _, ok := inspector.(DockerRuntimeInputApplicationTransport); ok {
		t.Fatal("resource inspector widened into input application authority")
	}
	if _, ok := cleanup.(DockerRuntimeInputApplicationTransport); ok {
		t.Fatal("resource cleanup widened into input application authority")
	}
	_, err := inspector.Inspect(context.Background(), descriptor)
	if DockerRuntimeInputResourceErrorCode(err) != DockerRuntimeInputResourceErrorUnsupported {
		t.Fatalf("local Windows resource inspector error = %v", err)
	}
	_ = application
}
