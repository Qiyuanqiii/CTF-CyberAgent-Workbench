package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/verification"
)

const (
	VerificationSnapshotExportProtocolVersion = "operator_verification_plan_item_snapshot_export.v1"
	VerificationSnapshotProtocolVersion       = "operator_verification_plan_item_snapshot.v1"
	VerificationSnapshotExportFormatJSON      = "json"
	VerificationSnapshotExportFormatMarkdown  = "markdown"
	MaxVerificationSnapshotExportBytes        = 256 * 1024
)

type VerificationSnapshotReference struct {
	ID                       string               `json:"id"`
	PlanID                   string               `json:"plan_id"`
	PlanItemOrdinal          int                  `json:"plan_item_ordinal"`
	PlanItemSHA256           string               `json:"plan_item_sha256"`
	EvidenceID               string               `json:"evidence_id"`
	EvidenceOutcome          verification.Outcome `json:"evidence_outcome"`
	EvidenceEventSequence    int64                `json:"evidence_event_sequence"`
	AssociationEventSequence int64                `json:"association_event_sequence"`
	AssociatedAt             time.Time            `json:"associated_at"`
}

type VerificationPlanItemSnapshot struct {
	ProtocolVersion                string                          `json:"protocol_version"`
	RunID                          string                          `json:"run_id"`
	SessionID                      string                          `json:"session_id"`
	WorkspaceID                    string                          `json:"workspace_id"`
	PlanID                         string                          `json:"plan_id"`
	PlanSHA256                     string                          `json:"plan_sha256"`
	PlanItemOrdinal                int                             `json:"plan_item_ordinal"`
	PlanItemSHA256                 string                          `json:"plan_item_sha256"`
	SnapshotHighWaterEventSequence int64                           `json:"snapshot_high_water_event_sequence"`
	AssociatedEvidenceCount        int                             `json:"associated_evidence_count"`
	PassCount                      int                             `json:"pass_count"`
	FailCount                      int                             `json:"fail_count"`
	UnknownCount                   int                             `json:"unknown_count"`
	ReturnedAssociationCount       int                             `json:"returned_association_count"`
	AssociationsTruncated          bool                            `json:"associations_truncated"`
	Associations                   []VerificationSnapshotReference `json:"associations"`
	MetadataOnly                   bool                            `json:"metadata_only"`
	ReadOnly                       bool                            `json:"read_only"`
	PrivatePlanBodyIncluded        bool                            `json:"private_plan_body_included"`
	PrivateEvidenceBodiesIncluded  bool                            `json:"private_evidence_bodies_included"`
	OperatorIdentityIncluded       bool                            `json:"operator_identity_included"`
	ResultInferred                 bool                            `json:"result_inferred"`
	CommandExecuted                bool                            `json:"command_executed"`
	ModelAssertion                 bool                            `json:"model_assertion"`
	RecordRewritten                bool                            `json:"record_rewritten"`
	Approval                       bool                            `json:"approval"`
	AuthorityGranted               bool                            `json:"authority_granted"`
	MutationSupported              bool                            `json:"mutation_supported"`
	ExecutionStarted               bool                            `json:"execution_started"`
}

type VerificationSnapshotExport struct {
	ProtocolVersion                string
	SnapshotProtocolVersion        string
	Format                         string
	Filename                       string
	MIMEType                       string
	RunID                          string
	SessionID                      string
	WorkspaceID                    string
	PlanID                         string
	PlanSHA256                     string
	PlanItemOrdinal                int
	PlanItemSHA256                 string
	SnapshotHighWaterEventSequence int64
	AssociatedEvidenceCount        int
	PassCount                      int
	FailCount                      int
	UnknownCount                   int
	ReturnedAssociationCount       int
	AssociationsTruncated          bool
	ContentSHA256                  string
	ContentBytes                   int
	Content                        string
	MetadataOnly                   bool
	ReadOnly                       bool
	DownloadOnly                   bool
	PrivatePlanBodyIncluded        bool
	PrivateEvidenceBodiesIncluded  bool
	OperatorIdentityIncluded       bool
	ResultInferred                 bool
	CommandExecuted                bool
	ModelAssertion                 bool
	RecordRewritten                bool
	Approval                       bool
	AuthorityGranted               bool
	MutationSupported              bool
	ExecutionStarted               bool
}

