package sandbox

import (
	"errors"
	"strconv"
	"time"
)

const (
	DockerProductionEvidenceReviewProtocolVersion  = "sandbox_docker_production_evidence_review.v1"
	DockerProductionEvidenceReviewOperationVersion = "sandbox_docker_production_evidence_review_operation.v1"

	DockerProductionEvidenceReviewDecisionAccepted = "accepted"
	DockerProductionEvidenceReviewDecisionRejected = "rejected"

	DockerProductionEvidenceReviewReasonMetadataScopeAccepted = "metadata_scope_accepted"
	DockerProductionEvidenceReviewReasonIntegrityConcern      = "integrity_concern"
	DockerProductionEvidenceReviewReasonEnvironmentConcern    = "environment_concern"
	DockerProductionEvidenceReviewReasonScopeConcern          = "scope_concern"
	DockerProductionEvidenceReviewReasonInsufficientEvidence  = "insufficient_evidence"
	DockerProductionEvidenceReviewReasonOperatorRejected      = "operator_rejected"

	DockerProductionEvidenceReviewTrustClass = "operator_receipt_review_non_authorizing"
	MaxDockerProductionEvidenceReviewsPerRun = MaxDockerProductionEvidencePerRun
)

type DockerProductionEvidenceReview struct {
	ID                         string
	EvidenceID                 string
	AttemptID                  string
	StartGateReviewID          string
	RunID                      string
	MissionID                  string
	WorkspaceID                string
	ProtocolVersion            string
	OperationKeyDigest         string
	RequestFingerprint         string
	EvidenceOperationKeyDigest string
	EvidenceCaptureFingerprint string
	HarnessResultFingerprint   string
	AuthorityFingerprint       string
	ThreatModelFingerprint     string
	SuiteFingerprint           string
	EnvironmentFingerprint     string
	Decision                   string
	ReasonCode                 string
	TrustClass                 string
	OperatorConfirmed          bool
	ReceiptAccepted            bool
	RealDaemonContacted        bool
	RequiredCheckCount         int
	ObservedCount              int
	ProductionVerifiedCount    int
	SufficientCheckCount       int
	BlockerCount               int
	StartGatePassed            bool
	ContainerStartAuthorized   bool
	ProcessExecutionAuthorized bool
	OutputExportAuthorized     bool
	ArtifactCommitAuthorized   bool
	ReviewFingerprint          string
	ReviewedBy                 string
	CreatedAt                  time.Time
	Replayed                   bool
}

func NewDockerProductionEvidenceReview(id, keyDigest, reviewedBy, decision,
	reasonCode string, evidence DockerProductionEvidence,
	attempt DockerProductionEvidenceAttemptRecord, operatorConfirmed bool,
	createdAt time.Time,
) (DockerProductionEvidenceReview, error) {
	if validateStoredIdentity("Docker production evidence review id", id) != nil ||
		validateStoredIdentity("Docker production evidence reviewer", reviewedBy) != nil ||
		!validDigest(keyDigest) || !operatorConfirmed || createdAt.IsZero() ||
		evidence.Validate() != nil || attempt.Validate() != nil ||
		attempt.Result != nil || attempt.HarnessResult == nil ||
		attempt.HarnessResult.Validate() != nil ||
		attempt.HarnessResult.EvidenceID != evidence.ID ||
		attempt.HarnessResult.EvidenceCaptureFingerprint != evidence.CaptureFingerprint ||
		attempt.Attempt.ID != attempt.HarnessResult.AttemptID ||
		attempt.Attempt.ReviewID != evidence.ReviewID ||
		attempt.Attempt.RunID != evidence.RunID ||
		attempt.Attempt.OperationKeyDigest != evidence.OperationKeyDigest ||
		evidence.Status != DockerProductionEvidenceStatusComplete ||
		!evidence.RealDaemonContacted || evidence.RequiredCheckCount != MaxBackendChecks ||
		evidence.ObservedCount != MaxBackendChecks || evidence.ProductionVerifiedCount != 0 ||
		evidence.SufficientCheckCount != 0 || evidence.BlockerCount != MaxBackendChecks ||
		evidence.StartGatePassed || evidence.ContainerStartAuthorized ||
		evidence.ProcessExecutionAuthorized || evidence.OutputExportAuthorized ||
		evidence.ArtifactCommitAuthorized || createdAt.Before(evidence.CreatedAt) ||
		createdAt.Before(attempt.HarnessResult.CreatedAt) ||
		!validDockerProductionEvidenceReviewDecision(decision, reasonCode) {
		return DockerProductionEvidenceReview{}, errors.New(
			"docker production evidence review input is invalid or authorizing")
	}
	value := DockerProductionEvidenceReview{
		ID: id, EvidenceID: evidence.ID, AttemptID: attempt.Attempt.ID,
		StartGateReviewID: evidence.ReviewID, RunID: evidence.RunID,
		MissionID: evidence.MissionID, WorkspaceID: evidence.WorkspaceID,
		ProtocolVersion:            DockerProductionEvidenceReviewProtocolVersion,
		OperationKeyDigest:         keyDigest,
		EvidenceOperationKeyDigest: evidence.OperationKeyDigest,
		EvidenceCaptureFingerprint: evidence.CaptureFingerprint,
		HarnessResultFingerprint:   attempt.HarnessResult.ResultFingerprint,
		AuthorityFingerprint:       evidence.AuthorityFingerprint,
		ThreatModelFingerprint:     evidence.ThreatModelFingerprint,
		SuiteFingerprint:           evidence.SuiteFingerprint,
		EnvironmentFingerprint:     evidence.EnvironmentFingerprint,
		Decision:                   decision, ReasonCode: reasonCode,
		TrustClass:          DockerProductionEvidenceReviewTrustClass,
		OperatorConfirmed:   true,
		ReceiptAccepted:     decision == DockerProductionEvidenceReviewDecisionAccepted,
		RealDaemonContacted: true, RequiredCheckCount: MaxBackendChecks,
		ObservedCount: MaxBackendChecks, BlockerCount: MaxBackendChecks,
		ReviewedBy: reviewedBy, CreatedAt: createdAt.UTC(),
	}
	value.RequestFingerprint = DockerProductionEvidenceReviewRequestFingerprint(value)
	value.ReviewFingerprint = dockerProductionEvidenceReviewFingerprint(value)
	return value, value.Validate()
}

