package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
)

func TestSpecialistModelLedgerCommitsUsagePolicyAndSessionExactlyOnce(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st, "Specialist model ledger", 2, 64)
	attempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")), "model-ledger-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	ref := attemptRef(attempt)
	next, elapsed, err := st.NextSpecialistModelAttempt(ctx, ref)
	if err != nil || next != 1 || elapsed != 0 {
		t.Fatalf("unexpected next model attempt: next=%d elapsed=%d err=%v", next, elapsed, err)
	}
	modelAttempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3,
		Provider: "mock", Model: "mock-code",
	}
	inserted, err := st.RecordSpecialistModelStarted(ctx, ref, modelAttempt)
	if err != nil || !inserted {
		t.Fatalf("model call did not start: inserted=%t err=%v", inserted, err)
	}
	inserted, err = st.RecordSpecialistModelStarted(ctx, ref, modelAttempt)
	if err != nil || inserted {
		t.Fatalf("model start replay was not stable: inserted=%t err=%v", inserted, err)
	}
	action := domain.SpecialistAction{
		Version: domain.SpecialistLifecycleVersion, Kind: domain.SpecialistActionContinue,
		Message: "continue with the bounded analysis",
	}
	modelAttempt.Elapsed = 2 * time.Millisecond
	response := llm.ChatResponse{
		Text:  specialistActionResponse(t, action),
		Usage: llm.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5},
	}
	decision := policy.Decision{Allowed: true, Reason: "allowed in Specialist test", Risk: "low"}
	charged, err := st.RecordSpecialistModelCompleted(ctx, ref, modelAttempt, response,
		"inspect only the assigned mission", action, decision)
	if err != nil || charged.UsageRecordedAt == nil || charged.Usage.TotalTokens != 5 ||
		charged.Usage.ExecutionMillis != 2 {
		t.Fatalf("model terminal usage was not committed: attempt=%#v err=%v", charged, err)
	}
	replayed, err := st.RecordSpecialistModelCompleted(ctx, ref, modelAttempt, response,
		"inspect only the assigned mission", action, decision)
	if err != nil || replayed.ID != charged.ID || replayed.Usage != charged.Usage {
		t.Fatalf("model terminal replay was not stable: attempt=%#v err=%v", replayed, err)
	}
	if _, err := st.RecordSpecialistModelCompleted(ctx, ref, modelAttempt, response,
		"a different Specialist input", action, decision); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed-input model replay was accepted: code=%s err=%v", apperror.CodeOf(err), err)
	}
	messages, err := st.ListSessionMessages(ctx, fixture.Child.SessionID, true)
	if err != nil || len(messages) != 2 || messages[0].Role != "user" ||
		messages[1].Role != "assistant" || messages[1].Content != action.Message {
		t.Fatalf("Specialist Session messages are invalid: messages=%#v err=%v", messages, err)
	}
	if _, _, err := st.RecordSpecialistAttemptUsage(ctx, ref,
		domain.AgentAttemptUsage{TotalTokens: 1}, "model-ledger-double-usage-0001"); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("model usage was charged twice: code=%s err=%v", apperror.CodeOf(err), err)
	}
	continued, _, err := st.ContinueSpecialistAttempt(ctx, ref,
		"model-ledger-continue-0001")
	if err != nil || continued.Status != domain.AgentAttemptContinued {
		t.Fatalf("committed model turn did not continue: attempt=%#v err=%v", continued, err)
	}
	var status, actionKind string
	var policyAllowed, userMessageID, assistantMessageID int64
	if err := st.db.QueryRowContext(ctx, `SELECT status, action_kind, policy_allowed,
		user_message_id, assistant_message_id FROM specialist_model_calls
		WHERE agent_attempt_id = ? AND model_attempt_number = 1`, attempt.ID).
		Scan(&status, &actionKind, &policyAllowed, &userMessageID, &assistantMessageID); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || actionKind != "continue" || policyAllowed != 1 ||
		userMessageID <= 0 || assistantMessageID <= 0 {
		t.Fatalf("model ledger row is invalid: status=%s action=%s allowed=%d messages=%d/%d",
			status, actionKind, policyAllowed, userMessageID, assistantMessageID)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE specialist_model_calls SET outcome = 'permanent'
		WHERE agent_attempt_id = ? AND model_attempt_number = 1`, attempt.ID); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("terminal model row was mutable: %v", err)
	}
	timeline, err := st.ListRunEvents(ctx, fixture.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countRunEventType(timeline, events.ModelStartedEvent) != 1 ||
		countRunEventType(timeline, events.ModelCompletedEvent) != 1 ||
		countRunEventType(timeline, events.PolicyDecisionEvent) != 1 ||
		countRunEventType(timeline, events.AgentAttemptUsageRecordedEvent) != 1 {
		t.Fatalf("Specialist model audit stream is incomplete: %#v", timeline)
	}
}

func TestSpecialistModelLedgerRetriesThenFinishesWithCumulativeElapsed(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st, "Specialist model retry", 1, 64)
	attempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")), "model-retry-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	ref := attemptRef(attempt)
	first := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "retry", Model: "model",
	}
	if inserted, err := st.RecordSpecialistModelStarted(ctx, ref, first); err != nil || !inserted {
		t.Fatalf("first retry model call did not start: inserted=%t err=%v", inserted, err)
	}
	first.Outcome = llm.OutcomeRetryable
	first.ErrorText = "temporary reset"
	first.Elapsed = 3 * time.Millisecond
	first.RetryPlanned = true
	if failed, err := st.RecordSpecialistModelFailed(ctx, ref, first, nil); err != nil ||
		failed.UsageRecordedAt != nil {
		t.Fatalf("retryable model failure was not recorded: attempt=%#v err=%v", failed, err)
	}
	next, elapsed, err := st.NextSpecialistModelAttempt(ctx, ref)
	if err != nil || next != 2 || elapsed != 3 {
		t.Fatalf("retry ledger did not advance: next=%d elapsed=%d err=%v", next, elapsed, err)
	}
	second := llm.ModelAttempt{
		Number: 2, TransportAttempt: 2, MaxAttempts: 3, Provider: "retry", Model: "model",
	}
	if inserted, err := st.RecordSpecialistModelStarted(ctx, ref, second); err != nil || !inserted {
		t.Fatalf("second retry model call did not start: inserted=%t err=%v", inserted, err)
	}
	report := domain.CompletionReport{
		Version: domain.CompletionReportVersion, Outcome: domain.CompletionSucceeded,
		Summary: "Specialist retry completed", WorkItemIDs: []string{}, NoteIDs: []string{},
	}
	action := domain.SpecialistAction{
		Version: domain.SpecialistLifecycleVersion, Kind: domain.SpecialistActionFinish,
		Message: "completed after retry", Report: &report,
	}
	second.Elapsed = 4 * time.Millisecond
	response := llm.ChatResponse{
		Text:  specialistActionResponse(t, action),
		Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
	}
	charged, err := st.RecordSpecialistModelCompleted(ctx, ref, second, response,
		"finish the assigned mission", action,
		policy.Decision{Allowed: true, Reason: "allowed after retry"})
	if err != nil || charged.Usage.ExecutionMillis != 7 {
		t.Fatalf("retry elapsed time was not accumulated: attempt=%#v err=%v", charged, err)
	}
	completion, replayed, err := st.FinishSpecialist(ctx, domain.AgentCompletion{
		ID: idgen.New("completion"), RunID: attempt.RunID, AgentID: attempt.AgentID,
		ParentAgentID: attempt.ParentAgentID, AttemptID: attempt.ID, Report: report,
		MessageID: idgen.New("agentmsg"), CreatedAt: time.Now().UTC(),
	}, "model-retry-finish-0001")
	if err != nil || replayed || completion.Report.Outcome != domain.CompletionSucceeded {
		t.Fatalf("Specialist did not finish: completion=%#v replayed=%t err=%v",
			completion, replayed, err)
	}
}

func TestSpecialistInvalidModelResponseChargesUsageAndRedactsFailure(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st, "invalid Specialist response", 2, 16)
	attempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")), "invalid-model-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	modelAttempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 1, Provider: "invalid", Model: "model",
	}
	ref := attemptRef(attempt)
	if inserted, err := st.RecordSpecialistModelStarted(ctx, ref, modelAttempt); err != nil || !inserted {
		t.Fatalf("invalid response model call did not start: inserted=%t err=%v", inserted, err)
	}
	secret := "sk-" + strings.Repeat("z", 32)
	modelAttempt.Outcome = llm.OutcomeInvalidResponse
	modelAttempt.ErrorText = "invalid JSON included " + secret
	modelAttempt.Elapsed = time.Millisecond
	usage := llm.Usage{InputTokens: 3, OutputTokens: 3, TotalTokens: 6}
	charged, err := st.RecordSpecialistModelFailed(ctx, ref, modelAttempt, &usage)
	if err != nil || charged.UsageRecordedAt == nil || charged.Usage.TotalTokens != 6 {
		t.Fatalf("invalid response usage was not charged: attempt=%#v err=%v", charged, err)
	}
	crashed, _, err := st.CrashSpecialistAttempt(ctx, domain.AgentAttemptFailureRequest{
		Ref: ref, Failure: domain.AgentAttemptFailure{
			Code: "invalid_response", Reason: "Specialist lifecycle response was invalid",
		}, NotificationMessageID: idgen.New("agentmsg"), FailedAt: time.Now().UTC(),
	}, "invalid-model-crash-0001")
	if err != nil || crashed.Status != domain.AgentAttemptCrashed {
		t.Fatalf("invalid model attempt did not crash: attempt=%#v err=%v", crashed, err)
	}
	var errorText string
	if err := st.db.QueryRowContext(ctx, `SELECT error_text FROM specialist_model_calls
		WHERE agent_attempt_id = ? AND model_attempt_number = 1`, attempt.ID).Scan(&errorText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(errorText, secret) || !strings.Contains(errorText, "[REDACTED:api-key]") {
		t.Fatalf("model failure was not redacted: %q", errorText)
	}
}

func TestSpecialistModelSQLiteTriggersRejectSkippedAndStaleTerminalWrites(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st, "Specialist model triggers", 2, 16)
	attempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")), "model-trigger-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	ref := attemptRef(attempt)
	started := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "mock", Model: "model",
	}
	if inserted, err := st.RecordSpecialistModelStarted(ctx, ref, started); err != nil || !inserted {
		t.Fatalf("model call did not start: inserted=%t err=%v", inserted, err)
	}
	if _, err := st.db.ExecContext(ctx, `INSERT INTO specialist_model_calls
		(agent_attempt_id, run_id, agent_id, model_attempt_number, transport_attempt,
		max_attempts, provider, model, status, started_at)
		VALUES (?, ?, ?, 3, 3, 3, 'mock', 'model', 'started', ?)`, attempt.ID,
		attempt.RunID, attempt.AgentID, ts(time.Now().UTC())); err == nil ||
		!strings.Contains(err.Error(), "next attempt") {
		t.Fatalf("SQLite accepted a skipped model attempt: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE specialist_model_calls SET status = 'failed',
		outcome = 'invalid_response', error_text = 'forged usage', elapsed_millis = 1,
		input_tokens = 1, output_tokens = 1, total_tokens = 2, usage_recorded = 1,
		finished_at = ? WHERE agent_attempt_id = ? AND model_attempt_number = 1`,
		ts(time.Now().UTC()), attempt.ID); err == nil || !strings.Contains(err.Error(), "usage state") {
		t.Fatalf("SQLite accepted model usage without Agent usage: %v", err)
	}
	if _, _, err := st.ReleaseRunExecutionLease(ctx, fixture.Lease); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE specialist_model_calls SET status = 'failed',
		outcome = 'permanent', error_text = 'stale worker', elapsed_millis = 1,
		finished_at = ? WHERE agent_attempt_id = ? AND model_attempt_number = 1`,
		ts(time.Now().UTC()), attempt.ID); err == nil || !strings.Contains(err.Error(), "usage state") {
		t.Fatalf("SQLite accepted a stale model terminal write: %v", err)
	}
}

func TestSchemaV26PreservesSpecialistRuntimeAndAddsModelLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v25.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixtureWithoutLease(t, ctx, st,
		"Specialist model migration", 1, 16)
	for _, statement := range removeSchemaV26ForTestStatements() {
		if _, err := st.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("downgrade v26 fixture with %q: %v", statement, err)
		}
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if version, err := st.SchemaVersion(ctx); err != nil || version != LatestSchemaVersion {
		t.Fatalf("v25 database did not upgrade: version=%d err=%v", version, err)
	}
	fixture.Lease = acquireTestRunExecutionLease(t, ctx, st, fixture.Run.ID)
	attempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")), "migrated-model-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	inserted, err := st.RecordSpecialistModelStarted(ctx, attemptRef(attempt), llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 1, Provider: "mock", Model: "model",
	})
	if err != nil || !inserted {
		t.Fatalf("migrated model ledger is unusable: inserted=%t err=%v", inserted, err)
	}
}

func specialistActionResponse(t testing.TB, action domain.SpecialistAction) string {
	t.Helper()
	encoded, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
