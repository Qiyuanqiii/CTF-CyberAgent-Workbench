package domain

import (
	"testing"
	"time"
)

func TestRunExecutionProfileDefinitionsRemainClosedAndNonAuthorizing(t *testing.T) {
	now := time.Now().UTC()
	mission := Mission{ID: "mission-profile-test", Profile: ProfileCode}
	run := Run{ID: "run-profile-test", MissionID: mission.ID}
	initial, err := NewInitialRunExecutionProfileSnapshot("profile-snapshot-one",
		run, mission, "test_operator", "preview by default", now)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Profile != RunExecutionProfilePreview || initial.Backend != ExecutionBackendNoop ||
		initial.RequiredGate != ExecutionGateNone || initial.ProcessEnabled ||
		initial.ExecutionAuthorized || initial.CapabilityGrant {
		t.Fatalf("unexpected initial execution profile: %#v", initial)
	}
	docker, err := initial.Next("profile-snapshot-two", RunExecutionProfileDocker,
		"test_operator", "select isolated backend", now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if docker.Backend != ExecutionBackendDocker ||
		docker.ApprovalPolicy != ExecutionApprovalAlways ||
		docker.FilesystemScope != ExecutionFilesystemWorkspace ||
		docker.NetworkScope != ExecutionNetworkDisabled ||
		docker.RequiredGate != ExecutionGateDockerProductionStart ||
		docker.ProcessEnabled || docker.ExecutionAuthorized || docker.CapabilityGrant {
		t.Fatalf("Docker selection escaped its closed boundary: %#v", docker)
	}
	local, err := docker.Next("profile-snapshot-three", RunExecutionProfileLocal,
		"test_operator", "select local workspace backend", now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if local.Backend != ExecutionBackendLocal || local.RiskTier != ExecutionRiskHigh ||
		local.RequiredGate != ExecutionGateLocalOSSandbox || local.ProcessEnabled ||
		local.ExecutionAuthorized || local.CapabilityGrant {
		t.Fatalf("local selection escaped its closed boundary: %#v", local)
	}
}

func TestRunExecutionProfileRejectsTamperedAuthorityAndControlMapping(t *testing.T) {
	now := time.Now().UTC()
	mission := Mission{ID: "mission-profile-tamper", Profile: ProfileCode}
	run := Run{ID: "run-profile-tamper", MissionID: mission.ID}
	base, err := NewInitialRunExecutionProfileSnapshot("profile-tamper-one", run,
		mission, "test_operator", "closed profile", now)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*RunExecutionProfileSnapshot){
		func(value *RunExecutionProfileSnapshot) { value.ProcessEnabled = true },
		func(value *RunExecutionProfileSnapshot) { value.ExecutionAuthorized = true },
		func(value *RunExecutionProfileSnapshot) { value.CapabilityGrant = true },
		func(value *RunExecutionProfileSnapshot) { value.Backend = ExecutionBackendLocal },
		func(value *RunExecutionProfileSnapshot) { value.RequiredGate = ExecutionGateLocalOSSandbox },
	}
	for index, mutate := range mutations {
		candidate := base
		mutate(&candidate)
		if err := candidate.Validate(); err == nil {
			t.Fatalf("tampered execution profile %d validated: %#v", index, candidate)
		}
	}
}
