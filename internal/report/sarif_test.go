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
		len(decoded.Runs[0].Results) != 2 {
		t.Fatalf("unexpected SARIF envelope: %#v", decoded)
	}
	run := decoded.Runs[0]
	if run.AutomationDetails.ID != "cyberagent/"+report.ID ||
		run.Properties["cyberagentProjectionDigest"] != report.ProjectionDigest ||
		run.Properties["cyberagentDraftCount"] != float64(1) ||
		run.Properties["cyberagentValidatedCount"] != float64(1) ||
		run.Properties["cyberagentAcceptedCount"] != float64(1) ||
		run.Properties["cyberagentFixedCount"] != float64(1) ||
		run.Properties["cyberagentRejectedCount"] != float64(1) {
		t.Fatalf("SARIF run metadata drifted: %#v", run)
	}
	var validatedFinding domain.Finding
	for _, finding := range report.Findings {
		if effectiveFindingStatus(finding) == domain.FindingStatusValidated {
			validatedFinding = finding
		}
	}
	var result sarifResult
	confirmedStatuses := map[string]int{}
	for _, candidate := range run.Results {
		status, _ := candidate.Properties["cyberagentFindingStatus"].(string)
		confirmedStatuses[status]++
		if status == string(domain.FindingStatusValidated) {
			result = candidate
		}
	}
	if confirmedStatuses[string(domain.FindingStatusValidated)] != 1 ||
		confirmedStatuses[string(domain.FindingStatusAccepted)] != 1 ||
		confirmedStatuses[string(domain.FindingStatusFixed)] != 0 {
		t.Fatalf("SARIF unresolved lifecycle projection drifted: %#v", confirmedStatuses)
	}
	if result.Properties["cyberagentValidationStatus"] != "validated" ||
		result.Properties["cyberagentFindingStatus"] != "validated" ||
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
		"PRIVATE-ACCEPTANCE-REASON", "PRIVATE-REMEDIATION-NOTE",
		"PRIVATE-FIX-REASON",
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
		}, matched: 2, passed: false},
		{name: "active includes draft", policy: GatePolicy{
			FailStatus: GateStatusActive, MinSeverity: domain.FindingSeverityHigh,
		}, matched: 3, passed: false},
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
				result.ValidatedCount != 1 || result.AcceptedCount != 1 ||
				result.FixedCount != 1 || result.RejectedCount != 1 {
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
		{Severity: domain.ReadOnlyFindingHigh, Category: "security",
			Title: "Accepted high", Detail: "Accepted unresolved detail.",
			Path: "src/accepted.go", LineStart: 5, LineEnd: 5, Confidence: 85},
		{Severity: domain.ReadOnlyFindingCritical, Category: "security",
			Title: "Fixed critical", Detail: "Fixed source detail.",
			Path: "src/fixed.go", LineStart: 9, LineEnd: 9, Confidence: 95},
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
			addValidatedFindingOverlay(t, projected.ID, finding, "sarif", createdAt,
				"PRIVATE-SARIF-NOTE", "PRIVATE-SARIF-REASON")
		case "Accepted high":
			addValidatedFindingOverlay(t, projected.ID, finding, "accepted", createdAt,
				"accepted Evidence", "accepted validation")
			finding.Acceptance = &domain.FindingAcceptance{
				ID: "accepted-decision", ReportID: projected.ID,
				FindingID: finding.ID, RunID: finding.RunID,
				FromStatus: domain.FindingStatusValidated, Status: domain.FindingStatusAccepted,
				ValidationID:                     finding.Validation.ID,
				ValidationArtifactEvidenceCount:  finding.Validation.ArtifactEvidenceCount,
				ValidationArtifactEvidenceDigest: finding.Validation.ArtifactEvidenceDigest,
				DecidedBy:                        "operator", Reason: "PRIVATE-ACCEPTANCE-REASON",
				Version: 1, CreatedAt: createdAt.Add(2 * time.Second),
			}
		case "Fixed critical":
			addValidatedFindingOverlay(t, projected.ID, finding, "fixed", createdAt,
				"fixed validation Evidence", "fixed validation")
			finding.Acceptance = &domain.FindingAcceptance{
				ID: "fixed-acceptance", ReportID: projected.ID,
				FindingID: finding.ID, RunID: finding.RunID,
				FromStatus: domain.FindingStatusValidated, Status: domain.FindingStatusAccepted,
				ValidationID:                     finding.Validation.ID,
				ValidationArtifactEvidenceCount:  finding.Validation.ArtifactEvidenceCount,
				ValidationArtifactEvidenceDigest: finding.Validation.ArtifactEvidenceDigest,
				DecidedBy:                        "operator", Reason: "fixed acceptance",
				Version: 1, CreatedAt: createdAt.Add(2 * time.Second),
			}
			finding.RemediationEvidence = []domain.FindingArtifactEvidence{{
				ID: "fixed-remediation-evidence", ReportID: projected.ID,
				FindingID: finding.ID, RunID: finding.RunID, Ordinal: 1,
				ArtifactID:     "fixed-remediation-artifact",
				ArtifactSHA256: strings.Repeat("e", 64), ArtifactSize: 48,
				ArtifactMIME: "text/plain; charset=utf-8", ArtifactStream: "stdout",
				ArtifactTool: "shell", ArtifactSource: "fixed-remediation-tool",
				ArtifactRedacted: true, AttachedBy: "operator",
				Note: "PRIVATE-REMEDIATION-NOTE", CreatedAt: createdAt.Add(3 * time.Second),
			}}
			remediationDigest, err := domain.FindingRemediationEvidenceDigest(
				finding.RemediationEvidence)
			if err != nil {
				t.Fatal(err)
			}
			finding.Fix = &domain.FindingFix{
				ID: "fixed-decision", ReportID: projected.ID,
				FindingID: finding.ID, RunID: finding.RunID,
				AcceptanceID: finding.Acceptance.ID,
				FromStatus:   domain.FindingStatusAccepted, Status: domain.FindingStatusFixed,
				RemediationEvidenceCount: 1, RemediationEvidenceDigest: remediationDigest,
				DecidedBy: "operator", Reason: "PRIVATE-FIX-REASON",
				Version: 1, CreatedAt: createdAt.Add(4 * time.Second),
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

func addValidatedFindingOverlay(t *testing.T, reportID string,
	finding *domain.Finding, prefix string, createdAt time.Time, note, reason string,
) {
	t.Helper()
	finding.ArtifactEvidence = []domain.FindingArtifactEvidence{{
		ID: prefix + "-artifact-evidence", ReportID: reportID,
		FindingID: finding.ID, RunID: finding.RunID, Ordinal: 1,
		ArtifactID: prefix + "-artifact", ArtifactSHA256: strings.Repeat("d", 64),
		ArtifactSize: 32, ArtifactMIME: "text/plain; charset=utf-8",
		ArtifactStream: "stdout", ArtifactTool: "shell",
		ArtifactSource: prefix + "-tool", ArtifactRedacted: true,
		AttachedBy: "operator", Note: note, CreatedAt: createdAt,
	}}
	digest, err := domain.FindingArtifactEvidenceDigest(finding.ArtifactEvidence)
	if err != nil {
		t.Fatal(err)
	}
	finding.Validation = &domain.FindingValidation{
		ID: prefix + "-validation", ReportID: reportID,
		FindingID: finding.ID, RunID: finding.RunID,
		FromStatus: domain.FindingStatusDraft, Status: domain.FindingStatusValidated,
		DecidedBy: "operator", Reason: reason,
		ArtifactEvidenceCount: 1, ArtifactEvidenceDigest: digest,
		Version: 1, CreatedAt: createdAt.Add(time.Second),
	}
}

func TestMarkdownRendersFullFindingLifecycle(t *testing.T) {
	report := validationProjectionFixture(t)
	encoded, err := Render(report, FormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, expected := range []string{
		"- Draft: 1", "- Validated: 1", "- Accepted: 1", "- Fixed: 1",
		"- Rejected: 1", "Acceptance decision:",
		"Remediation Artifact Evidence:", "Fix decision:",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("Markdown omitted lifecycle text %q: %s", expected, text)
		}
	}
}
