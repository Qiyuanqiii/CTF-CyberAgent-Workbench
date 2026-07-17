package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/skills"
)

const externalSkillSelectionSelect = `SELECT id, run_id, mission_id,
	mode_snapshot_id, mode_revision, protocol_version, surface, profile,
	token_budget, token_upper_bound, item_count, selection_fingerprint,
	requested_by, operator_confirmed, context_delivery_authorized,
	tool_capability_grant, created_at FROM run_external_skill_selections`

func (s *SQLiteStore) CreateExternalSkillSelection(ctx context.Context,
	selection skills.ExternalSelection, operation skills.ExternalSelectionOperation,
	event events.Event,
) (skills.ExternalSelection, bool, error) {
	selection = skills.CloneExternalSelection(selection)
	if err := validateExternalSkillSelectionMutation(selection, operation, event); err != nil {
		return skills.ExternalSelection{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return skills.ExternalSelection{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSkillSelectionWriteLockTx(ctx, tx, selection.RunID); err != nil {
		return skills.ExternalSelection{}, false, err
	}
	if existing, found, err := getExternalSkillSelectionOperation(ctx, tx,
		operation.KeyDigest); err != nil {
		return skills.ExternalSelection{}, false, err
	} else if found {
		if err := validateExternalSkillSelectionReplay(existing, operation); err != nil {
			return skills.ExternalSelection{}, false, err
		}
		stored, err := getExternalSkillSelection(ctx, tx, existing.SelectionID)
		if err != nil {
			return skills.ExternalSelection{}, false, err
		}
		if err := validateExternalSkillSelectionOperationBinding(existing, stored); err != nil {
			return skills.ExternalSelection{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return skills.ExternalSelection{}, false, err
		}
		return stored, true, nil
	}
	if _, found, err := getExternalSkillSelectionByRun(ctx, tx,
		selection.RunID); err != nil {
		return skills.ExternalSelection{}, false, err
	} else if found {
		return skills.ExternalSelection{}, false, apperror.New(apperror.CodeConflict,
			"run already has an immutable external Skill selection")
	}
	run, mission, mode, err := requireExternalSkillSelectionBindingTx(ctx, tx, selection)
	if err != nil {
		return skills.ExternalSelection{}, false, err
	}
	if event.RunID != run.ID || event.MissionID != mission.ID ||
		mode.ID != selection.ModeSnapshotID || !event.CreatedAt.Equal(selection.CreatedAt) {
		return skills.ExternalSelection{}, false, apperror.New(apperror.CodeInvalidArgument,
			"external Skill selection event scope or timestamp does not match")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_external_skill_selections
		(id, run_id, mission_id, mode_snapshot_id, mode_revision, protocol_version,
		surface, profile, token_budget, token_upper_bound, item_count,
		selection_fingerprint, requested_by, operator_confirmed,
		context_delivery_authorized, tool_capability_grant, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		selection.ID, selection.RunID, selection.MissionID, selection.ModeSnapshotID,
		selection.ModeRevision, selection.ProtocolVersion, selection.Surface,
		selection.Profile, selection.TokenBudget, selection.TokenUpperBound,
		selection.ItemCount, selection.Fingerprint, selection.RequestedBy,
		boolInt(selection.OperatorConfirmed), boolInt(selection.ContextDeliveryAuthorized),
		boolInt(selection.ToolCapabilityGrant), ts(selection.CreatedAt)); err != nil {
		return skills.ExternalSelection{}, false, err
	}
	for _, item := range selection.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO run_external_skill_selection_items
			(selection_id, ordinal, installation_id, installation_fingerprint,
			install_result_fingerprint, name, version, surface, content_sha256,
			content_bytes, token_upper_bound, archive_sha256, archive_bytes,
			package_fingerprint, object_key, trust_class, tool_dependency_count,
			specialist_eligible) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.SelectionID, item.Ordinal, item.InstallationID,
			item.InstallationFingerprint, item.InstallResultFingerprint, item.Name,
			item.Version, item.Surface, item.ContentSHA256, item.ContentBytes,
			item.TokenUpperBound, item.ArchiveSHA256, item.ArchiveBytes,
			item.PackageFingerprint, item.ObjectKey, item.TrustClass,
			item.ToolDependencyCount, boolInt(item.SpecialistEligible)); err != nil {
			return skills.ExternalSelection{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_external_skill_selection_operations
		(operation_key_digest, request_fingerprint, selection_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.SelectionID, operation.RunID,
		operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverExternalSkillSelection(ctx, operation, err)
	}
	if _, err := insertRunEventTx(ctx, tx, event); err != nil {
		return skills.ExternalSelection{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return skills.ExternalSelection{}, false, err
	}
	return skills.CloneExternalSelection(selection), false, nil
}

func (s *SQLiteStore) GetExternalSkillSelection(ctx context.Context,
	id string,
) (skills.ExternalSelection, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return skills.ExternalSelection{}, apperror.New(apperror.CodeInvalidArgument,
			"external Skill selection id is invalid")
	}
	return getExternalSkillSelection(ctx, s.db, id)
}

func (s *SQLiteStore) GetExternalSkillSelectionByRun(ctx context.Context,
	runID string,
) (skills.ExternalSelection, bool, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return skills.ExternalSelection{}, false, apperror.New(
			apperror.CodeInvalidArgument, "external Skill selection Run id is invalid")
	}
	return getExternalSkillSelectionByRun(ctx, s.db, runID)
}

func (s *SQLiteStore) GetExternalSkillSelectionOperation(ctx context.Context,
	keyDigest string,
) (skills.ExternalSelectionOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return skills.ExternalSelectionOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "external Skill selection operation digest is invalid")
	}
	return getExternalSkillSelectionOperation(ctx, s.db, keyDigest)
}

func (s *SQLiteStore) Load(ctx context.Context,
	descriptor skills.PackageObjectDescriptor,
) (skills.LoadedPackageObject, error) {
	if s == nil || strings.TrimSpace(s.home) == "" {
		return skills.LoadedPackageObject{}, errors.New("skill package object home is unavailable")
	}
	objects, err := skills.NewLocalPackageObjectStore(s.home)
	if err != nil {
		return skills.LoadedPackageObject{}, err
	}
	return objects.Load(ctx, descriptor)
}

func validateExternalSkillSelectionMutation(selection skills.ExternalSelection,
	operation skills.ExternalSelectionOperation, event events.Event,
) error {
	if err := selection.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"external Skill selection is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"external Skill selection operation is invalid", err)
	}
	if operation.SelectionID != selection.ID || operation.RunID != selection.RunID ||
		operation.RequestedBy != selection.RequestedBy ||
		!operation.CreatedAt.Equal(selection.CreatedAt) ||
		operation.RequestFingerprint != skills.ExternalSelectionRequestFingerprint(selection) {
		return apperror.New(apperror.CodeInvalidArgument,
			"external Skill selection operation does not match its selection")
	}
	if err := validateExternalSkillSelectionEvent(event, selection); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"external Skill selection event is invalid", err)
	}
	return nil
}

