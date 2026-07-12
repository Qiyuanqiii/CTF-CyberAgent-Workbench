package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/session"
)

const (
	maxSpecialistHistoryMessages = 12
	maxSpecialistHistoryBytes    = 64 * 1024
	maxSpecialistInputBytes      = 32 * 1024
	maxSpecialistGoalRunes       = 8 * 1024
	maxSpecialistStreamChunks    = 4096
	defaultSpecialistOutputLimit = 4096
)

type SpecialistRunnerStore interface {
	RunExecutionLeaseStore
	GetRun(ctx context.Context, id string) (domain.Run, error)
	GetMission(ctx context.Context, id string) (domain.Mission, error)
	GetAgentNode(ctx context.Context, id string) (domain.AgentNode, error)
	GetSession(ctx context.Context, id string) (session.Session, error)
	ListRecentSessionMessages(ctx context.Context, sessionID string,
		includeCompacted bool, limit int) ([]session.Message, error)
	BeginSpecialistAttempt(ctx context.Context, start domain.AgentAttemptStart,
		operationKey string) (domain.AgentAttempt, bool, error)
	RecoverSpecialistAttempts(ctx context.Context,
		lease domain.RunExecutionLease) ([]domain.AgentAttempt, error)
	NextSpecialistModelAttempt(ctx context.Context,
		ref domain.AgentAttemptRef) (int, int64, error)
	RecordSpecialistModelStarted(ctx context.Context, ref domain.AgentAttemptRef,
		attempt llm.ModelAttempt) (bool, error)
	RecordSpecialistModelCompleted(ctx context.Context, ref domain.AgentAttemptRef,
		attempt llm.ModelAttempt, response llm.ChatResponse, input string,
		action domain.SpecialistAction, decision policy.Decision) (domain.AgentAttempt, error)
	RecordSpecialistModelFailed(ctx context.Context, ref domain.AgentAttemptRef,
		attempt llm.ModelAttempt, usage *llm.Usage) (domain.AgentAttempt, error)
	ContinueSpecialistAttempt(ctx context.Context, ref domain.AgentAttemptRef,
		operationKey string) (domain.AgentAttempt, bool, error)
	CrashSpecialistAttempt(ctx context.Context, request domain.AgentAttemptFailureRequest,
		operationKey string) (domain.AgentAttempt, bool, error)
	FinishSpecialist(ctx context.Context, completion domain.AgentCompletion,
		operationKey string) (domain.AgentCompletion, bool, error)
}

type SpecialistTurnResult struct {
	RunID             string
	AgentID           string
	ParentAgentID     string
	SessionID         string
	AttemptID         string
	Turn              int64
	AttemptStatus     domain.AgentAttemptStatus
	Action            domain.SpecialistAction
	Completion        domain.AgentCompletion
	Provider          string
	Model             string
	Usage             domain.AgentAttemptUsage
	ModelAttempts     int
	ModelOutcome      llm.Outcome
	StreamEvents      int
	StreamBytes       int
	RecoveredAttempts int
}

// SpecialistRunner is an opt-in, internal-only no-tool child runtime. No CLI,
// HTTP, or model-controlled spawn path constructs it.
type SpecialistRunner struct {
	store       SpecialistRunnerStore
	router      *llm.Router
	checker     policy.Checker
	retryPolicy ModelRetryPolicy
	leaseOwner  string
	leasePolicy RunExecutionLeasePolicy
}

func NewSpecialistRunner(store SpecialistRunnerStore, router *llm.Router,
	checker policy.Checker,
) *SpecialistRunner {
	return &SpecialistRunner{
		store: store, router: router, checker: checker,
		retryPolicy: DefaultModelRetryPolicy(), leaseOwner: idgen.New("specialist-worker"),
		leasePolicy: DefaultRunExecutionLeasePolicy(),
	}
}

