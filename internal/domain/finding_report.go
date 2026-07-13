package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
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
	MaxFindingArtifactEvidence                 = 64
	MaxFindingRemediationEvidence              = 64
	MaxFindingValidationTextRunes              = 2048
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

type FindingArtifactEvidence struct {
	ID               string    `json:"id"`
	ReportID         string    `json:"report_id"`
	FindingID        string    `json:"finding_id"`
	RunID            string    `json:"run_id"`
	Ordinal          int       `json:"ordinal"`
	ArtifactID       string    `json:"artifact_id"`
	ArtifactSHA256   string    `json:"artifact_sha256"`
	ArtifactSize     int64     `json:"artifact_size_bytes"`
	ArtifactMIME     string    `json:"artifact_mime"`
	ArtifactStream   string    `json:"artifact_stream"`
	ArtifactTool     string    `json:"artifact_tool"`
	ArtifactSource   string    `json:"artifact_source_id"`
	ArtifactRedacted bool      `json:"artifact_redacted"`
	AttachedBy       string    `json:"attached_by"`
	Note             string    `json:"note"`
	CreatedAt        time.Time `json:"created_at"`
}

func (e FindingArtifactEvidence) Validate() error {
	for _, value := range []string{e.ID, e.ReportID, e.FindingID, e.RunID,
		e.ArtifactID, e.ArtifactTool, e.ArtifactSource, e.AttachedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding Artifact Evidence identities are invalid")
		}
	}
	if e.Ordinal <= 0 || e.Ordinal > MaxFindingArtifactEvidence ||
		!validLowerHexDigest(e.ArtifactSHA256) || e.ArtifactSize <= 0 ||
		e.ArtifactSize > 4*1024*1024 ||
		(e.ArtifactStream != "stdout" && e.ArtifactStream != "stderr") ||
		!validFindingValidationText(e.Note) || e.CreatedAt.IsZero() {
		return errors.New("finding Artifact Evidence is invalid")
	}
	if strings.TrimSpace(e.ArtifactMIME) != e.ArtifactMIME ||
		len([]byte(e.ArtifactMIME)) > 256 {
		return errors.New("finding Artifact Evidence MIME is invalid")
	}
	if _, _, err := mime.ParseMediaType(e.ArtifactMIME); err != nil {
		return errors.New("finding Artifact Evidence MIME is invalid")
	}
	return nil
}

type FindingValidation struct {
	ID                     string        `json:"id"`
	ReportID               string        `json:"report_id"`
	FindingID              string        `json:"finding_id"`
	RunID                  string        `json:"run_id"`
	FromStatus             FindingStatus `json:"from_status"`
	Status                 FindingStatus `json:"status"`
	DecidedBy              string        `json:"decided_by"`
	Reason                 string        `json:"reason"`
	ArtifactEvidenceCount  int           `json:"artifact_evidence_count"`
	ArtifactEvidenceDigest string        `json:"artifact_evidence_digest"`
	Version                int64         `json:"version"`
	CreatedAt              time.Time     `json:"created_at"`
}

type FindingAcceptance struct {
	ID                               string        `json:"id"`
	ReportID                         string        `json:"report_id"`
	FindingID                        string        `json:"finding_id"`
	RunID                            string        `json:"run_id"`
	FromStatus                       FindingStatus `json:"from_status"`
	Status                           FindingStatus `json:"status"`
	ValidationID                     string        `json:"validation_id"`
	ValidationArtifactEvidenceCount  int           `json:"validation_artifact_evidence_count"`
	ValidationArtifactEvidenceDigest string        `json:"validation_artifact_evidence_digest"`
	DecidedBy                        string        `json:"decided_by"`
	Reason                           string        `json:"reason"`
	Version                          int64         `json:"version"`
	CreatedAt                        time.Time     `json:"created_at"`
}

