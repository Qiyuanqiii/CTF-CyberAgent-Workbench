package scriptprocess

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	processes map[string]Process
}

func (s *memoryStore) SaveScriptProcess(_ context.Context, process Process) (Process, error) {
	if err := process.Validate(); err != nil {
		return Process{}, err
	}
	s.processes[process.ID] = process
	return process, nil
}

func (s *memoryStore) GetScriptProcess(_ context.Context, id string) (Process, error) {
	process, ok := s.processes[id]
	if !ok {
		return Process{}, errors.New("script process not found")
	}
	return process, nil
}

func (s *memoryStore) ListScriptProcesses(_ context.Context, filter ListFilter) ([]Process, error) {
	var processes []Process
	for _, process := range s.processes {
		if filter.RunID != "" && process.RunID != filter.RunID ||
			filter.SessionID != "" && process.SessionID != filter.SessionID ||
			filter.Status != "" && process.Status != filter.Status {
			continue
		}
		processes = append(processes, process)
	}
	return processes, nil
}

func TestEncodeProposalIsDeterministicAndCopiesArguments(t *testing.T) {
	arguments := []string{"scripts/noop.py", "--value", "demo"}
	encoded, err := EncodeProposal(Proposal{Executable: " python ", Arguments: arguments, RequestedBackend: " LOCAL "})
	if err != nil {
		t.Fatal(err)
	}
	arguments[0] = "changed.py"
	want := `{"schema":"script_process.v1","executable":"python","arguments":["scripts/noop.py","--value","demo"],"working_directory":".","requested_backend":"local","execution_mode":"disabled"}`
	if encoded != want {
		t.Fatalf("encoded proposal mismatch:\n got: %s\nwant: %s", encoded, want)
	}
}

func TestProcessValidationRejectsUnnormalizedAndExecutableOutput(t *testing.T) {
	process := validProcess()
	process.Arguments = []string{"value\x00"}
	if err := process.Validate(); err == nil {
		t.Fatal("expected NUL argument rejection")
	}
	process = validProcess()
	process.Status = StatusProposed
	process.Stdout = "unexpected"
	if err := process.Validate(); err == nil {
		t.Fatal("expected output on proposed process to be rejected")
	}
	process = validProcess()
	process.Risk = " Medium "
	if err := process.Validate(); err == nil {
		t.Fatal("expected unnormalized risk rejection")
	}
}

func TestManagerApprovalIsRecoverableAndNeverExecutes(t *testing.T) {
	process := validProcess()
	store := &memoryStore{processes: map[string]Process{process.ID: process}}
	manager := NewManager(store)
	completed, err := manager.Approve(context.Background(), process.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != StatusCompleted || completed.Version != 3 || completed.ExitCode != 0 ||
		!strings.Contains(completed.Stdout, `"execution_mode":"disabled"`) {
		t.Fatalf("unexpected completed dry run: %#v", completed)
	}
	replayed, err := manager.Approve(context.Background(), process.ID)
	if err != nil || replayed.Version != completed.Version || replayed.Stdout != completed.Stdout {
		t.Fatalf("completed approval was not idempotent: %#v err=%v", replayed, err)
	}
}

func validProcess() Process {
	now := time.Now().UTC()
	return Process{
		ID: "process-1", OperationKeyDigest: Fingerprint("operation"), RunID: "run-1",
		SessionID: "sess-1", WorkspaceID: "ws-1", Executable: "python",
		Arguments: []string{"scripts/noop.py"}, WorkingDirectory: ".", RequestedBackend: BackendSandbox,
		ExecutionMode: ExecutionDisabled, Status: StatusProposed, Risk: "medium",
		PolicyReason: "tool call allowed", RequestFingerprint: Fingerprint("request"),
		ApprovalFingerprint: Fingerprint("approval"), RequestedBy: "test", Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
}
