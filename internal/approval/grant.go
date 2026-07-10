package approval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type GrantStatus string

const (
	GrantActive  GrantStatus = "active"
	GrantRevoked GrantStatus = "revoked"
)

func (s GrantStatus) Valid() bool {
	return s == GrantActive || s == GrantRevoked
}

type SessionGrant struct {
	ID                 string
	RunID              string
	SessionID          string
	WorkspaceID        string
	ToolName           string
	ActionClass        string
	Status             GrantStatus
	RequestFingerprint string
	Reason             string
	RevocationReason   string
	GrantedBy          string
	RevokedBy          string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	RevokedAt          *time.Time
}

func (g SessionGrant) Validate() error {
	for label, value := range map[string]string{
		"id": g.ID, "run id": g.RunID, "session id": g.SessionID, "workspace id": g.WorkspaceID,
		"tool name": g.ToolName, "action class": g.ActionClass, "granted by": g.GrantedBy,
		"revoked by": g.RevokedBy,
	} {
		if strings.TrimSpace(value) != value || !utf8.ValidString(value) || len([]rune(value)) > MaxIdentityRunes {
			return fmt.Errorf("approval grant %s must be normalized and bounded UTF-8", label)
		}
	}
	if g.ID == "" || g.RunID == "" || g.SessionID == "" || g.ToolName == "" || g.ActionClass == "" || g.GrantedBy == "" {
		return errors.New("approval grant identity, run, session, tool, action class, and grantor are required")
	}
	if !g.Status.Valid() {
		return fmt.Errorf("invalid approval grant status %q", g.Status)
	}
	if !validFingerprint(g.RequestFingerprint) {
		return errors.New("approval grant request fingerprint must be a SHA-256 hex digest")
	}
	if !utf8.ValidString(g.Reason) || len([]rune(g.Reason)) > MaxReasonRunes ||
		!utf8.ValidString(g.RevocationReason) || len([]rune(g.RevocationReason)) > MaxReasonRunes {
		return fmt.Errorf("approval grant reason exceeds %d characters", MaxReasonRunes)
	}
	if g.Version <= 0 || g.CreatedAt.IsZero() || g.UpdatedAt.IsZero() || g.UpdatedAt.Before(g.CreatedAt) {
		return errors.New("approval grant version and timestamps are invalid")
	}
	if g.Status == GrantActive {
		if g.RevokedAt != nil || g.RevokedBy != "" || g.RevocationReason != "" {
			return errors.New("active approval grant cannot have revocation metadata")
		}
	} else if g.RevokedAt == nil || g.RevokedAt.Before(g.CreatedAt) || g.RevokedBy == "" {
		return errors.New("revoked approval grant requires revoker and revocation time")
	}
	return nil
}

type CreateGrantRequest struct {
	SessionID      string
	WorkspaceID    string
	ToolName       string
	ActionClass    string
	Reason         string
	GrantedBy      string
	IdempotencyKey string
}

func (r CreateGrantRequest) Normalize() (CreateGrantRequest, error) {
	r.SessionID = strings.TrimSpace(r.SessionID)
	r.WorkspaceID = strings.TrimSpace(r.WorkspaceID)
	r.ToolName = strings.TrimSpace(r.ToolName)
	r.ActionClass = strings.TrimSpace(r.ActionClass)
	r.Reason = strings.TrimSpace(r.Reason)
	r.GrantedBy = strings.TrimSpace(r.GrantedBy)
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	for label, value := range map[string]string{
		"session id": r.SessionID, "workspace id": r.WorkspaceID, "tool name": r.ToolName,
		"action class": r.ActionClass, "grantor": r.GrantedBy, "idempotency key": r.IdempotencyKey,
	} {
		if !utf8.ValidString(value) || len([]rune(value)) > MaxIdentityRunes {
			return CreateGrantRequest{}, fmt.Errorf("approval grant %s must be bounded UTF-8", label)
		}
	}
	if r.SessionID == "" || r.ToolName == "" || r.ActionClass == "" || r.GrantedBy == "" || r.IdempotencyKey == "" {
		return CreateGrantRequest{}, errors.New("approval grant session, tool, action class, grantor, and idempotency key are required")
	}
	if !utf8.ValidString(r.Reason) || len([]rune(r.Reason)) > MaxReasonRunes {
		return CreateGrantRequest{}, fmt.Errorf("approval grant reason exceeds %d characters", MaxReasonRunes)
	}
	return r, nil
}

type RevokeGrantRequest struct {
	GrantID        string
	Reason         string
	RevokedBy      string
	IdempotencyKey string
}

func (r RevokeGrantRequest) Normalize() (RevokeGrantRequest, error) {
	r.GrantID = strings.TrimSpace(r.GrantID)
	r.Reason = strings.TrimSpace(r.Reason)
	r.RevokedBy = strings.TrimSpace(r.RevokedBy)
	r.IdempotencyKey = strings.TrimSpace(r.IdempotencyKey)
	for label, value := range map[string]string{
		"grant id": r.GrantID, "revoker": r.RevokedBy, "idempotency key": r.IdempotencyKey,
	} {
		if value == "" || !utf8.ValidString(value) || len([]rune(value)) > MaxIdentityRunes {
			return RevokeGrantRequest{}, fmt.Errorf("approval grant %s is required and must be bounded UTF-8", label)
		}
	}
	if !utf8.ValidString(r.Reason) || len([]rune(r.Reason)) > MaxReasonRunes {
		return RevokeGrantRequest{}, fmt.Errorf("approval grant reason exceeds %d characters", MaxReasonRunes)
	}
	return r, nil
}

type GrantResult struct {
	Grant    SessionGrant
	Replayed bool
}

type GrantQuery struct {
	RunID       string
	SessionID   string
	WorkspaceID string
	ToolName    string
	ActionClass string
}

type GrantListFilter struct {
	RunID     string
	SessionID string
	ToolName  string
	Status    GrantStatus
	Limit     int
}

type GrantStore interface {
	CreateSessionGrant(ctx context.Context, request CreateGrantRequest) (GrantResult, error)
	RevokeSessionGrant(ctx context.Context, request RevokeGrantRequest) (GrantResult, error)
	AuthorizeApprovalWithSessionGrant(ctx context.Context, proposalID string, grantID string) (DecisionResult, error)
	FindActiveSessionGrant(ctx context.Context, query GrantQuery) (SessionGrant, bool, error)
	GetSessionGrant(ctx context.Context, id string) (SessionGrant, error)
	ListSessionGrants(ctx context.Context, filter GrantListFilter) ([]SessionGrant, error)
}

func GrantRequestFingerprint(request CreateGrantRequest) string {
	return Fingerprint("session_grant.v1", request.SessionID, request.WorkspaceID, request.ToolName,
		request.ActionClass, request.Reason, request.GrantedBy)
}

func GrantRevocationFingerprint(request RevokeGrantRequest) string {
	return Fingerprint("session_grant_revoke.v1", request.GrantID, request.Reason, request.RevokedBy)
}

func GrantOperationKeyDigest(idempotencyKey string) string {
	return Fingerprint("approval_grant_operation_key.v1", strings.TrimSpace(idempotencyKey))
}