func (value DockerProductionEvidenceReview) Validate() error {
	for label, identity := range map[string]string{
		"id": value.ID, "evidence": value.EvidenceID, "attempt": value.AttemptID,
		"start-gate review": value.StartGateReviewID, "Run": value.RunID,
		"Mission": value.MissionID, "workspace": value.WorkspaceID,
		"reviewer": value.ReviewedBy,
	} {
		if err := validateStoredIdentity("Docker production evidence review "+label,
			identity); err != nil {
			return err
		}
	}
	for _, digest := range []string{
		value.OperationKeyDigest, value.RequestFingerprint,
		value.EvidenceOperationKeyDigest,
		value.EvidenceCaptureFingerprint, value.HarnessResultFingerprint,
		value.AuthorityFingerprint, value.ThreatModelFingerprint,
		value.SuiteFingerprint, value.EnvironmentFingerprint,
		value.ReviewFingerprint,
	} {
		if !validDigest(digest) {
			return errors.New("docker production evidence review fingerprint is invalid")
		}
	}
	if value.ProtocolVersion != DockerProductionEvidenceReviewProtocolVersion ||
		value.TrustClass != DockerProductionEvidenceReviewTrustClass ||
		!validDockerProductionEvidenceReviewDecision(value.Decision, value.ReasonCode) ||
		!value.OperatorConfirmed ||
		value.ReceiptAccepted != (value.Decision == DockerProductionEvidenceReviewDecisionAccepted) ||
		!value.RealDaemonContacted || value.RequiredCheckCount != MaxBackendChecks ||
		value.ObservedCount != MaxBackendChecks || value.ProductionVerifiedCount != 0 ||
		value.SufficientCheckCount != 0 || value.BlockerCount != MaxBackendChecks ||
		value.StartGatePassed || value.ContainerStartAuthorized ||
		value.ProcessExecutionAuthorized || value.OutputExportAuthorized ||
		value.ArtifactCommitAuthorized || value.CreatedAt.IsZero() ||
		value.RequestFingerprint != DockerProductionEvidenceReviewRequestFingerprint(value) ||
		value.ReviewFingerprint != dockerProductionEvidenceReviewFingerprint(value) {
		return errors.New("docker production evidence review widened authority")
	}
	return nil
}

func validDockerProductionEvidenceReviewDecision(decision, reasonCode string) bool {
	if decision == DockerProductionEvidenceReviewDecisionAccepted {
		return reasonCode == DockerProductionEvidenceReviewReasonMetadataScopeAccepted
	}
	if decision != DockerProductionEvidenceReviewDecisionRejected {
		return false
	}
	switch reasonCode {
	case DockerProductionEvidenceReviewReasonIntegrityConcern,
		DockerProductionEvidenceReviewReasonEnvironmentConcern,
		DockerProductionEvidenceReviewReasonScopeConcern,
		DockerProductionEvidenceReviewReasonInsufficientEvidence,
		DockerProductionEvidenceReviewReasonOperatorRejected:
		return true
	default:
		return false
	}
}

