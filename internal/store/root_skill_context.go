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

const rootSkillContextPreparationSelect = `SELECT id, run_id, mission_id,
	root_agent_id, supervisor_attempt_id, turn_number, selection_id,
	protocol_version, profile, selection_fingerprint, context_fingerprint,
	item_count, token_budget, token_upper_bound, redaction_count, prepared_at
	FROM root_skill_context_preparations`

const rootSkillContextCommitSelect = `SELECT preparation_id, run_id,
	supervisor_attempt_id, model_attempt, committed_at
	FROM root_skill_context_commits`

func (s *SQLiteStore) PrepareRootSkillContext(ctx context.Context,
	checkpoint domain.SupervisorCheckpoint, request skills.RootContextPreparationRequest,
) (skills.RootContextPreparation, error) {
	if err := checkpoint.Validate(); err != nil {
		return skills.RootContextPreparation{}, err
	}
	if checkpoint.Phase != domain.SupervisorTurnStarted {
		return skills.RootContextPreparation{}, apperror.New(apperror.CodeFailedPrecondition,
			"only a started Supervisor turn can prepare root Skill context")
	}
	if err := request.Validate(); err != nil {
		return skills.RootContextPreparation{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"root Skill context preparation is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return skills.RootContextPreparation{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireSkillContextWriteLockTx(ctx, tx, checkpoint.RunID); err != nil {
		return skills.RootContextPreparation{}, err
	}
	run, current, err := requireActiveSupervisorAttemptTx(ctx, tx, checkpoint)
	if err != nil {
		return skills.RootContextPreparation{}, err
	}
	root, found, err := getRootAgentTx(ctx, tx, run.ID)
	if err != nil {
		return skills.RootContextPreparation{}, err
	}
	if !found || root.Role != domain.AgentRoleRoot || root.Status != domain.AgentRunning ||
		root.ActiveAttemptID != current.AttemptID {
		return skills.RootContextPreparation{}, apperror.New(apperror.CodeConflict,
			"root Agent is not bound to the preparing Supervisor attempt")
	}
	if request.RunID != run.ID || request.MissionID != run.MissionID ||
		request.RootAgentID != root.ID || request.SupervisorAttemptID != current.AttemptID ||
		request.Turn != current.NextTurn {
		return skills.RootContextPreparation{}, apperror.New(apperror.CodeInvalidArgument,
			"root Skill context scope does not match the active Supervisor turn")
	}
	selection, found, err := getSkillSelectionByRun(ctx, tx, run.ID)
	if err != nil {
		return skills.RootContextPreparation{}, err
	}
	if !found {
		return skills.RootContextPreparation{}, apperror.New(apperror.CodeFailedPrecondition,
			"root Skill context requires a persisted Run selection")
	}
	if err := validateRootSkillContextSelection(selection, request); err != nil {
		return skills.RootContextPreparation{}, err
	}

	existing, found, err := getRootSkillContextPreparationByAttemptTx(ctx, tx,
		request.RunID, request.SupervisorAttemptID)
	if err != nil {
		return skills.RootContextPreparation{}, err
	}
	if found {
		if !sameRootSkillContextRequest(existing.RootContextPreparationRequest, request) {
			return skills.RootContextPreparation{}, apperror.New(apperror.CodeConflict,
				"prepared root Skill context does not match the reconstructed context")
		}
		existing.Recovered = true
		if err := tx.Commit(); err != nil {
			return skills.RootContextPreparation{}, err
		}
		return existing, nil
	}

	preparation := skills.RootContextPreparation{
		ID: idgen.New("skillctx"), RootContextPreparationRequest: request,
		PreparedAt: time.Now().UTC(),
	}
	if err := preparation.Validate(); err != nil {
		return skills.RootContextPreparation{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO root_skill_context_preparations
		(id, run_id, mission_id, root_agent_id, supervisor_attempt_id, turn_number,
		selection_id, protocol_version, profile, selection_fingerprint,
		context_fingerprint, item_count, token_budget, token_upper_bound,
		redaction_count, prepared_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		preparation.ID, request.RunID, request.MissionID, request.RootAgentID,
		request.SupervisorAttemptID, request.Turn, request.SelectionID,
		request.ProtocolVersion, request.Profile, request.SelectionFingerprint,
		request.ContextFingerprint, request.ItemCount, request.TokenBudget,
		request.TokenUpperBound, request.RedactionCount, ts(preparation.PreparedAt)); err != nil {
		return skills.RootContextPreparation{}, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.SkillContextPreparedEvent,
		"skills", preparation.ID, rootSkillContextEventPayload(preparation, 0)); err != nil {
		return skills.RootContextPreparation{}, err
	}
	if err := tx.Commit(); err != nil {
		return skills.RootContextPreparation{}, err
	}
	return preparation, nil
}

func commitRootSkillContextTx(ctx context.Context, tx *sql.Tx, run domain.Run,
	checkpoint domain.SupervisorCheckpoint, modelAttempt int,
) error {
	selection, selected, err := getSkillSelectionByRun(ctx, tx, run.ID)
	if err != nil {
		return err
	}
	preparation, prepared, err := getRootSkillContextPreparationByAttemptTx(ctx, tx,
		run.ID, checkpoint.AttemptID)
	if err != nil {
		return err
	}
	if !selected {
		if prepared {
			return apperror.New(apperror.CodeFailedPrecondition,
				"root Skill context exists without a persisted selection")
		}
		return nil
	}
	if !prepared {
		return apperror.New(apperror.CodeFailedPrecondition,
			"persisted Skill selection was not prepared for the active root turn")
	}
	if err := validateRootSkillContextSelection(selection,
		preparation.RootContextPreparationRequest); err != nil {
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
			"prepared root Skill context is not bound to the active root Agent")
	}
	existing, found, err := getRootSkillContextCommitTx(ctx, tx, preparation.ID)
	if err != nil {
		return err
	}
	if found {
		if existing.RunID != run.ID || existing.SupervisorAttemptID != checkpoint.AttemptID ||
			existing.ModelAttempt <= 0 || existing.ModelAttempt > modelAttempt {
			return apperror.New(apperror.CodeConflict,
				"root Skill context commit does not match the model attempt")
		}
		return nil
	}
	commit := skills.RootContextCommit{
		PreparationID: preparation.ID, RunID: run.ID,
		SupervisorAttemptID: checkpoint.AttemptID, ModelAttempt: modelAttempt,
		CommittedAt: time.Now().UTC(),
	}
	if err := commit.Validate(); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO root_skill_context_commits
		(preparation_id, run_id, supervisor_attempt_id, model_attempt, committed_at)
		VALUES (?, ?, ?, ?, ?)`, commit.PreparationID, commit.RunID,
		commit.SupervisorAttemptID, commit.ModelAttempt, ts(commit.CommittedAt)); err != nil {
		return err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.SkillContextCommittedEvent,
		"skills", preparation.ID, rootSkillContextEventPayload(preparation,
			commit.ModelAttempt)); err != nil {
		return err
	}
	return nil
}

func rootSkillContextEventPayload(preparation skills.RootContextPreparation,
	modelAttempt int,
) map[string]any {
	payload := map[string]any{
		"protocol": preparation.ProtocolVersion, "agent_id": preparation.RootAgentID,
		"turn": preparation.Turn, "item_count": preparation.ItemCount,
		"token_budget":      preparation.TokenBudget,
		"token_upper_bound": preparation.TokenUpperBound,
		"redaction_count":   preparation.RedactionCount,
		"root_only":         true, "tool_capability_grant": false,
	}
	if modelAttempt > 0 {
		payload["model_attempt"] = modelAttempt
	}
	return payload
}

func validateRootSkillContextSelection(selection skills.Selection,
	request skills.RootContextPreparationRequest,
) error {
	if err := selection.Validate(); err != nil {
		return apperror.Wrap(apperror.CodeFailedPrecondition,
			"durable Skill selection is invalid", err)
	}
	if selection.ID != request.SelectionID || selection.RunID != request.RunID ||
		selection.MissionID != request.MissionID || selection.Profile != request.Profile ||
		selection.Fingerprint != request.SelectionFingerprint ||
		selection.ItemCount != request.ItemCount || selection.TokenBudget != request.TokenBudget ||
		request.TokenUpperBound > selection.TokenUpperBound {
		return apperror.New(apperror.CodeFailedPrecondition,
			"root Skill context does not match its persisted selection")
	}
	return nil
}

func acquireSkillContextWriteLockTx(ctx context.Context, tx *sql.Tx, runID string) error {
	result, err := tx.ExecContext(ctx, `UPDATE runs SET updated_at = updated_at WHERE id = ?`, runID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return apperror.New(apperror.CodeNotFound, "root Skill context Run was not found")
	}
	return nil
}

func getRootSkillContextPreparationByAttemptTx(ctx context.Context, tx *sql.Tx,
	runID string, attemptID string,
) (skills.RootContextPreparation, bool, error) {
	item, err := scanRootSkillContextPreparation(tx.QueryRowContext(ctx,
		rootSkillContextPreparationSelect+` WHERE run_id = ? AND supervisor_attempt_id = ?`,
		runID, attemptID))
	if errors.Is(err, sql.ErrNoRows) {
		return skills.RootContextPreparation{}, false, nil
	}
	return item, err == nil, err
}

func getRootSkillContextCommitTx(ctx context.Context, tx *sql.Tx,
	preparationID string,
) (skills.RootContextCommit, bool, error) {
	item, err := scanRootSkillContextCommit(tx.QueryRowContext(ctx,
		rootSkillContextCommitSelect+` WHERE preparation_id = ?`, preparationID))
	if errors.Is(err, sql.ErrNoRows) {
		return skills.RootContextCommit{}, false, nil
	}
	return item, err == nil, err
}

func scanRootSkillContextPreparation(row scanner) (skills.RootContextPreparation, error) {
	var preparation skills.RootContextPreparation
	var profile string
	var preparedAt string
	request := &preparation.RootContextPreparationRequest
	if err := row.Scan(&preparation.ID, &request.RunID, &request.MissionID,
		&request.RootAgentID, &request.SupervisorAttemptID, &request.Turn,
		&request.SelectionID, &request.ProtocolVersion, &profile,
		&request.SelectionFingerprint, &request.ContextFingerprint,
		&request.ItemCount, &request.TokenBudget, &request.TokenUpperBound,
		&request.RedactionCount, &preparedAt); err != nil {
		return skills.RootContextPreparation{}, err
	}
	request.Profile = domain.Profile(profile)
	preparation.PreparedAt = parseTS(preparedAt)
	return preparation, preparation.Validate()
}

func scanRootSkillContextCommit(row scanner) (skills.RootContextCommit, error) {
	var commit skills.RootContextCommit
	var committedAt string
	if err := row.Scan(&commit.PreparationID, &commit.RunID,
		&commit.SupervisorAttemptID, &commit.ModelAttempt, &committedAt); err != nil {
		return skills.RootContextCommit{}, err
	}
	commit.CommittedAt = parseTS(committedAt)
	return commit, commit.Validate()
}

func sameRootSkillContextRequest(left skills.RootContextPreparationRequest,
	right skills.RootContextPreparationRequest,
) bool {
	return left == right
}
