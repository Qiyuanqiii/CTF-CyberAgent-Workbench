package sandbox

import (
	"context"
	"time"
)

type RunRequest struct {
	Command    string
	Args       []string
	WorkingDir string
	Env        map[string]string
	Timeout    time.Duration
}

type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type Runner interface {
	Name() string
	Available(ctx context.Context) bool
	Run(ctx context.Context, req RunRequest) (RunResult, error)
}
