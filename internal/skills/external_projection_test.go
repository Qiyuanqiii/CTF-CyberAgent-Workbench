package skills

import (
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

func TestExternalSkillProjectionValidatesBoundsAccountingAndClosedAuthority(t *testing.T) {
	base := validExternalSkillProjectionFixture()
	if err := base.Validate(); err != nil {
		t.Fatalf("valid external Skill projection failed: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ExternalSkillProjection)
	}{
		{name: "tool authority", mutate: func(value *ExternalSkillProjection) {
			value.ToolCapabilityGrant = true
		}},
		{name: "item count", mutate: func(value *ExternalSkillProjection) {
			value.ItemCount++
		}},
		{name: "token accounting", mutate: func(value *ExternalSkillProjection) {
			value.TokenUpperBound++
		}},
		{name: "two specialists", mutate: func(value *ExternalSkillProjection) {
			value.Items[1].SpecialistEligible = true
		}},
		{name: "unordered references", mutate: func(value *ExternalSkillProjection) {
			value.Items[0].Name, value.Items[1].Name = value.Items[1].Name, value.Items[0].Name
		}},
		{name: "noncontiguous ordinal", mutate: func(value *ExternalSkillProjection) {
			value.Items[1].Ordinal = 3
		}},
		{name: "excess declared tools", mutate: func(value *ExternalSkillProjection) {
			value.Items[0].DeclaredToolCount = MaxToolDependencies + 1
		}},
		{name: "committed without preparation", mutate: func(value *ExternalSkillProjection) {
			value.RootCommittedCount = value.RootPreparedCount + 1
		}},
		{name: "cyber profile widening", mutate: func(value *ExternalSkillProjection) {
			value.Surface = domain.ExecutionSurfaceCyber
		}},
		{name: "private identity ambiguity", mutate: func(value *ExternalSkillProjection) {
			value.RunID = "run-valid\x00forged"
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := CloneExternalSkillProjection(base)
			test.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatalf("invalid external Skill projection was accepted: %#v", value)
			}
		})
	}
}

func validExternalSkillProjectionFixture() ExternalSkillProjection {
	return ExternalSkillProjection{
		ProtocolVersion:           ExternalSkillProjectionProtocolVersion,
		RunID:                     "run-external-projection",
		ModeRevision:              1,
		Surface:                   domain.ExecutionSurfaceCode,
		Profile:                   domain.ProfileReview,
		TokenBudget:               512,
		TokenUpperBound:           300,
		ItemCount:                 2,
		OperatorConfirmed:         true,
		ContextDeliveryAuthorized: true,
		ToolCapabilityGrant:       false,
		RootPreparedCount:         2,
		RootCommittedCount:        1,
		SpecialistPreparedCount:   1,
		SpecialistCommittedCount:  1,
		Items: []ExternalSkillProjectionItem{
			{Ordinal: 1, Name: "alpha-review", Version: "1.0.0", TokenUpperBound: 100,
				TrustClass: PackageTrustOperatorInstalledUntrusted, DeclaredToolCount: 1,
				SpecialistEligible: true},
			{Ordinal: 2, Name: "beta-review", Version: "1.0.0", TokenUpperBound: 200,
				TrustClass: PackageTrustOperatorInstalledUntrusted, DeclaredToolCount: 2},
		},
		CreatedAt: time.Date(2026, time.July, 17, 10, 0, 0, 0, time.UTC),
	}
}
