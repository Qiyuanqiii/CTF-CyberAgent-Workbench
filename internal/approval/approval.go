package approval

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxIdentityRunes = 256
	MaxReasonRunes   = 2048
)

type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDenied   Status = "denied"
)

func (s Status) Valid() bool {
	switch s {
	case StatusPending, StatusApproved, StatusDenied:
		return true
	default:
		return false
	}
}

type Action string

const (
	ActionApprove Action = "approve"
	ActionDeny    Action = "deny"
)

func (a Action) Status() (Status, bool) {
	switch a {
	case ActionApprove:
		return StatusApproved, true
	case ActionDeny:
		return StatusDenied, true
	default:
		return "", false
	}
}

type Record struct {
	ID                 string
	IdempotencyKey     string
	ProposalID         string
	RunID              string
	SessionID          string
	WorkspaceID        string
	ToolName           string
	ActionClass        string
	Mode               string
	Status             Status
	RequestFingerprint string
	DecisionReason     string
	RequestedBy        string
	ReviewedBy         string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DecidedAt          *time.Time
}

func (r Record) Validate() error {
	for label, value := range map[string]string{
		"id": r.ID, "idempotency key": r.IdempotencyKey, "proposal id": r.ProposalID,
		"run id": r.RunID, "session id": r.SessionID, "workspace id": r.WorkspaceID,
		"tool name": r.ToolName, "action class": r.ActionClass, "mode": r.Mode,
		"requested by": r.RequestedBy, "reviewed by": r.ReviewedBy,
	} {
		if strings.TrimSpace(value) != value || !utf8.ValidString(value) || len([]rune(value)) > MaxIdentityRunes {
			return fmt.Errorf("approval %s must be normalized and bounded UTF-8", label)
		}
	}
	if r.ID == "" || r.IdempotencyKey == "" || r.ProposalID == "" || r.ToolName == "" || r.ActionClass == "" || r.Mode == "" {
		return errors.New("approval identity, proposal, tool, action class, and mode are required")
	}
	if !r.Status.Valid() {
		return fmt.Errorf("invalid approval status %q", r.Status)
	}
	if !validFingerprint(r.RequestFingerprint) {
		return errors.New("approval request fingerprint must be a SHA-256 hex digest")
	}
	if !utf8.ValidString(r.DecisionReason) || len([]rune(r.DecisionReason)) > MaxReasonRunes {
		return fmt.Errorf("approval decision reason exceeds %d characters", MaxReasonRunes)
	}
	if r.Version <= 0 || r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) {
		return errors.New("approval version and timestamps are invalid")
	}
	if r.Status == StatusPending {
		if r.DecidedAt != nil || r.ReviewedBy != "" {
			return errors.New("pending approval cannot have decision metadata")
		}
	} else if r.DecidedAt == nil || r.DecidedAt.Before(r.CreatedAt) || r.ReviewedBy == "" {
		return errors.New("decided approval requires reviewer and decision time")
	}
	return nil
}

type Proposal struct {
	IdempotencyKey     string
	ProposalID         string
	SessionID          string
	WorkspaceID        string
	ToolName           string
	ActionClass        string
	Mode               string
	Status             Status
	RequestFingerprint string
	DecisionReason     string
	RequestedBy        string
	ReviewedBy         string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DecidedAt          *time.Time
}

type DecisionRequest struct {
	ProposalID     string
	IdempotencyKey string
	Action         Action
	Reason         string
	ReviewedBy     string
}

func (r DecisionRequest) Normalize() (DecisionRequest, error) {
	r.ProposalID = strings.TrimSpace(r.ProposalID)
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	r.Action = Action(strings.TrimSpace(string(r.Action)))
	r.Reason = strings.TrimSpace(r.Reason)
	r.ReviewedBy = strings.TrimSpace(r.ReviewedBy)
	if r.ProposalID == "" || r.IdempotencyKey == "" || r.ReviewedBy == "" {
		return DecisionRequest{}, errors.New("approval proposal, idempotency key, and reviewer are required")
	}
	if _, ok := r.Action.Status(); !ok {
		return DecisionRequest{}, fmt.Errorf("invalid approval action %q", r.Action)
	}
	for label, value := range map[string]string{
		"proposal id": r.ProposalID, "idempotency key": r.IdempotencyKey, "reviewer": r.ReviewedBy,
	} {
		if !utf8.ValidString(value) || len([]rune(value)) > MaxIdentityRunes {
			return DecisionRequest{}, fmt.Errorf("approval %s must be bounded UTF-8", label)
		}
	}
	if !utf8.ValidString(r.Reason) || len([]rune(r.Reason)) > MaxReasonRunes {
		return DecisionRequest{}, fmt.Errorf("approval reason exceeds %d characters", MaxReasonRunes)
	}
	if r.Action == ActionApprove && r.Reason != "" {
		return DecisionRequest{}, errors.New("approval cannot include a denial reason")
	}
	return r, nil
}

type DecisionResult struct {
	Approval Record
	Replayed bool
}

type ListFilter struct {
	RunID     string
	SessionID string
	Status    Status
	ToolName  string
	Limit     int
}

type Store interface {
	EnsureApproval(ctx context.Context, proposal Proposal) (Record, error)
	DecideApproval(ctx context.Context, request DecisionRequest) (DecisionResult, error)
	GetApproval(ctx context.Context, id string) (Record, error)
	GetApprovalByProposal(ctx context.Context, proposalID string) (Record, error)
	ListApprovals(ctx context.Context, filter ListFilter) ([]Record, error)
}

func ProposalIdempotencyKey(toolName string, proposalID string) string {
	return "proposal:" + strings.TrimSpace(toolName) + ":" + strings.TrimSpace(proposalID)
}

func ReviewIdempotencyKey(toolName string, proposalID string, action Action) string {
	return "review:" + strings.TrimSpace(toolName) + ":" + strings.TrimSpace(proposalID) + ":" + string(action)
}

func ShellFingerprint(sessionID string, workspaceID string, command string) string {
	return Fingerprint("shell", sessionID, workspaceID, command)
}

func FileEditFingerprint(sessionID string, workspaceID string, path string, proposedHash string) string {
	return Fingerprint("replace_file", sessionID, workspaceID, path, proposedHash)
}

func DecisionFingerprint(request DecisionRequest) string {
	return Fingerprint(request.ProposalID, string(request.Action), request.Reason, request.ReviewedBy)
}

func OperationKeyDigest(idempotencyKey string) string {
	return Fingerprint("approval_operation_key.v1", strings.TrimSpace(idempotencyKey))
}

func Fingerprint(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		value := []byte(part)
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = hash.Write(value)
		_, _ = hash.Write([]byte{'|'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validFingerprint(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
