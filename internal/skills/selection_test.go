package skills

import (
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
)

func TestResolveSelectionIsDeterministicBoundedAndDefensive(t *testing.T) {
	content := []byte("# Selection fixture\n")
	first := fixtureManifest(content)
	second := fixtureManifest(content)
	second.Name = "code-review"
	second.Description = "Second coding metadata."
	registry := &Registry{entries: map[string]registryEntry{
		first.Name:  {manifest: first, content: append([]byte(nil), content...)},
		second.Name: {manifest: second, content: append([]byte(nil), content...)},
	}}
	request := ResolveSelectionRequest{
		SelectionID: "skill-selection-first", RunID: "run-selection",
		MissionID: "mission-selection", Profile: domain.ProfileCode,
		Names: []string{"code-review", "code"}, TokenBudget: 1024,
		RequestedBy: "operator", CreatedAt: time.Now().UTC(),
	}
	selection, err := registry.ResolveSelection(request)
	if err != nil {
		t.Fatal(err)
	}
	if selection.ItemCount != 2 || selection.Items[0].Name != "code" ||
		selection.Items[1].Name != "code-review" || selection.TokenUpperBound != len(content)*2 ||
		selection.Fingerprint == "" {
		t.Fatalf("resolved selection drifted: %#v", selection)
	}
	request.SelectionID = "skill-selection-second"
	request.Names = []string{"code", "code-review"}
	request.CreatedAt = request.CreatedAt.Add(time.Second)
	reordered, err := registry.ResolveSelection(request)
	if err != nil || reordered.Fingerprint != selection.Fingerprint {
		t.Fatalf("selection fingerprint is not deterministic: %#v err=%v", reordered, err)
	}
	clone := CloneSelection(selection)
	clone.Items[0].Name = "changed"
	if selection.Items[0].Name != "code" {
		t.Fatal("selection clone exposed the original item slice")
	}
}

func TestResolveSelectionRejectsProfileBudgetAndIdentityDrift(t *testing.T) {
	registry, err := BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	base := ResolveSelectionRequest{
		SelectionID: "skill-selection-test", RunID: "run-test", MissionID: "mission-test",
		Profile: domain.ProfileCode, Names: []string{"code"}, TokenBudget: 4096,
		RequestedBy: "operator", CreatedAt: time.Now().UTC(),
	}
	tests := []struct {
		name   string
		mutate func(*ResolveSelectionRequest)
		want   string
	}{
		{name: "profile mismatch", mutate: func(r *ResolveSelectionRequest) { r.Profile = domain.ProfileReview }, want: "incompatible"},
		{name: "duplicate", mutate: func(r *ResolveSelectionRequest) { r.Names = []string{"code", "code"} }, want: "duplicated"},
		{name: "budget", mutate: func(r *ResolveSelectionRequest) { r.TokenBudget = 1 }, want: "require token upper bound"},
		{name: "too many", mutate: func(r *ResolveSelectionRequest) { r.Names = make([]string, MaxSelectionItems+1) }, want: "between 1 and"},
		{name: "missing identity", mutate: func(r *ResolveSelectionRequest) { r.RunID = "" }, want: "identities"},
		{name: "control identity", mutate: func(r *ResolveSelectionRequest) { r.RequestedBy = "operator\nadmin" }, want: "identities"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			request.Names = append([]string(nil), base.Names...)
			test.mutate(&request)
			_, err := registry.ResolveSelection(request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestSelectionValidationPinsFingerprintAndAccounting(t *testing.T) {
	registry, err := BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.ResolveSelection(ResolveSelectionRequest{
		SelectionID: "skill-selection-validation", RunID: "run-validation",
		MissionID: "mission-validation", Profile: domain.ProfileCode,
		Names: []string{"code"}, TokenBudget: 4096, RequestedBy: "operator",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	mutated := CloneSelection(selection)
	mutated.Items[0].Version = "2.0.0"
	if err := mutated.Validate(); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("version drift error = %v", err)
	}
	mutated.Fingerprint = SelectionFingerprint(mutated)
	if SelectionRequestFingerprint(mutated) != SelectionRequestFingerprint(selection) {
		t.Fatal("Registry version drift changed the operator intent fingerprint")
	}
	mutated.Items[0].Name = "learn"
	if SelectionRequestFingerprint(mutated) == SelectionRequestFingerprint(selection) {
		t.Fatal("Skill name drift did not change the operator intent fingerprint")
	}
	mutated = CloneSelection(selection)
	mutated.TokenUpperBound++
	if err := mutated.Validate(); err == nil || !strings.Contains(err.Error(), "accounting") {
		t.Fatalf("accounting drift error = %v", err)
	}
	mutated = CloneSelection(selection)
	mutated.Items[0].TokenUpperBound--
	mutated.TokenUpperBound--
	mutated.Fingerprint = SelectionFingerprint(mutated)
	if err := mutated.Validate(); err == nil || !strings.Contains(err.Error(), "item 1") {
		t.Fatalf("underreported item budget error = %v", err)
	}
	operation := SelectionOperation{
		KeyDigest: strings.Repeat("a", 64), RequestFingerprint: SelectionRequestFingerprint(selection),
		SelectionID: selection.ID, RunID: selection.RunID, RequestedBy: selection.RequestedBy,
		CreatedAt: selection.CreatedAt,
	}
	if err := operation.Validate(); err != nil {
		t.Fatalf("valid selection operation failed: %v", err)
	}
}
