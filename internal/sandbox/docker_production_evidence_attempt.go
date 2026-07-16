package sandbox

import (
	"errors"
	"strconv"
	"time"
)

const (
	DockerProductionEvidenceAttemptProtocolVersion        = "sandbox_docker_production_evidence_attempt.v1"
	DockerProductionEvidenceReconciliationProtocolVersion = "sandbox_docker_production_evidence_reconciliation.v1"
	DockerProductionEvidenceAttemptFailureProtocolVersion = "sandbox_docker_production_evidence_attempt_failure.v1"
	DockerProductionEvidenceAttemptResultProtocolVersion  = "sandbox_docker_production_evidence_attempt_result.v1"

	DockerProductionEvidenceAttemptLeaseActive   = "active"
	DockerProductionEvidenceAttemptLeaseReleased = "released"

	DockerProductionEvidenceReconciliationInitial  = "initial_generation_quiescent"
	DockerProductionEvidenceReconciliationRestart  = "restart_generation_quiescent"
	DockerProductionEvidenceAttemptResultCommitted = "evidence_committed"

	DockerProductionEvidenceAttemptErrorCollector       = "collector_failed"
	DockerProductionEvidenceAttemptErrorInvalidResponse = "invalid_observation"
	DockerProductionEvidenceAttemptErrorUnsafeContact   = "unsafe_daemon_contact"
	DockerProductionEvidenceAttemptErrorCanceled        = "context_canceled"
	DockerProductionEvidenceAttemptErrorDeadline        = "deadline_exceeded"
	DockerProductionEvidenceAttemptErrorPersistence     = "persistence_failed"

	MaxDockerProductionEvidenceAttemptFailures = 16
	MaxDockerProductionEvidenceAttemptsPerRun  = 32

	DefaultDockerProductionEvidenceCaptureTimeout  = 30 * time.Second
	MinDockerProductionEvidenceCaptureTimeout      = time.Second
	MaxDockerProductionEvidenceCaptureTimeout      = 2 * time.Minute
	DefaultDockerProductionEvidenceAttemptLeaseTTL = 2 * time.Minute
	MinDockerProductionEvidenceAttemptLeaseTTL     = time.Second
	MaxDockerProductionEvidenceAttemptLeaseTTL     = 10 * time.Minute
	DockerProductionEvidenceLeaseSafetyMargin      = 250 * time.Millisecond
)

type DockerProductionEvidenceAttempt struct {
	ID                          string
	ReviewID                    string
	CleanupIntentID             string
	RunID                       string
	MissionID                   string
	WorkspaceID                 string
	ProtocolVersion             string
	OperationKeyDigest          string
	RequestFingerprint          string
	ReviewFingerprint           string
	AuthorityFingerprint        string
	ThreatModelFingerprint      string
	SuiteFingerprint            string
	EndpointClass               string
	EndpointFingerprint         string
	CaptureTimeoutMillis        int64
	OperatorConfirmed           bool
	RealDaemonContactAuthorized bool
	ContainerStartAuthorized    bool
	ProcessExecutionAuthorized  bool
	OutputExportAuthorized      bool
	ArtifactCommitAuthorized    bool
	AttemptFingerprint          string
	RequestedBy                 string
	CreatedAt                   time.Time
}

