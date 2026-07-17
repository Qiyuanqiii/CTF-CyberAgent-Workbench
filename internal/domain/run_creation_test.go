package domain

import (
	"strings"
	"testing"
	"time"
)

func TestRunCreationOperationValidation(t *testing.T) {
	valid := RunCreationOperation{
		ProtocolVersion: RunCreationProtocolVersion,
		KeyDigest:       strings.Repeat("a", 64), RequestFingerprint: strings.Repeat("b", 64),
		MissionID: "mission-create", RunID: "run-create", SessionID: "sess-create",
		WorkspaceID: "workspace-create", RequestedBy: "http_control", CreatedAt: time.Now().UTC(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*RunCreationOperation){
		func(value *RunCreationOperation) { value.ProtocolVersion = "run_creation.v2" },
		func(value *RunCreationOperation) { value.KeyDigest = strings.Repeat("A", 64) },
		func(value *RunCreationOperation) { value.RequestFingerprint = "short" },
		func(value *RunCreationOperation) { value.RunID = " run-create" },
		func(value *RunCreationOperation) { value.RequestedBy = "" },
		func(value *RunCreationOperation) { value.CreatedAt = time.Time{} },
	}
	for index, mutate := range mutations {
		candidate := valid
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("mutation %d was accepted", index)
		}
	}
	candidate := valid
	candidate.MissionID = ""
	candidate.RunID = ""
	first := candidate.Validate().Error()
	for range 100 {
		if current := candidate.Validate().Error(); current != first {
			t.Fatalf("multi-field validation was nondeterministic: first=%q current=%q", first, current)
		}
	}
}