func (r *SpecialistRunner) WithModelRetryPolicy(policy ModelRetryPolicy) *SpecialistRunner {
	if r != nil {
		r.retryPolicy = normalizeModelRetryPolicy(policy)
	}
	return r
}

func (r *SpecialistRunner) WithRunExecutionLeasePolicy(
	policy RunExecutionLeasePolicy,
) *SpecialistRunner {
	if r != nil {
		r.leasePolicy = policy
	}
	return r
}

func (r *SpecialistRunner) WithRunExecutionLeaseOwner(ownerID string) *SpecialistRunner {
	if r != nil {
		r.leaseOwner = strings.TrimSpace(ownerID)
	}
	return r
}

func (r *SpecialistRunner) Step(ctx context.Context, runID string,
	agentID string,
) (SpecialistTurnResult, error) {
	result := SpecialistTurnResult{
		RunID: strings.TrimSpace(runID), AgentID: strings.TrimSpace(agentID),
	}
	if r == nil || r.store == nil || r.router == nil || r.checker == nil {
		return result, apperror.New(apperror.CodeFailedPrecondition,
			"Specialist runner dependencies are required")
	}
	if result.RunID == "" || result.AgentID == "" {
		return result, apperror.New(apperror.CodeInvalidArgument,
			"Specialist run id and Agent id are required")
	}
	run, err := r.store.GetRun(ctx, result.RunID)
	if err != nil {
		return result, apperror.Normalize(err)
	}
	if run.Status != domain.RunRunning {
		return result, apperror.New(apperror.CodeFailedPrecondition,
			fmt.Sprintf("run %s is %s; Specialist requires running", run.ID, run.Status))
	}
	child, err := r.store.GetAgentNode(ctx, result.AgentID)
	if err != nil {
		return result, apperror.Normalize(err)
	}
	if child.RunID != run.ID || child.Role != domain.AgentRoleSpecialist || child.ParentID == "" ||
		(child.Status != domain.AgentReady && child.Status != domain.AgentRunning) {
		return result, apperror.New(apperror.CodeFailedPrecondition,
			"Specialist runner requires a ready child or a recoverable running child")
	}
	result.ParentAgentID = child.ParentID
	result.SessionID = child.SessionID
	err = withRunExecutionLease(ctx, r.store, run.ID, r.leaseOwner, r.leasePolicy,
		func(leaseCtx context.Context, lease domain.RunExecutionLease) error {
			return r.stepWithLease(leaseCtx, lease, &result)
		})
	return result, apperror.Normalize(err)
}

