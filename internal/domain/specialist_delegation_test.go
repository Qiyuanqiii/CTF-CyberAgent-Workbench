package domain

import (
	"strings"
	"testing"
)

func TestSpecialistDelegationSpecStrictlyNormalizesBoundedAssignments(t *testing.T) {
	spec, err := DecodeSpecialistDelegationSpec([]byte(`{
		"version":"specialist_delegation.v1",
		"assignments":[
			{"title":" Parser review ","goal":" Inspect the parser ","skills":["work_item_create","model.chat","model.chat"],"turn_limit":2,"token_limit":128},
			{"title":"Tests","goal":"Add focused tests","skills":["note_create"],"turn_limit":1,"token_limit":64}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(); err != nil || len(spec.Assignments) != 2 ||
		spec.Assignments[0].Ordinal != 1 || spec.Assignments[0].Title != "Parser review" ||
		len(spec.Assignments[0].Skills) != 2 || spec.Assignments[0].Skills[0] != "model.chat" {
		t.Fatalf("unexpected normalized delegation: %#v err=%v", spec, err)
	}
}

func TestSpecialistDelegationSpecRejectsProtocolAndShapeDrift(t *testing.T) {
	validAssignment := `{"title":"A","goal":"B","skills":["model.chat"],"turn_limit":1,"token_limit":1}`
	cases := map[string]string{
		"unknown field":            `{"version":"specialist_delegation.v1","assignments":[` + validAssignment + `],"spawn":true}`,
		"trailing data":            `{"version":"specialist_delegation.v1","assignments":[` + validAssignment + `]} {}`,
		"wrong version":            `{"version":"specialist_delegation.v2","assignments":[` + validAssignment + `]}`,
		"empty list":               `{"version":"specialist_delegation.v1","assignments":[]}`,
		"too many":                 `{"version":"specialist_delegation.v1","assignments":[` + validAssignment + `,` + validAssignment + `,` + validAssignment + `]}`,
		"duplicate":                `{"version":"specialist_delegation.v1","assignments":[` + validAssignment + `,` + validAssignment + `]}`,
		"zero budget":              `{"version":"specialist_delegation.v1","assignments":[{"title":"A","goal":"B","skills":["model.chat"],"turn_limit":0,"token_limit":1}]}`,
		"unknown assignment field": `{"version":"specialist_delegation.v1","assignments":[{"title":"A","goal":"B","skills":["model.chat"],"turn_limit":1,"token_limit":1,"agent_id":"spoof"}]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSpecialistDelegationSpec([]byte(raw)); err == nil {
				t.Fatal("expected strict delegation rejection")
			}
		})
	}
	oversized := `{"version":"specialist_delegation.v1","assignments":[{"title":"A","goal":"` +
		strings.Repeat("x", MaxSpecialistDelegationGoalRunes+1) +
		`","skills":["model.chat"],"turn_limit":1,"token_limit":1}]}`
	if _, err := DecodeSpecialistDelegationSpec([]byte(oversized)); err == nil {
		t.Fatal("expected oversized delegation goal rejection")
	}
}
