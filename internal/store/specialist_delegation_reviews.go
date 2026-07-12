package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/redact"
)

const specialistDelegationReviewSelect = `SELECT id, proposal_id, run_id, root_agent_id,
	decision, reason, reviewed_by, version, created_at FROM specialist_delegation_reviews`

func (s *SQLiteStore) CreateSpecialistDelegationReview(ctx context.Context,
	operation domain.SpecialistDelegationReviewOperation,
	review domain.SpecialistDelegationReview,
	reviewEvent events.Event,
) (domain.SpecialistDelegationReview, bool, error) {
	operation = normalizeSpecialistDelegationReviewOperation(operation)
	review = normalizeSpecialistDelegationReview(review)
	if err := validateSpecialistDelegationReviewMutation(operation, review, reviewEvent); err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireStructuredMutationWriteLockTx(ctx, tx, operation.RunID); err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	}
	existingOperation, found, err := getSpecialistDelegationReviewOperation(ctx, tx,
		operation.KeyDigest)
	if err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	}
	if found {
		if err := validateSpecialistDelegationReviewReplay(existingOperation, operation); err != nil {
			return domain.SpecialistDelegationReview{}, false, err
		}
		stored, err := getSpecialistDelegationReview(ctx, tx, existingOperation.ReviewID)
		if err != nil {
			return domain.SpecialistDelegationReview{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.SpecialistDelegationReview{}, false, err
		}
		return stored, true, nil
	}
	if existing, exists, err := getSpecialistDelegationReviewByProposal(ctx, tx,
		review.ProposalID); err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	} else if exists {
		return domain.SpecialistDelegationReview{}, false, apperror.New(
			apperror.CodeConflict, fmt.Sprintf("specialist delegation proposal was already %s by %s",
				existing.Decision, existing.ReviewedBy))
	}
	run, mission, err := requireSpecialistDelegationReviewBindingTx(ctx, tx, operation, review)
	if err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	}
	if reviewEvent.RunID != run.ID || reviewEvent.MissionID != mission.ID {
		return domain.SpecialistDelegationReview{}, false, apperror.New(
			apperror.CodeInvalidArgument, "specialist delegation review event scope is invalid")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO specialist_delegation_reviews
		(id, proposal_id, run_id, root_agent_id, decision, reason, reviewed_by, version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, review.ID, review.ProposalID, review.RunID,
		review.RootAgentID, review.Decision, review.Reason, review.ReviewedBy,
		review.Version, ts(review.CreatedAt)); err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO specialist_delegation_review_operations
		(operation_key_digest, request_fingerprint, review_id, proposal_id, run_id,
		reviewed_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, review.ID, review.ProposalID, review.RunID,
		operation.ReviewedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSpecialistDelegationReview(ctx, operation, err)
	}
	if _, err := insertRunEventTx(ctx, tx, reviewEvent); err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	}
	return review, false, nil
}

func (s *SQLiteStore) GetSpecialistDelegationReview(ctx context.Context,
	id string,
) (domain.SpecialistDelegationReview, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return domain.SpecialistDelegationReview{}, apperror.New(
			apperror.CodeInvalidArgument, "specialist delegation review id is invalid")
	}
	return getSpecialistDelegationReview(ctx, s.db, id)
}

func (s *SQLiteStore) GetSpecialistDelegationReviewByProposal(ctx context.Context,
	proposalID string,
) (domain.SpecialistDelegationReview, bool, error) {
	proposalID = strings.TrimSpace(proposalID)
	if !domain.ValidAgentID(proposalID) || strings.ContainsRune(proposalID, 0) {
		return domain.SpecialistDelegationReview{}, false, apperror.New(
			apperror.CodeInvalidArgument, "specialist delegation proposal id is invalid")
	}
	return getSpecialistDelegationReviewByProposal(ctx, s.db, proposalID)
}

