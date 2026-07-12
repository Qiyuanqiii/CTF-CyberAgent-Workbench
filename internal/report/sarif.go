package report

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"cyberagent-workbench/internal/domain"
)

const (
	SARIFVersion   = "2.1.0"
	SARIFSchemaURI = "https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json"
)

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool              sarifTool              `json:"tool"`
	AutomationDetails sarifAutomationDetails `json:"automationDetails"`
	Results           []sarifResult          `json:"results"`
	Properties        map[string]any         `json:"properties"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID                   string             `json:"id"`
	Name                 string             `json:"name"`
	ShortDescription     sarifMessage       `json:"shortDescription"`
	FullDescription      sarifMessage       `json:"fullDescription"`
	DefaultConfiguration sarifConfiguration `json:"defaultConfiguration"`
	Properties           map[string]any     `json:"properties"`
}

type sarifConfiguration struct {
	Level string `json:"level"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifAutomationDetails struct {
	ID string `json:"id"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Kind                string            `json:"kind"`
	Level               string            `json:"level"`
	Message             sarifMessage      `json:"message"`
	Locations           []sarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Properties          map[string]any    `json:"properties"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine,omitempty"`
}

var sarifSeverityOrder = []domain.FindingSeverity{
	domain.FindingSeverityInfo,
	domain.FindingSeverityLow,
	domain.FindingSeverityMedium,
	domain.FindingSeverityHigh,
	domain.FindingSeverityCritical,
}

func renderSARIF(value domain.FindingReport) ([]byte, error) {
	rules := make([]sarifRule, 0, len(sarifSeverityOrder))
	ruleIndexes := make(map[domain.FindingSeverity]int, len(sarifSeverityOrder))
	for index, severity := range sarifSeverityOrder {
		ruleIndexes[severity] = index
		rules = append(rules, newSARIFRule(severity))
	}
	results := make([]sarifResult, 0, len(value.Findings))
	statusCounts := map[domain.FindingStatus]int{
		domain.FindingStatusDraft:     0,
		domain.FindingStatusValidated: 0,
		domain.FindingStatusAccepted:  0,
		domain.FindingStatusFixed:     0,
		domain.FindingStatusRejected:  0,
	}
	for _, finding := range value.Findings {
		status := effectiveFindingStatus(finding)
		statusCounts[status]++
		if !confirmedUnresolvedFindingStatus(status) {
			continue
		}
		result, err := newSARIFResult(value, finding, ruleIndexes[finding.Severity])
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	log := sarifLog{
		Schema: SARIFSchemaURI, Version: SARIFVersion,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "CyberAgent Workbench",
				InformationURI: "https://github.com/Qiyuanqiii/CTF-CyberAgent-Workbench",
				Rules:          rules,
			}},
			AutomationDetails: sarifAutomationDetails{ID: "cyberagent/" + value.ID},
			Results:           results,
			Properties: map[string]any{
				"cyberagentReportId":         value.ID,
				"cyberagentRunId":            value.RunID,
				"cyberagentSourceKind":       value.SourceKind,
				"cyberagentSourceId":         value.SourceID,
				"cyberagentProjectionDigest": value.ProjectionDigest,
				"cyberagentDraftCount":       statusCounts[domain.FindingStatusDraft],
				"cyberagentValidatedCount":   statusCounts[domain.FindingStatusValidated],
				"cyberagentAcceptedCount":    statusCounts[domain.FindingStatusAccepted],
				"cyberagentFixedCount":       statusCounts[domain.FindingStatusFixed],
				"cyberagentRejectedCount":    statusCounts[domain.FindingStatusRejected],
			},
		}},
	}
	encoded, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func newSARIFRule(severity domain.FindingSeverity) sarifRule {
	name := strings.ToUpper(string(severity))
	return sarifRule{
		ID:                   "CYBERAGENT." + name,
		Name:                 "CyberAgent" + strings.ToUpper(string(severity[:1])) + string(severity[1:]) + "Finding",
		ShortDescription:     sarifMessage{Text: "CyberAgent " + string(severity) + " finding"},
		FullDescription:      sarifMessage{Text: "A finding projected by CyberAgent Workbench with source severity " + string(severity) + "."},
		DefaultConfiguration: sarifConfiguration{Level: sarifLevel(severity)},
		Properties: map[string]any{
			"tags":                     []string{"cyberagent", "severity-" + string(severity)},
			"cyberagentSourceSeverity": severity,
		},
	}
}

func newSARIFResult(report domain.FindingReport, finding domain.Finding,
	ruleIndex int,
) (sarifResult, error) {
	status := effectiveFindingStatus(finding)
	if !confirmedUnresolvedFindingStatus(status) {
		return sarifResult{}, errors.New(
			"only confirmed unresolved findings can be projected as SARIF results")
	}
	location := sarifPhysicalLocation{
		ArtifactLocation: sarifArtifactLocation{URI: sarifURI(finding.RelativePath)},
	}
	if finding.LineStart > 0 {
		location.Region = &sarifRegion{StartLine: finding.LineStart, EndLine: finding.LineEnd}
	}
	return sarifResult{
		RuleID:    "CYBERAGENT." + strings.ToUpper(string(finding.Severity)),
		RuleIndex: ruleIndex, Kind: "fail", Level: sarifLevel(finding.Severity),
		Message:   sarifMessage{Text: finding.Title + ": " + finding.Detail},
		Locations: []sarifLocation{{PhysicalLocation: location}},
		PartialFingerprints: map[string]string{
			"primaryLocationLineHash":      finding.Fingerprint,
			"cyberagentFindingFingerprint": finding.Fingerprint,
		},
		Properties: map[string]any{
			"cyberagentFindingId":             finding.ID,
			"cyberagentReportId":              report.ID,
			"cyberagentFindingStatus":         status,
			"cyberagentValidationStatus":      finding.Validation.Status,
			"cyberagentSourceSeverity":        finding.Severity,
			"cyberagentCategory":              finding.Category,
			"cyberagentConfidence":            finding.Confidence,
			"cyberagentModelEvidenceCount":    len(finding.Evidence),
			"cyberagentArtifactEvidenceCount": len(finding.ArtifactEvidence),
		},
	}, nil
}

func sarifLevel(severity domain.FindingSeverity) string {
	switch severity {
	case domain.FindingSeverityInfo, domain.FindingSeverityLow:
		return "note"
	case domain.FindingSeverityMedium:
		return "warning"
	case domain.FindingSeverityHigh, domain.FindingSeverityCritical:
		return "error"
	default:
		return "none"
	}
}

func sarifURI(relativePath string) string {
	parts := strings.Split(relativePath, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func effectiveFindingStatus(finding domain.Finding) domain.FindingStatus {
	return finding.EffectiveStatus()
}

func confirmedUnresolvedFindingStatus(status domain.FindingStatus) bool {
	return status == domain.FindingStatusValidated || status == domain.FindingStatusAccepted
}
