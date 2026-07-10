package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/llm"
)

const (
	activeCallDiscoveryInterval = 20 * time.Millisecond
	activeCallCancelTimeout     = 750 * time.Millisecond
	activeCallAuditTimeout      = 2 * time.Second
)

type ActiveCallSubscription interface {
	Events() <-chan application.ActiveCallEvent
	Dropped() bool
	Close()
}

type ActiveCallController interface {
	ActiveCallForSession(sessionID string) (application.ActiveCallInfo, bool)
	SubscribeActiveCall(runID string) (ActiveCallSubscription, error)
	CancelActiveCall(ctx context.Context, request application.ActiveCallCancelRequest) (application.ActiveCallCancelResult, error)
}

type activeCallView struct {
	Info      application.ActiveCallInfo
	State     string
	Outcome   llm.Outcome
	Sequence  int64
	Connected bool
	Dropped   bool
}

func (v activeCallView) summary() string {
	if v.State == "" {
		return ""
	}
	if strings.TrimSpace(v.Info.RunID) == "" {
		return "live=" + v.State
	}
	state := v.State
	if v.Info.CancelRequested && state != "cancelled" && state != "failed" {
		state = "cancelling"
	}
	return fmt.Sprintf("live=%s model=%s/%s a=%d chunks=%d bytes=%d",
		state, truncate(v.Info.Provider, 12), truncate(v.Info.Model, 16),
		v.Info.ModelAttempt, v.Info.StreamChunks, v.Info.StreamBytes)
}

func (v activeCallView) terminal() bool {
	return v.State == "completed" || v.State == "failed" || v.State == "cancelled"
}

type activeCallSubscribedMsg struct {
	generation   uint64
	subscription ActiveCallSubscription
	notFound     bool
	err          error
}

type activeCallEventMsg struct {
	generation   uint64
	subscription ActiveCallSubscription
	event        application.ActiveCallEvent
	open         bool
}

type activeCallCancelDoneMsg struct {
	generation uint64
	result     application.ActiveCallCancelResult
	err        error
}

func (m *Model) WithActiveCallController(controller ActiveCallController) *Model {
	if m != nil {
		m.activeCalls = controller
	}
	return m
}

func (m *Model) startSubmitAction(status string, input string) (tea.Model, tea.Cmd) {
	if m.activeCalls == nil {
		return m.startAction(status, m.submitCmd(input))
	}
	m.closeLiveSubscription()
	if m.liveDiscoverCancel != nil {
		m.liveDiscoverCancel()
	}
	m.liveGeneration++
	generation := m.liveGeneration
	discoveryCtx, cancel := context.WithCancel(context.Background())
	m.liveDiscoverCancel = cancel
	actionCtx, actionCancel := context.WithCancel(context.Background())
	m.actionCancel = actionCancel
	m.live = activeCallView{State: "discovering"}
	m.busy = true
	m.status = status
	return m, tea.Batch(m.submitCmdContext(actionCtx, input), m.discoverActiveCallCmd(discoveryCtx, generation, m.session.ID))
}

func (m *Model) discoverActiveCallCmd(ctx context.Context, generation uint64, sessionID string) tea.Cmd {
	controller := m.activeCalls
	return func() tea.Msg {
		if controller == nil {
			return activeCallSubscribedMsg{generation: generation, notFound: true}
		}
		ticker := time.NewTicker(activeCallDiscoveryInterval)
		defer ticker.Stop()
		for {
			if info, ok := controller.ActiveCallForSession(sessionID); ok {
				subscription, err := controller.SubscribeActiveCall(info.RunID)
				if err == nil {
					return activeCallSubscribedMsg{generation: generation, subscription: subscription}
				}
				if apperror.CodeOf(apperror.Normalize(err)) != apperror.CodeNotFound {
					return activeCallSubscribedMsg{generation: generation, err: err}
				}
			}
			select {
			case <-ctx.Done():
				return activeCallSubscribedMsg{generation: generation, notFound: true}
			case <-ticker.C:
			}
		}
	}
}

func waitActiveCallEventCmd(generation uint64, subscription ActiveCallSubscription) tea.Cmd {
	return func() tea.Msg {
		if subscription == nil {
			return activeCallEventMsg{generation: generation, open: false}
		}
		event, open := <-subscription.Events()
		return activeCallEventMsg{
			generation: generation, subscription: subscription, event: event, open: open,
		}
	}
}

func (m *Model) handleActiveCallSubscribed(msg activeCallSubscribedMsg) tea.Cmd {
	if msg.generation != m.liveGeneration || !m.busy {
		if msg.subscription != nil {
			msg.subscription.Close()
		}
		return nil
	}
	if msg.err != nil {
		m.live = activeCallView{State: "subscription-error"}
		m.status = "live status error: " + msg.err.Error()
		return nil
	}
	if msg.notFound || msg.subscription == nil {
		m.live = activeCallView{}
		return nil
	}
	m.liveSubscription = msg.subscription
	m.live = activeCallView{State: "connecting", Connected: true}
	return waitActiveCallEventCmd(msg.generation, msg.subscription)
}

