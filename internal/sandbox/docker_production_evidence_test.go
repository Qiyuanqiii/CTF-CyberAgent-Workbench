package sandbox

import (
	"context"
	"errors"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type dockerProductionEvidenceHarnessTestTransport struct {
	endpoint    DockerObservationEndpoint
	imageDigest string
	imageUser   string
	owned       int
	calls       []string
}

func newDockerProductionEvidenceHarnessTestTransport(t *testing.T,
	imageDigest string,
) *dockerProductionEvidenceHarnessTestTransport {
	t.Helper()
	endpoint, err := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	if err != nil {
		t.Fatal(err)
	}
	return &dockerProductionEvidenceHarnessTestTransport{
		endpoint: endpoint, imageDigest: imageDigest, imageUser: "65532:65532",
	}
}

func (transport *dockerProductionEvidenceHarnessTestTransport) Endpoint() DockerObservationEndpoint {
	return transport.endpoint
}

func (transport *dockerProductionEvidenceHarnessTestTransport) Ping(context.Context) error {
	transport.calls = append(transport.calls, "ping")
	return nil
}

func (transport *dockerProductionEvidenceHarnessTestTransport) Version(context.Context) (
	DockerDaemonVersion, error,
) {
	transport.calls = append(transport.calls, "version")
	return DockerDaemonVersion{APIVersion: "1.47", MinAPIVersion: "1.24",
		EngineVersion: "27.5.1", GitCommit: "abc123", OSType: "linux",
		Architecture: "amd64"}, nil
}

func (transport *dockerProductionEvidenceHarnessTestTransport) Info(context.Context) (
	DockerDaemonInfo, error,
) {
	transport.calls = append(transport.calls, "info")
	return DockerDaemonInfo{ID: "daemon-id", Name: "test-host",
		DockerRootDir: "/private/docker", ServerVersion: "27.5.1",
		OperatingSystem: "Test Linux", OSType: "linux", Architecture: "amd64",
		Driver: "overlay2", CgroupDriver: "systemd", CgroupVersion: "2",
		DefaultRuntime: "runc", NCPU: 8, MemoryBytes: 8 << 30, PidsLimit: true,
		SecurityOptions: []string{"name=seccomp,profile=builtin"}}, nil
}

func (transport *dockerProductionEvidenceHarnessTestTransport) InspectImage(_ context.Context,
	imageDigest string,
) (DockerImageInspection, error) {
	transport.calls = append(transport.calls, "inspect-image")
	if imageDigest != transport.imageDigest {
		return DockerImageInspection{}, errors.New("unexpected image digest")
	}
	return DockerImageInspection{ID: "sha256:" + strings.Repeat("a", 64),
		RepoDigests: []string{"example.invalid/workbench@" + imageDigest},
		OSType:      "linux", Architecture: "amd64", SizeBytes: 1 << 20,
		User: transport.imageUser, RootFSType: "layers", GraphDriver: "overlay2"}, nil
}

func (transport *dockerProductionEvidenceHarnessTestTransport) ListProductionEvidenceResources(
	_ context.Context, _ string,
) (DockerProductionEvidenceHarnessInventory, error) {
	transport.calls = append(transport.calls, "list-owned")
	resources := make([]string, transport.owned)
	for index := range resources {
		resources[index] = fingerprint("test-owned-resource", strings.Repeat("x", index+1))
	}
	return NewDockerProductionEvidenceHarnessInventory(transport.endpoint, resources)
}

func TestLocalDockerProductionEvidenceCollectorFailsClosedByPlatformAndOptIn(t *testing.T) {
	request := testDockerProductionEvidenceCaptureRequest(t)
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
		AttemptID: "attempt", LeaseGeneration: 1,
		EndpointClass:       DockerObservationEndpointLocalUnix,
		EndpointFingerprint: testDockerProductionEvidenceCaptureRequest(t).EndpointFingerprint,
		DeadlineAt:          time.Now().UTC().Add(time.Minute),
	})
	if err == nil {
		t.Fatal("canceled evidence capture succeeded")
	}
}

func TestDockerProductionEvidenceCollectorRejectsExpiredDeadline(t *testing.T) {
	request := testDockerProductionEvidenceCaptureRequest(t)
	request.DeadlineAt = time.Now().UTC().Add(-time.Millisecond)
	collector := LocalDockerProductionEvidenceCollector{
		platform: "linux", arch: "test", lookup: func(string) (string, bool) { return "1", true },
	}
	if _, err := collector.Capture(context.Background(), request); err != context.DeadlineExceeded {
		t.Fatalf("expired production evidence deadline reached collector: %v", err)
	}
}