func NewDockerProductionEvidenceAttempt(id, operationKeyDigest, requestedBy string,
	review DockerStartGateReview, endpoint DockerObservationEndpoint,
	operatorConfirmed bool, captureTimeout time.Duration, now time.Time,
) (DockerProductionEvidenceAttempt, error) {
	if review.Validate() != nil || review.Replayed || endpoint.Validate() != nil ||
		endpoint.Class != DockerObservationEndpointLocalUnix || !operatorConfirmed ||
		requestedBy != review.RequestedBy || now.Before(review.CreatedAt) ||
		ValidateDockerProductionEvidenceCaptureTimeout(captureTimeout) != nil {
		return DockerProductionEvidenceAttempt{}, errors.New("docker production evidence attempt authority is invalid")
	}
	value := DockerProductionEvidenceAttempt{
		ID: id, ReviewID: review.ID, CleanupIntentID: review.CleanupIntentID,
		RunID: review.RunID, MissionID: review.MissionID, WorkspaceID: review.WorkspaceID,
		ProtocolVersion:    DockerProductionEvidenceAttemptProtocolVersion,
		OperationKeyDigest: operationKeyDigest,
		RequestFingerprint: DockerProductionEvidenceCaptureRequestFingerprint(review.ID,
			review.RunID, review.AuthorityFingerprint,
			DockerProductionEvidenceSuiteFingerprint(), requestedBy),
		ReviewFingerprint:      review.ReviewFingerprint,
		AuthorityFingerprint:   review.AuthorityFingerprint,
		ThreatModelFingerprint: review.ThreatModelFingerprint,
		SuiteFingerprint:       DockerProductionEvidenceSuiteFingerprint(),
		EndpointClass:          endpoint.Class, EndpointFingerprint: endpoint.Fingerprint,
		CaptureTimeoutMillis: captureTimeout.Milliseconds(), OperatorConfirmed: true,
		RequestedBy: requestedBy, CreatedAt: now.UTC(),
	}
	value.AttemptFingerprint = dockerProductionEvidenceAttemptFingerprint(value)
	return value, value.Validate()
}

func (value DockerProductionEvidenceAttempt) Validate() error {
	for _, identity := range []string{value.ID, value.ReviewID, value.CleanupIntentID,
		value.RunID, value.MissionID, value.WorkspaceID, value.RequestedBy} {
		if validateStoredIdentity("Docker production evidence attempt identity", identity) != nil {
			return errors.New("docker production evidence attempt identity is invalid")
		}
	}
	for _, digest := range []string{value.OperationKeyDigest, value.RequestFingerprint,
		value.ReviewFingerprint, value.AuthorityFingerprint, value.ThreatModelFingerprint,
		value.SuiteFingerprint, value.EndpointFingerprint, value.AttemptFingerprint} {
		if !validDigest(digest) {
			return errors.New("docker production evidence attempt digest is invalid")
		}
	}
	endpoint, err := NewDockerObservationEndpoint(value.EndpointClass)
	timeout := time.Duration(value.CaptureTimeoutMillis) * time.Millisecond
	if err != nil || value.ProtocolVersion != DockerProductionEvidenceAttemptProtocolVersion ||
		value.SuiteFingerprint != DockerProductionEvidenceSuiteFingerprint() ||
		value.EndpointClass != DockerObservationEndpointLocalUnix ||
		value.EndpointFingerprint != endpoint.Fingerprint ||
		ValidateDockerProductionEvidenceCaptureTimeout(timeout) != nil ||
		!value.OperatorConfirmed || value.RealDaemonContactAuthorized ||
		value.ContainerStartAuthorized || value.ProcessExecutionAuthorized ||
		value.OutputExportAuthorized || value.ArtifactCommitAuthorized ||
		value.CreatedAt.IsZero() ||
		value.RequestFingerprint != DockerProductionEvidenceCaptureRequestFingerprint(
			value.ReviewID, value.RunID, value.AuthorityFingerprint,
			value.SuiteFingerprint, value.RequestedBy) ||
		value.AttemptFingerprint != dockerProductionEvidenceAttemptFingerprint(value) {
		return errors.New("docker production evidence attempt widened authority")
	}
	return nil
}

func dockerProductionEvidenceAttemptFingerprint(value DockerProductionEvidenceAttempt) string {
	return fingerprint(DockerProductionEvidenceAttemptProtocolVersion, value.ReviewID,
		value.CleanupIntentID, value.RunID, value.MissionID, value.WorkspaceID,
		value.OperationKeyDigest, value.RequestFingerprint, value.ReviewFingerprint,
		value.AuthorityFingerprint, value.ThreatModelFingerprint, value.SuiteFingerprint,
		value.EndpointClass, value.EndpointFingerprint,
		strconv.FormatInt(value.CaptureTimeoutMillis, 10),
		strconv.FormatBool(value.OperatorConfirmed),
		strconv.FormatBool(value.RealDaemonContactAuthorized),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized), value.RequestedBy)
}

