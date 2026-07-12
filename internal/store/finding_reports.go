package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	reporting "cyberagent-workbench/internal/report"
)

const findingReportSelect = `SELECT id, run_id, source_kind, source_id,
	protocol_version, status, title, projection_digest, finding_count, evidence_count,
	info_count, low_count, medium_count, high_count, critical_count, version, created_at
	FROM finding_reports`

const findingSelect = `SELECT id, report_id, run_id, ordinal, fingerprint, status,
	severity, category, title, detail, relative_path, line_start, line_end, confidence,
	version, created_at FROM findings`

const findingEvidenceSelect = `SELECT id, report_id, finding_id, run_id, ordinal,
	kind, source_kind, source_id, source_shard, source_ordinal, source_fingerprint,
	source_digest, relative_path, line_start, line_end, confidence, created_at
	FROM finding_evidence`

func (s *SQLiteStore) EnsureReadOnlyFanoutFindingReport(ctx context.Context,
	executionID string,
) (domain.FindingReport, bool, error) {
	executionID = strings.TrimSpace(executionID)
	if !domain.ValidAgentID(executionID) {
		return domain.FindingReport{}, false, apperror.New(
			apperror.CodeInvalidArgument, "read-only fan-out execution id is invalid")
	}
	var runID string
	if err := s.db.QueryRowContext(ctx, `SELECT run_id FROM readonly_fanout_executions
		WHERE id = ?`, executionID).Scan(&runID); err != nil {
		return domain.FindingReport{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return domain.FindingReport{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := acquireReadOnlyFanoutWriteLockTx(ctx, tx, runID); err != nil {
		return domain.FindingReport{}, false, err
	}
	record, err := getReadOnlyFanoutExecutionRecord(ctx, tx, executionID)
	if err != nil {
		return domain.FindingReport{}, false, err
	}
	if record.Execution.Status != domain.ReadOnlyFanoutExecutionCompleted ||
		record.Execution.FinishedAt == nil || record.Execution.RunID != runID {
		return domain.FindingReport{}, false, apperror.New(
			apperror.CodeFailedPrecondition,
			"finding report requires a completed read-only fan-out execution")
	}
	var existingID, existingStatus string
	err = tx.QueryRowContext(ctx, `SELECT id, status FROM finding_reports
		WHERE source_kind = ? AND source_id = ?`,
		domain.FindingReportSourceReadOnlyFanoutExecution, executionID).
		Scan(&existingID, &existingStatus)
	if err == nil {
		if existingStatus != string(domain.FindingReportGenerated) {
			return domain.FindingReport{}, false, apperror.New(
				apperror.CodeConflict, "finding report is not in a durable generated state")
		}
		existing, err := getFindingReport(ctx, tx, existingID)
		if err != nil {
			return domain.FindingReport{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.FindingReport{}, false, err
		}
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.FindingReport{}, false, err
	}
	sources, err := listReadOnlyFanoutSourceFindings(ctx, tx, executionID)
	if err != nil {
		return domain.FindingReport{}, false, err
	}
	projected, err := reporting.ProjectReadOnlyFanout(record.Execution, sources)
	if err != nil {
		return domain.FindingReport{}, false, apperror.Wrap(
			apperror.CodeConflict, "read-only fan-out finding projection failed", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding_reports
		(id, run_id, source_kind, source_id, protocol_version, status, title,
		projection_digest, finding_count, evidence_count, info_count, low_count,
		medium_count, high_count, critical_count, version, created_at)
		VALUES (?, ?, ?, ?, ?, 'building', ?, '', 0, 0, 0, 0, 0, 0, 0, 1, ?)`,
		projected.ID, projected.RunID, projected.SourceKind, projected.SourceID,
		projected.ProtocolVersion, projected.Title, ts(projected.CreatedAt)); err != nil {
		return domain.FindingReport{}, false, err
	}
	for _, finding := range projected.Findings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO findings
			(id, report_id, run_id, ordinal, fingerprint, status, severity, category,
			title, detail, relative_path, line_start, line_end, confidence, version,
			created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			finding.ID, finding.ReportID, finding.RunID, finding.Ordinal,
			finding.Fingerprint, finding.Status, finding.Severity, finding.Category,
			finding.Title, finding.Detail, finding.RelativePath, finding.LineStart,
			finding.LineEnd, finding.Confidence, finding.Version,
			ts(finding.CreatedAt)); err != nil {
			return domain.FindingReport{}, false, err
		}
		for _, evidence := range finding.Evidence {
			if _, err := tx.ExecContext(ctx, `INSERT INTO finding_evidence
				(id, report_id, finding_id, run_id, ordinal, kind, source_kind,
				source_id, source_shard, source_ordinal, source_fingerprint,
				source_digest, relative_path, line_start, line_end, confidence, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				evidence.ID, evidence.ReportID, evidence.FindingID, evidence.RunID,
				evidence.Ordinal, evidence.Kind, evidence.SourceKind, evidence.SourceID,
				evidence.SourceShard, evidence.SourceOrdinal, evidence.SourceFingerprint,
				evidence.SourceDigest, evidence.RelativePath, evidence.LineStart,
				evidence.LineEnd, evidence.Confidence, ts(evidence.CreatedAt)); err != nil {
				return domain.FindingReport{}, false, err
			}
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE finding_reports SET status = 'generated',
		projection_digest = ?, finding_count = ?, evidence_count = ?, info_count = ?,
		low_count = ?, medium_count = ?, high_count = ?, critical_count = ?, version = 2
		WHERE id = ? AND status = 'building' AND version = 1`,
		projected.ProjectionDigest, projected.FindingCount, projected.EvidenceCount,
		projected.Severity.Info, projected.Severity.Low, projected.Severity.Medium,
		projected.Severity.High, projected.Severity.Critical, projected.ID)
	if err != nil {
		return domain.FindingReport{}, false, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return domain.FindingReport{}, false, err
		}
		return domain.FindingReport{}, false, apperror.New(
			apperror.CodeConflict, "finding report generation lost its race")
	}
	run, _, err := getCoordinatorRunTx(ctx, tx, projected.RunID)
	if err != nil {
		return domain.FindingReport{}, false, err
	}
	if err := appendSupervisorEventTx(ctx, tx, run, events.FindingReportGeneratedEvent,
		"report", projected.ID, map[string]any{
			"report_id": projected.ID, "source_kind": projected.SourceKind,
			"source_id": projected.SourceID, "protocol": projected.ProtocolVersion,
			"projection_digest": projected.ProjectionDigest,
			"finding_count":     projected.FindingCount,
			"evidence_count":    projected.EvidenceCount,
			"severity": map[string]int{
				"info": projected.Severity.Info, "low": projected.Severity.Low,
				"medium": projected.Severity.Medium, "high": projected.Severity.High,
				"critical": projected.Severity.Critical,
			},
		}); err != nil {
		return domain.FindingReport{}, false, err
	}
	stored, err := getFindingReport(ctx, tx, projected.ID)
	if err != nil {
		return domain.FindingReport{}, false, err
	}
	if stored.ProjectionDigest != projected.ProjectionDigest {
		return domain.FindingReport{}, false, apperror.New(
			apperror.CodeConflict, "stored finding report projection drifted")
	}
	if err := tx.Commit(); err != nil {
		return domain.FindingReport{}, false, err
	}
	return stored, false, nil
}

func (s *SQLiteStore) GetFindingReport(ctx context.Context,
	id string,
) (domain.FindingReport, error) {
	id = strings.TrimSpace(id)
	if !domain.ValidAgentID(id) {
		return domain.FindingReport{}, apperror.New(
			apperror.CodeInvalidArgument, "finding report id is invalid")
	}
	return getFindingReport(ctx, s.db, id)
}

func getFindingReport(ctx context.Context, queryer readOnlyFanoutQueryer,
	id string,
) (domain.FindingReport, error) {
	report, err := scanFindingReport(queryer.QueryRowContext(ctx,
		findingReportSelect+` WHERE id = ? AND status = 'generated'`, id))
	if err != nil {
		return domain.FindingReport{}, err
	}
	rows, err := queryer.QueryContext(ctx, findingSelect+
		` WHERE report_id = ? ORDER BY ordinal`, id)
	if err != nil {
		return domain.FindingReport{}, err
	}
	for rows.Next() {
		finding, err := scanFinding(rows)
		if err != nil {
			_ = rows.Close()
			return domain.FindingReport{}, err
		}
		report.Findings = append(report.Findings, finding)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return domain.FindingReport{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.FindingReport{}, err
	}
	evidenceRows, err := queryer.QueryContext(ctx, findingEvidenceSelect+
		` WHERE report_id = ? ORDER BY finding_id, ordinal`, id)
	if err != nil {
		return domain.FindingReport{}, err
	}
	evidenceByFinding := make(map[string][]domain.FindingEvidence, len(report.Findings))
	for evidenceRows.Next() {
		evidence, err := scanFindingEvidence(evidenceRows)
		if err != nil {
			_ = evidenceRows.Close()
			return domain.FindingReport{}, err
		}
		evidenceByFinding[evidence.FindingID] = append(
			evidenceByFinding[evidence.FindingID], evidence)
	}
	if err := evidenceRows.Err(); err != nil {
		_ = evidenceRows.Close()
		return domain.FindingReport{}, err
	}
	if err := evidenceRows.Close(); err != nil {
		return domain.FindingReport{}, err
	}
	for index := range report.Findings {
		report.Findings[index].Evidence = evidenceByFinding[report.Findings[index].ID]
	}
	if err := report.Validate(); err != nil {
		return domain.FindingReport{}, apperror.Wrap(
			apperror.CodeConflict, "stored finding report is invalid", err)
	}
	return report, nil
}

func listReadOnlyFanoutSourceFindings(ctx context.Context,
	queryer readOnlyFanoutQueryer, executionID string,
) ([]reporting.ReadOnlyFanoutSourceFinding, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT finding.shard_ordinal,
		finding.ordinal, finding.fingerprint, shard.report_digest, finding.severity,
		finding.category, finding.title, finding.detail, finding.relative_path,
		finding.line_start, finding.line_end, finding.confidence
		FROM readonly_fanout_findings finding
		JOIN readonly_fanout_execution_shards shard
			ON shard.execution_id = finding.execution_id
			AND shard.ordinal = finding.shard_ordinal
		WHERE finding.execution_id = ?
		ORDER BY finding.shard_ordinal, finding.ordinal`, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]reporting.ReadOnlyFanoutSourceFinding, 0)
	for rows.Next() {
		var source reporting.ReadOnlyFanoutSourceFinding
		if err := rows.Scan(&source.ShardOrdinal, &source.Ordinal, &source.Fingerprint,
			&source.ReportDigest, &source.Finding.Severity, &source.Finding.Category,
			&source.Finding.Title, &source.Finding.Detail, &source.Finding.Path,
			&source.Finding.LineStart, &source.Finding.LineEnd,
			&source.Finding.Confidence); err != nil {
			return nil, err
		}
		result = append(result, source)
	}
	return result, rows.Err()
}

func scanFindingReport(row scanner) (domain.FindingReport, error) {
	var value domain.FindingReport
	var createdAt string
	if err := row.Scan(&value.ID, &value.RunID, &value.SourceKind, &value.SourceID,
		&value.ProtocolVersion, &value.Status, &value.Title, &value.ProjectionDigest,
		&value.FindingCount, &value.EvidenceCount, &value.Severity.Info,
		&value.Severity.Low, &value.Severity.Medium, &value.Severity.High,
		&value.Severity.Critical, &value.Version, &createdAt); err != nil {
		return domain.FindingReport{}, err
	}
	value.CreatedAt = parseTS(createdAt)
	value.Findings = []domain.Finding{}
	return value, nil
}

func scanFinding(row scanner) (domain.Finding, error) {
	var value domain.Finding
	var createdAt string
	if err := row.Scan(&value.ID, &value.ReportID, &value.RunID, &value.Ordinal,
		&value.Fingerprint, &value.Status, &value.Severity, &value.Category,
		&value.Title, &value.Detail, &value.RelativePath, &value.LineStart,
		&value.LineEnd, &value.Confidence, &value.Version, &createdAt); err != nil {
		return domain.Finding{}, err
	}
	value.CreatedAt = parseTS(createdAt)
	value.Evidence = []domain.FindingEvidence{}
	return value, nil
}

func scanFindingEvidence(row scanner) (domain.FindingEvidence, error) {
	var value domain.FindingEvidence
	var createdAt string
	if err := row.Scan(&value.ID, &value.ReportID, &value.FindingID, &value.RunID,
		&value.Ordinal, &value.Kind, &value.SourceKind, &value.SourceID,
		&value.SourceShard, &value.SourceOrdinal, &value.SourceFingerprint,
		&value.SourceDigest, &value.RelativePath, &value.LineStart, &value.LineEnd,
		&value.Confidence, &createdAt); err != nil {
		return domain.FindingEvidence{}, err
	}
	value.CreatedAt = parseTS(createdAt)
	return value, nil
}
