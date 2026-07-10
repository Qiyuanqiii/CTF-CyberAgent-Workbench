package agent

import "cyberagent-workbench/internal/tools"

type Plan struct {
	Summary   string
	ToolCalls []tools.Call
}

type Planner struct{}

func NewPlanner() Planner {
	return Planner{}
}