func (r *SpecialistRunner) stepWithLease(ctx context.Context,
	lease domain.RunExecutionLease, result *SpecialistTurnResult,
) error {
	recovered, err := r.store.RecoverSpecialistAttempts(ctx, lease)
	if err != nil {
		return apperror.Normalize(err)
	}
	result.RecoveredAttempts = len(recovered)
	child, err := r.store.GetAgentNode(ctx, result.AgentID)
	if err != nil {
		return apperror.Normalize(err)
	}
	if child.RunID != result.RunID || child.Role != domain.AgentRoleSpecialist ||
		child.ParentID != result.ParentAgentID || child.Status != domain.AgentReady {
		return apperror.New(apperror.CodeFailedPrecondition,
			"Specialist is not ready after lease recovery")
	}
	attemptID := idgen.New("attempt")
	attempt, _, err := r.store.BeginSpecialistAttempt(ctx, domain.AgentAttemptStart{
		AttemptID: attemptID, RunID: child.RunID, AgentID: child.ID,
		ParentAgentID: child.ParentID, Lease: lease, StartedAt: time.Now().UTC(),
	}, "specialist-start-"+attemptID)
	if err != nil {
		return apperror.Normalize(err)
	}
	result.AttemptID = attempt.ID
	result.Turn = attempt.Turn
	result.AttemptStatus = attempt.Status
	ref := specialistAttemptRef(attempt)
	run, err := r.store.GetRun(ctx, child.RunID)
	if err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	mission, err := r.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	childSession, err := r.store.GetSession(ctx, child.SessionID)
	if err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	if childSession.Status != session.StatusActive {
		return r.failAttempt(ctx, result, ref,
			apperror.New(apperror.CodeFailedPrecondition, "Specialist Session is not active"))
	}
	history, err := r.store.ListRecentSessionMessages(ctx, child.SessionID, false,
		maxSpecialistHistoryMessages)
	if err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	input, err := specialistTurnInput(mission, child, attempt)
	if err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	request, err := specialistRequest(history, input, child)
	if err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	refModel, err := supervisorModelRef(r.router, run.Config.ModelRoute)
	if err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	request.MaxTokens, err = specialistOutputLimit(child)
	if err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	modelCall, err := r.callModelWithRetry(ctx, run, ref, refModel, request)
	result.ModelAttempts = modelCall.Attempt.Number
	result.ModelOutcome = modelCall.Attempt.Outcome
	result.StreamEvents = modelCall.StreamEvents
	result.StreamBytes = modelCall.StreamBytes
	result.Provider = modelCall.Attempt.Provider
	result.Model = modelCall.Attempt.Model
	if err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	response := modelCall.Response
	if response == nil {
		return r.recordInvalidAndFail(ctx, result, ref, modelCall.Attempt, nil,
			errors.New("provider returned an empty Specialist response"))
	}
	if len(response.ToolCalls) != 0 {
		return r.recordInvalidAndFail(ctx, result, ref, modelCall.Attempt, &response.Usage,
			errors.New("specialist no-tool turn returned tool calls"))
	}
	action, err := domain.DecodeSpecialistAction(response.Text)
	if err != nil {
		return r.recordInvalidAndFail(ctx, result, ref, modelCall.Attempt, &response.Usage, err)
	}
	safeAction, err := safeSpecialistAction(action)
	if err != nil {
		return r.recordInvalidAndFail(ctx, result, ref, modelCall.Attempt, &response.Usage, err)
	}
	decision := r.checker.CheckText("specialist_assistant_response",
		specialistActionPolicyText(safeAction))
	modelCall.Attempt.Outcome = llm.OutcomeSuccess
	eventCtx, cancelEvent := specialistEventContext(ctx)
	charged, storeErr := r.store.RecordSpecialistModelCompleted(eventCtx, ref,
		modelCall.Attempt, *response, input, action, decision)
	cancelEvent()
	if storeErr != nil {
		return r.failAttempt(ctx, result, ref, storeErr)
	}
	result.Usage = charged.Usage
	result.ModelOutcome = llm.OutcomeSuccess
	result.Action = safeAction
	if err := ctx.Err(); err != nil {
		return r.failAttempt(ctx, result, ref, err)
	}
	if !decision.Allowed || decision.NeedsApproval {
		return r.failAttemptWithCode(ctx, result, ref, "policy_denied",
			apperror.New(apperror.CodePolicyDenied,
				"policy denied Specialist response: "+decision.Reason))
	}
	switch safeAction.Kind {
	case domain.SpecialistActionContinue:
		continued, _, err := r.store.ContinueSpecialistAttempt(ctx, ref,
			"specialist-continue-"+attempt.ID)
		if err != nil {
			return r.failAttempt(ctx, result, ref, err)
		}
		result.AttemptStatus = continued.Status
		return nil
	case domain.SpecialistActionFinish:
		completionID := idgen.New("completion")
		completion, _, err := r.store.FinishSpecialist(ctx, domain.AgentCompletion{
			ID: completionID, RunID: attempt.RunID, AgentID: attempt.AgentID,
			ParentAgentID: attempt.ParentAgentID, AttemptID: attempt.ID,
			Report: *safeAction.Report, MessageID: idgen.New("agentmsg"),
			CreatedAt: time.Now().UTC(),
		}, "specialist-finish-"+attempt.ID)
		if err != nil {
			return r.failAttempt(ctx, result, ref, err)
		}
		result.Completion = completion
		result.AttemptStatus = domain.AgentAttemptFinished
		return nil
	default:
		return r.failAttempt(ctx, result, ref,
			apperror.New(apperror.CodeFailedPrecondition,
				"Specialist returned an unsupported lifecycle action"))
	}
}