func ValidateDockerProductionEvidenceCaptureTimeout(value time.Duration) error {
	if value < MinDockerProductionEvidenceCaptureTimeout ||
		value > MaxDockerProductionEvidenceCaptureTimeout ||
		value%time.Millisecond != 0 {
		return errors.New("docker production evidence capture timeout is outside the supported range")
	}
	return nil
}

type DockerProductionEvidenceAttemptLease struct {
	AttemptID  string
	LeaseID    string
	OwnerID    string
	Generation int64
	Status     string
	AcquiredAt time.Time
	ExpiresAt  time.Time
	ReleasedAt *time.Time
}

func (value DockerProductionEvidenceAttemptLease) Validate() error {
	if validateStoredIdentity("Docker production evidence attempt lease attempt", value.AttemptID) != nil ||
		validateStoredIdentity("Docker production evidence attempt lease id", value.LeaseID) != nil ||
		validateStoredIdentity("Docker production evidence attempt lease owner", value.OwnerID) != nil ||
		value.Generation < 1 || (value.Status != DockerProductionEvidenceAttemptLeaseActive &&
		value.Status != DockerProductionEvidenceAttemptLeaseReleased) ||
		value.AcquiredAt.IsZero() || value.ExpiresAt.IsZero() ||
		!value.ExpiresAt.After(value.AcquiredAt) {
		return errors.New("docker production evidence attempt lease is invalid")
	}
	if value.Status == DockerProductionEvidenceAttemptLeaseActive && value.ReleasedAt != nil {
		return errors.New("active Docker production evidence attempt lease cannot be released")
	}
	if value.Status == DockerProductionEvidenceAttemptLeaseReleased &&
		(value.ReleasedAt == nil || value.ReleasedAt.Before(value.AcquiredAt)) {
		return errors.New("released Docker production evidence attempt lease requires a release time")
	}
	return nil
}

func (value DockerProductionEvidenceAttemptLease) ActiveAt(now time.Time) bool {
	return value.Status == DockerProductionEvidenceAttemptLeaseActive && now.Before(value.ExpiresAt)
}

func ValidateDockerProductionEvidenceAttemptLeaseTTL(value time.Duration) error {
	if value < MinDockerProductionEvidenceAttemptLeaseTTL ||
		value > MaxDockerProductionEvidenceAttemptLeaseTTL {
		return errors.New("docker production evidence attempt lease TTL is outside the supported range")
	}
	return nil
}

type DockerProductionEvidenceReconciliation struct {
	AttemptID                 string
	Generation                int64
	PreviousGeneration        int64
	ProtocolVersion           string
	Status                    string
	EndpointClass             string
	EndpointFingerprint       string
	RealDaemonContacted       bool
	DaemonReadCount           int
	ReconciledResourceCount   int
	ReconciliationFingerprint string
	CreatedAt                 time.Time
}

func NewDockerProductionEvidenceReconciliation(attempt DockerProductionEvidenceAttempt,
	lease DockerProductionEvidenceAttemptLease, now time.Time,
) (DockerProductionEvidenceReconciliation, error) {
	if attempt.Validate() != nil || lease.Validate() != nil ||
		lease.AttemptID != attempt.ID || lease.Status != DockerProductionEvidenceAttemptLeaseActive ||
		now.Before(lease.AcquiredAt) || !now.Before(lease.ExpiresAt) {
		return DockerProductionEvidenceReconciliation{}, errors.New("docker production evidence reconciliation authority is invalid")
	}
	status := DockerProductionEvidenceReconciliationInitial
	previous := int64(0)
	if lease.Generation > 1 {
		status = DockerProductionEvidenceReconciliationRestart
		previous = lease.Generation - 1
	}
	value := DockerProductionEvidenceReconciliation{
		AttemptID: attempt.ID, Generation: lease.Generation, PreviousGeneration: previous,
		ProtocolVersion: DockerProductionEvidenceReconciliationProtocolVersion,
		Status:          status, EndpointClass: attempt.EndpointClass,
		EndpointFingerprint: attempt.EndpointFingerprint, CreatedAt: now.UTC(),
	}
	value.ReconciliationFingerprint = dockerProductionEvidenceReconciliationFingerprint(value)
	return value, value.Validate()
}

