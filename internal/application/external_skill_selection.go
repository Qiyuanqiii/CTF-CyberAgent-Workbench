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

type ExternalSkillSelectionStore interface {
	GetMission(context.Context, string) (domain.Mission, error)
	GetRun(context.Context, string) (domain.Run, error)
	GetRunMode(context.Context, string) (domain.RunModeSnapshot, error)
	GetInstalledPackageByRef(context.Context, string, string) (
		skills.InstalledPackage, bool, error)
	GetExternalSkillSelection(context.Context, string) (skills.ExternalSelection, error)
	GetExternalSkillSelectionByRun(context.Context, string) (
		skills.ExternalSelection, bool, error)
	GetExternalSkillSelectionOperation(context.Context, string) (
		skills.ExternalSelectionOperation, bool, error)
	CreateExternalSkillSelection(context.Context, skills.ExternalSelection,
		skills.ExternalSelectionOperation, events.Event) (skills.ExternalSelection, bool, error)
}

type ExternalSkillSelectionService struct {
	store ExternalSkillSelectionStore
}

type SelectExternalSkillsRequest struct {
	RunID                   string
	PackageRefs             []string
	SpecialistRef           string
	TokenBudget             int
	OperationKey            string
	RequestedBy             string
	ConfirmUntrustedContext bool
}

type SelectExternalSkillsResult struct {
	Selection skills.ExternalSelection
	Replayed  bool
}

func NewExternalSkillSelectionService(
	store ExternalSkillSelectionStore,
) *ExternalSkillSelectionService {
	return &ExternalSkillSelectionService{store: store}
}