type specialistModelCallResult struct {
	Response     *llm.ChatResponse
	Attempt      llm.ModelAttempt
	StreamEvents int
	StreamBytes  int
}

func (r *SpecialistRunner) callModelWithRetry(ctx context.Context, run domain.Run,
	ref domain.AgentAttemptRef, modelRef llm.ModelRef, request llm.ChatRequest,
) (specialistModelCallResult, error) {
	result := specialistModelCallResult{}
	retryPolicy := normalizeModelRetryPolicy(r.retryPolicy)
	for {
		if err := ctx.Err(); err != nil {
			return result, apperror.Normalize(err)
		}
		number, consumedMillis, err := r.store.NextSpecialistModelAttempt(ctx, ref)
		if err != nil {
			return result, apperror.Normalize(err)
		}
		if number > retryPolicy.MaxAttempts {
			return result, apperror.New(apperror.CodeUnavailable,
				"Specialist model retry limit was exhausted")
		}
		if specialistModelBudgetExhausted(run.Budget, consumedMillis, 0) {
			return result, apperror.New(apperror.CodeDeadlineExceeded,
				"Specialist model execution timeout was exhausted")
		}
		attempt := llm.ModelAttempt{
			Number: number, TransportAttempt: number, MaxAttempts: retryPolicy.MaxAttempts,
			Provider: modelRef.Provider, Model: modelRef.Model,
		}
		inserted, err := r.store.RecordSpecialistModelStarted(ctx, ref, attempt)
		if err != nil {
			return result, apperror.Normalize(err)
		}
		if !inserted {
			return result, apperror.New(apperror.CodeConflict,
				"Specialist model attempt is already active")
		}
		callCtx, cancelCall := specialistModelContext(ctx, run.Budget, consumedMillis)
		startedAt := time.Now()
		streamed, callErr := streamSpecialistModel(callCtx, r.router, modelRef, request)
		attempt.Elapsed = time.Since(startedAt)
		attempt.StreamEvents = streamed.Events
		attempt.StreamBytes = streamed.Bytes
		cancelCall()
		result.Attempt = attempt
		result.StreamEvents += streamed.Events
		result.StreamBytes += streamed.Bytes
		if callErr == nil {
			result.Response = streamed.Response
			return result, nil
		}
		providerErr := llm.NormalizeProviderError(modelRef.Provider, callErr)
		attempt.Outcome = providerErr.Kind
		attempt.ErrorText = providerErr.Error()
		attempt.RetryAfter = providerErr.RetryAfter
		attempt.RetryPlanned = providerErr.Kind.Retryable() && number < retryPolicy.MaxAttempts &&
			ctx.Err() == nil &&
			!specialistModelBudgetExhausted(run.Budget, consumedMillis, attempt.Elapsed) &&
			retryPolicy.allowsRetryAfter(providerErr)
		result.Attempt = attempt
		eventCtx, cancelEvent := specialistEventContext(ctx)
		_, storeErr := r.store.RecordSpecialistModelFailed(eventCtx, ref, attempt, nil)
		cancelEvent()
		if storeErr != nil {
			return result, errors.Join(providerApplicationError(providerErr), storeErr)
		}
		if !attempt.RetryPlanned {
			return result, providerApplicationError(providerErr)
		}
		if err := waitForModelRetry(ctx, retryPolicy.delay(number, providerErr)); err != nil {
			return result, apperror.Normalize(err)
		}
	}
}

type specialistStreamResult struct {
	Response *llm.ChatResponse
	Events   int
	Bytes    int
}