func (a FindingAcceptance) Validate() error {
	for _, value := range []string{a.ID, a.ReportID, a.FindingID, a.RunID,
		a.ValidationID, a.DecidedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding acceptance identities are invalid")
		}
	}
	if a.FromStatus != FindingStatusValidated || a.Status != FindingStatusAccepted ||
		a.ValidationArtifactEvidenceCount <= 0 ||
		a.ValidationArtifactEvidenceCount > MaxFindingArtifactEvidence ||
		!validLowerHexDigest(a.ValidationArtifactEvidenceDigest) ||
		!validFindingValidationText(a.Reason) || a.Version != 1 || a.CreatedAt.IsZero() {
		return errors.New("finding acceptance is invalid")
	}
	return nil
}

type FindingFix struct {
	ID                        string        `json:"id"`
	ReportID                  string        `json:"report_id"`
	FindingID                 string        `json:"finding_id"`
	RunID                     string        `json:"run_id"`
	AcceptanceID              string        `json:"acceptance_id"`
	FromStatus                FindingStatus `json:"from_status"`
	Status                    FindingStatus `json:"status"`
	RemediationEvidenceCount  int           `json:"remediation_evidence_count"`
	RemediationEvidenceDigest string        `json:"remediation_evidence_digest"`
	DecidedBy                 string        `json:"decided_by"`
	Reason                    string        `json:"reason"`
	Version                   int64         `json:"version"`
	CreatedAt                 time.Time     `json:"created_at"`
}

func (f FindingFix) Validate() error {
	for _, value := range []string{f.ID, f.ReportID, f.FindingID, f.RunID,
		f.AcceptanceID, f.DecidedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding fix identities are invalid")
		}
	}
	if f.FromStatus != FindingStatusAccepted || f.Status != FindingStatusFixed ||
		f.RemediationEvidenceCount <= 0 ||
		f.RemediationEvidenceCount > MaxFindingRemediationEvidence ||
		!validLowerHexDigest(f.RemediationEvidenceDigest) ||
		!validFindingValidationText(f.Reason) || f.Version != 1 || f.CreatedAt.IsZero() {
		return errors.New("finding fix is invalid")
	}
	return nil
}

func (v FindingValidation) Validate() error {
	for _, value := range []string{v.ID, v.ReportID, v.FindingID, v.RunID, v.DecidedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding validation identities are invalid")
		}
	}
	if v.FromStatus != FindingStatusDraft ||
		(v.Status != FindingStatusValidated && v.Status != FindingStatusRejected) ||
		!validFindingValidationText(v.Reason) || v.ArtifactEvidenceCount < 0 ||
		v.ArtifactEvidenceCount > MaxFindingArtifactEvidence ||
		!validLowerHexDigest(v.ArtifactEvidenceDigest) || v.Version != 1 ||
		v.CreatedAt.IsZero() ||
		(v.Status == FindingStatusValidated && v.ArtifactEvidenceCount == 0) {
		return errors.New("finding validation is invalid")
	}
	return nil
}

type FindingArtifactEvidenceOperation struct {
	KeyDigest          string
	RequestFingerprint string
	EvidenceID         string
	FindingID          string
	ArtifactID         string
	RunID              string
	AttachedBy         string
	CreatedAt          time.Time
}

func (o FindingArtifactEvidenceOperation) Validate() error {
	for _, value := range []string{o.EvidenceID, o.FindingID, o.ArtifactID,
		o.RunID, o.AttachedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding Artifact Evidence operation identities are invalid")
		}
	}
	if !validLowerHexDigest(o.KeyDigest) ||
		!validLowerHexDigest(o.RequestFingerprint) || o.CreatedAt.IsZero() {
		return errors.New("finding Artifact Evidence operation is invalid")
	}
	return nil
}

type FindingValidationOperation struct {
	KeyDigest          string
	RequestFingerprint string
	ValidationID       string
	FindingID          string
	RunID              string
	Status             FindingStatus
	DecidedBy          string
	CreatedAt          time.Time
}

