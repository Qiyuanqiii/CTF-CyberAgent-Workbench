package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/toolgateway"
)

func TestSQLiteReadPagesUseStableOffsetsAndBoundedMetadataQueries(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "read-pages.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	for index := 1; index <= 3; index++ {
		record := session.New("", "manual session", "review")
		record.ID = "sess-page-" + string(rune('0'+index))
		record.CreatedAt = base.Add(time.Duration(index) * time.Minute)
		record.UpdatedAt = record.CreatedAt
		if err := st.SaveSession(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	allSessions, err := st.ListSessions(ctx)
	if err != nil || len(allSessions) != 3 {
		t.Fatalf("unexpected Session seed: %#v err=%v", allSessions, err)
	}
	sessionPage, err := st.ListSessionsPage(ctx, 1, 1)
	if err != nil || len(sessionPage) != 1 || sessionPage[0].ID != allSessions[1].ID {
		t.Fatalf("Session offset page is unstable: %#v err=%v", sessionPage, err)
	}

	messageSession := allSessions[len(allSessions)-1]
	for index, compacted := range []bool{false, true, false} {
		message := session.NewMessage(messageSession.ID, "user", "message "+string(rune('1'+index)))
		message.Compacted = compacted
		if _, err := st.SaveSessionMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	visible, err := st.ListSessionMessagesPage(ctx, messageSession.ID, false, 0, 10)
	if err != nil || len(visible) != 2 || visible[0].Content != "message 1" || visible[1].Content != "message 3" {
		t.Fatalf("compacted message filter is inconsistent: %#v err=%v", visible, err)
	}
	messagePage, err := st.ListSessionMessagesPage(ctx, messageSession.ID, true, 1, 1)
	if err != nil || len(messagePage) != 1 || messagePage[0].Content != "message 2" || !messagePage[0].Compacted {
		t.Fatalf("message offset page is unstable: %#v err=%v", messagePage, err)
	}

	runService := application.NewRunService(st)
	_, firstRun, err := runService.Create(ctx, application.CreateRunRequest{
		Goal: "first paged Run", Profile: "review", ModelRoute: "review",
		Budget: domain.Budget{MaxTurns: 5, MaxToolCalls: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstRun, err = runService.Start(ctx, firstRun.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runService.Create(ctx, application.CreateRunRequest{
		Goal: "second paged Run", Profile: "code", ModelRoute: "code",
		Budget: domain.Budget{MaxTurns: 5, MaxToolCalls: 10},
	}); err != nil {
		t.Fatal(err)
	}
	allRuns, err := st.ListRuns(ctx, domain.RunFilter{Limit: 100})
	if err != nil || len(allRuns) != 2 {
		t.Fatalf("unexpected Run seed: %#v err=%v", allRuns, err)
	}
	runPage, err := st.ListRuns(ctx, domain.RunFilter{Limit: 1, Offset: 1})
	if err != nil || len(runPage) != 1 || runPage[0].ID != allRuns[1].ID {
		t.Fatalf("Run offset page is unstable: %#v err=%v", runPage, err)
	}
	eventList, err := st.ListRunEvents(ctx, firstRun.ID)
	if err != nil || len(eventList) < 4 {
		t.Fatalf("unexpected Run events: %#v err=%v", eventList, err)
	}
	eventPage, err := st.ListRunEventsPage(ctx, firstRun.ID, 1, 1)
	if err != nil || len(eventPage) != 1 || eventPage[0].EventID != eventList[1].EventID {
		t.Fatalf("event offset page is unstable: %#v err=%v", eventPage, err)
	}
	eventSequencePage, err := st.ListRunEventsAfterSequence(ctx, firstRun.ID, eventList[0].Sequence, 2)
	if err != nil || len(eventSequencePage) != 2 || eventSequencePage[0].Sequence != eventList[1].Sequence ||
		eventSequencePage[1].Sequence != eventList[2].Sequence {
		t.Fatalf("event sequence page is unstable: %#v err=%v", eventSequencePage, err)
	}
	noLaterEvents, err := st.ListRunEventsAfterSequence(ctx, firstRun.ID,
		eventList[len(eventList)-1].Sequence, 10)
	if err != nil || len(noLaterEvents) != 0 || noLaterEvents == nil {
		t.Fatalf("empty event sequence page is unstable: %#v err=%v", noLaterEvents, err)
	}

	workService := application.NewWorkItemService(st)
	noteService := application.NewNoteService(st)
	for index := 1; index <= 3; index++ {
		if _, err := workService.Create(ctx, application.CreateWorkItemRequest{
			RunID: firstRun.ID, Title: "paged work " + string(rune('0'+index)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := noteService.Create(ctx, application.CreateNoteRequest{
			RunID: firstRun.ID, Title: "paged note " + string(rune('0'+index)), Content: "content",
		}); err != nil {
			t.Fatal(err)
		}
	}
	allWork, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: firstRun.ID, Limit: 10})
	if err != nil || len(allWork) != 3 {
		t.Fatalf("unexpected WorkItem seed: %#v err=%v", allWork, err)
	}
	workPage, err := st.ListWorkItems(ctx, domain.WorkItemFilter{RunID: firstRun.ID, Limit: 1, Offset: 1})
	if err != nil || len(workPage) != 1 || workPage[0].ID != allWork[1].ID {
		t.Fatalf("WorkItem offset page is unstable: %#v err=%v", workPage, err)
	}
	allNotes, err := st.ListNotes(ctx, domain.NoteFilter{RunID: firstRun.ID, Limit: 10})
	if err != nil || len(allNotes) != 3 {
		t.Fatalf("unexpected Note seed: %#v err=%v", allNotes, err)
	}
	notePage, err := st.ListNotes(ctx, domain.NoteFilter{RunID: firstRun.ID, Limit: 1, Offset: 1})
	if err != nil || len(notePage) != 1 || notePage[0].ID != allNotes[1].ID {
		t.Fatalf("Note offset page is unstable: %#v err=%v", notePage, err)
	}

	gateway := toolgateway.New(st, policy.NewDefaultChecker())
	proposal, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "echo metadata"},
		RunID: firstRun.ID, SessionID: firstRun.SessionID, RequestedBy: "read_page_test",
	})
	if err != nil || proposal.Proposal == nil {
		t.Fatalf("tool proposal failed: %#v err=%v", proposal, err)
	}
	reviewed, err := gateway.Review(ctx, toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, Tool: toolgateway.ShellTool,
		ProposalID: proposal.Proposal.ID, ReviewedBy: "read_page_test",
	})
	if err != nil || reviewed.Result == nil {
		t.Fatalf("tool review failed: %#v err=%v", reviewed, err)
	}
	artifactID := reviewed.Result.Metadata["artifact_stdout_id"]
	descriptor, err := st.GetRunArtifactDescriptor(ctx, artifactID)
	if err != nil || descriptor.ID != artifactID || descriptor.SizeBytes <= 0 {
		t.Fatalf("metadata-only Artifact lookup failed: %#v err=%v", descriptor, err)
	}
	blob, err := st.GetRunArtifact(ctx, artifactID)
	if err != nil || blob.Descriptor != descriptor || blob.Content == "" {
		t.Fatalf("Artifact descriptor diverged from Blob metadata: %#v err=%v", blob, err)
	}
	artifactPage, err := st.ListRunArtifacts(ctx, artifact.ListFilter{RunID: firstRun.ID, Limit: 1, Offset: 1})
	if err != nil || len(artifactPage) != 0 {
		t.Fatalf("Artifact offset page is unstable: %#v err=%v", artifactPage, err)
	}
}

func TestSQLiteReadPagesRejectOutOfRangeBounds(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "read-page-bounds.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	if _, err := st.ListSessionsPage(ctx, -1, 1); err == nil {
		t.Fatal("negative Session page offset was accepted")
	}
	if _, err := st.ListSessionsPage(ctx, 0, maxStoreReadPageLimit+1); err == nil {
		t.Fatal("oversized Session page limit was accepted")
	}
	if _, err := st.ListSessionMessagesPage(ctx, "session", false, maxStoreListOffset+1, 1); err == nil {
		t.Fatal("oversized message page offset was accepted")
	}
	if _, err := st.ListRunEventsPage(ctx, "run", 0, 0); err == nil {
		t.Fatal("zero event page limit was accepted")
	}
	if _, err := st.ListRunEventsAfterSequence(ctx, "run", -1, 1); err == nil {
		t.Fatal("negative event sequence cursor was accepted")
	}
	if _, err := st.ListRunEventsAfterSequence(ctx, "run", 0, maxStoreReadPageLimit+1); err == nil {
		t.Fatal("oversized event sequence page was accepted")
	}
	if _, err := st.ListRuns(ctx, domain.RunFilter{Offset: maxStoreListOffset + 1}); err == nil {
		t.Fatal("oversized Run list offset was accepted")
	}
}
