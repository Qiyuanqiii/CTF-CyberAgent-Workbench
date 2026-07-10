package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type SupervisorPhase string

const (
	SupervisorIdle        SupervisorPhase = "idle"
	SupervisorTurnStarted SupervisorPhase = "turn_started"
	SupervisorTurnFailed  SupervisorPhase = "turn_failed"
)

type SupervisorCheckpoint struct {
	RunID     string
	NextTurn  int
	Phase     SupervisorPhase
	AttemptID string
	LastError string
	UpdatedAt time.Time
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
	case SupervisorIdle:
		if strings.TrimSpace(c.AttemptID) != "" {
			return errors.New("idle checkpoint cannot have an active attempt")
		}
	case SupervisorTurnStarted, SupervisorTurnFailed:
		if strings.TrimSpace(c.AttemptID) == "" {
			return fmt.Errorf("checkpoint phase %s requires an attempt id", c.Phase)
		}
	default:
		return fmt.Errorf("invalid supervisor checkpoint phase %q", c.Phase)
	}
	if c.UpdatedAt.IsZero() {
		return errors.New("checkpoint timestamp is required")
	}
	return nil
}
