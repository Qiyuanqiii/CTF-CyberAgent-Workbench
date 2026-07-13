package application

import (
	"context"
	"errors"
	"sort"
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
	"cyberagent-workbench/internal/skills"
)

type SkillSelectionStore interface {
	GetMission(ctx context.Context, id string) (domain.Mission, error)
	GetRun(ctx context.Context, id string) (domain.Run, error)
	GetSkillSelection(ctx context.Context, id string) (skills.Selection, error)
	GetSkillSelectionByRun(ctx context.Context, runID string) (skills.Selection, bool, error)
	GetSkillSelectionOperation(ctx context.Context, keyDigest string) (skills.SelectionOperation, bool, error)
	CreateSkillSelection(ctx context.Context, selection skills.Selection,
		operation skills.SelectionOperation, event events.Event) (skills.Selection, bool, error)
}

type SkillSelectionService struct {
	store    SkillSelectionStore
	registry *skills.Registry
}

type SelectSkillsRequest struct {
	RunID        string
	Names        []string
	TokenBudget  int
	OperationKey string
	RequestedBy  string
}

type SelectSkillsResult struct {
	Selection skills.Selection
	Replayed  bool
}

func NewSkillSelectionService(store SkillSelectionStore, registry *skills.Registry) *SkillSelectionService {
	return &SkillSelectionService{store: store, registry: registry}
}

func (s *SkillSelectionService) Select(ctx context.Context, request SelectSkillsRequest) (SelectSkillsResult, error) {
	if s == nil || s.store == nil || s.registry == nil {
		return SelectSkillsResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"skill selection store and Registry are required")
	}
	normalized, err := normalizeSelectSkillsRequest(request)
	if err != nil {
		return SelectSkillsResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			err.Error(), err)
	}
	run, err := s.store.GetRun(ctx, normalized.RunID)
	if err != nil {
		return SelectSkillsResult{}, apperror.Normalize(err)
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return SelectSkillsResult{}, apperror.Normalize(err)
	}
	keyDigest := runmutation.Fingerprint("skill_selection_operation.v1", run.ID,
		normalized.OperationKey)
	requestFingerprint := skills.SelectionIntentFingerprint(run.ID, mission.ID,
		mission.Profile, normalized.TokenBudget, normalized.Names, normalized.RequestedBy)
	if existing, found, err := s.store.GetSkillSelectionOperation(ctx, keyDigest); err != nil {
		return SelectSkillsResult{}, apperror.Normalize(err)
	} else if found {
		if existing.RequestFingerprint != requestFingerprint || existing.RunID != run.ID ||
			existing.RequestedBy != normalized.RequestedBy {
			return SelectSkillsResult{}, apperror.New(apperror.CodeConflict,
				"skill selection operation key was already used for different intent")
		}
		stored, err := s.store.GetSkillSelection(ctx, existing.SelectionID)
		if err == nil && (existing.SelectionID != stored.ID || existing.RunID != stored.RunID ||
			existing.RequestedBy != stored.RequestedBy ||
			!existing.CreatedAt.Equal(stored.CreatedAt) ||
			skills.SelectionRequestFingerprint(stored) != existing.RequestFingerprint) {
			return SelectSkillsResult{}, apperror.New(apperror.CodeInternal,
				"stored Skill selection operation binding is invalid")
		}
		return SelectSkillsResult{Selection: stored, Replayed: true}, apperror.Normalize(err)
	}
	now := time.Now().UTC()
	selection, err := s.registry.ResolveSelection(skills.ResolveSelectionRequest{
		SelectionID: idgen.New("skill-selection"), RunID: run.ID, MissionID: mission.ID,
		Profile: mission.Profile, Names: normalized.Names, TokenBudget: normalized.TokenBudget,
		RequestedBy: normalized.RequestedBy, CreatedAt: now,
	})
	if err != nil {
		return SelectSkillsResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			err.Error(), err)
	}
	if skills.SelectionRequestFingerprint(selection) != requestFingerprint {
		return SelectSkillsResult{}, apperror.New(apperror.CodeInternal,
			"resolved Skill selection intent fingerprint is inconsistent")
	}
	if run.Status != domain.RunCreated {
		return SelectSkillsResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"skills can only be selected before a Run starts")
	}
	operation := skills.SelectionOperation{
		KeyDigest: keyDigest, RequestFingerprint: requestFingerprint,
		SelectionID: selection.ID, RunID: run.ID, RequestedBy: normalized.RequestedBy,
		CreatedAt: selection.CreatedAt,
	}
	event, err := events.New(run.ID, mission.ID, events.SkillSelectionCreatedEvent,
		"skills", selection.ID, map[string]any{
			"protocol": selection.ProtocolVersion, "profile": selection.Profile,
			"item_count": selection.ItemCount, "token_budget": selection.TokenBudget,
			"token_upper_bound": selection.TokenUpperBound,
			"context_injection": false, "tool_capability_grant": false,
		})
	if err != nil {
		return SelectSkillsResult{}, err
	}
	event.CreatedAt = selection.CreatedAt
	stored, replayed, err := s.store.CreateSkillSelection(ctx, selection, operation, event)
	return SelectSkillsResult{Selection: stored, Replayed: replayed}, apperror.Normalize(err)
}