func (s *ExternalSkillSelectionService) Select(ctx context.Context,
	request SelectExternalSkillsRequest,
) (SelectExternalSkillsResult, error) {
	if s == nil || s.store == nil {
		return SelectExternalSkillsResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"external Skill selection store is required")
	}
	normalized, err := normalizeSelectExternalSkillsRequest(request)
	if err != nil {
		return SelectExternalSkillsResult{}, apperror.Wrap(
			apperror.CodeInvalidArgument, err.Error(), err)
	}
	if !normalized.ConfirmUntrustedContext {
		return SelectExternalSkillsResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"external Skill context selection requires explicit operator confirmation")
	}
	run, err := s.store.GetRun(ctx, normalized.RunID)
	if err != nil {
		return SelectExternalSkillsResult{}, apperror.Normalize(err)
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return SelectExternalSkillsResult{}, apperror.Normalize(err)
	}
	keyDigest := runmutation.Fingerprint("external_skill_selection_operation.v1",
		run.ID, normalized.OperationKey)
	if existing, found, err := s.store.GetExternalSkillSelectionOperation(ctx,
		keyDigest); err != nil {
		return SelectExternalSkillsResult{}, apperror.Normalize(err)
	} else if found {
		if existing.RunID != run.ID || existing.RequestedBy != normalized.RequestedBy {
			return SelectExternalSkillsResult{}, apperror.New(apperror.CodeConflict,
				"external Skill selection operation key was already used for different intent")
		}
		stored, err := s.store.GetExternalSkillSelection(ctx, existing.SelectionID)
		if err != nil {
			return SelectExternalSkillsResult{}, apperror.Normalize(err)
		}
		if existing.SelectionID != stored.ID || existing.RunID != stored.RunID ||
			existing.RequestedBy != stored.RequestedBy ||
			!existing.CreatedAt.Equal(stored.CreatedAt) ||
			stored.MissionID != mission.ID ||
			skills.ExternalSelectionRequestFingerprint(stored) != existing.RequestFingerprint {
			return SelectExternalSkillsResult{}, apperror.New(apperror.CodeInternal,
				"stored external Skill selection operation binding is invalid")
		}
		if !externalSelectionMatchesRequest(stored, normalized) {
			return SelectExternalSkillsResult{}, apperror.New(apperror.CodeConflict,
				"external Skill selection operation key was already used for different intent")
		}
		return SelectExternalSkillsResult{Selection: stored, Replayed: true}, nil
	}
	mode, err := s.store.GetRunMode(ctx, run.ID)
	if err != nil {
		return SelectExternalSkillsResult{}, apperror.Normalize(err)
	}
	packages := make([]skills.InstalledPackage, 0, len(normalized.PackageRefs))
	for _, ref := range normalized.PackageRefs {
		name, version, err := skills.ParseInstalledPackageRef(ref)
		if err != nil {
			return SelectExternalSkillsResult{}, apperror.Wrap(
				apperror.CodeInvalidArgument, "external Skill reference is invalid", err)
		}
		installed, found, err := s.store.GetInstalledPackageByRef(ctx, name, version)
		if err != nil {
			return SelectExternalSkillsResult{}, apperror.Normalize(err)
		}
		if !found {
			return SelectExternalSkillsResult{}, apperror.New(apperror.CodeNotFound,
				"installed external Skill package was not found: "+ref)
		}
		packages = append(packages, installed)
	}
	now := time.Now().UTC()
	candidate, err := skills.ResolveExternalSelection(skills.ResolveExternalSelectionRequest{
		SelectionID: idgen.New("external-skill-selection"), RunID: run.ID,
		MissionID: mission.ID, ModeSnapshotID: mode.ID, ModeRevision: mode.Revision,
		Surface: mode.Surface, Profile: mission.Profile, Packages: packages,
		SpecialistRef: normalized.SpecialistRef, TokenBudget: normalized.TokenBudget,
		RequestedBy: normalized.RequestedBy, Confirmed: true, CreatedAt: now,
	})
	if err != nil {
		return SelectExternalSkillsResult{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"external Skill selection could not be resolved", err)
	}
	requestFingerprint := skills.ExternalSelectionRequestFingerprint(candidate)
	if run.Status != domain.RunCreated {
		return SelectExternalSkillsResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"external Skills can only be selected before a Run starts")
	}
	operation := skills.ExternalSelectionOperation{
		KeyDigest: keyDigest, RequestFingerprint: requestFingerprint,
		SelectionID: candidate.ID, RunID: run.ID, RequestedBy: candidate.RequestedBy,
		CreatedAt: candidate.CreatedAt,
	}
	event, err := events.New(run.ID, mission.ID,
		events.ExternalSkillSelectionCreatedEvent, "external_skills", candidate.ID,
		map[string]any{
			"protocol": candidate.ProtocolVersion, "surface": candidate.Surface,
			"profile": candidate.Profile, "item_count": candidate.ItemCount,
			"token_budget":       candidate.TokenBudget,
			"token_upper_bound":  candidate.TokenUpperBound,
			"operator_confirmed": true, "context_delivery": true,
			"tool_capability_grant": false,
		})
	if err != nil {
		return SelectExternalSkillsResult{}, err
	}
	event.CreatedAt = candidate.CreatedAt
	stored, replayed, err := s.store.CreateExternalSkillSelection(ctx, candidate,
		operation, event)
	return SelectExternalSkillsResult{Selection: stored, Replayed: replayed},
		apperror.Normalize(err)
}

func externalSelectionMatchesRequest(selection skills.ExternalSelection,
	request SelectExternalSkillsRequest,
) bool {
	if selection.RunID != request.RunID || selection.RequestedBy != request.RequestedBy ||
		selection.TokenBudget != request.TokenBudget ||
		len(selection.Items) != len(request.PackageRefs) {
		return false
	}
	specialistRef := ""
	for index, item := range selection.Items {
		if skills.FormatInstalledPackageRef(item.Name, item.Version) !=
			request.PackageRefs[index] {
			return false
		}
		if item.SpecialistEligible {
			if specialistRef != "" {
				return false
			}
			specialistRef = request.PackageRefs[index]
		}
	}
	return specialistRef == request.SpecialistRef
}

