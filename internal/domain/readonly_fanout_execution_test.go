package domain

import (
	"strings"
	"testing"
	"time"
)

func TestReadOnlyFanoutReportIsStrictAndShardScoped(t *testing.T) {
	allowed := map[string]struct{}{"internal/a.go": {}}
	text := `{"version":"readonly_fanout_report.v1","summary":"reviewed shard","findings":[{"severity":"medium","category":"correctness","title":"unchecked error","detail":"the returned error is ignored","path":"internal/a.go","line_start":10,"line_end":10,"confidence":90}]}`
	report, err := DecodeReadOnlyFanoutReport(text, allowed)
	if err != nil || len(report.Findings) != 1 {
		t.Fatalf("strict report did not decode: %#v err=%v", report, err)
	}
	if _, err := ReadOnlyFanoutFindingFingerprint("fanout-execution-1", 1,
		report.Findings[0]); err != nil {
		t.Fatal(err)
	}
	outside := strings.Replace(text, "internal/a.go", "outside.go", 1)
	if _, err := DecodeReadOnlyFanoutReport(outside, allowed); err == nil {
		t.Fatal("finding outside shard was accepted")
	}
	unknown := strings.Replace(text, `"summary":"reviewed shard"`,
		`"summary":"reviewed shard","tools":[]`, 1)
	if _, err := DecodeReadOnlyFanoutReport(unknown, allowed); err == nil {
		t.Fatal("unknown report field was accepted")
	}
	encoded, err := EncodeReadOnlyFanoutReport(report, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if digest, err := ReadOnlyFanoutReportDigest(encoded); err != nil || len(digest) != 64 {
		t.Fatalf("report digest failed: digest=%q err=%v", digest, err)
	}
}

func TestReadOnlyFanoutExecutionAllowsCancellationBeforeModelStart(t *testing.T) {
	now := time.Now().UTC()
	finished := now.Add(time.Second)
	shard := ReadOnlyFanoutExecutionShard{
		ExecutionID: "fanout-execution-1", PlanID: "fanout-plan-1", Ordinal: 1,
		Status:      ReadOnlyFanoutExecutionShardCancelled,
		InputDigest: strings.Repeat("a", 64), ErrorCode: "cancelled",
		ErrorReason: "cancelled before model start", Version: 2,
		CreatedAt: now, UpdatedAt: finished, FinishedAt: &finished,
	}
	if err := shard.Validate(); err != nil {
		t.Fatal(err)
	}
}
