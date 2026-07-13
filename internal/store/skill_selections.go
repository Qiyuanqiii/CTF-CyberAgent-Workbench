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

const skillSelectionSelect = `SELECT id, run_id, mission_id, protocol_version,
	profile, token_budget, token_upper_bound, item_count, selection_fingerprint,
	requested_by, created_at FROM run_skill_selections`

type skillSelectionQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func (s *SQLiteStore) CreateSkillSelection(ctx context.Context, selection skills.Selection,
	operation skills.SelectionOperation, event events.Event,
) (skills.Selection, bool, error) {
	selection = skills.CloneSelection(selection)
	if err := validateSkillSelectionMutation(selection, operation, event); err != nil {
		return skills.Selection{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return skills.Selection{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSkillSelectionWriteLockTx(ctx, tx, selection.RunID); err != nil {
		return skills.Selection{}, false, err
	}
	if existing, found, err := getSkillSelectionOperation(ctx, tx, operation.KeyDigest); err != nil {
		return skills.Selection{}, false, err
	} else if found {
		if err := validateSkillSelectionReplay(existing, operation); err != nil {
			return skills.Selection{}, false, err
		}
		stored, err := getSkillSelection(ctx, tx, existing.SelectionID)
		if err != nil {
			return skills.Selection{}, false, err
		}
		if err := validateSkillSelectionOperationBinding(existing, stored); err != nil {
			return skills.Selection{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return skills.Selection{}, false, err
		}
		return stored, true, nil
	}
	if _, found, err := getSkillSelectionByRun(ctx, tx, selection.RunID); err != nil {
		return skills.Selection{}, false, err
	} else if found {
		return skills.Selection{}, false, apperror.New(apperror.CodeConflict,
			"run already has an immutable Skill selection")
	}
	run, mission, err := requireSkillSelectionBindingTx(ctx, tx, selection)
	if err != nil {
		return skills.Selection{}, false, err
	}
	if event.RunID != run.ID || event.MissionID != mission.ID ||
		!event.CreatedAt.Equal(selection.CreatedAt) {
		return skills.Selection{}, false, apperror.New(apperror.CodeInvalidArgument,
			"skill selection event scope or timestamp does not match")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_skill_selections
		(id, run_id, mission_id, protocol_version, profile, token_budget,
		token_upper_bound, item_count, selection_fingerprint, requested_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, selection.ID, selection.RunID,
		selection.MissionID, selection.ProtocolVersion, selection.Profile,
		selection.TokenBudget, selection.TokenUpperBound, selection.ItemCount,
		selection.Fingerprint, selection.RequestedBy, ts(selection.CreatedAt)); err != nil {
		return skills.Selection{}, false, err
	}
	for _, item := range selection.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO run_skill_selection_items
			(selection_id, ordinal, name, version, content_sha256, content_bytes,
			token_upper_bound) VALUES (?, ?, ?, ?, ?, ?, ?)`, item.SelectionID,
			item.Ordinal, item.Name, item.Version, item.ContentSHA256,
			item.ContentBytes, item.TokenUpperBound); err != nil {
			return skills.Selection{}, false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_skill_selection_operations
		(operation_key_digest, request_fingerprint, selection_id, run_id,
		requested_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, operation.KeyDigest,
		operation.RequestFingerprint, operation.SelectionID, operation.RunID,
		operation.RequestedBy, ts(operation.CreatedAt)); err != nil {
		_ = tx.Rollback()
		return s.recoverSkillSelection(ctx, operation, err)
	}
	if _, err := insertRunEventTx(ctx, tx, event); err != nil {
		return skills.Selection{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return skills.Selection{}, false, err
	}
	return skills.CloneSelection(selection), false, nil
}

func (s *SQLiteStore) GetSkillSelection(ctx context.Context, id string) (skills.Selection, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) || strings.ContainsRune(id, 0) {
		return skills.Selection{}, apperror.New(apperror.CodeInvalidArgument,
			"skill selection id is invalid")
	}
	return getSkillSelection(ctx, s.db, id)
}

func (s *SQLiteStore) GetSkillSelectionByRun(ctx context.Context,
	runID string,
) (skills.Selection, bool, error) {
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return skills.Selection{}, false, apperror.New(apperror.CodeInvalidArgument,
			"skill selection Run id is invalid")
	}
	return getSkillSelectionByRun(ctx, s.db, runID)
}

func (s *SQLiteStore) GetSkillSelectionOperation(ctx context.Context,
	keyDigest string,
) (skills.SelectionOperation, bool, error) {
	keyDigest = strings.TrimSpace(keyDigest)
	if !validStoreDigest(keyDigest) {
		return skills.SelectionOperation{}, false, apperror.New(
			apperror.CodeInvalidArgument, "skill selection operation digest is invalid")
	}
	return getSkillSelectionOperation(ctx, s.db, keyDigest)
}

func validateSkillSelectionMutation(selection skills.Selection,
	operation skills.SelectionOperation, event events.Event,
) error {
	if err := selection.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument, "skill selection is invalid", err)
	}
	if err := operation.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"skill selection operation is invalid", err)
	}
	if operation.SelectionID != selection.ID || operation.RunID != selection.RunID ||
		operation.RequestedBy != selection.RequestedBy ||
		!operation.CreatedAt.Equal(selection.CreatedAt) ||
		operation.RequestFingerprint != skills.SelectionRequestFingerprint(selection) {
		return apperror.New(apperror.CodeInvalidArgument,
			"skill selection operation does not match its selection")
	}
	if err := validateSkillSelectionEvent(event, selection); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"skill selection event is invalid", err)
	}
	return nil
}

func validateSkillSelectionEvent(event events.Event, selection skills.Selection) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if event.Type != events.SkillSelectionCreatedEvent || event.Source != "skills" ||
		event.SubjectID != selection.ID {
		return errors.New("skill selection event identity is invalid")
	}
	if err := rejectDuplicateSkillSelectionEventFields(event.PayloadJSON); err != nil {
		return err
	}
	var payload struct {
		Protocol            string         `json:"protocol"`
		Profile             domain.Profile `json:"profile"`
		ItemCount           int            `json:"item_count"`
		TokenBudget         int            `json:"token_budget"`
		TokenUpperBound     int            `json:"token_upper_bound"`
		ContextInjection    *bool          `json:"context_injection"`
		ToolCapabilityGrant *bool          `json:"tool_capability_grant"`
	}
	decoder := json.NewDecoder(strings.NewReader(event.PayloadJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("skill selection event contains trailing data")
	}
	if payload.Protocol != selection.ProtocolVersion || payload.Profile != selection.Profile ||
		payload.ItemCount != selection.ItemCount || payload.TokenBudget != selection.TokenBudget ||
		payload.TokenUpperBound != selection.TokenUpperBound ||
		payload.ContextInjection == nil || *payload.ContextInjection ||
		payload.ToolCapabilityGrant == nil || *payload.ToolCapabilityGrant {
		return errors.New("skill selection event does not match its closed capability boundary")
	}
	return nil
}

func rejectDuplicateSkillSelectionEventFields(payloadJSON string) error {
	decoder := json.NewDecoder(strings.NewReader(payloadJSON))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return errors.New("skill selection event payload must be a JSON object")
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return errors.New("skill selection event field is invalid")
		}
		field, ok := token.(string)
		if !ok {
			return errors.New("skill selection event field name is invalid")
		}
		if _, exists := seen[field]; exists {
			return errors.New("skill selection event contains a duplicate field")
		}
		seen[field] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return errors.New("skill selection event field value is invalid")
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return errors.New("skill selection event payload is not closed")
	}
	return nil
}

func acquireSkillSelectionWriteLockTx(ctx context.Context, tx *sql.Tx, runID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`, runID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return apperror.New(apperror.CodeNotFound, "skill selection Run was not found")
	}
	return nil
}

func requireSkillSelectionBindingTx(ctx context.Context, tx *sql.Tx,
	selection skills.Selection,
) (domain.Run, domain.Mission, error) {
	run, mission, err := getCoordinatorRunTx(ctx, tx, selection.RunID)
	if err != nil {
		return domain.Run{}, domain.Mission{}, err
	}
	if run.MissionID != selection.MissionID || mission.ID != selection.MissionID ||
		mission.Profile != selection.Profile || run.Status != domain.RunCreated ||
		selection.CreatedAt.Before(run.CreatedAt) {
		return domain.Run{}, domain.Mission{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"skill selection requires its created Run and matching Mission Profile")
	}
	return run, mission, nil
}

func validateSkillSelectionReplay(existing, request skills.SelectionOperation) error {
	if existing.KeyDigest != request.KeyDigest ||
		existing.RequestFingerprint != request.RequestFingerprint ||
		existing.RunID != request.RunID || existing.RequestedBy != request.RequestedBy {
		return apperror.New(apperror.CodeConflict,
			"skill selection operation key was already used for different intent")
	}
	return nil
}

func validateSkillSelectionOperationBinding(operation skills.SelectionOperation,
	selection skills.Selection,
) error {
	if operation.SelectionID != selection.ID || operation.RunID != selection.RunID ||
		operation.RequestedBy != selection.RequestedBy ||
		!operation.CreatedAt.Equal(selection.CreatedAt) ||
		operation.RequestFingerprint != skills.SelectionRequestFingerprint(selection) {
		return apperror.New(apperror.CodeInternal,
			"stored Skill selection operation binding is invalid")
	}
	return nil
}

func (s *SQLiteStore) recoverSkillSelection(ctx context.Context,
	operation skills.SelectionOperation, original error,
) (skills.Selection, bool, error) {
	existing, found, err := getSkillSelectionOperation(ctx, s.db, operation.KeyDigest)
	if err != nil || !found {
		if err == nil {
			return skills.Selection{}, false, original
		}
		return skills.Selection{}, false, errors.Join(original, err)
	}
	if err := validateSkillSelectionReplay(existing, operation); err != nil {
		return skills.Selection{}, false, err
	}
	selection, err := s.GetSkillSelection(ctx, existing.SelectionID)
	if err != nil {
		return skills.Selection{}, false, err
	}
	if err := validateSkillSelectionOperationBinding(existing, selection); err != nil {
		return skills.Selection{}, false, err
	}
	return selection, true, nil
}

func getSkillSelection(ctx context.Context, queryer skillSelectionQueryer,
	id string,
) (skills.Selection, error) {
	selection, err := scanSkillSelection(queryer.QueryRowContext(ctx,
		skillSelectionSelect+` WHERE id = ?`, id))
	if err != nil {
		return skills.Selection{}, err
	}
	if err := loadSkillSelectionItems(ctx, queryer, &selection); err != nil {
		return skills.Selection{}, err
	}
	return selection, selection.Validate()
}

func getSkillSelectionByRun(ctx context.Context, queryer skillSelectionQueryer,
	runID string,
) (skills.Selection, bool, error) {
	selection, err := scanSkillSelection(queryer.QueryRowContext(ctx,
		skillSelectionSelect+` WHERE run_id = ?`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return skills.Selection{}, false, nil
	}
	if err != nil {
		return skills.Selection{}, false, err
	}
	if err := loadSkillSelectionItems(ctx, queryer, &selection); err != nil {
		return skills.Selection{}, false, err
	}
	return selection, true, selection.Validate()
}

func scanSkillSelection(scanner interface{ Scan(...any) error }) (skills.Selection, error) {
	var selection skills.Selection
	var createdAt string
	err := scanner.Scan(&selection.ID, &selection.RunID, &selection.MissionID,
		&selection.ProtocolVersion, &selection.Profile, &selection.TokenBudget,
		&selection.TokenUpperBound, &selection.ItemCount, &selection.Fingerprint,
		&selection.RequestedBy, &createdAt)
	if err != nil {
		return skills.Selection{}, err
	}
	selection.CreatedAt = parseTS(createdAt)
	return selection, nil
}

func loadSkillSelectionItems(ctx context.Context, queryer skillSelectionQueryer,
	selection *skills.Selection,
) error {
	rows, err := queryer.QueryContext(ctx, `SELECT selection_id, ordinal, name,
		version, content_sha256, content_bytes, token_upper_bound
		FROM run_skill_selection_items WHERE selection_id = ? ORDER BY ordinal`, selection.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	items := make([]skills.SelectionItem, 0, selection.ItemCount)
	for rows.Next() {
		var item skills.SelectionItem
		if err := rows.Scan(&item.SelectionID, &item.Ordinal, &item.Name,
			&item.Version, &item.ContentSHA256, &item.ContentBytes,
			&item.TokenUpperBound); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	selection.Items = items
	return nil
}

func getSkillSelectionOperation(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, keyDigest string) (skills.SelectionOperation, bool, error) {
	var operation skills.SelectionOperation
	var createdAt string
	err := queryer.QueryRowContext(ctx, `SELECT operation_key_digest,
		request_fingerprint, selection_id, run_id, requested_by, created_at
		FROM run_skill_selection_operations WHERE operation_key_digest = ?`, keyDigest).
		Scan(&operation.KeyDigest, &operation.RequestFingerprint, &operation.SelectionID,
			&operation.RunID, &operation.RequestedBy, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return skills.SelectionOperation{}, false, nil
	}
	if err != nil {
		return skills.SelectionOperation{}, false, err
	}
	operation.CreatedAt = parseTS(createdAt)
	return operation, true, operation.Validate()
}
