package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
)

type SpecialistOperatorScheduleRequest struct {
	ID                string
	ApplicationID     string
	ProposalID        string
	RunID             string
	RootAgentID       string
	AgentIDs          []string
	MaxRounds         int
	PolicyFingerprint string
	RequestedBy       string
	CreatedAt         time.Time
}

func (r SpecialistOperatorScheduleRequest) Validate() error {
	for _, value := range []string{r.ID, r.ApplicationID, r.ProposalID, r.RunID,
		r.RootAgentID, r.RequestedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("specialist operator schedule identities are invalid")
		}
	}
	agentIDs, err := normalizeSpecialistScheduleAgentIDs(r.AgentIDs)
	if err != nil || !slices.Equal(agentIDs, r.AgentIDs) {
		return errors.New("specialist operator schedule Agent ids must be sorted and unique")
	}
	if r.MaxRounds <= 0 || r.MaxRounds > MaxSpecialistScheduleRounds ||
		!validLowerHexDigest(r.PolicyFingerprint) || r.CreatedAt.IsZero() {
		return errors.New("specialist operator schedule request is invalid")
	}
	return nil
}

type SpecialistOperatorScheduleOperation struct {
	KeyDigest          string
	RequestFingerprint string
	RequestID          string
	ApplicationID      string
	ProposalID         string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func (o SpecialistOperatorScheduleOperation) Validate() error {
	for _, value := range []string{o.RequestID, o.ApplicationID, o.ProposalID,
		o.RunID, o.RequestedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("specialist operator schedule operation identities are invalid")
		}
	}
	if !validLowerHexDigest(o.KeyDigest) ||
		!validLowerHexDigest(o.RequestFingerprint) || o.CreatedAt.IsZero() {
		return errors.New("specialist operator schedule operation is invalid")
	}
	return nil
}

type SpecialistOperatorScheduleAttempt struct {
	RequestID  string
	ScheduleID string
	Ordinal    int
	CreatedAt  time.Time
}

func (a SpecialistOperatorScheduleAttempt) Validate() error {
	for _, value := range []string{a.RequestID, a.ScheduleID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("specialist operator schedule attempt identities are invalid")
		}
	}
	if a.Ordinal <= 0 || a.CreatedAt.IsZero() {
		return errors.New("specialist operator schedule attempt is invalid")
	}
	return nil
}
