package sandbox

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

type LocalRunner struct{}

func NewLocalRunner() LocalRunner {
	return LocalRunner{}
}

func (LocalRunner) Name() string {
	return "local"
}

func (LocalRunner) Available(ctx context.Context) bool {
	return true
}

func (LocalRunner) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	if req.Command == "" {
		return RunResult{ExitCode: 127}, errors.New("command is required")
	}
	timeout := req.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, req.Command, req.Args...)
	cmd.Dir = req.WorkingDir
	for k, v := range req.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		code = 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		}
	}
	return RunResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: code}, err
}
