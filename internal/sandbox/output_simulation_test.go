package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestInMemoryOutputHarnessStagesRedactsAndCommitsAtomically(t *testing.T) {
	manifest := validManifest()
	plan, err := NewOutputExportPlan(manifest)
	if err != nil {
		t.Fatal(err)
	}
	fixture := OutputFixture{ProtocolVersion: OutputFixtureProtocolVersion, Outputs: []OutputFixtureItem{
		{Kind: OutputKindStdout, FileType: OutputFileTypeStream, Content: "ok\n"},
		{Kind: OutputKindStderr, FileType: OutputFileTypeStream,
			Content: "API_KEY=sk-123456789012345678901234567890\n"},
	}}
	result, err := NewInMemoryOutputHarness().Simulate(context.Background(), plan, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if result.FakeCommitCount != len(plan.Slots) || result.TotalBytes < 1 ||
		result.FixtureDigest == "" || result.TransactionDigest == "" ||
		!result.Descriptors[1].Redacted {
		t.Fatalf("output simulation did not stage a redacted atomic transaction: %#v", result)
	}

	failing := NewInMemoryOutputHarness()
	failing.FailCommitAtOrdinal = 2
	if _, err := failing.Simulate(context.Background(), plan, fixture); err == nil {
		t.Fatal("injected fake commit failure did not roll back")
	}
}

func TestInMemoryOutputHarnessRejectsLinksAndAggregateOverflow(t *testing.T) {
	manifest := validManifest()
	manifest.Mounts[0].Source = "src"
	manifest.Mounts = append(manifest.Mounts, Mount{
		Source: "outputs", Target: "/outputs", Access: MountReadWrite,
	})
	manifest.Output.Paths = []string{"/outputs/result.txt"}
	manifest.Resources.MaxOutputBytes = 16
	plan, err := NewOutputExportPlan(manifest)
	if err != nil {
		t.Fatal(err)
	}
	fixture := OutputFixture{ProtocolVersion: OutputFixtureProtocolVersion, Outputs: []OutputFixtureItem{
		{Kind: OutputKindStdout, FileType: OutputFileTypeStream, Content: "a"},
		{Kind: OutputKindStderr, FileType: OutputFileTypeStream, Content: "b"},
		{Kind: OutputKindFile, FileType: OutputFileTypeSymlink, Content: "c"},
	}}
	if _, err := NewInMemoryOutputHarness().Simulate(context.Background(), plan, fixture); err == nil {
		t.Fatal("symlink output fixture was accepted")
	}
	fixture.Outputs[2].FileType = OutputFileTypeRegular
	fixture.Outputs[2].Content = strings.Repeat("x", 32)
	if _, err := NewInMemoryOutputHarness().Simulate(context.Background(), plan, fixture); err == nil {
		t.Fatal("aggregate output overflow was accepted")
	}
}

func TestDecodeOutputFixtureRejectsDuplicateAndUnknownFields(t *testing.T) {
	valid := `{"protocol_version":"sandbox_output_fixture.v1","outputs":[{"kind":"stdout","file_type":"stream","content":"ok"}]}`
	if _, err := DecodeOutputFixture([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	duplicate := `{"protocol_version":"sandbox_output_fixture.v1","protocol_version":"sandbox_output_fixture.v1","outputs":[]}`
	if _, err := DecodeOutputFixture([]byte(duplicate)); err == nil {
		t.Fatal("duplicate output fixture field was accepted")
	}
	unknown := `{"protocol_version":"sandbox_output_fixture.v1","outputs":[],"command":"whoami"}`
	if _, err := DecodeOutputFixture([]byte(unknown)); err == nil {
		t.Fatal("unknown output fixture field was accepted")
	}
}
