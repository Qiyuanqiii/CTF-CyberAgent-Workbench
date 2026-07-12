package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"cyberagent-workbench/internal/domain"
)

const readOnlyFanoutReportTitle = "Read-only Fan-out Audit Report"

type ReadOnlyFanoutSourceFinding struct {
	ShardOrdinal int
	Ordinal      int
	Fingerprint  string
	ReportDigest string
	Finding      domain.ReadOnlyFanoutFinding
}

type findingGroup struct {
	fingerprint string
	severity    domain.FindingSeverity
	category    string
	title       string
	detail      string
	path        string
	lineStart   int
	lineEnd     int
	confidence  int
	sources     []ReadOnlyFanoutSourceFinding
}

func ProjectReadOnlyFanout(execution domain.ReadOnlyFanoutExecution,
	sources []ReadOnlyFanoutSourceFinding,
) (domain.FindingReport, error) {
	if err := execution.Validate(); err != nil {
		return domain.FindingReport{}, fmt.Errorf("invalid read-only fan-out execution: %w", err)
	}
	if execution.Status != domain.ReadOnlyFanoutExecutionCompleted ||
		execution.FinishedAt == nil {
		return domain.FindingReport{}, errors.New(
			"finding report requires a completed read-only fan-out execution")
	}
	expectedSources := 0
	shardDigests := make(map[int]string, len(execution.Shards))
	for _, shard := range execution.Shards {
		expectedSources += shard.FindingCount
		shardDigests[shard.Ordinal] = shard.ReportDigest
	}
	if len(sources) != expectedSources {
		return domain.FindingReport{}, errors.New(
			"read-only fan-out finding ledger count does not match its execution")
	}

	groups := make(map[string]*findingGroup, len(sources))
	seenSources := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		if err := validateReadOnlyFanoutSourceFinding(execution, shardDigests, source); err != nil {
			return domain.FindingReport{}, err
		}
		sourceKey := fmt.Sprintf("%d:%d", source.ShardOrdinal, source.Ordinal)
		if _, found := seenSources[sourceKey]; found {
			return domain.FindingReport{}, errors.New(
				"read-only fan-out source finding coordinate is duplicated")
		}
		seenSources[sourceKey] = struct{}{}
		fingerprint, err := genericFindingFingerprint(source.Finding)
		if err != nil {
			return domain.FindingReport{}, err
		}
		group, found := groups[fingerprint]
		if !found {
			group = &findingGroup{
				fingerprint: fingerprint,
				severity:    domain.FindingSeverity(source.Finding.Severity),
				category:    source.Finding.Category, title: source.Finding.Title,
				detail: source.Finding.Detail, path: source.Finding.Path,
				lineStart: source.Finding.LineStart, lineEnd: source.Finding.LineEnd,
				confidence: source.Finding.Confidence,
			}
			groups[fingerprint] = group
		} else if source.Finding.Confidence < group.confidence {
			group.confidence = source.Finding.Confidence
		}
		group.sources = append(group.sources, source)
	}

	ordered := make([]*findingGroup, 0, len(groups))
	for _, group := range groups {
		slices.SortFunc(group.sources, compareSourceFindings)
		ordered = append(ordered, group)
	}
	slices.SortFunc(ordered, compareFindingGroups)

	createdAt := execution.FinishedAt.UTC()
	reportID := deterministicID("report", domain.FindingReportProtocolVersion,
		domain.FindingReportSourceReadOnlyFanoutExecution, execution.ID)
	result := domain.FindingReport{
		ID: reportID, RunID: execution.RunID,
		SourceKind: domain.FindingReportSourceReadOnlyFanoutExecution,
		SourceID:   execution.ID, ProtocolVersion: domain.FindingReportProtocolVersion,
		Status: domain.FindingReportGenerated, Title: readOnlyFanoutReportTitle,
		FindingCount: len(ordered), EvidenceCount: len(sources),
		Version: 2, CreatedAt: createdAt,
		Findings: make([]domain.Finding, len(ordered)),
	}
	for index, group := range ordered {
		findingID := deterministicID("finding", reportID, group.fingerprint)
		finding := domain.Finding{
			ID: findingID, ReportID: reportID, RunID: execution.RunID,
			Ordinal: index + 1, Fingerprint: group.fingerprint,
			Status: domain.FindingStatusDraft, Severity: group.severity,
			Category: group.category, Title: group.title, Detail: group.detail,
			RelativePath: group.path, LineStart: group.lineStart,
			LineEnd: group.lineEnd, Confidence: group.confidence,
			Version: 1, CreatedAt: createdAt,
			Evidence: make([]domain.FindingEvidence, len(group.sources)),
		}
		for evidenceIndex, source := range group.sources {
			finding.Evidence[evidenceIndex] = domain.FindingEvidence{
				ID: deterministicID("evidence", findingID, execution.ID,
					fmt.Sprint(source.ShardOrdinal), fmt.Sprint(source.Ordinal)),
				ReportID: reportID, FindingID: findingID, RunID: execution.RunID,
				Ordinal:    evidenceIndex + 1,
				Kind:       domain.FindingEvidenceKindModelAssertion,
				SourceKind: domain.FindingEvidenceSourceReadOnlyFanoutFinding,
				SourceID:   execution.ID, SourceShard: source.ShardOrdinal,
				SourceOrdinal: source.Ordinal, SourceFingerprint: source.Fingerprint,
				SourceDigest: source.ReportDigest, RelativePath: source.Finding.Path,
				LineStart: source.Finding.LineStart, LineEnd: source.Finding.LineEnd,
				Confidence: source.Finding.Confidence, CreatedAt: createdAt,
			}
		}
		if err := result.Severity.Add(finding.Severity); err != nil {
			return domain.FindingReport{}, err
		}
		result.Findings[index] = finding
	}
	digest, err := domain.FindingReportProjectionDigest(result)
	if err != nil {
		return domain.FindingReport{}, err
	}
	result.ProjectionDigest = digest
	if err := result.Validate(); err != nil {
		return domain.FindingReport{}, err
	}
	return result, nil
}

