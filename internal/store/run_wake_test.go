package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/runmutation"
)

func TestRunWakeIntentFencesOwnerRetriesAndCancellation(t *testing.T) {
	ctx := context.Background()
	state := openWorkItemTestStore(t)
	run := createEligibleRunWakeTestRun(t, ctx, state)
	now := time.Now().UTC()
	intent, operation := newRunWakeTestSchedule(run, now, 3)
	created, storedOperation, replayed, err := state.CreateRunWakeIntent(ctx,
		intent, operation)
	if err != nil || replayed || storedOperation.EventSequence <= 0 ||
		created.Status != domain.RunWakeQueued {
		t.Fatalf("schedule result=%#v operation=%#v replayed=%t err=%v",
			created, storedOperation, replayed, err)
	}
	if replayIntent, replayOperation, replayed, err := state.CreateRunWakeIntent(ctx,
		intent, operation); err != nil || !replayed || replayIntent.ID != created.ID ||
		replayOperation.EventSequence != storedOperation.EventSequence {
		t.Fatalf("schedule replay drifted: intent=%#v operation=%#v replayed=%t err=%v",
			replayIntent, replayOperation, replayed, err)
	}
	if _, _, acquired, err := state.AcquireRunWake(ctx, intent.ID, "owner-early",
		"wake-lease-early", intent.NextWakeAt.Add(-time.Millisecond)); err != nil || acquired {
		t.Fatalf("wake acquired before due time: acquired=%t err=%v", acquired, err)
	}
	leased, firstLease, acquired, err := state.AcquireRunWake(ctx, intent.ID,
		"owner-first", "wake-lease-first", intent.NextWakeAt)
	if err != nil || !acquired || leased.Status != domain.RunWakeLeased ||
		firstLease.Generation != 1 || leased.ActiveLeaseID != firstLease.ID {
		t.Fatalf("first acquisition failed: intent=%#v lease=%#v acquired=%t err=%v",
			leased, firstLease, acquired, err)
	}
	if _, _, acquired, err := state.AcquireRunWake(ctx, intent.ID, "owner-second",
		"wake-lease-second", intent.NextWakeAt); err != nil || acquired {
		t.Fatalf("second owner acquired active intent: acquired=%t err=%v", acquired, err)
	}
	retryAt := firstLease.AcquiredAt.Add(time.Second)
	queued, err := state.ReleaseRunWakeForRetry(ctx, firstLease, retryAt)
	if err != nil || queued.Status != domain.RunWakeQueued || queued.AttemptCount != 1 ||
		!queued.NextWakeAt.Equal(retryAt.Add(5*time.Second)) {
		t.Fatalf("retry release failed: intent=%#v err=%v", queued, err)
	}
	leased, secondLease, acquired, err := state.AcquireRunWake(ctx, intent.ID,
		"owner-second", "wake-lease-second", queued.NextWakeAt)
	if err != nil || !acquired || leased.AttemptCount != 2 || secondLease.Generation != 2 {
		t.Fatalf("second acquisition failed: intent=%#v lease=%#v acquired=%t err=%v",
			leased, secondLease, acquired, err)
	}
	cancelAt := secondLease.AcquiredAt.Add(time.Second)
	cancelOperation := domain.RunWakeOperation{
		ProtocolVersion:    domain.RunWakeControlProtocolVersion,
		KeyDigest:          runmutation.RunWakeOperationDigest(run.ID, "run-wake-cancel-test-0001"),
		RequestFingerprint: runmutation.RunWakeCancelRequestFingerprint(run.ID, "operator"),
		Action:             domain.RunWakeCancel, RunID: run.ID, RequestedBy: "operator",
		CreatedAt: cancelAt,
	}
	cancelled, _, replayed, err := state.CancelRunWakeIntent(ctx, run.ID,
		cancelAt, cancelOperation)
	if err != nil || replayed || cancelled.Status != domain.RunWakeCancelled ||
		cancelled.ActiveLeaseID != "" || cancelled.CancelledAt == nil {
		t.Fatalf("wake cancellation failed: intent=%#v replayed=%t err=%v",
			cancelled, replayed, err)
	}
	if _, err := state.ReleaseRunWakeForRetry(ctx, secondLease,
		cancelAt.Add(time.Millisecond)); err == nil {
		t.Fatal("revoked owner released a cancelled wake intent")
	}
}

