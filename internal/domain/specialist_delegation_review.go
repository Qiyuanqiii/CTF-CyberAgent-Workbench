package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxSpecialistDelegationReviewReasonRunes = 2048
	MaxSpecialistDelegationReviewReasonBytes = 8 * 1024
)

type SpecialistDelegationReviewDecision string

const (
	SpecialistDelegationApproved SpecialistDelegationReviewDecision = "approved"
	SpecialistDelegationRejected SpecialistDelegationReviewDecision = "rejected"
)

func (d SpecialistDelegationReviewDecision) Valid() bool {
	return d == SpecialistDelegationApproved || d == SpecialistDelegationRejected
}

type SpecialistDelegationReview struct {
	ID          string
	ProposalID  string
	RunID       string
	RootAgentID string
	Decision    SpecialistDelegationReviewDecision
	Reason      string
	ReviewedBy  string
	Version     int64
	CreatedAt   time.Time
}

func (r SpecialistDelegationReview) Validate() error {
	for _, value := range []string{r.ID, r.ProposalID, r.RunID, r.RootAgentID, r.ReviewedBy} {
		if !validAgentIdentity(value, false) {
			return errors.New("specialist delegation review identities are required and normalized")
		}
	}
	if !r.Decision.Valid() {
		return fmt.Errorf("invalid specialist delegation review decision %q", r.Decision)
	}
	if !utf8.ValidString(r.Reason) || strings.TrimSpace(r.Reason) != r.Reason ||
		strings.ContainsRune(r.Reason, 0) ||
		utf8.RuneCountInString(r.Reason) > MaxSpecialistDelegationReviewReasonRunes ||
		len([]byte(r.Reason)) > MaxSpecialistDelegationReviewReasonBytes {
		return fmt.Errorf("specialist delegation review reason must be normalized and within %d characters and %d bytes",
			MaxSpecialistDelegationReviewReasonRunes, MaxSpecialistDelegationReviewReasonBytes)
	}
	if r.Decision == SpecialistDelegationRejected && r.Reason == "" {
		return errors.New("rejected specialist delegation review requires a reason")
	}
	if r.Version != 1 || r.CreatedAt.IsZero() {
		return errors.New("specialist delegation review version and creation time are required")
	}
	return nil
}

type SpecialistDelegationReviewOperation struct {
	KeyDigest          string
	RequestFingerprint string
	ReviewID           string
	ProposalID         string
	RunID              string
	ReviewedBy         string
	CreatedAt          time.Time
}

func (o SpecialistDelegationReviewOperation) Validate() error {
	for _, value := range []string{o.ReviewID, o.ProposalID, o.RunID, o.ReviewedBy} {
		if !validAgentIdentity(value, false) {
			return errors.New("specialist delegation review operation identities are required and normalized")
		}
	}
	if !validLowerHexDigest(o.KeyDigest) || !validLowerHexDigest(o.RequestFingerprint) {
		return errors.New("specialist delegation review operation digest is invalid")
	}
	if o.CreatedAt.IsZero() {
		return errors.New("specialist delegation review operation creation time is required")
	}
	return nil
}
