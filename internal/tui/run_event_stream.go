package tui

import (
	"context"
	"errors"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"cyberagent-workbench/internal/events"
)

const (
	tuiEventPollInterval      = 500 * time.Millisecond
	tuiEventPollRetryInterval = time.Second
	tuiEventPollTimeout       = 5 * time.Second
	tuiEventPollBatchSize     = 32
)

type runEventPollTickMsg struct {
	runID         string
	missionID     string
	afterSequence int64
}

type runEventPollResultMsg struct {
	tick     runEventPollTickMsg
	result   actionResult
	events   []events.Event
	changed  bool
	terminal bool
	err      error
}

func (m *Model) scheduleRunEventPoll(delay time.Duration) tea.Cmd {
	if m == nil || m.runStateStore == nil || !m.runContext.Found ||
		m.runContext.Run.Terminal() {
		return nil
	}
	tick := runEventPollTickMsg{runID: m.runContext.Run.ID,
		missionID: m.runContext.Run.MissionID, afterSequence: m.runContext.EventSequence}
	if delay <= 0 {
		return func() tea.Msg { return tick }
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return tick })
}

func (m *Model) pollRunEventsCmd(tick runEventPollTickMsg) tea.Cmd {
	if !m.matchesEventPollTick(tick) {
		return func() tea.Msg { return runEventPollResultMsg{tick: tick} }
	}
	sess := m.session
	sessionManager := m.sessionManager
	toolManager := m.toolManager
	workspaceStore := m.workspaceStore
	runStateStore := m.runStateStore
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), tuiEventPollTimeout)
		defer cancel()
		run, found, err := runStateStore.GetRunBySession(ctx, sess.ID)
		if err != nil {
			return runEventPollResultMsg{tick: tick, err: err}
		}
		if !found || run.ID != tick.runID || run.MissionID != tick.missionID {
			return runEventPollResultMsg{tick: tick,
				err: errors.New("TUI Run event stream changed scope unexpectedly")}
		}
		batch, err := runStateStore.ListRunEventsAfterSequence(ctx, run.ID,
			tick.afterSequence, tuiEventPollBatchSize)
		if err != nil {
			return runEventPollResultMsg{tick: tick, err: err}
		}
		if err := validateTUIEventBatch(run, tick.afterSequence, batch); err != nil {
			return runEventPollResultMsg{tick: tick, err: err}
		}
		if len(batch) == 0 {
			return runEventPollResultMsg{tick: tick, terminal: run.Terminal()}
		}
		result, err := loadActionResult(ctx, sess, sessionManager, toolManager,
			workspaceStore, runStateStore, "", "")
		if err != nil {
			return runEventPollResultMsg{tick: tick, err: err}
		}
		if !result.runContext.Found || result.runContext.Run.ID != tick.runID ||
			result.runContext.Run.MissionID != tick.missionID {
			return runEventPollResultMsg{tick: tick,
				err: errors.New("TUI refreshed projection changed event scope")}
		}
		return runEventPollResultMsg{tick: tick, result: result, events: batch,
			changed: true, terminal: result.runContext.Run.Terminal()}
	}
}

func (m *Model) handleRunEventPollResult(msg runEventPollResultMsg) (tea.Model, tea.Cmd) {
	if !m.matchesEventPollTick(msg.tick) {
		return m, m.scheduleRunEventPoll(tuiEventPollInterval)
	}
	if msg.err != nil {
		m.eventPollError = true
		return m, m.scheduleRunEventPoll(tuiEventPollRetryInterval)
	}
	m.eventPollError = false
	if msg.changed {
		status := m.status
		m.applyActionResult(msg.result)
		m.status = status
	}
	if msg.terminal || m.runContext.Run.Terminal() {
		return m, nil
	}
	return m, m.scheduleRunEventPoll(tuiEventPollInterval)
}

func (m *Model) matchesEventPollTick(tick runEventPollTickMsg) bool {
	return m != nil && m.runStateStore != nil && m.runContext.Found &&
		m.runContext.Run.ID == tick.runID && m.runContext.Run.MissionID == tick.missionID &&
		m.runContext.EventSequence == tick.afterSequence
}
