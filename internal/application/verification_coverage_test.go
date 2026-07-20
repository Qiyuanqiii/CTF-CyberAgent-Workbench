package application

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/verification"
)

type verificationCoverageBoundaryStore struct {
	plans        []verification.Plan
	counts       []verification.PlanItemCoverageCount
	associations []verification.PlanEvidenceAssociation
}

func (s verificationCoverageBoundaryStore) GetVerificationPlan(context.Context,
	string,
) (verification.Plan, error) {
	if len(s.plans) == 0 {
		return verification.Plan{}, apperror.New(apperror.CodeNotFound,
			"verification plan was not found")
	}
	return s.plans[0], nil
}

func (s verificationCoverageBoundaryStore) GetRun(context.Context, string) (domain.Run, error) {
	return domain.Run{ID: "run-1", MissionID: "mission-1", SessionID: "session-1"}, nil
}

func (s verificationCoverageBoundaryStore) GetMission(context.Context,
	string,
) (domain.Mission, error) {
	return domain.Mission{ID: "mission-1", WorkspaceID: "workspace-1"}, nil
}

func (s verificationCoverageBoundaryStore) GetRunMode(context.Context,
	string,
) (domain.RunModeSnapshot, error) {
	return domain.RunModeSnapshot{RunID: "run-1", Surface: domain.ExecutionSurfaceCode}, nil
}

func (s verificationCoverageBoundaryStore) GetSession(context.Context,
	string,
) (session.Session, error) {
	return session.Session{ID: "session-1", WorkspaceID: "workspace-1"}, nil
}

func (s verificationCoverageBoundaryStore) GetWorkspaceInfo(context.Context,
	string,
) (session.WorkspaceInfo, error) {
	return session.WorkspaceInfo{ID: "workspace-1"}, nil
}

func (s verificationCoverageBoundaryStore) ListVerificationPlans(context.Context,
	string, int,
) ([]verification.Plan, error) {
	return append([]verification.Plan(nil), s.plans...), nil
}

func (s verificationCoverageBoundaryStore) ListVerificationPlanEvidenceAssociations(
	context.Context, string, int,
) ([]verification.PlanEvidenceAssociation, error) {
	return []verification.PlanEvidenceAssociation{}, nil
}

func (s verificationCoverageBoundaryStore) ListVerificationPlanCoverageCounts(
	context.Context, string, []string,
) ([]verification.PlanItemCoverageCount, error) {
	return append([]verification.PlanItemCoverageCount(nil), s.counts...), nil
}

