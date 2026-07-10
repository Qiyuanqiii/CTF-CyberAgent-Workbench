package sandbox

import (
	"context"
	"fmt"
	"strings"
)

type NoopRunner struct{}

func NewNoopRunner() NoopRunner {
	return NoopRunner{}
}

func (NoopRunner) Name() string {
	return "noop"
}

func (NoopRunner) Available(ctx context.Context) bool {
	return true
}

func (NoopRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	cmd := strings.Join(append([]string{req.Command}, req.Args...), " ")
	return RunResult{Stdout: fmt.Sprintf("dry run: %s", cmd), ExitCode: 0}, nil
}