func validateReadOnlyFanoutSourceFinding(execution domain.ReadOnlyFanoutExecution,
	shardDigests map[int]string, source ReadOnlyFanoutSourceFinding,
) error {
	if source.ShardOrdinal <= 0 || source.ShardOrdinal > execution.Parallelism ||
		source.Ordinal <= 0 || source.Ordinal > domain.MaxReadOnlyFanoutFindings ||
		!validDigest(source.Fingerprint) || !validDigest(source.ReportDigest) ||
		shardDigests[source.ShardOrdinal] != source.ReportDigest {
		return errors.New("read-only fan-out source finding metadata is invalid")
	}
	allowed := map[string]struct{}{source.Finding.Path: {}}
	if err := source.Finding.Validate(allowed); err != nil {
		return fmt.Errorf("invalid read-only fan-out source finding: %w", err)
	}
	return nil
}

func genericFindingFingerprint(finding domain.ReadOnlyFanoutFinding) (string, error) {
	type facts struct {
		Severity  domain.ReadOnlyFindingSeverity `json:"severity"`
		Category  string                         `json:"category"`
		Title     string                         `json:"title"`
		Detail    string                         `json:"detail"`
		Path      string                         `json:"path"`
		LineStart int                            `json:"line_start"`
		LineEnd   int                            `json:"line_end"`
	}
	encoded, err := json.Marshal(facts{
		Severity: finding.Severity, Category: finding.Category, Title: finding.Title,
		Detail: finding.Detail, Path: finding.Path, LineStart: finding.LineStart,
		LineEnd: finding.LineEnd,
	})
	if err != nil {
		return "", err
	}
	return digest("finding_facts.v1", string(encoded)), nil
}

func compareSourceFindings(left, right ReadOnlyFanoutSourceFinding) int {
	if left.ShardOrdinal != right.ShardOrdinal {
		return left.ShardOrdinal - right.ShardOrdinal
	}
	return left.Ordinal - right.Ordinal
}

func compareFindingGroups(left, right *findingGroup) int {
	if rank := severityRank(right.severity) - severityRank(left.severity); rank != 0 {
		return rank
	}
	if compared := strings.Compare(left.path, right.path); compared != 0 {
		return compared
	}
	if left.lineStart != right.lineStart {
		return left.lineStart - right.lineStart
	}
	if left.lineEnd != right.lineEnd {
		return left.lineEnd - right.lineEnd
	}
	for _, pair := range [][2]string{{left.category, right.category},
		{left.title, right.title}, {left.detail, right.detail},
		{left.fingerprint, right.fingerprint}} {
		if compared := strings.Compare(pair[0], pair[1]); compared != 0 {
			return compared
		}
	}
	return 0
}

func severityRank(value domain.FindingSeverity) int {
	switch value {
	case domain.FindingSeverityCritical:
		return 5
	case domain.FindingSeverityHigh:
		return 4
	case domain.FindingSeverityMedium:
		return 3
	case domain.FindingSeverityLow:
		return 2
	case domain.FindingSeverityInfo:
		return 1
	default:
		return 0
	}
}

func deterministicID(prefix string, parts ...string) string {
	return prefix + "-" + digest(append([]string{"deterministic_id.v1", prefix}, parts...)...)
}

func digest(parts ...string) string {
	hash := sha256.New()
	for index, part := range parts {
		if index > 0 {
			hash.Write([]byte{0})
		}
		hash.Write([]byte(part))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