func (s *ExternalSkillSelectionService) GetForRun(ctx context.Context,
	runID string,
) (skills.ExternalSelection, error) {
	if s == nil || s.store == nil {
		return skills.ExternalSelection{}, apperror.New(apperror.CodeFailedPrecondition,
			"external Skill selection store is required")
	}
	runID = strings.TrimSpace(runID)
	if !domain.ValidAgentID(runID) || strings.ContainsRune(runID, 0) {
		return skills.ExternalSelection{}, apperror.New(apperror.CodeInvalidArgument,
			"external Skill selection Run id is invalid")
	}
	selection, found, err := s.store.GetExternalSkillSelectionByRun(ctx, runID)
	if err != nil {
		return skills.ExternalSelection{}, apperror.Normalize(err)
	}
	if !found {
		return skills.ExternalSelection{}, apperror.New(apperror.CodeNotFound,
			"external Skill selection was not found")
	}
	return selection, nil
}

func normalizeSelectExternalSkillsRequest(
	request SelectExternalSkillsRequest,
) (SelectExternalSkillsRequest, error) {
	originalKey := request.OperationKey
	request.RunID = strings.TrimSpace(request.RunID)
	request.SpecialistRef = strings.TrimSpace(request.SpecialistRef)
	request.RequestedBy = strings.TrimSpace(redact.String(request.RequestedBy))
	if request.RequestedBy == "" {
		request.RequestedBy = "cli_operator"
	}
	if request.TokenBudget == 0 {
		request.TokenBudget = skills.DefaultExternalSelectionTokenBudget
	}
	if !domain.ValidAgentID(request.RunID) || strings.ContainsRune(request.RunID, 0) ||
		!domain.ValidAgentID(request.RequestedBy) || strings.ContainsRune(request.RequestedBy, 0) {
		return SelectExternalSkillsRequest{}, errors.New("bounded Run and operator identities are required")
	}
	if request.OperationKey != strings.TrimSpace(originalKey) ||
		!utf8.ValidString(request.OperationKey) {
		return SelectExternalSkillsRequest{}, errors.New("external Skill selection operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil {
		return SelectExternalSkillsRequest{}, err
	}
	for _, current := range request.OperationKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return SelectExternalSkillsRequest{}, errors.New("external Skill selection operation key cannot contain whitespace or control characters")
		}
	}
	if len(request.PackageRefs) == 0 ||
		len(request.PackageRefs) > skills.MaxExternalSelectionItems {
		return SelectExternalSkillsRequest{}, errors.New("external Skill selection requires a bounded non-empty package list")
	}
	refs := make([]string, len(request.PackageRefs))
	for index, ref := range request.PackageRefs {
		if strings.TrimSpace(ref) != ref || !utf8.ValidString(ref) {
			return SelectExternalSkillsRequest{}, errors.New("external Skill references must be normalized UTF-8")
		}
		name, version, err := skills.ParseInstalledPackageRef(ref)
		if err != nil {
			return SelectExternalSkillsRequest{}, err
		}
		refs[index] = skills.FormatInstalledPackageRef(name, version)
	}
	sort.Strings(refs)
	for index := 1; index < len(refs); index++ {
		if refs[index] == refs[index-1] {
			return SelectExternalSkillsRequest{}, errors.New("external Skill references must be unique")
		}
	}
	if request.SpecialistRef != "" {
		name, version, err := skills.ParseInstalledPackageRef(request.SpecialistRef)
		if err != nil {
			return SelectExternalSkillsRequest{}, err
		}
		request.SpecialistRef = skills.FormatInstalledPackageRef(name, version)
		if index := sort.SearchStrings(refs, request.SpecialistRef); index >= len(refs) ||
			refs[index] != request.SpecialistRef {
			return SelectExternalSkillsRequest{}, errors.New("specialist external Skill must also appear in the selected package list")
		}
	}
	request.PackageRefs = refs
	return request, nil
}
