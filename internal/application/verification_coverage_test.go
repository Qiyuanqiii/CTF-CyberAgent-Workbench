package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/verification"
)

type verificationCoverageBoundaryStore struct {
	plans  []verification.Plan
	counts []verification.PlanItemCoverageCount
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
