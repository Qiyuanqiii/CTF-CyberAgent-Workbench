package operationreceipt

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxHistoryItems = 100

// TerminalRecord is an internal Store-to-Application projection. SourceID,
// paths, and content digests must never cross the HTTP boundary.
type TerminalRecord struct {
	SourceID     string
	Kind         Kind
	RunID        string
	WorkspaceID  string
	Path         string
	ProposedHash string
	Outcome      string
	CompletedAt  time.Time
}

func (r TerminalRecord) Validate() error {
	if !validHistoryIdentity(r.SourceID) ||
		r.CompletedAt.IsZero() {
		return errors.New("terminal operation source identity and completion time are required")
	}
	switch r.Kind {
	case KindFileEditApply:
		if !validHistoryIdentity(r.RunID) || !validHistoryIdentity(r.WorkspaceID) ||
			!validHistoryPath(r.Path) || !validHistoryDigest(r.ProposedHash) ||
			(r.Outcome != "applied" && r.Outcome != "failed") {
			return errors.New("terminal FileEdit receipt source is invalid")
		}
	case KindRunWakeConsume:
		if !validHistoryIdentity(r.RunID) || r.WorkspaceID != "" || r.Path != "" ||
			r.ProposedHash != "" || (r.Outcome != "completed" && r.Outcome != "failed") {
			return errors.New("terminal Run wake receipt source is invalid")
		}
	case KindSkillPackageInstall:
		if r.RunID != "" || r.WorkspaceID != "" || r.Path != "" ||
			r.ProposedHash != "" || r.Outcome != "installed" {
			return errors.New("terminal Skill installation receipt source is invalid")
		}
	default:
		return errors.New("terminal operation receipt kind is invalid")
	}
	return nil
}

func validHistoryIdentity(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		utf8.RuneCountInString(value) > 256 {
		return false
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return false
		}
	}
	return true
}

func validHistoryPath(value string) bool {
	if value == "" || value == "." || value != strings.TrimSpace(value) ||
		!utf8.ValidString(value) || utf8.RuneCountInString(value) > 512 ||
		strings.HasPrefix(value, "/") || strings.ContainsAny(value, `\:`) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
		for _, current := range part {
			if unicode.IsControl(current) {
				return false
			}
		}
	}
	return true
}

func validHistoryDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

type HistoryItem struct {
	ID          string
	Scope       string
	RunID       string
	CompletedAt time.Time
	Receipt     Receipt
}

type History struct {
	ProtocolVersion string
	Items           []HistoryItem
	Truncated       bool
}
