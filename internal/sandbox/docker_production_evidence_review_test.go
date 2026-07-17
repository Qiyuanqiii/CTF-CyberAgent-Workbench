package sandbox

import (
	"strings"
	"testing"
	"time"
)

func TestDockerProductionEvidenceReviewAcceptsOrRejectsReceiptWithoutAuthority(t *testing.T) {
	evidence, attempt := testCompletedDockerProductionEvidenceHarnessRecord(t)
	tests := []struct {
		decision string
		reason   string
		accepted bool
	}{
		{DockerProductionEvidenceReviewDecisionAccepted,
			DockerProductionEvidenceReviewReasonMetadataScopeAccepted, true},
		{DockerProductionEvidenceReviewDecisionRejected,
			DockerProductionEvidenceReviewReasonIntegrityConcern, false},
		{DockerProductionEvidenceReviewDecisionRejected,
			DockerProductionEvidenceReviewReasonEnvironmentConcern, false},
		{DockerProductionEvidenceReviewDecisionRejected,
			DockerProductionEvidenceReviewReasonScopeConcern, false},
		{DockerProductionEvidenceReviewDecisionRejected,
			DockerProductionEvidenceReviewReasonInsufficientEvidence, false},
		{DockerProductionEvidenceReviewDecisionRejected,
			DockerProductionEvidenceReviewReasonOperatorRejected, false},
	}
	for index, test := range tests {
		t.Run(test.reason, func(t *testing.T) {
			value, err := NewDockerProductionEvidenceReview(
				"evidence-review-"+string(rune('a'+index)), strings.Repeat("9", 64),
				"reviewer", test.decision, test.reason, evidence, attempt, true,
				evidence.CreatedAt.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			if value.ReceiptAccepted != test.accepted || value.StartGatePassed ||
				value.ProductionVerifiedCount != 0 || value.SufficientCheckCount != 0 ||
				value.BlockerCount != MaxBackendChecks || value.ContainerStartAuthorized ||
				value.ProcessExecutionAuthorized || value.OutputExportAuthorized ||
				value.ArtifactCommitAuthorized {
				t.Fatalf("review widened authority: %#v", value)
			}
			operation, err := NewDockerProductionEvidenceReviewOperation(
				value.OperationKeyDigest, value)
			if err != nil || operation.RequestFingerprint !=
				DockerProductionEvidenceReviewRequestFingerprint(value) {
				t.Fatalf("review operation invalid: %#v err=%v", operation, err)
			}
		})
	}
}

func TestDockerProductionEvidenceReviewRejectsInvalidDecisionLegacyReceiptAndTampering(t *testing.T) {
	evidence, attempt := testCompletedDockerProductionEvidenceHarnessRecord(t)
	newReview := func(decision, reason string, record DockerProductionEvidenceAttemptRecord) (
		DockerProductionEvidenceReview, error,
	) {
		return NewDockerProductionEvidenceReview("evidence-review", strings.Repeat("9", 64),
			"reviewer", decision, reason, evidence, record, true,
			evidence.CreatedAt.Add(time.Second))
	}
	if _, err := newReview(DockerProductionEvidenceReviewDecisionAccepted,
		DockerProductionEvidenceReviewReasonOperatorRejected, attempt); err == nil {
		t.Fatal("accepted receipt used a rejection reason")
	}
	if _, err := newReview(DockerProductionEvidenceReviewDecisionRejected,
		DockerProductionEvidenceReviewReasonMetadataScopeAccepted, attempt); err == nil {
		t.Fatal("rejected receipt used an acceptance reason")
	}
	inFlight := attempt
	inFlight.HarnessResult = nil
	inFlight.Lease.Status = DockerProductionEvidenceAttemptLeaseActive
	inFlight.Lease.ReleasedAt = nil
	if _, err := newReview(DockerProductionEvidenceReviewDecisionRejected,
		DockerProductionEvidenceReviewReasonInsufficientEvidence, inFlight); err == nil {
		t.Fatal("in-flight pre-v67 receipt was reviewable")
	}
	value, err := newReview(DockerProductionEvidenceReviewDecisionAccepted,
		DockerProductionEvidenceReviewReasonMetadataScopeAccepted, attempt)
	if err != nil {
		t.Fatal(err)
	}
	tampered := value
	tampered.StartGatePassed = true
	if err := tampered.Validate(); err == nil {
		t.Fatal("evidence review authorized start")
	}
	tampered = value
	tampered.ProductionVerifiedCount = 1
	if err := tampered.Validate(); err == nil {
		t.Fatal("evidence review fabricated production verification")
	}
	tampered = value
	tampered.ReasonCode = DockerProductionEvidenceReviewReasonOperatorRejected
	if err := tampered.Validate(); err == nil {
		t.Fatal("evidence review decision/reason binding was mutable")
	}
	tampered = value
	tampered.RequestFingerprint = strings.Repeat("a", 64)
	if err := tampered.Validate(); err == nil {
		t.Fatal("evidence review request fingerprint was mutable")
	}
}

func testCompletedDockerProductionEvidenceHarnessRecord(t *testing.T) (
	DockerProductionEvidence, DockerProductionEvidenceAttemptRecord,
) {
	t.Helper()
	review := testDockerStartGateReview(t)
	endpoint, err := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	if err != nil {
		t.Fatal(err)
	}
	now := review.CreatedAt.Add(time.Millisecond)
	attempt, err := NewDockerProductionEvidenceAttempt("production-attempt-v67",
		strings.Repeat("6", 64), review.RequestedBy, review, endpoint, true,
		DefaultDockerProductionEvidenceCaptureTimeout, now)
	if err != nil {
		t.Fatal(err)
	}
	lease := DockerProductionEvidenceAttemptLease{
		AttemptID: attempt.ID, LeaseID: "lease-v67", OwnerID: "worker-v67",
		Generation: 1, Status: DockerProductionEvidenceAttemptLeaseActive,
		AcquiredAt: now, ExpiresAt: now.Add(DefaultDockerProductionEvidenceAttemptLeaseTTL),
	}
	control, err := NewDockerProductionEvidenceReconciliation(attempt, lease,
		now.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	intent := DockerProductionEvidenceHarnessIntent{
		AttemptID: attempt.ID, ReviewID: review.ID,
		ContainerPlanID: review.ContainerPlanID, RunID: attempt.RunID,
		ProtocolVersion: DockerProductionEvidenceHarnessIntentProtocolVersion,
		ImageDigest:     "sha256:" + strings.Repeat("d", 64),
		EndpointClass:   attempt.EndpointClass, EndpointFingerprint: attempt.EndpointFingerprint,
		LabelSelectorFingerprint: fingerprint(
			"sandbox_docker_production_evidence_harness_label.v1",
			DockerProductionEvidenceHarnessLabelKey, attempt.ID),
		MaxDaemonReads:    DockerProductionEvidenceHarnessMaxDaemonReads,
		OperatorConfirmed: true, ReadOnlyDaemonContactAuthorized: true,
		RequestedBy: attempt.RequestedBy, CreatedAt: now.Add(2 * time.Millisecond),
	}
	intent.IntentFingerprint = dockerProductionEvidenceHarnessIntentFingerprint(intent)
	if err := intent.Validate(); err != nil {
		t.Fatal(err)
	}
	inventory, err := NewDockerProductionEvidenceHarnessInventory(endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	harnessReconciliation, err := NewDockerProductionEvidenceHarnessReconciliation(
		intent, lease, control, inventory, now.Add(3*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := NewDockerProductionEvidenceHarnessObservation(
		review.AuthorityFingerprint, strings.Repeat("5", 64))
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := NewDockerProductionEvidence("production-evidence-v67",
		attempt.OperationKeyDigest, review.RequestedBy, review, observation, true,
		now.Add(4*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewDockerProductionEvidenceHarnessResult(intent, lease,
		harnessReconciliation, evidence)
	if err != nil {
		t.Fatal(err)
	}
	releasedAt := result.CreatedAt
	lease.Status, lease.ReleasedAt = DockerProductionEvidenceAttemptLeaseReleased, &releasedAt
	record := DockerProductionEvidenceAttemptRecord{
		Attempt: attempt, Lease: lease,
		Reconciliations: []DockerProductionEvidenceReconciliation{control},
		HarnessIntent:   &intent,
		HarnessReconciliations: []DockerProductionEvidenceHarnessReconciliation{
			harnessReconciliation,
		},
		HarnessResult: &result,
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
	return evidence, record
}
