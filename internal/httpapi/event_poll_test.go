package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/store"
)

func TestRunEventPollUsesTheStreamHighWaterCursorWithoutGaps(t *testing.T) {
	fixture := newAPIFixture(t)
	timeline, err := fixture.store.ListRunEvents(context.Background(), fixture.run.ID)
	if err != nil || len(timeline) < 4 {
		t.Fatalf("load timeline: len=%d err=%v", len(timeline), err)
	}

	cursor := ""
	sequences := make([]int64, 0, len(timeline))
	for page := 0; page < len(timeline); page++ {
		poll := fixture.pollRequest(t, cursor, 2, nil)
		if poll.Code != http.StatusOK {
			t.Fatalf("poll status=%d body=%s", poll.Code, poll.Body.String())
		}
		assertSecurityHeaders(t, poll)
		view := decodeRunEventPoll(t, poll)
		if view.Version != RunEventPollVersion || view.RunID != fixture.run.ID || view.Cursor == "" ||
			len(view.Frames) == 0 || len(view.Frames) > 2 {
			t.Fatalf("invalid event poll view: %#v", view)
		}
		for _, frame := range view.Frames {
			if frame.Version != RunEventStreamVersion || frame.RequestID != poll.Header().Get("X-Request-ID") ||
				frame.RunID != fixture.run.ID || frame.Event.RunID != fixture.run.ID ||
				frame.Sequence != frame.Event.Sequence || frame.Cursor == "" {
				t.Fatalf("invalid poll frame: %#v", frame)
			}
			sequences = append(sequences, frame.Sequence)
		}
		if view.Cursor != view.Frames[len(view.Frames)-1].Cursor {
			t.Fatal("poll high-water cursor does not match the final committed frame")
		}
		cursor = view.Cursor
		if !view.HasMore {
			break
		}
	}
	if len(sequences) != len(timeline) {
		t.Fatalf("poll returned %d sequences, want %d", len(sequences), len(timeline))
	}
	for index, sequence := range sequences {
		if sequence != int64(index+1) || sequence != timeline[index].Sequence {
			t.Fatalf("sequence[%d]=%d timeline=%d", index, sequence, timeline[index].Sequence)
		}
	}

	empty := decodeRunEventPoll(t, fixture.pollRequest(t, cursor, 2, nil))
	if len(empty.Frames) != 0 || empty.HasMore || empty.Cursor != cursor {
		t.Fatalf("empty high-water poll changed state: %#v", empty)
	}
	if strings.Contains(mustJSON(t, empty), fixture.secret) ||
		strings.Contains(mustJSON(t, empty), fixture.leaseID) {
		t.Fatal("event poll exposed private state")
	}
}

func TestRunEventPollCursorObservesAnotherSQLiteConnectionAndResumesSSE(t *testing.T) {
	fixture := newAPIFixture(t)
	timeline, err := fixture.store.ListRunEvents(context.Background(), fixture.run.ID)
	if err != nil || len(timeline) == 0 {
		t.Fatalf("load timeline: len=%d err=%v", len(timeline), err)
	}
	cursor := encodeEventStreamCursor(fixture.run.ID, timeline[len(timeline)-1].Sequence)

	writerStore, err := store.Open(fixture.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writerStore.Close()
	created, err := application.NewNoteService(writerStore).Create(context.Background(),
		application.CreateNoteRequest{
			RunID: fixture.run.ID, Title: "desktop reconnect", Content: "durable event poll evidence",
		})
	if err != nil {
		t.Fatal(err)
	}
	poll := decodeRunEventPoll(t, fixture.pollRequest(t, cursor, 1, nil))
	if len(poll.Frames) != 1 || poll.Frames[0].Sequence != timeline[len(timeline)-1].Sequence+1 ||
		poll.Frames[0].Event.Type != "note.created" || poll.Frames[0].Event.SubjectID != created.ID {
		t.Fatalf("cross-connection poll is inconsistent: %#v", poll)
	}

	_, err = application.NewNoteService(writerStore).Create(context.Background(),
		application.CreateNoteRequest{
			RunID: fixture.run.ID, Title: "sse after desktop poll", Content: "shared cursor semantics",
		})
	if err != nil {
		t.Fatal(err)
	}
	fixture.api.eventStream = testEventStreamConfig(1, 500*time.Millisecond)
	stream := fixture.streamRequest(t, "?cursor="+url.QueryEscape(poll.Cursor), "")
	events := parseSSEEvents(t, stream.Body.Bytes())
	if len(events) != 1 || events[0].Data.Sequence != poll.Frames[0].Sequence+1 {
		t.Fatalf("SSE did not resume from poll cursor: %#v", events)
	}
}

func TestRunEventPollRejectsInvalidBoundariesBeforeReading(t *testing.T) {
	fixture := newAPIFixture(t)
	valid := encodeEventStreamCursor(fixture.run.ID, 1)
	crossRun := encodeEventStreamCursor("run-other", 1)
	tests := []struct {
		name   string
		path   string
		header http.Header
	}{
		{name: "unknown query", path: "?unknown=true"},
		{name: "blank cursor", path: "?cursor="},
		{name: "repeated cursor", path: "?cursor=" + valid + "&cursor=" + valid},
		{name: "malformed cursor", path: "?cursor=not-a-cursor"},
		{name: "cross Run cursor", path: "?cursor=" + url.QueryEscape(crossRun)},
		{name: "zero limit", path: "?limit=0"},
		{name: "oversized limit", path: "?limit=101"},
		{name: "repeated limit", path: "?limit=1&limit=2"},
		{name: "SSE header", header: http.Header{"Last-Event-Id": []string{valid}}},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			response := fixture.pollPathRequest(t, current.path, current.header)
			assertAPIError(t, response, http.StatusBadRequest, "INVALID_ARGUMENT")
		})
	}

	missing := fixture.request(t, http.MethodGet, "/api/v1/runs/missing/events/poll", testAccessToken,
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
	assertAPIError(t, missing, http.StatusNotFound, "NOT_FOUND")
	unauthorized := fixture.request(t, http.MethodGet,
		"/api/v1/runs/"+fixture.run.ID+"/events/poll", "",
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
	assertAPIError(t, unauthorized, http.StatusUnauthorized, "POLICY_DENIED")
}

func (f *apiFixture) pollRequest(t *testing.T, cursor string, limit int,
	header http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	query := "?limit=" + url.QueryEscape(strconv.Itoa(limit))
	if cursor != "" {
		query += "&cursor=" + url.QueryEscape(cursor)
	}
	return f.pollPathRequest(t, query, header)
}

func (f *apiFixture) pollPathRequest(t *testing.T, query string,
	header http.Header,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet,
		"http://127.0.0.1/api/v1/runs/"+f.run.ID+"/events/poll"+query, nil)
	request.Host = "127.0.0.1:8765"
	request.RemoteAddr = "127.0.0.1:45000"
	request.Header.Set("Authorization", "Bearer "+testAccessToken)
	for key, values := range header {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response := httptest.NewRecorder()
	f.api.ServeHTTP(response, request)
	return response
}

func decodeRunEventPoll(t *testing.T, response *httptest.ResponseRecorder) RunEventPollView {
	t.Helper()
	var envelope apiTestEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var view RunEventPollView
	if err := json.Unmarshal(envelope.Data, &view); err != nil {
		t.Fatal(err)
	}
	return view
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
