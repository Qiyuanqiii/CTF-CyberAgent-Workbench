package domain

import (
	"strings"
	"testing"
	"time"
)

func TestAgentAttemptLifecycleValidation(t *testing.T) {
	now := time.Now().UTC()
	attempt := AgentAttempt{
		ID: "attempt-1", RunID: "run-1", AgentID: "child-1", ParentAgentID: "root-1",
		LeaseID: "lease-1", LeaseGeneration: 2, Turn: 1, Status: AgentAttemptRunning,
		StartedAt: now, UpdatedAt: now,
	}
	if err := attempt.Validate(); err != nil {
		t.Fatalf("valid running attempt was rejected: %v", err)
	}
	usageAt := now.Add(time.Second)
	finishedAt := usageAt
	attempt.Status = AgentAttemptContinued
	attempt.Usage = AgentAttemptUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5, ExecutionMillis: 1000}
	attempt.UsageRecordedAt = &usageAt
	attempt.UpdatedAt = finishedAt
	attempt.FinishedAt = &finishedAt
	if err := attempt.Validate(); err != nil {
		t.Fatalf("valid continued attempt was rejected: %v", err)
	}
	attempt.Status = AgentAttemptFinished
	attempt.NotificationMessageID = "message-1"
	if err := attempt.Validate(); err != nil {
		t.Fatalf("valid finished attempt was rejected: %v", err)
	}
	attempt.Status = AgentAttemptCrashed
	attempt.Usage = AgentAttemptUsage{}
	attempt.UsageRecordedAt = nil
	attempt.Failure = AgentAttemptFailure{Code: "provider_crash", Reason: "worker exited"}
	if err := attempt.Validate(); err != nil {
		t.Fatalf("valid crashed attempt was rejected: %v", err)
	}
	attempt.Status = AgentAttemptInterrupted
	attempt.NotificationMessageID = ""
	if err := attempt.Validate(); err != nil {
		t.Fatalf("valid interrupted attempt was rejected: %v", err)
	}
}

func TestAgentAttemptRejectsInvalidUsageAndFailure(t *testing.T) {
	if err := (AgentAttemptUsage{InputTokens: 3, TotalTokens: 2}).Validate(); err == nil {
		t.Fatal("usage accepted a total smaller than input tokens")
	}
	if _, err := NormalizeAgentAttemptFailure(AgentAttemptFailure{
		Code: "Bad Code", Reason: "failed",
	}); err == nil {
		t.Fatal("failure accepted an unsupported code")
	}
	if _, err := NormalizeAgentAttemptFailure(AgentAttemptFailure{
		Code: "worker_lost", Reason: strings.Repeat("x", MaxAgentFailureReasonBytes+1),
	}); err == nil {
		t.Fatal("failure accepted an oversized reason")
	}
}
