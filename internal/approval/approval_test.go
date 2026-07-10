package approval

import (
	"testing"
	"time"
)

func TestFingerprintIsDeterministicAndLengthDelimited(t *testing.T) {
	first := Fingerprint("ab", "c")
	if first != Fingerprint("ab", "c") {
		t.Fatal("fingerprint is not deterministic")
	}
	if first == Fingerprint("a", "bc") {
		t.Fatal("length-delimited fingerprint collided")
	}
	if len(first) != 64 {
		t.Fatalf("unexpected fingerprint length: %d", len(first))
	}
	operationKey := OperationKeyDigest("client-review-key")
	if operationKey == "client-review-key" || len(operationKey) != 64 || operationKey != OperationKeyDigest("client-review-key") {
		t.Fatalf("operation key was not safely digested: %q", operationKey)
	}
}

func TestRecordRequiresConsistentDecisionMetadata(t *testing.T) {
	now := time.Now().UTC()
	record := Record{
		ID: "approval-test", IdempotencyKey: "proposal:shell:tool-test", ProposalID: "tool-test",
		ToolName: "shell", ActionClass: "shell", Mode: "per_call", Status: StatusPending,
		RequestFingerprint: Fingerprint("request"), RequestedBy: "test", Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	record.Status = StatusApproved
	if err := record.Validate(); err == nil {
		t.Fatal("expected decided approval without metadata to fail")
	}
	record.ReviewedBy = "operator"
	record.DecidedAt = &now
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestDecisionRequestRejectsIdempotencyKeyReuseShapeChanges(t *testing.T) {
	request := DecisionRequest{
		ProposalID: "tool-test", IdempotencyKey: "review:shell:tool-test:approve",
		Action: ActionApprove, ReviewedBy: "operator",
	}
	normalized, err := request.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if DecisionFingerprint(normalized) == DecisionFingerprint(DecisionRequest{
		ProposalID: "tool-other", IdempotencyKey: normalized.IdempotencyKey,
		Action: ActionApprove, ReviewedBy: "operator",
	}) {
		t.Fatal("decision fingerprint did not bind the proposal")
	}
}
