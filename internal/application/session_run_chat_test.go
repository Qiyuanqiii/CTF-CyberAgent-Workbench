package application_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
)

func TestSessionRunChatAutoStartsAndCommitsOneMessagePair(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "interactive review", Profile: "review", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	router := llm.NewDefaultRouter()
	manager := session.NewManager(st, router, policy.NewDefaultChecker()).WithRunChatExecutor(
		application.NewSessionRunChatExecutor(st, router, policy.NewDefaultChecker()),
	)
	result, err := manager.Send(ctx, sess.ID, "review this exact input")
	if err != nil {
		t.Fatal(err)
	}
	if result.RunID != run.ID || result.RunAction != string(domain.RootActionContinue) || result.RunStatus != string(domain.RunRunning) {
		t.Fatalf("unexpected run chat result: %#v", result)
	}
	if result.UserMessage.Content != "review this exact input" || result.UserMessage.ID == 0 || result.ReplyMessage.ID == 0 {
		t.Fatalf("unexpected committed messages: %#v", result)
	}
	messages, err := st.ListSessionMessages(ctx, sess.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content != "review this exact input" || messages[0].ID != result.UserMessage.ID || messages[1].ID != result.ReplyMessage.ID {
		t.Fatalf("run chat duplicated or changed messages: %#v", messages)
	}
	persisted, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, ok, err := st.GetSupervisorCheckpoint(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("checkpoint lookup ok=%t err=%v", ok, err)
	}
	if persisted.Status != domain.RunRunning || checkpoint.NextTurn != 2 || checkpoint.Phase != domain.SupervisorIdle || checkpoint.PendingInput != "" {
		t.Fatalf("unexpected persisted run chat state: run=%#v checkpoint=%#v", persisted, checkpoint)
	}
	items, err := st.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countEventType(items, events.AgentTurnCompletedEvent) != 1 || countEventType(items, events.SupervisorActionEvent) != 1 || countEventType(items, events.SessionMessageEvent) != 2 {
		t.Fatalf("unexpected run chat events: %#v", items)
	}
}

func TestSessionRunChatKeepsLegacyUnboundSessionCompatible(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	router := llm.NewDefaultRouter()
	manager := session.NewManager(st, router, policy.NewDefaultChecker()).WithRunChatExecutor(
		application.NewSessionRunChatExecutor(st, router, policy.NewDefaultChecker()),
	)
	sess, err := manager.Create(ctx, "", "legacy session", "learn")
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Send(ctx, sess.ID, "legacy chat")
	if err != nil {
		t.Fatal(err)
	}
	if result.RunID != "" || result.RunAction != "" || result.UserMessage.Content != "legacy chat" {
		t.Fatalf("legacy session unexpectedly used a run: %#v", result)
	}
	messages, err := st.ListSessionMessages(ctx, sess.ID, true)
	if err != nil || len(messages) != 2 {
		t.Fatalf("legacy message persistence len=%d err=%v", len(messages), err)
	}
}

func TestSessionRunChatWaitResumesWithNewInputAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cyberagent.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "interactive wait", Profile: "learn", ModelRoute: "lifecycle-test/model", Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionWait, "Please provide a choice.", "", "user choice required"),
		rootActionResponse(domain.RootActionContinue, "Choice received.", "", ""),
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	sess, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	manager := session.NewManager(st, router, policy.NewDefaultChecker()).WithRunChatExecutor(
		application.NewSessionRunChatExecutor(st, router, policy.NewDefaultChecker()),
	)
	first, err := manager.Send(ctx, sess.ID, "Which path should I take?")
	if err != nil {
		t.Fatal(err)
	}
	if first.RunAction != string(domain.RootActionWait) || first.RunStatus != string(domain.RunPaused) {
		t.Fatalf("unexpected wait result: %#v", first)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	manager = session.NewManager(st, router, policy.NewDefaultChecker()).WithRunChatExecutor(
		application.NewSessionRunChatExecutor(st, router, policy.NewDefaultChecker()),
	)
	second, err := manager.Send(ctx, sess.ID, "Take the safe path.")
	if err != nil {
		t.Fatal(err)
	}
	if second.RunAction != string(domain.RootActionContinue) || second.RunStatus != string(domain.RunRunning) {
		t.Fatalf("unexpected resumed result: %#v", second)
	}
	messages, err := st.ListSessionMessages(ctx, sess.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 || messages[0].Content != "Which path should I take?" || messages[2].Content != "Take the safe path." {
		t.Fatalf("unexpected resumed history: %#v", messages)
	}
	checkpoint, ok, err := st.GetSupervisorCheckpoint(ctx, run.ID)
	if err != nil || !ok || checkpoint.NextTurn != 3 || checkpoint.Phase != domain.SupervisorIdle || checkpoint.PendingInput != "" {
		t.Fatalf("unexpected resumed checkpoint ok=%t checkpoint=%#v err=%v", ok, checkpoint, err)
	}
}

func TestSessionRunChatRejectsMessagesAfterModelFinish(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "terminal session", Profile: "review", ModelRoute: "lifecycle-test/model", Budget: domain.Budget{MaxTurns: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider := &lifecycleProvider{responses: []string{
		rootActionResponse(domain.RootActionFinish, "Finished.", "session complete", ""),
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	sess, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	manager := session.NewManager(st, router, policy.NewDefaultChecker()).WithRunChatExecutor(
		application.NewSessionRunChatExecutor(st, router, policy.NewDefaultChecker()),
	)
	first, err := manager.Send(ctx, sess.ID, "finish this")
	if err != nil {
		t.Fatal(err)
	}
	if first.RunStatus != string(domain.RunCompleted) {
		t.Fatalf("run did not finish: %#v", first)
	}
	if _, err := manager.Send(ctx, sess.ID, "one more thing"); apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("terminal session accepted another message code=%s err=%v", apperror.CodeOf(err), err)
	}
	messages, err := st.ListSessionMessages(ctx, sess.ID, true)
	if err != nil || len(messages) != 2 {
		t.Fatalf("terminal rejection changed history len=%d err=%v", len(messages), err)
	}
}

func TestSessionRunChatFeedsCompactedSummaryBackToSupervisor(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "long interactive session", Profile: "learn", ModelRoute: "lifecycle-test/model", Budget: domain.Budget{MaxTurns: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	responses := make([]string, 6)
	for i := range responses {
		responses[i] = rootActionResponse(domain.RootActionContinue, "continue", "", "")
	}
	provider := &lifecycleProvider{responses: responses}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	sess, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	manager := session.NewManager(st, router, policy.NewDefaultChecker()).WithRunChatExecutor(
		application.NewSessionRunChatExecutor(st, router, policy.NewDefaultChecker()),
	)
	var fifth session.SendResult
	for i := 1; i <= 6; i++ {
		result, err := manager.Send(ctx, sess.ID, "message "+string(rune('0'+i)))
		if err != nil {
			t.Fatal(err)
		}
		if i == 5 {
			fifth = result
		}
	}
	if !fifth.Compacted || fifth.SummaryID == 0 {
		t.Fatalf("fifth turn did not compact context: %#v", fifth)
	}
	if len(provider.requests) != 6 {
		t.Fatalf("provider request count = %d, want 6", len(provider.requests))
	}
	foundSummary := false
	for _, message := range provider.requests[5].Messages {
		if message.Role == "system" && strings.Contains(message.Content, "Compacted session context") {
			t.Fatalf("compacted transcript was elevated to system context: %s", message.Content)
		}
		if message.Role == "user" && strings.Contains(message.Content, "Compacted session context") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Fatalf("compacted summary was not sent to Supervisor: %#v", provider.requests[5].Messages)
	}
}
