package fileedit

import (
	"errors"
	"strings"
	"time"
)

const FileEditApplyProtocolVersion = "file_edit_apply.v1"

type ApplyStatus string

const (
	ApplyCompleted ApplyStatus = "applied"
	ApplyFailed    ApplyStatus = "failed"
)

type ApplyOperation struct {
	ProtocolVersion    string
	KeyDigest          string
	RequestFingerprint string
	RunID              string
	SessionID          string
	WorkspaceID        string
	EditID             string
	Path               string
	OriginalHash       string
	ProposedHash       string
	ObservedHash       string
	AppliedBy          string
	EventSequence      int64
	CreatedAt          time.Time
}

func (o ApplyOperation) Validate() error {
	if o.ProtocolVersion != FileEditApplyProtocolVersion || !validDigest(o.KeyDigest) ||
		!validDigest(o.RequestFingerprint) || !validDigest(o.ProposedHash) ||
		(o.OriginalHash != missingHash && !validDigest(o.OriginalHash)) ||
		(o.ObservedHash != o.OriginalHash && o.ObservedHash != o.ProposedHash) ||
		o.EventSequence <= 0 || o.CreatedAt.IsZero() {
		return errors.New("FileEdit apply operation metadata is invalid")
	}
	for _, value := range []string{o.RunID, o.SessionID, o.WorkspaceID, o.EditID,
		o.Path, o.AppliedBy} {
		if value == "" || value != strings.TrimSpace(value) || len(value) > 256 ||
			strings.ContainsRune(value, 0) {
			return errors.New("FileEdit apply operation identity is invalid")
		}
	}
	return nil
}

type ApplyResult struct {
	OperationKeyDigest string
	Status             ApplyStatus
	ReasonCode         string
	EventSequence      int64
	CompletedAt        time.Time
}

func (r ApplyResult) Validate() error {
	if !validDigest(r.OperationKeyDigest) ||
		(r.Status != ApplyCompleted && r.Status != ApplyFailed) ||
		r.EventSequence <= 0 || r.CompletedAt.IsZero() ||
		r.ReasonCode != strings.TrimSpace(r.ReasonCode) || len(r.ReasonCode) > 64 ||
		strings.ContainsRune(r.ReasonCode, 0) ||
		(r.Status == ApplyCompleted && r.ReasonCode != "") ||
		(r.Status == ApplyFailed && r.ReasonCode == "") {
		return errors.New("FileEdit apply result is invalid")
	}
	return nil
}

func validDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	for _, current := range value {
		if (current < '0' || current > '9') && (current < 'a' || current > 'f') {
			return false
		}
	}
	return true
}
