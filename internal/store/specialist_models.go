package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/session"
)

const (
	maxSpecialistModelAttempts   = 5
	maxSpecialistModelInputBytes = 32 * 1024
)

const specialistModelCallSelect = `SELECT agent_attempt_id, run_id, agent_id,
	model_attempt_number, transport_attempt, max_attempts, provider, model, status, outcome,
	input_fingerprint, action_fingerprint,
	error_text, retry_after_millis, retry_planned, elapsed_millis, stream_events, stream_bytes,
	input_tokens, output_tokens, total_tokens, usage_recorded, action_kind, report_outcome,
	policy_allowed, policy_needs_approval, policy_risk, policy_reason,
	user_message_id, assistant_message_id, started_at, finished_at
	FROM specialist_model_calls`

type specialistModelCallRecord struct {
	AgentAttemptID      string
	RunID               string
	AgentID             string
	ModelAttempt        int
	TransportAttempt    int
	MaxAttempts         int
	Provider            string
	Model               string
	InputFingerprint    string
	ActionFingerprint   string
	Status              string
	Outcome             llm.Outcome
	ErrorText           string
	RetryAfterMillis    int64
	RetryPlanned        bool
	ElapsedMillis       int64
	StreamEvents        int
	StreamBytes         int
	Usage               domain.AgentAttemptUsage
	UsageRecorded       bool
	ActionKind          domain.SpecialistActionKind
	ReportOutcome       domain.CompletionOutcome
	PolicyAllowed       int
	PolicyNeedsApproval bool
	PolicyRisk          string
	PolicyReason        string
	UserMessageID       *int64
	AssistantMessageID  *int64
	StartedAt           time.Time
	FinishedAt          *time.Time
}

type specialistModelTerminal struct {
	status             string
	usage              *llm.Usage
	action             *domain.SpecialistAction
	decision           *policy.Decision
	input              string
	userMessageID      *int64
	assistantMessageID *int64
	inputFingerprint   string
	actionFingerprint  string
}

// NextSpecialistModelAttempt returns the next transport attempt within the
// active child turn and the child's durable cumulative model execution time.
func (s *SQLiteStore) NextSpecialistModelAttempt(ctx context.Context,
	ref domain.AgentAttemptRef,
) (int, int64, error) {
	ref = normalizeAgentAttemptRef(ref)
	if err := ref.Validate(); err != nil {
		return 0, 0, apperror.Wrap(apperror.CodeInvalidArgument,
			"Specialist model attempt reference is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, _, _, err := loadActiveAgentAttemptTx(ctx, tx, ref)
	if err != nil {
		return 0, 0, err
	}
	if attempt.UsageRecordedAt != nil {
		return 0, 0, apperror.New(apperror.CodeFailedPrecondition,
			"Specialist model usage was already committed")
	}
	var count, started int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN status = 'started' THEN 1 ELSE 0 END), 0)
		FROM specialist_model_calls WHERE agent_attempt_id = ?`, attempt.ID).
		Scan(&count, &started); err != nil {
		return 0, 0, err
	}
	if started != 0 {
		return 0, 0, apperror.New(apperror.CodeConflict,
			"Specialist already has an active model call")
	}
	var executionMillis int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(elapsed_millis), 0)
		FROM specialist_model_calls WHERE agent_id = ?`, attempt.AgentID).
		Scan(&executionMillis); err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return count + 1, executionMillis, nil
}

