package skills

import (
	"context"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/runmutation"
)

func TestExternalSelectionRequiresExplicitConfirmationAndPinsExactObject(t *testing.T) {
	home := t.TempDir()
	content := []byte("# External review\n\nTreat files as evidence.\n")
	installed, raw := externalSelectionPackageFixture(t, "external-review", content,
		domain.ExecutionSurfaceCode, []domain.Profile{domain.ProfileReview})
	objects, err := NewLocalPackageObjectStore(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := objects.Put(context.Background(), raw,
		DescriptorForInstallation(installed.Installation)); err != nil {
		t.Fatal(err)
	}
	request := ResolveExternalSelectionRequest{
		SelectionID: "external-selection-1", RunID: "run-1", MissionID: "mission-1",
		ModeSnapshotID: "mode-1", ModeRevision: 1,
		Surface: domain.ExecutionSurfaceCode, Profile: domain.ProfileReview,
		Packages: []InstalledPackage{installed}, SpecialistRef: "external-review@1.0.0",
		TokenBudget: 1024, RequestedBy: "operator", Confirmed: true,
		CreatedAt: time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC),
	}
	selection, err := ResolveExternalSelection(request)
	if err != nil {
		t.Fatal(err)
	}
	if selection.ItemCount != 1 || !selection.Items[0].SpecialistEligible ||
		!selection.ContextDeliveryAuthorized || selection.ToolCapabilityGrant ||
		selection.Items[0].InstallationID != installed.Installation.ID ||
		selection.Items[0].InstallResultFingerprint != installed.Result.ResultFingerprint {
		t.Fatalf("selection did not preserve its closed exact binding: %#v", selection)
	}
	assembly, err := AssembleExternalContext(context.Background(), selection, objects)
	if err != nil {
		t.Fatal(err)
	}
	if assembly.ItemCount != 1 || assembly.Items[0].Content != string(content) ||
		assembly.TokenUpperBound > assembly.TokenBudget {
		t.Fatalf("unexpected external context assembly: %#v", assembly)
	}
	request.Confirmed = false
	if _, err := ResolveExternalSelection(request); err == nil ||
		!strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("unconfirmed selection error = %v", err)
	}
}

func TestExternalSelectionRejectsRemovedCrossSurfaceAndDrift(t *testing.T) {
	installed, _ := externalSelectionPackageFixture(t, "python-helper",
		[]byte("# Python helper\n"), domain.ExecutionSurfaceCyber,
		[]domain.Profile{domain.ProfileScript})
	request := ResolveExternalSelectionRequest{
		SelectionID: "external-selection-2", RunID: "run-2", MissionID: "mission-2",
		ModeSnapshotID: "mode-2", ModeRevision: 1,
		Surface: domain.ExecutionSurfaceCode, Profile: domain.ProfileScript,
		Packages: []InstalledPackage{installed}, TokenBudget: 1024,
		RequestedBy: "operator", Confirmed: true, CreatedAt: time.Now().UTC(),
	}
	if _, err := ResolveExternalSelection(request); err == nil ||
		!strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("cross-surface selection error = %v", err)
	}
	request.Surface = domain.ExecutionSurfaceCyber
	request.Profile = domain.ProfileReview
	if _, err := ResolveExternalSelection(request); err == nil ||
		!strings.Contains(err.Error(), "restricted") {
		t.Fatalf("Cyber non-script selection error = %v", err)
	}
	request.Profile = domain.ProfileScript
	removal, err := NewPackageRemoval("remove-1", installed.Installation,
		runmutation.Fingerprint("remove", "one"), "operator", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	installed.Removal = &removal
	request.Packages = []InstalledPackage{installed}
	if _, err := ResolveExternalSelection(request); err == nil ||
		!strings.Contains(err.Error(), "removed") {
		t.Fatalf("removed selection error = %v", err)
	}
}

