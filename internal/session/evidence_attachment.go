package session

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const EvidenceAttachmentProtocolVersion = "session_evidence_attachment.v1"

type EvidenceAttachment struct {
	ID                 string
	ProtocolVersion    string
	OperationKeyDigest string
	RequestFingerprint string
	RunID              string
	SessionID          string
	WorkspaceID        string
	SourceKind         string
	SourceRef          string
	ContentSHA256      string
	SessionMessageID   int64
	AttachedBy         string
	EventSequence      int64
	CreatedAt          time.Time
}

func (a EvidenceAttachment) Validate() error {
	if a.ProtocolVersion != EvidenceAttachmentProtocolVersion ||
		!validSHA256(a.OperationKeyDigest) || !validSHA256(a.RequestFingerprint) ||
		!validSHA256(a.ContentSHA256) || a.SourceKind != SourceWorkspaceFile ||
		a.SessionMessageID <= 0 || a.EventSequence <= 0 || a.CreatedAt.IsZero() {
		return errors.New("evidence attachment protocol, digest, or durable binding is invalid")
	}
	for _, value := range []string{a.ID, a.RunID, a.SessionID, a.WorkspaceID, a.AttachedBy} {
		if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
			utf8.RuneCountInString(value) > 256 || strings.ContainsRune(value, 0) {
			return errors.New("evidence attachment identity is invalid")
		}
		for _, current := range value {
			if unicode.IsControl(current) {
				return errors.New("evidence attachment identity contains control characters")
			}
		}
	}
	if a.SourceRef == "" || a.SourceRef != strings.TrimSpace(a.SourceRef) ||
		!utf8.ValidString(a.SourceRef) || utf8.RuneCountInString(a.SourceRef) > MaxContextSourceRefRunes ||
		strings.ContainsRune(a.SourceRef, 0) || !validWorkspaceEvidenceRef(a.SourceRef) {
		return errors.New("evidence attachment source reference is invalid")
	}
	for _, current := range a.SourceRef {
		if unicode.IsControl(current) {
			return errors.New("evidence attachment source reference contains control characters")
		}
	}
	return nil
}

func validWorkspaceEvidenceRef(value string) bool {
	if value == "." || strings.HasPrefix(value, "/") ||
		strings.ContainsAny(value, `\:`) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
