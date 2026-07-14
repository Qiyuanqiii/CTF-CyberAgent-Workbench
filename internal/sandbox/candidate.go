package sandbox

import (
	"errors"
	"fmt"
	"time"
)

const ExecutionCandidateProtocolVersion = "sandbox_execution_candidate.v1"

type ExecutionCandidate struct {
	ID                       string
	PreparationID            string
	RunID                    string
	MissionID                string
	WorkspaceID              string
	ProtocolVersion          string
	ManifestFingerprint      string
	AuthorizationFingerprint string
	WorkspaceFingerprint     string
	ScopeFingerprint         string
	PolicyFingerprint        string
	MountBindingFingerprint  string
	ApprovalID               string
	ApprovalStatus           ApprovalStatus
	MountCount               int
	RegularFileMountCount    int
	DirectoryMountCount      int
	TokensUsed               int64
	ExecutionMillisUsed      int64
	ToolCallsUsed            int64
	BudgetChecked            bool
	LeaseQuiescent           bool
	BackendEnabled           bool
	ExecutionAuthorized      bool
	RequestedBy              string
	ValidatedAt              time.Time
}

type CandidateOperation struct {
	KeyDigest          string
	RequestFingerprint string
	CandidateID        string
	PreparationID      string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

type ValidatedExecutionCandidate struct {
	Candidate ExecutionCandidate
	Replayed  bool
}

func (c ExecutionCandidate) Validate() error {
	for label, value := range map[string]string{
		"candidate id": c.ID, "preparation id": c.PreparationID, "Run id": c.RunID,
		"Mission id": c.MissionID, "workspace id": c.WorkspaceID,
		"requester": c.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if c.ProtocolVersion != ExecutionCandidateProtocolVersion {
		return fmt.Errorf("unsupported sandbox execution candidate protocol %q", c.ProtocolVersion)
	}
	for label, digest := range map[string]string{
		"manifest": c.ManifestFingerprint, "authorization": c.AuthorizationFingerprint,
		"workspace": c.WorkspaceFingerprint, "scope": c.ScopeFingerprint,
		"policy": c.PolicyFingerprint, "mount binding": c.MountBindingFingerprint,
	} {
		if !validDigest(digest) {
			return fmt.Errorf("sandbox execution candidate %s fingerprint is invalid", label)
		}
	}
	if c.MountCount < 1 || c.MountCount > MaxMounts ||
		c.RegularFileMountCount < 0 || c.DirectoryMountCount < 0 ||
		c.RegularFileMountCount+c.DirectoryMountCount != c.MountCount {
		return errors.New("sandbox execution candidate mount counts are invalid")
	}
	if c.TokensUsed < 0 || c.ExecutionMillisUsed < 0 || c.ToolCallsUsed < 0 {
		return errors.New("sandbox execution candidate usage counters cannot be negative")
	}
	if !c.BudgetChecked || !c.LeaseQuiescent || c.BackendEnabled || c.ExecutionAuthorized {
		return errors.New("sandbox execution candidate must remain budget-checked, quiescent, and execution-disabled")
	}
	if c.ApprovalID == "" {
		if c.ApprovalStatus != ApprovalNotRequired {
			return errors.New("sandbox execution candidate without approval must record not_required")
		}
	} else {
		if err := validateStoredIdentity("candidate approval id", c.ApprovalID); err != nil {
			return err
		}
		if c.ApprovalStatus != ApprovalApproved {
			return errors.New("sandbox execution candidate approval must be approved")
		}
	}
	if c.ValidatedAt.IsZero() {
		return errors.New("sandbox execution candidate timestamp is required")
	}
	return nil
}

func (o CandidateOperation) Validate() error {
	for label, value := range map[string]string{
		"candidate operation candidate id":   o.CandidateID,
		"candidate operation preparation id": o.PreparationID,
		"candidate operation Run id":         o.RunID,
		"candidate operation requester":      o.RequestedBy,
	} {
		if err := validateStoredIdentity(label, value); err != nil {
			return err
		}
	}
	if !validDigest(o.KeyDigest) || !validDigest(o.RequestFingerprint) {
		return errors.New("sandbox execution candidate operation digests are invalid")
	}
	if o.CreatedAt.IsZero() {
		return errors.New("sandbox execution candidate operation timestamp is required")
	}
	return nil
}

func CandidateRequestFingerprint(preparationID, manifestFingerprint, approvalID,
	requestedBy string,
) string {
	return fingerprint("sandbox_execution_candidate_request.v1", preparationID,
		manifestFingerprint, approvalID, requestedBy)
}
