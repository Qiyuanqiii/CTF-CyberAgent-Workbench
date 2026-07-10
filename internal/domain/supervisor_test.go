package domain

import (
	"testing"
	"time"
)

func TestSupervisorCheckpointValidation(t *testing.T) {
	now := time.Now().UTC()
	valid := []SupervisorCheckpoint{
		{RunID: "run-1", NextTurn: 1, Phase: SupervisorIdle, UpdatedAt: now},
		{RunID: "run-1", NextTurn: 2, Phase: SupervisorTurnStarted, AttemptID: "attempt-1", UpdatedAt: now},
		{RunID: "run-1", NextTurn: 2, Phase: SupervisorTurnFailed, AttemptID: "attempt-1", LastError: "failed", UpdatedAt: now},
	}
	for _, checkpoint := range valid {
		if err := checkpoint.Validate(); err != nil {
			t.Fatalf("valid checkpoint rejected: %v", err)
		}
	}
	invalid := valid[0]
	invalid.Phase = SupervisorTurnStarted
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected started checkpoint without attempt to fail")
	}
}