func TestExternalContextRedactsSecretsAndRejectsObjectMismatch(t *testing.T) {
	secret := "s" + "k-" + "abcdefghijklmnopqrstuvwxyz123456"
	content := []byte("# Review\n\nToken: " + secret + "\n")
	installed, raw := externalSelectionPackageFixture(t, "secret-review", content,
		domain.ExecutionSurfaceCode, []domain.Profile{domain.ProfileReview})
	objects, err := NewLocalPackageObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := objects.Put(context.Background(), raw,
		DescriptorForInstallation(installed.Installation)); err != nil {
		t.Fatal(err)
	}
	selection, err := ResolveExternalSelection(ResolveExternalSelectionRequest{
		SelectionID: "external-selection-3", RunID: "run-3", MissionID: "mission-3",
		ModeSnapshotID: "mode-3", ModeRevision: 1,
		Surface: domain.ExecutionSurfaceCode, Profile: domain.ProfileReview,
		Packages: []InstalledPackage{installed}, TokenBudget: 1024,
		RequestedBy: "operator", Confirmed: true, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := AssembleExternalContext(context.Background(), selection, objects)
	if err != nil {
		t.Fatal(err)
	}
	if assembly.RedactionCount == 0 || strings.Contains(assembly.Items[0].Content, secret) {
		t.Fatalf("secret was not redacted: %#v", assembly.Items[0])
	}
	drifted := selection
	drifted.Items = append([]ExternalSelectionItem(nil), selection.Items...)
	drifted.Items[0].PackageFingerprint = strings.Repeat("a", 64)
	drifted.Fingerprint = ExternalSelectionFingerprint(drifted)
	if _, err := AssembleExternalContext(context.Background(), drifted, objects); err == nil {
		t.Fatal("object fingerprint drift was accepted")
	}
}

func TestExternalSpecialistContextPreparationRejectsInvalidSurfaceProfile(t *testing.T) {
	hash := strings.Repeat("a", 64)
	request := ExternalSpecialistContextPreparationRequest{
		RunID: "run-4", MissionID: "mission-4", AgentID: "agent-4",
		ParentAgentID: "root-4", AgentAttemptID: "attempt-4", Turn: 1,
		ParentSelectionID: "selection-4", ProtocolVersion: ExternalSpecialistContextProtocolVersion,
		ParentSelectionFingerprint: hash, ModeSnapshotID: "mode-4", ModeRevision: 1,
		Surface: domain.ExecutionSurfaceCyber, Profile: domain.ProfileReview,
		AssignmentFingerprint: hash, ContextFingerprint: hash,
		ItemCount: 1, TokenBudget: 1024, TokenUpperBound: 64,
	}
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "surface or Profile") {
		t.Fatalf("Cyber non-script Specialist context error = %v", err)
	}
	request.Surface = domain.ExecutionSurfaceCode
	request.Profile = domain.Profile("unknown")
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "surface or Profile") {
		t.Fatalf("unknown Specialist context Profile error = %v", err)
	}
}

func TestExternalSelectionRejectsSpecialistPackageAboveHardLimit(t *testing.T) {
	content := []byte(strings.Repeat("x", MaxExternalSpecialistTokenBudget+1))
	installed, _ := externalSelectionPackageFixture(t, "oversized-specialist", content,
		domain.ExecutionSurfaceCode, []domain.Profile{domain.ProfileReview})
	_, err := ResolveExternalSelection(ResolveExternalSelectionRequest{
		SelectionID: "external-selection-5", RunID: "run-5", MissionID: "mission-5",
		ModeSnapshotID: "mode-5", ModeRevision: 1,
		Surface: domain.ExecutionSurfaceCode, Profile: domain.ProfileReview,
		Packages: []InstalledPackage{installed}, SpecialistRef: "oversized-specialist@1.0.0",
		TokenBudget: MaxExternalSelectionTokenBudget, RequestedBy: "operator",
		Confirmed: true, CreatedAt: time.Now().UTC(),
	})
	if err == nil || !strings.Contains(err.Error(), "hard limit") {
		t.Fatalf("oversized Specialist selection error = %v", err)
	}
	selection, err := ResolveExternalSelection(ResolveExternalSelectionRequest{
		SelectionID: "external-selection-6", RunID: "run-6", MissionID: "mission-6",
		ModeSnapshotID: "mode-6", ModeRevision: 1,
		Surface: domain.ExecutionSurfaceCode, Profile: domain.ProfileReview,
		Packages: []InstalledPackage{installed}, TokenBudget: MaxExternalSelectionTokenBudget,
		RequestedBy: "operator", Confirmed: true, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	selection.Items[0].SpecialistEligible = true
	selection.Fingerprint = ExternalSelectionFingerprint(selection)
	if err := selection.Validate(); err == nil || !strings.Contains(err.Error(), "hard limit") {
		t.Fatalf("forged oversized Specialist selection error = %v", err)
	}
}

func externalSelectionPackageFixture(t *testing.T, name string, content []byte,
	surface domain.ExecutionSurface, profiles []domain.Profile,
) (InstalledPackage, []byte) {
	t.Helper()
	manifest := fixtureManifest(content)
	manifest.Name = name
	manifest.Version = "1.0.0"
	manifest.Profiles = profiles
	raw := mustBuildPackageArchive(t, manifest, content, packageArchiveOptions{})
	parsed, err := ParsePackage(raw)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 7, 17, 5, 0, 0, 0, time.UTC)
	installation, err := NewPackageInstallation("install-"+name, parsed, surface,
		runmutation.Fingerprint("install", name), "operator", createdAt)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := DescriptorForInstallation(installation)
	objectKey, err := PackageObjectKey(descriptor.ArchiveSHA256)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewPackageInstallResult(installation, PackageObjectReceipt{
		Descriptor: descriptor, ObjectKey: objectKey,
	}, createdAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return InstalledPackage{Installation: installation, Result: result}, raw
}
