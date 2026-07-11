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
	"cyberagent-workbench/internal/session"
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
	sent, err := service.Send(ctx, coordinator.SendRequest{
		RunID: run.ID, RecipientAgentID: root.ID, Kind: domain.AgentMessageInstruction,
		Payload:        map[string]any{"goal": "inspect", "credential": rawSecret},
		IdempotencyKey: "coordinator-message-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := sent.Message
	if sent.Replayed {
		t.Fatal("first agent message send was reported as a replay")
	}
	if strings.Contains(message.PayloadJSON, rawSecret) || !strings.Contains(message.PayloadJSON, "[REDACTED:api-key]") {
		t.Fatalf("agent inbox did not redact its payload: %s", message.PayloadJSON)
	}
	replayed, err := service.Send(ctx, coordinator.SendRequest{
		RunID: run.ID, RecipientAgentID: root.ID, Kind: domain.AgentMessageInstruction,
		Payload:        map[string]any{"goal": "inspect", "credential": rawSecret},
		IdempotencyKey: "coordinator-message-0001",
	})
	if err != nil || !replayed.Replayed || replayed.Message.ID != message.ID {
		t.Fatalf("agent message replay was not stable: result=%#v err=%v", replayed, err)
	}
	if _, err := service.Send(ctx, coordinator.SendRequest{
		RunID: run.ID, RecipientAgentID: root.ID, Kind: domain.AgentMessageInstruction,
		Payload:        map[string]any{"goal": "different intent"},
		IdempotencyKey: "coordinator-message-0001",
	}); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("idempotency key reuse did not conflict: code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := service.Send(ctx, coordinator.SendRequest{
		RunID: run.ID, RecipientAgentID: root.ID, Kind: domain.AgentMessageInstruction,
		Payload:        map[string]any{rawSecret: "must not persist"},
		IdempotencyKey: "coordinator-message-0002",
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

func TestSpecialistAdmissionIsOptInBoundedAndLifecycleSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "specialists.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	runService := application.NewRunService(st)
	_, run, err := runService.Create(ctx, application.CreateRunRequest{
		Goal: "coordinate bounded specialists", Profile: "code",
		Budget: domain.Budget{MaxTurns: 10, MaxTokens: 1000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runService.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	root, found, err := st.GetRootAgent(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("root was not created: found=%t err=%v", found, err)
	}
	firstRequest := coordinator.AdmitSpecialistRequest{
		RunID: run.ID, ParentAgentID: root.ID, Title: "focused note analyst",
		Skills: []string{"note_create", "model.chat"}, TurnLimit: 2, TokenLimit: 200,
		IdempotencyKey: "specialist-admission-0001",
	}
	if _, err := coordinator.New(st).AdmitSpecialist(ctx, firstRequest); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("default coordinator enabled specialist admission: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	if _, err := coordinator.NewWithSpecialistAdmission(st, coordinator.SpecialistAdmissionPolicy{}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("invalid specialist policy was accepted: code=%s err=%v", apperror.CodeOf(err), err)
	}
	service, err := coordinator.NewWithSpecialistAdmission(st, coordinator.SpecialistAdmissionPolicy{
		MaxChildren: 2, MaxTurnsPerChild: 3, MaxTokensPerChild: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	overBudget := firstRequest
	overBudget.TurnLimit = 4
	overBudget.IdempotencyKey = "specialist-admission-over-budget"
	if _, err := service.AdmitSpecialist(ctx, overBudget); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("per-child admission budget was not enforced: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	escalated := firstRequest
	escalated.Skills = []string{"shell.exec"}
	escalated.IdempotencyKey = "specialist-admission-bad-skill"
	if _, err := service.AdmitSpecialist(ctx, escalated); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("specialist skill escalation was not rejected: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	first, err := service.AdmitSpecialist(ctx, firstRequest)
	if err != nil || first.Replayed {
		t.Fatalf("first specialist admission failed: result=%#v err=%v", first, err)
	}
	if first.Agent.Role != domain.AgentRoleSpecialist || first.Agent.ParentID != root.ID ||
		first.Agent.Depth != 1 || first.Agent.ChildLimit != 0 || first.Agent.TurnLimit != 2 ||
		first.Agent.TokenLimit != 200 || first.Agent.SessionID == root.SessionID {
		t.Fatalf("unexpected first specialist: %#v", first.Agent)
	}
	childSession, err := st.GetSession(ctx, first.Agent.SessionID)
	if err != nil || childSession.Status != session.StatusActive || childSession.Route != "code" ||
		childSession.WorkspaceID != "" {
		t.Fatalf("specialist Session was not independently created: session=%#v err=%v", childSession, err)
	}
	replayed, err := service.AdmitSpecialist(ctx, firstRequest)
	if err != nil || !replayed.Replayed || replayed.Agent.ID != first.Agent.ID ||
		replayed.Agent.SessionID != first.Agent.SessionID {
		t.Fatalf("specialist admission replay was not stable: result=%#v err=%v", replayed, err)
	}
	changed := firstRequest
	changed.Title = "changed specialist intent"
	if _, err := service.AdmitSpecialist(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed admission intent did not conflict: code=%s err=%v", apperror.CodeOf(err), err)
	}
	projectedRoot, err := st.GetAgentNode(ctx, root.ID)
	if err != nil || projectedRoot.ChildLimit != 2 || projectedRoot.TurnLimit != 8 ||
		projectedRoot.TokenLimit != 800 {
		t.Fatalf("first reservation was not projected to root: root=%#v err=%v", projectedRoot, err)
	}
	secondRequest := coordinator.AdmitSpecialistRequest{
		RunID: run.ID, ParentAgentID: root.ID, Title: "focused work planner",
		Skills: []string{"work_item_create"}, TurnLimit: 2, TokenLimit: 200,
		IdempotencyKey: "specialist-admission-0002",
	}
	second, err := service.AdmitSpecialist(ctx, secondRequest)
	if err != nil || second.Replayed || second.Agent.ID == first.Agent.ID {
		t.Fatalf("second specialist admission failed: result=%#v err=%v", second, err)
	}
	projectedRoot, err = st.GetAgentNode(ctx, root.ID)
	if err != nil || projectedRoot.TurnLimit != 6 || projectedRoot.TokenLimit != 600 {
		t.Fatalf("second reservation was not projected to root: root=%#v err=%v", projectedRoot, err)
	}
	third := secondRequest
	third.Title = "over capacity"
	third.IdempotencyKey = "specialist-admission-0003"
	if _, err := service.AdmitSpecialist(ctx, third); apperror.CodeOf(err) != apperror.CodeResourceExhausted {
		t.Fatalf("third specialist was not rejected: code=%s err=%v", apperror.CodeOf(err), err)
	}
	graph, err := service.Restore(ctx, run.ID)
	if err != nil || len(graph.Nodes) != domain.MaxAgentNodesPerRun {
		t.Fatalf("specialist graph was not recoverable: nodes=%d err=%v", len(graph.Nodes), err)
	}
	if _, err := runService.Pause(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.Agent.ID, second.Agent.ID} {
		node, err := st.GetAgentNode(ctx, id)
		if err != nil || node.Status != domain.AgentWaiting || node.StatusReason != "run paused" {
			t.Fatalf("paused Run did not pause specialist %s: node=%#v err=%v", id, node, err)
		}
	}
	if _, err := runService.Resume(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.Agent.ID, second.Agent.ID} {
		node, err := st.GetAgentNode(ctx, id)
		if err != nil || node.Status != domain.AgentReady {
			t.Fatalf("resumed Run did not restore specialist %s: node=%#v err=%v", id, node, err)
		}
	}
	if _, err := runService.Cancel(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	for _, child := range []domain.AgentNode{first.Agent, second.Agent} {
		node, err := st.GetAgentNode(ctx, child.ID)
		if err != nil || node.Status != domain.AgentCancelled || node.FinishedAt == nil {
			t.Fatalf("Run cancellation did not terminate specialist %s: node=%#v err=%v", child.ID, node, err)
		}
		sess, err := st.GetSession(ctx, child.SessionID)
		if err != nil || sess.Status != session.StatusArchived {
			t.Fatalf("terminal specialist Session was not archived: session=%#v err=%v", sess, err)
		}
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countType(items, events.AgentCapacityReservedEvent) != 2 ||
		countType(items, events.AgentRegisteredEvent) != 3 {
		t.Fatalf("unexpected specialist admission event stream: %#v", items)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	service, err = coordinator.NewWithSpecialistAdmission(st, coordinator.SpecialistAdmissionPolicy{
		MaxChildren: 2, MaxTurnsPerChild: 3, MaxTokensPerChild: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	restartedGraph, err := service.Restore(ctx, run.ID)
	if err != nil || len(restartedGraph.Nodes) != 3 {
		t.Fatalf("restart did not restore specialist graph: graph=%#v err=%v", restartedGraph, err)
	}
	replayed, err = service.AdmitSpecialist(ctx, firstRequest)
	if err != nil || !replayed.Replayed || replayed.Agent.ID != first.Agent.ID ||
		replayed.Agent.Status != domain.AgentCancelled {
		t.Fatalf("terminal admission replay was not stable: result=%#v err=%v", replayed, err)
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
