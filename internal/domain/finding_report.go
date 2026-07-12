package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	FindingReportProtocolVersion               = "finding_report.v1"
	FindingReportSourceReadOnlyFanoutExecution = "readonly_fanout_execution"
	FindingEvidenceKindModelAssertion          = "model_assertion"
	FindingEvidenceSourceReadOnlyFanoutFinding = "readonly_fanout_finding"
	MaxFindingReportFindings                   = MaxReadOnlyFanoutParallelism * MaxReadOnlyFanoutFindings
	MaxFindingReportEvidence                   = MaxFindingReportFindings
	MaxFindingReportTitleRunes                 = 256
)

type FindingSeverity string

const (
	FindingSeverityInfo     FindingSeverity = "info"
	FindingSeverityLow      FindingSeverity = "low"
	FindingSeverityMedium   FindingSeverity = "medium"
	FindingSeverityHigh     FindingSeverity = "high"
	FindingSeverityCritical FindingSeverity = "critical"
)

func (s FindingSeverity) Valid() bool {
	switch s {
	case FindingSeverityInfo, FindingSeverityLow, FindingSeverityMedium,
		FindingSeverityHigh, FindingSeverityCritical:
		return true
	default:
		return false
	}
}

type FindingStatus string

const (
	FindingStatusDraft      FindingStatus = "draft"
	FindingStatusValidating FindingStatus = "validating"
	FindingStatusValidated  FindingStatus = "validated"
	FindingStatusAccepted   FindingStatus = "accepted"
	FindingStatusFixed      FindingStatus = "fixed"
	FindingStatusRejected   FindingStatus = "rejected"
)

func (s FindingStatus) Valid() bool {
	switch s {
	case FindingStatusDraft, FindingStatusValidating, FindingStatusValidated,
		FindingStatusAccepted, FindingStatusFixed, FindingStatusRejected:
		return true
	default:
		return false
	}
}

type FindingReportStatus string

const FindingReportGenerated FindingReportStatus = "generated"

func (s FindingReportStatus) Valid() bool {
	return s == FindingReportGenerated
}

type FindingSeveritySummary struct {
	Info     int `json:"info"`
	Low      int `json:"low"`
	Medium   int `json:"medium"`
	High     int `json:"high"`
	Critical int `json:"critical"`
}

func (s FindingSeveritySummary) Total() int {
	return s.Info + s.Low + s.Medium + s.High + s.Critical
}

func (s *FindingSeveritySummary) Add(severity FindingSeverity) error {
	if s == nil {
		return errors.New("finding severity summary is required")
	}
	switch severity {
	case FindingSeverityInfo:
		s.Info++
	case FindingSeverityLow:
		s.Low++
	case FindingSeverityMedium:
		s.Medium++
	case FindingSeverityHigh:
		s.High++
	case FindingSeverityCritical:
		s.Critical++
	default:
		return errors.New("finding severity is invalid")
	}
	return nil
}

type FindingEvidence struct {
	ID                string    `json:"id"`
	ReportID          string    `json:"report_id"`
	FindingID         string    `json:"finding_id"`
	RunID             string    `json:"run_id"`
	Ordinal           int       `json:"ordinal"`
	Kind              string    `json:"kind"`
	SourceKind        string    `json:"source_kind"`
	SourceID          string    `json:"source_id"`
	SourceShard       int       `json:"source_shard"`
	SourceOrdinal     int       `json:"source_ordinal"`
	SourceFingerprint string    `json:"source_fingerprint"`
	SourceDigest      string    `json:"source_digest"`
	RelativePath      string    `json:"relative_path"`
	LineStart         int       `json:"line_start"`
	LineEnd           int       `json:"line_end"`
	Confidence        int       `json:"confidence"`
	CreatedAt         time.Time `json:"created_at"`
}

func (e FindingEvidence) Validate() error {
	for _, value := range []string{e.ID, e.ReportID, e.FindingID, e.RunID, e.SourceID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding evidence identities are invalid")
		}
	}
	if e.Ordinal <= 0 || e.Ordinal > MaxFindingReportEvidence ||
		e.Kind != FindingEvidenceKindModelAssertion ||
		e.SourceKind != FindingEvidenceSourceReadOnlyFanoutFinding ||
		e.SourceShard <= 0 || e.SourceShard > MaxReadOnlyFanoutParallelism ||
		e.SourceOrdinal <= 0 || e.SourceOrdinal > MaxReadOnlyFanoutFindings ||
		!validLowerHexDigest(e.SourceFingerprint) ||
		!validLowerHexDigest(e.SourceDigest) ||
		!validReadOnlyFanoutPath(e.RelativePath) || e.LineStart < 0 ||
		e.LineEnd < e.LineStart || e.Confidence < 0 || e.Confidence > 100 ||
		e.CreatedAt.IsZero() {
		return errors.New("finding evidence is invalid")
	}
	return nil
}

type Finding struct {
	ID           string            `json:"id"`
	ReportID     string            `json:"report_id"`
	RunID        string            `json:"run_id"`
	Ordinal      int               `json:"ordinal"`
	Fingerprint  string            `json:"fingerprint"`
	Status       FindingStatus     `json:"status"`
	Severity     FindingSeverity   `json:"severity"`
	Category     string            `json:"category"`
	Title        string            `json:"title"`
	Detail       string            `json:"detail"`
	RelativePath string            `json:"relative_path"`
	LineStart    int               `json:"line_start"`
	LineEnd      int               `json:"line_end"`
	Confidence   int               `json:"confidence"`
	Version      int64             `json:"version"`
	CreatedAt    time.Time         `json:"created_at"`
	Evidence     []FindingEvidence `json:"evidence"`
}

