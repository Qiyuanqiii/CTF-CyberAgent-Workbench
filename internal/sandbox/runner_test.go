package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestLocalRunnerIsFailClosed(t *testing.T) {
	runner := NewLocalRunner()
	if runner.Available(context.Background()) {
		t.Fatal("local runner unexpectedly reported available")
	}
	result, err := runner.Run(context.Background(), RunRequest{
		Command: "must-not-run", Args: []string{"side-effect"},
	})
	if err == nil || result.ExitCode != 126 || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("local runner did not fail closed: result=%#v err=%v", result, err)
	}
}

func TestNoopRunnerValidatesCancellationAndRedactsDisplay(t *testing.T) {
	runner := NewNoopRunner()
	if _, err := runner.Run(context.Background(), RunRequest{}); err == nil {
		t.Fatal("noop runner accepted an empty command")
	}
	secret := "sk-" + strings.Repeat("x", 32)
	result, err := runner.Run(context.Background(), RunRequest{
		Command: "echo", Args: []string{secret},
	})
	if err != nil || result.ExitCode != 0 || strings.Contains(result.Stdout, secret) ||
		!strings.Contains(result.Stdout, "[REDACTED:api-key]") {
		t.Fatalf("noop runner did not produce a redacted dry run: result=%#v err=%v", result, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = runner.Run(cancelled, RunRequest{Command: "echo"})
	if err == nil || result.ExitCode != 130 {
		t.Fatalf("noop runner ignored cancellation: result=%#v err=%v", result, err)
	}
}

func TestDockerRunnerHonorsPreCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := NewDockerRunnerWithBinary("docker").Run(ctx, RunRequest{Command: "echo"})
	if err == nil || result.ExitCode != 130 {
		t.Fatalf("docker runner ignored cancellation: result=%#v err=%v", result, err)
	}
}
