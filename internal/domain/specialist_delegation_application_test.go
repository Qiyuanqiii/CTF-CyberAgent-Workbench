package domain

import (
	"strings"
	"testing"
	"time"
)

func TestSpecialistDelegationApplicationValidation(t *testing.T) {
	checks := []SpecialistDelegationPolicyCheck{{
		Ordinal: 1, Allowed: true, Reason: "allowed by test policy",
	}}
	fingerprint, err := SpecialistDelegationPolicyFingerprint(checks)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	application := SpecialistDelegationApplication{
		ID: "delegation-application-1", ReviewID: "delegation-review-1",
		ProposalID: "delegation-1", RunID: "run-1", RootAgentID: "agent-root",
		Status: SpecialistDelegationApplying, AssignmentCount: 1,
		PolicyFingerprint: fingerprint, MaxChildren: 2,
		MaxTurnsPerChild: 4, MaxTokensPerChild: 1024,
		RequestedBy: "cli_operator", Version: 1, CreatedAt: now, UpdatedAt: now,
		Assignments: []SpecialistDelegationApplicationAssignment{{
			ApplicationID: "delegation-application-1", ProposalID: "delegation-1",
			Ordinal: 1, Status: SpecialistDelegationAssignmentPending,
			AdmissionOperationDigest:   strings.Repeat("a", 64),
			InstructionOperationDigest: strings.Repeat("b", 64),
			Version:                    1, CreatedAt: now, UpdatedAt: now,
		}},
	}
	if err := application.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := application
	invalid.Status = SpecialistDelegationApplied
	invalid.Version = 2
	invalid.CompletedAt = &now
	if err := invalid.Validate(); err == nil {
		t.Fatal("incomplete applied delegation was accepted")
	}
	if _, err := SpecialistDelegationAdmissionOperationKey(application.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := SpecialistDelegationInstructionOperationKey(application.ID, 3); err == nil {
		t.Fatal("out-of-range assignment operation key was accepted")
	}
}

func TestSpecialistDelegationPolicyFingerprintRequiresOrderedChecks(t *testing.T) {
	checks := []SpecialistDelegationPolicyCheck{
		{Ordinal: 2, Allowed: true, Reason: "second"},
		{Ordinal: 1, Allowed: true, Reason: "first"},
	}
	if _, err := SpecialistDelegationPolicyFingerprint(checks); err == nil {
		t.Fatal("out-of-order policy checks were accepted")
	}
}
