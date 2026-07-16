package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLocalDockerProductionEvidenceCollectorFailsClosedByPlatformAndOptIn(t *testing.T) {
	request := DockerProductionEvidenceCaptureRequest{
		ReviewID: "review", RunID: "run", AuthorityFingerprint: strings.Repeat("1", 64),
	}
	tests := []struct {
		name         string
		platform     string
		lookup       func(string) (string, bool)
		wantStatus   string
		wantPlatform string
		wantEndpoint string
	}{
		{
			name: "unsupported host", platform: "windows",
			lookup:       func(string) (string, bool) { return "1", true },
			wantStatus:   DockerProductionEvidenceStatusUnsupported,
			wantPlatform: DockerProductionEvidencePlatformUnsupported,
			wantEndpoint: DockerProductionEvidenceEndpointNone,
		},
		{
			name: "linux without opt in", platform: "linux",
			lookup:       func(string) (string, bool) { return "", false },
			wantStatus:   DockerProductionEvidenceStatusOptIn,
			wantPlatform: DockerProductionEvidencePlatformLinux,
			wantEndpoint: DockerObservationEndpointLocalUnix,
		},
		{
			name: "linux explicit opt in remains pending", platform: "linux",
			lookup: func(name string) (string, bool) {
				if name != DockerProductionEvidenceOptInEnv {
					t.Fatalf("unexpected environment lookup %q", name)
				}
				return "1", true
			},
			wantStatus:   DockerProductionEvidenceStatusPending,
			wantPlatform: DockerProductionEvidencePlatformLinux,
			wantEndpoint: DockerObservationEndpointLocalUnix,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			collector := LocalDockerProductionEvidenceCollector{
				platform: test.platform, arch: "test-arch", lookup: test.lookup,
			}
			observation, err := collector.Capture(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if observation.Status != test.wantStatus ||
				observation.PlatformClass != test.wantPlatform ||
				observation.EndpointClass != test.wantEndpoint || observation.RealDaemonContacted ||
				len(observation.Items) != MaxBackendChecks {
				t.Fatalf("unexpected fail-closed observation: %#v", observation)
			}
			for _, item := range observation.Items {
				if item.State != DockerProductionEvidenceStateNotObserved || item.Observed ||
					item.ProductionVerified || item.SufficientForStart {
					t.Fatalf("inactive collector produced evidence: %#v", item)
				}
			}
		})
	}
}

func TestDockerProductionEvidenceCanRecordMachineChecksWithoutAuthorizingStart(t *testing.T) {
	review := testDockerStartGateReview(t)
	environmentFingerprint := strings.Repeat("5", 64)
	items := newUnobservedDockerProductionEvidenceItems(review.AuthorityFingerprint,
		environmentFingerprint)
	for index := range items {
		items[index].Observed = true
		if index%2 == 0 {
			items[index].State = DockerProductionEvidenceStateVerified
			items[index].ProductionVerified = true
		} else {
			items[index].State = DockerProductionEvidenceStateFailed
		}
		items[index].EvidenceDigest = dockerProductionEvidenceItemDigest(items[index],
			review.AuthorityFingerprint, environmentFingerprint)
	}
	observation := DockerProductionEvidenceObservation{
		Source:                 DockerProductionEvidenceSourceLocal,
		TrustClass:             DockerProductionEvidenceTrustClass,
		Status:                 DockerProductionEvidenceStatusComplete,
		PlatformClass:          DockerProductionEvidencePlatformLinux,
		EndpointClass:          DockerObservationEndpointLocalUnix,
		SuiteFingerprint:       DockerProductionEvidenceSuiteFingerprint(),
		EnvironmentFingerprint: environmentFingerprint,
		RealDaemonContacted:    true, Items: items,
	}
	value, err := NewDockerProductionEvidence("production-evidence", strings.Repeat("6", 64),
		"operator", review, observation, true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if value.ObservedCount != MaxBackendChecks ||
		value.ProductionVerifiedCount != MaxBackendChecks/2 || value.SufficientCheckCount != 0 ||
		value.BlockerCount != MaxBackendChecks || value.StartGatePassed ||
		value.ContainerStartAuthorized || value.ProcessExecutionAuthorized ||
		value.OutputExportAuthorized || value.ArtifactCommitAuthorized {
		t.Fatalf("production evidence widened authority: %#v", value)
	}
	operation, err := NewDockerProductionEvidenceOperation(value.OperationKeyDigest, value)
	if err != nil || operation.RequestFingerprint != DockerProductionEvidenceRequestFingerprint(value) {
		t.Fatalf("production evidence operation invalid: %#v err=%v", operation, err)
	}

	tampered := value
	tampered.StartGatePassed = true
	if err := tampered.Validate(); err == nil {
		t.Fatal("production evidence authorized start")
	}
	tampered = value
	tampered.Items = append([]DockerProductionEvidenceItem(nil), value.Items...)
	tampered.Items[0].SufficientForStart = true
	if err := tampered.Validate(); err == nil {
		t.Fatal("production evidence item became sufficient for start")
	}
}

func TestDockerProductionEvidenceCollectorHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	collector := LocalDockerProductionEvidenceCollector{
		platform: "linux", arch: "test", lookup: func(string) (string, bool) { return "", false },
	}
	_, err := collector.Capture(ctx, DockerProductionEvidenceCaptureRequest{
		ReviewID: "review", RunID: "run", AuthorityFingerprint: strings.Repeat("1", 64),
	})
	if err == nil {
		t.Fatal("canceled evidence capture succeeded")
	}
}

func TestDockerProductionEvidenceValidationErrorsAreDeterministic(t *testing.T) {
	value := DockerProductionEvidence{}
	for range 20 {
		err := value.Validate()
		if err == nil || err.Error() !=
			"sandbox Docker production evidence id must be normalized and bounded UTF-8" {
			t.Fatalf("nondeterministic evidence validation error: %v", err)
		}
	}
	operation := DockerProductionEvidenceOperation{}
	for range 20 {
		err := operation.Validate()
		if err == nil || err.Error() !=
			"sandbox Docker production evidence operation evidence must be normalized and bounded UTF-8" {
			t.Fatalf("nondeterministic operation validation error: %v", err)
		}
	}
}

func testDockerStartGateReview(t *testing.T) DockerStartGateReview {
	t.Helper()
	binding := DockerStartGateReviewBinding{
		CleanupIntentID: "cleanup-intent", CleanupResultID: "cleanup-result",
		ApplicationIntentID: "application-intent", ApplicationResultID: "application-result",
		ProjectionID: "projection", ContainerPlanID: "container-plan",
		PreflightID: "preflight", RunID: "run", MissionID: "mission", WorkspaceID: "workspace",
		ManifestFingerprint:      strings.Repeat("1", 64),
		ThreatModelFingerprint:   strings.Repeat("2", 64),
		CleanupResultFingerprint: strings.Repeat("3", 64), MaxLogBytes: 4096,
	}
	review, err := NewDockerStartGateReview("start-gate-review", strings.Repeat("4", 64),
		"operator", binding, true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return review
}