func (o FindingValidationOperation) Validate() error {
	for _, value := range []string{o.ValidationID, o.FindingID, o.RunID, o.DecidedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding validation operation identities are invalid")
		}
	}
	if !validLowerHexDigest(o.KeyDigest) ||
		!validLowerHexDigest(o.RequestFingerprint) ||
		(o.Status != FindingStatusValidated && o.Status != FindingStatusRejected) ||
		o.CreatedAt.IsZero() {
		return errors.New("finding validation operation is invalid")
	}
	return nil
}

type FindingAcceptanceOperation struct {
	KeyDigest          string
	RequestFingerprint string
	AcceptanceID       string
	ValidationID       string
	FindingID          string
	RunID              string
	DecidedBy          string
	CreatedAt          time.Time
}

func (o FindingAcceptanceOperation) Validate() error {
	for _, value := range []string{o.AcceptanceID, o.ValidationID, o.FindingID,
		o.RunID, o.DecidedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding acceptance operation identities are invalid")
		}
	}
	if !validLowerHexDigest(o.KeyDigest) ||
		!validLowerHexDigest(o.RequestFingerprint) || o.CreatedAt.IsZero() {
		return errors.New("finding acceptance operation is invalid")
	}
	return nil
}

type FindingRemediationEvidenceOperation struct {
	KeyDigest          string
	RequestFingerprint string
	EvidenceID         string
	AcceptanceID       string
	FindingID          string
	ArtifactID         string
	RunID              string
	AttachedBy         string
	CreatedAt          time.Time
}

func (o FindingRemediationEvidenceOperation) Validate() error {
	for _, value := range []string{o.EvidenceID, o.AcceptanceID, o.FindingID,
		o.ArtifactID, o.RunID, o.AttachedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding remediation Evidence operation identities are invalid")
		}
	}
	if !validLowerHexDigest(o.KeyDigest) ||
		!validLowerHexDigest(o.RequestFingerprint) || o.CreatedAt.IsZero() {
		return errors.New("finding remediation Evidence operation is invalid")
	}
	return nil
}

type FindingFixOperation struct {
	KeyDigest          string
	RequestFingerprint string
	FixID              string
	AcceptanceID       string
	FindingID          string
	RunID              string
	DecidedBy          string
	CreatedAt          time.Time
}

func (o FindingFixOperation) Validate() error {
	for _, value := range []string{o.FixID, o.AcceptanceID, o.FindingID,
		o.RunID, o.DecidedBy} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding fix operation identities are invalid")
		}
	}
	if !validLowerHexDigest(o.KeyDigest) ||
		!validLowerHexDigest(o.RequestFingerprint) || o.CreatedAt.IsZero() {
		return errors.New("finding fix operation is invalid")
	}
	return nil
}