type VerificationSnapshotExportService struct {
	detail *VerificationCoverageDetailService
}

func NewVerificationSnapshotExportService(
	store VerificationCoverageDetailStore,
) *VerificationSnapshotExportService {
	return &VerificationSnapshotExportService{detail: NewVerificationCoverageDetailService(store)}
}

func (s *VerificationSnapshotExportService) Build(ctx context.Context, runID string,
	planID string, ordinal int, format string,
) (VerificationSnapshotExport, error) {
	if s == nil || s.detail == nil {
		return VerificationSnapshotExport{}, apperror.New(apperror.CodeFailedPrecondition,
			"verification snapshot export service is required")
	}
	if format != strings.TrimSpace(format) ||
		(format != VerificationSnapshotExportFormatJSON &&
			format != VerificationSnapshotExportFormatMarkdown) {
		return VerificationSnapshotExport{}, apperror.New(apperror.CodeInvalidArgument,
			"verification snapshot export format must be json or markdown")
	}
	detail, err := s.detail.Detail(ctx, runID, planID, ordinal)
	if err != nil {
		return VerificationSnapshotExport{}, err
	}
	if detail.ProtocolVersion != verification.PlanItemCoverageProtocolVersion ||
		detail.RunID != runID || detail.PlanID != planID || detail.PlanItemOrdinal != ordinal ||
		detail.SnapshotHighWaterEventSequence != detail.LatestAssociationEventSequence ||
		detail.AssociatedEvidenceCount != detail.PassCount+detail.FailCount+detail.UnknownCount ||
		len(detail.Associations) > verification.MaxCoverageAssociations ||
		detail.AssociationsTruncated != (len(detail.Associations) < detail.AssociatedEvidenceCount) ||
		!detail.MetadataOnly || !detail.ReadOnly || detail.PrivatePlanBodyIncluded ||
		detail.PrivateEvidenceBodiesIncluded || detail.OperatorIdentityIncluded ||
		detail.ResultInferred || detail.CommandExecuted || detail.ModelAssertion ||
		detail.RecordRewritten || detail.Approval || detail.AuthorityGranted {
		return VerificationSnapshotExport{}, apperror.New(apperror.CodeConflict,
			"verification snapshot export source widened its read-only boundary")
	}
	references := make([]VerificationSnapshotReference, len(detail.Associations))
	for index, association := range detail.Associations {
		references[index] = VerificationSnapshotReference{
			ID: association.ID, PlanID: association.PlanID,
			PlanItemOrdinal: association.PlanItemOrdinal, PlanItemSHA256: association.PlanItemSHA256,
			EvidenceID: association.EvidenceID, EvidenceOutcome: association.EvidenceOutcome,
			EvidenceEventSequence:    association.EvidenceEventSequence,
			AssociationEventSequence: association.AssociationSequence,
			AssociatedAt:             association.CreatedAt,
		}
	}
	snapshot := VerificationPlanItemSnapshot{
		ProtocolVersion: VerificationSnapshotProtocolVersion,
		RunID:           detail.RunID, SessionID: detail.SessionID, WorkspaceID: detail.WorkspaceID,
		PlanID: detail.PlanID, PlanSHA256: detail.PlanSHA256,
		PlanItemOrdinal: detail.PlanItemOrdinal, PlanItemSHA256: detail.PlanItemSHA256,
		SnapshotHighWaterEventSequence: detail.SnapshotHighWaterEventSequence,
		AssociatedEvidenceCount:        detail.AssociatedEvidenceCount,
		PassCount:                      detail.PassCount, FailCount: detail.FailCount, UnknownCount: detail.UnknownCount,
		ReturnedAssociationCount: len(references), AssociationsTruncated: detail.AssociationsTruncated,
		Associations: references, MetadataOnly: true, ReadOnly: true,
	}
	var content []byte
	mimeType := "application/json"
	extension := "json"
	if format == VerificationSnapshotExportFormatJSON {
		content, err = json.MarshalIndent(snapshot, "", "  ")
		if err == nil {
			content = append(content, '\n')
		}
	} else {
		content = []byte(renderVerificationSnapshotMarkdown(snapshot))
		mimeType = "text/markdown; charset=utf-8"
		extension = "md"
	}
	if err != nil {
		return VerificationSnapshotExport{}, apperror.Wrap(apperror.CodeInternal,
			"verification snapshot export could not be encoded", err)
	}
	if len(content) == 0 || len(content) > MaxVerificationSnapshotExportBytes {
		return VerificationSnapshotExport{}, apperror.New(apperror.CodeResourceExhausted,
			"verification snapshot export exceeds the bounded content limit")
	}
	digest := sha256.Sum256(content)
	return VerificationSnapshotExport{
		ProtocolVersion:         VerificationSnapshotExportProtocolVersion,
		SnapshotProtocolVersion: VerificationSnapshotProtocolVersion, Format: format,
		Filename: fmt.Sprintf("cyberagent-verification-snapshot-%s-%s-item-%d.%s",
			safeHandoffFilenameID(detail.RunID), safeHandoffFilenameID(detail.PlanID), ordinal, extension),
		MIMEType: mimeType, RunID: detail.RunID, SessionID: detail.SessionID,
		WorkspaceID: detail.WorkspaceID, PlanID: detail.PlanID, PlanSHA256: detail.PlanSHA256,
		PlanItemOrdinal: detail.PlanItemOrdinal, PlanItemSHA256: detail.PlanItemSHA256,
		SnapshotHighWaterEventSequence: detail.SnapshotHighWaterEventSequence,
		AssociatedEvidenceCount:        detail.AssociatedEvidenceCount,
		PassCount:                      detail.PassCount, FailCount: detail.FailCount, UnknownCount: detail.UnknownCount,
		ReturnedAssociationCount: len(references), AssociationsTruncated: detail.AssociationsTruncated,
		ContentSHA256: hex.EncodeToString(digest[:]), ContentBytes: len(content),
		Content: string(content), MetadataOnly: true, ReadOnly: true, DownloadOnly: true,
	}, nil
}

