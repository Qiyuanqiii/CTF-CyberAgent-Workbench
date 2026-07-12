package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type SpecialistDelegationApplicationStatus string

const (
	SpecialistDelegationApplying SpecialistDelegationApplicationStatus = "applying"
	SpecialistDelegationApplied  SpecialistDelegationApplicationStatus = "applied"
	SpecialistDelegationAborted  SpecialistDelegationApplicationStatus = "aborted"
)

func (s SpecialistDelegationApplicationStatus) Valid() bool {
	return s == SpecialistDelegationApplying || s == SpecialistDelegationApplied ||
		s == SpecialistDelegationAborted
}

type SpecialistDelegationAssignmentApplicationStatus string

const (
	SpecialistDelegationAssignmentPending    SpecialistDelegationAssignmentApplicationStatus = "pending"
	SpecialistDelegationAssignmentAdmitted   SpecialistDelegationAssignmentApplicationStatus = "admitted"
	SpecialistDelegationAssignmentInstructed SpecialistDelegationAssignmentApplicationStatus = "instructed"
)

func (s SpecialistDelegationAssignmentApplicationStatus) Valid() bool {
	return s == SpecialistDelegationAssignmentPending ||
		s == SpecialistDelegationAssignmentAdmitted ||
		s == SpecialistDelegationAssignmentInstructed
}

type SpecialistDelegationPolicyCheck struct {
	Ordinal       int
	Allowed       bool
	NeedsApproval bool
	Risk          string
	Reason        string
}

func (c SpecialistDelegationPolicyCheck) Validate() error {
	if c.Ordinal <= 0 || c.Ordinal > MaxSpecialistDelegationAssignments {
		return errors.New("specialist delegation policy ordinal is invalid")
	}
	for label, value := range map[string]string{"risk": c.Risk, "reason": c.Reason} {
		if !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
			strings.ContainsRune(value, 0) || utf8.RuneCountInString(value) > 2048 ||
			len([]byte(value)) > 8*1024 {
			return fmt.Errorf("specialist delegation policy %s is invalid", label)
		}
	}
	if c.Reason == "" {
		return errors.New("specialist delegation policy reason is required")
	}
	return nil
}