func (s *SQLiteStore) RecordSpecialistModelStarted(ctx context.Context,
	ref domain.AgentAttemptRef, modelAttempt llm.ModelAttempt,
) (bool, error) {
	ref = normalizeAgentAttemptRef(ref)
	if err := ref.Validate(); err != nil {
		return false, apperror.Wrap(apperror.CodeInvalidArgument,
			"Specialist model attempt reference is invalid", err)
	}
	modelAttempt = sanitizeModelAttempt(modelAttempt)
	if modelAttempt.Outcome != "" || modelAttempt.ErrorText != "" || modelAttempt.RetryAfter != 0 ||
		modelAttempt.Elapsed != 0 || modelAttempt.RetryPlanned || modelAttempt.StreamEvents != 0 ||
		modelAttempt.StreamBytes != 0 {
		return false, apperror.New(apperror.CodeInvalidArgument,
			"started Specialist model call cannot contain terminal metadata")
	}
	if err := validateSpecialistModelIdentity(modelAttempt); err != nil {
		return false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	attempt, child, run, err := loadActiveAgentAttemptTx(ctx, tx, ref)
	if err != nil {
		return false, err
	}
	if attempt.UsageRecordedAt != nil {
		return false, apperror.New(apperror.CodeFailedPrecondition,
			"Specialist model usage was already committed")
	}
	existing, found, err := getSpecialistModelCallTx(ctx, tx, attempt.ID, modelAttempt.Number)
	if err != nil {
		return false, err
	}
	if found {
		if err := requireSpecialistModelIdentity(existing, modelAttempt); err != nil {
			return false, err
		}
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	var count, active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN status = 'started' THEN 1 ELSE 0 END), 0)
		FROM specialist_model_calls WHERE agent_attempt_id = ?`, attempt.ID).
		Scan(&count, &active); err != nil {
		return false, err
	}
	if active != 0 || modelAttempt.Number != count+1 {
		return false, apperror.New(apperror.CodeConflict,
			"Specialist model attempt is not the next durable call")
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO specialist_model_calls
		(agent_attempt_id, run_id, agent_id, model_attempt_number, transport_attempt,
		max_attempts, provider, model, status, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'started', ?)`, attempt.ID, attempt.RunID,
		attempt.AgentID, modelAttempt.Number, modelAttempt.TransportNumber(), modelAttempt.MaxAttempts,
		modelAttempt.Provider, modelAttempt.Model, ts(now)); err != nil {
		return false, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.ModelStartedEvent,
		"specialist_model_gateway", specialistModelSubject(attempt.ID, modelAttempt.Number), map[string]any{
			"agent_id": child.ID, "parent_agent_id": child.ParentID, "agent_attempt_id": attempt.ID,
			"turn": attempt.Turn, "model_attempt": modelAttempt.Number,
			"transport_attempt": modelAttempt.TransportNumber(), "max_attempts": modelAttempt.MaxAttempts,
			"provider": modelAttempt.Provider, "model": modelAttempt.Model, "context": modelAttempt.Context,
		}); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) RecordSpecialistModelCompleted(ctx context.Context,
	ref domain.AgentAttemptRef, modelAttempt llm.ModelAttempt, response llm.ChatResponse,
	input string, action domain.SpecialistAction, decision policy.Decision,
) (domain.AgentAttempt, error) {
	modelAttempt = sanitizeModelAttempt(modelAttempt)
	modelAttempt.Outcome = llm.OutcomeSuccess
	modelAttempt.ErrorText = ""
	modelAttempt.RetryAfter = 0
	modelAttempt.RetryPlanned = false
	if err := validateSpecialistModelIdentity(modelAttempt); err != nil {
		return domain.AgentAttempt{}, err
	}
	if err := modelAttempt.ValidateCompleted(); err != nil {
		return domain.AgentAttempt{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"completed Specialist model call is invalid", err)
	}
	if len(response.ToolCalls) != 0 {
		return domain.AgentAttempt{}, apperror.New(apperror.CodeFailedPrecondition,
			"Specialist no-tool turn cannot complete with tool calls")
	}
	decoded, err := domain.DecodeSpecialistAction(response.Text)
	if err != nil {
		return domain.AgentAttempt{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"Specialist response does not match its lifecycle action", err)
	}
	normalized, err := domain.NormalizeSpecialistAction(action)
	if err != nil || !sameSpecialistAction(decoded, normalized) {
		return domain.AgentAttempt{}, apperror.New(apperror.CodeConflict,
			"Specialist lifecycle action differs from the provider response")
	}
	safeAction, err := redactSpecialistAction(normalized)
	if err != nil {
		return domain.AgentAttempt{}, err
	}
	decision, err = normalizeSpecialistPolicyDecision(decision)
	if err != nil {
		return domain.AgentAttempt{}, err
	}
	input = redact.String(strings.TrimSpace(input))
	if input == "" || !utf8.ValidString(input) || len([]byte(input)) > maxSpecialistModelInputBytes {
		return domain.AgentAttempt{}, apperror.New(apperror.CodeInvalidArgument,
			"Specialist model input is empty, invalid, or too large")
	}
	if err := response.Usage.Validate(); err != nil {
		return domain.AgentAttempt{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"Specialist provider returned invalid usage", err)
	}
	actionJSON, err := json.Marshal(safeAction)
	if err != nil {
		return domain.AgentAttempt{}, err
	}
	return s.recordSpecialistModelTerminal(ctx, ref, modelAttempt, specialistModelTerminal{
		status: "completed", usage: &response.Usage, action: &safeAction,
		decision:          &decision,
		input:             input,
		inputFingerprint:  runmutation.Fingerprint("specialist_model_input.v1", input),
		actionFingerprint: runmutation.Fingerprint("specialist_model_action.v1", string(actionJSON)),
	})
}

