package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
)

type wakeWorkerHealthFake struct {
	health application.RunWakeWorkerHealth
}

func (f wakeWorkerHealthFake) Health() application.RunWakeWorkerHealth { return f.health }

func TestRuntimeCapabilitiesAreReadOnlyAndDefaultClosed(t *testing.T) {
	fixture := newAPIFixture(t)
	response := performSessionMessageRequest(t, fixture.api, http.MethodGet,
		"/api/v1/capabilities", testAccessToken, "", "", nil)
	var view RuntimeCapabilitiesView
	decodeDataStatus(t, response, http.StatusOK, &view)
	if view.ProtocolVersion != RuntimeCapabilitiesProtocolVersion ||
		view.RunControlEnabled != fixture.api.controlEnabled ||
		view.RunCreationEnabled != fixture.api.runCreationEnabled ||
		view.SessionMessageEnabled != fixture.api.sessionMessageEnabled ||
		view.FileEditProposalEnabled || view.ProviderCredentialEnabled ||
		view.RunWakeWorkerEnabled ||
		view.ProcessExecutionEnabled || view.ShellExecutionEnabled ||
		view.DockerExecutionEnabled || view.WakeWorker.Enabled ||
		view.WakeWorker.State != "disabled" || view.WakeWorker.Active ||
		view.WakeWorker.RuntimeEnableSupported || view.WakeWorker.PersistentService ||
		view.WakeWorker.Concurrency != 1 || view.WakeWorker.MaxSteps != 1 {
		t.Fatalf("default capability projection widened authority: %#v", view)
	}
	assertAPIError(t, performSessionMessageRequest(t, fixture.api, http.MethodGet,
		"/api/v1/capabilities", testControlToken, "", "", nil),
		http.StatusUnauthorized, "POLICY_DENIED")
}

func TestRuntimeCapabilitiesProjectBoundedWorkerHealthWithoutPrivateState(t *testing.T) {
	fixture := newAPIFixture(t)
	source := wakeWorkerHealthFake{health: application.RunWakeWorkerHealth{
		ProtocolVersion: application.RunWakeWorkerHealthProtocolVersion,
		State:           application.RunWakeWorkerDraining, Active: true,
		PollIntervalMillis: (2 * time.Second).Milliseconds(),
		Concurrency:        application.RunWakeWorkerConcurrency,
		MaxSteps:           application.RunWakeWorkerMaxSteps,
	}}
	api, err := New(fixture.store, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken,
		RunWakeWorkerEnabled: true, RunWakeWorkerHealthSource: source,
		AppVersion: "worker-health-test"})
	if err != nil {
		t.Fatal(err)
	}
	response := performSessionMessageRequest(t, api, http.MethodGet,
		"/api/v1/capabilities", testAccessToken, "", "", nil)
	var view RuntimeCapabilitiesView
	decodeDataStatus(t, response, http.StatusOK, &view)
	if !view.RunWakeWorkerEnabled || !view.WakeWorker.Enabled ||
		view.WakeWorker.State != "draining" || !view.WakeWorker.Active ||
		view.WakeWorker.PollIntervalMillis != 2000 ||
		view.WakeWorker.RuntimeEnableSupported || view.WakeWorker.PersistentService {
		t.Fatalf("worker health projection is invalid: %#v", view)
	}
	raw := strings.ToLower(response.Body.String())
	for _, forbidden := range []string{"owner", "lease", "token", "private_error", "run_id"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("worker capability projection exposed %q: %s", forbidden, raw)
		}
	}
}

func TestRuntimeCapabilitiesRejectWorkerWithoutControlToken(t *testing.T) {
	fixture := newAPIFixture(t)
	_, err := New(fixture.store, Config{AccessToken: testAccessToken,
		RunWakeWorkerEnabled: true, RunWakeWorkerHealthSource: wakeWorkerHealthFake{
			health: application.RunWakeWorkerHealth{
				ProtocolVersion: application.RunWakeWorkerHealthProtocolVersion,
				State: application.RunWakeWorkerReady,
				PollIntervalMillis: application.DefaultRunWakeWorkerInterval.Milliseconds(),
				Concurrency: application.RunWakeWorkerConcurrency,
				MaxSteps: application.RunWakeWorkerMaxSteps,
			},
		}, AppVersion: "worker-control-token-test"})
	if err == nil || apperror.CodeOf(err) != apperror.CodeInvalidArgument {
		t.Fatalf("worker without control token error=%v", err)
	}
}

func TestRuntimeCapabilitiesRejectImpossibleWorkerHealth(t *testing.T) {
	fixture := newAPIFixture(t)
	api, err := New(fixture.store, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken, RunWakeWorkerEnabled: true,
		RunWakeWorkerHealthSource: wakeWorkerHealthFake{health: application.RunWakeWorkerHealth{
			ProtocolVersion: application.RunWakeWorkerHealthProtocolVersion,
			State: application.RunWakeWorkerStopped, Active: true,
			PollIntervalMillis: application.DefaultRunWakeWorkerInterval.Milliseconds(),
			Concurrency: application.RunWakeWorkerConcurrency,
			MaxSteps: application.RunWakeWorkerMaxSteps,
		}}, AppVersion: "invalid-worker-health-test"})
	if err != nil {
		t.Fatal(err)
	}
	response := performSessionMessageRequest(t, api, http.MethodGet,
		"/api/v1/capabilities", testAccessToken, "", "", nil)
	assertAPIError(t, response, http.StatusInternalServerError, "INTERNAL")
}

func TestRuntimeCapabilitiesRejectMismatchedWorkerConfiguration(t *testing.T) {
	fixture := newAPIFixture(t)
	if _, err := New(fixture.store, Config{AccessToken: testAccessToken,
		RunWakeWorkerEnabled: true}); err == nil {
		t.Fatal("enabled worker without health source was accepted")
	}
	if _, err := New(fixture.store, Config{AccessToken: testAccessToken,
		RunWakeWorkerHealthSource: wakeWorkerHealthFake{}}); err == nil {
		t.Fatal("disabled worker retained a health source")
	}
}