func (f Finding) Validate() error {
	for _, value := range []string{f.ID, f.ReportID, f.RunID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding identities are invalid")
		}
	}
	if f.Ordinal <= 0 || f.Ordinal > MaxFindingReportFindings ||
		!validLowerHexDigest(f.Fingerprint) || f.Status != FindingStatusDraft ||
		!f.Severity.Valid() ||
		!validReadOnlyReportText(f.Category, MaxReadOnlyFanoutFindingCategoryRunes) ||
		!validReadOnlyReportText(f.Title, MaxReadOnlyFanoutFindingTitleRunes) ||
		!validReadOnlyReportText(f.Detail, MaxReadOnlyFanoutFindingDetailRunes) ||
		!validReadOnlyFanoutPath(f.RelativePath) || f.LineStart < 0 ||
		f.LineEnd < f.LineStart || f.Confidence < 0 || f.Confidence > 100 ||
		f.Version != 1 || f.CreatedAt.IsZero() || len(f.Evidence) == 0 ||
		len(f.Evidence) > MaxFindingReportEvidence {
		return errors.New("finding is invalid")
	}
	seenEvidence := make(map[string]struct{}, len(f.Evidence))
	seenSources := make(map[string]struct{}, len(f.Evidence))
	for index, evidence := range f.Evidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
		if evidence.ReportID != f.ReportID || evidence.FindingID != f.ID ||
			evidence.RunID != f.RunID || evidence.Ordinal != index+1 ||
			!evidence.CreatedAt.Equal(f.CreatedAt) || evidence.RelativePath != f.RelativePath ||
			evidence.LineStart != f.LineStart || evidence.LineEnd != f.LineEnd ||
			evidence.Confidence < f.Confidence {
			return errors.New("finding evidence projection is inconsistent")
		}
		if _, found := seenEvidence[evidence.ID]; found {
			return errors.New("finding evidence id is duplicated")
		}
		seenEvidence[evidence.ID] = struct{}{}
		sourceKey := fmt.Sprintf("%s:%d:%d", evidence.SourceID,
			evidence.SourceShard, evidence.SourceOrdinal)
		if _, found := seenSources[sourceKey]; found {
			return errors.New("finding evidence source is duplicated")
		}
		seenSources[sourceKey] = struct{}{}
	}
	return nil
}

type FindingReport struct {
	ID               string                 `json:"id"`
	RunID            string                 `json:"run_id"`
	SourceKind       string                 `json:"source_kind"`
	SourceID         string                 `json:"source_id"`
	ProtocolVersion  string                 `json:"protocol_version"`
	Status           FindingReportStatus    `json:"status"`
	Title            string                 `json:"title"`
	ProjectionDigest string                 `json:"projection_digest"`
	FindingCount     int                    `json:"finding_count"`
	EvidenceCount    int                    `json:"evidence_count"`
	Severity         FindingSeveritySummary `json:"severity"`
	Version          int64                  `json:"version"`
	CreatedAt        time.Time              `json:"created_at"`
	Findings         []Finding              `json:"findings"`
}

func (r FindingReport) Validate() error {
	for _, value := range []string{r.ID, r.RunID, r.SourceID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding report identities are invalid")
		}
	}
	if r.SourceKind != FindingReportSourceReadOnlyFanoutExecution ||
		r.ProtocolVersion != FindingReportProtocolVersion || !r.Status.Valid() ||
		!utf8.ValidString(r.Title) || strings.TrimSpace(r.Title) != r.Title ||
		r.Title == "" || utf8.RuneCountInString(r.Title) > MaxFindingReportTitleRunes ||
		!validLowerHexDigest(r.ProjectionDigest) || r.FindingCount < 0 ||
		r.FindingCount > MaxFindingReportFindings || r.EvidenceCount < 0 ||
		r.EvidenceCount > MaxFindingReportEvidence || r.Severity.Total() != r.FindingCount ||
		r.Version != 2 || r.CreatedAt.IsZero() || len(r.Findings) != r.FindingCount {
		return errors.New("finding report metadata is invalid")
	}
	seenFindings := make(map[string]struct{}, len(r.Findings))
	var summary FindingSeveritySummary
	evidenceCount := 0
	for index, finding := range r.Findings {
		if err := finding.Validate(); err != nil {
			return err
		}
		if finding.ReportID != r.ID || finding.RunID != r.RunID ||
			finding.Ordinal != index+1 || !finding.CreatedAt.Equal(r.CreatedAt) {
			return errors.New("finding report projection is inconsistent")
		}
		if _, found := seenFindings[finding.ID]; found {
			return errors.New("finding id is duplicated")
		}
		seenFindings[finding.ID] = struct{}{}
		if err := summary.Add(finding.Severity); err != nil {
			return err
		}
		evidenceCount += len(finding.Evidence)
	}
	if summary != r.Severity || evidenceCount != r.EvidenceCount {
		return errors.New("finding report counts are inconsistent")
	}
	digest, err := FindingReportProjectionDigest(r)
	if err != nil || digest != r.ProjectionDigest {
		return errors.New("finding report projection digest is inconsistent")
	}
	return nil
}

func FindingReportProjectionDigest(report FindingReport) (string, error) {
	type projection FindingReport
	copy := projection(report)
	copy.ProjectionDigest = ""
	encoded, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	return findingReportDigest("finding_report_projection.v1", string(encoded)), nil
}

func findingReportDigest(parts ...string) string {
	hash := sha256.New()
	for index, part := range parts {
		if index > 0 {
			hash.Write([]byte{0})
		}
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
