package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
)

type fileEditProposalMemoryStore struct {
	mu            sync.Mutex
	run           domain.Run
	mission       domain.Mission
	session       session.Session
	workspace     session.WorkspaceInfo
	edits         map[string]fileedit.Edit
	saves         int
	failAfterSave bool
}

func (s *fileEditProposalMemoryStore) GetRun(context.Context, string) (domain.Run, error) {
	return s.run, nil
}
func (s *fileEditProposalMemoryStore) GetMission(context.Context, string) (domain.Mission, error) {
	return s.mission, nil
}
func (s *fileEditProposalMemoryStore) GetSession(context.Context, string) (session.Session, error) {
	return s.session, nil
}
func (s *fileEditProposalMemoryStore) GetWorkspaceInfo(context.Context, string) (session.WorkspaceInfo, error) {
	return s.workspace, nil
}
func (s *fileEditProposalMemoryStore) SaveFileEdit(_ context.Context,
	edit fileedit.Edit,
) (fileedit.Edit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.edits[edit.ID] = edit
	s.saves++
	if s.failAfterSave {
		s.failAfterSave = false
		return fileedit.Edit{}, errors.New("uncertain save response")
	}
	return edit, nil
}
func (s *fileEditProposalMemoryStore) GetFileEdit(_ context.Context,
	id string,
) (fileedit.Edit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	edit, found := s.edits[id]
	if !found {
		return fileedit.Edit{}, errors.New("not found")
	}
	return edit, nil
}
func (s *fileEditProposalMemoryStore) ListFileEdits(context.Context,
	fileedit.ListFilter,
) ([]fileedit.Edit, error) {
	return nil, nil
}

func newFileEditProposalFixture(t *testing.T) (*fileEditProposalMemoryStore,
	*FileEditProposalService, string,
) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store := &fileEditProposalMemoryStore{
		run: domain.Run{ID: "run-proposal", MissionID: "mission-proposal",
			SessionID: "session-proposal", Status: domain.RunRunning},
		mission: domain.Mission{ID: "mission-proposal", WorkspaceID: "workspace-proposal"},
		session: session.Session{ID: "session-proposal", WorkspaceID: "workspace-proposal",
			Title: "proposal", Route: "code", Status: session.StatusActive,
			CreatedAt: now, UpdatedAt: now},
		workspace: session.WorkspaceInfo{ID: "workspace-proposal", Name: "proposal",
			RootPath: root},
		edits: make(map[string]fileedit.Edit),
	}
	return store, NewFileEditProposalService(store, policy.NewDefaultChecker()), path
}

