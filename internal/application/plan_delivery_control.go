package application

import (
	"context"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
)

const PlanDeliveryControlProtocolVersion = "plan_delivery_control.v1"

type PlanDeliveryControlStore interface {
	PlanDeliverySelectionStore
	RunStore
}

type PlanDeliveryControlService struct {
	store     PlanDeliveryControlStore
	selection *PlanDeliveryService
	runs      *RunService
}

type ControlPlanDirectionRequest struct {
	Version      string
	RunID        string
	ProposalID   string
	Direction    int
	OperationKey string
	RequestedBy  string
}

type ControlPlanDirectionResult struct {
	Selection domain.PlanDeliverySelection
	WorkItems []domain.WorkItem
	Replayed  bool
}

type ControlPlanDeliveryTransitionRequest struct {
	Version      string
	RunID        string
	OperationKey string
	RequestedBy  string
}

type ControlPlanDeliveryTransitionResult struct {
	SelectionID string
	AppliedMode domain.RunModeSnapshot
	CurrentMode domain.RunModeSnapshot
	Replayed    bool
}

func NewPlanDeliveryControlService(store PlanDeliveryControlStore) *PlanDeliveryControlService {
	return &PlanDeliveryControlService{store: store,
		selection: NewPlanDeliveryService(store), runs: NewRunService(store)}
}

func (s *PlanDeliveryControlService) SelectDirection(ctx context.Context,
	request ControlPlanDirectionRequest,
) (ControlPlanDirectionResult, error) {
	if s == nil || s.store == nil || s.selection == nil {
		return ControlPlanDirectionResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"Plan/Delivery control store is required")
	}
	if err := validatePlanDeliveryControlIdentity(request.Version, request.RunID); err != nil {
		return ControlPlanDirectionResult{}, err
	}
	proposal, err := s.selection.GetProposal(ctx, request.ProposalID)
	if err != nil {
		return ControlPlanDirectionResult{}, err
	}
	if proposal.RunID != request.RunID {
		return ControlPlanDirectionResult{}, apperror.New(apperror.CodeFailedPrecondition,
			"Plan proposal does not belong to the requested Run")
	}
	result, err := s.selection.Select(ctx, SelectPlanDeliveryDirectionRequest{
		ProposalID: proposal.ID, Direction: request.Direction,
		OperationKey: request.OperationKey, RequestedBy: request.RequestedBy,
	})
	if err != nil {
		return ControlPlanDirectionResult{}, err
	}
	return ControlPlanDirectionResult{Selection: result.Selection,
		WorkItems: result.WorkItems, Replayed: result.Replayed}, nil
}

func (s *PlanDeliveryControlService) EnterDelivery(ctx context.Context,
	request ControlPlanDeliveryTransitionRequest,
) (ControlPlanDeliveryTransitionResult, error) {
	if s == nil || s.store == nil || s.selection == nil || s.runs == nil {
		return ControlPlanDeliveryTransitionResult{}, apperror.New(
			apperror.CodeFailedPrecondition, "Plan/Delivery control store is required")
	}
	if err := validatePlanDeliveryControlIdentity(request.Version, request.RunID); err != nil {
		return ControlPlanDeliveryTransitionResult{}, err
	}
	selection, found, err := s.selection.SelectionForRun(ctx, request.RunID)
	if err != nil {
		return ControlPlanDeliveryTransitionResult{}, err
	}
	if !found || selection.RunID != request.RunID {
		return ControlPlanDeliveryTransitionResult{}, apperror.New(
			apperror.CodeFailedPrecondition,
			"Plan direction must be selected before entering Deliver")
	}
	changed, err := s.runs.ChangePhase(ctx, ChangeRunPhaseRequest{
		RunID: request.RunID, Phase: string(domain.ExecutionPhaseDeliver),
		OperationKey: request.OperationKey, RequestedBy: request.RequestedBy,
		Reason: "operator accepted selected Plan direction",
	})
	if err != nil {
		return ControlPlanDeliveryTransitionResult{}, err
	}
	current, err := s.store.GetRunMode(ctx, request.RunID)
	if err != nil {
		return ControlPlanDeliveryTransitionResult{}, apperror.Normalize(err)
	}
	return ControlPlanDeliveryTransitionResult{SelectionID: selection.ID,
		AppliedMode: changed.Mode, CurrentMode: current, Replayed: changed.Replayed}, nil
}

func validatePlanDeliveryControlIdentity(version string, runID string) error {
	if version != PlanDeliveryControlProtocolVersion {
		return apperror.New(apperror.CodeInvalidArgument,
			"unsupported Plan/Delivery control version")
	}
	if runID != strings.TrimSpace(runID) || !domain.ValidAgentID(runID) ||
		strings.ContainsRune(runID, 0) {
		return apperror.New(apperror.CodeInvalidArgument,
			"Plan/Delivery control Run id is invalid")
	}
	return nil
}