func (value DockerProductionEvidenceReconciliation) Validate() error {
	if validateStoredIdentity("Docker production evidence reconciliation attempt", value.AttemptID) != nil ||
		value.Generation < 1 || value.ProtocolVersion != DockerProductionEvidenceReconciliationProtocolVersion ||
		!validDigest(value.EndpointFingerprint) || !validDigest(value.ReconciliationFingerprint) ||
		value.EndpointClass != DockerObservationEndpointLocalUnix || value.RealDaemonContacted ||
		value.DaemonReadCount != 0 || value.ReconciledResourceCount != 0 || value.CreatedAt.IsZero() {
		return errors.New("docker production evidence reconciliation is invalid")
	}
	if (value.Generation == 1 && (value.PreviousGeneration != 0 ||
		value.Status != DockerProductionEvidenceReconciliationInitial)) ||
		(value.Generation > 1 && (value.PreviousGeneration != value.Generation-1 ||
			value.Status != DockerProductionEvidenceReconciliationRestart)) ||
		value.ReconciliationFingerprint != dockerProductionEvidenceReconciliationFingerprint(value) {
		return errors.New("docker production evidence reconciliation generation is invalid")
	}
	return nil
}

func dockerProductionEvidenceReconciliationFingerprint(value DockerProductionEvidenceReconciliation) string {
	return fingerprint(DockerProductionEvidenceReconciliationProtocolVersion, value.AttemptID,
		strconv.FormatInt(value.Generation, 10), strconv.FormatInt(value.PreviousGeneration, 10),
		value.Status, value.EndpointClass, value.EndpointFingerprint,
		strconv.FormatBool(value.RealDaemonContacted), strconv.Itoa(value.DaemonReadCount),
		strconv.Itoa(value.ReconciledResourceCount))
}

type DockerProductionEvidenceAttemptFailure struct {
	AttemptID          string
	Sequence           int
	Generation         int64
	ProtocolVersion    string
	Code               string
	FailureFingerprint string
	CreatedAt          time.Time
}

func NewDockerProductionEvidenceAttemptFailure(attemptID string, sequence int,
	generation int64, code string, now time.Time,
) (DockerProductionEvidenceAttemptFailure, error) {
	value := DockerProductionEvidenceAttemptFailure{AttemptID: attemptID, Sequence: sequence,
		Generation: generation, ProtocolVersion: DockerProductionEvidenceAttemptFailureProtocolVersion,
		Code: code, CreatedAt: now.UTC()}
	value.FailureFingerprint = fingerprint(DockerProductionEvidenceAttemptFailureProtocolVersion,
		attemptID, strconv.Itoa(sequence), strconv.FormatInt(generation, 10), code)
	return value, value.Validate()
}

func (value DockerProductionEvidenceAttemptFailure) Validate() error {
	if validateStoredIdentity("Docker production evidence attempt failure", value.AttemptID) != nil ||
		value.Sequence < 1 || value.Sequence > MaxDockerProductionEvidenceAttemptFailures ||
		value.Generation < 1 || value.ProtocolVersion != DockerProductionEvidenceAttemptFailureProtocolVersion ||
		!validDockerProductionEvidenceAttemptFailureCode(value.Code) ||
		!validDigest(value.FailureFingerprint) || value.CreatedAt.IsZero() ||
		value.FailureFingerprint != fingerprint(DockerProductionEvidenceAttemptFailureProtocolVersion,
			value.AttemptID, strconv.Itoa(value.Sequence), strconv.FormatInt(value.Generation, 10), value.Code) {
		return errors.New("docker production evidence attempt failure is invalid")
	}
	return nil
}

