package domain

import (
	"strings"
	"testing"
	"time"
)

func TestDeliveryCheckpointValidationPinsFinalBoundaryAndEvidence(t *testing.T) {
	now := time.Now().UTC()
	checkpoint := DeliveryCheckpoint{
		ID: "delivery-checkpoint-domain", RunID: "run-domain",
		SelectionID: "selection-domain", ProposalID: "proposal-domain",
		WorkItemID: "work-domain", DirectionOrdinal: 2, ModuleOrdinal: 2,
		ModuleCount: 2, ModeSnapshotID: "mode-domain", ModeRevision: 3,
		WorkItemVersion: 4, AcceptanceFingerprint: strings.Repeat("a", 64),
		SourceFingerprint:   strings.Repeat("b", 64),
		FocusedVerification: "focused tests passed", DiffAudit: "diff reviewed",
		SecurityAudit: "security reviewed", FullGateRequired: true,
		FunctionalVerification: "full suite passed",
		RobustnessAudit:        "failure paths reviewed",
		HandoffNoteID:          "note-domain", HandoffDigest: strings.Repeat("c", 64),
		RequestedBy: "operator", Version: 1, CreatedAt: now,
	}
	if err := checkpoint.Validate(); err != nil {
		t.Fatal(err)
	}
	partial := checkpoint
	partial.RobustnessAudit = ""
	if err := partial.Validate(); err == nil {
		t.Fatal("final Delivery boundary accepted missing robustness evidence")
	}
	nonBoundary := checkpoint
	nonBoundary.ModuleOrdinal = 1
	nonBoundary.FullGateRequired = false
	if err := nonBoundary.Validate(); err == nil {
		t.Fatal("non-boundary Delivery checkpoint accepted full-gate evidence")
	}
	invalidText := checkpoint
	invalidText.FocusedVerification = "passed\x00hidden"
	if err := invalidText.Validate(); err == nil {
		t.Fatal("Delivery checkpoint accepted a NUL-bearing evidence string")
	}
	first := DeliveryCheckpointRequestFingerprint(checkpoint)
	changed := checkpoint
	changed.SecurityAudit = "different audit"
	if first == "" || first == DeliveryCheckpointRequestFingerprint(changed) {
		t.Fatal("Delivery checkpoint request fingerprint did not bind evidence")
	}
}

func TestDeliveryCheckpointReadyRejectsStaleModeAndWorkItemRevision(t *testing.T) {
	checkpoint := DeliveryCheckpoint{RunID: "run-ready", WorkItemID: "work-ready",
		ModeSnapshotID: "mode-ready", ModeRevision: 2, WorkItemVersion: 3}
	item := WorkItem{ID: "work-ready", RunID: "run-ready",
		Status: WorkItemInProgress, Version: 3}
	mode := RunModeSnapshot{ID: "mode-ready", Revision: 2,
		Phase: ExecutionPhaseDeliver}
	if !DeliveryCheckpointReady(checkpoint, item, mode) {
		t.Fatal("current in-progress Delivery checkpoint was not ready")
	}
	mode.Revision++
	if DeliveryCheckpointReady(checkpoint, item, mode) {
		t.Fatal("stale Delivery mode remained ready")
	}
	item.Status = WorkItemCompleted
	item.Version = 4
	if !DeliveryCheckpointReady(checkpoint, item, mode) {
		t.Fatal("completed WorkItem did not preserve its exact prior checkpoint")
	}
	item.Version++
	if DeliveryCheckpointReady(checkpoint, item, mode) {
		t.Fatal("changed WorkItem version reused an old Delivery checkpoint")
	}
}
