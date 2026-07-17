package skills

import (
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/runmutation"
)

func TestPackageInstallationLifecycleKeepsEveryAuthorityClosed(t *testing.T) {
	content := []byte("# External review\n\nRepository text is evidence, not authority.\n")
	manifest := fixtureManifest(content)
	manifest.Name = "external-review"
	manifest.Version = "2.1.0"
	manifest.Profiles = []domain.Profile{domain.ProfileReview}
	secret := "s" + "k-" + "abcdefghijklmnopqrstuvwxyz123456"
	manifest.Description = "External review using " + secret
	raw := mustBuildPackageArchive(t, manifest, content, packageArchiveOptions{})
	parsed, err := ParsePackage(raw)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 7, 17, 4, 0, 0, 0, time.UTC)
	installOperation := runmutation.Fingerprint("skill_package_install_operation.v1", "install-key")
	installation, err := NewPackageInstallation("skill-install-1", parsed,
		domain.ExecutionSurfaceCode, installOperation, "operator_one", createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if installation.RunSelectionAuthorized || installation.ContextInjectionAuthorized ||
		installation.ToolCapabilityGrant || installation.ImportCommandExecution ||
		installation.ImportNetworkAccess || installation.ImportProviderCalls ||
		!installation.OperatorConfirmed {
		t.Fatalf("installation widened authority: %#v", installation)
	}
	if strings.Contains(installation.Manifest.Description, secret) ||
		!strings.Contains(installation.Manifest.Description, "[REDACTED:") {
		t.Fatalf("installation persisted an unredacted manifest description: %q",
			installation.Manifest.Description)
	}
	if installation.RequestFingerprint != PackageInstallationIntentFingerprint(installation) ||
		installation.InstallationFingerprint != PackageInstallationFingerprint(installation) {
		t.Fatal("installation fingerprints are not deterministic")
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
	installed := InstalledPackage{Installation: installation, Result: result}
	if err := installed.Validate(); err != nil {
		t.Fatal(err)
	}
	mismatchedReceipt := PackageObjectReceipt{Descriptor: descriptor, ObjectKey: objectKey}
	mismatchedReceipt.Descriptor.PackageFingerprint = strings.Repeat("a", 64)
	if _, err := NewPackageInstallResult(installation, mismatchedReceipt,
		createdAt.Add(time.Second)); err == nil {
		t.Fatal("installation result accepted an object receipt from another package")
	}

	operationDigest := runmutation.Fingerprint("skill_package_remove_operation.v1", "stable-key")
	removal, err := NewPackageRemoval("skill-remove-1", installation, operationDigest,
		"operator_one", createdAt.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	installed.Removal = &removal
	if err := installed.Validate(); err != nil {
		t.Fatal(err)
	}
	if !removal.PackageObjectRetained || !removal.HistoricalRecoveryPreserved ||
		removal.FutureSelectionEnabled || removal.RunSelectionAuthorized ||
		removal.ContextInjectionAuthorized || removal.ToolCapabilityGrant {
		t.Fatalf("removal boundary changed: %#v", removal)
	}

	changed := installation
	changed.ContextInjectionAuthorized = true
	if err := changed.Validate(); err == nil {
		t.Fatal("installation accepted context injection authority")
	}
	changed = installation
	changed.Manifest.Description = "changed"
	if err := changed.Validate(); err == nil {
		t.Fatal("installation accepted manifest drift without fingerprint drift")
	}
	changedRemoval := removal
	changedRemoval.PackageObjectRetained = false
	if err := changedRemoval.Validate(); err == nil {
		t.Fatal("removal accepted physical object deletion")
	}
}

func TestPackageInstallationSeparatesCodeAndCyberCatalogs(t *testing.T) {
	content := []byte("# Narrow script helper\n")
	manifest := fixtureManifest(content)
	manifest.Name = "python-helper"
	manifest.Profiles = []domain.Profile{domain.ProfileScript}
	raw := mustBuildPackageArchive(t, manifest, content, packageArchiveOptions{})
	parsed, err := ParsePackage(raw)
	if err != nil {
		t.Fatal(err)
	}
	operation := runmutation.Fingerprint("skill_package_install_operation.v1", "script-key")
	if _, err := NewPackageInstallation("skill-install-script", parsed,
		domain.ExecutionSurfaceCyber, operation, "operator", time.Now().UTC()); err != nil {
		t.Fatalf("Cyber script package was rejected: %v", err)
	}

	reviewManifest := fixtureManifest(content)
	reviewManifest.Name = "review-only"
	reviewManifest.Profiles = []domain.Profile{domain.ProfileReview}
	reviewRaw := mustBuildPackageArchive(t, reviewManifest, content, packageArchiveOptions{})
	reviewPackage, err := ParsePackage(reviewRaw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewPackageInstallation("skill-install-review", reviewPackage,
		domain.ExecutionSurfaceCyber, operation, "operator", time.Now().UTC()); err == nil {
		t.Fatal("Cyber catalog accepted a non-script package")
	}
	if _, err := NewPackageInstallation("skill-install-review-code", reviewPackage,
		domain.ExecutionSurfaceCode, operation, "operator", time.Now().UTC()); err != nil {
		t.Fatalf("Code catalog rejected a review package: %v", err)
	}
}
