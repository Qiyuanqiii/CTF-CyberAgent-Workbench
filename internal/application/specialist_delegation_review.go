package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
)

type SpecialistDelegationReviewStore interface {
	GetRun(ctx context.Context, id string) (domain.Run, error)
	GetSpecialistDelegationProposal(ctx context.Context,
		id string) (domain.SpecialistDelegationProposal, error)
	CreateSpecialistDelegationReview(ctx context.Context,
		operation domain.SpecialistDelegationReviewOperation,
		review domain.SpecialistDelegationReview,
		reviewEvent events.Event) (domain.SpecialistDelegationReview, bool, error)
}

type SpecialistDelegationReviewService struct {
	store SpecialistDelegationReviewStore
}

func NewSpecialistDelegationReviewService(
	store SpecialistDelegationReviewStore,
) *SpecialistDelegationReviewService {
	return &SpecialistDelegationReviewService{store: store}
}

type ReviewSpecialistDelegationRequest struct {
	ProposalID   string
	OperationKey string
	Decision     domain.SpecialistDelegationReviewDecision
	Reason       string
	ReviewedBy   string
}

type ReviewSpecialistDelegationResult struct {
	Review   domain.SpecialistDelegationReview
	Replayed bool
}

func (s *SpecialistDelegationReviewService) Review(ctx context.Context,
	request ReviewSpecialistDelegationRequest,
) (ReviewSpecialistDelegationResult, error) {
	if s == nil || s.store == nil {
		return ReviewSpecialistDelegationResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "specialist delegation review store is required")
	}
	normalized, err := normalizeSpecialistDelegationReviewRequest(request)
	if err != nil {
		return ReviewSpecialistDelegationResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "specialist delegation review request is invalid", err)
	}
	proposal, err := s.store.GetSpecialistDelegationProposal(ctx, normalized.ProposalID)
	if err != nil {
		return ReviewSpecialistDelegationResult{}, apperror.Normalize(err)
	}
	run, err := s.store.GetRun(ctx, proposal.RunID)
	if err != nil {
		return ReviewSpecialistDelegationResult{}, apperror.Normalize(err)
	}
	now := time.Now().UTC()
	if now.Before(proposal.CreatedAt) {
		now = proposal.CreatedAt
	}
	review := domain.SpecialistDelegationReview{
		ID: idgen.New("delegation-review"), ProposalID: proposal.ID, RunID: proposal.RunID,
		RootAgentID: proposal.RootAgentID, Decision: normalized.Decision,
		Reason: normalized.Reason, ReviewedBy: normalized.ReviewedBy,
		Version: 1, CreatedAt: now,
	}
	fingerprint, err := specialistDelegationReviewFingerprint(normalized, proposal)
	if err != nil {
		return ReviewSpecialistDelegationResult{}, err
	}
	operation := domain.SpecialistDelegationReviewOperation{
		KeyDigest: runmutation.OperationKeyDigest("specialist_delegation_review", proposal.RunID,
			normalized.OperationKey),
		RequestFingerprint: fingerprint, ReviewID: review.ID,
		ProposalID: proposal.ID, RunID: proposal.RunID,
		ReviewedBy: normalized.ReviewedBy, CreatedAt: now,
	}
	reviewEvent, err := events.New(run.ID, run.MissionID, events.AgentDelegationReviewedEvent,
		"operator", review.ID, map[string]any{
			"review_id": review.ID, "proposal_id": proposal.ID,
			"root_agent_id": proposal.RootAgentID, "decision": review.Decision,
			"reviewed_by": review.ReviewedBy, "review_version": review.Version,
			"admission_authorized": false, "application_required": true,
		})
	if err != nil {
		return ReviewSpecialistDelegationResult{}, err
	}
	reviewEvent.CreatedAt = now
	stored, replayed, err := s.store.CreateSpecialistDelegationReview(ctx, operation,
		review, reviewEvent)
	if err != nil {
		return ReviewSpecialistDelegationResult{}, apperror.Normalize(err)
	}
	return ReviewSpecialistDelegationResult{Review: stored, Replayed: replayed}, nil
}

func normalizeSpecialistDelegationReviewRequest(
	request ReviewSpecialistDelegationRequest,
) (ReviewSpecialistDelegationRequest, error) {
	originalKey := request.OperationKey
	request.ProposalID = strings.TrimSpace(request.ProposalID)
	request.Decision = domain.SpecialistDelegationReviewDecision(
		strings.TrimSpace(string(request.Decision)))
	request.Reason = strings.TrimSpace(redact.String(request.Reason))
	request.ReviewedBy = strings.TrimSpace(redact.String(request.ReviewedBy))
	if !domain.ValidAgentID(request.ProposalID) || !domain.ValidAgentID(request.ReviewedBy) ||
		strings.ContainsRune(request.ProposalID, 0) || strings.ContainsRune(request.ReviewedBy, 0) {
		return ReviewSpecialistDelegationRequest{}, errors.New("proposal and reviewer identities are required")
	}
	if !request.Decision.Valid() {
		return ReviewSpecialistDelegationRequest{}, fmt.Errorf(
			"invalid specialist delegation review decision %q", request.Decision)
	}
	if request.OperationKey != strings.TrimSpace(originalKey) ||
		!utf8.ValidString(request.OperationKey) ||
		len([]byte(request.OperationKey)) < domain.MinAgentOperationKeyBytes ||
		len([]byte(request.OperationKey)) > domain.MaxAgentOperationKeyBytes {
		return ReviewSpecialistDelegationRequest{}, fmt.Errorf(
			"specialist delegation review operation key must be normalized UTF-8 between %d and %d bytes",
			domain.MinAgentOperationKeyBytes, domain.MaxAgentOperationKeyBytes)
	}
	for _, current := range request.OperationKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return ReviewSpecialistDelegationRequest{}, errors.New(
				"specialist delegation review operation key cannot contain whitespace or control characters")
		}
	}
	validation := domain.SpecialistDelegationReview{
		ID: "review-validation", ProposalID: request.ProposalID, RunID: "run-validation",
		RootAgentID: "agent-validation", Decision: request.Decision,
		Reason: request.Reason, ReviewedBy: request.ReviewedBy,
		Version: 1, CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := validation.Validate(); err != nil {
		return ReviewSpecialistDelegationRequest{}, err
	}
	return request, nil
}

func specialistDelegationReviewFingerprint(request ReviewSpecialistDelegationRequest,
	proposal domain.SpecialistDelegationProposal,
) (string, error) {
	intent := struct {
		ProposalID string                                    `json:"proposal_id"`
		RunID      string                                    `json:"run_id"`
		Decision   domain.SpecialistDelegationReviewDecision `json:"decision"`
		Reason     string                                    `json:"reason"`
		ReviewedBy string                                    `json:"reviewed_by"`
	}{
		ProposalID: proposal.ID, RunID: proposal.RunID, Decision: request.Decision,
		Reason: request.Reason, ReviewedBy: request.ReviewedBy,
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", fmt.Errorf("encode specialist delegation review fingerprint: %w", err)
	}
	return runmutation.Fingerprint("specialist_delegation_review.v1", string(encoded)), nil
}
