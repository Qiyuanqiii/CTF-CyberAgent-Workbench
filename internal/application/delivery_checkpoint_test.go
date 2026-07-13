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
	"cyberagent-workbench/internal/store"
)

func TestDeliveryCheckpointGatesSelectedWorkItemsAndFinalBoundary(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "delivery-checkpoint.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run, proposal := createPausedPlanProposal(t, ctx, st)
	chosen, err := application.NewPlanDeliveryService(st).Select(ctx,
		application.SelectPlanDeliveryDirectionRequest{
			ProposalID: proposal.ID, Direction: 2,
			OperationKey: "delivery-choice-0001", RequestedBy: "operator",
		})
	if err != nil || len(chosen.WorkItems) != 2 {
		t.Fatalf("Delivery selection fixture failed: %#v err=%v", chosen, err)
	}
	runService := application.NewRunService(st)
	if _, err := runService.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "deliver", OperationKey: "delivery-mode-0001",
		RequestedBy: "operator", Reason: "accepted balanced direction",
	}); err != nil {
		t.Fatal(err)
	}
	work := application.NewWorkItemService(st)
	first, err := work.Transition(ctx, chosen.WorkItems[0].ID, 0,
		domain.WorkItemInProgress, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := work.Transition(ctx, first.ID, first.Version,
		domain.WorkItemCompleted, ""); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("selected WorkItem completed without a checkpoint: %v", err)
	}
	checkpointService := application.NewDeliveryCheckpointService(st)
	request := application.RecordDeliveryCheckpointRequest{
		WorkItemID: first.ID, OperationKey: "delivery-checkpoint-0001",
		RequestedBy: "operator", FocusedVerification: "go test ./internal/application passed",
		DiffAudit: "git diff --check passed", SecurityAudit: "focused security review passed",
		HandoffSummary: "Implemented and verified the bounded core path.",
	}
	firstGate, err := checkpointService.Record(ctx, request)
	if err != nil || firstGate.Replayed || firstGate.Checkpoint.FullGateRequired ||
		firstGate.Checkpoint.WorkItemVersion != first.Version || !firstGate.Note.Pinned ||
		firstGate.Note.Title != "Delivery handoff: slice 1" {
		t.Fatalf("first Delivery checkpoint failed: %#v err=%v", firstGate, err)
	}
	replay, err := checkpointService.Record(ctx, request)
	if err != nil || !replay.Replayed || replay.Checkpoint.ID != firstGate.Checkpoint.ID ||
		replay.Note.ID != firstGate.Note.ID {
		t.Fatalf("Delivery checkpoint replay drifted: %#v err=%v", replay, err)
	}
	changed := request
	changed.DiffAudit = "different result"
	if _, err := checkpointService.Record(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("same Delivery key accepted changed intent: %v", err)
	}
	if _, err := application.NewNoteService(st).Update(ctx,
		application.UpdateNoteRequest{ID: firstGate.Note.ID,
			ExpectedVersion: firstGate.Note.Version,
			Content:         stringPointerForDelivery("tampered")}); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("Delivery handoff Note remained mutable: %v", err)
	}
	if _, err := runService.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "plan", OperationKey: "delivery-mode-0002",
		RequestedBy: "operator", Reason: "revisit plan before completion",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runService.ChangePhase(ctx, application.ChangeRunPhaseRequest{
		RunID: run.ID, Phase: "deliver", OperationKey: "delivery-mode-0003",
		RequestedBy: "operator", Reason: "return to delivery",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Transition(ctx, first.ID, first.Version,
		domain.WorkItemCompleted, ""); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("stale mode checkpoint authorized completion: %v", err)
	}
	request.OperationKey = "delivery-checkpoint-0002"
	currentGate, err := checkpointService.Record(ctx, request)
	if err != nil || currentGate.Checkpoint.ModeRevision == firstGate.Checkpoint.ModeRevision {
		t.Fatalf("current mode checkpoint was not recorded: %#v err=%v", currentGate, err)
	}
	first, err = work.Transition(ctx, first.ID, first.Version,
		domain.WorkItemCompleted, "")
	if err != nil || first.Status != domain.WorkItemCompleted {
		t.Fatalf("current checkpoint did not authorize completion: %#v err=%v", first, err)
	}
	second, err := work.Transition(ctx, chosen.WorkItems[1].ID, 0,
		domain.WorkItemInProgress, "")
	if err != nil {
		t.Fatal(err)
	}
	finalRequest := application.RecordDeliveryCheckpointRequest{
		WorkItemID: second.ID, OperationKey: "delivery-checkpoint-final-0001",
		RequestedBy: "operator", FocusedVerification: "focused package tests passed",
		DiffAudit: "full diff reviewed", SecurityAudit: "security boundary reviewed",
		HandoffSummary: "Audited the completed delivery module.",
	}
	if _, err := checkpointService.Record(ctx, finalRequest); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("final boundary accepted a partial gate: %v", err)
	}
	finalRequest.FunctionalVerification = "go test ./... passed"
	finalRequest.RobustnessAudit = "race and failure-path review passed"
	finalGate, err := checkpointService.Record(ctx, finalRequest)
	if err != nil || !finalGate.Checkpoint.FullGateRequired {
		t.Fatalf("full Delivery boundary checkpoint failed: %#v err=%v", finalGate, err)
	}
	second, err = work.Transition(ctx, second.ID, second.Version,
		domain.WorkItemCompleted, "")
	if err != nil || second.Status != domain.WorkItemCompleted {
		t.Fatalf("final Delivery WorkItem did not complete: %#v err=%v", second, err)
	}
	checkpoints, err := checkpointService.List(ctx, run.ID, 20)
	if err != nil || len(checkpoints) != 3 {
		t.Fatalf("Delivery checkpoint history is incomplete: %#v err=%v", checkpoints, err)
	}
	mode, err := st.GetRunMode(ctx, run.ID)
	if err != nil || !domain.DeliveryCheckpointReady(finalGate.Checkpoint, second, mode) {
		t.Fatalf("completed Delivery gate projection is not ready: mode=%#v err=%v", mode, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countEventType(timeline, events.DeliveryCheckpointRecordedEvent) != 3 {
		t.Fatalf("Delivery checkpoint timeline is inconsistent: %#v err=%v", timeline, err)
	}
	for _, event := range timeline {
		if event.Type != events.DeliveryCheckpointRecordedEvent {
			continue
		}
		for _, private := range []string{
			request.FocusedVerification, finalRequest.SecurityAudit,
			firstGate.Checkpoint.AcceptanceFingerprint,
			firstGate.Checkpoint.SourceFingerprint,
			firstGate.Checkpoint.HandoffDigest,
		} {
			if strings.Contains(event.PayloadJSON, private) {
				t.Fatalf("Delivery evidence or internal digest leaked into event %s",
					event.PayloadJSON)
			}
		}
	}
}

func stringPointerForDelivery(value string) *string { return &value }

func TestDeliveryCheckpointConcurrentReplayConvergesAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery-checkpoint-concurrent.db")
	primary, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer primary.Close()
	ctx := context.Background()
	run, proposal := createPausedPlanProposal(t, ctx, primary)
	selected, err := application.NewPlanDeliveryService(primary).Select(ctx,
		application.SelectPlanDeliveryDirectionRequest{
			ProposalID: proposal.ID, Direction: 2,
			OperationKey: "delivery-concurrent-choice", RequestedBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewRunService(primary).ChangePhase(ctx,
		application.ChangeRunPhaseRequest{RunID: run.ID, Phase: "deliver",
			OperationKey: "delivery-concurrent-mode", RequestedBy: "operator",
			Reason: "accepted direction"}); err != nil {
		t.Fatal(err)
	}
	item, err := application.NewWorkItemService(primary).Transition(ctx,
		selected.WorkItems[0].ID, 0, domain.WorkItemInProgress, "")
	if err != nil {
		t.Fatal(err)
	}
	secondary, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondary.Close()
	stores := []*store.SQLiteStore{primary, secondary}
	request := application.RecordDeliveryCheckpointRequest{
		WorkItemID: item.ID, OperationKey: "delivery-concurrent-checkpoint",
		RequestedBy: "operator", FocusedVerification: "focused tests passed",
		DiffAudit: "diff review passed", SecurityAudit: "security review passed",
		HandoffSummary: "concurrent checkpoint handoff",
	}
	const callers = 8
	results := make([]application.RecordDeliveryCheckpointResult, callers)
	errorsFound := make([]error, callers)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := 0; index < callers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errorsFound[index] = application.NewDeliveryCheckpointService(
				stores[index%len(stores)]).Record(ctx, request)
		}(index)
	}
	close(start)
	wait.Wait()
	winnerID := results[0].Checkpoint.ID
	if winnerID == "" {
		t.Fatalf("first concurrent checkpoint failed: %v", errorsFound[0])
	}
	for index := range results {
		if errorsFound[index] != nil || results[index].Checkpoint.ID != winnerID ||
			results[index].Note.ID != results[0].Note.ID {
			t.Fatalf("concurrent checkpoint %d diverged: %#v err=%v",
				index, results[index], errorsFound[index])
		}
	}
	checkpoints, err := primary.ListDeliveryCheckpoints(ctx, run.ID, 20)
	if err != nil || len(checkpoints) != 1 {
		t.Fatalf("concurrent replay duplicated checkpoint rows: %#v err=%v",
			checkpoints, err)
	}
	timeline, err := primary.ListRunEvents(ctx, run.ID)
	if err != nil || countEventType(timeline, events.DeliveryCheckpointRecordedEvent) != 1 {
		t.Fatalf("concurrent replay duplicated events: %#v err=%v", timeline, err)
	}
}