func normalizeSpecialistDelegationReview(
	review domain.SpecialistDelegationReview,
) domain.SpecialistDelegationReview {
	review.ID = strings.TrimSpace(review.ID)
	review.ProposalID = strings.TrimSpace(review.ProposalID)
	review.RunID = strings.TrimSpace(review.RunID)
	review.RootAgentID = strings.TrimSpace(review.RootAgentID)
	review.Decision = domain.SpecialistDelegationReviewDecision(
		strings.TrimSpace(string(review.Decision)))
	review.Reason = strings.TrimSpace(redact.String(review.Reason))
	review.ReviewedBy = strings.TrimSpace(redact.String(review.ReviewedBy))
	review.CreatedAt = review.CreatedAt.UTC()
	return review
}

func normalizeSpecialistDelegationReviewOperation(
	operation domain.SpecialistDelegationReviewOperation,
) domain.SpecialistDelegationReviewOperation {
	operation.KeyDigest = strings.TrimSpace(operation.KeyDigest)
	operation.RequestFingerprint = strings.TrimSpace(operation.RequestFingerprint)
	operation.ReviewID = strings.TrimSpace(operation.ReviewID)
	operation.ProposalID = strings.TrimSpace(operation.ProposalID)
	operation.RunID = strings.TrimSpace(operation.RunID)
	operation.ReviewedBy = strings.TrimSpace(redact.String(operation.ReviewedBy))
	operation.CreatedAt = operation.CreatedAt.UTC()
	return operation
}

func validateSpecialistDelegationReviewMutation(
	operation domain.SpecialistDelegationReviewOperation,
	review domain.SpecialistDelegationReview,
	reviewEvent events.Event,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"specialist delegation review operation is invalid", err)
	}
	if err := review.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"specialist delegation review is invalid", err)
	}
	if operation.ReviewID != review.ID || operation.ProposalID != review.ProposalID ||
		operation.RunID != review.RunID || operation.ReviewedBy != review.ReviewedBy {
		return apperror.New(apperror.CodeInvalidArgument,
			"specialist delegation review operation does not match its review")
	}
	if err := reviewEvent.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"specialist delegation review event is invalid", err)
	}
	if err := validateSpecialistDelegationReviewEvent(reviewEvent, review); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"specialist delegation review event metadata is invalid", err)
	}
	return nil
}

