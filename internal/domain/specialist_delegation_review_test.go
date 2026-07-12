package domain

import (
	"strings"
	"testing"
	"time"
)

func TestSpecialistDelegationReviewValidation(t *testing.T) {
	now := time.Now().UTC()
	valid := SpecialistDelegationReview{
		ID: "delegation-review-1", ProposalID: "delegation-1", RunID: "run-1",
		RootAgentID: "agent-1", Decision: SpecialistDelegationApproved,
		ReviewedBy: "cli_operator", Version: 1, CreatedAt: now,
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	rejected := valid
	rejected.Decision = SpecialistDelegationRejected
	if err := rejected.Validate(); err == nil {
		t.Fatal("rejected review without a reason was accepted")
	}
	rejected.Reason = "outside the authorized scope"
	if err := rejected.Validate(); err != nil {
		t.Fatal(err)
	}
	tooLong := valid
	tooLong.Reason = strings.Repeat("x", MaxSpecialistDelegationReviewReasonRunes+1)
	if err := tooLong.Validate(); err == nil {
		t.Fatal("oversized review reason was accepted")
	}
	invalidDecision := valid
	invalidDecision.Decision = "pending"
	if err := invalidDecision.Validate(); err == nil {
		t.Fatal("invalid review decision was accepted")
	}
}