func streamSpecialistModel(ctx context.Context, router *llm.Router, ref llm.ModelRef,
	request llm.ChatRequest,
) (specialistStreamResult, error) {
	chunks, err := router.StreamChatModelRef(ctx, ref, request)
	if err != nil {
		return specialistStreamResult{}, err
	}
	var output bytes.Buffer
	result := specialistStreamResult{}
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case chunk, ok := <-chunks:
			if !ok {
				return result, llm.NewProviderError(llm.OutcomeInvalidResponse,
					ref.Provider, "stream closed before a final chunk", nil)
			}
			result.Events++
			if result.Events > maxSpecialistStreamChunks {
				return result, llm.NewProviderError(llm.OutcomeInvalidResponse,
					ref.Provider, "Specialist stream exceeded its chunk limit", nil)
			}
			if chunk.Err != nil {
				return result, llm.NormalizeProviderError(ref.Provider, chunk.Err)
			}
			if len(chunk.ToolCalls) != 0 && !chunk.Done {
				return result, llm.NewProviderError(llm.OutcomeInvalidResponse,
					ref.Provider, "tool calls are only valid on a final stream chunk", nil)
			}
			if len(chunk.Text) > llm.MaxModelOutputBytes-output.Len() {
				return result, llm.NewProviderError(llm.OutcomeInvalidResponse,
					ref.Provider, "Specialist stream output exceeds 65536 bytes", nil)
			}
			_, _ = output.WriteString(chunk.Text)
			result.Bytes = output.Len()
			if !chunk.Done {
				continue
			}
			if chunk.Usage == nil {
				return result, llm.NewProviderError(llm.OutcomeInvalidResponse,
					ref.Provider, "final Specialist stream chunk omitted usage", nil)
			}
			if err := chunk.Usage.Validate(); err != nil {
				return result, llm.NewProviderError(llm.OutcomeInvalidResponse,
					ref.Provider, "final Specialist stream returned invalid usage", err)
			}
			if !utf8.Valid(output.Bytes()) {
				return result, llm.NewProviderError(llm.OutcomeInvalidResponse,
					ref.Provider, "Specialist stream returned invalid UTF-8", nil)
			}
			toolCalls, err := llm.NormalizeToolCalls(chunk.ToolCalls)
			if err != nil {
				return result, llm.NewProviderError(llm.OutcomeInvalidResponse,
					ref.Provider, "final Specialist stream returned invalid tool calls", err)
			}
			provider := strings.TrimSpace(chunk.Provider)
			if provider == "" {
				provider = ref.Provider
			}
			model := strings.TrimSpace(chunk.Model)
			if model == "" {
				model = ref.Model
			}
			result.Response = &llm.ChatResponse{
				Text: output.String(), ToolCalls: toolCalls, Usage: *chunk.Usage,
				Provider: provider, Model: model,
			}
			return result, nil
		}
	}
}

