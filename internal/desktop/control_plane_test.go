package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/store"
)

const desktopControlPlaneTestToken = "desktop-control-plane-read-token-0123456789"

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