func validateExternalSkillSelectionEvent(event events.Event,
	selection skills.ExternalSelection,
) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if event.Type != events.ExternalSkillSelectionCreatedEvent ||
		event.Source != "external_skills" || event.SubjectID != selection.ID {
		return errors.New("external Skill selection event identity is invalid")
	}
	if err := rejectDuplicateSkillSelectionEventFields(event.PayloadJSON); err != nil {
		return err
	}
	var payload struct {
		Protocol            string                  `json:"protocol"`
		Surface             domain.ExecutionSurface `json:"surface"`
		Profile             domain.Profile          `json:"profile"`
		ItemCount           int                     `json:"item_count"`
		TokenBudget         int                     `json:"token_budget"`
		TokenUpperBound     int                     `json:"token_upper_bound"`
		OperatorConfirmed   *bool                   `json:"operator_confirmed"`
		ContextDelivery     *bool                   `json:"context_delivery"`
		ToolCapabilityGrant *bool                   `json:"tool_capability_grant"`
	}
	decoder := json.NewDecoder(strings.NewReader(event.PayloadJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("external Skill selection event contains trailing data")
	}
	if payload.Protocol != selection.ProtocolVersion || payload.Surface != selection.Surface ||
		payload.Profile != selection.Profile || payload.ItemCount != selection.ItemCount ||
		payload.TokenBudget != selection.TokenBudget ||
		payload.TokenUpperBound != selection.TokenUpperBound ||
		payload.OperatorConfirmed == nil || !*payload.OperatorConfirmed ||
		payload.ContextDelivery == nil || !*payload.ContextDelivery ||
		payload.ToolCapabilityGrant == nil || *payload.ToolCapabilityGrant {
		return errors.New("external Skill selection event does not match its capability boundary")
	}
	return nil
}