func SpecialistDelegationPolicyFingerprint(
	checks []SpecialistDelegationPolicyCheck,
) (string, error) {
	if len(checks) == 0 || len(checks) > MaxSpecialistDelegationAssignments {
		return "", errors.New("specialist delegation policy checks are required")
	}
	normalized := slices.Clone(checks)
	for index := range normalized {
		normalized[index].Risk = strings.TrimSpace(normalized[index].Risk)
		normalized[index].Reason = strings.TrimSpace(normalized[index].Reason)
		if err := normalized[index].Validate(); err != nil {
			return "", err
		}
		if normalized[index].Ordinal != index+1 {
			return "", errors.New("specialist delegation policy checks must be contiguous and ordered")
		}
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(append([]byte("specialist_delegation_application_policy.v1\x00"), encoded...))
	return hex.EncodeToString(digest[:]), nil
}

type SpecialistDelegationApplicationAssignment struct {
	ApplicationID              string
	ProposalID                 string
	Ordinal                    int
	Status                     SpecialistDelegationAssignmentApplicationStatus
	AdmissionOperationDigest   string
	InstructionOperationDigest string
	AgentID                    string
	MessageID                  string
	Version                    int64
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

func (a SpecialistDelegationApplicationAssignment) Validate() error {
	for _, value := range []string{a.ApplicationID, a.ProposalID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("specialist delegation application assignment identities are invalid")
		}
	}
	if a.Ordinal <= 0 || a.Ordinal > MaxSpecialistDelegationAssignments || !a.Status.Valid() ||
		!validLowerHexDigest(a.AdmissionOperationDigest) ||
		!validLowerHexDigest(a.InstructionOperationDigest) {
		return errors.New("specialist delegation application assignment state is invalid")
	}
	if a.CreatedAt.IsZero() || a.UpdatedAt.Before(a.CreatedAt) {
		return errors.New("specialist delegation application assignment timestamps are invalid")
	}
	switch a.Status {
	case SpecialistDelegationAssignmentPending:
		if a.AgentID != "" || a.MessageID != "" || a.Version != 1 {
			return errors.New("pending specialist delegation assignment has result metadata")
		}
	case SpecialistDelegationAssignmentAdmitted:
		if !validAgentIdentity(a.AgentID, false) || a.MessageID != "" || a.Version != 2 {
			return errors.New("admitted specialist delegation assignment is invalid")
		}
	case SpecialistDelegationAssignmentInstructed:
		if !validAgentIdentity(a.AgentID, false) || !validAgentIdentity(a.MessageID, false) ||
			a.Version != 3 {
			return errors.New("instructed specialist delegation assignment is invalid")
		}
	}
	return nil
}

type SpecialistDelegationApplication struct {
	ID                string
	ReviewID          string
	ProposalID        string
	RunID             string
	RootAgentID       string
	Status            SpecialistDelegationApplicationStatus
	AssignmentCount   int
	PolicyFingerprint string
	MaxChildren       int
	MaxTurnsPerChild  int64
	MaxTokensPerChild int64
	RequestedBy       string
	StopCode          string
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	CompletedAt       *time.Time
	Assignments       []SpecialistDelegationApplicationAssignment
}

func (a SpecialistDelegationApplication) Validate() error {
	for _, value := range []string{a.ID, a.ReviewID, a.ProposalID, a.RunID,
		a.RootAgentID, a.RequestedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("specialist delegation application identities are invalid")
		}
	}
	if !a.Status.Valid() || a.AssignmentCount <= 0 ||
		a.AssignmentCount > MaxSpecialistDelegationAssignments ||
		!validLowerHexDigest(a.PolicyFingerprint) || a.MaxChildren <= 0 ||
		a.MaxChildren > MaxAgentChildren || a.MaxTurnsPerChild <= 0 ||
		a.MaxTokensPerChild <= 0 || a.MaxTokensPerChild > MaxAgentTokenReservation {
		return errors.New("specialist delegation application policy or state is invalid")
	}
	if a.CreatedAt.IsZero() || a.UpdatedAt.Before(a.CreatedAt) {
		return errors.New("specialist delegation application timestamps are invalid")
	}
	if len(a.Assignments) != a.AssignmentCount {
		return errors.New("specialist delegation application assignment count is inconsistent")
	}
	allInstructed := true
	latestAssignmentUpdate := a.CreatedAt
	for index, assignment := range a.Assignments {
		if err := assignment.Validate(); err != nil {
			return err
		}
		if assignment.ApplicationID != a.ID || assignment.ProposalID != a.ProposalID ||
			assignment.Ordinal != index+1 {
			return errors.New("specialist delegation application assignments are not contiguous")
		}
		allInstructed = allInstructed &&
			assignment.Status == SpecialistDelegationAssignmentInstructed
		if assignment.UpdatedAt.After(latestAssignmentUpdate) {
			latestAssignmentUpdate = assignment.UpdatedAt
		}
	}
	switch a.Status {
	case SpecialistDelegationApplying:
		if a.Version != 1 || a.CompletedAt != nil || a.StopCode != "" {
			return errors.New("applying specialist delegation has terminal metadata")
		}
	case SpecialistDelegationApplied:
		if a.Version != 2 || a.CompletedAt == nil || a.StopCode != "" || !allInstructed {
			return errors.New("applied specialist delegation is incomplete")
		}
	case SpecialistDelegationAborted:
		if a.Version != 2 || a.CompletedAt == nil || strings.TrimSpace(a.StopCode) == "" {
			return errors.New("aborted specialist delegation requires terminal metadata")
		}
	}
	if a.CompletedAt != nil && (a.CompletedAt.Before(a.CreatedAt) ||
		a.CompletedAt.Before(latestAssignmentUpdate) ||
		a.UpdatedAt.Before(*a.CompletedAt) || a.UpdatedAt.After(*a.CompletedAt)) {
		return errors.New("specialist delegation application completion time is invalid")
	}
	return nil
}

type SpecialistDelegationApplicationOperation struct {
	KeyDigest          string
	RequestFingerprint string
	ApplicationID      string
	ReviewID           string
	ProposalID         string
	RunID              string
	RequestedBy        string
	CreatedAt          time.Time
}

func (o SpecialistDelegationApplicationOperation) Validate() error {
	for _, value := range []string{o.ApplicationID, o.ReviewID, o.ProposalID, o.RunID,
		o.RequestedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("specialist delegation application operation identities are invalid")
		}
	}
	if !validLowerHexDigest(o.KeyDigest) || !validLowerHexDigest(o.RequestFingerprint) ||
		o.CreatedAt.IsZero() {
		return errors.New("specialist delegation application operation is invalid")
	}
	return nil
}

func SpecialistDelegationAdmissionOperationKey(applicationID string, ordinal int) (string, error) {
	return specialistDelegationApplicationOperationKey(applicationID, ordinal, "admit")
}

func SpecialistDelegationInstructionOperationKey(applicationID string, ordinal int) (string, error) {
	return specialistDelegationApplicationOperationKey(applicationID, ordinal, "instruction")
}

func specialistDelegationApplicationOperationKey(applicationID string, ordinal int,
	kind string,
) (string, error) {
	applicationID = strings.TrimSpace(applicationID)
	if !validAgentIdentity(applicationID, false) || ordinal <= 0 ||
		ordinal > MaxSpecialistDelegationAssignments {
		return "", errors.New("specialist delegation application operation scope is invalid")
	}
	key := fmt.Sprintf("delegation-application:%s:%d:%s", applicationID, ordinal, kind)
	return NormalizeAgentOperationKey(key)
}
