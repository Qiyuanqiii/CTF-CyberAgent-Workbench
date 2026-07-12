package report

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

func TestProjectReadOnlyFanoutDeduplicatesExactFactsConservatively(t *testing.T) {
	findings := []domain.ReadOnlyFanoutFinding{
		{
			Severity: domain.ReadOnlyFindingHigh, Category: "correctness",
			Title: "Unchecked boundary", Detail: "The input boundary is unchecked.",
			Path: "src/module-a.go", LineStart: 7, LineEnd: 9, Confidence: 90,
		},
		{
			Severity: domain.ReadOnlyFindingHigh, Category: "correctness",
			Title: "Unchecked boundary", Detail: "The input boundary is unchecked.",
			Path: "src/module-a.go", LineStart: 7, LineEnd: 9, Confidence: 40,
		},
	}
	execution, reportDigest := completedProjectionExecution(t, findings)
	sources := []ReadOnlyFanoutSourceFinding{
		{ShardOrdinal: 1, Ordinal: 1, Fingerprint: strings.Repeat("a", 64),
			ReportDigest: reportDigest, Finding: findings[0]},
		{ShardOrdinal: 1, Ordinal: 2, Fingerprint: strings.Repeat("b", 64),
			ReportDigest: reportDigest, Finding: findings[1]},
	}
	projected, err := ProjectReadOnlyFanout(execution, sources)
	if err != nil {
		t.Fatal(err)
	}
	if projected.FindingCount != 1 || projected.EvidenceCount != 2 ||
		projected.Severity.High != 1 || len(projected.Findings) != 1 ||
		projected.Findings[0].Severity != domain.FindingSeverityHigh ||
		projected.Findings[0].Confidence != 40 ||
		len(projected.Findings[0].Evidence) != 2 {
		t.Fatalf("unexpected conservative projection: %#v", projected)
	}
	reversed := []ReadOnlyFanoutSourceFinding{sources[1], sources[0]}
	replayed, err := ProjectReadOnlyFanout(execution, reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, replayed) {
		t.Fatalf("projection depends on source order:\nfirst=%#v\nsecond=%#v",
			projected, replayed)
	}
	tampered := projected
	tampered.Severity.High = 0
	tampered.Severity.Critical = 1
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered severity summary passed validation")
	}
}

func TestProjectReadOnlyFanoutDoesNotMergeDifferentSeverity(t *testing.T) {
	findings := []domain.ReadOnlyFanoutFinding{
		{Severity: domain.ReadOnlyFindingHigh, Category: "security", Title: "Risk",
			Detail: "Same claimed fact.", Path: "src/module-a.go", Confidence: 80},
		{Severity: domain.ReadOnlyFindingLow, Category: "security", Title: "Risk",
			Detail: "Same claimed fact.", Path: "src/module-a.go", Confidence: 80},
	}
	execution, reportDigest := completedProjectionExecution(t, findings)
	projected, err := ProjectReadOnlyFanout(execution, []ReadOnlyFanoutSourceFinding{
		{ShardOrdinal: 1, Ordinal: 1, Fingerprint: strings.Repeat("c", 64),
			ReportDigest: reportDigest, Finding: findings[0]},
		{ShardOrdinal: 1, Ordinal: 2, Fingerprint: strings.Repeat("d", 64),
			ReportDigest: reportDigest, Finding: findings[1]},
	})
	if err != nil {
		t.Fatal(err)
	}
	if projected.FindingCount != 2 || projected.Severity.High != 1 ||
		projected.Severity.Low != 1 ||
		projected.Findings[0].Severity != domain.FindingSeverityHigh {
		t.Fatalf("severity facts were merged or reordered incorrectly: %#v", projected)
	}
}