func (m *Model) handleActiveCallEvent(msg activeCallEventMsg) tea.Cmd {
	if msg.generation != m.liveGeneration || msg.subscription != m.liveSubscription {
		if msg.subscription != nil {
			msg.subscription.Close()
		}
		return nil
	}
	if !msg.open {
		dropped := msg.subscription != nil && msg.subscription.Dropped()
		m.liveSubscription = nil
		m.live.Connected = false
		m.live.Dropped = dropped
		if dropped {
			m.live.State = "disconnected"
			m.status = "live status disconnected: slow consumer"
		} else if !m.live.terminal() {
			m.live.State = "closed"
		}
		return nil
	}
	if err := msg.event.Validate(); err != nil {
		msg.subscription.Close()
		m.liveSubscription = nil
		m.live = activeCallView{State: "invalid", Connected: false}
		m.status = "invalid live status: " + err.Error()
		return nil
	}
	m.live.Info = msg.event.Call
	m.live.Sequence = msg.event.Sequence
	m.live.Outcome = msg.event.Outcome
	m.live.Connected = true
	switch msg.event.Type {
	case application.ActiveCallSnapshotEvent, application.ActiveCallStartedEvent:
		m.live.State = "streaming"
	case application.ActiveCallProgressEvent:
		m.live.State = "streaming"
	case application.ActiveCallCancelRequestedEvent:
		m.live.State = "cancelling"
	case application.ActiveCallCompletedEvent:
		m.live.State = "completed"
	case application.ActiveCallFailedEvent:
		if msg.event.Outcome == llm.OutcomeCancelled {
			m.live.State = "cancelled"
		} else {
			m.live.State = "failed"
		}
	}
	return waitActiveCallEventCmd(msg.generation, msg.subscription)
}

func (m *Model) cancelActiveCallCmd(generation uint64, sessionID string) tea.Cmd {
	controller := m.activeCalls
	return func() tea.Msg {
		if controller == nil {
			return activeCallCancelDoneMsg{generation: generation}
		}
		ctx, cancel := context.WithTimeout(context.Background(), activeCallCancelTimeout)
		defer cancel()
		ticker := time.NewTicker(activeCallDiscoveryInterval)
		defer ticker.Stop()
		for {
			if info, ok := controller.ActiveCallForSession(sessionID); ok {
				auditCtx, auditCancel := context.WithTimeout(context.Background(), activeCallAuditTimeout)
				result, err := controller.CancelActiveCall(auditCtx, application.ActiveCallCancelRequest{
					RunID: info.RunID, Reason: "cancelled from TUI",
				})
				auditCancel()
				return activeCallCancelDoneMsg{generation: generation, result: result, err: err}
			}
			select {
			case <-ctx.Done():
				return activeCallCancelDoneMsg{generation: generation}
			case <-ticker.C:
			}
		}
	}
}

func (m *Model) handleActiveCallCancelDone(msg activeCallCancelDoneMsg) {
	if msg.generation != m.liveGeneration {
		return
	}
	m.cancelPending = false
	if msg.err != nil {
		m.status = "cancel failed: " + msg.err.Error()
		return
	}
	if !msg.result.Found {
		if m.actionCancel != nil {
			m.actionCancel()
			m.live.State = "cancelling"
			m.status = "current action cancellation requested"
			return
		}
		m.status = "no active model call"
		return
	}
	m.live.Info = msg.result.Call
	if msg.result.Signaled {
		m.live.State = "cancelling"
		m.status = "model cancellation requested"
		return
	}
	if msg.result.AlreadyRequested {
		m.live.State = "cancelling"
		m.status = "model cancellation already requested"
		return
	}
	m.status = "model call ended before cancellation signal"
}

func (m *Model) stopLiveTracking(actionErr error) {
	if m.liveDiscoverCancel != nil {
		m.liveDiscoverCancel()
		m.liveDiscoverCancel = nil
	}
	if m.actionCancel != nil {
		m.actionCancel()
		m.actionCancel = nil
	}
	m.closeLiveSubscription()
	m.cancelPending = false
	if strings.TrimSpace(m.live.Info.RunID) != "" && !m.live.terminal() {
		m.live.Connected = false
		if actionErr == nil {
			m.live.State = "completed"
			m.live.Outcome = llm.OutcomeSuccess
		} else if apperror.CodeOf(apperror.Normalize(actionErr)) == apperror.CodeCancelled {
			m.live.State = "cancelled"
			m.live.Outcome = llm.OutcomeCancelled
		} else {
			m.live.State = "failed"
			m.live.Outcome = llm.OutcomePermanent
		}
	}
	m.liveGeneration++
}

func (m *Model) closeLiveSubscription() {
	if m.liveSubscription != nil {
		m.liveSubscription.Close()
		m.liveSubscription = nil
	}
}
