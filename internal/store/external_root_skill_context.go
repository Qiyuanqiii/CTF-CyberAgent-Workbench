package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/skills"
)

const externalRootContextPreparationSelect = `SELECT id, run_id, mission_id,
	root_agent_id, supervisor_attempt_id, turn_number, selection_id,
	protocol_version, surface, profile, selection_fingerprint, context_fingerprint,
	item_count, token_budget, token_upper_bound, redaction_count, prepared_at
	FROM root_external_skill_context_preparations`

const externalRootContextCommitSelect = `SELECT preparation_id, run_id,
	supervisor_attempt_id, model_attempt, committed_at
	FROM root_external_skill_context_commits`

func (s *SQLiteStore) PrepareExternalRootSkillContext(ctx context.Context,
	checkpoint domain.SupervisorCheckpoint,
	request skills.ExternalRootContextPreparationRequest,
) (skills.ExternalRootContextPreparation, error) {
	if err := checkpoint.Validate(); err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return skills.ExternalRootContextPreparation{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"only a started Supervisor turn can prepare external root Skill context")
	}
	if err := request.Validate(); err != nil {
		return skills.ExternalRootContextPreparation{}, apperror.Wrap(
			apperror.CodeInvalidArgument, "external root Skill context is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSkillContextWriteLockTx(ctx, tx, checkpoint.RunID); err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	run, current, err := requireActiveSupervisorAttemptTx(ctx, tx, checkpoint)
	if err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	root, found, err := getRootAgentTx(ctx, tx, run.ID)
	if err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	if !found || root.Role != domain.AgentRoleRoot || root.Status != domain.AgentRunning ||
		root.ActiveAttemptID != current.AttemptID || request.RunID != run.ID ||
		request.MissionID != run.MissionID || request.RootAgentID != root.ID ||
		request.SupervisorAttemptID != current.AttemptID || request.Turn != current.NextTurn {
		return skills.ExternalRootContextPreparation{}, apperror.New(
			apperror.CodeConflict,
			"external root Skill context does not match the active Supervisor turn")
	}
	selection, found, err := getExternalSkillSelectionByRun(ctx, tx, run.ID)
	if err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	if !found {
		return skills.ExternalRootContextPreparation{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"external root Skill context requires a persisted external selection")
	}
	if err := validateExternalRootContextSelection(selection, request); err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	existing, found, err := getExternalRootContextPreparationByAttemptTx(ctx, tx,
		run.ID, current.AttemptID)
	if err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	if found {
		if existing.ExternalRootContextPreparationRequest != request {
			return skills.ExternalRootContextPreparation{}, apperror.New(
				apperror.CodeConflict,
				"prepared external root Skill context does not match reconstructed context")
		}
		existing.Recovered = true
		if err := tx.Commit(); err != nil {
			return skills.ExternalRootContextPreparation{}, err
		}
		return existing, nil
	}
	preparation := skills.ExternalRootContextPreparation{
		ID:                                    idgen.New("external-skillctx"),
		ExternalRootContextPreparationRequest: request,
		PreparedAt:                            time.Now().UTC(),
	}
	if err := preparation.Validate(); err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO root_external_skill_context_preparations
		(id, run_id, mission_id, root_agent_id, supervisor_attempt_id, turn_number,
		selection_id, protocol_version, surface, profile, selection_fingerprint,
		context_fingerprint, item_count, token_budget, token_upper_bound,
		redaction_count, prepared_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		preparation.ID, request.RunID, request.MissionID, request.RootAgentID,
		request.SupervisorAttemptID, request.Turn, request.SelectionID,
		request.ProtocolVersion, request.Surface, request.Profile,
		request.SelectionFingerprint, request.ContextFingerprint, request.ItemCount,
		request.TokenBudget, request.TokenUpperBound, request.RedactionCount,
		ts(preparation.PreparedAt)); err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run,
		events.ExternalSkillContextPreparedEvent, "external_skills", preparation.ID,
		externalRootContextEventPayload(preparation, 0)); err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	if err := tx.Commit(); err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	return preparation, nil
}

