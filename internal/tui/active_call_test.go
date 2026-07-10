package tui

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolrun"
)

func TestModelRendersLiveCallAndCancelsWithoutQuitting(t *testing.T) {
	model, controller := newActiveCallTestModel(t)
	model.busy = true
	model.liveGeneration = 1
	discoveryCtx, cancelDiscovery := context.WithCancel(context.Background())
	defer cancelDiscovery()

	subscribed := model.discoverActiveCallCmd(discoveryCtx, model.liveGeneration, model.session.ID)()
	updated, waitCmd := model.Update(subscribed)
	model = updated.(*Model)
	if waitCmd == nil {
		t.Fatal("active call discovery did not start a subscription wait")
	}
	updated, waitCmd = model.Update(waitCmd())
	model = updated.(*Model)
	if waitCmd == nil || model.live.State != "streaming" {
		t.Fatalf("active call snapshot was not applied: %#v", model.live)
	}

	progressInfo := controller.info
	progressInfo.StreamChunks = 3
	progressInfo.StreamBytes = 120
	controller.subscription.events <- application.ActiveCallEvent{
		Version: application.ActiveCallEnvelopeVersion, Sequence: 2,
		Type: application.ActiveCallProgressEvent, Call: progressInfo, DeltaBytes: 120, CreatedAt: time.Now().UTC(),
	}
	updated, waitCmd = model.Update(waitCmd())
	model = updated.(*Model)
	if waitCmd == nil {
		t.Fatal("live progress did not schedule the next subscription wait")
	}
	snapshot := model.Snapshot()
	for _, want := range []string{"live=streaming", "active-test/model", "chunks=3", "bytes=120", "Ctrl+X cancel"} {
		if !strings.Contains(snapshot, want) {
			t.Fatalf("live snapshot missing %q:\n%s", want, snapshot)
		}
	}

	updated, quitCmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(*Model)
	if quitCmd != nil || !strings.Contains(model.status, "Ctrl+X") {
		t.Fatalf("busy escape unexpectedly quit the TUI: cmd=%#v status=%q", quitCmd, model.status)
	}
	updated, cancelCmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	model = updated.(*Model)
	if cancelCmd == nil || !model.cancelPending {
		t.Fatal("Ctrl+X did not start an asynchronous cancellation")
	}
	updated, _ = model.Update(cancelCmd())
	model = updated.(*Model)
	if controller.cancelCalls != 1 || !strings.Contains(model.status, "cancellation requested") {
		t.Fatalf("unexpected cancellation result: calls=%d status=%q", controller.cancelCalls, model.status)
	}

	updated, _ = model.Update(actionDoneMsg{err: context.Canceled})
	model = updated.(*Model)
	if model.busy || model.live.State != "cancelled" {
		t.Fatalf("cancelled action did not settle the TUI state: busy=%t live=%#v", model.busy, model.live)
	}
	_, quitCmd = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if quitCmd == nil {
		t.Fatal("idle escape did not quit the TUI")
	}
	if _, ok := quitCmd().(tea.QuitMsg); !ok {
		t.Fatalf("idle escape returned %T instead of tea.QuitMsg", quitCmd())
	}
}

func TestModelReportsDroppedLiveSubscriber(t *testing.T) {
	model, controller := newActiveCallTestModel(t)
	model.busy = true
	model.liveGeneration = 4
	<-controller.subscription.events
	controller.subscription.dropped.Store(true)
	controller.subscription.Close()

	updated, waitCmd := model.Update(activeCallSubscribedMsg{
		generation: model.liveGeneration, subscription: controller.subscription,
	})
	model = updated.(*Model)
	if waitCmd == nil {
		t.Fatal("closed subscription did not schedule a channel read")
	}
	updated, next := model.Update(waitCmd())
	model = updated.(*Model)
	if next != nil || model.live.State != "disconnected" || !model.live.Dropped {
		t.Fatalf("slow subscriber state was not rendered: next=%#v live=%#v", next, model.live)
	}
	if !strings.Contains(model.Snapshot(), "live=disconnected") || !strings.Contains(model.status, "slow consumer") {
		t.Fatalf("slow subscriber status missing from snapshot:\n%s", model.Snapshot())
	}
}

func TestModelFallsBackToCancellingCurrentRequest(t *testing.T) {
	model, _ := newActiveCallTestModel(t)
	requestCtx, cancel := context.WithCancel(context.Background())
	model.actionCancel = cancel
	model.liveGeneration = 3
	model.cancelPending = true
	updated, _ := model.Update(activeCallCancelDoneMsg{generation: 3})
	model = updated.(*Model)
	select {
	case <-requestCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("missing active call did not cancel the current application request")
	}
	if model.cancelPending || model.status != "current action cancellation requested" || model.live.State != "cancelling" {
		t.Fatalf("unexpected request cancellation fallback state: pending=%t status=%q live=%#v", model.cancelPending, model.status, model.live)
	}
}