func dockerProductionEvidenceReviewFingerprint(value DockerProductionEvidenceReview) string {
	return fingerprint(DockerProductionEvidenceReviewProtocolVersion, value.ID,
		value.EvidenceID, value.AttemptID, value.StartGateReviewID, value.RunID,
		value.MissionID, value.WorkspaceID, value.OperationKeyDigest,
		value.RequestFingerprint,
		value.EvidenceOperationKeyDigest, value.EvidenceCaptureFingerprint,
		value.HarnessResultFingerprint, value.AuthorityFingerprint,
		value.ThreatModelFingerprint, value.SuiteFingerprint,
		value.EnvironmentFingerprint, value.Decision, value.ReasonCode,
		value.TrustClass, strconv.FormatBool(value.OperatorConfirmed),
		strconv.FormatBool(value.ReceiptAccepted),
		strconv.FormatBool(value.RealDaemonContacted),
		strconv.Itoa(value.RequiredCheckCount), strconv.Itoa(value.ObservedCount),
		strconv.Itoa(value.ProductionVerifiedCount),
		strconv.Itoa(value.SufficientCheckCount), strconv.Itoa(value.BlockerCount),
		strconv.FormatBool(value.StartGatePassed),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized), value.ReviewedBy,
		value.CreatedAt.UTC().Format(time.RFC3339Nano))
}

type DockerProductionEvidenceReviewOperation struct {
	KeyDigest          string
	RequestFingerprint string
	ReviewID           string
	EvidenceID         string
	AttemptID          string
	RunID              string
	ReviewedBy         string
	CreatedAt          time.Time
}

func NewDockerProductionEvidenceReviewOperation(keyDigest string,
	value DockerProductionEvidenceReview,
) (DockerProductionEvidenceReviewOperation, error) {
	operation := DockerProductionEvidenceReviewOperation{
		KeyDigest: keyDigest, RequestFingerprint: value.RequestFingerprint,
		ReviewID: value.ID, EvidenceID: value.EvidenceID, AttemptID: value.AttemptID,
		RunID: value.RunID, ReviewedBy: value.ReviewedBy, CreatedAt: value.CreatedAt,
	}
	return operation, operation.Validate()
}

func (value DockerProductionEvidenceReviewOperation) Validate() error {
	for label, identity := range map[string]string{
		"review": value.ReviewID, "evidence": value.EvidenceID,
		"attempt": value.AttemptID, "Run": value.RunID, "reviewer": value.ReviewedBy,
	} {
		if err := validateStoredIdentity("Docker production evidence review operation "+label,
			identity); err != nil {
			return err
		}
	}
	if !validDigest(value.KeyDigest) || !validDigest(value.RequestFingerprint) ||
		value.CreatedAt.IsZero() {
		return errors.New("docker production evidence review operation is invalid")
	}
	return nil
}

func DockerProductionEvidenceReviewRequestFingerprint(
	value DockerProductionEvidenceReview,
) string {
	return fingerprint(DockerProductionEvidenceReviewOperationVersion,
		value.EvidenceID, value.AttemptID, value.StartGateReviewID, value.RunID,
		value.MissionID, value.WorkspaceID, value.EvidenceOperationKeyDigest,
		value.EvidenceCaptureFingerprint, value.HarnessResultFingerprint,
		value.AuthorityFingerprint, value.ThreatModelFingerprint,
		value.SuiteFingerprint, value.EnvironmentFingerprint, value.Decision,
		value.ReasonCode, value.ReviewedBy)
}

func DockerProductionEvidenceReviewReasonDescription(reasonCode string) string {
	switch reasonCode {
	case DockerProductionEvidenceReviewReasonMetadataScopeAccepted:
		return "operator accepted the bounded metadata receipt; no execution authority was granted"
	case DockerProductionEvidenceReviewReasonIntegrityConcern:
		return "operator rejected the receipt because its integrity requires investigation"
	case DockerProductionEvidenceReviewReasonEnvironmentConcern:
		return "operator rejected the captured environment identity"
	case DockerProductionEvidenceReviewReasonScopeConcern:
		return "operator rejected the receipt scope"
	case DockerProductionEvidenceReviewReasonInsufficientEvidence:
		return "operator rejected the receipt as insufficient evidence"
	case DockerProductionEvidenceReviewReasonOperatorRejected:
		return "operator rejected the receipt without granting authority"
	default:
		return "unknown review reason"
	}
}
