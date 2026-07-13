package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/redact"
)

const deliveryCheckpointSelect = `SELECT id, run_id, selection_id, proposal_id,
	work_item_id, direction_ordinal, module_ordinal, module_count,
	mode_snapshot_id, mode_revision, work_item_version, acceptance_fingerprint,
	source_fingerprint, focused_verification, diff_audit, security_audit,
	full_gate_required, functional_verification, robustness_audit,
	handoff_note_id, handoff_digest, requested_by, version, created_at
	FROM delivery_checkpoints`

func (s *SQLiteStore) RecordDeliveryCheckpoint(ctx context.Context,
	operation domain.DeliveryCheckpointOperation,
	checkpoint domain.DeliveryCheckpoint, note domain.Note,
	checkpointEvent events.Event, noteEvent events.Event,
) (domain.DeliveryCheckpoint, bool, error) {
	operation = normalizeDeliveryCheckpointOperation(operation)
	checkpoint = normalizeDeliveryCheckpoint(checkpoint)
	note = redactAndNormalizeNote(note)
	if err := validateDeliveryCheckpointMutation(operation, checkpoint, note,
		checkpointEvent, noteEvent); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireDeliveryCheckpointWriteLockTx(ctx, tx, checkpoint.RunID); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	if existing, found, err := getDeliveryCheckpointOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	} else if found {
		if err := validateDeliveryCheckpointReplay(existing, operation); err != nil {
			return domain.DeliveryCheckpoint{}, false, err
		}
		stored, err := getDeliveryCheckpoint(ctx, tx, existing.CheckpointID)
		if err != nil {
			return domain.DeliveryCheckpoint{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.DeliveryCheckpoint{}, false, err
		}
		return stored, true, nil
	}
	var existingID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM delivery_checkpoints
		WHERE work_item_id = ? AND mode_revision = ? AND work_item_version = ?`,
		checkpoint.WorkItemID, checkpoint.ModeRevision,
		checkpoint.WorkItemVersion).Scan(&existingID)
	if err == nil {
		return domain.DeliveryCheckpoint{}, false, apperror.New(
			apperror.CodeConflict,
			"Delivery checkpoint already exists for this WorkItem and revision")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.DeliveryCheckpoint{}, false, err
	}
	run, mission, proposal, selection, module, item, mode, err :=
		requireDeliveryCheckpointBindingTx(ctx, tx, checkpoint)
	if err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	if err := validateDeliveryCheckpointProjection(checkpoint, proposal,
		selection, module, item, mode); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	if note.RunID != run.ID || note.ID != checkpoint.HandoffNoteID ||
		!note.CreatedAt.Equal(checkpoint.CreatedAt) ||
		!note.UpdatedAt.Equal(checkpoint.CreatedAt) ||
		domain.DeliveryHandoffDigest(note.Title, note.Content) != checkpoint.HandoffDigest {
		return domain.DeliveryCheckpoint{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Delivery handoff Note does not match its checkpoint")
	}
	if checkpointEvent.RunID != run.ID || checkpointEvent.MissionID != mission.ID ||
		noteEvent.RunID != run.ID || noteEvent.MissionID != mission.ID {
		return domain.DeliveryCheckpoint{}, false, apperror.New(
			apperror.CodeInvalidArgument,
			"Delivery checkpoint event scope does not match the Run")
	}
	if err := insertNewNoteTx(ctx, tx, note); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_checkpoints
		(id, run_id, selection_id, proposal_id, work_item_id, direction_ordinal,
		module_ordinal, module_count, mode_snapshot_id, mode_revision,
		work_item_version, acceptance_fingerprint, source_fingerprint,
		focused_verification, diff_audit, security_audit, full_gate_required,
		functional_verification, robustness_audit, handoff_note_id,
		handoff_digest, requested_by, version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		checkpoint.ID, checkpoint.RunID, checkpoint.SelectionID,
		checkpoint.ProposalID, checkpoint.WorkItemID,
		checkpoint.DirectionOrdinal, checkpoint.ModuleOrdinal,
		checkpoint.ModuleCount, checkpoint.ModeSnapshotID,
		checkpoint.ModeRevision, checkpoint.WorkItemVersion,
		checkpoint.AcceptanceFingerprint, checkpoint.SourceFingerprint,
		checkpoint.FocusedVerification, checkpoint.DiffAudit,
		checkpoint.SecurityAudit, boolInt(checkpoint.FullGateRequired),
		checkpoint.FunctionalVerification, checkpoint.RobustnessAudit,
		checkpoint.HandoffNoteID, checkpoint.HandoffDigest,
		checkpoint.RequestedBy, checkpoint.Version,
		ts(checkpoint.CreatedAt)); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO delivery_checkpoint_operations
		(operation_key_digest, request_fingerprint, checkpoint_id, run_id,
		work_item_id, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, checkpoint.ID, checkpoint.RunID,
		checkpoint.WorkItemID, checkpoint.RequestedBy,
		ts(checkpoint.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverDeliveryCheckpoint(ctx, operation, err)
	}
	if _, err := insertRunEventTx(ctx, tx, noteEvent); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	if _, err := insertRunEventTx(ctx, tx, checkpointEvent); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	return checkpoint, false, nil
}

func acquireDeliveryCheckpointWriteLockTx(ctx context.Context, tx *sql.Tx,
	runID string,
) error {
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at
		WHERE id = ? AND status = ?`, runID, domain.RunPaused)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Delivery checkpoint requires a paused Run")
	}
	return nil
}

