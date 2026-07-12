package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
)

func TestSpecialistScheduleSummaryFinishesAndReplaysExactlyOnce(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st,
		"Specialist schedule summary", 2, 64)
	usage, err := st.GetRunAgentUsage(ctx, fixture.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	usage.ReadOnlyFanoutTokens = 17
	usage.TotalTokens += usage.ReadOnlyFanoutTokens
	usage.ReadOnlyFanoutMillis = 23
	usage.TotalExecutionMillis += usage.ReadOnlyFanoutMillis
	started, err := st.StartSpecialistSchedule(ctx, domain.SpecialistScheduleStart{
		ID: "schedule-summary-0001", RunID: fixture.Run.ID,
		AgentIDs: []string{fixture.Child.ID}, MaxRounds: 2,
		Lease: fixture.Lease, UsageBefore: usage, StartedAt: time.Now().UTC(),
	})
	if err != nil || started.RecoveredSchedule ||
		started.Schedule.Status != domain.SpecialistScheduleRunning {
		t.Fatalf("Specialist schedule did not start: result=%#v err=%v", started, err)
	}
	finish := domain.SpecialistScheduleFinish{
		ID: started.Schedule.ID, Lease: fixture.Lease,
		Status: domain.SpecialistScheduleCompleted, StopReason: "round_limit",
		RoundsCompleted: 2, TurnsStarted: 1, RecoveredAttempts: 0,
		UsageAfter: usage, FinishedAt: time.Now().UTC(),
	}
	completed, err := st.FinishSpecialistSchedule(ctx, finish)
	if err != nil || completed.Status != domain.SpecialistScheduleCompleted ||
		completed.RoundsCompleted != 2 || completed.TurnsStarted != 1 ||
		completed.FinishedAt == nil || completed.UsageBefore.ReadOnlyFanoutTokens != 17 ||
		completed.UsageAfter.ReadOnlyFanoutMillis != 23 {
		t.Fatalf("Specialist schedule did not finish: schedule=%#v err=%v", completed, err)
	}
	replayed, err := st.FinishSpecialistSchedule(ctx, finish)
	if err != nil || replayed.ID != completed.ID || replayed.Status != completed.Status {
		t.Fatalf("Specialist schedule finish replay drifted: schedule=%#v err=%v", replayed, err)
	}
	changed := finish
	changed.TurnsStarted++
	if _, err := st.FinishSpecialistSchedule(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("changed schedule finish replay was accepted: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE specialist_schedules SET stop_reason = 'changed'
		WHERE id = ?`, completed.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("terminal Specialist schedule was mutable: %v", err)
	}
	eventLog, err := st.ListRunEvents(ctx, fixture.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countRunEventType(eventLog, events.AgentScheduleStartedEvent) != 1 ||
		countRunEventType(eventLog, events.AgentScheduleStoppedEvent) != 1 {
		t.Fatalf("Specialist schedule audit lifecycle is incomplete: %#v", eventLog)
	}
	for _, event := range eventLog {
		if strings.Contains(event.PayloadJSON, fixture.Lease.LeaseID) ||
			strings.Contains(event.PayloadJSON, `"lease_generation"`) {
			t.Fatalf("Specialist schedule event exposed fencing data: %s", event.PayloadJSON)
		}
	}
}
