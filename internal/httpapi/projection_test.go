package httpapi

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

func TestProjectionViewsOmitPrivateLedgersNarrativesAndRawReports(t *testing.T) {
	now := time.Now().UTC()
	private := "private-operator-narrative"
	report := domain.FindingReport{
		ID: "report-projection-0001", RunID: "run-projection-0001",
		SourceKind: domain.FindingReportSourceReadOnlyFanoutExecution,
		SourceID:   "execution-projection-0001", Status: domain.FindingReportGenerated,
		Title: "Projection audit", FindingCount: 1, EvidenceCount: 1,
		Severity: domain.FindingSeveritySummary{High: 1}, Version: 2, CreatedAt: now,
		Findings: []domain.Finding{{
			ID: "finding-projection-0001", Ordinal: 1, Severity: domain.FindingSeverityHigh,
			Category: "boundary", Title: "Unsafe boundary", Detail: "bounded public detail",
			RelativePath: "src/main.go", LineStart: 7, LineEnd: 8, Confidence: 90,
			Evidence: []domain.FindingEvidence{{ID: "evidence-projection-0001",
				SourceID: "execution-projection-0001", SourceShard: 1, SourceOrdinal: 1,
				RelativePath: "src/main.go", LineStart: 7, LineEnd: 8, Confidence: 90,
				SourceDigest: private, SourceFingerprint: private}},
			ArtifactEvidence: []domain.FindingArtifactEvidence{{
				ID: "artifact-evidence-projection-0001", ArtifactID: "artifact-projection-0001",
				ArtifactSize: 42, ArtifactMIME: "text/plain", ArtifactStream: "stdout",
				AttachedBy: private, Note: private, ArtifactSHA256: private, CreatedAt: now}},
			Validation: &domain.FindingValidation{Status: domain.FindingStatusValidated,
				Reason: private, DecidedBy: private, ArtifactEvidenceCount: 1, CreatedAt: now},
		}},
	}
	encoded, err := json.Marshal(findingReportView(report))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{private, "source_digest", "source_fingerprint",
		"artifact_sha256", "attached_by", `"note"`, `"reason"`, `"decided_by"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("Finding projection exposed %q: %s", forbidden, encoded)
		}
	}

	execution := domain.ReadOnlyFanoutExecutionSummary{ID: "execution-projection-0001",
		Status: domain.ReadOnlyFanoutExecutionFailed, RequestedBy: "operator", StartedAt: now,
		UpdatedAt: now, Shards: []domain.ReadOnlyFanoutExecutionShardSummary{{Ordinal: 1,
			Status: domain.ReadOnlyFanoutExecutionShardFailed, ErrorCode: "invalid_response"}}}
	executionJSON, err := json.Marshal(fanoutExecutionView(execution))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"report_json", "report_digest", "input_digest", "error_reason"} {
		if strings.Contains(string(executionJSON), forbidden) {
			t.Fatalf("Fan-out projection exposed %q: %s", forbidden, executionJSON)
		}
	}

	nodeJSON, err := json.Marshal(agentNodeView(domain.AgentNode{ID: "agent-projection-0001",
		StatusReason: private, Skills: []string{}, CreatedAt: now, UpdatedAt: now}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(nodeJSON), private) || strings.Contains(string(nodeJSON), "status_reason") {
		t.Fatalf("Agent projection exposed private status reason: %s", nodeJSON)
	}
}
