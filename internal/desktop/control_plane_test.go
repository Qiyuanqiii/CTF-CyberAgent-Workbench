package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/store"
)

const desktopControlPlaneTestToken = "desktop-control-plane-read-token-0123456789"
const desktopControlPlaneControlToken = "desktop-control-plane-control-token-012345"

type desktopAPIEnvelope struct {
	Version   string          `json:"version"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

func TestControlPlaneSharesCLIStoreAndReopensFromAHighWaterCursor(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "shared.db")
	cliStore, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer cliStore.Close()
	_, run, err := application.NewRunService(cliStore).Create(context.Background(),
		application.CreateRunRequest{
			Goal: "verify Desktop and CLI SQLite concurrency", Profile: "review", ModelRoute: "review",
			Budget: domain.Budget{MaxTurns: 4},
		})
	if err != nil {
		t.Fatal(err)
	}
	run, err = application.NewRunService(cliStore).Start(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}

	first, err := OpenControlPlane(ControlPlaneConfig{
		DatabasePath: databasePath, ReadToken: desktopControlPlaneTestToken, AppVersion: "desktop-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	initial := readDesktopEventPoll(t, first.Handler(), run.ID, "")
	if len(initial.Frames) == 0 || initial.Cursor == "" {
		t.Fatalf("initial Desktop timeline is empty: %#v", initial)
	}

	created, err := application.NewNoteService(cliStore).Create(context.Background(),
		application.CreateNoteRequest{
			RunID: run.ID, Title: "CLI concurrent write", Content: "visible to the open Desktop connection",
		})
	if err != nil {
		t.Fatal(err)
	}
	afterWrite := readDesktopEventPoll(t, first.Handler(), run.ID, initial.Cursor)
	if len(afterWrite.Frames) != 1 || afterWrite.Frames[0].Event.Type != "note.created" ||
		afterWrite.Frames[0].Event.SubjectID != created.ID {
		t.Fatalf("Desktop did not observe the CLI write: %#v", afterWrite)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("idempotent close failed: %v", err)
	}

	secondNote, err := application.NewNoteService(cliStore).Create(context.Background(),
		application.CreateNoteRequest{
			RunID: run.ID, Title: "CLI write while Desktop closed", Content: "visible after reopen",
		})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenControlPlane(ControlPlaneConfig{
		DatabasePath: databasePath, ReadToken: desktopControlPlaneTestToken, AppVersion: "desktop-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	afterReopen := readDesktopEventPoll(t, reopened.Handler(), run.ID, afterWrite.Cursor)
	if len(afterReopen.Frames) != 1 || afterReopen.Frames[0].Event.SubjectID != secondNote.ID ||
		afterReopen.Frames[0].Sequence != afterWrite.Frames[0].Sequence+1 {
		t.Fatalf("Desktop reopen did not resume exactly: %#v", afterReopen)
	}
}

func TestControlPlaneConcurrentOpenOnAnInitializedDatabase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "concurrent.db")
	initialized, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := initialized.Close(); err != nil {
		t.Fatal(err)
	}

	const workers = 6
	start := make(chan struct{})
	results := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			plane, err := OpenControlPlane(ControlPlaneConfig{
				DatabasePath: databasePath, ReadToken: desktopControlPlaneTestToken,
				AppVersion: fmt.Sprintf("desktop-test-%d", index),
			})
			if err != nil {
				results <- err
				return
			}
			response := desktopAPIRequest(plane.Handler(), "/api/v1/health")
			if response.Code != http.StatusOK {
				results <- fmt.Errorf("health status %d: %s", response.Code, response.Body.String())
				_ = plane.Close()
				return
			}
			results <- plane.Close()
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestControlPlaneSeparatesRunCreationFromExistingRunControls(t *testing.T) {
	plane, err := OpenControlPlane(ControlPlaneConfig{
		DatabasePath: filepath.Join(t.TempDir(), "creation.db"),
		ReadToken:    desktopControlPlaneTestToken, ControlToken: desktopControlPlaneControlToken,
		RunCreationEnabled: true, AppVersion: "desktop-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer plane.Close()
	workspace := store.WorkspaceRecord{ID: "workspace-desktop-create", Name: "desktop-create",
		RootPath: t.TempDir()}
	if err := plane.stateStore.SaveWorkspace(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	body := `{"version":"run_creation.v1","goal":"Desktop Run",` +
		`"workspace_id":"` + workspace.ID + `"}`
	created := desktopControlRequest(plane.Handler(), http.MethodPost, "/api/v1/runs",
		"desktop-run-create-operation-0001", body)
	if created.Code != http.StatusAccepted {
		t.Fatalf("Run creation status=%d body=%s", created.Code, created.Body.String())
	}
	var envelope desktopAPIEnvelope
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var result httpapi.RunCreationControlView
	if err := json.Unmarshal(envelope.Data, &result); err != nil || result.Run.ID == "" {
		t.Fatalf("Run creation response=%#v err=%v", result, err)
	}
	profile := desktopControlRequest(plane.Handler(), http.MethodPost,
		"/api/v1/runs/"+result.Run.ID+"/execution-profile",
		"desktop-profile-operation-0001", `{"profile":"docker"}`)
	if profile.Code != http.StatusNotFound {
		t.Fatalf("Run creation capability widened profile control: status=%d body=%s",
			profile.Code, profile.Body.String())
	}
}

func TestControlPlaneSeparatesSessionMessagesFromOtherControls(t *testing.T) {
	plane, err := OpenControlPlane(ControlPlaneConfig{
		DatabasePath: filepath.Join(t.TempDir(), "messages.db"),
		ReadToken:    desktopControlPlaneTestToken, ControlToken: desktopControlPlaneControlToken,
		SessionMessageEnabled: true, AppVersion: "desktop-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer plane.Close()
	runs := application.NewRunService(plane.stateStore)
	_, created, err := runs.Create(t.Context(), application.CreateRunRequest{
		Goal: "Desktop Session message", Profile: "review",
		Budget: domain.Budget{MaxTurns: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runs.Start(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	requestPath := "/api/v1/sessions/" + run.SessionID + "/messages"
	submitted := desktopControlRequest(plane.Handler(), http.MethodPost, requestPath,
		"desktop-session-message-operation-0001",
		`{"version":"session_message_submission.v1","content":"Review the current diff"}`)
	if submitted.Code != http.StatusAccepted {
		t.Fatalf("Session message status=%d body=%s", submitted.Code, submitted.Body.String())
	}
	var envelope desktopAPIEnvelope
	if err := json.Unmarshal(submitted.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var result httpapi.SessionMessageControlView
	if err := json.Unmarshal(envelope.Data, &result); err != nil ||
		result.RunID != run.ID || result.SessionID != run.SessionID ||
		result.Steering.Status != string(domain.OperatorSteeringPending) ||
		result.ExecutionStarted || result.ModelCalled || result.ToolCalled || result.CapabilityGrant {
		t.Fatalf("Session message response=%#v err=%v", result, err)
	}
	history, err := plane.stateStore.ListSessionMessages(t.Context(), run.SessionID, true)
	if err != nil || len(history) != 0 {
		t.Fatalf("Session message was committed before Supervisor delivery: %#v err=%v", history, err)
	}
	creation := desktopControlRequest(plane.Handler(), http.MethodPost, "/api/v1/runs",
		"desktop-session-message-operation-0002",
		`{"version":"run_creation.v1","goal":"blocked","workspace_id":"workspace"}`)
	if creation.Code != http.StatusNotFound {
		t.Fatalf("Session capability widened Run creation: status=%d body=%s",
			creation.Code, creation.Body.String())
	}
	profile := desktopControlRequest(plane.Handler(), http.MethodPost,
		"/api/v1/runs/"+run.ID+"/execution-profile",
		"desktop-session-message-operation-0003", `{"profile":"preview"}`)
	if profile.Code != http.StatusNotFound {
		t.Fatalf("Session capability widened profile control: status=%d body=%s",
			profile.Code, profile.Body.String())
	}
}

func desktopControlRequest(handler http.Handler, method string, path string,
	key string, body string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	request.Host = "127.0.0.1:8765"
	request.RemoteAddr = "127.0.0.1:45678"
	request.Header.Set("Authorization", "Bearer "+desktopControlPlaneControlToken)
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func readDesktopEventPoll(t *testing.T, handler http.Handler, runID string,
	cursor string,
) httpapi.RunEventPollView {
	t.Helper()
	path := "/api/v1/runs/" + url.PathEscape(runID) + "/events/poll?limit=100"
	if cursor != "" {
		path += "&cursor=" + url.QueryEscape(cursor)
	}
	response := desktopAPIRequest(handler, path)
	if response.Code != http.StatusOK {
		t.Fatalf("event poll status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope desktopAPIEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var view httpapi.RunEventPollView
	if err := json.Unmarshal(envelope.Data, &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func desktopAPIRequest(handler http.Handler, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
	request.Host = "127.0.0.1:8765"
	request.RemoteAddr = "127.0.0.1:45678"
	request.Header.Set("Authorization", "Bearer "+desktopControlPlaneTestToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