func requireExternalSkillSelectionBindingTx(ctx context.Context, tx *sql.Tx,
	selection skills.ExternalSelection,
) (domain.Run, domain.Mission, domain.RunModeSnapshot, error) {
	run, mission, err := getCoordinatorRunTx(ctx, tx, selection.RunID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, domain.RunModeSnapshot{}, err
	}
	mode, err := getCurrentRunModeSnapshot(ctx, tx, run.ID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, domain.RunModeSnapshot{}, err
	}
	if run.MissionID != selection.MissionID || mission.ID != selection.MissionID ||
		mission.Profile != selection.Profile || run.Status != domain.RunCreated ||
		mode.ID != selection.ModeSnapshotID || mode.Revision != selection.ModeRevision ||
		mode.Surface != selection.Surface || mode.Profile != selection.Profile ||
		selection.CreatedAt.Before(run.CreatedAt) {
		return domain.Run{}, domain.Mission{}, domain.RunModeSnapshot{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"external Skill selection requires its created Run and current matching mode")
	}
	return run, mission, mode, nil
}

func getExternalSkillSelection(ctx context.Context, queryer skillSelectionQueryer,
	id string,
) (skills.ExternalSelection, error) {
	selection, err := scanExternalSkillSelection(queryer.QueryRowContext(ctx,
		externalSkillSelectionSelect+` WHERE id = ?`, id))
	if err != nil {
		return skills.ExternalSelection{}, err
	}
	if err := loadExternalSkillSelectionItems(ctx, queryer, &selection); err != nil {
		return skills.ExternalSelection{}, err
	}
	return selection, selection.Validate()
}

