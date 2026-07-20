package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/verification"
)

func TestVerificationSnapshotExportIsDeterministicBoundedAndBodyless(t *testing.T) {
	plan := validCoverageBoundaryPlan()
	digest := strings.Repeat("b", 64)
	createdAt := time.Date(2026, time.July, 20, 1, 2, 3, 0, time.UTC)
	associations := []verification.PlanEvidenceAssociation{
		{ID: "association-2", ProtocolVersion: verification.PlanEvidenceAssociationProtocolVersion,
			OperationKeyDigest: digest, RequestFingerprint: digest, RunID: "run-1",
			SessionID: "session-1", WorkspaceID: "workspace-1", PlanID: plan.ID,
			PlanItemOrdinal: 1, PlanItemSHA256: plan.Items[0].ItemSHA256,
			EvidenceID: "evidence-2", EvidenceOutcome: verification.OutcomeFail,
			EvidenceEventSequence: 8, AssociatedBy: "operator", EventSequence: 9,
			CreatedAt: createdAt},
		{ID: "association-1", ProtocolVersion: verification.PlanEvidenceAssociationProtocolVersion,
			OperationKeyDigest: digest, RequestFingerprint: digest, RunID: "run-1",
			SessionID: "session-1", WorkspaceID: "workspace-1", PlanID: plan.ID,
			PlanItemOrdinal: 1, PlanItemSHA256: plan.Items[0].ItemSHA256,
			EvidenceID: "evidence-1", EvidenceOutcome: verification.OutcomePass,
			EvidenceEventSequence: 6, AssociatedBy: "operator", EventSequence: 7,
			CreatedAt: createdAt.Add(-time.Minute)},
	}
	store := verificationCoverageBoundaryStore{plans: []verification.Plan{plan},
		counts: []verification.PlanItemCoverageCount{{PlanID: plan.ID, PlanItemOrdinal: 1,
			PlanItemSHA256: plan.Items[0].ItemSHA256, AssociatedEvidenceCount: 2,
			PassCount: 1, FailCount: 1, LatestAssociationEventSequence: 9}},
		associations: associations}
	service := NewVerificationSnapshotExportService(store)
	first, err := service.Build(t.Context(), "run-1", plan.ID, 1,
		VerificationSnapshotExportFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Build(t.Context(), "run-1", plan.ID, 1,
		VerificationSnapshotExportFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	digestBytes := sha256.Sum256([]byte(first.Content))
	if first != second || first.ProtocolVersion != VerificationSnapshotExportProtocolVersion ||
		first.SnapshotProtocolVersion != VerificationSnapshotProtocolVersion ||
		first.SnapshotHighWaterEventSequence != 9 || first.AssociatedEvidenceCount != 2 ||
		first.PassCount != 1 || first.FailCount != 1 || first.UnknownCount != 0 ||
		first.ReturnedAssociationCount != 2 || first.AssociationsTruncated ||
		first.ContentBytes != len(first.Content) ||
		first.ContentSHA256 != hex.EncodeToString(digestBytes[:]) || !first.MetadataOnly ||
		!first.ReadOnly || !first.DownloadOnly || first.PrivatePlanBodyIncluded ||
		first.PrivateEvidenceBodiesIncluded || first.OperatorIdentityIncluded ||
		first.ResultInferred || first.CommandExecuted || first.ModelAssertion ||
		first.RecordRewritten || first.Approval || first.AuthorityGranted ||
		first.MutationSupported || first.ExecutionStarted {
		t.Fatalf("verification snapshot export widened authority: %#v", first)
	}
	var snapshot VerificationPlanItemSnapshot
	if err := json.Unmarshal([]byte(first.Content), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ProtocolVersion != VerificationSnapshotProtocolVersion ||
		snapshot.RunID != "run-1" || snapshot.PlanID != plan.ID ||
		snapshot.SnapshotHighWaterEventSequence != 9 || len(snapshot.Associations) != 2 ||
		!snapshot.MetadataOnly || !snapshot.ReadOnly || snapshot.ResultInferred ||
		snapshot.PrivatePlanBodyIncluded || snapshot.PrivateEvidenceBodiesIncluded ||
		snapshot.OperatorIdentityIncluded || snapshot.CommandExecuted || snapshot.ModelAssertion ||
		snapshot.RecordRewritten || snapshot.Approval || snapshot.AuthorityGranted ||
		snapshot.MutationSupported || snapshot.ExecutionStarted ||
		strings.Contains(first.Content, plan.Title) || strings.Contains(first.Content, plan.Summary) ||
		strings.Contains(first.Content, `"associated_by"`) ||
		strings.Contains(first.Content, `"authored_by"`) ||
		strings.Contains(first.Content, `"recorded_by"`) {
		t.Fatalf("verification snapshot content exposed a private body or authority: %#v", snapshot)
	}
	markdown, err := service.Build(t.Context(), "run-1", plan.ID, 1,
		VerificationSnapshotExportFormatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if markdown.MIMEType != "text/markdown; charset=utf-8" ||
		!strings.HasSuffix(markdown.Filename, ".md") ||
		!strings.Contains(markdown.Content, "Snapshot event high-water: `9`") ||
		!strings.Contains(markdown.Content, "infers no result") ||
		strings.Contains(markdown.Content, plan.Title) || strings.Contains(markdown.Content, plan.Summary) {
		t.Fatalf("verification Markdown snapshot lost its boundary: %#v", markdown)
	}
	if _, err := service.Build(t.Context(), "run-1", plan.ID, 1, "text"); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("invalid export format code=%s err=%v", apperror.CodeOf(err), err)
	}
}