func (s *SQLiteStore) GetDeliveryCheckpoint(ctx context.Context,
	id string,
) (domain.DeliveryCheckpoint, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return domain.DeliveryCheckpoint{}, apperror.New(
			apperror.CodeInvalidArgument, "Delivery checkpoint id is invalid")
	}
	return getDeliveryCheckpoint(ctx, s.db, id)
}

func (s *SQLiteStore) ListDeliveryCheckpoints(ctx context.Context,
	runID string, limit int,
) ([]domain.DeliveryCheckpoint, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Delivery checkpoint Run id is invalid")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 500 {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"Delivery checkpoint limit must be between 1 and 500")
	}
	rows, err := s.db.QueryContext(ctx, deliveryCheckpointSelect+
		` WHERE run_id = ? ORDER BY module_ordinal, created_at, id LIMIT ?`,
		runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]domain.DeliveryCheckpoint, 0)
	for rows.Next() {
		value, err := scanDeliveryCheckpoint(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *SQLiteStore) DeliveryGateEnforced(ctx context.Context,
	runID string,
) (bool, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return false, apperror.New(apperror.CodeInvalidArgument,
			"Delivery gate Run id is invalid")
	}
	var enrolled int
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1
		FROM delivery_gate_enrollments WHERE run_id = ?)`, runID).Scan(&enrolled); err != nil {
		return false, err
	}
	return enrolled != 0, nil
}

func normalizeDeliveryCheckpointOperation(
	operation domain.DeliveryCheckpointOperation,
) domain.DeliveryCheckpointOperation {
	operation.KeyDigest = strings.TrimSpace(operation.KeyDigest)
	operation.RequestFingerprint = strings.TrimSpace(operation.RequestFingerprint)
	operation.CheckpointID = strings.TrimSpace(operation.CheckpointID)
	operation.RunID = strings.TrimSpace(operation.RunID)
	operation.WorkItemID = strings.TrimSpace(operation.WorkItemID)
	operation.RequestedBy = strings.TrimSpace(redact.String(operation.RequestedBy))
	operation.CreatedAt = operation.CreatedAt.UTC()
	return operation
}

func normalizeDeliveryCheckpoint(
	checkpoint domain.DeliveryCheckpoint,
) domain.DeliveryCheckpoint {
	checkpoint.ID = strings.TrimSpace(checkpoint.ID)
	checkpoint.RunID = strings.TrimSpace(checkpoint.RunID)
	checkpoint.SelectionID = strings.TrimSpace(checkpoint.SelectionID)
	checkpoint.ProposalID = strings.TrimSpace(checkpoint.ProposalID)
	checkpoint.WorkItemID = strings.TrimSpace(checkpoint.WorkItemID)
	checkpoint.ModeSnapshotID = strings.TrimSpace(checkpoint.ModeSnapshotID)
	checkpoint.AcceptanceFingerprint = strings.TrimSpace(checkpoint.AcceptanceFingerprint)
	checkpoint.SourceFingerprint = strings.TrimSpace(checkpoint.SourceFingerprint)
	checkpoint.FocusedVerification = strings.TrimSpace(redact.String(
		checkpoint.FocusedVerification))
	checkpoint.DiffAudit = strings.TrimSpace(redact.String(checkpoint.DiffAudit))
	checkpoint.SecurityAudit = strings.TrimSpace(redact.String(checkpoint.SecurityAudit))
	checkpoint.FunctionalVerification = strings.TrimSpace(redact.String(
		checkpoint.FunctionalVerification))
	checkpoint.RobustnessAudit = strings.TrimSpace(redact.String(
		checkpoint.RobustnessAudit))
	checkpoint.HandoffNoteID = strings.TrimSpace(checkpoint.HandoffNoteID)
	checkpoint.HandoffDigest = strings.TrimSpace(checkpoint.HandoffDigest)
	checkpoint.RequestedBy = strings.TrimSpace(redact.String(checkpoint.RequestedBy))
	checkpoint.CreatedAt = checkpoint.CreatedAt.UTC()
	return checkpoint
}

func validateDeliveryCheckpointMutation(
	operation domain.DeliveryCheckpointOperation,
	checkpoint domain.DeliveryCheckpoint, note domain.Note,
	checkpointEvent events.Event, noteEvent events.Event,
) error {
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Delivery checkpoint operation is invalid", err)
	}
	if err := checkpoint.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Delivery checkpoint is invalid", err)
	}
	if operation.CheckpointID != checkpoint.ID || operation.RunID != checkpoint.RunID ||
		operation.WorkItemID != checkpoint.WorkItemID ||
		operation.RequestedBy != checkpoint.RequestedBy ||
		!operation.CreatedAt.Equal(checkpoint.CreatedAt) ||
		operation.RequestFingerprint != domain.DeliveryCheckpointRequestFingerprint(checkpoint) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Delivery checkpoint operation does not match its checkpoint")
	}
	if err := validateNewNote(note, noteEvent); err != nil {
		return err
	}
	if checkpointEvent.Type != events.DeliveryCheckpointRecordedEvent ||
		checkpointEvent.Source != "delivery" || checkpointEvent.SubjectID != checkpoint.ID ||
		checkpointEvent.RunID != checkpoint.RunID ||
		!checkpointEvent.CreatedAt.Equal(checkpoint.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Delivery checkpoint event identity is invalid")
	}
	if err := checkpointEvent.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Delivery checkpoint event is invalid", err)
	}
	if noteEvent.Source != "delivery" ||
		!noteEvent.CreatedAt.Equal(checkpoint.CreatedAt) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Delivery handoff Note event identity is invalid")
	}
	return nil
}

func requireDeliveryCheckpointBindingTx(ctx context.Context, tx *sql.Tx,
	checkpoint domain.DeliveryCheckpoint,
) (domain.Run, domain.Mission, domain.PlanDeliveryProposal,
	domain.PlanDeliverySelection, domain.PlanDeliveryModule, domain.WorkItem,
	domain.RunModeSnapshot, error,
) {
	var emptyRun domain.Run
	var emptyMission domain.Mission
	var emptyProposal domain.PlanDeliveryProposal
	var emptySelection domain.PlanDeliverySelection
	var emptyModule domain.PlanDeliveryModule
	var emptyItem domain.WorkItem
	var emptyMode domain.RunModeSnapshot
	run, mission, err := getCoordinatorRunTx(ctx, tx, checkpoint.RunID)
	if err != nil {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, err
	}
	selection, found, err := getPlanDeliverySelectionByRun(ctx, tx, run.ID)
	if err != nil || !found {
		if err == nil {
			err = apperror.New(apperror.CodeFailedPrecondition,
				"Delivery checkpoint requires a selected Plan/Delivery direction")
		}
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, err
	}
	var enrolled int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_gate_enrollments
		WHERE run_id = ? AND selection_id = ?)`, run.ID, selection.ID).Scan(&enrolled); err != nil {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, err
	}
	if enrolled == 0 {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, apperror.New(apperror.CodeFailedPrecondition,
				"legacy Plan/Delivery Run is not enrolled in v44 checkpoint gates")
	}
	proposal, err := getPlanDeliveryProposal(ctx, tx, selection.ProposalID)
	if err != nil {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, err
	}
	item, err := getWorkItemTx(ctx, tx, checkpoint.WorkItemID)
	if err != nil {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, err
	}
	mode, err := getCurrentRunModeSnapshot(ctx, tx, run.ID)
	if err != nil {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, err
	}
	selected, found := selectedDeliveryItem(selection, item.ID)
	if !found || selection.DirectionOrdinal < 1 ||
		selection.DirectionOrdinal > len(proposal.Spec.Directions) {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, apperror.New(apperror.CodeFailedPrecondition,
				"Delivery checkpoint source selection is inconsistent")
	}
	direction := proposal.Spec.Directions[selection.DirectionOrdinal-1]
	if selected.ModuleOrdinal < 1 || selected.ModuleOrdinal > len(direction.Modules) {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, apperror.New(apperror.CodeFailedPrecondition,
				"Delivery checkpoint source module is missing")
	}
	module := direction.Modules[selected.ModuleOrdinal-1]
	if run.Status != domain.RunPaused || mode.Phase != domain.ExecutionPhaseDeliver ||
		item.Status != domain.WorkItemInProgress ||
		checkpoint.SelectionID != selection.ID || checkpoint.ProposalID != proposal.ID ||
		checkpoint.WorkItemVersion != item.Version || checkpoint.ModeSnapshotID != mode.ID ||
		checkpoint.ModeRevision != mode.Revision ||
		checkpoint.DirectionOrdinal != selection.DirectionOrdinal ||
		checkpoint.ModuleOrdinal != selected.ModuleOrdinal ||
		checkpoint.ModuleCount != len(selection.Items) {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, apperror.New(apperror.CodeFailedPrecondition,
				"Delivery checkpoint requires the current paused Deliver revision and WorkItem version")
	}
	var activeLeases int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM run_execution_leases
		WHERE run_id = ? AND status = 'active'
			AND julianday(expires_at) > julianday('now')`, run.ID).Scan(&activeLeases); err != nil {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, err
	}
	if activeLeases != 0 {
		return emptyRun, emptyMission, emptyProposal, emptySelection, emptyModule,
			emptyItem, emptyMode, apperror.New(apperror.CodeFailedPrecondition,
				"Delivery checkpoint requires a quiescent Run without an active execution lease")
	}
	return run, mission, proposal, selection, module, item, mode, nil
}

func validateDeliveryCheckpointProjection(checkpoint domain.DeliveryCheckpoint,
	proposal domain.PlanDeliveryProposal, selection domain.PlanDeliverySelection,
	module domain.PlanDeliveryModule, item domain.WorkItem,
	mode domain.RunModeSnapshot,
) error {
	if item.Title != module.Title || item.Description != module.Objective ||
		!slices.Equal(item.AcceptanceCriteria, module.AcceptanceCriteria) ||
		checkpoint.AcceptanceFingerprint != domain.DeliveryAcceptanceFingerprint(item.AcceptanceCriteria) ||
		checkpoint.SourceFingerprint != domain.DeliverySourceFingerprint(proposal,
			selection, module, item) || checkpoint.ModeSnapshotID != mode.ID {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Delivery checkpoint source or acceptance fingerprint is stale")
	}
	expectedDependencies := make([]string, len(module.Dependencies))
	for index, ordinal := range module.Dependencies {
		if ordinal < 1 || ordinal > len(selection.Items) {
			return apperror.New(apperror.CodeFailedPrecondition,
				"Delivery checkpoint module dependency is invalid")
		}
		expectedDependencies[index] = selection.Items[ordinal-1].WorkItemID
	}
	if !slices.Equal(item.Dependencies, expectedDependencies) {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Delivery checkpoint WorkItem dependencies are stale")
	}
	return nil
}

func selectedDeliveryItem(selection domain.PlanDeliverySelection,
	workItemID string,
) (domain.PlanDeliverySelectionItem, bool) {
	for _, selected := range selection.Items {
		if selected.WorkItemID == workItemID {
			return selected, true
		}
	}
	return domain.PlanDeliverySelectionItem{}, false
}

func getDeliveryCheckpoint(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (domain.DeliveryCheckpoint, error) {
	value, err := scanDeliveryCheckpoint(queryer.QueryRowContext(ctx,
		deliveryCheckpointSelect+` WHERE id = ?`, strings.TrimSpace(id)))
	return value, err
}

func scanDeliveryCheckpoint(row scanner) (domain.DeliveryCheckpoint, error) {
	var value domain.DeliveryCheckpoint
	var fullGate int
	var createdAt string
	if err := row.Scan(&value.ID, &value.RunID, &value.SelectionID,
		&value.ProposalID, &value.WorkItemID, &value.DirectionOrdinal,
		&value.ModuleOrdinal, &value.ModuleCount, &value.ModeSnapshotID,
		&value.ModeRevision, &value.WorkItemVersion,
		&value.AcceptanceFingerprint, &value.SourceFingerprint,
		&value.FocusedVerification, &value.DiffAudit, &value.SecurityAudit,
		&fullGate, &value.FunctionalVerification, &value.RobustnessAudit,
		&value.HandoffNoteID, &value.HandoffDigest, &value.RequestedBy,
		&value.Version, &createdAt); err != nil {
		return domain.DeliveryCheckpoint{}, err
	}
	value.FullGateRequired = fullGate != 0
	value.CreatedAt = parseTS(createdAt)
	return value, value.Validate()
}

func getDeliveryCheckpointOperation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (domain.DeliveryCheckpointOperation, bool, error) {
	var operation domain.DeliveryCheckpointOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest,
		request_fingerprint, checkpoint_id, run_id, work_item_id, requested_by,
		created_at FROM delivery_checkpoint_operations
		WHERE operation_key_digest = ?`, keyDigest).Scan(&operation.KeyDigest,
		&operation.RequestFingerprint, &operation.CheckpointID, &operation.RunID,
		&operation.WorkItemID, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.DeliveryCheckpointOperation{}, false, nil
	}
	if err != nil {
		return domain.DeliveryCheckpointOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	return operation, true, operation.Validate()
}