func TestRenderFindingReportIsDeterministicAndEscapesMarkdown(t *testing.T) {
	findings := []domain.ReadOnlyFanoutFinding{{
		Severity: domain.ReadOnlyFindingMedium, Category: "markup|test",
		Title: "# injected [heading]", Detail: "</blockquote>\n# forged heading",
		Path: "src/mod`ule.go", LineStart: 1, LineEnd: 2, Confidence: 55,
	}}
	execution, reportDigest := completedProjectionExecution(t, findings)
	projected, err := ProjectReadOnlyFanout(execution, []ReadOnlyFanoutSourceFinding{{
		ShardOrdinal: 1, Ordinal: 1, Fingerprint: strings.Repeat("e", 64),
		ReportDigest: reportDigest, Finding: findings[0],
	}})
	if err != nil {
		t.Fatal(err)
	}
	markdownOne, err := Render(projected, FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	markdownTwo, err := Render(projected, FormatMarkdown)
	if err != nil || string(markdownOne) != string(markdownTwo) {
		t.Fatalf("Markdown rendering drifted: err=%v", err)
	}
	text := string(markdownOne)
	if strings.Contains(text, "\n# forged heading") ||
		strings.Contains(text, "</blockquote>") ||
		!strings.Contains(text, "\\# injected \\[heading\\]") ||
		!strings.Contains(text, "&lt;/blockquote&gt;") ||
		!strings.Contains(text, "`` src/mod`ule.go:1-2 ``") {
		t.Fatalf("Markdown renderer did not neutralize model markup:\n%s", text)
	}
	encoded, err := Render(projected, FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var decoded domain.FindingReport
	if err := json.Unmarshal(encoded, &decoded); err != nil ||
		decoded.ProjectionDigest != projected.ProjectionDigest {
		t.Fatalf("JSON report is invalid: err=%v value=%#v", err, decoded)
	}
}

func TestFindingValidationOverlayPreservesSourceProjectionAndEscapesNarrative(t *testing.T) {
	findings := []domain.ReadOnlyFanoutFinding{{
		Severity: domain.ReadOnlyFindingHigh, Category: "security",
		Title: "Validated boundary", Detail: "A bounded claim.",
		Path: "src/module-a.go", LineStart: 3, LineEnd: 3, Confidence: 80,
	}}
	execution, reportDigest := completedProjectionExecution(t, findings)
	projected, err := ProjectReadOnlyFanout(execution, []ReadOnlyFanoutSourceFinding{{
		ShardOrdinal: 1, Ordinal: 1, Fingerprint: strings.Repeat("f", 64),
		ReportDigest: reportDigest, Finding: findings[0],
	}})
	if err != nil {
		t.Fatal(err)
	}
	sourceProjectionDigest := projected.ProjectionDigest
	createdAt := projected.Findings[0].CreatedAt.Add(time.Second)
	projected.Findings[0].ArtifactEvidence = []domain.FindingArtifactEvidence{{
		ID: "artifact-evidence-render", ReportID: projected.ID,
		FindingID: projected.Findings[0].ID, RunID: projected.RunID, Ordinal: 1,
		ArtifactID: "artifact-render", ArtifactSHA256: strings.Repeat("a", 64),
		ArtifactSize: 24, ArtifactMIME: "text/plain; charset=utf-8",
		ArtifactStream: "stdout", ArtifactTool: "shell",
		ArtifactSource: "tool-render", ArtifactRedacted: true, AttachedBy: "operator",
		Note: "confirmed | evidence <tag>", CreatedAt: createdAt,
	}}
	evidenceDigest, err := domain.FindingArtifactEvidenceDigest(
		projected.Findings[0].ArtifactEvidence)
	if err != nil {
		t.Fatal(err)
	}
	projected.Findings[0].Validation = &domain.FindingValidation{
		ID: "finding-validation-render", ReportID: projected.ID,
		FindingID: projected.Findings[0].ID, RunID: projected.RunID,
		FromStatus: domain.FindingStatusDraft, Status: domain.FindingStatusValidated,
		DecidedBy: "operator", Reason: "verified # decision </section>",
		ArtifactEvidenceCount: 1, ArtifactEvidenceDigest: evidenceDigest,
		Version: 1, CreatedAt: createdAt.Add(time.Second),
	}
	if err := projected.Validate(); err != nil {
		t.Fatal(err)
	}
	recomputed, err := domain.FindingReportProjectionDigest(projected)
	if err != nil || recomputed != sourceProjectionDigest {
		t.Fatalf("validation changed source projection digest: got=%s want=%s err=%v",
			recomputed, sourceProjectionDigest, err)
	}
	encoded, err := Render(projected, FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, expected := range []string{
		"- Validated: 1", "- Artifact Evidence records: 1", "- Status: `validated`",
		"Model assertion evidence:", "Artifact Evidence:",
		"confirmed \\| evidence &lt;tag&gt;", "verified \\# decision &lt;/section&gt;",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("validation report is missing %q:\n%s", expected, text)
		}
	}
}

func completedProjectionExecution(t *testing.T,
	findings []domain.ReadOnlyFanoutFinding,
) (domain.ReadOnlyFanoutExecution, string) {
	t.Helper()
	allowed := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		allowed[finding.Path] = struct{}{}
	}
	if len(allowed) == 0 {
		allowed["src/empty.go"] = struct{}{}
	}
	reportJSON, err := domain.EncodeReadOnlyFanoutReport(domain.ReadOnlyFanoutReport{
		Version: domain.ReadOnlyFanoutReportVersion, Summary: "Bounded source audit.",
		Findings: findings,
	}, allowed)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := domain.ReadOnlyFanoutReportDigest(reportJSON)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	started := created.Add(time.Second)
	finished := started.Add(2 * time.Second)
	shard := domain.ReadOnlyFanoutExecutionShard{
		ExecutionID: "fanout-execution-projection", PlanID: "fanout-plan-projection",
		Ordinal: 1, Status: domain.ReadOnlyFanoutExecutionShardCompleted,
		InputDigest: strings.Repeat("1", 64), AttemptCount: 1, CurrentAttempt: 1,
		Provider: "mock", Model: "mock-1", InputTokens: 10, OutputTokens: 10,
		TotalTokens: 20, ElapsedMillis: 2000, ReportJSON: reportJSON,
		ReportDigest: digest, FindingCount: len(findings), Version: 2,
		CreatedAt: created, UpdatedAt: finished, StartedAt: &started, FinishedAt: &finished,
	}
	execution := domain.ReadOnlyFanoutExecution{
		ID: "fanout-execution-projection", PlanID: "fanout-plan-projection",
		RunID: "run-projection", WorkspaceID: "workspace-projection",
		Status: domain.ReadOnlyFanoutExecutionCompleted, Parallelism: 1,
		MaxOutputTokensPerShard: 512, SnapshotDigest: strings.Repeat("2", 64),
		RequestedBy: "operator", Version: 2, StartedAt: created,
		UpdatedAt: finished, FinishedAt: &finished,
		Shards: []domain.ReadOnlyFanoutExecutionShard{shard},
	}
	if err := execution.Validate(); err != nil {
		t.Fatal(err)
	}
	return execution, digest
}