func validateSpecialistDelegationReviewEvent(event events.Event,
	review domain.SpecialistDelegationReview,
) error {
	if event.Type != events.AgentDelegationReviewedEvent || event.Source != "operator" ||
		event.SubjectID != review.ID {
		return errors.New("specialist delegation review event identity is invalid")
	}
	var payload struct {
		ReviewID            string                                    `json:"review_id"`
		ProposalID          string                                    `json:"proposal_id"`
		RootAgentID         string                                    `json:"root_agent_id"`
		Decision            domain.SpecialistDelegationReviewDecision `json:"decision"`
		ReviewedBy          string                                    `json:"reviewed_by"`
		ReviewVersion       int64                                     `json:"review_version"`
		AdmissionAuthorized *bool                                     `json:"admission_authorized"`
		ApplicationRequired *bool                                     `json:"application_required"`
	}
	decoder := json.NewDecoder(strings.NewReader(event.PayloadJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("specialist delegation review event contains trailing data")
	}
	if payload.ReviewID != review.ID || payload.ProposalID != review.ProposalID ||
		payload.RootAgentID != review.RootAgentID || payload.Decision != review.Decision ||
		payload.ReviewedBy != review.ReviewedBy || payload.ReviewVersion != review.Version ||
		payload.AdmissionAuthorized == nil || *payload.AdmissionAuthorized ||
		payload.ApplicationRequired == nil || !*payload.ApplicationRequired {
		return errors.New("specialist delegation review event does not match its review")
	}
	return nil
}

func requireSpecialistDelegationReviewBindingTx(ctx context.Context, tx *sql.Tx,
	operation domain.SpecialistDelegationReviewOperation,
	review domain.SpecialistDelegationReview,
) (domain.Run, domain.Mission, error) {
	proposal, err := getSpecialistDelegationProposalTx(ctx, tx, review.ProposalID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, err
	}
	if proposal.RunID != review.RunID || proposal.RootAgentID != review.RootAgentID ||
		proposal.ID != operation.ProposalID || proposal.Status != domain.SpecialistDelegationProposed ||
		review.CreatedAt.Before(proposal.CreatedAt) {
		return domain.Run{}, domain.Mission{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"specialist delegation review does not match an immutable proposal")
	}
	run, mission, err := getCoordinatorRunTx(ctx, tx, review.RunID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, err
	}
	if review.Decision == domain.SpecialistDelegationApproved && run.Status != domain.RunRunning {
		return domain.Run{}, domain.Mission{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"specialist delegation can only be approved while its Run is running")
	}
	var rootCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_nodes
		WHERE id = ? AND run_id = ? AND role = 'root' AND parent_id IS NULL`,
		review.RootAgentID, review.RunID).Scan(&rootCount); err != nil {
		return domain.Run{}, domain.Mission{}, err
	}
	if rootCount != 1 {
		return domain.Run{}, domain.Mission{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"specialist delegation review root Agent binding is invalid")
	}
	return run, mission, nil
}

func validateSpecialistDelegationReviewReplay(existing,
	request domain.SpecialistDelegationReviewOperation,
) error {
	if existing.KeyDigest != request.KeyDigest ||
		existing.RequestFingerprint != request.RequestFingerprint ||
		existing.ProposalID != request.ProposalID || existing.RunID != request.RunID ||
		existing.ReviewedBy != request.ReviewedBy {
		return apperror.New(apperror.CodeConflict,
			"specialist delegation review operation key was already used for different intent")
	}
	return nil
}

func (s *SQLiteStore) recoverSpecialistDelegationReview(ctx context.Context,
	operation domain.SpecialistDelegationReviewOperation, original error,
) (domain.SpecialistDelegationReview, bool, error) {
	existing, found, err := getSpecialistDelegationReviewOperation(ctx, s.db,
		operation.KeyDigest)
	if err != nil || !found {
		if err == nil {
			return domain.SpecialistDelegationReview{}, false, original
		}
		return domain.SpecialistDelegationReview{}, false, errors.Join(original, err)
	}
	if err := validateSpecialistDelegationReviewReplay(existing, operation); err != nil {
		return domain.SpecialistDelegationReview{}, false, err
	}
	review, err := s.GetSpecialistDelegationReview(ctx, existing.ReviewID)
	return review, true, err
}

func getSpecialistDelegationReviewOperation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (domain.SpecialistDelegationReviewOperation, bool, error) {
	var operation domain.SpecialistDelegationReviewOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest, request_fingerprint,
		review_id, proposal_id, run_id, reviewed_by, created_at
		FROM specialist_delegation_review_operations WHERE operation_key_digest = ?`, keyDigest).
		Scan(&operation.KeyDigest, &operation.RequestFingerprint, &operation.ReviewID,
			&operation.ProposalID, &operation.RunID, &operation.ReviewedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SpecialistDelegationReviewOperation{}, false, nil
	}
	if err != nil {
		return domain.SpecialistDelegationReviewOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	return operation, true, operation.Validate()
}

func getSpecialistDelegationReview(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (domain.SpecialistDelegationReview, error) {
	return scanSpecialistDelegationReview(queryer.QueryRowContext(ctx,
		specialistDelegationReviewSelect+` WHERE id = ?`, id))
}

func getSpecialistDelegationReviewByProposal(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, proposalID string) (domain.SpecialistDelegationReview, bool, error) {
	review, err := scanSpecialistDelegationReview(queryer.QueryRowContext(ctx,
		specialistDelegationReviewSelect+` WHERE proposal_id = ?`, proposalID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.SpecialistDelegationReview{}, false, nil
	}
	return review, err == nil, err
}

func scanSpecialistDelegationReview(row scanner) (domain.SpecialistDelegationReview, error) {
	var review domain.SpecialistDelegationReview
	var decision string
	var createdAt string
	err := row.Scan(&review.ID, &review.ProposalID, &review.RunID, &review.RootAgentID,
		&decision, &review.Reason, &review.ReviewedBy, &review.Version, &createdAt)
	if err != nil {
		return domain.SpecialistDelegationReview{}, err
	}
	review.Decision = domain.SpecialistDelegationReviewDecision(decision)
	review.CreatedAt = parseTS(createdAt)
	return review, review.Validate()
}
