package domain

import (
	"strings"
	"testing"
	"time"
)

func TestReadOnlyFanoutTiersRemainIndependentFromCoreAgentLimit(t *testing.T) {
	if MaxAgentChildren != 2 || MaxSpecialistDelegationAssignments != 2 {
		t.Fatalf("core delegation limit drifted: children=%d assignments=%d",
			MaxAgentChildren, MaxSpecialistDelegationAssignments)
	}
	for _, test := range []struct {
		tier  ReadOnlyFanoutTier
		files int
		want  int
	}{
		{ReadOnlyFanoutAuto, 1, 1},
		{ReadOnlyFanoutAuto, 8, 2},
		{ReadOnlyFanoutAuto, 9, 4},
		{ReadOnlyFanoutAuto, 33, 6},
		{ReadOnlyFanoutSix, 4, 4},
		{ReadOnlyFanoutSix, 12, 6},
	} {
		got, err := ResolveReadOnlyFanoutParallelism(test.tier, test.files)
		if err != nil || got != test.want {
			t.Fatalf("tier=%s files=%d got=%d want=%d err=%v",
				test.tier, test.files, got, test.want, err)
		}
	}
	if _, err := ParseReadOnlyFanoutTier("3"); err == nil {
		t.Fatal("unsupported parallelism tier was accepted")
	}
}

func TestReadOnlyFanoutPlanValidationBindsSnapshotAndSafeCapabilities(t *testing.T) {
	now := time.Now().UTC()
	planID := "fanout-plan-1"
	files := []ReadOnlyFanoutFile{
		{PlanID: planID, Ordinal: 1, ShardOrdinal: 1, RelativePath: "a.go",
			SizeBytes: 10, ContentSHA256: strings.Repeat("a", 64)},
		{PlanID: planID, Ordinal: 2, ShardOrdinal: 2, RelativePath: "b.go",
			SizeBytes: 20, ContentSHA256: strings.Repeat("b", 64)},
	}
	snapshot, err := ReadOnlyFanoutSnapshotDigest(files)
	if err != nil {
		t.Fatal(err)
	}
	capability, err := ReadOnlyFanoutCapabilityFingerprint(DefaultReadOnlyFanoutCapabilities())
	if err != nil {
		t.Fatal(err)
	}
	if capability != ReadOnlyFanoutCapabilityFingerprintV1 {
		t.Fatalf("read-only capability fingerprint drifted: %s", capability)
	}
	shards := make([]ReadOnlyFanoutShard, 2)
	for index := range shards {
		digest, err := ReadOnlyFanoutShardDigest(index+1, files)
		if err != nil {
			t.Fatal(err)
		}
		shards[index] = ReadOnlyFanoutShard{
			PlanID: planID, Ordinal: index + 1, Status: ReadOnlyFanoutShardPending,
			FileCount: 1, TotalBytes: files[index].SizeBytes, InputDigest: digest,
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
	}
	plan := ReadOnlyFanoutPlan{
		ID: planID, RunID: "run-1", WorkspaceID: "ws-1", ScopePath: ".",
		Goal: "review the selected source", ProtocolVersion: ReadOnlyFanoutProtocolVersion,
		RequestedTier: ReadOnlyFanoutTwo, EffectiveParallelism: 2,
		Status: ReadOnlyFanoutPlanned, CapabilityFingerprint: capability,
		SnapshotDigest: snapshot, FileCount: 2, TotalBytes: 30, ShardCount: 2,
		RequestedBy: "operator", Version: 1, CreatedAt: now, UpdatedAt: now,
		Files: files, Shards: shards,
	}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	tampered := plan
	tampered.Files = append([]ReadOnlyFanoutFile(nil), files...)
	tampered.Files[0].ContentSHA256 = strings.Repeat("c", 64)
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered snapshot was accepted")
	}
	unsafe := DefaultReadOnlyFanoutCapabilities()
	unsafe.Network = true
	if _, err := ReadOnlyFanoutCapabilityFingerprint(unsafe); err == nil {
		t.Fatal("network capability was accepted by the read-only envelope")
	}
}