type Finding struct {
	ID                  string                    `json:"id"`
	ReportID            string                    `json:"report_id"`
	RunID               string                    `json:"run_id"`
	Ordinal             int                       `json:"ordinal"`
	Fingerprint         string                    `json:"fingerprint"`
	Status              FindingStatus             `json:"status"`
	Severity            FindingSeverity           `json:"severity"`
	Category            string                    `json:"category"`
	Title               string                    `json:"title"`
	Detail              string                    `json:"detail"`
	RelativePath        string                    `json:"relative_path"`
	LineStart           int                       `json:"line_start"`
	LineEnd             int                       `json:"line_end"`
	Confidence          int                       `json:"confidence"`
	Version             int64                     `json:"version"`
	CreatedAt           time.Time                 `json:"created_at"`
	Evidence            []FindingEvidence         `json:"evidence"`
	ArtifactEvidence    []FindingArtifactEvidence `json:"artifact_evidence,omitempty"`
	Validation          *FindingValidation        `json:"validation,omitempty"`
	Acceptance          *FindingAcceptance        `json:"acceptance,omitempty"`
	RemediationEvidence []FindingArtifactEvidence `json:"remediation_evidence,omitempty"`
	Fix                 *FindingFix               `json:"fix,omitempty"`
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
	seenArtifacts := make(map[string]struct{}, len(f.ArtifactEvidence))
	for index, evidence := range f.ArtifactEvidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
		if evidence.ReportID != f.ReportID || evidence.FindingID != f.ID ||
			evidence.RunID != f.RunID || evidence.Ordinal != index+1 {
			return errors.New("finding Artifact Evidence projection is inconsistent")
		}
		if _, found := seenArtifacts[evidence.ArtifactID]; found {
			return errors.New("finding Artifact Evidence is duplicated")
		}
		seenArtifacts[evidence.ArtifactID] = struct{}{}
	}
	if len(f.ArtifactEvidence) > MaxFindingArtifactEvidence {
		return errors.New("finding has too much Artifact Evidence")
	}
	if f.Validation != nil {
		if err := f.Validation.Validate(); err != nil {
			return err
		}
		if f.Validation.ReportID != f.ReportID || f.Validation.FindingID != f.ID ||
			f.Validation.RunID != f.RunID ||
			f.Validation.ArtifactEvidenceCount != len(f.ArtifactEvidence) {
			return errors.New("finding validation projection is inconsistent")
		}
		digest, err := FindingArtifactEvidenceDigest(f.ArtifactEvidence)
		if err != nil || digest != f.Validation.ArtifactEvidenceDigest {
			return errors.New("finding validation Evidence digest is inconsistent")
		}
	}
	if f.Acceptance != nil {
		if err := f.Acceptance.Validate(); err != nil {
			return err
		}
		if f.Validation == nil || f.Validation.Status != FindingStatusValidated ||
			f.Acceptance.ReportID != f.ReportID || f.Acceptance.FindingID != f.ID ||
			f.Acceptance.RunID != f.RunID ||
			f.Acceptance.ValidationID != f.Validation.ID ||
			f.Acceptance.ValidationArtifactEvidenceCount !=
				f.Validation.ArtifactEvidenceCount ||
			f.Acceptance.ValidationArtifactEvidenceDigest !=
				f.Validation.ArtifactEvidenceDigest ||
			f.Acceptance.CreatedAt.Before(f.Validation.CreatedAt) {
			return errors.New("finding acceptance projection is inconsistent")
		}
	}
	if len(f.RemediationEvidence) > MaxFindingRemediationEvidence {
		return errors.New("finding has too much remediation Evidence")
	}
	if len(f.RemediationEvidence) > 0 && f.Acceptance == nil {
		return errors.New("finding remediation Evidence requires acceptance")
	}
	seenRemediationArtifacts := make(map[string]struct{}, len(f.RemediationEvidence))
	for index, evidence := range f.RemediationEvidence {
		if err := evidence.Validate(); err != nil {
			return err
		}
		if evidence.ReportID != f.ReportID || evidence.FindingID != f.ID ||
			evidence.RunID != f.RunID || evidence.Ordinal != index+1 ||
			f.Acceptance == nil || evidence.CreatedAt.Before(f.Acceptance.CreatedAt) {
			return errors.New("finding remediation Evidence projection is inconsistent")
		}
		if _, found := seenArtifacts[evidence.ArtifactID]; found {
			return errors.New("finding remediation Evidence reused validation Artifact")
		}
		if _, found := seenRemediationArtifacts[evidence.ArtifactID]; found {
			return errors.New("finding remediation Evidence is duplicated")
		}
		seenRemediationArtifacts[evidence.ArtifactID] = struct{}{}
	}
	if f.Fix != nil {
		if err := f.Fix.Validate(); err != nil {
			return err
		}
		if f.Acceptance == nil || f.Fix.ReportID != f.ReportID ||
			f.Fix.FindingID != f.ID || f.Fix.RunID != f.RunID ||
			f.Fix.AcceptanceID != f.Acceptance.ID ||
			f.Fix.RemediationEvidenceCount != len(f.RemediationEvidence) ||
			f.Fix.CreatedAt.Before(f.Acceptance.CreatedAt) {
			return errors.New("finding fix projection is inconsistent")
		}
		digest, err := FindingRemediationEvidenceDigest(f.RemediationEvidence)
		if err != nil || digest != f.Fix.RemediationEvidenceDigest {
			return errors.New("finding fix Evidence digest is inconsistent")
		}
		if len(f.RemediationEvidence) == 0 ||
			f.Fix.CreatedAt.Before(f.RemediationEvidence[len(f.RemediationEvidence)-1].CreatedAt) {
			return errors.New("finding fix predates its remediation Evidence")
		}
	}
	return nil
}