func TestModelBatchesSubmitWithLiveDiscovery(t *testing.T) {
	model, _ := newActiveCallTestModel(t)
	model.input.SetValue("hello from TUI")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*Model)
	if cmd == nil || !model.busy || model.live.State != "discovering" {
		t.Fatalf("submit did not enter live discovery: cmd=%#v busy=%t live=%#v", cmd, model.busy, model.live)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("expected submit and discovery batch, got %T %#v", cmd(), cmd())
	}
	model.stopLiveTracking(context.Canceled)
}

func TestModelLiveCallEndToEndCancellation(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	provider := newTUIBlockingProvider()
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	checker := policy.NewDefaultChecker()
	runService := application.NewRunService(st)
	_, run, err := runService.Create(context.Background(), application.CreateRunRequest{
		Goal: "TUI active call", Profile: "review", ModelRoute: provider.Name() + "/model",
		Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.GetSession(context.Background(), run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	registry := application.NewActiveCallRegistry()
	executor := application.NewSessionRunChatExecutor(st, router, checker).WithActiveCalls(registry)
	sessionManager := session.NewManager(st, router, checker).WithRunChatExecutor(executor)
	toolManager := toolrun.NewManager(st, checker)
	supervisor := application.NewRunSupervisor(st, router, checker).WithActiveCalls(registry)
	controller := &supervisorActiveCallController{supervisor: supervisor}
	model, err := NewModel(context.Background(), sess, sessionManager, toolManager)
	if err != nil {
		t.Fatal(err)
	}
	model.WithActiveCallController(controller)
	model.input.SetValue("start blocking model")
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*Model)
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("expected submit/discovery batch, got %T", cmd())
	}
	messages := make(chan tea.Msg, len(batch))
	for _, child := range batch {
		go func(command tea.Cmd) { messages <- command() }(child)
	}
	select {
	case <-provider.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("TUI provider did not start")
	}
	var subscribed tea.Msg
	select {
	case subscribed = <-messages:
	case <-time.After(3 * time.Second):
		t.Fatal("TUI did not discover the active call")
	}
	if _, ok := subscribed.(activeCallSubscribedMsg); !ok {
		t.Fatalf("expected activeCallSubscribedMsg, got %T", subscribed)
	}
	updated, waitCmd := model.Update(subscribed)
	model = updated.(*Model)
	updated, waitCmd = model.Update(waitCmd())
	model = updated.(*Model)
	if model.live.State != "streaming" || waitCmd == nil {
		t.Fatalf("TUI did not apply the active call snapshot: %#v", model.live)
	}
	close(provider.releaseChunk)
	updated, waitCmd = model.Update(waitCmd())
	model = updated.(*Model)
	if model.live.Info.StreamBytes == 0 || waitCmd == nil {
		t.Fatalf("TUI did not receive live stream progress: %#v", model.live)
	}

	updated, cancelCmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	model = updated.(*Model)
	if cancelCmd == nil {
		t.Fatal("Ctrl+X did not produce a cancellation command")
	}
	updated, _ = model.Update(cancelCmd())
	model = updated.(*Model)
	if !strings.Contains(model.status, "cancellation requested") {
		t.Fatalf("unexpected TUI cancellation status: %s", model.status)
	}
	var completed tea.Msg
	select {
	case completed = <-messages:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled TUI send did not complete")
	}
	if _, ok := completed.(actionDoneMsg); !ok {
		t.Fatalf("expected actionDoneMsg, got %T", completed)
	}
	updated, _ = model.Update(completed)
	model = updated.(*Model)
	if model.busy || model.live.State != "cancelled" || !strings.Contains(model.Snapshot(), "live=cancelled") {
		t.Fatalf("TUI did not settle as cancelled: busy=%t live=%#v\n%s", model.busy, model.live, model.Snapshot())
	}
	items, err := st.ListRunEvents(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	cancelEvents := 0
	failedEvents := 0
	for _, event := range items {
		switch event.Type {
		case events.ModelCancelRequestedEvent:
			cancelEvents++
		case events.ModelFailedEvent:
			failedEvents++
		}
	}
	if cancelEvents != 1 || failedEvents != 1 {
		t.Fatalf("unexpected durable TUI cancellation events: cancel=%d failed=%d", cancelEvents, failedEvents)
	}
}

func TestFooterHelpFitsSupportedWidths(t *testing.T) {
	for _, width := range []int{80, 100, 120, 145, 180} {
		if help := footerHelp(width); len(help) > width-2 {
			t.Fatalf("footer help exceeds width %d: len=%d %q", width, len(help), help)
		}
	}
}

type fakeActiveCallController struct {
	info         application.ActiveCallInfo
	active       bool
	subscription *fakeActiveCallSubscription
	cancelCalls  int
}

type supervisorActiveCallController struct {
	supervisor *application.RunSupervisor
}

func (c *supervisorActiveCallController) ActiveCallForSession(sessionID string) (application.ActiveCallInfo, bool) {
	return c.supervisor.ActiveCallForSession(sessionID)
}

func (c *supervisorActiveCallController) SubscribeActiveCall(runID string) (ActiveCallSubscription, error) {
	return c.supervisor.SubscribeActiveCall(runID)
}

func (c *supervisorActiveCallController) CancelActiveCall(ctx context.Context, request application.ActiveCallCancelRequest) (application.ActiveCallCancelResult, error) {
	return c.supervisor.CancelActiveCall(ctx, request)
}

type tuiBlockingProvider struct {
	entered      chan struct{}
	releaseChunk chan struct{}
	once         sync.Once
}

func newTUIBlockingProvider() *tuiBlockingProvider {
	return &tuiBlockingProvider{entered: make(chan struct{}), releaseChunk: make(chan struct{})}
}

func (*tuiBlockingProvider) Name() string { return "tui-active-test" }

func (p *tuiBlockingProvider) ListModels(context.Context) ([]llm.ModelInfo, error) {
	return []llm.ModelInfo{{ID: "model", Provider: p.Name()}}, nil
}

func (*tuiBlockingProvider) Chat(context.Context, llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, apperror.New(apperror.CodeInternal, "TUI active-call test requires streaming")
}

func (p *tuiBlockingProvider) StreamChat(ctx context.Context, _ llm.ChatRequest) (<-chan llm.ChatChunk, error) {
	chunks := make(chan llm.ChatChunk, 1)
	p.once.Do(func() { close(p.entered) })
	go func() {
		defer close(chunks)
		select {
		case <-p.releaseChunk:
		case <-ctx.Done():
			return
		}
		select {
		case chunks <- llm.ChatChunk{Text: "partial TUI stream"}:
		case <-ctx.Done():
			return
		}
		<-ctx.Done()
	}()
	return chunks, nil
}

func (*tuiBlockingProvider) SupportsTools(string) bool    { return false }
func (*tuiBlockingProvider) SupportsVision(string) bool   { return false }
func (*tuiBlockingProvider) SupportsJSONMode(string) bool { return true }

func (c *fakeActiveCallController) ActiveCallForSession(sessionID string) (application.ActiveCallInfo, bool) {
	return c.info, c.active && sessionID == c.info.SessionID
}

func (c *fakeActiveCallController) SubscribeActiveCall(runID string) (ActiveCallSubscription, error) {
	if !c.active || runID != c.info.RunID {
		return nil, context.Canceled
	}
	return c.subscription, nil
}

func (c *fakeActiveCallController) CancelActiveCall(context.Context, application.ActiveCallCancelRequest) (application.ActiveCallCancelResult, error) {
	c.cancelCalls++
	info := c.info
	info.CancelRequested = true
	c.info = info
	return application.ActiveCallCancelResult{
		Found: true, AuditRecorded: true, Signaled: true, Call: info,
	}, nil
}

type fakeActiveCallSubscription struct {
	events  chan application.ActiveCallEvent
	dropped atomic.Bool
	once    sync.Once
}

func (s *fakeActiveCallSubscription) Events() <-chan application.ActiveCallEvent { return s.events }
func (s *fakeActiveCallSubscription) Dropped() bool                              { return s.dropped.Load() }
func (s *fakeActiveCallSubscription) Close() {
	s.once.Do(func() { close(s.events) })
}

func newActiveCallTestModel(t *testing.T) (*Model, *fakeActiveCallController) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolrun.NewManager(st, policy.NewDefaultChecker())
	sess, err := sessionManager.Create(context.Background(), "", "live", "learn")
	if err != nil {
		t.Fatal(err)
	}
	model, err := NewModel(context.Background(), sess, sessionManager, toolManager)
	if err != nil {
		t.Fatal(err)
	}
	info := application.ActiveCallInfo{
		RunID: "run-live", SessionID: sess.ID, AttemptID: "attempt-live",
		ModelAttempt: 1, TransportAttempt: 1, MaxAttempts: 3,
		Provider: "active-test", Model: "model", StartedAt: time.Now().UTC(),
	}
	subscription := &fakeActiveCallSubscription{events: make(chan application.ActiveCallEvent, 8)}
	subscription.events <- application.ActiveCallEvent{
		Version: application.ActiveCallEnvelopeVersion, Sequence: 1,
		Type: application.ActiveCallSnapshotEvent, Call: info, CreatedAt: time.Now().UTC(),
	}
	controller := &fakeActiveCallController{info: info, active: true, subscription: subscription}
	return model.WithActiveCallController(controller), controller
}
