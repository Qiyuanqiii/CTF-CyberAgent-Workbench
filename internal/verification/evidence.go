package verification

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	EvidenceProtocolVersion  = "operator_verification_evidence.v1"
	InventoryProtocolVersion = "operator_verification_inventory.v1"
	MaxTitleRunes            = 160
	MaxSummaryRunes          = 2048
	MaxInventoryItems        = 100
)

type Outcome string

const (
	OutcomePass    Outcome = "pass"
	OutcomeFail    Outcome = "fail"
	OutcomeUnknown Outcome = "unknown"
)

func (o Outcome) Valid() bool {
	switch o {
	case OutcomePass, OutcomeFail, OutcomeUnknown:
		return true
	default:
		return false
	}
}

// Evidence is one immutable operator observation. It records no command,
// model claim, approval, or execution capability.
type Evidence struct {
	ID                 string
	ProtocolVersion    string
	OperationKeyDigest string
	RequestFingerprint string
	RunID              string
	SessionID          string
	WorkspaceID        string
	Outcome            Outcome
	Title              string
	Summary            string
	SummarySHA256      string
	Redacted           bool
	RecordedBy         string
	EventSequence      int64
	CreatedAt          time.Time
}

func (e Evidence) Validate() error {
	if e.ProtocolVersion != EvidenceProtocolVersion || !e.Outcome.Valid() ||
		!validDigest(e.OperationKeyDigest) || !validDigest(e.RequestFingerprint) ||
		!validDigest(e.SummarySHA256) || e.SummarySHA256 != SummaryDigest(e.Summary) ||
		e.EventSequence <= 0 || e.CreatedAt.IsZero() {
		return errors.New("verification evidence protocol, outcome, digest, or durable binding is invalid")
	}
	for _, value := range []string{e.ID, e.RunID, e.SessionID, e.WorkspaceID, e.RecordedBy} {
		if !validIdentity(value) {
			return errors.New("verification evidence identity is invalid")
		}
	}
	if err := ValidateText(e.Title, MaxTitleRunes, false); err != nil {
		return errors.New("verification evidence title is invalid")
	}
	if err := ValidateText(e.Summary, MaxSummaryRunes, true); err != nil {
		return errors.New("verification evidence summary is invalid")
	}
	return nil
}

func ValidateText(value string, maxRunes int, multiline bool) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > maxRunes || strings.ContainsRune(value, 0) {
		return errors.New("verification evidence text is not normalized")
	}
	for _, current := range value {
		if unicode.IsControl(current) && (!multiline ||
			(current != '\n' && current != '\t')) {
			return errors.New("verification evidence text contains a forbidden control character")
		}
	}
	return nil
}

func SummaryDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validIdentity(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > 256 || strings.ContainsRune(value, 0) {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return false
		}
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
