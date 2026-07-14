package sandbox

import (
	"errors"
	"fmt"
	"mime"
	"strconv"
	"time"
)

const (
	DisabledExecutionProtocolVersion = "sandbox_execution.v1"
	CancellationProtocolVersion      = "sandbox_execution_cancel.v1"
	CleanupProtocolVersion           = "sandbox_cleanup.v1"
	MaxInputArtifactTotalBytes       = 16 * 1024 * 1024
	MinExecutionLeaseTTL             = time.Second
	MaxExecutionLeaseTTL             = 5 * time.Minute
)

func ValidateExecutionLeaseTTL(ttl time.Duration) error {
	if ttl < MinExecutionLeaseTTL || ttl > MaxExecutionLeaseTTL {
		return fmt.Errorf("sandbox execution lease TTL must be between %s and %s",
			MinExecutionLeaseTTL, MaxExecutionLeaseTTL)
	}
	return nil
}

type ExecutionLeaseStatus string

const (
	ExecutionLeaseActive   ExecutionLeaseStatus = "active"
	ExecutionLeaseReleased ExecutionLeaseStatus = "released"
)

func (s ExecutionLeaseStatus) Valid() bool {
	return s == ExecutionLeaseActive || s == ExecutionLeaseReleased
}

type LifecycleStatus string

const (
	LifecyclePrepared        LifecycleStatus = "prepared"
	LifecycleCancelPending   LifecycleStatus = "cancel_requested"
	LifecycleCleanupComplete LifecycleStatus = "cleaned"
)

type InputArtifactBinding struct {
	ExecutionID string
	Ordinal     int
	ArtifactID  string
	SHA256      string
	SizeBytes   int64
	MIME        string
	Stream      string
	SourceID    string
	Redacted    bool
}