func (s *SQLiteStore) RecordSpecialistModelFailed(ctx context.Context,
	ref domain.AgentAttemptRef, modelAttempt llm.ModelAttempt, usage *llm.Usage,
) (domain.AgentAttempt, error) {
	modelAttempt = sanitizeModelAttempt(modelAttempt)
	if err := validateSpecialistModelIdentity(modelAttempt); err != nil {
		return domain.AgentAttempt{}, err
	}
	if err := modelAttempt.ValidateFailed(); err != nil {
		return domain.AgentAttempt{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"failed Specialist model call is invalid", err)
	}
	if modelAttempt.RetryPlanned && (!modelAttempt.Outcome.Retryable() ||
		modelAttempt.Number >= modelAttempt.MaxAttempts || usage != nil) {
		return domain.AgentAttempt{}, apperror.New(apperror.CodeInvalidArgument,
			"Specialist retry plan conflicts with its terminal outcome")
	}
	if usage != nil {
		copy := *usage
		if err := copy.Validate(); err != nil {
			return domain.AgentAttempt{}, apperror.Wrap(apperror.CodeFailedPrecondition,
				"Specialist provider returned invalid usage", err)
		}
		usage = &copy
	}
	return s.recordSpecialistModelTerminal(ctx, ref, modelAttempt, specialistModelTerminal{
		status: "failed", usage: usage,
	})
}