func validateDeliveryCheckpointReplay(existing,
	request domain.DeliveryCheckpointOperation,
) error {
	if existing.KeyDigest != request.KeyDigest ||
		existing.RequestFingerprint != request.RequestFingerprint ||
		existing.RunID != request.RunID || existing.WorkItemID != request.WorkItemID ||
		existing.RequestedBy != request.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"Delivery checkpoint idempotency key was already used for different intent")
	}
	return nil
}

func (s *SQLiteStore) recoverDeliveryCheckpoint(ctx context.Context,
	operation domain.DeliveryCheckpointOperation, original error,
) (domain.DeliveryCheckpoint, bool, error) {
	existing, found, err := getDeliveryCheckpointOperation(ctx, s.db,
		operation.KeyDigest)
	if err != nil || !found {
		if err == nil {
			return domain.DeliveryCheckpoint{}, false, original
		}
		return domain.DeliveryCheckpoint{}, false, errors.Join(original, err)
	}
	if err := validateDeliveryCheckpointReplay(existing, operation); err != nil {
		return domain.DeliveryCheckpoint{}, false, err
	}
	value, err := s.GetDeliveryCheckpoint(ctx, existing.CheckpointID)
	return value, true, err
}

func requireSelectedWorkItemDeliveryCheckpointTx(ctx context.Context, tx *sql.Tx,
	item domain.WorkItem,
) error {
	var enrolled int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM plan_delivery_selection_items selected
		JOIN plan_delivery_selections selection ON selection.id = selected.selection_id
		JOIN delivery_gate_enrollments enrollment
			ON enrollment.run_id = selection.run_id AND enrollment.selection_id = selection.id
		WHERE selected.work_item_id = ? AND selection.run_id = ?
	)`, item.ID, item.RunID).Scan(&enrolled); err != nil {
		return err
	}
	if enrolled == 0 {
		return nil
	}
	mode, err := getCurrentRunModeSnapshot(ctx, tx, item.RunID)
	if err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"current Deliver mode is required before WorkItem completion", err)
	}
	if mode.Phase != domain.ExecutionPhaseDeliver {
		return apperror.New(apperror.CodeFailedPrecondition,
			"selected WorkItem can only complete in Deliver phase")
	}
	var checkpointCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM delivery_checkpoints checkpoint
		JOIN delivery_checkpoint_operations operation ON operation.checkpoint_id = checkpoint.id
		WHERE checkpoint.run_id = ? AND checkpoint.work_item_id = ? AND checkpoint.work_item_version = ?
			AND mode_snapshot_id = ? AND mode_revision = ?`, item.RunID, item.ID,
		item.Version, mode.ID, mode.Revision).Scan(&checkpointCount); err != nil {
		return err
	}
	if checkpointCount != 1 {
		return apperror.New(apperror.CodeFailedPrecondition,
			"selected WorkItem requires a checkpoint for its current version and Deliver revision")
	}
	return nil
}