func getExternalSkillSelectionByRun(ctx context.Context, queryer skillSelectionQueryer,
	runID string,
) (skills.ExternalSelection, bool, error) {
	selection, err := scanExternalSkillSelection(queryer.QueryRowContext(ctx,
		externalSkillSelectionSelect+` WHERE run_id = ?`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return skills.ExternalSelection{}, false, nil
	}
	if err != nil {
		return skills.ExternalSelection{}, false, err
	}
	if err := loadExternalSkillSelectionItems(ctx, queryer, &selection); err != nil {
		return skills.ExternalSelection{}, false, err
	}
	return selection, true, selection.Validate()
}

func scanExternalSkillSelection(scanner interface{ Scan(...any) error }) (
	skills.ExternalSelection, error,
) {
	var value skills.ExternalSelection
	var confirmed, delivery, grant int
	var createdAt string
	err := scanner.Scan(&value.ID, &value.RunID, &value.MissionID, &value.ModeSnapshotID,
		&value.ModeRevision, &value.ProtocolVersion, &value.Surface, &value.Profile,
		&value.TokenBudget, &value.TokenUpperBound, &value.ItemCount, &value.Fingerprint,
		&value.RequestedBy, &confirmed, &delivery, &grant, &createdAt)
	if err != nil {
		return skills.ExternalSelection{}, err
	}
	value.OperatorConfirmed = confirmed != 0
	value.ContextDeliveryAuthorized = delivery != 0
	value.ToolCapabilityGrant = grant != 0
	value.CreatedAt = parseTS(createdAt)
	return value, nil
}

func loadExternalSkillSelectionItems(ctx context.Context, queryer skillSelectionQueryer,
	selection *skills.ExternalSelection,
) error {
	rows, err := queryer.QueryContext(ctx, `SELECT selection_id, ordinal,
		installation_id, installation_fingerprint, install_result_fingerprint,
		name, version, surface, content_sha256, content_bytes, token_upper_bound,
		archive_sha256, archive_bytes, package_fingerprint, object_key, trust_class,
		tool_dependency_count, specialist_eligible
		FROM run_external_skill_selection_items WHERE selection_id = ? ORDER BY ordinal`,
		selection.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := make([]skills.ExternalSelectionItem, 0, selection.ItemCount)
	for rows.Next() {
		var item skills.ExternalSelectionItem
		var specialist int
		if err := rows.Scan(&item.SelectionID, &item.Ordinal, &item.InstallationID,
			&item.InstallationFingerprint, &item.InstallResultFingerprint, &item.Name,
			&item.Version, &item.Surface, &item.ContentSHA256, &item.ContentBytes,
			&item.TokenUpperBound, &item.ArchiveSHA256, &item.ArchiveBytes,
			&item.PackageFingerprint, &item.ObjectKey, &item.TrustClass,
			&item.ToolDependencyCount, &specialist); err != nil {
			return err
		}
		item.SpecialistEligible = specialist != 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	selection.Items = items
	return nil
}

func getExternalSkillSelectionOperation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (skills.ExternalSelectionOperation, bool, error) {
	var operation skills.ExternalSelectionOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest,
		request_fingerprint, selection_id, run_id, requested_by, created_at
		FROM run_external_skill_selection_operations WHERE operation_key_digest = ?`,
		keyDigest).Scan(&operation.KeyDigest, &operation.RequestFingerprint,
		&operation.SelectionID, &operation.RunID, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return skills.ExternalSelectionOperation{}, false, nil
	}
	if err != nil {
		return skills.ExternalSelectionOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	return operation, true, operation.Validate()
}

func validateExternalSkillSelectionReplay(existing,
	request skills.ExternalSelectionOperation,
) error {
	if existing.KeyDigest != request.KeyDigest ||
		existing.RequestFingerprint != request.RequestFingerprint ||
		existing.RunID != request.RunID || existing.RequestedBy != request.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"external Skill selection operation key was already used for different intent")
	}
	return nil
}

func validateExternalSkillSelectionOperationBinding(
	operation skills.ExternalSelectionOperation, selection skills.ExternalSelection,
) error {
	if operation.SelectionID != selection.ID || operation.RunID != selection.RunID ||
		operation.RequestedBy != selection.RequestedBy ||
		!operation.CreatedAt.Equal(selection.CreatedAt) ||
		operation.RequestFingerprint != skills.ExternalSelectionRequestFingerprint(selection) {
		return apperror.New(apperror.CodeInternal,
			"stored external Skill selection operation binding is invalid")
	}
	return nil
}

func (s *SQLiteStore) recoverExternalSkillSelection(ctx context.Context,
	operation skills.ExternalSelectionOperation, original error,
) (skills.ExternalSelection, bool, error) {
	existing, found, err := getExternalSkillSelectionOperation(ctx, s.db,
		operation.KeyDigest)
	if err != nil || !found {
		if err == nil {
			return skills.ExternalSelection{}, false, original
		}
		return skills.ExternalSelection{}, false, errors.Join(original, err)
	}
	if err := validateExternalSkillSelectionReplay(existing, operation); err != nil {
		return skills.ExternalSelection{}, false, err
	}
	selection, err := s.GetExternalSkillSelection(ctx, existing.SelectionID)
	if err != nil {
		return skills.ExternalSelection{}, false, err
	}
	if err := validateExternalSkillSelectionOperationBinding(existing, selection); err != nil {
		return skills.ExternalSelection{}, false, err
	}
	return selection, true, nil
}
