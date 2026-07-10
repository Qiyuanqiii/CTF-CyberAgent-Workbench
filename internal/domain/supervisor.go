package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type SupervisorPhase string

const (
	SupervisorIdle         SupervisorPhase = "idle"
	SupervisorTurnStarted  SupervisorPhase = "turn_started"
	SupervisorTurnFailed   SupervisorPhase = "turn_failed"
	SupervisorWaiting      SupervisorPhase = "waiting"
	SupervisorRunCompleted SupervisorPhase = "run_completed"
	SupervisorRunFailed    SupervisorPhase = "run_failed"
)

type SupervisorCheckpoint struct {
	RunID           string
	NextTurn        int
	Phase           SupervisorPhase
	AttemptID       string
	PendingInput    string
	LastError       string
	InputTokens     int64
	OutputTokens    int64
	TotalTokens     int64
	ExecutionMillis int64
	UpdatedAt       time.Time
}

type SupervisorTurn struct {
	Run        Run
	Mission    Mission
	Checkpoint SupervisorCheckpoint
	Recovered  bool
}

func (c SupervisorCheckpoint) Validate() error {
	if strings.TrimSpace(c.RunID) == "" {
		return errors.New("checkpoint run id is required")
	}
	if c.NextTurn <= 0 {
		return errors.New("checkpoint next turn must be positive")
	}
	switch c.Phase {
	case SupervisorIdle, SupervisorWaiting, SupervisorRunCompleted, SupervisorRunFailed:
		if strings.TrimSpace(c.AttemptID) != "" {
			return fmt.Errorf("checkpoint phase %s cannot have an active attempt", c.Phase)
		}
		if strings.TrimSpace(c.PendingInput) != "" {
			return fmt.Errorf("checkpoint phase %s cannot have pending input", c.Phase)
		}
	case SupervisorTurnStarted, SupervisorTurnFailed:
		if strings.TrimSpace(c.AttemptID) == "" {
			return fmt.Errorf("checkpoint phase %s requires an attempt id", c.Phase)
		}
	default:
		return fmt.Errorf("invalid supervisor checkpoint phase %q", c.Phase)
	}
	if c.InputTokens < 0 || c.OutputTokens < 0 || c.TotalTokens < 0 || c.ExecutionMillis < 0 {
		return errors.New("checkpoint usage counters cannot be negative")
	}
	if c.UpdatedAt.IsZero() {
		return errors.New("checkpoint timestamp is required")
	}
	return nil
}
