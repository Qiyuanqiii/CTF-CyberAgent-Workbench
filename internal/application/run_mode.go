package application

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
)

type ChangeRunPhaseRequest struct {
	RunID        string
	Phase        string
	OperationKey string
	RequestedBy  string
	Reason       string
}

type ChangeRunPhaseResult struct {
	Mode     domain.RunModeSnapshot
	Replayed bool
}

func (s *RunService) Mode(ctx context.Context, runID string) (domain.RunModeSnapshot, error) {
	if s == nil || s.store == nil {
		return domain.RunModeSnapshot{}, apperror.New(apperror.CodeFailedPrecondition,
			"run store is required")
	}
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return domain.RunModeSnapshot{}, apperror.New(apperror.CodeInvalidArgument,
			"run mode Run id is invalid")
	}
	mode, err := s.store.GetRunMode(ctx, runID)
	return mode, apperror.Normalize(err)
}

func (s *RunService) ChangePhase(ctx context.Context,
	request ChangeRunPhaseRequest,
) (ChangeRunPhaseResult, error) {
	if s == nil || s.store == nil {
		return ChangeRunPhaseResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"run store is required")
	}
	normalized, target, err := normalizeChangeRunPhaseRequest(request)
	if err != nil {
		return ChangeRunPhaseResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			err.Error(), err)
	}
	keyDigest := runmutation.Fingerprint("run_mode_operation.v1", normalized.RunID,
		normalized.OperationKey)
	requestFingerprint := runmutation.Fingerprint("run_phase_change_request.v1",
		normalized.RunID, string(target), normalized.RequestedBy, normalized.Reason)
	if replay, found, err := s.loadRunPhaseReplay(ctx, keyDigest, requestFingerprint,
		normalized.RunID, normalized.RequestedBy, target); err != nil {
		return ChangeRunPhaseResult{}, err
	} else if found {
		return replay, nil
	}
	run, err := s.store.GetRun(ctx, normalized.RunID)
	if err != nil {
		return ChangeRunPhaseResult{}, apperror.Normalize(err)
	}
	if !domain.CanChangeRunPhase(run.Status) {
		return ChangeRunPhaseResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"run phase can only change while the Run is created or paused")
	}
	current, err := s.store.GetRunMode(ctx, run.ID)
	if err != nil {
		return ChangeRunPhaseResult{}, apperror.Normalize(err)
	}
	if current.Phase == target {
		// Another Store may have committed this exact operation after our first
		// lookup but before the current mode read. Recheck the atomic operation
		// ledger before treating the already-reached phase as a new request.
		if replay, found, err := s.loadRunPhaseReplay(ctx, keyDigest, requestFingerprint,
			normalized.RunID, normalized.RequestedBy, target); err != nil {
			return ChangeRunPhaseResult{}, err
		} else if found {
			return replay, nil
		}
		return ChangeRunPhaseResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"run is already in the requested phase")
	}
	now := time.Now().UTC()
	if now.Before(current.CreatedAt) {
		now = current.CreatedAt
	}
	next, err := current.Next(idgen.New("run-mode"), target, normalized.RequestedBy,
		normalized.Reason, now)
	if err != nil {
		return ChangeRunPhaseResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"run phase transition is invalid", err)
	}
	operation := domain.RunModeOperation{
		KeyDigest: keyDigest, RequestFingerprint: requestFingerprint,
		SnapshotID: next.ID, RunID: next.RunID, RequestedBy: next.RequestedBy,
		CreatedAt: next.CreatedAt,
	}
	event, err := events.New(next.RunID, next.MissionID, events.RunPhaseChangedEvent,
		"run_mode", next.ID, map[string]any{
			"protocol": next.ProtocolVersion, "revision": next.Revision,
			"surface": next.Surface, "from": current.Phase, "to": next.Phase,
			"policy_version":       next.PolicyVersion,
			"network_mode":         next.Scope.NetworkMode,
			"allowed_target_count": len(next.Scope.AllowedTargets),
			"requested_by":         next.RequestedBy, "reason": next.Reason,
			"capability_grant": false,
		})
	if err != nil {
		return ChangeRunPhaseResult{}, err
	}
	event.CreatedAt = next.CreatedAt
	stored, replayed, err := s.store.TransitionRunPhase(ctx, next, operation, event)
	return ChangeRunPhaseResult{Mode: stored, Replayed: replayed}, apperror.Normalize(err)
}

