package sandbox

import (
	"context"
	"errors"
)

type LocalRunner struct{}

func NewLocalRunner() LocalRunner {
	return LocalRunner{}
}

func (LocalRunner) Name() string {
	return "local"
}

func (LocalRunner) Available(ctx context.Context) bool {
	return false
}

func (LocalRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if err := ctx.Err(); err != nil {
		return RunResult{ExitCode: 130}, err
	}
	return RunResult{ExitCode: 126}, errors.New("local runner is disabled; no host process was started")
}
