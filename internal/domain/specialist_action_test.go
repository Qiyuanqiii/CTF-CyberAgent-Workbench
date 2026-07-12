package domain

import (
	"strings"
	"testing"
)

func TestDecodeSpecialistActionStrictLifecycle(t *testing.T) {
	continued, err := DecodeSpecialistAction(`{
		"version":"specialist_lifecycle.v1","action":"continue","message":"keep investigating"
	}`)
	if err != nil || continued.Kind != SpecialistActionContinue || continued.Report != nil {
		t.Fatalf("valid continuation was rejected: action=%#v err=%v", continued, err)
	}

	finished, err := DecodeSpecialistAction(`{
		"version":"specialist_lifecycle.v1","action":"finish","message":"analysis complete",
		"report":{"version":"agent_completion.v1","outcome":"succeeded","summary":"done","work_item_ids":[],"note_ids":[]}
	}`)
	if err != nil || finished.Kind != SpecialistActionFinish || finished.Report == nil ||
		finished.Report.Outcome != CompletionSucceeded {
		t.Fatalf("valid finish was rejected: action=%#v err=%v", finished, err)
	}

	invalid := []string{
		`{"version":"specialist_lifecycle.v1","action":"continue","message":"ok","usage":{"total_tokens":1}}`,
		`{"version":"specialist_lifecycle.v1","action":"continue","message":"ok","report":{"version":"agent_completion.v1","outcome":"succeeded","summary":"done","work_item_ids":[],"note_ids":[]}}`,
		`{"version":"specialist_lifecycle.v1","action":"finish","message":"done"}`,
		`{"version":"specialist_lifecycle.v1","action":"wait","message":"done"}`,
		`{"version":"specialist_lifecycle.v1","action":"continue","message":"ok"}{}`,
		`{"version":"specialist_lifecycle.v1","action":"continue","message":"` + strings.Repeat("x", MaxSpecialistMessageBytes+1) + `"}`,
	}
	for index, payload := range invalid {
		if _, err := DecodeSpecialistAction(payload); err == nil {
			t.Fatalf("invalid Specialist action %d was accepted", index)
		}
	}
}
