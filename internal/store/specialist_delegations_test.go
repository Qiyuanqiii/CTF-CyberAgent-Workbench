package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/toolgateway"
)

func TestSpecialistDelegationConcurrentReplayConvergesWithoutSpawning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delegation-concurrency.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	other, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ctx := context.Background()
	run, turn, lease := beginDelegationTestTurn(t, ctx, st, "concurrent delegation")
	gateways := []*toolgateway.Gateway{
		newDelegationTestGateway(st), newDelegationTestGateway(other),
	}
	payload := delegationTestPayload(t, domain.SpecialistDelegationSpec{
		Version: domain.SpecialistDelegationVersion,
		Assignments: []domain.SpecialistDelegationAssignment{{
			Title: "Parser review", Goal: "Review parser boundaries",
			Skills: []string{"model.chat"}, TurnLimit: 2, TokenLimit: 128,
		}},
	})
	call := toolgateway.ToolCall{
		Name: toolgateway.SpecialistDelegationProposeTool, Payload: payload,
		OperationKey: "concurrent-specialist-delegation", RunID: run.ID,
		AgentID: turn.Agent.ID, SessionID: run.SessionID, WorkspaceID: "ws-delegation",
		RequestedBy: "run_supervisor", LeaseID: lease.LeaseID,
		LeaseGeneration: lease.Generation,
	}
	const workers = 8
	type invocationResult struct {
		outcome toolgateway.Outcome
		err     error
	}
	results := make(chan invocationResult, workers)
	var wg sync.WaitGroup
	for index := range workers {
		wg.Add(1)
		go func(gateway *toolgateway.Gateway) {
			defer wg.Done()
			outcome, err := gateway.Invoke(ctx, call)
			results <- invocationResult{outcome: outcome, err: err}
		}(gateways[index%len(gateways)])
	}
	wg.Wait()
	close(results)
	proposalIDs := map[string]bool{}
	created := 0
	for result := range results {
		if result.err != nil || result.outcome.Result == nil {
			t.Fatalf("concurrent delegation failed: %#v err=%v", result.outcome, result.err)
		}
		proposalIDs[result.outcome.Result.Metadata["proposal_id"]] = true
		if result.outcome.Result.Metadata["replayed"] == "false" {
			created++
		}
		if result.outcome.Result.Metadata["admission_authorized"] != "false" {
			t.Fatalf("proposal claimed admission: %#v", result.outcome.Result.Metadata)
		}
	}
	if len(proposalIDs) != 1 || created != 1 {
		t.Fatalf("concurrent delegation did not converge: ids=%#v created=%d", proposalIDs, created)
	}
	proposals, err := st.ListSpecialistDelegationProposals(ctx, run.ID, 10)
	if err != nil || len(proposals) != 1 {
		t.Fatalf("delegation ledger contains duplicates: %#v err=%v", proposals, err)
	}
	nodes, err := st.ListAgentNodes(ctx, run.ID)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("delegation proposal spawned children: %#v err=%v", nodes, err)
	}
	timeline, err := st.ListRunEvents(ctx, run.ID)
	if err != nil || countRunEventType(timeline, events.AgentDelegationProposedEvent) != 1 ||
		countRunEventType(timeline, events.ToolCompletedEvent) != 1 ||
		countRunEventType(timeline, events.ToolBudgetChargedEvent) != workers {
		t.Fatalf("concurrent delegation audit stream is inconsistent: %#v err=%v", timeline, err)
	}
	var rawOperationKeys int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM specialist_delegation_operations
		WHERE operation_key_digest = ? OR request_fingerprint = ?`, call.OperationKey,
		call.OperationKey).Scan(&rawOperationKeys); err != nil {
		t.Fatal(err)
	}
	if rawOperationKeys != 0 {
		t.Fatal("raw Specialist delegation operation key was persisted")
	}
	var leaseColumns int
	if err := st.db.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM pragma_table_info('specialist_delegation_operations')
		WHERE name IN ('lease_id', 'lease_generation')`).Scan(&leaseColumns); err != nil {
		t.Fatal(err)
	}
	if leaseColumns != 0 {
		t.Fatal("Specialist delegation operation ledger persisted lease identity columns")
	}
}

