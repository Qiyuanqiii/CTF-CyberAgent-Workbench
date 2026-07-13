package skills

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"cyberagent-workbench/internal/domain"
)

func TestRegistryAssemblesDeterministicSelectionBoundContext(t *testing.T) {
	registry, err := BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.ResolveSelection(ResolveSelectionRequest{
		SelectionID: "selection-context-1", RunID: "run-context-1",
		MissionID: "mission-context-1", Profile: domain.ProfileCode,
		Names: []string{"code"}, TokenBudget: DefaultSelectionTokenBudget,
		RequestedBy: "operator", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := registry.AssembleContext(selection)
	if err != nil {
		t.Fatal(err)
	}
	second, err := registry.AssembleContext(selection)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint || first.ItemCount != 1 ||
		first.Items[0].Name != "code" || first.TokenUpperBound != len(first.Items[0].Content) ||
		!strings.Contains(first.Items[0].Content, "Code workflow") {
		t.Fatalf("assembled Skill context drifted: %#v", first)
	}
	if request := first.Preparation("agent-root", "attempt-root", 2); request.ContextFingerprint != first.Fingerprint ||
		request.SelectionFingerprint != selection.Fingerprint || request.TokenBudget != selection.TokenBudget {
		t.Fatalf("preparation provenance drifted: %#v", request)
	}

	tampered := first
	tampered.Items = append([]ContextItem(nil), first.Items...)
	tampered.Items[0].Content += " changed"
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "content is invalid") {
		t.Fatalf("tampered delivery validation error = %v", err)
	}

	drifted := CloneSelection(selection)
	drifted.Items[0].Version = "1.0.1"
	drifted.Fingerprint = SelectionFingerprint(drifted)
	if err := drifted.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.AssembleContext(drifted); err == nil ||
		!strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("pinned Registry drift error = %v", err)
	}
}

func TestBuiltinRegistryRetainsArchivedPinnedContext(t *testing.T) {
	registry, err := BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.ResolveSelection(ResolveSelectionRequest{
		SelectionID: "selection-archived", RunID: "run-archived",
		MissionID: "mission-archived", Profile: domain.ProfileCode,
		Names: []string{"code"}, TokenBudget: DefaultSelectionTokenBudget,
		RequestedBy: "operator", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	archived := registry.versions["code"]["1.0.0"].manifest
	selection.Items[0].Version = archived.Version
	selection.Items[0].ContentSHA256 = archived.ContentSHA256
	selection.Items[0].ContentBytes = archived.ContentBytes
	selection.Items[0].TokenUpperBound = archived.ContentTokenUpperBound
	selection.TokenUpperBound = archived.ContentTokenUpperBound
	selection.Fingerprint = SelectionFingerprint(selection)
	if err := selection.Validate(); err != nil {
		t.Fatal(err)
	}
	assembly, err := registry.AssembleContext(selection)
	if err != nil {
		t.Fatal(err)
	}
	if assembly.Items[0].Version != "1.0.0" ||
		!strings.Contains(assembly.Items[0].Content, "Metadata placeholder") ||
		registry.List(domain.ProfileCode)[0].Version != "1.1.0" {
		t.Fatalf("archived Skill delivery drifted: %#v", assembly)
	}
}

func TestRegistryRedactsSkillContextBeforeBudgeting(t *testing.T) {
	secret := "s" + "k-" + strings.Repeat("z", 28)
	content := []byte("# Safe workflow\n\nAPI_KEY=" + secret + "\n")
	manifest := fixtureManifest(content)
	rawManifest, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := LoadFS(fstest.MapFS{
		"skills/code/manifest.json": &fstest.MapFile{Data: rawManifest},
		"skills/code/SKILL.md":      &fstest.MapFile{Data: content},
	}, "skills")
	if err != nil {
		t.Fatal(err)
	}
	selection, err := registry.ResolveSelection(ResolveSelectionRequest{
		SelectionID: "selection-redaction", RunID: "run-redaction",
		MissionID: "mission-redaction", Profile: domain.ProfileCode,
		Names: []string{"code"}, TokenBudget: DefaultSelectionTokenBudget,
		RequestedBy: "operator", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := registry.AssembleContext(selection)
	if err != nil {
		t.Fatal(err)
	}
	if assembly.RedactionCount < 1 || strings.Contains(assembly.Items[0].Content, secret) ||
		!strings.Contains(assembly.Items[0].Content, "[REDACTED:secret]") ||
		assembly.TokenUpperBound > selection.TokenUpperBound {
		t.Fatalf("redacted Skill context is invalid: %#v", assembly)
	}
}