func (s *RunService) loadRunPhaseReplay(ctx context.Context, keyDigest string,
	requestFingerprint string, runID string, requestedBy string,
	target domain.ExecutionPhase,
) (ChangeRunPhaseResult, bool, error) {
	existing, found, err := s.store.GetRunModeOperation(ctx, keyDigest)
	if err != nil {
		return ChangeRunPhaseResult{}, false, apperror.Normalize(err)
	}
	if !found {
		return ChangeRunPhaseResult{}, false, nil
	}
	if existing.RequestFingerprint != requestFingerprint || existing.RunID != runID ||
		existing.RequestedBy != requestedBy {
		return ChangeRunPhaseResult{}, true, apperror.New(apperror.CodeConflict,
			"run mode operation key was already used for different intent")
	}
	stored, err := s.store.GetRunModeSnapshot(ctx, existing.SnapshotID)
	if err != nil {
		return ChangeRunPhaseResult{}, true, apperror.Normalize(err)
	}
	if stored.ID != existing.SnapshotID || stored.RunID != existing.RunID ||
		stored.RequestedBy != existing.RequestedBy ||
		!stored.CreatedAt.Equal(existing.CreatedAt) || stored.Phase != target {
		return ChangeRunPhaseResult{}, true, apperror.New(apperror.CodeInternal,
			"stored run mode operation binding is invalid")
	}
	return ChangeRunPhaseResult{Mode: stored, Replayed: true}, true, nil
}

func normalizeChangeRunPhaseRequest(request ChangeRunPhaseRequest) (
	ChangeRunPhaseRequest, domain.ExecutionPhase, error,
) {
	originalKey := request.OperationKey
	request.RunID = strings.TrimSpace(request.RunID)
	request.RequestedBy = strings.TrimSpace(redact.String(request.RequestedBy))
	request.Reason = strings.TrimSpace(redact.String(request.Reason))
	if request.RequestedBy == "" {
		request.RequestedBy = "cli_operator"
	}
	if request.Reason == "" {
		request.Reason = "operator requested phase change"
	}
	if !domain.ValidAgentID(request.RunID) || strings.ContainsRune(request.RunID, 0) ||
		!domain.ValidAgentID(request.RequestedBy) || strings.ContainsRune(request.RequestedBy, 0) {
		return ChangeRunPhaseRequest{}, "", errors.New(
			"bounded Run and operator identities are required")
	}
	if !utf8.ValidString(request.Reason) || strings.ContainsRune(request.Reason, 0) ||
		utf8.RuneCountInString(request.Reason) > domain.MaxRunModeReasonRunes {
		return ChangeRunPhaseRequest{}, "", errors.New("run phase reason is invalid or too long")
	}
	if request.OperationKey != strings.TrimSpace(originalKey) ||
		!utf8.ValidString(request.OperationKey) {
		return ChangeRunPhaseRequest{}, "", errors.New(
			"run mode operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil {
		return ChangeRunPhaseRequest{}, "", err
	}
	for _, current := range request.OperationKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return ChangeRunPhaseRequest{}, "", errors.New(
				"run mode operation key cannot contain whitespace or control characters")
		}
	}
	target, err := domain.ParseExecutionPhase(request.Phase)
	if err != nil {
		return ChangeRunPhaseRequest{}, "", err
	}
	request.Phase = string(target)
	return request, target, nil
}