func requireDeliverySelectionCompletionTx(ctx context.Context, tx *sql.Tx,
	runID string,
) error {
	var enrolled int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1
		FROM delivery_gate_enrollments WHERE run_id = ?)`, runID).Scan(&enrolled); err != nil {
		return err
	}
	if enrolled == 0 {
		return nil
	}
	var incomplete int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM plan_delivery_selection_items selected
		JOIN plan_delivery_selections selection ON selection.id = selected.selection_id
		JOIN work_items work ON work.id = selected.work_item_id
		WHERE selection.run_id = ? AND (
			work.status != 'completed' OR NOT EXISTS (
				SELECT 1 FROM delivery_checkpoints checkpoint
				JOIN delivery_checkpoint_operations operation
					ON operation.checkpoint_id = checkpoint.id
				JOIN run_mode_snapshots mode ON mode.id = checkpoint.mode_snapshot_id
				WHERE checkpoint.run_id = selection.run_id
					AND checkpoint.selection_id = selection.id
					AND checkpoint.work_item_id = work.id
					AND checkpoint.work_item_version = work.version - 1
					AND mode.run_id = selection.run_id
					AND mode.revision = checkpoint.mode_revision
					AND mode.phase = 'deliver'
			)
		)`, runID).Scan(&incomplete); err != nil {
		return err
	}
	if incomplete != 0 {
		return apperror.New(apperror.CodeFailedPrecondition,
			fmt.Sprintf("Run has %d incomplete Delivery checkpoint gate(s)", incomplete))
	}
	return nil
}
