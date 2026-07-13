package application_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/store"
)

const planDeliveryTestPayload = `{"version":"plan_delivery.v1","directions":[` +
	`{"title":"Conservative","summary":"Keep the blast radius small.","tradeoffs":["More sequential work"],"modules":[` +
	`{"title":"Inspect boundaries","objective":"Document the current contracts.","acceptance_criteria":["Contracts are documented"],"dependencies":[]}]},` +
	`{"title":"Balanced","summary":"Deliver one complete vertical path.","tradeoffs":["Moderate implementation breadth"],"modules":[` +
	`{"title":"Implement core","objective":"Implement the bounded core path.","acceptance_criteria":["Focused tests pass"],"dependencies":[]},` +
	`{"title":"Audit delivery","objective":"Audit behavior and update handoff notes.","acceptance_criteria":["Audit is recorded","Regression tests pass"],"dependencies":[1]}]},` +
	`{"title":"Accelerated","summary":"Prepare more independent slices.","tradeoffs":["Higher review load"],"modules":[` +
	`{"title":"Parallel preparation","objective":"Prepare bounded independent slices.","acceptance_criteria":["Slices remain independent"],"dependencies":[]}]}]}`

func TestPlanDeliveryProposalChoiceAndProjectionAreReviewGated(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "plan-delivery.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runService := application.NewRunService(st)
	_, run, err := runService.Create(ctx, application.CreateRunRequest{
		Goal: "plan a bounded implementation", Profile: "review", Phase: "plan",
		ModelRoute: "tool-loop/model",
		Budget:     domain.Budget{MaxTurns: 4, MaxTokens: 1000, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runService.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("provider-plan", "plan_delivery_propose", planDeliveryTestPayload),
		textResponse(rootActionResponse(domain.RootActionWait,
			"three directions are ready", "", "operator direction choice required")),
	}}
	result, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID)
	if err != nil || result.RunStatus != domain.RunPaused || result.ToolCalls != 1 {
		t.Fatalf("Plan proposal turn failed: %#v err=%v", result, err)
	}
	requests := provider.Requests()
	if len(requests) != 2 || len(requests[0].Tools) != 4 ||
		!hasToolResult(requests[1], `"selection_authorized":"false"`) {
		t.Fatalf("Plan-only tool boundary was not delivered: %#v", requests)
	}
	service := application.NewPlanDeliveryService(st)
	proposals, err := service.ListProposals(ctx, run.ID, 10)
	if err != nil || len(proposals) != 1 ||
		len(proposals[0].Spec.Directions) != domain.PlanDeliveryDirectionCount {
		t.Fatalf("proposal was not persisted: %#v err=%v", proposals, err)
	}
	before, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if err != nil || len(before) != 0 {
		t.Fatalf("proposal executed work before operator choice: %#v err=%v", before, err)
	}
	chosen, err := service.Select(ctx, application.SelectPlanDeliveryDirectionRequest{
		ProposalID: proposals[0].ID, Direction: 2,
		OperationKey: "choose-balanced-0001", RequestedBy: "operator",
	})
	if err != nil || chosen.Selection.DirectionOrdinal != 2 ||
		len(chosen.WorkItems) != 2 || !chosen.Note.Pinned ||
		chosen.Note.Category != domain.NoteDecision ||
		!strings.Contains(chosen.Note.Content, "Balanced") ||
		strings.Contains(chosen.Note.Content, proposals[0].Fingerprint) ||
		len(chosen.WorkItems[1].Dependencies) != 1 ||
		chosen.WorkItems[1].Dependencies[0] != chosen.WorkItems[0].ID {
		t.Fatalf("direction choice did not project atomically: %#v err=%v", chosen, err)
	}
	replay, err := service.Select(ctx, application.SelectPlanDeliveryDirectionRequest{
		ProposalID: proposals[0].ID, Direction: 2,
		OperationKey: "choose-balanced-0001", RequestedBy: "operator",
	})
	if err != nil || !replay.Replayed || replay.Selection.ID != chosen.Selection.ID ||
		len(replay.WorkItems) != 2 {
		t.Fatalf("direction choice replay did not converge: %#v err=%v", replay, err)
	}
	current, err := st.GetRun(ctx, run.ID)
	if err != nil || current.Status != domain.RunPaused {
		t.Fatalf("direction choice changed Run lifecycle: %#v err=%v", current, err)
	}
	mode, err := st.GetRunMode(ctx, run.ID)
	if err != nil || mode.Phase != domain.ExecutionPhasePlan {
		t.Fatalf("direction choice changed Run phase: %#v err=%v", mode, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countEventType(timeline, events.PlanDeliveryProposedEvent) != 1 ||
		countEventType(timeline, events.PlanDeliveryDirectionSelectedEvent) != 1 ||
		countEventType(timeline, events.WorkItemCreatedEvent) != 2 ||
		countEventType(timeline, events.NoteCreatedEvent) != 1 {
		t.Fatalf("Plan/Delivery timeline is inconsistent: %#v err=%v", timeline, err)
	}
	for _, event := range timeline {
		if strings.Contains(event.PayloadJSON, proposals[0].Fingerprint) {
			t.Fatalf("Plan/Delivery fingerprint leaked into event %s: %s",
				event.Type, event.PayloadJSON)
		}
	}
	if _, err := runService.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "deliver", OperationKey: "deliver-balanced-0001",
		RequestedBy: "operator", Reason: "accepted direction 2",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeliverPhaseRejectsUnadvertisedPlanDeliveryToolBeforeBudget(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "plan-delivery-deliver.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run := newStartedRunForProvider(t, st, "tool-loop",
		domain.Budget{MaxTurns: 3, MaxTokens: 1000, MaxToolCalls: 4})
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("provider-unadvertised-plan", "plan_delivery_propose",
			planDeliveryTestPayload),
		textResponse(rootActionResponse(domain.RootActionContinue,
			"ignored an unavailable Plan tool", "", "")),
	}}
	result, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID)
	if err != nil || result.ProtocolRepairs != 1 || result.ToolCalls != 0 ||
		result.ModelAttempts != 2 {
		t.Fatalf("Deliver-phase Plan tool did not use bounded protocol repair: %#v err=%v",
			result, err)
	}
	requests := provider.Requests()
	if len(requests) != 2 || len(requests[0].Tools) != 3 || len(requests[1].Tools) != 0 {
		t.Fatalf("Deliver or repair request advertised an invalid tool: %#v", requests)
	}
	proposals, err := st.ListPlanDeliveryProposals(ctx, run.ID, 10)
	if err != nil || len(proposals) != 0 {
		t.Fatalf("unadvertised Plan tool persisted a proposal: %#v err=%v", proposals, err)
	}
	usage, err := st.GetToolCallUsage(ctx, run.ID)
	if err != nil || usage.Consumed != 0 {
		t.Fatalf("unadvertised Plan tool consumed tool budget: %#v err=%v", usage, err)
	}
}

