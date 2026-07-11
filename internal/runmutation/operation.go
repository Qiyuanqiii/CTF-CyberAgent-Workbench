package runmutation

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxIdentityRunes = 256

type TargetKind string

const (
	TargetWorkItem TargetKind = "work_item"
	TargetNote     TargetKind = "note"
)

func (k TargetKind) Valid() bool {
	return k == TargetWorkItem || k == TargetNote
}

type Operation struct {
	KeyDigest          string
	RequestFingerprint string
	InvocationID       string
	RunID              string
	SessionID          string
	WorkspaceID        string
	ToolName           string
	TargetKind         TargetKind
	TargetID           string
	RequestedBy        string
	CreatedAt          time.Time
}

func (o Operation) Validate() error {
	for label, value := range map[string]string{
		"invocation id": o.InvocationID, "run id": o.RunID, "session id": o.SessionID,
		"workspace id": o.WorkspaceID, "tool name": o.ToolName, "target id": o.TargetID,
		"requester": o.RequestedBy,
	} {
		if !utf8.ValidString(value) || strings.TrimSpace(value) != value || len([]rune(value)) > MaxIdentityRunes {
			return fmt.Errorf("structured mutation %s must be normalized and bounded UTF-8", label)
		}
	}
	if o.InvocationID == "" || o.RunID == "" || o.SessionID == "" || o.ToolName == "" ||
		o.TargetID == "" || o.RequestedBy == "" {
		return errors.New("structured mutation invocation, Run, Session, tool, target, and requester are required")
	}
	if !validDigest(o.KeyDigest) || !validDigest(o.RequestFingerprint) {
		return errors.New("structured mutation key and request fingerprints must be lowercase SHA-256 digests")
	}
	if !o.TargetKind.Valid() {
		return fmt.Errorf("invalid structured mutation target kind %q", o.TargetKind)
	}
	if o.CreatedAt.IsZero() {
		return errors.New("structured mutation creation time is required")
	}
	return nil
}

func OperationKeyDigest(toolName string, runID string, operationKey string) string {
	return Fingerprint("structured_tool_operation.v1", toolName, runID, operationKey)
}

func Fingerprint(parts ...string) string {
	hash := sha256.New()
	var size [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(size[:], uint64(len([]byte(part))))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