func TestLinuxDockerProductionEvidenceHarnessIsExactReadOnlyAndNonAuthorizing(t *testing.T) {
	imageDigest := "sha256:" + strings.Repeat("d", 64)
	transport := newDockerProductionEvidenceHarnessTestTransport(t, imageDigest)
	collector := LocalDockerProductionEvidenceCollector{
		platform: "linux", arch: "amd64",
		lookup: func(name string) (string, bool) {
			if name != DockerProductionEvidenceOptInEnv {
				t.Fatalf("unexpected environment lookup %q", name)
			}
			return "1", true
		},
		transport: transport,
	}
	if !collector.HarnessEnabled() {
		t.Fatal("explicit Linux harness opt-in was ignored")
	}
	base := testDockerProductionEvidenceCaptureRequest(t)
	harnessRequest := DockerProductionEvidenceHarnessRequest{
		DockerProductionEvidenceCaptureRequest: base,
		ImageDigest:                            imageDigest, IntentFingerprint: strings.Repeat("2", 64),
		ControlReconciliationFingerprint: strings.Repeat("3", 64),
	}
	inventory, err := collector.ReconcileHarness(context.Background(), harnessRequest)
	if err != nil || !inventory.RealDaemonContacted || inventory.DaemonReadCount != 1 ||
		inventory.OwnedResourceCount != 0 {
		t.Fatalf("unexpected harness reconciliation: %#v err=%v", inventory, err)
	}
	observation, err := collector.CaptureHarness(context.Background(),
		DockerProductionEvidenceHarnessCaptureRequest{
			DockerProductionEvidenceHarnessRequest: harnessRequest,
			HarnessReconciliationFingerprint:       strings.Repeat("4", 64),
		})
	if err != nil {
		t.Fatal(err)
	}
	if observation.Status != DockerProductionEvidenceStatusComplete ||
		!observation.RealDaemonContacted || len(observation.Items) != MaxBackendChecks {
		t.Fatalf("unexpected harness observation: %#v", observation)
	}
	for _, item := range observation.Items {
		if !item.Observed || item.ProductionVerified || item.SufficientForStart {
			t.Fatalf("harness probe widened authority: %#v", item)
		}
	}
	wantCalls := []string{"list-owned", "ping", "version", "info", "inspect-image"}
	if !reflect.DeepEqual(transport.calls, wantCalls) {
		t.Fatalf("harness calls=%v, want %v", transport.calls, wantCalls)
	}
	interfaceType := reflect.TypeOf((*DockerProductionEvidenceHarnessTransport)(nil)).Elem()
	for _, forbidden := range []string{"Pull", "Create", "Start", "Exec", "Run", "Remove", "Delete"} {
		for index := 0; index < interfaceType.NumMethod(); index++ {
			if strings.Contains(interfaceType.Method(index).Name, forbidden) {
				t.Fatalf("harness transport exposes forbidden method %q",
					interfaceType.Method(index).Name)
			}
		}
	}
}

func TestLinuxDockerProductionEvidenceHarnessRejectsOwnedResourceCollision(t *testing.T) {
	imageDigest := "sha256:" + strings.Repeat("e", 64)
	transport := newDockerProductionEvidenceHarnessTestTransport(t, imageDigest)
	transport.owned = 1
	collector := LocalDockerProductionEvidenceCollector{
		platform: "linux", arch: "amd64",
		lookup:    func(string) (string, bool) { return "1", true },
		transport: transport,
	}
	request := DockerProductionEvidenceHarnessRequest{
		DockerProductionEvidenceCaptureRequest: testDockerProductionEvidenceCaptureRequest(t),
		ImageDigest:                            imageDigest, IntentFingerprint: strings.Repeat("2", 64),
		ControlReconciliationFingerprint: strings.Repeat("3", 64),
	}
	if _, err := collector.ReconcileHarness(context.Background(), request); err == nil {
		t.Fatal("harness adopted a pre-existing labeled resource")
	}
	if !reflect.DeepEqual(transport.calls, []string{"list-owned"}) {
		t.Fatalf("collision reached capture probes: %v", transport.calls)
	}
}