func (s *SQLiteStore) recordSpecialistModelTerminal(ctx context.Context,
	ref domain.AgentAttemptRef, modelAttempt llm.ModelAttempt,
	terminal specialistModelTerminal,
) (domain.AgentAttempt, error) {
	ref = normalizeAgentAttemptRef(ref)
	if err := ref.Validate(); err != nil {
		return domain.AgentAttempt{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"Specialist model attempt reference is invalid", err)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.AgentAttempt{}, err
	}
	defer func() { _ = tx.Rollback() }()
	currentAttempt, err := scanAgentAttempt(tx.QueryRowContext(ctx,
		agentAttemptSelect+` WHERE id = ?`, ref.AttemptID))
	if err != nil {
		return domain.AgentAttempt{}, err
	}
	if currentAttempt.RunID != ref.RunID || currentAttempt.AgentID != ref.AgentID {
		return domain.AgentAttempt{}, apperror.New(apperror.CodeInvalidArgument,
			"Specialist model attempt identity does not match its Agent attempt")
	}
	call, found, err := getSpecialistModelCallTx(ctx, tx, currentAttempt.ID, modelAttempt.Number)
	if err != nil {
		return domain.AgentAttempt{}, err
	}
	if !found {
		return domain.AgentAttempt{}, apperror.New(apperror.CodeFailedPrecondition,
			"Specialist model call was not started")
	}
	if err := requireSpecialistModelIdentity(call, modelAttempt); err != nil {
		return domain.AgentAttempt{}, err
	}
	if call.Status != "started" {
		if err := requireSpecialistTerminalReplay(call, modelAttempt, terminal); err != nil {
			return domain.AgentAttempt{}, err
		}
		if err := tx.Commit(); err != nil {
			return domain.AgentAttempt{}, err
		}
		return currentAttempt, nil
	}
	attempt, child, run, err := loadActiveAgentAttemptTx(ctx, tx, ref)
	if err != nil {
		return domain.AgentAttempt{}, err
	}
	if attempt.UsageRecordedAt != nil {
		return domain.AgentAttempt{}, apperror.New(apperror.CodeConflict,
			"Specialist model usage was already recorded")
	}
	elapsedMillis, err := supervisorElapsedMillis(modelAttempt.Elapsed)
	if err != nil {
		return domain.AgentAttempt{}, err
	}
	var usage domain.AgentAttemptUsage
	if terminal.usage != nil {
		inputTokens, outputTokens, totalTokens, err := supervisorUsage(*terminal.usage)
		if err != nil {
			return domain.AgentAttempt{}, err
		}
		var priorMillis int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(elapsed_millis), 0)
			FROM specialist_model_calls WHERE agent_attempt_id = ? AND model_attempt_number <> ?`,
			attempt.ID, modelAttempt.Number).Scan(&priorMillis); err != nil {
			return domain.AgentAttempt{}, err
		}
		executionMillis, err := supervisorAddCounter(priorMillis, elapsedMillis,
			"Specialist model execution time")
		if err != nil {
			return domain.AgentAttempt{}, err
		}
		usage = domain.AgentAttemptUsage{
			InputTokens: inputTokens, OutputTokens: outputTokens,
			TotalTokens: totalTokens, ExecutionMillis: executionMillis,
		}
		operationKey := "specialist-usage-" + runmutation.Fingerprint(
			"specialist_model_usage.v1", attempt.RunID, attempt.ID)
		keyDigest := runmutation.Fingerprint("agent_attempt_operation.v1", attempt.RunID,
			operationKey)
		requestFingerprint := runmutation.Fingerprint("agent_attempt_usage.v1", attempt.RunID,
			attempt.AgentID, attempt.ID, fmt.Sprint(usage.InputTokens), fmt.Sprint(usage.OutputTokens),
			fmt.Sprint(usage.TotalTokens), fmt.Sprint(usage.ExecutionMillis))
		if _, _, _, found, err := getAgentAttemptMutationTx(ctx, tx, keyDigest); err != nil {
			return domain.AgentAttempt{}, err
		} else if found {
			return domain.AgentAttempt{}, apperror.New(apperror.CodeConflict,
				"Specialist model usage mutation already exists before terminal commit")
		}
		attempt, child, err = applySpecialistUsageTx(ctx, tx, attempt, child, run, usage,
			keyDigest, requestFingerprint, time.Now().UTC())
		if err != nil {
			return domain.AgentAttempt{}, err
		}
	}
	now := time.Now().UTC()
	var actionKind, reportOutcome string
	policyAllowed := -1
	policyNeedsApproval := false
	policyRisk, policyReason := "", ""
	if terminal.action != nil && terminal.decision != nil {
		actionKind = string(terminal.action.Kind)
		if terminal.action.Report != nil {
			reportOutcome = string(terminal.action.Report.Outcome)
		}
		policyAllowed = 0
		if terminal.decision.Allowed {
			policyAllowed = 1
		}
		policyNeedsApproval = terminal.decision.NeedsApproval
		policyRisk = terminal.decision.Risk
		policyReason = terminal.decision.Reason
		if terminal.decision.Allowed && !terminal.decision.NeedsApproval {
			if child.SessionID == "" {
				return domain.AgentAttempt{}, apperror.New(apperror.CodeFailedPrecondition,
					"Specialist has no durable Session")
			}
			var sessionStatus string
			if err := tx.QueryRowContext(ctx, `SELECT status FROM sessions WHERE id = ?`,
				child.SessionID).Scan(&sessionStatus); err != nil {
				return domain.AgentAttempt{}, err
			}
			if sessionStatus != session.StatusActive {
				return domain.AgentAttempt{}, apperror.New(apperror.CodeConflict,
					"Specialist Session is not active")
			}
			userMessage, err := saveSessionMessageTx(ctx, tx, session.Message{
				SessionID: child.SessionID, Role: "user", Content: terminal.input, CreatedAt: now,
			})
			if err != nil {
				return domain.AgentAttempt{}, err
			}
			assistantMessage, err := saveSessionMessageTx(ctx, tx, session.Message{
				SessionID: child.SessionID, Role: "assistant", Content: terminal.action.Message,
				CreatedAt: now,
			})
			if err != nil {
				return domain.AgentAttempt{}, err
			}
			terminal.userMessageID = &userMessage.ID
			terminal.assistantMessageID = &assistantMessage.ID
		}
	}
	usageRecorded := terminal.usage != nil
	result, err := tx.ExecContext(ctx, `UPDATE specialist_model_calls SET status = ?, outcome = ?,
		input_fingerprint = ?, action_fingerprint = ?, error_text = ?, retry_after_millis = ?,
		retry_planned = ?, elapsed_millis = ?,
		stream_events = ?, stream_bytes = ?, input_tokens = ?, output_tokens = ?, total_tokens = ?,
		usage_recorded = ?, action_kind = ?, report_outcome = ?, policy_allowed = ?,
		policy_needs_approval = ?, policy_risk = ?, policy_reason = ?, user_message_id = ?,
		assistant_message_id = ?, finished_at = ?
		WHERE agent_attempt_id = ? AND model_attempt_number = ? AND status = 'started'`,
		terminal.status, modelAttempt.Outcome, terminal.inputFingerprint,
		terminal.actionFingerprint, modelAttempt.ErrorText,
		modelAttempt.RetryAfter.Milliseconds(), boolInt(modelAttempt.RetryPlanned), elapsedMillis,
		modelAttempt.StreamEvents, modelAttempt.StreamBytes, usage.InputTokens, usage.OutputTokens,
		usage.TotalTokens, boolInt(usageRecorded), actionKind, reportOutcome, policyAllowed,
		boolInt(policyNeedsApproval), policyRisk, policyReason, nullableInt64(terminal.userMessageID),
		nullableInt64(terminal.assistantMessageID), ts(now), attempt.ID, modelAttempt.Number)
	if err != nil {
		return domain.AgentAttempt{}, err
	}
	if err := requireSingleAgentAttemptUpdate(result,
		"Specialist model call changed before terminal commit"); err != nil {
		return domain.AgentAttempt{}, err
	}
	payload := map[string]any{
		"agent_id": attempt.AgentID, "parent_agent_id": attempt.ParentAgentID,
		"agent_attempt_id": attempt.ID, "turn": attempt.Turn,
		"model_attempt": modelAttempt.Number, "transport_attempt": modelAttempt.TransportNumber(),
		"max_attempts": modelAttempt.MaxAttempts, "provider": modelAttempt.Provider,
		"model": modelAttempt.Model, "outcome": modelAttempt.Outcome,
		"elapsed_millis": elapsedMillis, "stream_events": modelAttempt.StreamEvents,
		"stream_bytes": modelAttempt.StreamBytes,
	}
	eventType := events.ModelFailedEvent
	if terminal.status == "completed" {
		eventType = events.ModelCompletedEvent
		payload["usage"] = usage
		payload["action"] = actionKind
		payload["report_outcome"] = reportOutcome
		payload["policy_allowed"] = policyAllowed == 1
		payload["policy_needs_approval"] = policyNeedsApproval
		payload["session_user_message_id"] = terminal.userMessageID
		payload["session_assistant_message_id"] = terminal.assistantMessageID
	} else {
		payload["error"] = modelAttempt.ErrorText
		payload["retry_after_millis"] = modelAttempt.RetryAfter.Milliseconds()
		payload["retry_planned"] = modelAttempt.RetryPlanned
		if usageRecorded {
			payload["usage"] = usage
		}
	}
	subject := specialistModelSubject(attempt.ID, modelAttempt.Number)
	if err := appendSupervisorEventTx(ctx, tx, run, eventType,
		"specialist_model_gateway", subject, payload); err != nil {
		return domain.AgentAttempt{}, err
	}
	if terminal.decision != nil {
		if err := appendSupervisorEventTx(ctx, tx, run, events.PolicyDecisionEvent,
			"policy", subject, map[string]any{
				"context": "specialist_assistant_response", "agent_id": attempt.AgentID,
				"agent_attempt_id": attempt.ID, "allowed": terminal.decision.Allowed,
				"needs_approval": terminal.decision.NeedsApproval,
				"risk":           terminal.decision.Risk, "reason": terminal.decision.Reason,
			}); err != nil {
			return domain.AgentAttempt{}, err
		}
	}
	if usageRecorded {
		if _, err := createAgentGraphSnapshotTx(ctx, tx, run); err != nil {
			return domain.AgentAttempt{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.AgentAttempt{}, err
	}
	return attempt, nil
}

func validateSpecialistModelIdentity(attempt llm.ModelAttempt) error {
	if err := attempt.ValidateStarted(); err != nil {
		return apperror.Wrap(apperror.CodeInvalidArgument,
			"Specialist model call identity is invalid", err)
	}
	if attempt.ProtocolRepair != 0 || attempt.ToolRound != 0 ||
		attempt.Number != attempt.TransportNumber() || attempt.MaxAttempts > maxSpecialistModelAttempts {
		return apperror.New(apperror.CodeInvalidArgument,
			"Specialist no-tool model call has an unsupported attempt shape")
	}
	return nil
}

func getSpecialistModelCallTx(ctx context.Context, tx *sql.Tx, attemptID string,
	modelAttempt int,
) (specialistModelCallRecord, bool, error) {
	record, err := scanSpecialistModelCall(tx.QueryRowContext(ctx,
		specialistModelCallSelect+` WHERE agent_attempt_id = ? AND model_attempt_number = ?`,
		attemptID, modelAttempt))
	if errors.Is(err, sql.ErrNoRows) {
		return specialistModelCallRecord{}, false, nil
	}
	return record, err == nil, err
}

func scanSpecialistModelCall(row scanner) (specialistModelCallRecord, error) {
	var record specialistModelCallRecord
	var outcome, actionKind, reportOutcome, startedAt string
	var retryPlanned, usageRecorded, policyNeedsApproval int
	var userMessageID, assistantMessageID sql.NullInt64
	var finishedAt sql.NullString
	if err := row.Scan(&record.AgentAttemptID, &record.RunID, &record.AgentID,
		&record.ModelAttempt, &record.TransportAttempt, &record.MaxAttempts,
		&record.Provider, &record.Model, &record.Status, &outcome,
		&record.InputFingerprint, &record.ActionFingerprint, &record.ErrorText,
		&record.RetryAfterMillis, &retryPlanned, &record.ElapsedMillis,
		&record.StreamEvents, &record.StreamBytes, &record.Usage.InputTokens,
		&record.Usage.OutputTokens, &record.Usage.TotalTokens, &usageRecorded,
		&actionKind, &reportOutcome, &record.PolicyAllowed, &policyNeedsApproval,
		&record.PolicyRisk, &record.PolicyReason, &userMessageID, &assistantMessageID,
		&startedAt, &finishedAt); err != nil {
		return specialistModelCallRecord{}, err
	}
	record.Outcome = llm.Outcome(outcome)
	record.RetryPlanned = retryPlanned != 0
	record.UsageRecorded = usageRecorded != 0
	record.ActionKind = domain.SpecialistActionKind(actionKind)
	record.ReportOutcome = domain.CompletionOutcome(reportOutcome)
	record.PolicyNeedsApproval = policyNeedsApproval != 0
	if userMessageID.Valid {
		record.UserMessageID = &userMessageID.Int64
	}
	if assistantMessageID.Valid {
		record.AssistantMessageID = &assistantMessageID.Int64
	}
	record.StartedAt = parseTS(startedAt)
	record.FinishedAt = parseNullableTS(finishedAt)
	return record, nil
}

func requireSpecialistModelIdentity(record specialistModelCallRecord,
	attempt llm.ModelAttempt,
) error {
	if record.ModelAttempt != attempt.Number || record.TransportAttempt != attempt.TransportNumber() ||
		record.MaxAttempts != attempt.MaxAttempts || record.Provider != attempt.Provider ||
		record.Model != attempt.Model {
		return apperror.New(apperror.CodeConflict,
			"Specialist model call identity differs from its durable record")
	}
	return nil
}

func requireSpecialistTerminalReplay(record specialistModelCallRecord,
	attempt llm.ModelAttempt, terminal specialistModelTerminal,
) error {
	elapsedMillis, err := supervisorElapsedMillis(attempt.Elapsed)
	if err != nil {
		return err
	}
	if record.Status != terminal.status || record.Outcome != attempt.Outcome ||
		record.ErrorText != attempt.ErrorText ||
		record.RetryAfterMillis != attempt.RetryAfter.Milliseconds() ||
		record.RetryPlanned != attempt.RetryPlanned ||
		record.ElapsedMillis != elapsedMillis ||
		record.StreamEvents != attempt.StreamEvents || record.StreamBytes != attempt.StreamBytes {
		return apperror.New(apperror.CodeConflict,
			"Specialist model terminal replay differs from its durable record")
	}
	if terminal.usage != nil {
		input, output, total, err := supervisorUsage(*terminal.usage)
		if err != nil || !record.UsageRecorded || record.Usage.InputTokens != input ||
			record.Usage.OutputTokens != output || record.Usage.TotalTokens != total {
			return apperror.New(apperror.CodeConflict,
				"Specialist model usage replay differs from its durable record")
		}
	} else if record.UsageRecorded {
		return apperror.New(apperror.CodeConflict,
			"Specialist model replay omitted its durable usage")
	}
	if terminal.action != nil && terminal.decision != nil {
		reportOutcome := domain.CompletionOutcome("")
		if terminal.action.Report != nil {
			reportOutcome = terminal.action.Report.Outcome
		}
		allowed := 0
		if terminal.decision.Allowed {
			allowed = 1
		}
		if record.InputFingerprint != terminal.inputFingerprint ||
			record.ActionFingerprint != terminal.actionFingerprint ||
			record.ActionKind != terminal.action.Kind || record.ReportOutcome != reportOutcome ||
			record.PolicyAllowed != allowed ||
			record.PolicyNeedsApproval != terminal.decision.NeedsApproval ||
			record.PolicyRisk != terminal.decision.Risk ||
			record.PolicyReason != terminal.decision.Reason {
			return apperror.New(apperror.CodeConflict,
				"Specialist model action replay differs from its durable record")
		}
	} else if record.InputFingerprint != "" || record.ActionFingerprint != "" {
		return apperror.New(apperror.CodeConflict,
			"Specialist model replay omitted its durable intent fingerprints")
	}
	return nil
}

func normalizeSpecialistPolicyDecision(decision policy.Decision) (policy.Decision, error) {
	decision.Reason = sanitizeSupervisorText(decision.Reason)
	decision.Risk = sanitizeSupervisorText(decision.Risk)
	if decision.Reason == "" {
		return policy.Decision{}, apperror.New(apperror.CodeInvalidArgument,
			"Specialist policy decision reason is required")
	}
	return decision, nil
}

func redactSpecialistAction(action domain.SpecialistAction) (domain.SpecialistAction, error) {
	action.Message = redact.String(action.Message)
	if action.Report != nil {
		report := *action.Report
		report.Summary = redact.String(report.Summary)
		action.Report = &report
	}
	action, err := domain.NormalizeSpecialistAction(action)
	if err != nil {
		return domain.SpecialistAction{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"redacted Specialist action is invalid", err)
	}
	return action, nil
}

func sameSpecialistAction(left domain.SpecialistAction, right domain.SpecialistAction) bool {
	if left.Version != right.Version || left.Kind != right.Kind || left.Message != right.Message {
		return false
	}
	if left.Report == nil || right.Report == nil {
		return left.Report == nil && right.Report == nil
	}
	return left.Report.Version == right.Report.Version && left.Report.Outcome == right.Report.Outcome &&
		left.Report.Summary == right.Report.Summary &&
		slices.Equal(left.Report.WorkItemIDs, right.Report.WorkItemIDs) &&
		slices.Equal(left.Report.NoteIDs, right.Report.NoteIDs)
}

func specialistModelSubject(attemptID string, modelAttempt int) string {
	return fmt.Sprintf("%s:model:%d", strings.TrimSpace(attemptID), modelAttempt)
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
