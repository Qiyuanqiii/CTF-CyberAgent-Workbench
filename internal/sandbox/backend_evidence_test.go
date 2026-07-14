package sandbox

import (
	"context"
	"strings"
	"testing"
)

func TestSimulationBackendClientProducesUnverifiedBoundEvidence(t *testing.T) {
	manifest := validManifest()
	manifest.Backend = BackendDocker
	normalized, err := NormalizeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestFingerprint, err := normalized.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	outputPlan, err := NewOutputExportPlan(normalized)
	if err != nil {
		t.Fatal(err)
	}
	checks := RequiredBackendChecks()
	report, err := NewSimulationBackendClient().Probe(context.Background(), BackendEvidenceProbeRequest{
		PreflightID: "preflight_test", Backend: BackendDocker, Manifest: normalized,
		ManifestFingerprint:    manifestFingerprint,
		ThreatModelFingerprint: BackendThreatModelFingerprint(checks),
		OutputPlanFingerprint:  outputPlan.Fingerprint,
		ImageDigest:            "sha256:" + strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Source != BackendEvidenceSourceFake || report.TrustClass != BackendEvidenceTrustSimulation ||
		report.ProductionVerified || report.BackendAvailable || report.BackendEnabled ||
		report.ExecutionAuthorized || report.ArtifactCommitAuthorized || len(report.Items) != MaxBackendChecks {
		t.Fatalf("simulation widened backend authority: %#v", report)
	}
	for _, item := range report.Items {
		if !item.Satisfied || item.Verified || item.EvidenceState != BackendEvidenceStatePass {
			t.Fatalf("simulation evidence made a production claim: %#v", item)
		}
	}

	forged := report
	forged.Items = append([]BackendEvidenceItem(nil), report.Items...)
	forged.Items[0].Verified = true
	if err := forged.Validate(); err == nil {
		t.Fatal("verified fake evidence was accepted")
	}
}

func TestSimulationBackendClientRejectsNonDockerAndMalformedDigest(t *testing.T) {
	manifest := validManifest()
	fingerprint, err := manifest.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewOutputExportPlan(manifest)
	if err != nil {
		t.Fatal(err)
	}
	request := BackendEvidenceProbeRequest{
		PreflightID: "preflight_test", Backend: BackendNoop, Manifest: manifest,
		ManifestFingerprint:    fingerprint,
		ThreatModelFingerprint: BackendThreatModelFingerprint(RequiredBackendChecks()),
		OutputPlanFingerprint:  plan.Fingerprint,
		ImageDigest:            "sha256:" + strings.Repeat("b", 64),
	}
	if _, err := NewSimulationBackendClient().Probe(context.Background(), request); err == nil {
		t.Fatal("non-Docker fake evidence was accepted")
	}
	request.Backend = BackendDocker
	request.Manifest.Backend = BackendDocker
	request.ImageDigest = "sha256:latest"
	if _, err := NewSimulationBackendClient().Probe(context.Background(), request); err == nil {
		t.Fatal("malformed OCI image digest was accepted")
	}
}
