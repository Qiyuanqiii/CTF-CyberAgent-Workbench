package domain

import (
	"testing"
	"time"
)

func TestSupervisorCheckpointValidation(t *testing.T) {
	now := time.Now().UTC()
	valid := []SupervisorCheckpoint{
		{RunID: "run-1", NextTurn: 1, Phase: SupervisorIdle, UpdatedAt: now},
		{RunID: "run-1", NextTurn: 2, Phase: SupervisorTurnStarted, AttemptID: "attempt-1", PendingInput: "request", UpdatedAt: now},
		{RunID: "run-1", NextTurn: 2, Phase: SupervisorTurnStarted, AttemptID: "attempt-1", PendingInput: "request", RepairPhase: ProtocolRepairPending, RepairReason: "invalid JSON", UpdatedAt: now},
		{RunID: "run-1", NextTurn: 2, Phase: SupervisorTurnStarted, AttemptID: "attempt-1", PendingInput: "request", RepairPhase: ProtocolRepairExhausted, RepairReason: "repair remained invalid", UpdatedAt: now},
		{RunID: "run-1", NextTurn: 2, Phase: SupervisorTurnFailed, AttemptID: "attempt-1", PendingInput: "request", LastError: "failed", UpdatedAt: now},
		{RunID: "run-1", NextTurn: 2, Phase: SupervisorWaiting, UpdatedAt: now},
		{RunID: "run-1", NextTurn: 2, Phase: SupervisorRunCompleted, TotalTokens: 10, ExecutionMillis: 5, UpdatedAt: now},
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
	invalid = valid[0]
	invalid.TotalTokens = -1
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected negative usage to fail")
	}
	invalid = valid[0]
	invalid.PendingInput = "unexpected"
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected idle pending input to fail")
	}
	invalid = valid[1]
	invalid.RepairPhase = ProtocolRepairPending
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected repair phase without reason to fail")
	}
	invalid = valid[2]
	invalid.Phase = SupervisorTurnFailed
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected failed turn with repair state to fail")
	}
}
