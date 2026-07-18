package application_test

import (
	"context"
	"path/filepath"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/operationreceipt"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
)

type pendingWakeHandoff struct{}

func (pendingWakeHandoff) Execute(context.Context,
	application.ExecuteRunHandoffRequest,
) (application.ExecuteRunHandoffResult, error) {
	return application.ExecuteRunHandoffResult{Handoff: domain.RunExecutionHandoff{
		Operation: domain.RunExecutionHandoffOperation{ID: "run-handoff-still-pending"},
	}}, apperror.New(apperror.CodeConflict, "Run execution lease is active")
}

func TestForegroundRunWakeConsumerExecutesOnceAndSettlesCompleted(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "wake-consumer.db")
	state, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionContinue, "wake delivered", "", ""),
	}}
	_, created, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "foreground wake", Profile: "code",
			ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.NewRunService(state).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "execute from wake",
		OperationKey: "wake-consumer-steering-0001", RequestedBy: "operator",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewRunWakeControlService(state).Schedule(ctx,
		application.ScheduleRunWakeRequest{
			Version: domain.RunWakeControlProtocolVersion, RunID: run.ID,
			OperationKey: "wake-consumer-schedule-0001", RequestedBy: "operator",
			MaxAttempts: 3, InitialDelaySeconds: 0, BaseBackoffSeconds: 5,
			MaxBackoffSeconds: 30, MaxElapsedSeconds: 120,
		}); err != nil {
		t.Fatal(err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	handoff := application.NewRunExecutionHandoffService(state, router,
		policy.NewDefaultChecker())
	consumer := application.NewForegroundRunWakeConsumer(state, handoff)
	request := application.ConsumeRunWakeRequest{
		Version: domain.RunWakeConsumerProtocolVersion, RunID: run.ID,
		OwnerID: "foreground_test", MaxSteps: 1,
	}
	result, err := consumer.Consume(ctx, request)
	if err != nil || result.Intent.Status != domain.RunWakeCompleted ||
		result.Consumption.Status != domain.RunWakeConsumptionCompleted ||
		result.Handoff.Result == nil || provider.calls != 1 {
		t.Fatalf("consume result=%#v calls=%d err=%v", result, provider.calls, err)
	}
	records, err := state.ListTerminalOperationRecords(ctx, run.ID, 2)
	if err != nil || len(records) != 1 ||
		records[0].Kind != operationreceipt.KindRunWakeConsume ||
		records[0].Outcome != "completed" {
		t.Fatalf("Run wake terminal receipt source=%#v err=%v", records, err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	consumer = application.NewForegroundRunWakeConsumer(state,
		application.NewRunExecutionHandoffService(state, router,
			policy.NewDefaultChecker()))
	replay, err := consumer.Consume(ctx, request)
	if err != nil || !replay.Replayed || replay.Intent.Status != domain.RunWakeCompleted ||
		replay.Handoff.Result == nil || !replay.Handoff.Result.ModelCalled ||
		replay.Handoff.Operation.ID != replay.Consumption.HandoffOperationID || provider.calls != 1 {
		t.Fatalf("restart replay=%#v calls=%d err=%v", replay, provider.calls, err)
	}
}

func TestForegroundRunWakeConsumerPreservesPreparedHandoffForLaterReplay(t *testing.T) {
	ctx := context.Background()
	state, err := store.Open(filepath.Join(t.TempDir(), "wake-pending-replay.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionContinue, "wake replayed", "", ""),
	}}
	_, created, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "resume pending foreground wake", Profile: "code",
			ModelRoute: provider.Name() + "/model", Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.NewRunService(state).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "resume this wake",
		OperationKey: "wake-pending-steering-0001", RequestedBy: "operator",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewRunWakeControlService(state).Schedule(ctx,
		application.ScheduleRunWakeRequest{
			Version: domain.RunWakeControlProtocolVersion, RunID: run.ID,
			OperationKey: "wake-pending-schedule-0001", RequestedBy: "operator",
			MaxAttempts: 3, BaseBackoffSeconds: 5, MaxBackoffSeconds: 30,
			MaxElapsedSeconds: 120,
		}); err != nil {
		t.Fatal(err)
	}
	request := application.ConsumeRunWakeRequest{
		Version: domain.RunWakeConsumerProtocolVersion, RunID: run.ID,
		OwnerID: "pending_replay_test", MaxSteps: 1,
	}
	pending, err := application.NewForegroundRunWakeConsumer(state,
		pendingWakeHandoff{}).Consume(ctx, request)
	if apperror.CodeOf(err) != apperror.CodeConflict ||
		pending.Consumption.Status != domain.RunWakeConsumptionPrepared ||
		pending.Intent.Status != domain.RunWakeLeased || provider.calls != 0 {
		t.Fatalf("pending consume=%#v calls=%d err=%v", pending, provider.calls, err)
	}
	stored, found, err := state.GetRunWakeConsumption(ctx, pending.Intent.ID,
		pending.Intent.AttemptCount)
	if err != nil || !found || stored.Status != domain.RunWakeConsumptionPrepared {
		t.Fatalf("prepared consumption was not preserved: %#v found=%t err=%v",
			stored, found, err)
	}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	resumed, err := application.NewForegroundRunWakeConsumer(state,
		application.NewRunExecutionHandoffService(state, router,
			policy.NewDefaultChecker())).Consume(ctx, request)
	if err != nil || resumed.Intent.Status != domain.RunWakeCompleted ||
		resumed.Consumption.Status != domain.RunWakeConsumptionCompleted ||
		provider.calls != 1 {
		t.Fatalf("resumed consume=%#v calls=%d err=%v", resumed, provider.calls, err)
	}
}
