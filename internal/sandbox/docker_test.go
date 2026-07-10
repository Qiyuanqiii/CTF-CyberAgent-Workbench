package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestDockerRunnerUnavailableError(t *testing.T) {
	runner := NewDockerRunnerWithBinary("definitely-not-installed-docker-cyberagent")
	if runner.Available(context.Background()) {
		t.Fatal("test docker binary should not be available")
	}
	_, err := runner.Run(context.Background(), RunRequest{Command: "echo"})
	if err == nil {
		t.Fatal("expected unavailable error")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}
