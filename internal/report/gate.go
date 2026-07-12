package report

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

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
	ReportID         string      `json:"report_id"`
	RunID            string      `json:"run_id"`
	ProjectionDigest string      `json:"projection_digest"`
	Policy           GatePolicy  `json:"policy"`
	FindingCount     int         `json:"finding_count"`
	DraftCount       int         `json:"draft_count"`
	ValidatedCount   int         `json:"validated_count"`
	AcceptedCount    int         `json:"accepted_count"`
	FixedCount       int         `json:"fixed_count"`
	RejectedCount    int         `json:"rejected_count"`
	MatchedCount     int         `json:"matched_count"`
	Passed           bool        `json:"passed"`
	Matches          []GateMatch `json:"-"`
}

type GateMatch struct {
	FindingID    string
	Fingerprint  string
	Status       domain.FindingStatus
	Severity     domain.FindingSeverity
	Category     string
	Title        string
	Detail       string
	RelativePath string
	LineStart    int
	LineEnd      int
}

func (r GateResult) Validate() error {
	if err := r.Policy.Validate(); err != nil {
		return err
	}
	if !validGateIdentity(r.ReportID) || !validGateIdentity(r.RunID) ||
		!validGateDigest(r.ProjectionDigest) || r.FindingCount < 0 ||
		r.FindingCount > domain.MaxFindingReportFindings ||
		r.DraftCount < 0 || r.ValidatedCount < 0 || r.AcceptedCount < 0 ||
		r.FixedCount < 0 || r.RejectedCount < 0 || r.MatchedCount < 0 ||
		r.FindingCount != r.DraftCount+r.ValidatedCount+r.AcceptedCount+
			r.FixedCount+r.RejectedCount || r.MatchedCount != len(r.Matches) ||
		r.Passed != (r.MatchedCount == 0) {
		return errors.New("report gate result is inconsistent")
	}
	seen := make(map[string]struct{}, len(r.Matches))
	matchedStatuses := map[domain.FindingStatus]int{}
	minimum := severityRank(r.Policy.MinSeverity)
	for _, match := range r.Matches {
		if err := match.Validate(); err != nil {
			return err
		}
		if !gateMatchesStatus(r.Policy.FailStatus, match.Status) ||
			severityRank(match.Severity) < minimum {
			return errors.New("report gate match does not satisfy its policy")
		}
		if _, found := seen[match.FindingID]; found {
			return errors.New("report gate matches contain a duplicate Finding")
		}
		seen[match.FindingID] = struct{}{}
		matchedStatuses[match.Status]++
	}
	if matchedStatuses[domain.FindingStatusDraft] > r.DraftCount ||
		matchedStatuses[domain.FindingStatusValidated] > r.ValidatedCount ||
		matchedStatuses[domain.FindingStatusAccepted] > r.AcceptedCount {
		return errors.New("report gate matches exceed their lifecycle counts")
	}
	return nil
}

func (m GateMatch) Validate() error {
	if !validGateIdentity(m.FindingID) || !validGateDigest(m.Fingerprint) ||
		!m.Severity.Valid() ||
		(m.Status != domain.FindingStatusDraft &&
			m.Status != domain.FindingStatusValidated &&
			m.Status != domain.FindingStatusAccepted) ||
		!validGateText(m.Category, domain.MaxReadOnlyFanoutFindingCategoryRunes) ||
		!validGateText(m.Title, domain.MaxReadOnlyFanoutFindingTitleRunes) ||
		!validGateText(m.Detail, domain.MaxReadOnlyFanoutFindingDetailRunes) ||
		!validGatePath(m.RelativePath) || m.LineStart < 0 || m.LineEnd < m.LineStart {
		return errors.New("report gate match is invalid")
	}
	return nil
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
			result.Matches = append(result.Matches, GateMatch{
				FindingID: finding.ID, Fingerprint: finding.Fingerprint,
				Status: status, Severity: finding.Severity,
				Category: finding.Category, Title: finding.Title, Detail: finding.Detail,
				RelativePath: finding.RelativePath,
				LineStart:    finding.LineStart, LineEnd: finding.LineEnd,
			})
		}
	}
	result.Passed = result.MatchedCount == 0
	if err := result.Validate(); err != nil {
		return GateResult{}, err
	}
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

func validGateIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) && len([]byte(value)) <= 256
}

func validGateDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validGateText(value string, maxRunes int) bool {
	return value != "" && strings.TrimSpace(value) == value && utf8.ValidString(value) &&
		!strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= maxRunes &&
		len([]byte(value)) <= maxRunes*4
}

func validGatePath(value string) bool {
	return value != "" && utf8.ValidString(value) && strings.TrimSpace(value) == value &&
		!strings.ContainsRune(value, 0) && !strings.Contains(value, "\\") &&
		len([]byte(value)) <= domain.MaxReadOnlyFanoutPathBytes &&
		!strings.HasPrefix(value, "/") && path.Clean(value) == value &&
		value != "." && value != ".." && !strings.HasPrefix(value, "../") &&
		!strings.Contains(value, "/../")
}
