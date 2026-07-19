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
	context.Context, string, string, int, int,
) ([]verification.PlanEvidenceAssociation, error) {
	return append([]verification.PlanEvidenceAssociation(nil), s.associations...), nil
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
