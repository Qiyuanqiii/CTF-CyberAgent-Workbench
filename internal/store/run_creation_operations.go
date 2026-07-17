package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
)

type runCreationOperationQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *SQLiteStore) GetRunCreationOperation(ctx context.Context,
	keyDigest string,
) (domain.RunCreationOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return domain.RunCreationOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "Run creation operation digest is invalid")
	}
	return getRunCreationOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) CreateMissionRunWithOperation(ctx context.Context,
	mission domain.Mission, run domain.Run, mode domain.RunModeSnapshot,
	linkedSession session.Session, initialEvents []events.Event,
	operation domain.RunCreationOperation,
) (domain.RunCreationOperation, bool, error) {
	if err := validateControlledRunCreation(mission, run, mode, linkedSession,
		initialEvents, operation); err != nil {
		return domain.RunCreationOperation{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunCreationOperation{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	if existing, found, err := getRunCreationOperation(ctx, tx, operation.KeyDigest); err != nil {
		return domain.RunCreationOperation{}, false, err
	} else if found {
		if err := validateRunCreationReplay(existing, operation); err != nil {
			return domain.RunCreationOperation{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.RunCreationOperation{}, false, err
		}
		return existing, true, nil
	}

	if err := createMissionRunTx(ctx, tx, mission, run, mode, linkedSession,
		true, initialEvents); err != nil {
		return domain.RunCreationOperation{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_creation_operations
		(operation_key_digest, request_fingerprint, protocol_version, mission_id,
		run_id, session_id, workspace_id, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.ProtocolVersion, operation.MissionID,
		operation.RunID, operation.SessionID, operation.WorkspaceID,
		operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		return domain.RunCreationOperation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunCreationOperation{}, false, err
	}
	return operation, false, nil
}

func getRunCreationOperation(ctx context.Context, queryer runCreationOperationQueryer,
	keyDigest string,
) (domain.RunCreationOperation, bool, error) {
	var operation domain.RunCreationOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT protocol_version,
		operation_key_digest, request_fingerprint, mission_id, run_id, session_id,
		workspace_id, requested_by, created_at
		FROM run_creation_operations WHERE operation_key_digest = ?`, keyDigest).
		Scan(&operation.ProtocolVersion, &operation.KeyDigest,
			&operation.RequestFingerprint, &operation.MissionID, &operation.RunID,
			&operation.SessionID, &operation.WorkspaceID, &operation.RequestedBy,
			&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RunCreationOperation{}, false, nil
	}
	if err != nil {
		return domain.RunCreationOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	if err := operation.Validate(); err != nil {
		return domain.RunCreationOperation{}, false, fmt.Errorf(
			"validate stored Run creation operation: %w", err)
	}
	return operation, true, nil
}

func validateControlledRunCreation(mission domain.Mission, run domain.Run,
	mode domain.RunModeSnapshot, linkedSession session.Session,
	initialEvents []events.Event, operation domain.RunCreationOperation,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Run creation operation is invalid", err)
	}
	if err := mission.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "Mission is invalid", err)
	}
	if err := run.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "Run is invalid", err)
	}
	if err := mode.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "Run mode is invalid", err)
	}
	if err := linkedSession.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "Session is invalid", err)
	}
	if operation.MissionID != mission.ID || operation.RunID != run.ID ||
		operation.SessionID != linkedSession.ID ||
		operation.WorkspaceID != mission.WorkspaceID ||
		operation.WorkspaceID != linkedSession.WorkspaceID ||
		operation.RequestedBy != mode.RequestedBy ||
		!operation.CreatedAt.Equal(run.CreatedAt) ||
		!operation.CreatedAt.Equal(mission.CreatedAt) ||
		!operation.CreatedAt.Equal(linkedSession.CreatedAt) ||
		!operation.CreatedAt.Equal(mode.CreatedAt) ||
		!operation.CreatedAt.Equal(run.UpdatedAt) ||
		!operation.CreatedAt.Equal(mission.UpdatedAt) ||
		!operation.CreatedAt.Equal(linkedSession.UpdatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Run creation operation does not match its Mission, Run, Session, and mode")
	}
	if run.MissionID != mission.ID || run.SessionID != linkedSession.ID ||
		run.Status != domain.RunCreated || run.StartedAt != nil || run.FinishedAt != nil ||
		!run.Config.Interactive ||
		run.Config.ModelRoute != string(mission.Profile) ||
		run.Budget != domain.DefaultBudget() ||
		mission.Scope.WorkspaceID != mission.WorkspaceID ||
		mission.Scope.NetworkMode != "disabled" ||
		len(mission.Scope.AllowedTargets) != 0 || mode.Revision != 1 ||
		mode.MissionID != mission.ID || mode.RunID != run.ID ||
		mode.Profile != mission.Profile || !sameRunModeScope(mode.Scope, mission.Scope) ||
		linkedSession.Status != session.StatusActive || linkedSession.Title != mission.Goal ||
		linkedSession.Route != string(mission.Profile) {
		return apperror.New(apperror.CodeInvalidArgument,
			"controlled Run creation must remain interactive, workspace-bound, and non-networked")
	}
	if operation.RequestFingerprint != runmutation.RunCreationRequestFingerprint(
		mission.Goal, mission.WorkspaceID, string(mission.Profile), string(mode.Surface),
		string(mode.Phase), operation.RequestedBy) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Run creation request fingerprint does not match the controlled Run")
	}
	if len(initialEvents) != 2 || initialEvents[0].Type != events.RunCreatedEvent ||
		initialEvents[1].Type != events.SessionAttachedEvent {
		return apperror.New(apperror.CodeInvalidArgument,
			"controlled Run creation requires the exact initial event set")
	}
	return nil
}

func validateRunCreationReplay(existing domain.RunCreationOperation,
	requested domain.RunCreationOperation,
) error {
	if existing.ProtocolVersion != requested.ProtocolVersion ||
		existing.KeyDigest != requested.KeyDigest ||
		existing.RequestFingerprint != requested.RequestFingerprint ||
		existing.WorkspaceID != requested.WorkspaceID ||
		existing.RequestedBy != requested.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"Run creation idempotency key was already used for a different request")
	}
	return nil
}
