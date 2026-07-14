package sandbox

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const ValidationProtocolVersion = "sandbox_validation.v1"

type ApprovalStatus string

const (
	ApprovalNotApplicable ApprovalStatus = "not_applicable"
	ApprovalNotRequired   ApprovalStatus = "not_required"
	ApprovalRequired      ApprovalStatus = "required"
	ApprovalPending       ApprovalStatus = "pending"
	ApprovalApproved      ApprovalStatus = "approved"
	ApprovalDenied        ApprovalStatus = "denied"
)

func (s ApprovalStatus) Valid() bool {
	switch s {
	case ApprovalNotApplicable, ApprovalNotRequired, ApprovalRequired,
		ApprovalPending, ApprovalApproved, ApprovalDenied:
		return true
	default:
		return false
	}
}

type WorkspaceBinding struct {
	ID       string
	RootPath string
}

type Preparation struct {
	ID                       string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	CancellationID           string
	ProtocolVersion          string
	Backend                  Backend
	ManifestFingerprint      string
	AuthorizationFingerprint string
	WorkspaceFingerprint     string
	ScopeFingerprint         string
	CommandArgumentCount     int
	MountCount               int
	WritableMountCount       int
	EnvironmentCount         int
	SecretReferenceCount     int
	NetworkMode              string
	AllowedTargetCount       int
	InputArtifactCount       int
	OutputCount              int
	TimeoutSeconds           int
	GracePeriodMillis        int
	CPUQuotaMillis           int
	MemoryBytes              int64
	PIDs                     int
	MaxOutputBytes           int64
	RequestedBy              string
	PreparedAt               time.Time
}

type Validation struct {
	PreparationID       string
	RunID               string
	ProtocolVersion     string
	PolicyAllowed       bool
	NeedsApproval       bool
	Risk                string
	PolicyFingerprint   string
	ApprovalID          string
	ApprovalStatus      ApprovalStatus
	ValidatorName       string
	BackendEnabled      bool
	ExecutionAuthorized bool
	ValidatedAt         time.Time
}

