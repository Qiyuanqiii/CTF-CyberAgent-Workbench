package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
)

const runExecutionLeaseSelect = `SELECT run_id, lease_id, owner_id, generation, status,
	acquired_at, renewed_at, expires_at, released_at FROM run_execution_leases WHERE run_id = ?`

func (s *SQLiteStore) AcquireRunExecutionLease(ctx context.Context,
	request domain.AcquireRunExecutionLeaseRequest,
) (domain.RunExecutionLeaseAcquisition, error) {
	normalized, err := request.Normalize()
	if err != nil {
		return domain.RunExecutionLeaseAcquisition{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if safe := redact.String(normalized.OwnerID); safe != normalized.OwnerID {
		return domain.RunExecutionLeaseAcquisition{},
			apperror.New(apperror.CodeInvalidArgument, "run execution lease owner id cannot contain sensitive material")
	}
	if err := ctx.Err(); err != nil {
		return domain.RunExecutionLeaseAcquisition{}, apperror.Normalize(err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunExecutionLeaseAcquisition{}, err
	}
	defer func() { _ = tx.Rollback() }()
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, normalized.RunID))
	if err != nil {
		return domain.RunExecutionLeaseAcquisition{}, err
	}
	if run.Terminal() {
		return domain.RunExecutionLeaseAcquisition{}, apperror.New(apperror.CodeFailedPrecondition,
			fmt.Sprintf("run %s is terminal as %s", run.ID, run.Status))
	}
	now := time.Now().UTC()
	current, found, err := getRunExecutionLeaseTx(ctx, tx, run.ID)
	if err != nil {
		return domain.RunExecutionLeaseAcquisition{}, err
	}
	if found && current.ActiveAt(now) {
		if current.OwnerID != normalized.OwnerID || normalized.LeaseID == "" ||
			normalized.LeaseID != current.LeaseID {
			return domain.RunExecutionLeaseAcquisition{}, apperror.New(apperror.CodeConflict,
				fmt.Sprintf("run %s is already executing under an active lease until %s",
					run.ID, current.ExpiresAt.Format(time.RFC3339Nano)))
		}
		current.RenewedAt = now
		current.ExpiresAt = now.Add(normalized.TTL)
		result, err := tx.ExecContext(ctx, `UPDATE run_execution_leases SET renewed_at = ?, expires_at = ?
			WHERE run_id = ? AND lease_id = ? AND generation = ? AND owner_id = ? AND status = ?`,
			ts(current.RenewedAt), ts(current.ExpiresAt), current.RunID, current.LeaseID,
			current.Generation, current.OwnerID, domain.RunExecutionLeaseActive)
		if err != nil {
			return domain.RunExecutionLeaseAcquisition{}, err
		}
		if err := requireSingleLeaseUpdate(result, "run execution lease changed before replayed acquisition"); err != nil {
			return domain.RunExecutionLeaseAcquisition{}, err
		}
		if err := tx.Commit(); err != nil {
			return domain.RunExecutionLeaseAcquisition{}, err
		}
		return domain.RunExecutionLeaseAcquisition{Lease: current, Replayed: true}, nil
	}
	if normalized.LeaseID != "" {
		return domain.RunExecutionLeaseAcquisition{}, apperror.New(apperror.CodeConflict,
			"run execution lease replay token is no longer active")
	}

	generation := int64(1)
	previousGeneration := int64(0)
	tookOver := false
	if found {
		previousGeneration = current.Generation
		generation = current.Generation + 1
		tookOver = current.Status == domain.RunExecutionLeaseActive
	}
	lease := domain.RunExecutionLease{
		RunID: run.ID, LeaseID: idgen.New("lease"), OwnerID: normalized.OwnerID,
		Generation: generation, Status: domain.RunExecutionLeaseActive,
		AcquiredAt: now, RenewedAt: now, ExpiresAt: now.Add(normalized.TTL),
	}
	if err := lease.Validate(); err != nil {
		return domain.RunExecutionLeaseAcquisition{}, err
	}
	if !found {
		_, err = tx.ExecContext(ctx, `INSERT INTO run_execution_leases
			(run_id, lease_id, owner_id, generation, status, acquired_at, renewed_at, expires_at, released_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL)`, lease.RunID, lease.LeaseID, lease.OwnerID,
			lease.Generation, lease.Status, ts(lease.AcquiredAt), ts(lease.RenewedAt), ts(lease.ExpiresAt))
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE run_execution_leases SET lease_id = ?, owner_id = ?, generation = ?,
			status = ?, acquired_at = ?, renewed_at = ?, expires_at = ?, released_at = NULL
			WHERE run_id = ? AND generation = ?`, lease.LeaseID, lease.OwnerID, lease.Generation,
			lease.Status, ts(lease.AcquiredAt), ts(lease.RenewedAt), ts(lease.ExpiresAt), lease.RunID,
			current.Generation)
		if err == nil {
			err = requireSingleLeaseUpdate(result, "run execution lease changed before takeover")
		}
	}
	if err != nil {
		return domain.RunExecutionLeaseAcquisition{}, err
	}
	eventType := events.RunExecutionLeaseAcquiredEvent
	if tookOver {
		eventType = events.RunExecutionLeaseTakenOverEvent
	}
	if err := appendSupervisorEventTx(ctx, tx, run, eventType, "execution_lease_store", run.ID,
		map[string]any{
			"owner_id": lease.OwnerID, "generation": lease.Generation,
			"expires_at": lease.ExpiresAt, "previous_generation": previousGeneration,
		}); err != nil {
		return domain.RunExecutionLeaseAcquisition{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunExecutionLeaseAcquisition{}, err
	}
	return domain.RunExecutionLeaseAcquisition{Lease: lease, TookOver: tookOver}, nil
}

func (s *SQLiteStore) RenewRunExecutionLease(ctx context.Context, expected domain.RunExecutionLease,
	ttl time.Duration,
) (domain.RunExecutionLease, error) {
	if err := expected.Validate(); err != nil {
		return domain.RunExecutionLease{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	if err := domain.ValidateRunExecutionLeaseTTL(ttl); err != nil {
		return domain.RunExecutionLease{}, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunExecutionLease{}, err
	}
	defer func() { _ = tx.Rollback() }()
	current, found, err := getRunExecutionLeaseTx(ctx, tx, expected.RunID)
	if err != nil {
		return domain.RunExecutionLease{}, err
	}
	now := time.Now().UTC()
	if !found || !sameRunExecutionLease(current, expected) || !current.ActiveAt(now) {
		return domain.RunExecutionLease{}, apperror.New(apperror.CodeConflict,
			"run execution lease was lost or expired before renewal")
	}
	current.RenewedAt = now
	current.ExpiresAt = now.Add(ttl)
	result, err := tx.ExecContext(ctx, `UPDATE run_execution_leases SET renewed_at = ?, expires_at = ?
		WHERE run_id = ? AND lease_id = ? AND owner_id = ? AND generation = ? AND status = ?`,
		ts(current.RenewedAt), ts(current.ExpiresAt), current.RunID, current.LeaseID,
		current.OwnerID, current.Generation, domain.RunExecutionLeaseActive)
	if err != nil {
		return domain.RunExecutionLease{}, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.RunExecutionLease{}, err
	}
	if rows != 1 {
		return domain.RunExecutionLease{}, apperror.New(apperror.CodeConflict,
			"run execution lease changed before renewal")
	}
	if err := tx.Commit(); err != nil {
		return domain.RunExecutionLease{}, err
	}
	return current, nil
}

func (s *SQLiteStore) ReleaseRunExecutionLease(ctx context.Context,
	expected domain.RunExecutionLease,
) (domain.RunExecutionLease, bool, error) {
	if err := expected.Validate(); err != nil {
		return domain.RunExecutionLease{}, false, apperror.Wrap(apperror.CodeInvalidArgument, err.Error(), err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.RunExecutionLease{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	current, found, err := getRunExecutionLeaseTx(ctx, tx, expected.RunID)
	if err != nil {
		return domain.RunExecutionLease{}, false, err
	}
	if !found || !sameRunExecutionLease(current, expected) {
		return domain.RunExecutionLease{}, false, apperror.New(apperror.CodeConflict,
			"run execution lease was replaced before release")
	}
	if current.Status == domain.RunExecutionLeaseReleased {
		if err := tx.Commit(); err != nil {
			return domain.RunExecutionLease{}, false, err
		}
		return current, true, nil
	}
	now := time.Now().UTC()
	current.Status = domain.RunExecutionLeaseReleased
	current.ReleasedAt = &now
	result, err := tx.ExecContext(ctx, `UPDATE run_execution_leases SET status = ?, released_at = ?
		WHERE run_id = ? AND lease_id = ? AND owner_id = ? AND generation = ? AND status = ?`,
		current.Status, ts(now), current.RunID, current.LeaseID, current.OwnerID, current.Generation,
		domain.RunExecutionLeaseActive)
	if err != nil {
		return domain.RunExecutionLease{}, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.RunExecutionLease{}, false, err
	}
	if rows != 1 {
		return domain.RunExecutionLease{}, false, apperror.New(apperror.CodeConflict,
			"run execution lease changed before release")
	}
	run, err := scanRun(tx.QueryRowContext(ctx, `SELECT id, mission_id, session_id, status, config_json, budget_json,
		started_at, finished_at, created_at, updated_at FROM runs WHERE id = ?`, current.RunID))
	if err != nil {
		return domain.RunExecutionLease{}, false, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.RunExecutionLeaseReleasedEvent,
		"execution_lease_store", run.ID, map[string]any{
			"owner_id": current.OwnerID, "generation": current.Generation, "released_at": now,
		}); err != nil {
		return domain.RunExecutionLease{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.RunExecutionLease{}, false, err
	}
	return current, false, nil
}

func (s *SQLiteStore) GetRunExecutionLease(ctx context.Context, runID string) (domain.RunExecutionLease, bool, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" || len([]rune(runID)) > domain.MaxRunLeaseIdentityRunes {
		return domain.RunExecutionLease{}, false,
			apperror.New(apperror.CodeInvalidArgument, "run execution lease run id is required and bounded")
	}
	lease, err := scanRunExecutionLease(s.db.QueryRowContext(ctx, runExecutionLeaseSelect, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RunExecutionLease{}, false, nil
	}
	if err != nil {
		return domain.RunExecutionLease{}, false, err
	}
	return lease, true, nil
}

func getRunExecutionLeaseTx(ctx context.Context, tx *sql.Tx,
	runID string,
) (domain.RunExecutionLease, bool, error) {
	lease, err := scanRunExecutionLease(tx.QueryRowContext(ctx, runExecutionLeaseSelect, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.RunExecutionLease{}, false, nil
	}
	if err != nil {
		return domain.RunExecutionLease{}, false, err
	}
	return lease, true, nil
}

func requireRunExecutionLeaseTx(ctx context.Context, tx *sql.Tx, runID string, leaseID string,
	generation int64,
) error {
	current, found, err := getRunExecutionLeaseTx(ctx, tx, runID)
	if err != nil {
		return err
	}
	if !found || current.LeaseID != strings.TrimSpace(leaseID) || current.Generation != generation ||
		!current.ActiveAt(time.Now().UTC()) {
		return apperror.New(apperror.CodeConflict,
			"run execution lease fencing token is stale, released, or expired")
	}
	return nil
}

func sameRunExecutionLease(left domain.RunExecutionLease, right domain.RunExecutionLease) bool {
	return left.RunID == right.RunID && left.LeaseID == right.LeaseID && left.OwnerID == right.OwnerID &&
		left.Generation == right.Generation
}

func requireSingleLeaseUpdate(result sql.Result, message string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return apperror.New(apperror.CodeConflict, message)
	}
	return nil
}

func scanRunExecutionLease(row scanner) (domain.RunExecutionLease, error) {
	var lease domain.RunExecutionLease
	var status string
	var acquiredAt, renewedAt, expiresAt string
	var releasedAt sql.NullString
	if err := row.Scan(&lease.RunID, &lease.LeaseID, &lease.OwnerID, &lease.Generation, &status,
		&acquiredAt, &renewedAt, &expiresAt, &releasedAt); err != nil {
		return domain.RunExecutionLease{}, err
	}
	lease.Status = domain.RunExecutionLeaseStatus(status)
	lease.AcquiredAt = parseTS(acquiredAt)
	lease.RenewedAt = parseTS(renewedAt)
	lease.ExpiresAt = parseTS(expiresAt)
	if releasedAt.Valid {
		value := parseTS(releasedAt.String)
		lease.ReleasedAt = &value
	}
	return lease, lease.Validate()
}
