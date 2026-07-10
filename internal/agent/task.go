package agent

import (
	"time"

	"cyberagent-workbench/internal/idgen"
)

type TaskKind string

const (
	TaskCTF    TaskKind = "ctf"
	TaskScript TaskKind = "script"
	TaskCode   TaskKind = "code"
	TaskLearn  TaskKind = "learn"
	TaskReview TaskKind = "review"
)

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusDenied    = "denied"
	StatusFailed    = "failed"
)

type Task struct {
	ID          string
	Kind        TaskKind
	Goal        string
	WorkspaceID string
	Mode        string
	Status      string
	CreatedAt   time.Time
}

type Event struct {
	ID          int64
	TaskID      string
	WorkspaceID string
	Type        string
	Message     string
	PayloadJSON string
	CreatedAt   time.Time
}

func NewID(prefix string) string {
	return idgen.New(prefix)
}