func (f Finding) EffectiveStatus() FindingStatus {
	if f.Fix != nil {
		return FindingStatusFixed
	}
	if f.Acceptance != nil {
		return FindingStatusAccepted
	}
	if f.Validation != nil {
		return f.Validation.Status
	}
	return f.Status
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

type FindingReportSummary struct {
	ID            string
	RunID         string
	SourceKind    string
	SourceID      string
	Status        FindingReportStatus
	Title         string
	FindingCount  int
	EvidenceCount int
	Severity      FindingSeveritySummary
	Version       int64
	CreatedAt     time.Time
}

func (r FindingReportSummary) Validate() error {
	for _, value := range []string{r.ID, r.RunID, r.SourceID} {
		if !validAgentIdentity(value, false) || strings.ContainsRune(value, 0) {
			return errors.New("finding report summary identities are invalid")
		}
	}
	if r.SourceKind != FindingReportSourceReadOnlyFanoutExecution ||
		!r.Status.Valid() || !utf8.ValidString(r.Title) ||
		strings.TrimSpace(r.Title) != r.Title || r.Title == "" ||
		utf8.RuneCountInString(r.Title) > MaxFindingReportTitleRunes ||
		r.FindingCount < 0 || r.FindingCount > MaxFindingReportFindings ||
		r.EvidenceCount < 0 || r.EvidenceCount > MaxFindingReportEvidence ||
		r.Severity.Total() != r.FindingCount || r.Version != 2 || r.CreatedAt.IsZero() {
		return errors.New("finding report summary metadata is invalid")
	}
	return nil
}

func (r FindingReport) Summary() FindingReportSummary {
	return FindingReportSummary{
		ID: r.ID, RunID: r.RunID, SourceKind: r.SourceKind, SourceID: r.SourceID,
		Status: r.Status, Title: r.Title, FindingCount: r.FindingCount,
		EvidenceCount: r.EvidenceCount, Severity: r.Severity,
		Version: r.Version, CreatedAt: r.CreatedAt,
	}
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
	copy.Findings = append([]Finding(nil), report.Findings...)
	for index := range copy.Findings {
		copy.Findings[index].ArtifactEvidence = nil
		copy.Findings[index].Validation = nil
		copy.Findings[index].Acceptance = nil
		copy.Findings[index].RemediationEvidence = nil
		copy.Findings[index].Fix = nil
	}
	encoded, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	return findingReportDigest("finding_report_projection.v1", string(encoded)), nil
}

func FindingArtifactEvidenceDigest(evidence []FindingArtifactEvidence) (string, error) {
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return "", err
	}
	return findingReportDigest("finding_artifact_evidence.v1", string(encoded)), nil
}

func FindingRemediationEvidenceDigest(evidence []FindingArtifactEvidence) (string, error) {
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return "", err
	}
	return findingReportDigest("finding_remediation_evidence.v1", string(encoded)), nil
}

func validFindingValidationText(value string) bool {
	return value != "" && utf8.ValidString(value) && strings.TrimSpace(value) == value &&
		!strings.ContainsRune(value, 0) &&
		utf8.RuneCountInString(value) <= MaxFindingValidationTextRunes &&
		len([]byte(value)) <= MaxFindingValidationTextRunes*4
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
