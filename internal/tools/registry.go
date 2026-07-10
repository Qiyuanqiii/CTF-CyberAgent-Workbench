package tools

import (
	"context"
	"fmt"
	"sort"
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
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(tool Tool) {
	r.tools[tool.Name()] = tool
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
	tool, ok := r.tools[call.Name]
	if !ok {
		return Result{ExitCode: 127}, fmt.Errorf("tool %q is not registered", call.Name)
	}
	return tool.Run(ctx, call)
}