func TestPlanDeliveryChoiceFailsClosedWithActiveLease(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "plan-delivery-lease.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run, proposal := createPausedPlanProposal(t, ctx, st)
	lease := acquireTestRunExecutionLease(t, ctx, st, run.ID)
	defer func() { _, _, _ = st.ReleaseRunExecutionLease(ctx, lease) }()
	_, err = application.NewPlanDeliveryService(st).Select(ctx,
		application.SelectPlanDeliveryDirectionRequest{
			ProposalID: proposal.ID, Direction: 1,
			OperationKey: "leased-choice-0001", RequestedBy: "operator",
		})
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("active lease choice error=%v", err)
	}
	items, listErr := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if listErr != nil || len(items) != 0 {
		t.Fatalf("denied choice left partial WorkItems: %#v err=%v", items, listErr)
	}
}

func TestPlanDeliveryChoiceRejectsAmbiguousOperationKeys(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "plan-delivery-key.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	_, proposal := createPausedPlanProposal(t, ctx, st)
	service := application.NewPlanDeliveryService(st)
	for _, key := range []string{"short", " padded-plan-choice-0001 ",
		"plan choice with spaces", strings.Repeat("x", domain.MaxAgentOperationKeyBytes+1)} {
		_, err := service.Select(ctx, application.SelectPlanDeliveryDirectionRequest{
			ProposalID: proposal.ID, Direction: 1, OperationKey: key,
			RequestedBy: "operator",
		})
		if apperror.CodeOf(err) != apperror.CodeInvalidArgument {
			t.Fatalf("ambiguous operation key %q error=%v", key, err)
		}
	}
	_, err = service.Select(ctx, application.SelectPlanDeliveryDirectionRequest{
		ProposalID: proposal.ID, Direction: 1,
		OperationKey: "valid-plan-choice-0001", RequestedBy: "operator name",
	})
	if apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("ambiguous operator identity error=%v", err)
	}
	items, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: proposal.RunID})
	if err != nil || len(items) != 0 {
		t.Fatalf("invalid operation key left a partial projection: %#v err=%v", items, err)
	}
}