func (b InputArtifactBinding) Validate() error {
	for label, value := range map[string]string{
		"input execution id": b.ExecutionID, "input artifact id": b.ArtifactID,
		"input source id": b.SourceID,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if b.Ordinal < 1 || b.Ordinal > MaxInputArtifacts || !validDigest(b.SHA256) ||
		b.SizeBytes < 1 || b.SizeBytes > MaxInputArtifactTotalBytes {
		return errors.New("sandbox input Artifact ordinal, digest, or size is invalid")
	}
	if b.Stream != "stdout" && b.Stream != "stderr" {
		return errors.New("sandbox input Artifact stream is invalid")
	}
	if b.MIME == "" || len([]byte(b.MIME)) > 256 {
		return errors.New("sandbox input Artifact MIME is invalid")
	}
	if _, _, err := mime.ParseMediaType(b.MIME); err != nil {
		return errors.New("sandbox input Artifact MIME is invalid")
	}
	return nil
}

type OutputCapturePlan struct {
	CaptureStdout   bool
	CaptureStderr   bool
	OutputPathCount int
	MaxOutputBytes  int64
	Fingerprint     string
}

func NewOutputCapturePlan(manifest Manifest) OutputCapturePlan {
	parts := []string{
		"sandbox_output_capture_plan.v1",
		strconv.FormatBool(manifest.Output.CaptureStdout),
		strconv.FormatBool(manifest.Output.CaptureStderr),
		strconv.Itoa(len(manifest.Output.Paths)),
		strconv.FormatInt(manifest.Resources.MaxOutputBytes, 10),
	}
	parts = append(parts, manifest.Output.Paths...)
	return OutputCapturePlan{
		CaptureStdout: manifest.Output.CaptureStdout, CaptureStderr: manifest.Output.CaptureStderr,
		OutputPathCount: len(manifest.Output.Paths), MaxOutputBytes: manifest.Resources.MaxOutputBytes,
		Fingerprint: fingerprint(parts...),
	}
}

func (p OutputCapturePlan) Validate() error {
	if !p.CaptureStdout && !p.CaptureStderr && p.OutputPathCount == 0 {
		return errors.New("sandbox output capture plan must retain at least one output")
	}
	if p.OutputPathCount < 0 || p.OutputPathCount > MaxOutputPaths ||
		p.MaxOutputBytes < 1 || p.MaxOutputBytes > MaxCapturedOutputBytes ||
		!validDigest(p.Fingerprint) {
		return errors.New("sandbox output capture plan is outside protocol bounds")
	}
	return nil
}

type DisabledExecution struct {
	ID                       string
	CandidateID              string
	PreparationID            string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	CancellationID           string
	ProtocolVersion          string
	ManifestFingerprint      string
	AuthorizationFingerprint string
	PolicyFingerprint        string
	MountBindingFingerprint  string
	InputArtifactCount       int
	InputArtifactBytes       int64
	InputArtifactDigest      string
	OutputPlan               OutputCapturePlan
	InitialLeaseID           string
	InitialLeaseGeneration   int64
	BackendEnabled           bool
	ExecutionAuthorized      bool
	BackendStarted           bool
	RequestedBy              string
	CreatedAt                time.Time
}

func (e DisabledExecution) Validate() error {
	for label, value := range map[string]string{
		"execution id": e.ID, "candidate id": e.CandidateID,
		"execution preparation id": e.PreparationID, "execution Run id": e.RunID,
		"execution Mission id": e.MissionID, "execution workspace id": e.WorkspaceID,
		"execution cancellation id": e.CancellationID, "execution initial lease id": e.InitialLeaseID,
		"execution requester": e.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if e.ProtocolVersion != DisabledExecutionProtocolVersion {
		return fmt.Errorf("unsupported sandbox execution protocol %q", e.ProtocolVersion)
	}
	for label, digest := range map[string]string{
		"manifest": e.ManifestFingerprint, "authorization": e.AuthorizationFingerprint,
		"policy": e.PolicyFingerprint, "mount binding": e.MountBindingFingerprint,
		"input Artifact": e.InputArtifactDigest,
	} {
		if !validDigest(digest) {
			return fmt.Errorf("sandbox execution %s fingerprint is invalid", label)
		}
	}
	if e.InputArtifactCount < 0 || e.InputArtifactCount > MaxInputArtifacts ||
		e.InputArtifactBytes < 0 || e.InputArtifactBytes > MaxInputArtifactTotalBytes ||
		(e.InputArtifactCount == 0 && e.InputArtifactBytes != 0) ||
		(e.InputArtifactCount > 0 && e.InputArtifactBytes == 0) {
		return errors.New("sandbox execution input Artifact totals are invalid")
	}
	if err := e.OutputPlan.Validate(); err != nil {
		return err
	}
	if e.InitialLeaseGeneration != 1 || e.BackendEnabled || e.ExecutionAuthorized || e.BackendStarted {
		return errors.New("sandbox execution must start under generation one with every backend capability disabled")
	}
	if e.CreatedAt.IsZero() {
		return errors.New("sandbox execution creation timestamp is required")
	}
	return nil
}

type ExecutionLease struct {
	ExecutionID string
	LeaseID     string
	OwnerID     string
	Generation  int64
	Status      ExecutionLeaseStatus
	AcquiredAt  time.Time
	RenewedAt   time.Time
	ExpiresAt   time.Time
	ReleasedAt  *time.Time
}

func (l ExecutionLease) Validate() error {
	for label, value := range map[string]string{
		"execution lease execution id": l.ExecutionID,
		"execution lease id":           l.LeaseID, "execution lease owner": l.OwnerID,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if l.Generation < 1 || !l.Status.Valid() || l.AcquiredAt.IsZero() || l.RenewedAt.IsZero() ||
		l.ExpiresAt.IsZero() || l.RenewedAt.Before(l.AcquiredAt) || !l.ExpiresAt.After(l.RenewedAt) {
		return errors.New("sandbox execution lease fields are invalid")
	}
	if l.Status == ExecutionLeaseActive && l.ReleasedAt != nil {
		return errors.New("active sandbox execution lease cannot be released")
	}
	if l.Status == ExecutionLeaseReleased && (l.ReleasedAt == nil || l.ReleasedAt.Before(l.AcquiredAt)) {
		return errors.New("released sandbox execution lease requires a valid release time")
	}
	return nil
}

func (l ExecutionLease) ActiveAt(now time.Time) bool {
	return l.Status == ExecutionLeaseActive && now.Before(l.ExpiresAt)
}

type LeaseAcquisition struct {
	Lease    ExecutionLease
	Replayed bool
	TookOver bool
}

type CancellationRequest struct {
	ID              string
	ExecutionID     string
	RunID           string
	CancellationID  string
	ProtocolVersion string
	RequestedBy     string
	RequestedAt     time.Time
}

func (r CancellationRequest) Validate() error {
	for label, value := range map[string]string{
		"cancellation request id": r.ID, "cancellation execution id": r.ExecutionID,
		"cancellation Run id": r.RunID, "cancellation identity": r.CancellationID,
		"cancellation requester": r.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if r.ProtocolVersion != CancellationProtocolVersion || r.RequestedAt.IsZero() {
		return errors.New("sandbox cancellation protocol or timestamp is invalid")
	}
	return nil
}

type CleanupResult struct {
	ID                     string
	ExecutionID            string
	RunID                  string
	ProtocolVersion        string
	LeaseID                string
	LeaseGeneration        int64
	CancellationObserved   bool
	BackendStarted         bool
	OrphanDetected         bool
	OrphanReaped           bool
	InputArtifactsVerified bool
	OutputArtifactCount    int
	CleanupComplete        bool
	Outcome                string
	ReconciledBy           string
	CompletedAt            time.Time
}

func (r CleanupResult) Validate() error {
	for label, value := range map[string]string{
		"cleanup id": r.ID, "cleanup execution id": r.ExecutionID,
		"cleanup Run id": r.RunID, "cleanup lease id": r.LeaseID,
		"cleanup reconciler": r.ReconciledBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if r.ProtocolVersion != CleanupProtocolVersion || r.LeaseGeneration < 1 ||
		r.BackendStarted || r.OrphanDetected || r.OrphanReaped || !r.InputArtifactsVerified ||
		r.OutputArtifactCount != 0 || !r.CleanupComplete || r.Outcome != "backend_disabled" ||
		r.CompletedAt.IsZero() {
		return errors.New("sandbox cleanup must be a complete disabled-backend result")
	}
	return nil
}

type ExecutionOperation struct {
	KeyDigest          string
	RequestFingerprint string
	ExecutionID        string
	CandidateID        string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func (o ExecutionOperation) Validate() error {
	for label, value := range map[string]string{
		"execution operation execution id": o.ExecutionID,
		"execution operation candidate id": o.CandidateID,
		"execution operation Run id":       o.RunID, "execution operation requester": o.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if !validDigest(o.KeyDigest) || !validDigest(o.RequestFingerprint) || o.CreatedAt.IsZero() {
		return errors.New("sandbox execution operation digests or timestamp are invalid")
	}
	return nil
}

type CancellationOperation struct {
	KeyDigest          string
	RequestFingerprint string
	RequestID          string
	ExecutionID        string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func (o CancellationOperation) Validate() error {
	for label, value := range map[string]string{
		"cancellation operation request id":   o.RequestID,
		"cancellation operation execution id": o.ExecutionID,
		"cancellation operation Run id":       o.RunID, "cancellation operation requester": o.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if !validDigest(o.KeyDigest) || !validDigest(o.RequestFingerprint) || o.CreatedAt.IsZero() {
		return errors.New("sandbox cancellation operation digests or timestamp are invalid")
	}
	return nil
}

type CleanupOperation struct {
	KeyDigest          string
	RequestFingerprint string
	CleanupID          string
	ExecutionID        string
	RunID              string
	ReconciledBy       string
	CreatedAt          time.Time
}

func (o CleanupOperation) Validate() error {
	for label, value := range map[string]string{
		"cleanup operation cleanup id":   o.CleanupID,
		"cleanup operation execution id": o.ExecutionID,
		"cleanup operation Run id":       o.RunID, "cleanup operation reconciler": o.ReconciledBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if !validDigest(o.KeyDigest) || !validDigest(o.RequestFingerprint) || o.CreatedAt.IsZero() {
		return errors.New("sandbox cleanup operation digests or timestamp are invalid")
	}
	return nil
}

type Lifecycle struct {
	Execution    DisabledExecution
	Inputs       []InputArtifactBinding
	Lease        ExecutionLease
	Cancellation *CancellationRequest
	Cleanup      *CleanupResult
	Status       LifecycleStatus
	Replayed     bool
}

func ExecutionRequestFingerprint(execution DisabledExecution) string {
	return fingerprint("sandbox_execution_request.v1", execution.CandidateID,
		execution.ManifestFingerprint, execution.InputArtifactDigest,
		execution.OutputPlan.Fingerprint, execution.RequestedBy)
}

func CancellationRequestFingerprint(executionID, cancellationID, requestedBy string) string {
	return fingerprint("sandbox_execution_cancel_request.v1", executionID, cancellationID, requestedBy)
}

func CleanupRequestFingerprint(executionID string, cancellationObserved bool, reconciledBy string) string {
	return fingerprint("sandbox_cleanup_request.v1", executionID,
		strconv.FormatBool(cancellationObserved), reconciledBy)
}

func InputArtifactBindingsDigest(bindings []InputArtifactBinding) string {
	parts := []string{"sandbox_input_artifact_bindings.v1", strconv.Itoa(len(bindings))}
	for _, binding := range bindings {
		parts = append(parts, strconv.Itoa(binding.Ordinal), binding.ArtifactID, binding.SHA256,
			strconv.FormatInt(binding.SizeBytes, 10), binding.MIME, binding.Stream,
			binding.SourceID, strconv.FormatBool(binding.Redacted))
	}
	return fingerprint(parts...)
}