func TestSpecialistDelegationRejectsSkillEscalationAndDirectMutation(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "delegation-policy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	run, turn, lease := beginDelegationTestTurn(t, ctx, st, "delegation escalation")
	call := toolgateway.ToolCall{
		Name: toolgateway.SpecialistDelegationProposeTool,
		Payload: delegationTestPayload(t, domain.SpecialistDelegationSpec{
			Version: domain.SpecialistDelegationVersion,
			Assignments: []domain.SpecialistDelegationAssignment{{
				Title: "Escalate", Goal: "Request an unavailable capability",
				Skills: []string{"shell"}, TurnLimit: 1, TokenLimit: 32,
			}},
		}),
		OperationKey: "delegation-skill-escalation", RunID: run.ID,
		AgentID: turn.Agent.ID, SessionID: run.SessionID, WorkspaceID: "ws-delegation",
		RequestedBy: "run_supervisor", LeaseID: lease.LeaseID,
		LeaseGeneration: lease.Generation,
	}
	if _, err := newDelegationTestGateway(st).Invoke(ctx, call); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("Skill escalation was not rejected: code=%s err=%v", apperror.CodeOf(err), err)
	}
	proposals, err := st.ListSpecialistDelegationProposals(ctx, run.ID, 10)
	if err != nil || len(proposals) != 0 {
		t.Fatalf("rejected Skill escalation persisted a proposal: %#v err=%v", proposals, err)
	}

	valid := call
	valid.OperationKey = "delegation-valid-mutation"
	valid.Payload = delegationTestPayload(t, domain.SpecialistDelegationSpec{
		Version: domain.SpecialistDelegationVersion,
		Assignments: []domain.SpecialistDelegationAssignment{{
			Title: "Allowed", Goal: "Review code", Skills: []string{"model.chat"},
			TurnLimit: 1, TokenLimit: 32,
		}},
	})
	outcome, err := newDelegationTestGateway(st).Invoke(ctx, valid)
	if err != nil || outcome.Result == nil {
		t.Fatalf("valid delegation failed: %#v err=%v", outcome, err)
	}
	proposalID := outcome.Result.Metadata["proposal_id"]
	if _, err := st.db.ExecContext(ctx, `UPDATE specialist_delegation_proposals
		SET status = 'approved' WHERE id = ?`, proposalID); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("direct proposal mutation bypassed review boundary: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE specialist_delegation_assignments
		SET turn_limit = 99 WHERE proposal_id = ?`, proposalID); err == nil ||
		!strings.Contains(err.Error(), "immutable") {
		t.Fatalf("direct assignment mutation bypassed immutability: %v", err)
	}
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	malformedID := "delegation-malformed-skills"
	if _, err := tx.ExecContext(ctx, `INSERT INTO specialist_delegation_proposals
		(id, run_id, root_agent_id, session_id, workspace_id, protocol_version, status,
		assignment_count, requested_by, version, created_at)
		VALUES (?, ?, ?, ?, ?, 'specialist_delegation.v1', 'proposed', 1,
		'run_supervisor', 1, ?)`, malformedID, run.ID, turn.Agent.ID, run.SessionID,
		"ws-delegation", ts(time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO specialist_delegation_assignments
		(proposal_id, ordinal, title, goal, skills_json, turn_limit, token_limit)
		VALUES (?, 1, 'Malformed', 'Duplicate Skills', '["model.chat","model.chat"]', 1, 32)`,
		malformedID); err == nil || !strings.Contains(err.Error(), "exceeds root capability") {
		t.Fatalf("direct malformed Skill JSON bypassed schema trigger: %v", err)
	}
}

func TestLegacyRootLazilyGainsDelegationProposalCapability(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "delegation-legacy-root.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: "legacy root capability", Profile: "code",
		Budget: domain.Budget{MaxTurns: 5, MaxTokens: 500, MaxToolCalls: 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, found, err := st.GetRootAgent(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("root missing: %#v found=%t err=%v", root, found, err)
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE agent_nodes SET
		skills_json = '["model.chat","note_create","profile.code","work_item_create"]',
		version = version + 1 WHERE id = ?`, root.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	acquisition, err := st.AcquireRunExecutionLease(ctx,
		domain.AcquireRunExecutionLeaseRequest{
			RunID: run.ID, OwnerID: "legacy-root-worker", TTL: time.Minute,
		})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, acquisition.Lease, "resume legacy root")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(turn.Agent.Skills, domain.AgentSkillSpecialistDelegation) ||
		turn.Agent.Version <= root.Version {
		t.Fatalf("legacy root capability was not upgraded: before=%#v after=%#v", root, turn.Agent)
	}
}

func beginDelegationTestTurn(t *testing.T, ctx context.Context, st *SQLiteStore,
	goal string,
) (domain.Run, domain.SupervisorTurn, domain.RunExecutionLease) {
	t.Helper()
	service := application.NewRunService(st)
	_, run, err := service.Create(ctx, application.CreateRunRequest{
		Goal: goal, Profile: "code", WorkspaceID: "ws-delegation",
		Budget: domain.Budget{MaxTurns: 8, MaxTokens: 2000, MaxToolCalls: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = service.Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.RegisterRootAgent(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	acquisition, err := st.AcquireRunExecutionLease(ctx,
		domain.AcquireRunExecutionLeaseRequest{
			RunID: run.ID, OwnerID: "delegation-test-worker", TTL: time.Minute,
		})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, acquisition.Lease, goal)
	if err != nil {
		t.Fatal(err)
	}
	return run, turn, acquisition.Lease
}

func newDelegationTestGateway(st *SQLiteStore) *toolgateway.Gateway {
	return toolgateway.New(st, policy.NewDefaultChecker()).
		WithSpecialistDelegationExecutor(application.NewSpecialistDelegationToolExecutor(st))
}

func delegationTestPayload(t *testing.T,
	spec domain.SpecialistDelegationSpec,
) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
