package application_test

import (
	"context"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/store"
)

func TestPlanDeliveryControlKeepsSelectionAndTransitionSeparate(t *testing.T) {
	ctx := context.Background()
	st, run, proposal := preparePlanDeliveryControlFixture(t)
	service := application.NewPlanDeliveryControlService(st)
	selected, err := service.SelectDirection(ctx, application.ControlPlanDirectionRequest{
		Version: application.PlanDeliveryControlProtocolVersion, RunID: run.ID,
		ProposalID: proposal.ID, Direction: 2,
		OperationKey: "plan-control-direction-0001", RequestedBy: "desktop_operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	mode, err := st.GetRunMode(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Selection.DirectionOrdinal != 2 || len(selected.WorkItems) == 0 ||
		mode.Phase != domain.ExecutionPhasePlan {
		t.Fatalf("selection changed phase or omitted work: selected=%#v mode=%#v", selected, mode)
	}
	replayed, err := service.SelectDirection(ctx, application.ControlPlanDirectionRequest{
		Version: application.PlanDeliveryControlProtocolVersion, RunID: run.ID,
		ProposalID: proposal.ID, Direction: 2,
		OperationKey: "plan-control-direction-0001", RequestedBy: "desktop_operator",
	})
	if err != nil || !replayed.Replayed || replayed.Selection.ID != selected.Selection.ID {
		t.Fatalf("direction replay=%#v err=%v", replayed, err)
	}
	transition, err := service.EnterDelivery(ctx,
		application.ControlPlanDeliveryTransitionRequest{
			Version: application.PlanDeliveryControlProtocolVersion, RunID: run.ID,
			OperationKey: "plan-control-deliver-0001", RequestedBy: "desktop_operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	if transition.SelectionID != selected.Selection.ID || transition.Replayed ||
		transition.AppliedMode.Phase != domain.ExecutionPhaseDeliver ||
		transition.CurrentMode.Phase != domain.ExecutionPhaseDeliver {
		t.Fatalf("unexpected Deliver transition: %#v", transition)
	}
	delayed, err := service.EnterDelivery(ctx,
		application.ControlPlanDeliveryTransitionRequest{
			Version: application.PlanDeliveryControlProtocolVersion, RunID: run.ID,
			OperationKey: "plan-control-deliver-0001", RequestedBy: "desktop_operator",
		})
	if err != nil || !delayed.Replayed || delayed.AppliedMode.ID != transition.AppliedMode.ID {
		t.Fatalf("Deliver replay=%#v err=%v", delayed, err)
	}
}

func TestPlanDeliveryControlRejectsCrossRunAndUnselectedTransition(t *testing.T) {
	ctx := context.Background()
	st, run, proposal := preparePlanDeliveryControlFixture(t)
	_, other, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "other Plan Run", Profile: "code", Phase: "plan", ModelRoute: "code",
		Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err = application.NewRunService(st).Start(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	other, err = application.NewRunService(st).Pause(ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	service := application.NewPlanDeliveryControlService(st)
	_, err = service.SelectDirection(ctx, application.ControlPlanDirectionRequest{
		Version: application.PlanDeliveryControlProtocolVersion, RunID: other.ID,
		ProposalID: proposal.ID, Direction: 1,
		OperationKey: "plan-control-cross-run-0001", RequestedBy: "desktop_operator",
	})
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("cross-Run selection error=%v", err)
	}
	_, err = service.EnterDelivery(ctx, application.ControlPlanDeliveryTransitionRequest{
		Version: application.PlanDeliveryControlProtocolVersion, RunID: other.ID,
		OperationKey: "plan-control-unselected-0001", RequestedBy: "desktop_operator",
	})
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("unselected Deliver error=%v", err)
	}
	_ = run
}

func preparePlanDeliveryControlFixture(t *testing.T) (*store.SQLiteStore, domain.Run,
	domain.PlanDeliveryProposal,
) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "plan-control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "choose a bounded delivery direction", Profile: "code", Phase: "plan",
		ModelRoute: "code", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = application.NewRunService(st).Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("plan-control-provider", "plan_delivery_propose", planDeliveryTestPayload),
		textResponse(rootActionResponse(domain.RootActionWait,
			"three directions are ready", "", "operator direction choice required")),
	}}
	result, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID)
	if err != nil || result.RunStatus != domain.RunPaused {
		t.Fatalf("prepare Plan control proposal result=%#v err=%v", result, err)
	}
	proposals, err := application.NewPlanDeliveryService(st).ListProposals(ctx, run.ID, 10)
	if err != nil || len(proposals) != 1 {
		t.Fatalf("prepare Plan control proposals=%#v err=%v", proposals, err)
	}
	run, err = st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return st, run, proposals[0]
}
