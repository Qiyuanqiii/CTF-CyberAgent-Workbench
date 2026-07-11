package coordinator_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/coordinator"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/store"
)

func TestCoordinatorRestoresStableRootAndExactlyOnceInbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coordinator.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, run, err := application.NewRunService(st).Create(ctx, application.CreateRunRequest{
		Goal: "coordinate one root", Profile: "code", Budget: domain.Budget{MaxTurns: 8, MaxTokens: 1000},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := coordinator.New(st)
	root, created, err := service.RegisterRoot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if created || root.Status != domain.AgentReady || root.ChildLimit != 0 || root.TurnLimit != 8 || root.TokenLimit != 1000 {
		t.Fatalf("unexpected initial root projection: created=%t root=%#v", created, root)
	}
	initial, err := service.Restore(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if initial.RootAgentID != root.ID || len(initial.Nodes) != 1 || initial.LatestSnapshot.Version != 1 {
		t.Fatalf("unexpected initial graph: %#v", initial)
	}

	rawSecret := "sk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	message, err := service.Send(ctx, coordinator.SendRequest{
		RunID: run.ID, RecipientAgentID: root.ID, Kind: domain.AgentMessageInstruction,
		Payload: map[string]any{"goal": "inspect", "credential": rawSecret},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(message.PayloadJSON, rawSecret) || !strings.Contains(message.PayloadJSON, "[REDACTED:api-key]") {
		t.Fatalf("agent inbox did not redact its payload: %s", message.PayloadJSON)
	}
	if _, err := service.Send(ctx, coordinator.SendRequest{
		RunID: run.ID, RecipientAgentID: root.ID, Kind: domain.AgentMessageInstruction,
		Payload: map[string]any{rawSecret: "must not persist"},
	}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("sensitive agent message field name was not rejected: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	graph, err := service.Restore(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.PendingMessages) != 1 || graph.PendingMessages[0].ID != message.ID ||
		graph.LatestSnapshot.Version != 2 {
		t.Fatalf("sent message was not snapshotted: %#v", graph)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	service = coordinator.New(st)
	restored, err := service.Restore(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.RootAgentID != root.ID || len(restored.PendingMessages) != 1 {
		t.Fatalf("restart did not restore root identity and inbox: %#v", restored)
	}
	consumed, err := service.Consume(ctx, root.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(consumed) != 1 || consumed[0].ID != message.ID || consumed[0].Status != domain.AgentMessageConsumed {
		t.Fatalf("unexpected consumed inbox batch: %#v", consumed)
	}
	again, err := service.Consume(ctx, root.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("inbox message was consumed twice: %#v", again)
	}
	finalGraph, err := service.Restore(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(finalGraph.PendingMessages) != 0 || finalGraph.LatestSnapshot.Version != 3 {
		t.Fatalf("consumption was not projected into the graph: %#v", finalGraph)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countType(items, events.AgentRegisteredEvent) != 1 || countType(items, events.AgentMessageSentEvent) != 1 ||
		countType(items, events.AgentMessageConsumedEvent) != 1 {
		t.Fatalf("unexpected coordinator audit stream: %#v", items)
	}
}

func TestCoordinatorRegistrationIsConcurrentAndRunCancellationCascades(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "coordinator.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	runService := application.NewRunService(st)
	_, run, err := runService.Create(ctx, application.CreateRunRequest{
		Goal: "bounded registration", Profile: "learn", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := coordinator.New(st)
	const workers = 8
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			node, _, err := service.RegisterRoot(ctx, run.ID)
			if err != nil {
				errs <- err
				return
			}
			ids <- node.ID
		}()
	}
	wait.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent registration failed: %v", err)
	}
	rootID := ""
	for id := range ids {
		if rootID == "" {
			rootID = id
		}
		if id != rootID {
			t.Fatalf("concurrent registration returned multiple roots: %s and %s", rootID, id)
		}
	}
	if _, err := runService.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := runService.Cancel(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	graph, err := service.Restore(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 1 || graph.Nodes[0].ID != rootID || graph.Nodes[0].Status != domain.AgentCancelled ||
		graph.Nodes[0].FinishedAt == nil {
		t.Fatalf("run cancellation did not cascade to the root graph: %#v", graph)
	}
}

func countType(items []events.Event, eventType string) int {
	count := 0
	for _, item := range items {
		if item.Type == eventType {
			count++
		}
	}
	return count
}
