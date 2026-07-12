package report

import (
	"errors"
	"fmt"
	"strings"

	"cyberagent-workbench/internal/domain"
)

type GateStatus string

const (
	GateStatusValidated GateStatus = "validated"
	GateStatusActive    GateStatus = "active"
	GateStatusNone      GateStatus = "none"
)

func ParseGateStatus(value string) (GateStatus, error) {
	status := GateStatus(strings.ToLower(strings.TrimSpace(value)))
	switch status {
	case GateStatusValidated, GateStatusActive, GateStatusNone:
		return status, nil
	default:
		return "", errors.New("report gate fail-status must be validated, active, or none")
	}
}

func ParseGateSeverity(value string) (domain.FindingSeverity, error) {
	severity := domain.FindingSeverity(strings.ToLower(strings.TrimSpace(value)))
	if !severity.Valid() {
		return "", errors.New("report gate min-severity must be info, low, medium, high, or critical")
	}
	return severity, nil
}

type GatePolicy struct {
	FailStatus  GateStatus             `json:"fail_status"`
	MinSeverity domain.FindingSeverity `json:"min_severity"`
}

func (p GatePolicy) Validate() error {
	status, err := ParseGateStatus(string(p.FailStatus))
	if err != nil {
		return err
	}
	if status != p.FailStatus {
		return errors.New("report gate fail-status must use its canonical lowercase value")
	}
	severity, err := ParseGateSeverity(string(p.MinSeverity))
	if err != nil {
		return err
	}
	if severity != p.MinSeverity {
		return errors.New("report gate minimum severity must use its canonical lowercase value")
	}
	return nil
}

type GateResult struct {
	ReportID         string     `json:"report_id"`
	RunID            string     `json:"run_id"`
	ProjectionDigest string     `json:"projection_digest"`
	Policy           GatePolicy `json:"policy"`
	FindingCount     int        `json:"finding_count"`
	DraftCount       int        `json:"draft_count"`
	ValidatedCount   int        `json:"validated_count"`
	AcceptedCount    int        `json:"accepted_count"`
	FixedCount       int        `json:"fixed_count"`
	RejectedCount    int        `json:"rejected_count"`
	MatchedCount     int        `json:"matched_count"`
	Passed           bool       `json:"passed"`
}

func EvaluateGate(value domain.FindingReport, policy GatePolicy) (GateResult, error) {
	if err := value.Validate(); err != nil {
		return GateResult{}, err
	}
	if err := policy.Validate(); err != nil {
		return GateResult{}, err
	}
	result := GateResult{
		ReportID: value.ID, RunID: value.RunID,
		ProjectionDigest: value.ProjectionDigest, Policy: policy,
		FindingCount: len(value.Findings),
	}
	minimum := severityRank(policy.MinSeverity)
	for _, finding := range value.Findings {
		status := effectiveFindingStatus(finding)
		switch status {
		case domain.FindingStatusDraft:
			result.DraftCount++
		case domain.FindingStatusValidated:
			result.ValidatedCount++
		case domain.FindingStatusAccepted:
			result.AcceptedCount++
		case domain.FindingStatusFixed:
			result.FixedCount++
		case domain.FindingStatusRejected:
			result.RejectedCount++
		default:
			return GateResult{}, fmt.Errorf("unsupported finding gate status %q", status)
		}
		if gateMatchesStatus(policy.FailStatus, status) &&
			severityRank(finding.Severity) >= minimum {
			result.MatchedCount++
		}
	}
	result.Passed = result.MatchedCount == 0
	return result, nil
}

func gateMatchesStatus(policy GateStatus, status domain.FindingStatus) bool {
	switch policy {
	case GateStatusValidated:
		return status == domain.FindingStatusValidated ||
			status == domain.FindingStatusAccepted
	case GateStatusActive:
		return status == domain.FindingStatusDraft ||
			status == domain.FindingStatusValidated ||
			status == domain.FindingStatusAccepted
	case GateStatusNone:
		return false
	default:
		return false
	}
}