func TestPlanDeliveryConcurrentChoiceReplayConvergesAcrossStoreConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan-delivery-concurrent.db")
	primary, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer primary.Close()
	ctx := context.Background()
	run, proposal := createPausedPlanProposal(t, ctx, primary)
	secondary, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondary.Close()

	services := []*application.PlanDeliveryService{
		application.NewPlanDeliveryService(primary),
		application.NewPlanDeliveryService(secondary),
	}
	results := make([]application.SelectPlanDeliveryDirectionResult, len(services))
	errors := make([]error, len(services))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range services {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errors[index] = services[index].Select(ctx,
				application.SelectPlanDeliveryDirectionRequest{
					ProposalID: proposal.ID, Direction: 2,
					OperationKey: "choose-balanced-concurrent-0001", RequestedBy: "operator",
				})
		}(index)
	}
	close(start)
	wait.Wait()
	for index, err := range errors {
		if err != nil {
			t.Fatalf("concurrent choice %d failed: %v", index, err)
		}
	}
	if results[0].Selection.ID != results[1].Selection.ID ||
		results[0].Selection.NoteID != results[1].Selection.NoteID ||
		len(results[0].WorkItems) != 2 || len(results[1].WorkItems) != 2 ||
		results[0].Replayed == results[1].Replayed {
		t.Fatalf("concurrent replay did not converge: %#v %#v", results[0], results[1])
	}
	items, err := primary.ListWorkItems(ctx, domain.WorkItemFilter{RunID: run.ID})
	if err != nil || len(items) != 2 {
		t.Fatalf("concurrent choice duplicated WorkItems: %#v err=%v", items, err)
	}
	notes, err := primary.ListNotes(ctx, domain.NoteFilter{RunID: run.ID})
	if err != nil || len(notes) != 1 || notes[0].ID != results[0].Selection.NoteID {
		t.Fatalf("concurrent choice duplicated its handoff Note: %#v err=%v", notes, err)
	}
	_, err = services[0].Select(ctx, application.SelectPlanDeliveryDirectionRequest{
		ProposalID: proposal.ID, Direction: 1,
		OperationKey: "choose-balanced-concurrent-0001", RequestedBy: "operator",
	})
	if apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("same operation key accepted different direction: %v", err)
	}
}

func createPausedPlanProposal(t *testing.T, ctx context.Context,
	st *store.SQLiteStore,
) (domain.Run, domain.PlanDeliveryProposal) {
	t.Helper()
	runService := application.NewRunService(st)
	_, run, err := runService.Create(ctx, application.CreateRunRequest{
		Goal: "plan lease gate", Profile: "review", Phase: "plan",
		ModelRoute: "tool-loop/model",
		Budget:     domain.Budget{MaxTurns: 4, MaxTokens: 1000, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runService.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	provider := &scriptedToolProvider{responses: []*llm.ChatResponse{
		toolResponse("provider-plan-lease", "plan_delivery_propose", planDeliveryTestPayload),
		textResponse(rootActionResponse(domain.RootActionWait,
			"directions ready", "", "operator choice")),
	}}
	if _, err := newToolLoopSupervisor(st, provider).Step(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	proposals, err := st.ListPlanDeliveryProposals(ctx, run.ID, 10)
	if err != nil || len(proposals) != 1 {
		t.Fatalf("proposal fixture failed: %#v err=%v", proposals, err)
	}
	return run, proposals[0]
}