func specialistTurnInput(mission domain.Mission, child domain.AgentNode,
	attempt domain.AgentAttempt,
) (string, error) {
	goal := redact.String(strings.TrimSpace(mission.Goal))
	goalRunes := []rune(goal)
	if len(goalRunes) > maxSpecialistGoalRunes {
		goal = string(goalRunes[:maxSpecialistGoalRunes])
	}
	payload := struct {
		Goal            string         `json:"goal"`
		Profile         domain.Profile `json:"profile"`
		Skills          []string       `json:"skills"`
		Turn            int64          `json:"turn"`
		RemainingTurns  int64          `json:"remaining_turns"`
		RemainingTokens int64          `json:"remaining_tokens"`
		NetworkMode     string         `json:"network_mode"`
	}{
		Goal: goal, Profile: child.Profile, Skills: append([]string(nil), child.Skills...),
		Turn: attempt.Turn, RemainingTurns: max(int64(0), child.TurnLimit-attempt.Turn),
		RemainingTokens: max(int64(0), child.TokenLimit-child.TokensUsed),
		NetworkMode:     mission.Scope.NetworkMode,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	if len(encoded) == 0 || len(encoded) > maxSpecialistInputBytes {
		return "", apperror.New(apperror.CodeResourceExhausted,
			"Specialist turn input exceeds its bounded context")
	}
	return string(encoded), nil
}

func specialistRequest(history []session.Message, input string,
	child domain.AgentNode,
) (llm.ChatRequest, error) {
	if len(history) > maxSpecialistHistoryMessages {
		history = history[len(history)-maxSpecialistHistoryMessages:]
	}
	historyMessages := make([]llm.Message, 0, len(history))
	historyBytes := 0
	for index := len(history) - 1; index >= 0; index-- {
		role := strings.TrimSpace(history[index].Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := redact.String(strings.TrimSpace(history[index].Content))
		if content == "" || len([]byte(content)) > maxSpecialistHistoryBytes-historyBytes {
			continue
		}
		historyMessages = append(historyMessages, llm.Message{Role: role, Content: content})
		historyBytes += len([]byte(content))
	}
	for left, right := 0, len(historyMessages)-1; left < right; left, right = left+1, right-1 {
		historyMessages[left], historyMessages[right] = historyMessages[right], historyMessages[left]
	}
	messages := make([]llm.Message, 0, len(historyMessages)+2)
	messages = append(messages, llm.Message{
		Role: "system",
		Content: "You are an internal no-tool Specialist. Work only on the assigned mission context. " +
			"Do not request tools, shell, network access, credentials, or new agents. Return exactly one " +
			"specialist_lifecycle.v1 JSON object. Go validates policy, usage, leases, and lifecycle transitions.",
	})
	messages = append(messages, historyMessages...)
	messages = append(messages, llm.Message{Role: "user", Content: input})
	return llm.ChatRequest{
		Messages: messages, JSONMode: true, Metadata: map[string]string{
			"run_id": child.RunID, "session_id": child.SessionID, "agent_id": child.ID,
			"response_schema": domain.SpecialistLifecycleVersion,
		},
	}, nil
}

func specialistOutputLimit(child domain.AgentNode) (int, error) {
	remaining := child.TokenLimit - child.TokensUsed
	if remaining <= 0 {
		return 0, apperror.New(apperror.CodeResourceExhausted,
			"Specialist token budget is exhausted")
	}
	limit := int64(defaultSpecialistOutputLimit)
	if remaining < limit {
		limit = remaining
	}
	maxInt := int64(^uint(0) >> 1)
	if limit > maxInt {
		limit = maxInt
	}
	return int(limit), nil
}

func safeSpecialistAction(action domain.SpecialistAction) (domain.SpecialistAction, error) {
	action.Message = redact.String(action.Message)
	if action.Report != nil {
		report := *action.Report
		report.Summary = redact.String(report.Summary)
		action.Report = &report
	}
	return domain.NormalizeSpecialistAction(action)
}

func specialistActionPolicyText(action domain.SpecialistAction) string {
	parts := []string{action.Message}
	if action.Report != nil {
		parts = append(parts, action.Report.Summary)
	}
	return strings.Join(parts, "\n")
}

func (r *SpecialistRunner) recordInvalidAndFail(ctx context.Context,
	result *SpecialistTurnResult, ref domain.AgentAttemptRef, attempt llm.ModelAttempt,
	usage *llm.Usage, cause error,
) error {
	attempt.Outcome = llm.OutcomeInvalidResponse
	attempt.ErrorText = boundedSpecialistFailure(cause)
	attempt.RetryAfter = 0
	attempt.RetryPlanned = false
	eventCtx, cancelEvent := specialistEventContext(ctx)
	charged, storeErr := r.store.RecordSpecialistModelFailed(eventCtx, ref, attempt, usage)
	cancelEvent()
	if charged.ID != "" {
		result.Usage = charged.Usage
	}
	result.ModelOutcome = llm.OutcomeInvalidResponse
	invalid := apperror.Wrap(apperror.CodeFailedPrecondition,
		"Specialist lifecycle response was invalid", cause)
	if storeErr != nil {
		invalid = errors.Join(invalid, storeErr)
	}
	return r.failAttemptWithCode(ctx, result, ref, "invalid_response", invalid)
}

func (r *SpecialistRunner) failAttempt(ctx context.Context, result *SpecialistTurnResult,
	ref domain.AgentAttemptRef, cause error,
) error {
	return r.failAttemptWithCode(ctx, result, ref, specialistFailureCode(cause), cause)
}

func (r *SpecialistRunner) failAttemptWithCode(ctx context.Context,
	result *SpecialistTurnResult, ref domain.AgentAttemptRef, code string, cause error,
) error {
	failure := domain.AgentAttemptFailure{Code: code, Reason: boundedSpecialistFailure(cause)}
	eventCtx, cancelEvent := specialistEventContext(ctx)
	crashed, _, crashErr := r.store.CrashSpecialistAttempt(eventCtx,
		domain.AgentAttemptFailureRequest{
			Ref: ref, Failure: failure, NotificationMessageID: idgen.New("agentmsg"),
			FailedAt: time.Now().UTC(),
		}, "specialist-crash-"+ref.AttemptID)
	cancelEvent()
	if crashed.ID != "" {
		result.AttemptStatus = crashed.Status
		result.Usage = crashed.Usage
	}
	return errors.Join(apperror.Normalize(cause), apperror.Normalize(crashErr))
}

func specialistFailureCode(err error) string {
	switch apperror.CodeOf(apperror.Normalize(err)) {
	case apperror.CodeCancelled:
		return "cancelled"
	case apperror.CodeDeadlineExceeded:
		return "deadline_exceeded"
	case apperror.CodePolicyDenied:
		return "policy_denied"
	case apperror.CodeResourceExhausted:
		return "budget_exhausted"
	case apperror.CodeUnavailable:
		return "provider_unavailable"
	case apperror.CodeConflict:
		return "lease_conflict"
	default:
		return "specialist_failure"
	}
}

func boundedSpecialistFailure(err error) string {
	if err == nil {
		return "Specialist turn failed"
	}
	value := redact.String(strings.TrimSpace(err.Error()))
	runes := []rune(value)
	if len(runes) > domain.MaxAgentFailureReasonRunes {
		value = string(runes[:domain.MaxAgentFailureReasonRunes])
	}
	if len([]byte(value)) > domain.MaxAgentFailureReasonBytes {
		for len([]byte(value)) > domain.MaxAgentFailureReasonBytes && len(runes) > 0 {
			runes = runes[:len(runes)-1]
			value = string(runes)
		}
	}
	if value == "" {
		return "Specialist turn failed"
	}
	return value
}

func specialistAttemptRef(attempt domain.AgentAttempt) domain.AgentAttemptRef {
	return domain.AgentAttemptRef{
		RunID: attempt.RunID, AgentID: attempt.AgentID, AttemptID: attempt.ID,
	}
}

func specialistEventContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
}

func specialistModelBudgetExhausted(budget domain.Budget, consumedMillis int64,
	additional time.Duration,
) bool {
	return budget.TimeoutSeconds > 0 &&
		consumedMillis+additional.Milliseconds() >= budget.TimeoutSeconds*1000
}

func specialistModelContext(ctx context.Context, budget domain.Budget,
	consumedMillis int64,
) (context.Context, context.CancelFunc) {
	if budget.TimeoutSeconds <= 0 {
		return context.WithCancel(ctx)
	}
	remainingMillis := budget.TimeoutSeconds*1000 - consumedMillis
	if remainingMillis <= 0 {
		remainingMillis = 1
	}
	return context.WithTimeout(ctx, time.Duration(remainingMillis)*time.Millisecond)
}
