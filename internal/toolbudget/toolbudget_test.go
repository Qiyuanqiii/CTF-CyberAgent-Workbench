package toolbudget

import (
	"strings"
	"testing"
)

func TestChargeRequestNormalizeRequiresSupervisorLease(t *testing.T) {
	normalized, err := (ChargeRequest{
		RunID: " run-1 ", SessionID: " session-1 ", WorkspaceID: " workspace-1 ",
		ToolName: " note_create ", ActionClass: " run_memory ",
		LeaseID: " lease-1 ", LeaseGeneration: 2, RequestedBy: " run_supervisor ",
	}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.RunID != "run-1" || normalized.ToolName != "note_create" ||
		normalized.LeaseID != "lease-1" || normalized.LeaseGeneration != 2 {
		t.Fatalf("charge request was not normalized: %#v", normalized)
	}

	for _, request := range []ChargeRequest{
		{ToolName: "note_create", ActionClass: "run_memory", RequestedBy: RequesterRunSupervisor},
		{ToolName: "note_create", ActionClass: "run_memory", LeaseID: "lease-1"},
		{ToolName: "note_create", ActionClass: "run_memory", LeaseGeneration: 1},
		{ToolName: "note_create", ActionClass: "run_memory", LeaseID: "lease-1", LeaseGeneration: -1},
	} {
		if _, err := request.Normalize(); err == nil {
			t.Fatalf("invalid charge request was accepted: %#v", request)
		}
	}
}

func TestChargeRequestNormalizeRejectsMissingAndOversizedIdentity(t *testing.T) {
	if _, err := (ChargeRequest{ActionClass: "run_memory"}).Normalize(); err == nil {
		t.Fatal("charge request accepted a missing tool name")
	}
	if _, err := (ChargeRequest{
		ToolName: strings.Repeat("界", MaxIdentityRunes+1), ActionClass: "run_memory",
	}).Normalize(); err == nil {
		t.Fatal("charge request accepted an oversized identity")
	}
}
