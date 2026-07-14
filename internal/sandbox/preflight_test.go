package sandbox

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestDisabledBackendInspectorFreezesThreatModelWithoutPositiveClaims(t *testing.T) {
	handshake, err := NewDisabledBackendInspector().Inspect(context.Background(), BackendDocker)
	if err != nil {
		t.Fatal(err)
	}
	if handshake.Available || handshake.Status != PreflightStatusBackendDisabled ||
		handshake.Backend != BackendDocker || len(handshake.Checks) != MaxBackendChecks ||
		handshake.ContainerIdentity.Bound || handshake.ContainerIdentity.Fingerprint != "" {
		t.Fatalf("unexpected disabled backend handshake: %#v", handshake)
	}
	for index, check := range handshake.Checks {
		if check.Ordinal != index+1 || !check.Required || check.Verified ||
			check.EvidenceState != BackendCheckEvidenceNotProbed {
			t.Fatalf("backend check %d made an unsupported claim: %#v", index+1, check)
		}
	}
	if err := handshake.Validate(); err != nil {
		t.Fatal(err)
	}
	handshake.Checks[0].Verified = true
	if err := handshake.Validate(); err == nil {
		t.Fatal("disabled backend handshake accepted a forged verification claim")
	}
}

func TestOutputExportPlanUsesOpaqueLocatorsAndAtomicDisabledPolicy(t *testing.T) {
	manifest := validManifest()
	manifest.Mounts[0].Access = MountReadWrite
	manifest.Output.Paths = []string{"/workspace/results/report.json", "/workspace/results/log.txt"}
	plan, err := NewOutputExportPlan(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if plan.SlotCount != 4 || plan.PartialFailurePolicy != OutputPartialFailureAllOrNothing ||
		plan.TruncationPolicy != OutputTruncationAggregateHardCap || plan.RawPathsStored ||
		plan.ExportEnabled || plan.ArtifactCommitAuthorized {
		t.Fatalf("unexpected output export plan: %#v", plan)
	}
	if plan.Slots[0].Kind != OutputKindStdout || plan.Slots[1].Kind != OutputKindStderr {
		t.Fatalf("stream slots are not stable: %#v", plan.Slots)
	}
	for _, slot := range plan.Slots[2:] {
		if slot.Kind != OutputKindFile || !slot.RegularFileRequired ||
			!slot.SymlinkRejected || !slot.SpecialFileRejected ||
			slot.ArtifactCommitAuthorized {
			t.Fatalf("file slot is not fail closed: %#v", slot)
		}
	}
	representation := fmt.Sprintf("%#v", plan)
	for _, rawPath := range manifest.Output.Paths {
		if strings.Contains(representation, rawPath) {
			t.Fatalf("output plan retained raw path %q", rawPath)
		}
	}
	changed := manifest
	changed.Output.Paths = []string{"/workspace/results/report.json", "/workspace/results/other.txt"}
	changedPlan, err := NewOutputExportPlan(changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedPlan.Fingerprint == plan.Fingerprint {
		t.Fatal("changed output locator did not change the export-plan fingerprint")
	}
	plan.Slots[2].ArtifactCommitAuthorized = true
	if err := plan.Validate(); err == nil {
		t.Fatal("output plan accepted Artifact commit authorization")
	}
}