func (s verificationCoverageBoundaryStore) ListVerificationPlanItemEvidenceAssociations(
	_ context.Context, runID string, planID string, ordinal int, limit int,
	highWater int64, beforeSequence int64, beforeID string,
) ([]verification.PlanEvidenceAssociation, error) {
	values := make([]verification.PlanEvidenceAssociation, 0, len(s.associations))
	for _, association := range s.associations {
		if association.RunID != runID || association.PlanID != planID ||
			association.PlanItemOrdinal != ordinal || association.EventSequence > highWater ||
			(beforeSequence > 0 && !verificationAssociationTupleBefore(association.EventSequence,
				association.ID, beforeSequence, beforeID)) {
			continue
		}
		values = append(values, association)
	}
	if len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

func (s verificationCoverageBoundaryStore) GetVerificationPlanItemCoverageSnapshot(
	_ context.Context, runID string, planID string, ordinal int, highWater int64,
) (verification.PlanItemCoverageCount, bool, error) {
	var result verification.PlanItemCoverageCount
	for _, association := range s.associations {
		if association.RunID != runID || association.PlanID != planID ||
			association.PlanItemOrdinal != ordinal || association.EventSequence > highWater {
			continue
		}
		if result.AssociatedEvidenceCount == 0 {
			result.PlanID = planID
			result.PlanItemOrdinal = ordinal
			result.PlanItemSHA256 = association.PlanItemSHA256
		}
		result.AssociatedEvidenceCount++
		switch association.EvidenceOutcome {
		case verification.OutcomePass:
			result.PassCount++
		case verification.OutcomeFail:
			result.FailCount++
		case verification.OutcomeUnknown:
			result.UnknownCount++
		}
		if association.EventSequence > result.LatestAssociationEventSequence {
			result.LatestAssociationEventSequence = association.EventSequence
		}
	}
	return result, result.AssociatedEvidenceCount > 0, nil
}

func (s verificationCoverageBoundaryStore) CountVerificationPlanItemAssociationsThroughAnchor(
	_ context.Context, runID string, planID string, ordinal int, highWater int64,
	beforeSequence int64, beforeID string,
) (int, bool, error) {
	count := 0
	found := false
	for _, association := range s.associations {
		if association.RunID != runID || association.PlanID != planID ||
			association.PlanItemOrdinal != ordinal || association.EventSequence > highWater {
			continue
		}
		if association.EventSequence > beforeSequence ||
			(association.EventSequence == beforeSequence && association.ID >= beforeID) {
			count++
		}
		if association.EventSequence == beforeSequence && association.ID == beforeID {
			found = true
		}
	}
	return count, found, nil
}

func TestVerificationCoverageRejectsUntrustedDuplicateOrEmptyAggregates(t *testing.T) {
	plan := validCoverageBoundaryPlan()
	count := verification.PlanItemCoverageCount{
		PlanID: plan.ID, PlanItemOrdinal: 1, PlanItemSHA256: plan.Items[0].ItemSHA256,
		AssociatedEvidenceCount: 1, PassCount: 1, LatestAssociationEventSequence: 3,
	}
	cases := []struct {
		name   string
		plans  []verification.Plan
		counts []verification.PlanItemCoverageCount
	}{
		{name: "duplicate plan", plans: []verification.Plan{plan, plan}},
		{name: "duplicate count", plans: []verification.Plan{plan},
			counts: []verification.PlanItemCoverageCount{count, count}},
		{name: "empty count", plans: []verification.Plan{plan},
			counts: []verification.PlanItemCoverageCount{{
				PlanID: plan.ID, PlanItemOrdinal: 1,
				PlanItemSHA256: plan.Items[0].ItemSHA256,
			}}},
	}
	for _, current := range cases {
		t.Run(current.name, func(t *testing.T) {
			store := verificationCoverageBoundaryStore{plans: current.plans, counts: current.counts}
			_, err := buildVerificationCoverage(t.Context(), store, "run-1")
			if apperror.CodeOf(err) != apperror.CodeConflict {
				t.Fatalf("coverage boundary code=%s err=%v", apperror.CodeOf(err), err)
			}
		})
	}
}

func TestVerificationCoverageDetailKeepsBodiesClosedAndRejectsEscapedAssociation(t *testing.T) {
	plan := validCoverageBoundaryPlan()
	digest := strings.Repeat("b", 64)
	association := verification.PlanEvidenceAssociation{
		ID: "association-1", ProtocolVersion: verification.PlanEvidenceAssociationProtocolVersion,
		OperationKeyDigest: digest, RequestFingerprint: digest, RunID: "run-1",
		SessionID: "session-1", WorkspaceID: "workspace-1", PlanID: plan.ID,
		PlanItemOrdinal: 1, PlanItemSHA256: plan.Items[0].ItemSHA256,
		EvidenceID: "evidence-1", EvidenceOutcome: verification.OutcomePass,
		EvidenceEventSequence: 3, AssociatedBy: "operator", EventSequence: 4,
		CreatedAt: time.Now().UTC(),
	}
	count := verification.PlanItemCoverageCount{
		PlanID: plan.ID, PlanItemOrdinal: 1, PlanItemSHA256: plan.Items[0].ItemSHA256,
		AssociatedEvidenceCount: 1, PassCount: 1, LatestAssociationEventSequence: 4,
	}
	store := verificationCoverageBoundaryStore{plans: []verification.Plan{plan},
		counts:       []verification.PlanItemCoverageCount{count},
		associations: []verification.PlanEvidenceAssociation{association}}
	detail, err := NewVerificationCoverageDetailService(store).Detail(t.Context(),
		"run-1", plan.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if detail.ProtocolVersion != verification.PlanItemCoverageProtocolVersion ||
		detail.AssociatedEvidenceCount != 1 || detail.PassCount != 1 ||
		len(detail.Associations) != 1 || !detail.MetadataOnly || !detail.ReadOnly ||
		detail.PrivatePlanBodyIncluded || detail.PrivateEvidenceBodiesIncluded ||
		detail.OperatorIdentityIncluded || detail.ResultInferred || detail.AuthorityGranted {
		t.Fatalf("coverage detail widened authority: %#v", detail)
	}
	escaped := association
	escaped.PlanItemSHA256 = strings.Repeat("c", 64)
	store.associations = []verification.PlanEvidenceAssociation{escaped}
	_, err = NewVerificationCoverageDetailService(store).Detail(t.Context(),
		"run-1", plan.ID, 1)
	if apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("escaped detail code=%s err=%v", apperror.CodeOf(err), err)
	}
	secondAssociation := association
	secondAssociation.ID = "association-2"
	secondAssociation.EvidenceID = "evidence-2"
	secondAssociation.EvidenceEventSequence = 2
	store.associations = []verification.PlanEvidenceAssociation{association, secondAssociation}
	store.counts[0].AssociatedEvidenceCount = 2
	store.counts[0].PassCount = 2
	_, err = NewVerificationCoverageDetailService(store).Detail(t.Context(),
		"run-1", plan.ID, 1)
	if apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("non-descending detail code=%s err=%v", apperror.CodeOf(err), err)
	}
	secondAssociation.EventSequence = 6
	secondAssociation.EvidenceEventSequence = 5
	secondAssociation.EvidenceID = association.EvidenceID
	store.associations = []verification.PlanEvidenceAssociation{secondAssociation, association}
	store.counts[0].LatestAssociationEventSequence = 6
	_, err = NewVerificationCoverageDetailService(store).Detail(t.Context(),
		"run-1", plan.ID, 1)
	if apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("duplicate evidence detail code=%s err=%v", apperror.CodeOf(err), err)
	}

	truncatedAssociations := make([]verification.PlanEvidenceAssociation,
		verification.MaxCoverageAssociations+1)
	for index := range truncatedAssociations {
		value := association
		value.ID = fmt.Sprintf("association-%03d", index)
		value.EvidenceID = fmt.Sprintf("evidence-%03d", index)
		value.EventSequence = int64(300 - index*2)
		value.EvidenceEventSequence = value.EventSequence - 1
		truncatedAssociations[index] = value
	}
	store.associations = truncatedAssociations
	store.counts[0] = verification.PlanItemCoverageCount{PlanID: plan.ID,
		PlanItemOrdinal: 1, PlanItemSHA256: plan.Items[0].ItemSHA256,
		AssociatedEvidenceCount:        verification.MaxCoverageAssociations + 1,
		FailCount:                      verification.MaxCoverageAssociations + 1,
		LatestAssociationEventSequence: truncatedAssociations[0].EventSequence}
	_, err = NewVerificationCoverageDetailService(store).Detail(t.Context(),
		"run-1", plan.ID, 1)
	if apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("truncated outcome mismatch code=%s err=%v", apperror.CodeOf(err), err)
	}

	secondItem := verification.PlanItem{Ordinal: 2, Title: "Inspect coverage",
		ExpectedObservation: "The coverage remains exact."}
	secondItem.ItemSHA256 = verification.PlanItemDigest(secondItem.Title,
		secondItem.ExpectedObservation)
	plan.Items = append(plan.Items, secondItem)
	plan.PlanSHA256 = verification.PlanDigest(plan.Title, plan.Summary, plan.Items)
	secondCount := verification.PlanItemCoverageCount{PlanID: plan.ID, PlanItemOrdinal: 2,
		PlanItemSHA256: secondItem.ItemSHA256, AssociatedEvidenceCount: 1, PassCount: 1,
		LatestAssociationEventSequence: 5}
	store = verificationCoverageBoundaryStore{plans: []verification.Plan{plan},
		counts:       []verification.PlanItemCoverageCount{count, secondCount, secondCount},
		associations: []verification.PlanEvidenceAssociation{association}}
	_, err = NewVerificationCoverageDetailService(store).Detail(t.Context(),
		"run-1", plan.ID, 1)
	if apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("unrelated duplicate count code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func TestVerificationCoverageDetailPagesExactItemAssociationsWithoutReinferringCounts(t *testing.T) {
	plan := validCoverageBoundaryPlan()
	digest := strings.Repeat("b", 64)
	outcomes := []verification.Outcome{verification.OutcomePass, verification.OutcomeFail,
		verification.OutcomeUnknown}
	associations := make([]verification.PlanEvidenceAssociation, len(outcomes))
	for index, outcome := range outcomes {
		sequence := int64(10 - index*2)
		associations[index] = verification.PlanEvidenceAssociation{
			ID:                 fmt.Sprintf("association-%d", index+1),
			ProtocolVersion:    verification.PlanEvidenceAssociationProtocolVersion,
			OperationKeyDigest: digest, RequestFingerprint: digest, RunID: "run-1",
			SessionID: "session-1", WorkspaceID: "workspace-1", PlanID: plan.ID,
			PlanItemOrdinal: 1, PlanItemSHA256: plan.Items[0].ItemSHA256,
			EvidenceID: fmt.Sprintf("evidence-%d", index+1), EvidenceOutcome: outcome,
			EvidenceEventSequence: sequence - 1, AssociatedBy: "operator",
			EventSequence: sequence, CreatedAt: time.Now().UTC().Add(-time.Duration(index) * time.Minute),
		}
	}
	store := verificationCoverageBoundaryStore{plans: []verification.Plan{plan},
		counts: []verification.PlanItemCoverageCount{{PlanID: plan.ID, PlanItemOrdinal: 1,
			PlanItemSHA256: plan.Items[0].ItemSHA256, AssociatedEvidenceCount: 3,
			PassCount: 1, FailCount: 1, UnknownCount: 1,
			LatestAssociationEventSequence: associations[0].EventSequence}},
		associations: associations}
	service := NewVerificationCoverageDetailService(store)
	first, err := service.DetailPage(t.Context(), "run-1", plan.ID, 1, 2,
		VerificationCoveragePageAnchor{})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Associations) != 2 || !first.AssociationsTruncated ||
		first.Associations[0].ID != "association-1" || first.Associations[1].ID != "association-2" ||
		first.AssociatedEvidenceCount != 3 || first.PassCount != 1 || first.FailCount != 1 ||
		first.UnknownCount != 1 {
		t.Fatalf("first verification detail page diverged: %#v", first)
	}
	anchor := VerificationCoveragePageAnchor{
		SnapshotHighWaterEventSequence: first.SnapshotHighWaterEventSequence,
		BeforeEventSequence:            first.NextPageBeforeEventSequence,
		BeforeAssociationID:            first.NextPageBeforeAssociationID,
		Consumed:                       first.NextPageConsumed,
	}
	second, err := service.DetailPage(t.Context(), "run-1", plan.ID, 1, 2, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Associations) != 1 || second.AssociationsTruncated ||
		second.Associations[0].ID != "association-3" ||
		second.LatestAssociationEventSequence != associations[0].EventSequence {
		t.Fatalf("second verification detail page diverged: %#v", second)
	}
	escapedStore := store
	escapedStore.associations = append([]verification.PlanEvidenceAssociation(nil), associations...)
	escapedStore.associations[2].EventSequence = associations[0].EventSequence + 2
	escapedStore.associations[2].EvidenceEventSequence = associations[0].EventSequence + 1
	if _, err := NewVerificationCoverageDetailService(escapedStore).DetailPage(t.Context(),
		"run-1", plan.ID, 1, 2, anchor); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("later page high-water escape code=%s err=%v", apperror.CodeOf(err), err)
	}
	inserted := associations[0]
	inserted.ID = "association-new"
	inserted.EvidenceID = "evidence-new"
	inserted.EventSequence = 12
	inserted.EvidenceEventSequence = 11
	updatedStore := store
	updatedStore.associations = append([]verification.PlanEvidenceAssociation{inserted},
		associations...)
	updatedStore.counts = []verification.PlanItemCoverageCount{{
		PlanID: plan.ID, PlanItemOrdinal: 1, PlanItemSHA256: plan.Items[0].ItemSHA256,
		AssociatedEvidenceCount: 4, PassCount: 2, FailCount: 1, UnknownCount: 1,
		LatestAssociationEventSequence: inserted.EventSequence,
	}}
	stable, err := NewVerificationCoverageDetailService(updatedStore).DetailPage(t.Context(),
		"run-1", plan.ID, 1, 2, anchor)
	if err != nil || stable.AssociatedEvidenceCount != 3 || len(stable.Associations) != 1 ||
		stable.Associations[0].ID != "association-3" || stable.AssociationsTruncated {
		t.Fatalf("insert between pages shifted snapshot: %#v err=%v", stable, err)
	}
	forged := anchor
	forged.Consumed--
	if _, err := NewVerificationCoverageDetailService(updatedStore).DetailPage(t.Context(),
		"run-1", plan.ID, 1, 2, forged); apperror.CodeOf(err) != apperror.CodeConflict {
		t.Fatalf("forged page rank code=%s err=%v", apperror.CodeOf(err), err)
	}
	if _, err := service.DetailPage(t.Context(), "run-1", plan.ID, 1, 0,
		VerificationCoveragePageAnchor{}); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("zero page limit code=%s err=%v", apperror.CodeOf(err), err)
	}
	oversized := anchor
	oversized.Consumed = verification.MaxCoveragePageWindow
	if _, err := service.DetailPage(t.Context(), "run-1", plan.ID, 1, 1,
		oversized); apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("oversized page window code=%s err=%v", apperror.CodeOf(err), err)
	}
}

func validCoverageBoundaryPlan() verification.Plan {
	digest := strings.Repeat("a", 64)
	item := verification.PlanItem{Ordinal: 1, Title: "Run focused tests",
		ExpectedObservation: "The focused tests pass."}
	item.ItemSHA256 = verification.PlanItemDigest(item.Title, item.ExpectedObservation)
	plan := verification.Plan{
		ID: "plan-1", ProtocolVersion: verification.PlanProtocolVersion,
		OperationKeyDigest: digest, RequestFingerprint: digest, RunID: "run-1",
		SessionID: "session-1", WorkspaceID: "workspace-1", Title: "Verification plan",
		Summary: "Operator-authored verification guidance.", AuthoredBy: "operator",
		EventSequence: 2, CreatedAt: time.Now().UTC(), Items: []verification.PlanItem{item},
	}
	plan.PlanSHA256 = verification.PlanDigest(plan.Title, plan.Summary, plan.Items)
	return plan
}