func TestDockerProductionEvidenceHarnessRealDaemonOptIn(t *testing.T) {
	if runtime.GOOS != DockerProductionEvidencePlatformLinux {
		t.Skip("the production-evidence harness uses the fixed Linux local Unix endpoint")
	}
	if os.Getenv(DockerProductionEvidenceOptInEnv) != "1" {
		t.Skip("set CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE=1 for an opt-in real-daemon probe")
	}
	imageDigest := strings.TrimSpace(os.Getenv("CYBERAGENT_DOCKER_READONLY_IMAGE_DIGEST"))
	if !ValidOCIImageDigest(imageDigest) {
		t.Fatal("CYBERAGENT_DOCKER_READONLY_IMAGE_DIGEST must be an already-present OCI sha256 digest")
	}
	collector := NewLocalDockerProductionEvidenceCollector()
	if !collector.HarnessEnabled() {
		t.Fatal("explicit production-evidence harness opt-in was not enabled")
	}
	request := DockerProductionEvidenceHarnessRequest{
		DockerProductionEvidenceCaptureRequest: testDockerProductionEvidenceCaptureRequest(t),
		ImageDigest:                            imageDigest, IntentFingerprint: strings.Repeat("2", 64),
		ControlReconciliationFingerprint: strings.Repeat("3", 64),
	}
	request.AttemptID = "production-evidence-integration"
	inventory, err := collector.ReconcileHarness(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.OwnedResourceCount != 0 || inventory.DaemonReadCount != 1 ||
		!inventory.RealDaemonContacted {
		t.Fatalf("unexpected production-evidence inventory: %#v", inventory)
	}
	observation, err := collector.CaptureHarness(context.Background(),
		DockerProductionEvidenceHarnessCaptureRequest{
			DockerProductionEvidenceHarnessRequest: request,
			HarnessReconciliationFingerprint:       strings.Repeat("4", 64),
		})
	if err != nil {
		t.Fatal(err)
	}
	if observation.Status != DockerProductionEvidenceStatusComplete ||
		!observation.RealDaemonContacted || len(observation.Items) != MaxBackendChecks {
		t.Fatalf("unexpected production-evidence observation: %#v", observation)
	}
	for _, item := range observation.Items {
		if item.State != DockerProductionEvidenceStateFailed || !item.Observed ||
			item.ProductionVerified || item.SufficientForStart {
			t.Fatalf("real-daemon probe widened production authority: %#v", item)
		}
	}
}

func TestDockerProductionEvidenceAttemptRemainsGenerationFencedAndNonAuthorizing(t *testing.T) {
	review := testDockerStartGateReview(t)
	endpoint, err := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := NewDockerProductionEvidenceAttempt("production-attempt",
		strings.Repeat("6", 64), review.RequestedBy, review, endpoint, true,
		DefaultDockerProductionEvidenceCaptureTimeout, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	lease := DockerProductionEvidenceAttemptLease{AttemptID: attempt.ID,
		LeaseID: "lease", OwnerID: "worker", Generation: 1,
		Status:     DockerProductionEvidenceAttemptLeaseActive,
		AcquiredAt: now, ExpiresAt: now.Add(DefaultDockerProductionEvidenceAttemptLeaseTTL)}
	reconciliation, err := NewDockerProductionEvidenceReconciliation(attempt, lease,
		now.Add(time.Millisecond))
	if err != nil || reconciliation.RealDaemonContacted || reconciliation.DaemonReadCount != 0 {
		t.Fatalf("unexpected reconciliation: %#v err=%v", reconciliation, err)
	}
	collector := NewLocalDockerProductionEvidenceCollector()
	observation, err := collector.Capture(context.Background(),
		DockerProductionEvidenceCaptureRequest{ReviewID: review.ID, RunID: review.RunID,
			AuthorityFingerprint: review.AuthorityFingerprint, AttemptID: attempt.ID,
			LeaseGeneration: lease.Generation, EndpointClass: endpoint.Class,
			EndpointFingerprint: endpoint.Fingerprint,
			DeadlineAt:          now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := NewDockerProductionEvidence("production-evidence",
		attempt.OperationKeyDigest, review.RequestedBy, review, observation, true,
		now.Add(2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewDockerProductionEvidenceAttemptResult(attempt, lease,
		reconciliation, evidence)
	if err != nil || result.RealDaemonContacted || result.ContainerStartAuthorized ||
		result.ProcessExecutionAuthorized || result.ArtifactCommitAuthorized {
		t.Fatalf("attempt result widened authority: %#v err=%v", result, err)
	}
	releasedAt := result.CreatedAt
	lease.Status, lease.ReleasedAt = DockerProductionEvidenceAttemptLeaseReleased, &releasedAt
	record := DockerProductionEvidenceAttemptRecord{Attempt: attempt, Lease: lease,
		Reconciliations: []DockerProductionEvidenceReconciliation{reconciliation}, Result: &result}
	if err := record.Validate(); err != nil || record.StatusAt(time.Now().UTC()) != "evidence_committed" {
		t.Fatalf("invalid completed attempt: %#v err=%v", record, err)
	}
	tampered := attempt
	tampered.RealDaemonContactAuthorized = true
	if err := tampered.Validate(); err == nil {
		t.Fatal("attempt authorized daemon contact")
	}
}

func testDockerProductionEvidenceCaptureRequest(t *testing.T) DockerProductionEvidenceCaptureRequest {
	t.Helper()
	endpoint, err := NewDockerObservationEndpoint(DockerObservationEndpointLocalUnix)
	if err != nil {
		t.Fatal(err)
	}
	return DockerProductionEvidenceCaptureRequest{
		ReviewID: "review", RunID: "run", AuthorityFingerprint: strings.Repeat("1", 64),
		AttemptID: "attempt", LeaseGeneration: 1, EndpointClass: endpoint.Class,
		EndpointFingerprint: endpoint.Fingerprint,
		DeadlineAt:          time.Now().UTC().Add(time.Minute),
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
