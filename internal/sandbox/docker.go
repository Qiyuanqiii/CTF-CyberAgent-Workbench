package sandbox

import (
	"context"
	"fmt"
	"os/exec"
)

type DockerRunner struct {
	binary string
}

func NewDockerRunner() DockerRunner {
	return DockerRunner{binary: "docker"}
}

func NewDockerRunnerWithBinary(binary string) DockerRunner {
	return DockerRunner{binary: binary}
}

func (r DockerRunner) Name() string {
	return "docker"
}

func (r DockerRunner) Available(ctx context.Context) bool {
	_, err := exec.LookPath(r.binary)
	return err == nil
}

func (r DockerRunner) ValidateManifest(ctx context.Context, manifest Manifest) (Manifest, error) {
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	if !r.Available(ctx) {
		return Manifest{}, fmt.Errorf("docker runner unavailable: %q was not found on PATH", r.binary)
	}
	return Manifest{}, fmt.Errorf("docker runner is detected but execution remains disabled")
}

func (r DockerRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if err := ctx.Err(); err != nil {
		return RunResult{ExitCode: 130}, err
	}
	if !r.Available(ctx) {
		return RunResult{ExitCode: 127}, fmt.Errorf("docker runner unavailable: %q was not found on PATH", r.binary)
	}
	return RunResult{ExitCode: 2}, fmt.Errorf("docker runner is detected but not implemented in v0.1")
}