func (s *SkillSelectionService) GetForRun(ctx context.Context, runID string) (skills.Selection, error) {
	if s == nil || s.store == nil {
		return skills.Selection{}, apperror.New(apperror.CodeFailedPrecondition,
			"skill selection store is required")
	}
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return skills.Selection{}, apperror.New(apperror.CodeInvalidArgument,
			"skill selection Run id is invalid")
	}
	selection, found, err := s.store.GetSkillSelectionByRun(ctx, runID)
	if err != nil {
		return skills.Selection{}, apperror.Normalize(err)
	}
	if !found {
		return skills.Selection{}, apperror.New(apperror.CodeNotFound,
			"skill selection was not found")
	}
	return selection, nil
}

func normalizeSelectSkillsRequest(request SelectSkillsRequest) (SelectSkillsRequest, error) {
	originalKey := request.OperationKey
	request.RunID = strings.TrimSpace(request.RunID)
	request.RequestedBy = strings.TrimSpace(redact.String(request.RequestedBy))
	if request.RequestedBy == "" {
		request.RequestedBy = "cli_operator"
	}
	if request.TokenBudget == 0 {
		request.TokenBudget = skills.DefaultSelectionTokenBudget
	}
	if !domain.ValidAgentID(request.RunID) || strings.ContainsRune(request.RunID, 0) ||
		!domain.ValidAgentID(request.RequestedBy) || strings.ContainsRune(request.RequestedBy, 0) {
		return SelectSkillsRequest{}, errors.New("bounded Run and operator identities are required")
	}
	if request.OperationKey != strings.TrimSpace(originalKey) || !utf8.ValidString(request.OperationKey) {
		return SelectSkillsRequest{}, errors.New("skill selection operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil {
		return SelectSkillsRequest{}, err
	}
	for _, current := range request.OperationKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return SelectSkillsRequest{}, errors.New("skill selection operation key cannot contain whitespace or control characters")
		}
	}
	if len(request.Names) == 0 || len(request.Names) > skills.MaxSelectionItems {
		return SelectSkillsRequest{}, errors.New("skill selection requires a bounded non-empty name list")
	}
	names := make([]string, len(request.Names))
	for index, name := range request.Names {
		if strings.TrimSpace(name) != name || !utf8.ValidString(name) {
			return SelectSkillsRequest{}, errors.New("selected Skill names must be normalized UTF-8")
		}
		names[index] = name
	}
	sort.Strings(names)
	request.Names = names
	return request, nil
}