func TestFileEditProposalUsesOpaqueExactSourceAndReplaysWithoutWriting(t *testing.T) {
	store, service, path := newFileEditProposalFixture(t)
	source, err := service.IssueSource(t.Context(), store.run.ID, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !source.Editable || source.Content != "before\n" || source.Handle == "" ||
		source.ContentSHA256 != fileedit.HashText("before\n") {
		t.Fatalf("invalid proposal source: %#v", source)
	}
	request := CreateFileEditProposalRequest{Version: FileEditProposalProtocolVersion,
		RunID: store.run.ID, SourceHandle: source.Handle, ProposedText: "after\n"}
	created, err := service.Propose(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.Propose(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if created.Edit.Status != fileedit.StatusProposed || created.Replayed ||
		!replayed.Replayed || replayed.Edit.ID != created.Edit.ID || store.saves != 1 ||
		string(contents) != "before\n" {
		t.Fatalf("proposal crossed its pending-only boundary: created=%#v replay=%#v saves=%d content=%q",
			created, replayed, store.saves, contents)
	}
	request.ProposedText = "different\n"
	if _, err := service.Propose(t.Context(), request); err == nil {
		t.Fatal("one source handle was reused for different content")
	}
}

func TestFileEditProposalRejectsStaleAndRedactedSources(t *testing.T) {
	store, service, path := newFileEditProposalFixture(t)
	source, err := service.IssueSource(t.Context(), store.run.ID, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("changed elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Propose(t.Context(), CreateFileEditProposalRequest{
		Version: FileEditProposalProtocolVersion, RunID: store.run.ID,
		SourceHandle: source.Handle, ProposedText: "after\n",
	}); err == nil {
		t.Fatal("stale proposal source was accepted")
	}
	if err := os.WriteFile(path, []byte("token=sk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"),
		0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.IssueSource(t.Context(), store.run.ID, "README.md"); err == nil {
		t.Fatal("redacted file received an editable source handle")
	}
}

func TestFileEditProposalReconcilesUncertainSaveAndPermanentlyBindsIntent(t *testing.T) {
	store, service, _ := newFileEditProposalFixture(t)
	source, err := service.IssueSource(t.Context(), store.run.ID, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	request := CreateFileEditProposalRequest{Version: FileEditProposalProtocolVersion,
		RunID: store.run.ID, SourceHandle: source.Handle, ProposedText: "after\n"}
	store.failAfterSave = true
	if _, err := service.Propose(t.Context(), request); err == nil {
		t.Fatal("uncertain save response was reported as success")
	}
	changed := request
	changed.ProposedText = "different\n"
	if _, err := service.Propose(t.Context(), changed); err == nil {
		t.Fatal("source handle changed intent after an uncertain save")
	}
	replayed, err := service.Propose(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Edit.ID == "" || store.saves != 1 {
		t.Fatalf("uncertain proposal did not reconcile exactly once: %#v saves=%d",
			replayed, store.saves)
	}
}

func TestFileEditProposalReplayRejectsAnAdvancedReviewAsConflict(t *testing.T) {
	store, service, _ := newFileEditProposalFixture(t)
	source, err := service.IssueSource(t.Context(), store.run.ID, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	request := CreateFileEditProposalRequest{Version: FileEditProposalProtocolVersion,
		RunID: store.run.ID, SourceHandle: source.Handle, ProposedText: "after\n"}
	created, err := service.Propose(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	advanced := store.edits[created.Edit.ID]
	advanced.Status = fileedit.StatusDenied
	store.edits[created.Edit.ID] = advanced
	store.mu.Unlock()
	if _, err := service.Propose(t.Context(), request); err == nil ||
		apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("advanced proposal replay error = %v, want conflict", err)
	}
	if store.saves != 1 {
		t.Fatalf("advanced proposal replay saved %d edits, want 1", store.saves)
	}
}

func TestFileEditProposalReissuesOnlyAnUnchangedGoProjection(t *testing.T) {
	store, service, path := newFileEditProposalFixture(t)
	source, err := service.IssueSource(t.Context(), store.run.ID, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	reissued, err := service.ReissueSource(t.Context(), store.run.ID, source.Path,
		source.ContentSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if reissued.Handle == source.Handle || reissued.Content != source.Content ||
		reissued.ContentSHA256 != source.ContentSHA256 {
		t.Fatalf("source handle was not safely rotated: old=%#v new=%#v", source, reissued)
	}
	if err := os.WriteFile(path, []byte("changed after editor opened\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReissueSource(t.Context(), store.run.ID, source.Path,
		source.ContentSHA256); err == nil || apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("stale editor source reissue error=%v, want conflict", err)
	}
	if _, err := service.ReissueSource(t.Context(), store.run.ID, source.Path,
		"not-a-digest"); err == nil || apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("invalid reissue digest error=%v", err)
	}
}

func TestFileEditProposalRecoversDurablePendingReviewWithoutAuthority(t *testing.T) {
	store, service, path := newFileEditProposalFixture(t)
	source, err := service.IssueSource(t.Context(), store.run.ID, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Propose(t.Context(), CreateFileEditProposalRequest{
		Version: FileEditProposalProtocolVersion, RunID: store.run.ID,
		SourceHandle: source.Handle, ProposedText: "after restart\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	// A fresh service has no renderer source grants and models a process restart.
	restarted := NewFileEditProposalService(store, policy.NewDefaultChecker())
	recovery, err := restarted.Recover(t.Context(), store.run.ID, created.Edit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.ProtocolVersion != FileEditRecoveryProtocolVersion ||
		recovery.EditID != created.Edit.ID || recovery.OriginalContent != "before\n" ||
		recovery.ProposedContent != "after restart\n" || recovery.Stale ||
		!recovery.ReviewRequired || recovery.Editable {
		t.Fatalf("durable proposal recovery widened authority: %#v", recovery)
	}
	if err := os.WriteFile(path, []byte("changed before review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, err := restarted.Recover(t.Context(), store.run.ID, created.Edit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Stale || stale.CurrentContentHash == stale.OriginalSHA256 || stale.Editable {
		t.Fatalf("stale durable proposal was not surfaced safely: %#v", stale)
	}
	store.mu.Lock()
	advanced := store.edits[created.Edit.ID]
	advanced.Status = fileedit.StatusApproved
	store.edits[created.Edit.ID] = advanced
	store.mu.Unlock()
	if _, err := restarted.Recover(t.Context(), store.run.ID, created.Edit.ID); err == nil ||
		apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("advanced proposal recovery error=%v, want conflict", err)
	}
}

func TestFileEditProposalRecoversPendingNewFileWithoutCreatingIt(t *testing.T) {
	store, service, _ := newFileEditProposalFixture(t)
	if err := os.MkdirAll(filepath.Join(store.workspace.RootPath, "outputs"), 0o755); err != nil {
		t.Fatal(err)
	}
	proposed := "new file\n"
	edit := fileedit.Edit{ID: "edit-missing-source", SessionID: store.session.ID,
		WorkspaceID: store.workspace.ID, Path: "outputs/new.txt", Status: fileedit.StatusProposed,
		OriginalText: "", ProposedText: proposed, OriginalHash: "missing",
		ProposedHash: fileedit.HashText(proposed), CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC()}
	store.mu.Lock()
	store.edits[edit.ID] = edit
	store.mu.Unlock()

	recovery, err := service.Recover(t.Context(), store.run.ID, edit.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.OriginalSHA256 != "missing" || recovery.OriginalContent != "" ||
		recovery.CurrentContentHash != "missing" || recovery.Stale || recovery.Editable {
		t.Fatalf("missing-file recovery widened authority: %#v", recovery)
	}
	if _, err := os.Stat(filepath.Join(store.workspace.RootPath, "outputs", "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("recovery created the proposed file: %v", err)
	}
}