func commitExternalRootSkillContextTx(ctx context.Context, tx *sql.Tx,
	run domain.Run, checkpoint domain.SupervisorCheckpoint, modelAttempt int,
) error {
	selection, selected, err := getExternalSkillSelectionByRun(ctx, tx, run.ID)
	if err != nil {
		return err
	}
	preparation, prepared, err := getExternalRootContextPreparationByAttemptTx(ctx, tx,
		run.ID, checkpoint.AttemptID)
	if err != nil {
		return err
	}
	if !selected {
		if prepared {
			return apperror.New(apperror.CodeFailedPrecondition,
				"external root Skill context exists without a selection")
		}
		return nil
	}
	if !prepared {
		return apperror.New(apperror.CodeFailedPrecondition,
			"persisted external Skill selection was not prepared for the root model call")
	}
	if err := validateExternalRootContextSelection(selection,
		preparation.ExternalRootContextPreparationRequest); err != nil {
		return err
	}
	root, found, err := getRootAgentTx(ctx, tx, run.ID)
	if err != nil {
		return err
	}
	if !found || root.ID != preparation.RootAgentID || root.Role != domain.AgentRoleRoot ||
		root.Status != domain.AgentRunning || root.ActiveAttemptID != checkpoint.AttemptID ||
		preparation.Turn != checkpoint.NextTurn {
		return apperror.New(apperror.CodeConflict,
			"prepared external root Skill context is not bound to the active root Agent")
	}
	existing, found, err := getExternalRootContextCommitTx(ctx, tx, preparation.ID)
	if err != nil {
		return err
	}
	if found {
		if existing.RunID != run.ID || existing.SupervisorAttemptID != checkpoint.AttemptID ||
			existing.ModelAttempt <= 0 || existing.ModelAttempt > modelAttempt {
			return apperror.New(apperror.CodeConflict,
				"external root Skill context commit does not match the model attempt")
		}
		return nil
	}
	commit := skills.ExternalRootContextCommit{
		PreparationID: preparation.ID, RunID: run.ID,
		SupervisorAttemptID: checkpoint.AttemptID, ModelAttempt: modelAttempt,
		CommittedAt: time.Now().UTC(),
	}
	if err := commit.Validate(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO root_external_skill_context_commits
		(preparation_id, run_id, supervisor_attempt_id, model_attempt, committed_at)
		VALUES (?, ?, ?, ?, ?)`, commit.PreparationID, commit.RunID,
		commit.SupervisorAttemptID, commit.ModelAttempt, ts(commit.CommittedAt)); err != nil {
		return err
	}
	return appendSupervisorEventTx(ctx, tx, run,
		events.ExternalSkillContextCommittedEvent, "external_skills", preparation.ID,
		externalRootContextEventPayload(preparation, modelAttempt))
}

func validateExternalRootContextSelection(selection skills.ExternalSelection,
	request skills.ExternalRootContextPreparationRequest,
) error {
	if err := selection.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"durable external Skill selection is invalid", err)
	}
	if selection.ID != request.SelectionID || selection.RunID != request.RunID ||
		selection.MissionID != request.MissionID || selection.Surface != request.Surface ||
		selection.Profile != request.Profile ||
		selection.Fingerprint != request.SelectionFingerprint ||
		selection.ItemCount != request.ItemCount || selection.TokenBudget != request.TokenBudget ||
		request.TokenUpperBound > selection.TokenUpperBound {
		return apperror.New(apperror.CodeFailedPrecondition,
			"external root Skill context does not match its persisted selection")
	}
	return nil
}

func externalRootContextEventPayload(
	preparation skills.ExternalRootContextPreparation, modelAttempt int,
) map[string]any {
	payload := map[string]any{
		"protocol": preparation.ProtocolVersion, "agent_id": preparation.RootAgentID,
		"turn": preparation.Turn, "surface": preparation.Surface,
		"profile": preparation.Profile, "item_count": preparation.ItemCount,
		"token_budget":      preparation.TokenBudget,
		"token_upper_bound": preparation.TokenUpperBound,
		"redaction_count":   preparation.RedactionCount,
		"trust_class":       skills.PackageTrustOperatorInstalledUntrusted,
		"root_only":         true, "tool_capability_grant": false,
	}
	if modelAttempt > 0 {
		payload["model_attempt"] = modelAttempt
	}
	return payload
}

func getExternalRootContextPreparationByAttemptTx(ctx context.Context, tx *sql.Tx,
	runID, attemptID string,
) (skills.ExternalRootContextPreparation, bool, error) {
	value, err := scanExternalRootContextPreparation(tx.QueryRowContext(ctx,
		externalRootContextPreparationSelect+` WHERE run_id = ? AND supervisor_attempt_id = ?`,
		runID, attemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return skills.ExternalRootContextPreparation{}, false, nil
	}
	return value, err == nil, err
}

func getExternalRootContextCommitTx(ctx context.Context, tx *sql.Tx,
	preparationID string,
) (skills.ExternalRootContextCommit, bool, error) {
	value, err := scanExternalRootContextCommit(tx.QueryRowContext(ctx,
		externalRootContextCommitSelect+` WHERE preparation_id = ?`, preparationID))
	if errors.Is(err, sql.ErrNoRows) {
		return skills.ExternalRootContextCommit{}, false, nil
	}
	return value, err == nil, err
}

func scanExternalRootContextPreparation(row scanner) (
	skills.ExternalRootContextPreparation, error,
) {
	var value skills.ExternalRootContextPreparation
	request := &value.ExternalRootContextPreparationRequest
	var preparedAt string
	if err := row.Scan(&value.ID, &request.RunID, &request.MissionID,
		&request.RootAgentID, &request.SupervisorAttemptID, &request.Turn,
		&request.SelectionID, &request.ProtocolVersion, &request.Surface,
		&request.Profile, &request.SelectionFingerprint, &request.ContextFingerprint,
		&request.ItemCount, &request.TokenBudget, &request.TokenUpperBound,
		&request.RedactionCount, &preparedAt); err != nil {
		return skills.ExternalRootContextPreparation{}, err
	}
	value.PreparedAt = parseTS(preparedAt)
	return value, value.Validate()
}

func scanExternalRootContextCommit(row scanner) (skills.ExternalRootContextCommit, error) {
	var value skills.ExternalRootContextCommit
	var committedAt string
	if err := row.Scan(&value.PreparationID, &value.RunID,
		&value.SupervisorAttemptID, &value.ModelAttempt, &committedAt); err != nil {
		return skills.ExternalRootContextCommit{}, err
	}
	value.CommittedAt = parseTS(committedAt)
	return value, value.Validate()
}
