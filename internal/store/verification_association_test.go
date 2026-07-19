package store

import (
	"path/filepath"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/verification"
)

func TestVerificationAssociationIsCausalImmutableAndCoverageDoesNotInferResult(t *testing.T) {
	ctx := t.Context()
	state, err := Open(filepath.Join(t.TempDir(), "verification-association.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	workspace := WorkspaceRecord{ID: "workspace-verification-association",
		Name: "verification-association", RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "associate explicit verification facts",
			Profile: "code", WorkspaceID: workspace.ID,
			Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	planResult, err := application.NewVerificationPlanService(state).Record(ctx,
		application.RecordVerificationPlanRequest{
			Version: verification.PlanProtocolVersion, RunID: run.ID,
			Title: "Release checks", Summary: "Operator-authored guidance",
			Items: []application.VerificationPlanItemRequest{
				{Title: "Focused suite", ExpectedObservation: "Observe the focused suite"},
				{Title: "Boundary review", ExpectedObservation: "Observe no authority widening"},
			}, OperationKey: "verification-association-plan-operation-0001",
			AuthoredBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	evidenceService := application.NewVerificationEvidenceService(state)
	passEvidence, err := evidenceService.Record(ctx, application.RecordVerificationEvidenceRequest{
		Version: verification.EvidenceProtocolVersion, RunID: run.ID,
		Outcome: string(verification.OutcomePass), Title: "Focused observation",
		Summary: "The focused suite completed", OperationKey: "association-evidence-pass-0001",
		RecordedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	associationService := application.NewVerificationAssociationService(state)
	request := application.RecordVerificationAssociationRequest{
		Version: verification.PlanEvidenceAssociationProtocolVersion, RunID: run.ID,
		PlanID: planResult.Plan.ID, PlanItemOrdinal: 1, EvidenceID: passEvidence.Evidence.ID,
		OperationKey: "verification-association-operation-0001", AssociatedBy: "operator",
	}
	associated, err := associationService.Record(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if associated.Replayed || associated.Association.EventSequence <=
		passEvidence.Evidence.EventSequence || passEvidence.Evidence.EventSequence <=
		planResult.Plan.EventSequence {
		t.Fatalf("association did not preserve causal event order: %#v", associated)
	}
	replayed, err := associationService.Record(ctx, request)
	if err != nil || !replayed.Replayed || replayed.Association.ID != associated.Association.ID {
		t.Fatalf("association replay diverged: %#v err=%v", replayed, err)
	}
	changed := request
	changed.PlanItemOrdinal = 2
	if _, err := associationService.Record(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("same key accepted changed association intent: %v", err)
	}
	changed.OperationKey = "verification-association-operation-0002"
	if _, err := associationService.Record(ctx, changed); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("one evidence was associated to two plan items: %v", err)
	}

	failEvidence, err := evidenceService.Record(ctx, application.RecordVerificationEvidenceRequest{
		Version: verification.EvidenceProtocolVersion, RunID: run.ID,
		Outcome: string(verification.OutcomeFail), Title: "Focused regression",
		Summary: "A later explicit observation failed", OperationKey: "association-evidence-fail-0001",
		RecordedBy: "operator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := associationService.Record(ctx, application.RecordVerificationAssociationRequest{
		Version: verification.PlanEvidenceAssociationProtocolVersion, RunID: run.ID,
		PlanID: planResult.Plan.ID, PlanItemOrdinal: 1, EvidenceID: failEvidence.Evidence.ID,
		OperationKey: "verification-association-operation-0003", AssociatedBy: "operator",
	}); err != nil {
		t.Fatal(err)
	}
	coverage, err := associationService.Coverage(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.ProtocolVersion != verification.PlanCoverageProtocolVersion ||
		coverage.PlanCount != 1 || coverage.PlanItemCount != 2 ||
		coverage.ObservedPlanItemCount != 1 || coverage.AssociatedEvidenceCount != 2 ||
		len(coverage.Plans) != 1 || len(coverage.Plans[0].Items) != 2 ||
		coverage.Plans[0].ObservedItemCount != 1 ||
		coverage.Plans[0].AssociatedEvidenceCount != 2 ||
		coverage.Plans[0].Items[0].PassCount != 1 ||
		coverage.Plans[0].Items[0].FailCount != 1 ||
		coverage.Plans[0].Items[0].UnknownCount != 0 ||
		coverage.Plans[0].Items[1].AssociatedEvidenceCount != 0 ||
		len(coverage.Associations) != 2 || !coverage.MetadataOnly || !coverage.ReadOnly ||
		coverage.ResultInferred || coverage.CommandExecuted || coverage.ModelAssertion ||
		coverage.RecordRewritten || coverage.Approval || coverage.AuthorityGranted {
		t.Fatalf("coverage inferred or widened explicit facts: %#v", coverage)
	}
	handoff, err := application.NewCodeHandoffService(state).Build(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	projected := handoff.VerificationCoverage
	if projected.ProtocolVersion != verification.PlanCoverageProtocolVersion ||
		projected.PlanCount != 1 || projected.PlanItemCount != 2 ||
		projected.ObservedPlanItemCount != 1 || projected.UnobservedPlanItemCount != 1 ||
		projected.AssociatedEvidenceCount != 2 || projected.ContradictoryItemCount != 1 ||
		projected.ReturnedItemCount != 2 || len(projected.Items) != 2 || projected.Truncated ||
		projected.Items[0].PlanID != planResult.Plan.ID ||
		projected.Items[0].PassCount != 1 || projected.Items[0].FailCount != 1 ||
		projected.Items[0].UnknownCount != 0 ||
		projected.Items[1].AssociatedEvidenceCount != 0 || !projected.MetadataOnly ||
		!projected.ReadOnly || projected.ResultInferred || projected.PrivateBodiesIncluded {
		t.Fatalf("handoff coverage lost contradictions or widened authority: %#v", projected)
	}
	for _, format := range []string{application.CodeHandoffExportFormatMarkdown,
		application.CodeHandoffExportFormatJSON} {
		exported, err := application.NewCodeHandoffExportService(state).Build(ctx, run.ID, format)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(exported.Content, "Release checks") ||
			strings.Contains(exported.Content, "Focused suite") ||
			strings.Contains(exported.Content, "later explicit observation") ||
			!strings.Contains(exported.Content, planResult.Plan.ID) {
			t.Fatalf("%s handoff coverage exposed private text or lost identity: %s",
				format, exported.Content)
		}
		if format == application.CodeHandoffExportFormatMarkdown &&
			!strings.Contains(exported.Content,
				"1/2 items observed, 2 explicit associations, 1 contradictory items") {
			t.Fatalf("Markdown handoff omitted explicit contradiction counts: %s", exported.Content)
		}
		if format == application.CodeHandoffExportFormatJSON &&
			(!strings.Contains(exported.Content, `"contradictory_item_count": 1`) ||
				strings.Contains(exported.Content, `"aggregate_result"`)) {
			t.Fatalf("JSON handoff inferred or omitted coverage: %s", exported.Content)
		}
	}
	if _, err := state.db.ExecContext(ctx,
		`UPDATE operator_verification_plan_evidence_associations SET plan_item_ordinal = 2
		WHERE id = ?`, associated.Association.ID); err == nil {
		t.Fatal("verification association update was accepted")
	}
	if _, err := state.db.ExecContext(ctx,
		`DELETE FROM operator_verification_plan_evidence_associations WHERE id = ?`,
		associated.Association.ID); err == nil {
		t.Fatal("verification association delete was accepted")
	}
	timeline, err := state.ListRunEvents(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	eventCount := 0
	for _, event := range timeline {
		if event.Type != events.VerificationPlanEvidenceAssociatedEvent {
			continue
		}
		eventCount++
		if strings.Contains(event.PayloadJSON, "Focused suite") ||
			strings.Contains(event.PayloadJSON, "focused suite completed") ||
			!strings.Contains(event.PayloadJSON, `"operator_associated":true`) ||
			!strings.Contains(event.PayloadJSON, `"result_inferred":false`) ||
			!strings.Contains(event.PayloadJSON, `"record_rewritten":false`) ||
			!strings.Contains(event.PayloadJSON, `"authority_granted":false`) {
			t.Fatalf("association event leaked content or authority: %s", event.PayloadJSON)
		}
	}
	if eventCount != 2 {
		t.Fatalf("association event count=%d, want 2", eventCount)
	}
}

func TestVerificationAssociationRejectsEvidenceRecordedBeforePlan(t *testing.T) {
	ctx := t.Context()
	state, err := Open(filepath.Join(t.TempDir(), "verification-association-order.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	workspace := WorkspaceRecord{ID: "workspace-association-order", Name: "association-order",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "reject reverse causal association", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := application.NewVerificationEvidenceService(state).Record(ctx,
		application.RecordVerificationEvidenceRequest{Version: verification.EvidenceProtocolVersion,
			RunID: run.ID, Outcome: string(verification.OutcomeUnknown), Title: "Early evidence",
			Summary: "Recorded before the plan", OperationKey: "early-evidence-operation-0001",
			RecordedBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := application.NewVerificationPlanService(state).Record(ctx,
		application.RecordVerificationPlanRequest{Version: verification.PlanProtocolVersion,
			RunID: run.ID, Title: "Later plan", Summary: "Cannot claim the older observation",
			Items: []application.VerificationPlanItemRequest{{Title: "Later check",
				ExpectedObservation: "Observe a later fact"}},
			OperationKey: "later-plan-operation-0001", AuthoredBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = application.NewVerificationAssociationService(state).Record(ctx,
		application.RecordVerificationAssociationRequest{
			Version: verification.PlanEvidenceAssociationProtocolVersion, RunID: run.ID,
			PlanID: plan.Plan.ID, PlanItemOrdinal: 1, EvidenceID: evidence.Evidence.ID,
			OperationKey: "reverse-causal-association-0001", AssociatedBy: "operator",
		})
	if apperror.CodeOf(err) != apperror.CodeFailedPrecondition {
		t.Fatalf("reverse-causal association returned %v", err)
	}
	values, listErr := state.ListVerificationPlanEvidenceAssociations(ctx, run.ID, 1)
	if listErr != nil || len(values) != 0 {
		t.Fatalf("reverse-causal association left a record: %#v err=%v", values, listErr)
	}
}

func TestSchemaV81UpgradeFabricatesNoVerificationAssociation(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "v80.db")
	state, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	workspace := WorkspaceRecord{ID: "workspace-v81-upgrade", Name: "v81-upgrade",
		RootPath: t.TempDir()}
	if err := state.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(state).Create(ctx,
		application.CreateRunRequest{Goal: "preserve facts across v81", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewVerificationPlanService(state).Record(ctx,
		application.RecordVerificationPlanRequest{Version: verification.PlanProtocolVersion,
			RunID: run.ID, Title: "Legacy plan", Summary: "No fabricated association",
			Items: []application.VerificationPlanItemRequest{{Title: "Legacy check",
				ExpectedObservation: "Observe nothing synthesized"}},
			OperationKey: "v81-upgrade-plan-operation-0001", AuthoredBy: "operator"}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range append(removeSchemaV82ForTestStatements(), []string{
		`DROP TRIGGER trg_operator_verification_association_delete_immutable`,
		`DROP TRIGGER trg_operator_verification_association_update_immutable`,
		`DROP TRIGGER trg_operator_verification_association_insert`,
		`DROP TABLE operator_verification_plan_evidence_associations`,
		`DELETE FROM schema_migrations WHERE version = 81`,
	}...) {
		if _, err := state.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	values, err := state.ListVerificationPlanEvidenceAssociations(ctx, run.ID, 1)
	if err != nil || len(values) != 0 {
		t.Fatalf("v81 upgrade fabricated an association: %#v err=%v", values, err)
	}
	plans, err := state.ListVerificationPlans(ctx, run.ID, 2)
	if err != nil || len(plans) != 1 {
		t.Fatalf("v81 upgrade did not preserve the plan: %#v err=%v", plans, err)
	}
}
