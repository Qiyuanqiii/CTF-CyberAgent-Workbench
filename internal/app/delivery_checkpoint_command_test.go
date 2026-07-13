package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
)

var deliveryCheckpointIDPattern = regexp.MustCompile(
	`delivery-checkpoint-[0-9]{14}-[a-f0-9]{12}`)
var deliveryWorkIDPattern = regexp.MustCompile(`work-[0-9]{14}-[a-f0-9]{12}`)

const deliveryCLIPlanPayload = `{"version":"plan_delivery.v1","directions":[` +
	`{"title":"Conservative","summary":"Keep the change narrow.","tradeoffs":["More sequential work"],"modules":[{"title":"Inspect","objective":"Inspect boundaries.","acceptance_criteria":["Boundaries recorded"],"dependencies":[]}]},` +
	`{"title":"Balanced","summary":"Deliver a tested path.","tradeoffs":["Moderate breadth"],"modules":[{"title":"Implement","objective":"Implement the path.","acceptance_criteria":["Focused tests pass"],"dependencies":[]},{"title":"Audit","objective":"Audit the path.","acceptance_criteria":["Audit recorded"],"dependencies":[1]}]},` +
	`{"title":"Accelerated","summary":"Prepare independent work.","tradeoffs":["Higher review load"],"modules":[{"title":"Prepare","objective":"Prepare slices.","acceptance_criteria":["Slices bounded"],"dependencies":[]}]}]}`

func TestDeliveryCheckpointCLIRecordsReplaysListsAndShows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	created, stderr, code := executeTestCommand(t, "run", "create",
		"exercise Delivery checkpoint CLI", "--profile", "review", "--phase", "plan",
		"--max-turns", "4", "--max-tokens", "1000", "--max-tool-calls", "8")
	if code != 0 || stderr != "" {
		t.Fatalf("Run create failed: output=%s stderr=%s code=%d", created, stderr, code)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("Run id missing: %s", created)
	}
	if _, stderr, code = executeTestCommand(t, "run", "start", runID); code != 0 {
		t.Fatalf("Run start failed: %s", stderr)
	}
	st, err := store.Open(filepath.Join(home, "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	root, _, err := st.RegisterRootAgent(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := st.AcquireRunExecutionLease(ctx,
		domain.AcquireRunExecutionLeaseRequest{RunID: runID,
			OwnerID: "delivery-cli-fixture", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, acquired.Lease,
		"prepare Plan/Delivery CLI fixture")
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := toolgateway.New(st, policy.NewDefaultChecker()).
		WithPlanDeliveryExecutor(application.NewPlanDeliveryToolExecutor(st)).
		Invoke(ctx, toolgateway.ToolCall{
			Name:         toolgateway.PlanDeliveryProposeTool,
			Payload:      json.RawMessage(deliveryCLIPlanPayload),
			OperationKey: "delivery-cli-plan-proposal", RunID: runID,
			AgentID: root.ID, SessionID: turn.Agent.SessionID,
			LeaseID:         acquired.Lease.LeaseID,
			LeaseGeneration: acquired.Lease.Generation,
			RequestedBy:     "run_supervisor",
		})
	if err != nil || outcome.Result == nil {
		t.Fatalf("Plan proposal fixture failed: %#v err=%v", outcome, err)
	}
	proposalID := outcome.Result.Metadata["proposal_id"]
	attempt := llm.ModelAttempt{Number: 1, TransportAttempt: 1, MaxAttempts: 1,
		Provider: "test", Model: "test-model"}
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint,
		attempt); err != nil || !inserted {
		t.Fatalf("model start failed: inserted=%t err=%v", inserted, err)
	}
	attempt.Outcome = llm.OutcomeSuccess
	response := llm.ChatResponse{Text: "directions ready", Provider: "test",
		Model: "test-model", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}}
	checkpoint, err := st.RecordSupervisorModelCompleted(ctx, turn.Checkpoint,
		attempt, response)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := st.CompleteSupervisorTurn(ctx, checkpoint, response,
		domain.RootAction{Version: domain.RootLifecycleVersion,
			Kind: domain.RootActionWait, Message: "choose a direction",
			Reason: "operator choice required"},
		policy.Decision{Allowed: true}, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.ReleaseRunExecutionLease(ctx, acquired.Lease); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	chosen, stderr, code := executeTestCommand(t, "run", "plan", "choose",
		proposalID, "2", "--operation-key", "delivery-cli-choice-0001")
	if code != 0 || stderr != "" {
		t.Fatalf("direction choice failed: output=%s stderr=%s code=%d", chosen, stderr, code)
	}
	workIDs := deliveryWorkIDPattern.FindAllString(chosen, -1)
	if len(workIDs) < 2 {
		t.Fatalf("selected WorkItems missing: %s", chosen)
	}
	if _, stderr, code = executeTestCommand(t, "run", "phase", runID, "deliver",
		"--operation-key", "delivery-cli-mode-0001", "--reason", "accepted direction"); code != 0 {
		t.Fatalf("Deliver phase failed: %s", stderr)
	}
	started, stderr, code := executeTestCommand(t, "todo", "start", workIDs[0])
	if code != 0 || stderr != "" {
		t.Fatalf("WorkItem start failed: output=%s stderr=%s code=%d", started, stderr, code)
	}
	_, stderr, code = executeTestCommand(t, "todo", "complete", workIDs[0])
	if code != apperror.ExitCode(apperror.New(apperror.CodeFailedPrecondition, "gate")) ||
		!strings.Contains(stderr, "requires a checkpoint") {
		t.Fatalf("ungated CLI completion was not rejected: stderr=%s code=%d", stderr, code)
	}
	args := []string{"run", "delivery", "checkpoint", workIDs[0],
		"--operation-key", "delivery-cli-checkpoint-0001",
		"--focused", "focused tests passed", "--diff-audit", "diff review passed",
		"--security-audit", "security review passed", "--handoff", "slice ready"}
	recorded, stderr, code := executeTestCommand(t, args...)
	checkpointID := deliveryCheckpointIDPattern.FindString(recorded)
	if code != 0 || stderr != "" || checkpointID == "" ||
		!strings.Contains(recorded, "completion_gate_ready: true") ||
		!strings.Contains(recorded, "replayed: false") {
		t.Fatalf("checkpoint command failed: output=%s stderr=%s code=%d", recorded, stderr, code)
	}
	replayed, stderr, code := executeTestCommand(t, args...)
	if code != 0 || stderr != "" || !strings.Contains(replayed, "replayed: true") ||
		!strings.Contains(replayed, checkpointID) {
		t.Fatalf("checkpoint replay failed: output=%s stderr=%s code=%d", replayed, stderr, code)
	}
	listed, stderr, code := executeTestCommand(t, "run", "delivery", "list", runID)
	if code != 0 || stderr != "" || strings.Count(listed, checkpointID) != 1 ||
		!strings.Contains(listed, "full_gate=false") {
		t.Fatalf("checkpoint list failed: output=%s stderr=%s code=%d", listed, stderr, code)
	}
	shown, stderr, code := executeTestCommand(t, "run", "delivery", "show", checkpointID)
	if code != 0 || stderr != "" || !strings.Contains(shown, "focused_verification: focused tests passed") ||
		!strings.Contains(shown, "security_audit: security review passed") {
		t.Fatalf("checkpoint show failed: output=%s stderr=%s code=%d", shown, stderr, code)
	}
}
