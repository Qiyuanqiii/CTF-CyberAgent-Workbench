package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/store"
)

func TestModelRouteFileReviewAndWakeCommandsUseDurableBoundaries(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	output, stderr, code := executeTestCommand(t, "model", "set", "code", "mock/mock-code")
	if code != 0 || stderr != "" || !strings.Contains(output, "available: true") {
		t.Fatalf("model set output=%q stderr=%q code=%d", output, stderr, code)
	}

	st, err := store.Open(filepath.Join(home, "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	workspace := store.WorkspaceRecord{ID: "workspace-cli-controls", Name: "cli-controls",
		RootPath: root, CreatedAt: time.Now().UTC()}
	if err := st.SaveWorkspace(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	_, created, err := application.NewRunService(st).Create(t.Context(),
		application.CreateRunRequest{Goal: "exercise CLI controls", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.NewRunService(st).Start(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnqueueOperatorSteering(t.Context(), domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "queued CLI wake input",
		OperationKey: "cli-wake-queue-0001", RequestedBy: "cli_test",
	}); err != nil {
		t.Fatal(err)
	}
	edit, err := fileedit.NewManager(st).Propose(t.Context(), fileedit.Proposal{
		SessionID: run.SessionID, WorkspaceID: workspace.ID, WorkspaceRoot: root,
		Path: "review-only.txt", ProposedText: "do not write during review\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	output, stderr, code = executeTestCommand(t, "edit", "review-approve", run.ID, edit.ID)
	if code != 0 || stderr != "" || !strings.Contains(output, "file_written: false") ||
		!strings.Contains(output, "approved") {
		t.Fatalf("edit review output=%q stderr=%q code=%d", output, stderr, code)
	}
	if _, err := os.Stat(filepath.Join(root, "review-only.txt")); !os.IsNotExist(err) {
		t.Fatalf("CLI review wrote the file: %v", err)
	}

	output, stderr, code = executeTestCommand(t, "run", "wake", "schedule", run.ID,
		"--operation-key", "cli-wake-schedule-0001")
	if code != 0 || stderr != "" || !strings.Contains(output, "status: queued") ||
		!strings.Contains(output, "background_loop_enabled: false") {
		t.Fatalf("wake schedule output=%q stderr=%q code=%d", output, stderr, code)
	}
	output, stderr, code = executeTestCommand(t, "run", "wake", "cancel", run.ID,
		"--operation-key", "cli-wake-cancel-0001")
	if code != 0 || stderr != "" || !strings.Contains(output, "status: cancelled") {
		t.Fatalf("wake cancel output=%q stderr=%q code=%d", output, stderr, code)
	}
}
