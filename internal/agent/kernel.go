package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
)

type Store interface {
	GetTask(ctx context.Context, id string) (Task, error)
	UpdateTaskStatus(ctx context.Context, id string, status string) error
	RecordEvent(ctx context.Context, event Event) error
}

type Kernel struct {
	store   Store
	router  *llm.Router
	checker policy.Checker
}

func NewKernel(store Store, router *llm.Router, checker policy.Checker) *Kernel {
	return &Kernel{store: store, router: router, checker: checker}
}

func (k *Kernel) Step(ctx context.Context, taskID string) error {
	task, err := k.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	if err := k.store.UpdateTaskStatus(ctx, task.ID, StatusRunning); err != nil {
		return err
	}
	k.record(ctx, task, "task.running", "task moved to running", nil)

	req := llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "You are CyberAgent Workbench mock planner. Return safe, concise next steps."},
			{Role: "user", Content: fmt.Sprintf("kind=%s mode=%s goal=%s", task.Kind, task.Mode, task.Goal)},
		},
		Metadata: map[string]string{
			"task_id":      task.ID,
			"workspace_id": task.WorkspaceID,
		},
	}

	resp, err := k.router.Chat(ctx, string(task.Kind), req)
	if err != nil {
		_ = k.store.UpdateTaskStatus(ctx, task.ID, StatusFailed)
		k.record(ctx, task, "model.error", err.Error(), nil)
		return err
	}

	decision := k.checker.CheckText("model_response", resp.Text)
	k.record(ctx, task, "policy.decision", decision.Reason, decision)
	if !decision.Allowed {
		_ = k.store.UpdateTaskStatus(ctx, task.ID, StatusDenied)
		return fmt.Errorf("policy denied model response: %s", decision.Reason)
	}

	k.record(ctx, task, "model.response", resp.Text, map[string]any{
		"model":      resp.Model,
		"provider":   resp.Provider,
		"usage":      resp.Usage,
		"tool_calls": resp.ToolCalls,
	})

	if err := k.store.UpdateTaskStatus(ctx, task.ID, StatusCompleted); err != nil {
		return err
	}
	k.record(ctx, task, "task.completed", "task completed by mock agent step", nil)
	return nil
}

func (k *Kernel) record(ctx context.Context, task Task, typ string, message string, payload any) {
	var payloadJSON string
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			payloadJSON = string(b)
		}
	}
	_ = k.store.RecordEvent(ctx, Event{
		TaskID:      task.ID,
		WorkspaceID: task.WorkspaceID,
		Type:        typ,
		Message:     message,
		PayloadJSON: payloadJSON,
		CreatedAt:   time.Now().UTC(),
	})
}
