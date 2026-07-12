package domain

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNoteDetailsNormalizeAndValidate(t *testing.T) {
	now := time.Now().UTC()
	details, err := NormalizeNoteDetails(NoteDetails{
		Title: "  Parser decision  ", Content: " keep strict JSON ", Category: NoteDecision,
		Visibility: NoteVisibilityRun, Tags: []string{" Parser ", "SECURITY", "parser"},
		SourceRefs: []string{" docs/spec.md ", "docs/spec.md"}, EvidenceIDs: []string{"evidence-2", "evidence-1"},
		Pinned: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	note := Note{
		ID: "note-1", RunID: "run-1", Title: details.Title, Content: details.Content,
		Category: details.Category, Visibility: details.Visibility, Tags: details.Tags,
		SourceRefs: details.SourceRefs, EvidenceIDs: details.EvidenceIDs, Status: NoteActive,
		Pinned: details.Pinned, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := note.Validate(); err != nil {
		t.Fatal(err)
	}
	if note.Title != "Parser decision" || note.Content != "keep strict JSON" ||
		!slices.Equal(note.Tags, []string{"parser", "security"}) ||
		!slices.Equal(note.EvidenceIDs, []string{"evidence-1", "evidence-2"}) {
		t.Fatalf("note details were not normalized: %#v", note)
	}
}

func TestNoteOwnerVisibilityRules(t *testing.T) {
	if _, err := NormalizeNoteDetails(NoteDetails{
		Title: "private", Content: "owner only", Visibility: NoteVisibilityOwner,
	}); err == nil {
		t.Fatal("expected owner-visible note without owner to fail")
	}
	if _, err := NormalizeNoteDetails(NoteDetails{
		Title: "run", Content: "shared", Visibility: NoteVisibilityRun, Owner: "root",
	}); err == nil {
		t.Fatal("expected non-owner note with owner to fail")
	}
	details, err := NormalizeNoteDetails(NoteDetails{
		Title: "private", Content: "owner only", Visibility: NoteVisibilityOwner, Owner: "root",
	})
	if err != nil || details.Owner != "root" {
		t.Fatalf("valid owner note failed: %#v err=%v", details, err)
	}
	agentID := "agent-20260711123456-abcdef012345"
	agentDetails, err := NormalizeNoteDetails(NoteDetails{
		Title: "Agent private", Content: "owner only", Visibility: NoteVisibilityOwner,
		OwnerAgentID: agentID,
	})
	if err != nil || agentDetails.OwnerAgentID != agentID || agentDetails.Owner != agentID {
		t.Fatalf("Agent-owned private note did not receive its compatibility label: %#v err=%v",
			agentDetails, err)
	}
}

func TestNoteArchiveRestoreLifecycle(t *testing.T) {
	now := time.Now().UTC()
	note := Note{
		ID: "note-1", RunID: "run-1", Title: "finding", Content: "observed behavior",
		Category: NoteObservation, Visibility: NoteVisibilityRun, Status: NoteActive,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := note.Transition(NoteArchived, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if note.Status != NoteArchived || note.ArchivedAt == nil {
		t.Fatalf("note was not archived: %#v", note)
	}
	if err := note.ApplyDetails(NoteDetails{Title: "changed", Content: "changed"}, now.Add(2*time.Minute)); err == nil {
		t.Fatal("expected archived note update rejection")
	}
	if err := note.Transition(NoteActive, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if note.Status != NoteActive || note.ArchivedAt != nil {
		t.Fatalf("note was not restored: %#v", note)
	}
}

func TestNoteRejectsInvalidContentAndEvidence(t *testing.T) {
	if _, err := NormalizeNoteDetails(NoteDetails{
		Title: "large", Content: strings.Repeat("x", MaxNoteContentBytes+1),
	}); err == nil {
		t.Fatal("expected oversized content rejection")
	}
	if _, err := NormalizeNoteDetails(NoteDetails{
		Title: "evidence", Content: "content", EvidenceIDs: []string{"evidence with spaces"},
	}); err == nil {
		t.Fatal("expected evidence id whitespace rejection")
	}
	if _, err := NormalizeNoteDetails(NoteDetails{
		Title: "invalid UTF-8", Content: string([]byte{0xff}),
	}); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("expected invalid UTF-8 content rejection, got %v", err)
	}
	if _, err := NormalizeNoteDetails(NoteDetails{
		Title: "invalid tag", Content: "content", Tags: []string{string([]byte{0xfe})},
	}); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("expected invalid UTF-8 tag rejection, got %v", err)
	}
}