func validDockerProductionEvidenceAttemptFailureCode(value string) bool {
	switch value {
	case DockerProductionEvidenceAttemptErrorCollector,
		DockerProductionEvidenceAttemptErrorInvalidResponse,
		DockerProductionEvidenceAttemptErrorUnsafeContact,
		DockerProductionEvidenceAttemptErrorCanceled,
		DockerProductionEvidenceAttemptErrorDeadline,
		DockerProductionEvidenceAttemptErrorPersistence:
		return true
	default:
		return false
	}
}

type DockerProductionEvidenceAttemptResult struct {
	AttemptID                  string
	EvidenceID                 string
	ProtocolVersion            string
	Status                     string
	LeaseGeneration            int64
	ReconciliationFingerprint  string
	EvidenceCaptureFingerprint string
	RealDaemonContacted        bool
	ContainerStartAuthorized   bool
	ProcessExecutionAuthorized bool
	OutputExportAuthorized     bool
	ArtifactCommitAuthorized   bool
	ResultFingerprint          string
	CreatedAt                  time.Time
}

func NewDockerProductionEvidenceAttemptResult(attempt DockerProductionEvidenceAttempt,
	lease DockerProductionEvidenceAttemptLease,
	reconciliation DockerProductionEvidenceReconciliation,
	evidence DockerProductionEvidence,
) (DockerProductionEvidenceAttemptResult, error) {
	if attempt.Validate() != nil || lease.Validate() != nil || reconciliation.Validate() != nil ||
		evidence.Validate() != nil || lease.AttemptID != attempt.ID ||
		lease.Status != DockerProductionEvidenceAttemptLeaseActive ||
		reconciliation.AttemptID != attempt.ID || reconciliation.Generation != lease.Generation ||
		evidence.ReviewID != attempt.ReviewID || evidence.RunID != attempt.RunID ||
		evidence.OperationKeyDigest != attempt.OperationKeyDigest ||
		evidence.RequestedBy != attempt.RequestedBy || evidence.RealDaemonContacted ||
		evidence.CreatedAt.Before(reconciliation.CreatedAt) {
		return DockerProductionEvidenceAttemptResult{}, errors.New("docker production evidence attempt result authority is invalid")
	}
	value := DockerProductionEvidenceAttemptResult{
		AttemptID: attempt.ID, EvidenceID: evidence.ID,
		ProtocolVersion:            DockerProductionEvidenceAttemptResultProtocolVersion,
		Status:                     DockerProductionEvidenceAttemptResultCommitted,
		LeaseGeneration:            lease.Generation,
		ReconciliationFingerprint:  reconciliation.ReconciliationFingerprint,
		EvidenceCaptureFingerprint: evidence.CaptureFingerprint,
		CreatedAt:                  evidence.CreatedAt,
	}
	value.ResultFingerprint = dockerProductionEvidenceAttemptResultFingerprint(value)
	return value, value.Validate()
}

func (value DockerProductionEvidenceAttemptResult) Validate() error {
	if validateStoredIdentity("Docker production evidence attempt result attempt", value.AttemptID) != nil ||
		validateStoredIdentity("Docker production evidence attempt result evidence", value.EvidenceID) != nil ||
		value.ProtocolVersion != DockerProductionEvidenceAttemptResultProtocolVersion ||
		value.Status != DockerProductionEvidenceAttemptResultCommitted || value.LeaseGeneration < 1 ||
		!validDigest(value.ReconciliationFingerprint) ||
		!validDigest(value.EvidenceCaptureFingerprint) || !validDigest(value.ResultFingerprint) ||
		value.RealDaemonContacted || value.ContainerStartAuthorized ||
		value.ProcessExecutionAuthorized || value.OutputExportAuthorized ||
		value.ArtifactCommitAuthorized || value.CreatedAt.IsZero() ||
		value.ResultFingerprint != dockerProductionEvidenceAttemptResultFingerprint(value) {
		return errors.New("docker production evidence attempt result widened authority")
	}
	return nil
}

