package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

var runIDPattern = regexp.MustCompile(`run-[0-9]{14}-[a-f0-9]{12}`)
var sessionIDPattern = regexp.MustCompile(`sess-[0-9]{14}-[a-f0-9]{12}`)
var toolIDPattern = regexp.MustCompile(`tool-[0-9]{14}-[a-f0-9]{12}`)
var editIDPattern = regexp.MustCompile(`edit-[0-9]{14}-[a-f0-9]{12}`)

func executeTestCommand(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Execute(args, &out, &errOut)
	return out.String(), errOut.String(), code
}

func TestExecuteContextCancelsProviderAndPersistsFailure(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(entered)
		<-request.Context().Done()
	}))
	defer server.Close()
	t.Setenv("MIMO_API_KEY", "test-provider-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_MODEL", "test-model")

	created, stderr, code := executeTestCommand(t, "run", "create", "signal cancellation", "--profile", "review", "--route", "mimo/test-model")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("missing run id: %s", created)
	}
	if _, stderr, code := executeTestCommand(t, "run", "start", runID); code != 0 {
		t.Fatalf("run start failed: %s", stderr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	type commandResult struct {
		stderr string
		code   int
	}
	done := make(chan commandResult, 1)
	go func() {
		var out bytes.Buffer
		var errOut bytes.Buffer
		code := ExecuteContext(ctx, []string{"run", "step", runID}, &out, &errOut)
		done <- commandResult{stderr: errOut.String(), code: code}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("streaming provider was not called")
	}
	cancel()
	var result commandResult
	select {
	case result = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled CLI context did not stop the provider call")
	}
	if result.code != 7 || !strings.Contains(result.stderr, "context canceled") {
		t.Fatalf("unexpected cancelled command result: code=%d stderr=%s", result.code, result.stderr)
	}
	timeline, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 {
		t.Fatalf("run events failed: %s", stderr)
	}
	if strings.Count(timeline, "model.failed") != 1 || !strings.Contains(timeline, `"outcome":"cancelled"`) {
		t.Fatalf("provider cancellation was not durably audited: %s", timeline)
	}
	checkpoint, stderr, code := executeTestCommand(t, "run", "checkpoint", runID)
	if code != 0 || !strings.Contains(checkpoint, "phase: turn_started") {
		t.Fatalf("cancelled turn was not recoverable: code=%d stderr=%s checkpoint=%s", code, stderr, checkpoint)
	}
}

func TestRunCLIEndToEndLifecycle(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	if _, stderr, code := executeTestCommand(t, "workspace", "init", "demo"); code != 0 {
		t.Fatalf("workspace init failed: %s", stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "review this workspace", "--workspace", "demo", "--profile", "review", "--max-turns", "12")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	sessionID := sessionIDPattern.FindString(created)
	if runID == "" || sessionID == "" || !strings.Contains(created, "status: created") {
		t.Fatalf("unexpected create output: %s", created)
	}
	initialEvents, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || !strings.Contains(initialEvents, "run.created") || !strings.Contains(initialEvents, "session.attached") {
		t.Fatalf("unexpected initial events output=%s stderr=%s", initialEvents, stderr)
	}
	chatOutput, stderr, code := executeTestCommand(t, "session", "send", sessionID, "hello run timeline")
	if code != 0 {
		t.Fatalf("session send failed: %s", stderr)
	}
	if !strings.Contains(chatOutput, "[run "+runID+": action=continue status=running]") {
		t.Fatalf("session send did not expose supervised run state: %s", chatOutput)
	}
	toolOutput, stderr, code := executeTestCommand(t, "session", "send", sessionID, "/run echo hello")
	if code != 0 {
		t.Fatalf("tool proposal failed: %s", stderr)
	}
	toolID := toolIDPattern.FindString(toolOutput)
	if toolID == "" {
		t.Fatalf("missing tool id in output: %s", toolOutput)
	}
	if _, stderr, code := executeTestCommand(t, "tool", "approve", toolID); code != 0 {
		t.Fatalf("tool approval failed: %s", stderr)
	}
	editOutput, stderr, code := executeTestCommand(t, "edit", "propose", "--workspace", "demo", "--session", sessionID, "--path", "notes.txt", "--content", "timeline note")
	if code != 0 {
		t.Fatalf("file edit proposal failed: %s", stderr)
	}
	editID := editIDPattern.FindString(editOutput)
	if editID == "" {
		t.Fatalf("missing edit id in output: %s", editOutput)
	}
	if _, stderr, code := executeTestCommand(t, "edit", "approve", editID); code != 0 {
		t.Fatalf("file edit approval failed: %s", stderr)
	}
	for _, step := range []struct {
		action string
		status string
	}{
		{"start", "running"},
		{"pause", "paused"},
		{"resume", "running"},
		{"cancel", "cancelled"},
	} {
		stdout, stderr, code := executeTestCommand(t, "run", step.action, runID)
		if code != 0 || !strings.Contains(stdout, step.status) {
			t.Fatalf("run %s failed output=%s stderr=%s", step.action, stdout, stderr)
		}
	}
	shown, stderr, code := executeTestCommand(t, "run", "show", runID)
	if code != 0 || !strings.Contains(shown, "status: cancelled") || !strings.Contains(shown, `"max_turns":12`) {
		t.Fatalf("unexpected show output=%s stderr=%s", shown, stderr)
	}
	eventOutput, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || strings.Count(eventOutput, "run.status_changed") != 5 {
		t.Fatalf("unexpected event timeline output=%s stderr=%s", eventOutput, stderr)
	}
	for _, eventType := range []string{"session.message_created", "policy.decision", "tool.proposed", "tool.approved", "tool.completed", "file_edit.proposed", "file_edit.approved", "file_edit.applied"} {
		if !strings.Contains(eventOutput, eventType) {
			t.Fatalf("event timeline missing %s: %s", eventType, eventOutput)
		}
	}
	listed, stderr, code := executeTestCommand(t, "run", "list", "--status", "cancelled")
	if code != 0 || !strings.Contains(listed, runID) {
		t.Fatalf("unexpected list output=%s stderr=%s", listed, stderr)
	}
}

func TestRunCLIAdaptsLegacyTaskIdempotently(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	createdTask, stderr, code := executeTestCommand(t, "script", "new", "adapter smoke", "--workspace", "adapter-demo")
	if code != 0 {
		t.Fatalf("script new failed: %s", stderr)
	}
	taskID := regexp.MustCompile(`task-[0-9]{14}-[a-f0-9]{12}`).FindString(createdTask)
	if taskID == "" {
		t.Fatalf("missing task id: %s", createdTask)
	}
	first, stderr, code := executeTestCommand(t, "run", "adapt-task", taskID)
	if code != 0 || !strings.Contains(first, " adapted") {
		t.Fatalf("first adaptation failed output=%s stderr=%s code=%d", first, stderr, code)
	}
	runID := runIDPattern.FindString(first)
	if runID == "" || sessionIDPattern.FindString(first) == "" {
		t.Fatalf("missing adapted ids: %s", first)
	}
	second, stderr, code := executeTestCommand(t, "run", "adapt-task", taskID)
	if code != 0 || !strings.Contains(second, " reused") || runIDPattern.FindString(second) != runID {
		t.Fatalf("repeat adaptation was not idempotent output=%s stderr=%s code=%d", second, stderr, code)
	}
	timeline, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || strings.Count(timeline, "legacy.task_adapted") != 1 || strings.Count(timeline, "run.created") != 1 {
		t.Fatalf("unexpected adapted timeline output=%s stderr=%s", timeline, stderr)
	}
}

func TestCLIStableExitCodesPreserveErrorText(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	_, stderr, code := executeTestCommand(t, "run", "show")
	if code != 2 || stderr != "error: usage: cyberagent run show <run-id>\n" {
		t.Fatalf("unexpected invalid argument result code=%d stderr=%q", code, stderr)
	}
	_, stderr, code = executeTestCommand(t, "run", "show", "run-missing")
	if code != 3 || stderr != "error: sql: no rows in result set\n" {
		t.Fatalf("unexpected not found result code=%d stderr=%q", code, stderr)
	}
}

func TestRunCLISupervisorStepAndCheckpoint(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	_, stderr, code := executeTestCommand(t, "run", "checkpoint", "run-missing")
	if code != 3 {
		t.Fatalf("unexpected missing checkpoint result code=%d stderr=%s", code, stderr)
	}
	created, stderr, code := executeTestCommand(t, "run", "create", "supervisor cli smoke", "--profile", "review", "--max-turns", "1")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("missing run id: %s", created)
	}
	_, stderr, code = executeTestCommand(t, "run", "step", runID)
	if code != 4 || !strings.Contains(stderr, "supervisor requires running") {
		t.Fatalf("unexpected precondition result code=%d stderr=%s", code, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "run", "start", runID); code != 0 {
		t.Fatalf("run start failed: %s", stderr)
	}
	stepped, stderr, code := executeTestCommand(t, "run", "step", runID)
	if code != 0 || !strings.Contains(stepped, "turn 1 completed") || !strings.Contains(stepped, "model_attempts: 1") || !strings.Contains(stepped, "protocol_repairs: 0") || !strings.Contains(stepped, "stream_events: 1") || !strings.Contains(stepped, "stream_bytes:") || !strings.Contains(stepped, "model_outcome: success") || !strings.Contains(stepped, "action: continue") || !strings.Contains(stepped, "run_status: running") || !strings.Contains(stepped, "next_turn: 2") {
		t.Fatalf("unexpected step output=%s stderr=%s code=%d", stepped, stderr, code)
	}
	checkpoint, stderr, code := executeTestCommand(t, "run", "checkpoint", runID)
	if code != 0 || !strings.Contains(checkpoint, "phase: idle") || !strings.Contains(checkpoint, "next_turn: 2") {
		t.Fatalf("unexpected checkpoint output=%s stderr=%s code=%d", checkpoint, stderr, code)
	}
	_, stderr, code = executeTestCommand(t, "run", "step", runID)
	if code != 8 || !strings.Contains(stderr, "exhausted its 1 turn budget") {
		t.Fatalf("unexpected budget result code=%d stderr=%s", code, stderr)
	}
	timeline, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || strings.Count(timeline, "agent.turn_started") != 1 || strings.Count(timeline, "agent.turn_completed") != 1 || strings.Count(timeline, "model.started") != 1 || strings.Count(timeline, "model.completed") != 1 || strings.Count(timeline, "model.failed") != 0 {
		t.Fatalf("unexpected supervisor timeline output=%s stderr=%s", timeline, stderr)
	}
}

func TestRunCLIExecuteAndFinalize(t *testing.T) {
	t.Setenv("CYBERAGENT_HOME", t.TempDir())
	created, stderr, code := executeTestCommand(t, "run", "create", "execute cli smoke", "--profile", "code", "--max-turns", "3", "--max-tokens", "1000", "--timeout", "30s")
	if code != 0 {
		t.Fatalf("run create failed: %s", stderr)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("missing run id: %s", created)
	}
	if _, stderr, code := executeTestCommand(t, "run", "start", runID); code != 0 {
		t.Fatalf("run start failed: %s", stderr)
	}
	executed, stderr, code := executeTestCommand(t, "run", "execute", runID, "--max-steps", "2", "--finish", "--summary", "operator verified")
	if code != 0 || strings.Count(executed, "turn ") != 2 || strings.Count(executed, "\tcontinue\t") != 2 || !strings.Contains(executed, "finalized: completed") {
		t.Fatalf("unexpected execute output=%s stderr=%s code=%d", executed, stderr, code)
	}
	shown, stderr, code := executeTestCommand(t, "run", "show", runID)
	if code != 0 || !strings.Contains(shown, "status: completed") {
		t.Fatalf("unexpected finalized run output=%s stderr=%s", shown, stderr)
	}
	checkpoint, stderr, code := executeTestCommand(t, "run", "checkpoint", runID)
	if code != 0 || !strings.Contains(checkpoint, "phase: run_completed") || !strings.Contains(checkpoint, "next_turn: 3") || !strings.Contains(checkpoint, "total_tokens:") {
		t.Fatalf("unexpected finalized checkpoint output=%s stderr=%s", checkpoint, stderr)
	}
	timeline, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || strings.Count(timeline, "supervisor.run_completed") != 1 {
		t.Fatalf("unexpected completion timeline output=%s stderr=%s", timeline, stderr)
	}
	if _, stderr, code := executeTestCommand(t, "run", "finish", runID, "--summary", "repeat"); code != 0 {
		t.Fatalf("repeat finish was not idempotent: %s", stderr)
	}
	after, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || strings.Count(after, "supervisor.run_completed") != 1 {
		t.Fatalf("repeat finish duplicated timeline output=%s stderr=%s", after, stderr)
	}

	failedCreated, stderr, code := executeTestCommand(t, "run", "create", "fail cli smoke", "--profile", "review")
	if code != 0 {
		t.Fatalf("failed-run create failed: %s", stderr)
	}
	failedRunID := runIDPattern.FindString(failedCreated)
	if _, stderr, code := executeTestCommand(t, "run", "start", failedRunID); code != 0 {
		t.Fatalf("failed-run start failed: %s", stderr)
	}
	failed, stderr, code := executeTestCommand(t, "run", "fail", failedRunID, "--reason", "operator stopped")
	if code != 0 || !strings.Contains(failed, "finalized: failed") {
		t.Fatalf("unexpected fail output=%s stderr=%s code=%d", failed, stderr, code)
	}
}
