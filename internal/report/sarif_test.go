package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

func TestSARIFProjectionPreservesValidationTrustBoundary(t *testing.T) {
	report := validationProjectionFixture(t)
	first, err := Render(report, FormatSARIF)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(report, FormatSARIF)
	if err != nil || string(first) != string(second) {
		t.Fatalf("SARIF rendering drifted: err=%v", err)
	}
	var decoded sarifLog
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != SARIFSchemaURI || decoded.Version != SARIFVersion ||
		len(decoded.Runs) != 1 || len(decoded.Runs[0].Tool.Driver.Rules) != 5 ||
		len(decoded.Runs[0].Results) != 1 {
		t.Fatalf("unexpected SARIF envelope: %#v", decoded)
	}
	run := decoded.Runs[0]
	if run.AutomationDetails.ID != "cyberagent/"+report.ID ||
		run.Properties["cyberagentProjectionDigest"] != report.ProjectionDigest ||
		run.Properties["cyberagentDraftCount"] != float64(1) ||
		run.Properties["cyberagentValidatedCount"] != float64(1) ||
		run.Properties["cyberagentRejectedCount"] != float64(1) {
		t.Fatalf("SARIF run metadata drifted: %#v", run)
	}
	var validatedFinding domain.Finding
	for _, finding := range report.Findings {
		if effectiveFindingStatus(finding) == domain.FindingStatusValidated {
			validatedFinding = finding
		}
	}
	result := run.Results[0]
	if result.Properties["cyberagentValidationStatus"] != "validated" ||
		result.PartialFingerprints["primaryLocationLineHash"] != validatedFinding.Fingerprint ||
		result.PartialFingerprints["cyberagentFindingFingerprint"] != validatedFinding.Fingerprint {
		t.Fatalf("SARIF result crossed its validation boundary: %#v", result)
	}
	if result.Kind != "fail" || result.Level != "error" || len(result.Locations) != 1 ||
		strings.Contains(result.Locations[0].PhysicalLocation.ArtifactLocation.URI, `\`) ||
		result.Locations[0].PhysicalLocation.ArtifactLocation.URI !=
			"src/validated%20file%231.go" ||
		result.Locations[0].PhysicalLocation.Region == nil ||
		result.Locations[0].PhysicalLocation.Region.StartLine != 7 {
		t.Fatalf("validated SARIF result is incorrect: %#v", result)
	}
	text := string(first)
	for _, forbidden := range []string{
		"PRIVATE-SARIF-NOTE", "PRIVATE-SARIF-REASON", "PRIVATE-REJECTED-REASON",
		"cyberagentArtifactEvidenceDigest", "baselineState", "suppressions",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("SARIF leaked or fabricated %q: %s", forbidden, text)
		}
	}
}

func TestSARIFEmptyReportUsesStableEmptyResultArray(t *testing.T) {
	execution, _ := completedProjectionExecution(t, nil)
	projected, err := ProjectReadOnlyFanout(execution, nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := Render(projected, FormatSARIF)
	if err != nil {
		t.Fatal(err)
	}
	var decoded sarifLog
	if err := json.Unmarshal(encoded, &decoded); err != nil ||
		len(decoded.Runs) != 1 || decoded.Runs[0].Results == nil ||
		len(decoded.Runs[0].Results) != 0 || len(decoded.Runs[0].Tool.Driver.Rules) != 5 {
		t.Fatalf("empty SARIF projection is unstable: %#v err=%v", decoded, err)
	}
}

func TestReportGateRequiresExplicitDraftAdmission(t *testing.T) {
	report := validationProjectionFixture(t)
	tests := []struct {
		name    string
		policy  GatePolicy
		matched int
		passed  bool
	}{
		{name: "default validated high", policy: GatePolicy{
			FailStatus: GateStatusValidated, MinSeverity: domain.FindingSeverityHigh,
		}, matched: 1, passed: false},
		{name: "active includes draft", policy: GatePolicy{
			FailStatus: GateStatusActive, MinSeverity: domain.FindingSeverityHigh,
		}, matched: 2, passed: false},
		{name: "critical excludes draft high", policy: GatePolicy{
			FailStatus: GateStatusActive, MinSeverity: domain.FindingSeverityCritical,
		}, matched: 1, passed: false},
		{name: "disabled gate", policy: GatePolicy{
			FailStatus: GateStatusNone, MinSeverity: domain.FindingSeverityInfo,
		}, matched: 0, passed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := EvaluateGate(report, test.policy)
			if err != nil || result.MatchedCount != test.matched ||
				result.Passed != test.passed || result.DraftCount != 1 ||
				result.ValidatedCount != 1 || result.RejectedCount != 1 {
				t.Fatalf("unexpected gate result: %#v err=%v", result, err)
			}
		})
	}
	if _, err := ParseGateStatus("rejected"); err == nil {
		t.Fatal("unsupported rejected gate status was accepted")
	}
	if _, err := ParseGateSeverity("urgent"); err == nil {
		t.Fatal("unsupported gate severity was accepted")
	}
	if _, err := EvaluateGate(report, GatePolicy{
		FailStatus: " VALIDATED", MinSeverity: domain.FindingSeverityHigh,
	}); err == nil {
		t.Fatal("non-canonical gate status was accepted and could fail open")
	}
}

func validationProjectionFixture(t *testing.T) domain.FindingReport {
	t.Helper()
	sourceFindings := []domain.ReadOnlyFanoutFinding{
		{Severity: domain.ReadOnlyFindingCritical, Category: "security",
			Title: "Validated critical", Detail: "Validated source detail.",
			Path: "src/validated file#1.go", LineStart: 7, LineEnd: 8, Confidence: 90},
		{Severity: domain.ReadOnlyFindingHigh, Category: "correctness",
			Title: "Draft high", Detail: "Draft source detail.",
			Path: "src/draft.go", LineStart: 3, LineEnd: 3, Confidence: 70},
		{Severity: domain.ReadOnlyFindingMedium, Category: "quality",
			Title: "Rejected medium", Detail: "Rejected source detail.",
			Path: "src/rejected.go", Confidence: 60},
	}
	execution, reportDigest := completedProjectionExecution(t, sourceFindings)
	sources := make([]ReadOnlyFanoutSourceFinding, len(sourceFindings))
	for index, finding := range sourceFindings {
		sources[index] = ReadOnlyFanoutSourceFinding{
			ShardOrdinal: 1, Ordinal: index + 1,
			Fingerprint:  strings.Repeat(string(rune('a'+index)), 64),
			ReportDigest: reportDigest, Finding: finding,
		}
	}
	projected, err := ProjectReadOnlyFanout(execution, sources)
	if err != nil {
		t.Fatal(err)
	}
	sourceProjectionDigest := projected.ProjectionDigest
	createdAt := projected.CreatedAt.Add(time.Second)
	for index := range projected.Findings {
		finding := &projected.Findings[index]
		switch finding.Title {
		case "Validated critical":
			finding.ArtifactEvidence = []domain.FindingArtifactEvidence{{
				ID: "sarif-artifact-evidence", ReportID: projected.ID,
				FindingID: finding.ID, RunID: finding.RunID, Ordinal: 1,
				ArtifactID: "sarif-artifact", ArtifactSHA256: strings.Repeat("d", 64),
				ArtifactSize: 32, ArtifactMIME: "text/plain; charset=utf-8",
				ArtifactStream: "stdout", ArtifactTool: "shell",
				ArtifactSource: "sarif-tool", ArtifactRedacted: true,
				AttachedBy: "operator", Note: "PRIVATE-SARIF-NOTE", CreatedAt: createdAt,
			}}
			digest, err := domain.FindingArtifactEvidenceDigest(finding.ArtifactEvidence)
			if err != nil {
				t.Fatal(err)
			}
			finding.Validation = &domain.FindingValidation{
				ID: "sarif-validation", ReportID: projected.ID,
				FindingID: finding.ID, RunID: finding.RunID,
				FromStatus: domain.FindingStatusDraft, Status: domain.FindingStatusValidated,
				DecidedBy: "operator", Reason: "PRIVATE-SARIF-REASON",
				ArtifactEvidenceCount: 1, ArtifactEvidenceDigest: digest,
				Version: 1, CreatedAt: createdAt.Add(time.Second),
			}
		case "Rejected medium":
			digest, err := domain.FindingArtifactEvidenceDigest(finding.ArtifactEvidence)
			if err != nil {
				t.Fatal(err)
			}
			finding.Validation = &domain.FindingValidation{
				ID: "sarif-rejection", ReportID: projected.ID,
				FindingID: finding.ID, RunID: finding.RunID,
				FromStatus: domain.FindingStatusDraft, Status: domain.FindingStatusRejected,
				DecidedBy: "operator", Reason: "PRIVATE-REJECTED-REASON",
				ArtifactEvidenceDigest: digest, Version: 1, CreatedAt: createdAt,
			}
		}
	}
	if err := projected.Validate(); err != nil {
		t.Fatal(err)
	}
	if projected.ProjectionDigest != sourceProjectionDigest {
		t.Fatal("validation fixture changed its source projection digest")
	}
	return projected
}
