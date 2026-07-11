package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type SupervisorPhase string

type ProtocolRepairPhase string

const (
	SupervisorIdle         SupervisorPhase = "idle"
	SupervisorTurnStarted  SupervisorPhase = "turn_started"
	SupervisorTurnFailed   SupervisorPhase = "turn_failed"
	SupervisorWaiting      SupervisorPhase = "waiting"
	SupervisorRunCompleted SupervisorPhase = "run_completed"
	SupervisorRunFailed    SupervisorPhase = "run_failed"

	ProtocolRepairNone      ProtocolRepairPhase = ""
	ProtocolRepairPending   ProtocolRepairPhase = "pending"
	ProtocolRepairExhausted ProtocolRepairPhase = "exhausted"
)

type SupervisorCheckpoint struct {
	RunID           string
	LeaseID         string
	LeaseGeneration int64
	NextTurn        int
	Phase           SupervisorPhase
	AttemptID       string
	PendingInput    string
	RepairPhase     ProtocolRepairPhase
	RepairReason    string
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
	if (strings.TrimSpace(c.LeaseID) == "") != (c.LeaseGeneration == 0) || c.LeaseGeneration < 0 ||
		!utf8.ValidString(c.LeaseID) || len([]rune(c.LeaseID)) > MaxRunLeaseIdentityRunes {
		return errors.New("checkpoint execution lease identity and generation are inconsistent")
	}
	switch c.Phase {
	case SupervisorIdle, SupervisorWaiting, SupervisorRunCompleted, SupervisorRunFailed:
		if strings.TrimSpace(c.AttemptID) != "" {
			return fmt.Errorf("checkpoint phase %s cannot have an active attempt", c.Phase)
		}
		if strings.TrimSpace(c.PendingInput) != "" {
			return fmt.Errorf("checkpoint phase %s cannot have pending input", c.Phase)
		}
		if c.RepairPhase != ProtocolRepairNone || strings.TrimSpace(c.RepairReason) != "" {
			return fmt.Errorf("checkpoint phase %s cannot have protocol repair state", c.Phase)
		}
	case SupervisorTurnStarted, SupervisorTurnFailed:
		if strings.TrimSpace(c.AttemptID) == "" {
			return fmt.Errorf("checkpoint phase %s requires an attempt id", c.Phase)
		}
		if c.Phase == SupervisorTurnFailed && (c.RepairPhase != ProtocolRepairNone || strings.TrimSpace(c.RepairReason) != "") {
			return errors.New("failed supervisor turn cannot have protocol repair state")
		}
		switch c.RepairPhase {
		case ProtocolRepairNone:
			if strings.TrimSpace(c.RepairReason) != "" {
				return errors.New("protocol repair reason requires an active repair phase")
			}
		case ProtocolRepairPending, ProtocolRepairExhausted:
			if strings.TrimSpace(c.RepairReason) == "" {
				return errors.New("protocol repair phase requires a reason")
			}
		default:
			return fmt.Errorf("invalid protocol repair phase %q", c.RepairPhase)
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
