package store

import (
	"context"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
)

func TestRunAgentUsageReconcilesRootAndSpecialistLedgers(t *testing.T) {
	st := openWorkItemTestStore(t)
	ctx := context.Background()
	fixture := prepareSpecialistAttemptFixture(t, ctx, st,
		"aggregate Agent usage", 3, 64)
	attempt, _, err := st.BeginSpecialistAttempt(ctx,
		newAttemptStart(fixture, idgen.New("attempt")), "usage-reconcile-start-0001")
	if err != nil {
		t.Fatal(err)
	}
	usage := domain.AgentAttemptUsage{
		InputTokens: 4, OutputTokens: 3, TotalTokens: 7, ExecutionMillis: 11,
	}
	if _, _, err := st.RecordSpecialistAttemptUsage(ctx, attemptRef(attempt), usage,
		"usage-reconcile-charge-0001"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ContinueSpecialistAttempt(ctx, attemptRef(attempt),
		"usage-reconcile-continue-0001"); err != nil {
		t.Fatal(err)
	}

	aggregate, err := st.GetRunAgentUsage(ctx, fixture.Run.ID)
	if err != nil || aggregate.RootTokens != 0 || aggregate.SpecialistTokens != 7 ||
		aggregate.TotalTokens != 7 || aggregate.TotalExecutionMillis != 0 {
		t.Fatalf("aggregate usage is invalid: usage=%#v err=%v", aggregate, err)
	}

	if _, err := st.db.ExecContext(ctx, `UPDATE agent_nodes SET tokens_used = 1
		WHERE id = ?`, fixture.Root.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetRunAgentUsage(ctx, fixture.Run.ID); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("root projection drift was accepted: code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE agent_nodes SET tokens_used = 0
		WHERE id = ?`, fixture.Root.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE agent_nodes SET tokens_used = tokens_used + 1
		WHERE id = ?`, fixture.Child.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetRunAgentUsage(ctx, fixture.Run.ID); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("Specialist projection drift was accepted: code=%s err=%v",
			apperror.CodeOf(err), err)
	}
}
