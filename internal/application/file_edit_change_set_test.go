package application

import (
	"testing"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
)

func TestBuildFileEditChangeSetPreservesIndependentFileStates(t *testing.T) {
	run := domain.Run{ID: "run-change-set", MissionID: "mission-change-set",
		SessionID: "session-change-set"}
	mission := domain.Mission{ID: run.MissionID, WorkspaceID: "workspace-change-set"}
	previews := []fileedit.Preview{
		{ID: "edit-proposed", SessionID: run.SessionID, WorkspaceID: mission.WorkspaceID,
			Path: "a.txt", Status: fileedit.StatusProposed, Diff: "+a\n"},
		{ID: "edit-applied", SessionID: run.SessionID, WorkspaceID: mission.WorkspaceID,
			Path: "b.txt", Status: fileedit.StatusApplied, Diff: "+b\n"},
		{ID: "edit-failed", SessionID: run.SessionID, WorkspaceID: mission.WorkspaceID,
			Path: "c.txt", Status: fileedit.StatusFailed, Diff: "+c\n"},
	}
	result, err := BuildFileEditChangeSet(run, mission, previews)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 || result.Counts.Proposed != 1 ||
		result.Counts.Applied != 1 || result.Counts.Failed != 1 ||
		result.TotalDiffBytes != 9 {
		t.Fatalf("unexpected change set: %+v", result)
	}
	previews[0].Status = fileedit.StatusDenied
	if result.Items[0].Status != fileedit.StatusProposed {
		t.Fatal("change set did not own its projected slice")
	}
}

func TestBuildFileEditChangeSetRejectsCrossRunRecords(t *testing.T) {
	run := domain.Run{ID: "run-change-set", MissionID: "mission-change-set",
		SessionID: "session-change-set"}
	mission := domain.Mission{ID: run.MissionID, WorkspaceID: "workspace-change-set"}
	_, err := BuildFileEditChangeSet(run, mission, []fileedit.Preview{{
		ID: "edit-cross-run", SessionID: "session-other", WorkspaceID: mission.WorkspaceID,
		Path: "a.txt", Status: fileedit.StatusProposed,
	}})
	if err == nil {
		t.Fatal("expected cross-Run file edit to be rejected")
	}
}
