package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"cyberagent-workbench/internal/domain"
)

func TestGitHubAnnotationsUseTheValidatedGateProjection(t *testing.T) {
	result, err := EvaluateGate(validationProjectionFixture(t), GatePolicy{
		FailStatus: GateStatusValidated, MinSeverity: domain.FindingSeverityHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := RenderGitHubAnnotations(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if result.MatchedCount != 2 || len(result.Matches) != 2 ||
		strings.Count(text, "\n") != 2 ||
		!strings.Contains(text, "::error file=src/validated file#1.go,line=7,endLine=8,") ||
		!strings.Contains(text, "title=CyberAgent CRITICAL%3A Validated critical") ||
		!strings.Contains(text, "status=validated; category=security;") ||
		!strings.Contains(text, "file=src/accepted.go,line=5,endLine=5,") ||
		!strings.Contains(text, "status=accepted; category=security;") {
		t.Fatalf("GitHub annotations drifted from GateResult: result=%#v output=%s",
			result, text)
	}
	for _, forbidden := range []string{
		"PRIVATE-SARIF-NOTE", "PRIVATE-SARIF-REASON",
		"PRIVATE-ACCEPTANCE-REASON", "PRIVATE-REMEDIATION-NOTE",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("GitHub annotation leaked private lifecycle text %q: %s",
				forbidden, text)
		}
	}
	jsonResult, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(jsonResult), "matches") ||
		strings.Contains(string(jsonResult), "Validated critical") ||
		strings.Contains(string(jsonResult), "src/validated") {
		t.Fatalf("GateResult JSON compatibility leaked annotation details: %s", jsonResult)
	}
}

func TestGitHubAnnotationsEscapeWorkflowCommandInjection(t *testing.T) {
	result, err := EvaluateGate(validationProjectionFixture(t), GatePolicy{
		FailStatus: GateStatusActive, MinSeverity: domain.FindingSeverityCritical,
	})
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("critical GateResult fixture drifted: %#v err=%v", result, err)
	}
	result.Matches[0].RelativePath = "src/a,b:%\n::error.go"
	result.Matches[0].Title = "Bad,title:100%\nnext"
	result.Matches[0].Detail = "first%\r\n::warning file=escape.go::injected\x1b[31m"
	encoded, err := RenderGitHubAnnotations(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Count(text, "\n") != 1 || strings.Contains(text, "\r") ||
		!strings.HasPrefix(text, "::error file=src/a%2Cb%3A%25%0A%3A%3Aerror.go,") ||
		!strings.Contains(text,
			"title=CyberAgent CRITICAL%3A Bad%2Ctitle%3A100%25%0Anext::") ||
		!strings.Contains(text,
			"first%25%0D%0A::warning file=escape.go::injected\\u001B[31m") ||
		strings.ContainsRune(text, '\x1b') {
		t.Fatalf("GitHub workflow command escaping is unsafe: %q", text)
	}
	changed := result
	changed.MatchedCount++
	if _, err := RenderGitHubAnnotations(changed); err == nil {
		t.Fatal("inconsistent GateResult was rendered")
	}
	changed = result
	changed.Matches[0].Status = domain.FindingStatusFixed
	if _, err := RenderGitHubAnnotations(changed); err == nil {
		t.Fatal("non-matching lifecycle state was rendered")
	}
}

func TestGitHubAnnotationSeverityAndPassingOutput(t *testing.T) {
	tests := []struct {
		severity domain.FindingSeverity
		command  string
	}{
		{domain.FindingSeverityInfo, "notice"},
		{domain.FindingSeverityLow, "notice"},
		{domain.FindingSeverityMedium, "warning"},
		{domain.FindingSeverityHigh, "error"},
		{domain.FindingSeverityCritical, "error"},
	}
	for _, test := range tests {
		command, err := githubAnnotationCommand(test.severity)
		if err != nil || command != test.command {
			t.Fatalf("severity %s mapped to %q: %v", test.severity, command, err)
		}
	}
	result, err := EvaluateGate(validationProjectionFixture(t), GatePolicy{
		FailStatus: GateStatusNone, MinSeverity: domain.FindingSeverityInfo,
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := RenderGitHubAnnotations(result)
	if err != nil || len(encoded) != 0 || !result.Passed {
		t.Fatalf("passing GateResult emitted annotations: %q result=%#v err=%v",
			encoded, result, err)
	}
}

func TestGitHubAnnotationsHonorTheReportFindingBound(t *testing.T) {
	result := GateResult{
		ReportID: "report-" + strings.Repeat("a", 64), RunID: "run-bounded",
		ProjectionDigest: strings.Repeat("b", 64),
		Policy: GatePolicy{
			FailStatus: GateStatusActive, MinSeverity: domain.FindingSeverityInfo,
		},
		FindingCount: domain.MaxFindingReportFindings,
		DraftCount:   domain.MaxFindingReportFindings,
		MatchedCount: domain.MaxFindingReportFindings,
		Passed:       false,
	}
	for index := range domain.MaxFindingReportFindings {
		result.Matches = append(result.Matches, GateMatch{
			FindingID:   fmt.Sprintf("finding-%03d", index+1),
			Fingerprint: fmt.Sprintf("%064x", index+1),
			Status:      domain.FindingStatusDraft, Severity: domain.FindingSeverityInfo,
			Category: "quality", Title: fmt.Sprintf("Bounded finding %d", index+1),
			Detail:       "Bounded annotation detail.",
			RelativePath: fmt.Sprintf("src/file-%03d.go", index+1),
		})
	}
	encoded, err := RenderGitHubAnnotations(result)
	if err != nil || strings.Count(string(encoded), "\n") != domain.MaxFindingReportFindings ||
		strings.Contains(string(encoded), ",line=") {
		t.Fatalf("bounded no-line annotations drifted: bytes=%d err=%v", len(encoded), err)
	}
	result.FindingCount++
	result.DraftCount++
	result.MatchedCount++
	result.Matches = append(result.Matches, GateMatch{
		FindingID: "finding-over-limit", Fingerprint: strings.Repeat("c", 64),
		Status: domain.FindingStatusDraft, Severity: domain.FindingSeverityInfo,
		Category: "quality", Title: "Over limit", Detail: "Must be rejected.",
		RelativePath: "src/over-limit.go",
	})
	if _, err := RenderGitHubAnnotations(result); err == nil {
		t.Fatal("GateResult above the report Finding bound was rendered")
	}
}