type Operation struct {
	KeyDigest          string
	RequestFingerprint string
	PreparationID      string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

type PreparedIntent struct {
	Preparation Preparation
	Validation  Validation
	Replayed    bool
}

func (p Preparation) Validate() error {
	for label, value := range map[string]string{
		"preparation id": p.ID, "Run id": p.RunID, "Mission id": p.MissionID,
		"workspace id": p.WorkspaceID, "cancellation id": p.CancellationID,
		"requester": p.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if p.ProtocolVersion != ManifestProtocolVersion || !p.Backend.Valid() {
		return errors.New("sandbox preparation protocol or backend is invalid")
	}
	for label, digest := range map[string]string{
		"manifest": p.ManifestFingerprint, "authorization": p.AuthorizationFingerprint,
		"workspace": p.WorkspaceFingerprint, "scope": p.ScopeFingerprint,
	} {
		if !validDigest(digest) {
			return fmt.Errorf("sandbox preparation %s fingerprint is invalid", label)
		}
	}
	if p.CommandArgumentCount < 0 || p.CommandArgumentCount > MaxCommandArguments ||
		p.MountCount < 1 || p.MountCount > MaxMounts ||
		p.WritableMountCount < 0 || p.WritableMountCount > p.MountCount ||
		p.EnvironmentCount < 0 || p.EnvironmentCount > MaxEnvironmentBindings ||
		p.SecretReferenceCount < 0 || p.SecretReferenceCount > p.EnvironmentCount ||
		p.AllowedTargetCount < 0 || p.AllowedTargetCount > MaxNetworkTargets ||
		p.InputArtifactCount < 0 || p.InputArtifactCount > MaxInputArtifacts ||
		p.OutputCount < 1 || p.OutputCount > MaxOutputPaths+2 {
		return errors.New("sandbox preparation metadata counts are outside protocol bounds")
	}
	if (p.NetworkMode == "disabled" && p.AllowedTargetCount != 0) ||
		(p.NetworkMode == "allowlist" && p.AllowedTargetCount == 0) ||
		(p.NetworkMode != "disabled" && p.NetworkMode != "allowlist") {
		return errors.New("sandbox preparation network metadata is invalid")
	}
	if p.TimeoutSeconds < 1 || p.TimeoutSeconds > MaxTimeoutSeconds ||
		p.GracePeriodMillis < 0 || p.GracePeriodMillis > MaxCancellationGraceMS ||
		p.CPUQuotaMillis < 1 || p.CPUQuotaMillis > MaxCPUQuotaMillis ||
		p.MemoryBytes < MinMemoryBytes || p.MemoryBytes > MaxMemoryBytes ||
		p.PIDs < 1 || p.PIDs > MaxPIDs ||
		p.MaxOutputBytes < 1 || p.MaxOutputBytes > MaxCapturedOutputBytes {
		return errors.New("sandbox preparation resource metadata is outside protocol bounds")
	}
	if p.PreparedAt.IsZero() {
		return errors.New("sandbox preparation timestamp is required")
	}
	return nil
}

func (v Validation) Validate() error {
	if err := validateStoredIdentity("validation preparation id", v.PreparationID); err != nil {
		return err
	}
	if err := validateStoredIdentity("validation Run id", v.RunID); err != nil {
		return err
	}
	if v.ProtocolVersion != ValidationProtocolVersion || !validDigest(v.PolicyFingerprint) {
		return errors.New("sandbox validation protocol or policy fingerprint is invalid")
	}
	if v.Risk != "low" && v.Risk != "medium" && v.Risk != "high" && v.Risk != "critical" {
		return fmt.Errorf("sandbox validation risk %q is invalid", v.Risk)
	}
	if !v.ApprovalStatus.Valid() {
		return fmt.Errorf("sandbox validation approval status %q is invalid", v.ApprovalStatus)
	}
	if err := validateStoredIdentity("validator name", v.ValidatorName); err != nil {
		return err
	}
	if v.ValidatorName != "noop" || v.BackendEnabled || v.ExecutionAuthorized {
		return errors.New("sandbox validation must remain Noop and execution-disabled")
	}
	if !v.PolicyAllowed {
		if v.NeedsApproval || v.ApprovalID != "" || v.ApprovalStatus != ApprovalNotApplicable {
			return errors.New("policy-denied sandbox validation cannot bind approval")
		}
	} else if !v.NeedsApproval {
		if v.ApprovalID != "" || v.ApprovalStatus != ApprovalNotRequired {
			return errors.New("sandbox validation without approval requirement cannot bind approval")
		}
	} else if v.ApprovalID == "" {
		if v.ApprovalStatus != ApprovalRequired {
			return errors.New("sandbox validation requiring approval must record required status")
		}
	} else {
		if err := validateStoredIdentity("validation approval id", v.ApprovalID); err != nil {
			return err
		}
		if v.ApprovalStatus != ApprovalPending && v.ApprovalStatus != ApprovalApproved &&
			v.ApprovalStatus != ApprovalDenied {
			return errors.New("bound sandbox approval status is invalid")
		}
	}
	if v.ValidatedAt.IsZero() {
		return errors.New("sandbox validation timestamp is required")
	}
	return nil
}

func (o Operation) Validate() error {
	for label, value := range map[string]string{
		"operation preparation id": o.PreparationID, "operation Run id": o.RunID,
		"operation requester": o.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if !validDigest(o.KeyDigest) || !validDigest(o.RequestFingerprint) {
		return errors.New("sandbox operation digests are invalid")
	}
	if o.CreatedAt.IsZero() {
		return errors.New("sandbox operation timestamp is required")
	}
	return nil
}

func IntentRequestFingerprint(preparation Preparation, validation Validation) string {
	return fingerprint("sandbox_intent_request.v1", preparation.RunID,
		preparation.MissionID, preparation.WorkspaceID, preparation.ManifestFingerprint,
		preparation.AuthorizationFingerprint, preparation.WorkspaceFingerprint,
		preparation.ScopeFingerprint, string(preparation.Backend), validation.PolicyFingerprint,
		validation.ApprovalID, string(validation.ApprovalStatus), preparation.RequestedBy)
}

func validateStoredIdentity(label, value string) error {
	if value == "" || strings.TrimSpace(value) != value || !utf8.ValidString(value) ||
		len([]rune(value)) > 256 || strings.ContainsRune(value, 0) {
		return fmt.Errorf("sandbox %s must be normalized and bounded UTF-8", label)
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return fmt.Errorf("sandbox %s cannot contain control characters", label)
		}
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != sha256DigestLength || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256DigestLength/2
}

const sha256DigestLength = 64