func TestRunWakeIntentAllowsOnlyOneConcurrentOwner(t *testing.T) {
	ctx := context.Background()
	state := openWorkItemTestStore(t)
	run := createEligibleRunWakeTestRun(t, ctx, state)
	now := time.Now().UTC()
	intent, operation := newRunWakeTestSchedule(run, now, 2)
	if _, _, _, err := state.CreateRunWakeIntent(ctx, intent, operation); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan bool, 2)
	errorsSeen := make(chan error, 2)
	var workers sync.WaitGroup
	for index := 0; index < 2; index++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			<-start
			_, _, acquired, err := state.AcquireRunWake(ctx, intent.ID,
				idgen.New("wake-owner"), idgen.New("wake-lease"), intent.NextWakeAt)
			results <- acquired
			errorsSeen <- err
		}(index)
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsSeen)
	acquiredCount := 0
	for acquired := range results {
		if acquired {
			acquiredCount++
		}
	}
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	if acquiredCount != 1 {
		t.Fatalf("concurrent ownership winners=%d, want 1", acquiredCount)
	}
}

func TestRunWakeIntentExhaustsTotalAttemptBudgetAndIsImmutable(t *testing.T) {
	ctx := context.Background()
	state := openWorkItemTestStore(t)
	run := createEligibleRunWakeTestRun(t, ctx, state)
	now := time.Now().UTC()
	intent, operation := newRunWakeTestSchedule(run, now, 1)
	if _, _, _, err := state.CreateRunWakeIntent(ctx, intent, operation); err != nil {
		t.Fatal(err)
	}
	_, lease, acquired, err := state.AcquireRunWake(ctx, intent.ID,
		"owner-budget", "wake-lease-budget", intent.NextWakeAt)
	if err != nil || !acquired {
		t.Fatalf("budget acquisition failed: acquired=%t err=%v", acquired, err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE run_wake_leases
		SET status = 'released', ended_at = ? WHERE id = ?`,
		ts(lease.AcquiredAt.Add(time.Second)), lease.ID); err == nil {
		t.Fatal("Run wake lease was released without a matching audit event")
	}
	exhausted, err := state.ReleaseRunWakeForRetry(ctx, lease,
		lease.AcquiredAt.Add(time.Second))
	if err != nil || exhausted.Status != domain.RunWakeExhausted ||
		exhausted.AttemptCount != exhausted.MaxAttempts {
		t.Fatalf("wake budget did not exhaust: intent=%#v err=%v", exhausted, err)
	}
	if _, err := state.db.ExecContext(ctx, `DELETE FROM run_wake_intents WHERE id = ?`,
		intent.ID); err == nil {
		t.Fatal("Run wake intent deletion was accepted")
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE run_wake_operations
		SET requested_by = 'other' WHERE operation_key_digest = ?`,
		operation.KeyDigest); err == nil {
		t.Fatal("Run wake operation mutation was accepted")
	}
}

func TestRunWakeExpiredOwnerIsFencedByNextGeneration(t *testing.T) {
	ctx := context.Background()
	state := openWorkItemTestStore(t)
	run := createEligibleRunWakeTestRun(t, ctx, state)
	now := time.Now().UTC()
	intent, operation := newRunWakeTestSchedule(run, now, 2)
	if _, _, _, err := state.CreateRunWakeIntent(ctx, intent, operation); err != nil {
		t.Fatal(err)
	}
	_, first, acquired, err := state.AcquireRunWake(ctx, intent.ID,
		"owner-expired", "wake-lease-expired", intent.NextWakeAt)
	if err != nil || !acquired {
		t.Fatalf("first acquisition failed: acquired=%t err=%v", acquired, err)
	}
	owned, second, acquired, err := state.AcquireRunWake(ctx, intent.ID,
		"owner-takeover", "wake-lease-takeover", first.ExpiresAt)
	if err != nil || !acquired || second.Generation != 2 ||
		owned.ActiveLeaseID != second.ID || owned.AttemptCount != 2 {
		t.Fatalf("expired takeover failed: intent=%#v lease=%#v acquired=%t err=%v",
			owned, second, acquired, err)
	}
	if _, err := state.ReleaseRunWakeForRetry(ctx, first,
		first.ExpiresAt); err == nil {
		t.Fatal("expired generation retained release authority")
	}
}

func TestRunWakeExpiredFinalGenerationCommitsExhaustionBeforeLeaseEnd(t *testing.T) {
	ctx := context.Background()
	state := openWorkItemTestStore(t)
	run := createEligibleRunWakeTestRun(t, ctx, state)
	now := time.Now().UTC()
	intent, operation := newRunWakeTestSchedule(run, now, 1)
	if _, _, _, err := state.CreateRunWakeIntent(ctx, intent, operation); err != nil {
		t.Fatal(err)
	}
	_, first, acquired, err := state.AcquireRunWake(ctx, intent.ID,
		"owner-final-expired", "wake-lease-final-expired", intent.NextWakeAt)
	if err != nil || !acquired {
		t.Fatalf("final generation acquisition failed: acquired=%t err=%v", acquired, err)
	}
	exhausted, replacement, acquired, err := state.AcquireRunWake(ctx, intent.ID,
		"owner-after-exhaustion", "wake-lease-after-exhaustion", first.ExpiresAt)
	if err != nil || acquired || replacement.ID != "" ||
		exhausted.Status != domain.RunWakeExhausted || exhausted.ActiveLeaseID != "" ||
		exhausted.AttemptCount != exhausted.MaxAttempts {
		t.Fatalf("expired final generation was not exhausted atomically: intent=%#v lease=%#v acquired=%t err=%v",
			exhausted, replacement, acquired, err)
	}
	var leaseStatus string
	if err := state.db.QueryRowContext(ctx,
		`SELECT status FROM run_wake_leases WHERE id = ?`, first.ID).Scan(&leaseStatus); err != nil {
		t.Fatal(err)
	}
	if leaseStatus != string(domain.RunWakeLeaseExpired) {
		t.Fatalf("expired final lease status=%q", leaseStatus)
	}
}

func TestSchemaV74UpgradeDoesNotFabricateWakeIntent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v73.db")
	state, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, run := createWorkItemTestRun(t, ctx, state, "v74 upgrade test")
	for _, statement := range removeSchemaV74ForTestStatements() {
		if _, err := state.db.ExecContext(ctx, statement); err != nil {
			_ = state.Close()
			t.Fatalf("remove schema v74 with %q: %v", statement, err)
		}
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if version, err := upgraded.SchemaVersion(ctx); err != nil ||
		version != LatestSchemaVersion {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	if intent, found, err := upgraded.GetLatestRunWakeIntent(ctx, run.ID); err != nil || found {
		t.Fatalf("v74 fabricated wake intent=%#v found=%t err=%v", intent, found, err)
	}
}

func createEligibleRunWakeTestRun(t *testing.T, ctx context.Context,
	state *SQLiteStore,
) domain.Run {
	t.Helper()
	_, created := createWorkItemTestRun(t, ctx, state, "Run wake ownership test")
	run, err := application.NewRunService(state).Start(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.EnqueueOperatorSteering(ctx, domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "queued wake instruction",
		OperationKey: idgen.New("wake-steering-operation"), RequestedBy: "operator",
	}); err != nil {
		t.Fatal(err)
	}
	return run
}

func newRunWakeTestSchedule(run domain.Run, now time.Time,
	maxAttempts int,
) (domain.RunWakeIntent, domain.RunWakeOperation) {
	intent := domain.RunWakeIntent{
		ID: idgen.New("wake"), ProtocolVersion: domain.RunWakeIntentProtocolVersion,
		RunID: run.ID, SessionID: run.SessionID, Status: domain.RunWakeQueued,
		MaxAttempts: maxAttempts, InitialDelaySeconds: 1,
		BaseBackoffSeconds: 5, MaxBackoffSeconds: 20, MaxElapsedSeconds: 120,
		NextWakeAt: now.Add(time.Second), DeadlineAt: now.Add(120 * time.Second),
		CreatedAt: now, UpdatedAt: now,
	}
	operationKey := idgen.New("wake-schedule-operation")
	operation := domain.RunWakeOperation{
		ProtocolVersion: domain.RunWakeControlProtocolVersion,
		KeyDigest:       runmutation.RunWakeOperationDigest(run.ID, operationKey),
		RequestFingerprint: runmutation.RunWakeScheduleRequestFingerprint(run.ID,
			"operator", maxAttempts, 1, 5, 20, 120),
		Action: domain.RunWakeSchedule, IntentID: intent.ID, RunID: run.ID,
		RequestedBy: "operator", CreatedAt: now,
	}
	return intent, operation
}
