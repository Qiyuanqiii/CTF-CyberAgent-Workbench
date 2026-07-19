package tools

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	DefaultExecutionTimeout = 15 * time.Second
	MaxExecutionTimeout     = 5 * time.Minute
)

type Schema struct {
	Description string
	Parameters  map[string]string
}

type Call struct {
	Name       string
	Args       map[string]string
	WorkingDir string
}

type Result struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	MIME      string
	Truncated bool
}

type Tool interface {
	Name() string
	Schema() Schema
	Run(ctx context.Context, call Call) (Result, error)
}

type Registry struct {
	tools            map[string]Tool
	executionTimeout time.Duration
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}, executionTimeout: DefaultExecutionTimeout}
}

func (r *Registry) Register(tool Tool) {
	if r == nil || tool == nil {
		return
	}
	r.tools[tool.Name()] = tool
}

func (r *Registry) WithExecutionTimeout(timeout time.Duration) *Registry {
	if r != nil {
		r.executionTimeout = timeout
	}
	return r
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (r *Registry) Run(ctx context.Context, call Call) (Result, error) {
	if r == nil {
		return Result{ExitCode: 1}, errors.New("tool registry is required")
	}
	if ctx == nil {
		return Result{ExitCode: 1}, errors.New("tool context is required")
	}
	if r.executionTimeout <= 0 || r.executionTimeout > MaxExecutionTimeout {
		return Result{ExitCode: 1}, fmt.Errorf("tool execution timeout must be between 1ns and %s", MaxExecutionTimeout)
	}
	tool, ok := r.tools[call.Name]
	if !ok {
		return Result{ExitCode: 127}, fmt.Errorf("tool %q is not registered", call.Name)
	}
	if err := ctx.Err(); err != nil {
		return interruptedResult(err), err
	}

	executionCtx, cancel := context.WithTimeout(ctx, r.executionTimeout)
	defer cancel()
	type executionResult struct {
		result Result
		err    error
	}
	done := make(chan executionResult, 1)
	go func() {
		var outcome executionResult
		defer func() {
			if recovered := recover(); recovered != nil {
				outcome.result = Result{ExitCode: 1, Stderr: "tool execution panicked"}
				outcome.err = fmt.Errorf("tool %q panicked: %v", call.Name, recovered)
			}
			done <- outcome
		}()
		outcome.result, outcome.err = tool.Run(executionCtx, call)
	}()

	select {
	case outcome := <-done:
		if err := executionCtx.Err(); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = ctxErr
			}
			return interruptedResult(err), fmt.Errorf("tool %q interrupted: %w", call.Name, err)
		}
		return outcome.result, outcome.err
	case <-executionCtx.Done():
		err := executionCtx.Err()
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return interruptedResult(err), fmt.Errorf("tool %q interrupted: %w", call.Name, err)
	}
}

func interruptedResult(err error) Result {
	exitCode := 130
	reason := "tool execution cancelled"
	if errors.Is(err, context.DeadlineExceeded) {
		exitCode = 124
		reason = "tool execution timed out"
	}
	return Result{ExitCode: exitCode, Stderr: strings.TrimSpace(reason), MIME: "text/plain; charset=utf-8"}
}
