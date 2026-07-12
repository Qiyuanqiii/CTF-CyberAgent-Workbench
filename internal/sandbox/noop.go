package sandbox

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/redact"
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
	if err := ctx.Err(); err != nil {
		return RunResult{ExitCode: 130}, err
	}
	if strings.TrimSpace(req.Command) == "" {
		return RunResult{ExitCode: 127}, errors.New("command is required")
	}
	cmd := strings.Join(append([]string{req.Command}, req.Args...), " ")
	return RunResult{Stdout: fmt.Sprintf("dry run: %s", redact.String(cmd)), ExitCode: 0}, nil
}