func dockerProductionEvidenceAttemptResultFingerprint(value DockerProductionEvidenceAttemptResult) string {
	return fingerprint(DockerProductionEvidenceAttemptResultProtocolVersion, value.AttemptID,
		value.EvidenceID, value.Status, strconv.FormatInt(value.LeaseGeneration, 10),
		value.ReconciliationFingerprint, value.EvidenceCaptureFingerprint,
		strconv.FormatBool(value.RealDaemonContacted),
		strconv.FormatBool(value.ContainerStartAuthorized),
		strconv.FormatBool(value.ProcessExecutionAuthorized),
		strconv.FormatBool(value.OutputExportAuthorized),
		strconv.FormatBool(value.ArtifactCommitAuthorized))
}

type DockerProductionEvidenceAttemptRecord struct {
	Attempt         DockerProductionEvidenceAttempt
	Lease           DockerProductionEvidenceAttemptLease
	Reconciliations []DockerProductionEvidenceReconciliation
	Failures        []DockerProductionEvidenceAttemptFailure
	Result          *DockerProductionEvidenceAttemptResult
	Replayed        bool
	TookOver        bool
}

func (value DockerProductionEvidenceAttemptRecord) Validate() error {
	if value.Attempt.Validate() != nil || value.Lease.Validate() != nil ||
		value.Lease.AttemptID != value.Attempt.ID ||
		len(value.Failures) > MaxDockerProductionEvidenceAttemptFailures {
		return errors.New("docker production evidence attempt record is invalid")
	}
	previousGeneration := int64(0)
	for _, reconciliation := range value.Reconciliations {
		if reconciliation.Validate() != nil || reconciliation.AttemptID != value.Attempt.ID ||
			reconciliation.Generation <= previousGeneration || reconciliation.Generation > value.Lease.Generation {
			return errors.New("docker production evidence reconciliation record is invalid")
		}
		previousGeneration = reconciliation.Generation
	}
	for index, failure := range value.Failures {
		if failure.Validate() != nil || failure.AttemptID != value.Attempt.ID ||
			failure.Sequence != index+1 || failure.Generation > value.Lease.Generation {
			return errors.New("docker production evidence attempt failure sequence is invalid")
		}
	}
	if value.Result == nil {
		return nil
	}
	if value.Result.Validate() != nil || value.Result.AttemptID != value.Attempt.ID ||
		value.Result.LeaseGeneration != value.Lease.Generation ||
		value.Lease.Status != DockerProductionEvidenceAttemptLeaseReleased ||
		len(value.Reconciliations) == 0 ||
		value.Reconciliations[len(value.Reconciliations)-1].Generation != value.Result.LeaseGeneration ||
		value.Reconciliations[len(value.Reconciliations)-1].ReconciliationFingerprint !=
			value.Result.ReconciliationFingerprint {
		return errors.New("docker production evidence attempt result binding is invalid")
	}
	return nil
}

func (value DockerProductionEvidenceAttemptRecord) StatusAt(now time.Time) string {
	if value.Result != nil {
		return "evidence_committed"
	}
	if value.Lease.ActiveAt(now) {
		return "leased"
	}
	if value.Lease.Status == DockerProductionEvidenceAttemptLeaseActive {
		return "interrupted"
	}
	if len(value.Failures) > 0 {
		return "failed"
	}
	return "pending"
}

func (value DockerProductionEvidenceAttemptRecord) CurrentReconciliation() (
	DockerProductionEvidenceReconciliation, bool,
) {
	for index := len(value.Reconciliations) - 1; index >= 0; index-- {
		if value.Reconciliations[index].Generation == value.Lease.Generation {
			return value.Reconciliations[index], true
		}
	}
	return DockerProductionEvidenceReconciliation{}, false
}

type DockerProductionEvidenceAttemptAcquisition struct {
	Record   DockerProductionEvidenceAttemptRecord
	Replayed bool
	TookOver bool
}