func renderVerificationSnapshotMarkdown(value VerificationPlanItemSnapshot) string {
	var output strings.Builder
	output.WriteString("# CyberAgent Verification Snapshot\n\n")
	fmt.Fprintf(&output, "- Run: `%s`\n", markdownCell(value.RunID))
	fmt.Fprintf(&output, "- Session: `%s`\n", markdownCell(value.SessionID))
	fmt.Fprintf(&output, "- Workspace: `%s`\n", markdownCell(value.WorkspaceID))
	fmt.Fprintf(&output, "- Plan: `%s`\n", markdownCell(value.PlanID))
	fmt.Fprintf(&output, "- Plan SHA-256: `%s`\n", markdownCell(value.PlanSHA256))
	fmt.Fprintf(&output, "- Item: `%d`\n", value.PlanItemOrdinal)
	fmt.Fprintf(&output, "- Item SHA-256: `%s`\n", markdownCell(value.PlanItemSHA256))
	fmt.Fprintf(&output, "- Snapshot event high-water: `%d`\n\n",
		value.SnapshotHighWaterEventSequence)
	output.WriteString("## Explicit Observations\n\n")
	output.WriteString("| Associated | Pass | Fail | Unknown | Returned | Truncated |\n")
	output.WriteString("| ---: | ---: | ---: | ---: | ---: | --- |\n")
	fmt.Fprintf(&output, "| %d | %d | %d | %d | %d | %t |\n\n",
		value.AssociatedEvidenceCount, value.PassCount, value.FailCount, value.UnknownCount,
		value.ReturnedAssociationCount, value.AssociationsTruncated)
	if len(value.Associations) == 0 {
		output.WriteString("No explicit evidence associations are present in this snapshot.\n\n")
	} else {
		output.WriteString("| Association | Evidence | Outcome | Evidence event | Association event | Associated |\n")
		output.WriteString("| --- | --- | --- | ---: | ---: | --- |\n")
		for _, association := range value.Associations {
			fmt.Fprintf(&output, "| `%s` | `%s` | %s | %d | %d | %s |\n",
				markdownCell(association.ID), markdownCell(association.EvidenceID),
				markdownCell(string(association.EvidenceOutcome)), association.EvidenceEventSequence,
				association.AssociationEventSequence,
				association.AssociatedAt.UTC().Format(time.RFC3339Nano))
		}
		output.WriteByte('\n')
	}
	output.WriteString("## Safety Boundary\n\n")
	output.WriteString("This is a read-only metadata snapshot. It contains no private plan or evidence bodies, identifies no operator, infers no result, grants no approval or authority, mutates no record, and starts no execution.\n")
	return output.String()
}
