package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/store"
)

type parsedSSEEvent struct {
	ID    string
	Event string
	Data  RunEventStreamView
}

func TestRunEventStreamReplaysBoundedEventsAndResumesExactly(t *testing.T) {
	fixture := newAPIFixture(t)
	fixture.api.eventStream = testEventStreamConfig(3, 500*time.Millisecond)

	first := fixture.streamRequest(t, "", "")
	if first.Code != http.StatusOK || !strings.HasPrefix(first.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("first event stream failed: status=%d headers=%#v body=%s",
			first.Code, first.Header(), first.Body.String())
	}
	assertSecurityHeaders(t, first)
	if strings.Contains(first.Body.String(), fixture.secret) || strings.Contains(first.Body.String(), fixture.leaseID) ||
		strings.Contains(first.Body.String(), `"lease_id"`) {
		t.Fatal("event stream exposed a secret or execution fencing token")
	}
	firstEvents := parseSSEEvents(t, first.Body.Bytes())
	assertSSESequences(t, firstEvents, 1, 2, 3)
	for _, event := range firstEvents {
		if event.ID != event.Data.Cursor || event.Data.RequestID != first.Header().Get("X-Request-ID") ||
			event.Data.RunID != fixture.run.ID || event.Data.Event.Sequence != event.Data.Sequence {
			t.Fatalf("event stream envelope is inconsistent: %#v", event)
		}
	}

	fixture.api.eventStream = testEventStreamConfig(2, 500*time.Millisecond)
	second := fixture.streamRequest(t, "", firstEvents[len(firstEvents)-1].ID)
	secondEvents := parseSSEEvents(t, second.Body.Bytes())
	assertSSESequences(t, secondEvents, 4, 5)

	fixture.api.eventStream = testEventStreamConfig(1, 500*time.Millisecond)
	third := fixture.streamRequest(t, "?cursor="+url.QueryEscape(secondEvents[len(secondEvents)-1].ID), "")
	thirdEvents := parseSSEEvents(t, third.Body.Bytes())
	assertSSESequences(t, thirdEvents, 6)
}

func TestRunEventStreamRejectsInvalidOrCrossRunCursorsBeforeStreaming(t *testing.T) {
	fixture := newAPIFixture(t)
	fixture.api.eventStream = testEventStreamConfig(1, 100*time.Millisecond)
	valid := encodeEventStreamCursor(fixture.run.ID, 1)
	crossRun := encodeEventStreamCursor("run-other", 1)
	invalidJSON := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"s":1,"r":"wrong","extra":true}`))

	tests := []struct {
		name        string
		query       string
		lastEventID []string
	}{
		{name: "unknown query", query: "?unknown=true"},
		{name: "blank cursor", query: "?cursor="},
		{name: "repeated cursor", query: "?cursor=" + valid + "&cursor=" + valid},
		{name: "malformed cursor", query: "?cursor=not-a-cursor"},
		{name: "cross Run cursor", query: "?cursor=" + url.QueryEscape(crossRun)},
		{name: "unknown cursor field", query: "?cursor=" + url.QueryEscape(invalidJSON)},
		{name: "blank header", lastEventID: []string{" "}},
		{name: "repeated header", lastEventID: []string{valid, valid}},
		{name: "query and header", query: "?cursor=" + valid, lastEventID: []string{valid}},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			response := fixture.streamRequestValues(t, current.query, current.lastEventID)
			assertAPIError(t, response, http.StatusBadRequest, "INVALID_ARGUMENT")
			if strings.HasPrefix(response.Header().Get("Content-Type"), "text/event-stream") {
				t.Fatal("invalid cursor committed an SSE response")
			}
		})
	}

	missing := fixture.request(t, http.MethodGet, "/api/v1/runs/missing/events/stream", testAccessToken,
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
	assertAPIError(t, missing, http.StatusNotFound, "NOT_FOUND")
	unauthorized := fixture.request(t, http.MethodGet,
		"/api/v1/runs/"+fixture.run.ID+"/events/stream", "",
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
	assertAPIError(t, unauthorized, http.StatusUnauthorized, "POLICY_DENIED")
}

func TestRunEventStreamHeartbeatsWithoutInventingDurableEvents(t *testing.T) {
	fixture := newAPIFixture(t)
	timeline, err := fixture.store.ListRunEvents(context.Background(), fixture.run.ID)
	if err != nil || len(timeline) == 0 {
		t.Fatalf("load timeline: len=%d err=%v", len(timeline), err)
	}
	fixture.api.eventStream = EventStreamConfig{
		PollInterval: 5 * time.Millisecond, HeartbeatInterval: 10 * time.Millisecond,
		MaxDuration: 45 * time.Millisecond, WriteTimeout: 20 * time.Millisecond,
		BatchSize: 2, MaxEvents: 10,
	}
	cursor := encodeEventStreamCursor(fixture.run.ID, timeline[len(timeline)-1].Sequence)
	started := time.Now()
	response := fixture.streamRequest(t, "?cursor="+url.QueryEscape(cursor), "")
	elapsed := time.Since(started)
	if response.Code != http.StatusOK || len(parseSSEEvents(t, response.Body.Bytes())) != 0 ||
		bytes.Count(response.Body.Bytes(), []byte(": heartbeat\n\n")) < 2 {
		t.Fatalf("heartbeat-only stream is invalid: status=%d body=%s", response.Code, response.Body.String())
	}
	if elapsed < 25*time.Millisecond || elapsed > 500*time.Millisecond {
		t.Fatalf("stream duration bound was not respected: %s", elapsed)
	}
	before, err := fixture.store.ListRunEvents(context.Background(), fixture.run.ID)
	if err != nil || len(before) != len(timeline) {
		t.Fatalf("read stream mutated the Run timeline: before=%d after=%d err=%v", len(timeline), len(before), err)
	}
}

func TestRunEventStreamObservesAnotherSQLiteConnection(t *testing.T) {
	fixture := newAPIFixture(t)
	fixture.api.eventStream = EventStreamConfig{
		PollInterval: 5 * time.Millisecond, HeartbeatInterval: 100 * time.Millisecond,
		MaxDuration: time.Second, WriteTimeout: 100 * time.Millisecond,
		BatchSize: 10, MaxEvents: 10,
	}
	timeline, err := fixture.store.ListRunEvents(context.Background(), fixture.run.ID)
	if err != nil || len(timeline) == 0 {
		t.Fatalf("load timeline: len=%d err=%v", len(timeline), err)
	}
	cursor := encodeEventStreamCursor(fixture.run.ID, timeline[len(timeline)-1].Sequence)

	server := httptest.NewServer(fixture.api)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		server.URL+"/api/v1/runs/"+fixture.run.ID+"/events/stream?cursor="+url.QueryEscape(cursor), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testAccessToken)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("stream status=%d body=%s", response.StatusCode, body)
	}

	writerStore, err := store.Open(fixture.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writerStore.Close()
	created, err := application.NewNoteService(writerStore).Create(context.Background(), application.CreateNoteRequest{
		RunID: fixture.run.ID, Title: "cross connection observation", Content: "visible through durable polling",
	})
	if err != nil {
		t.Fatal(err)
	}
	streamEvent := readOneSSEEvent(t, response.Body)
	if streamEvent.Data.Sequence != timeline[len(timeline)-1].Sequence+1 ||
		streamEvent.Data.Event.Type != "note.created" || streamEvent.Data.Event.SubjectID != created.ID {
		t.Fatalf("cross-connection event is inconsistent: %#v", streamEvent)
	}
}

func TestHTTPServerShutdownCancelsLongEventStreams(t *testing.T) {
	fixture := newAPIFixture(t)
	timeline, err := fixture.store.ListRunEvents(context.Background(), fixture.run.ID)
	if err != nil || len(timeline) == 0 {
		t.Fatalf("load timeline: len=%d err=%v", len(timeline), err)
	}
	fixture.api.eventStream = EventStreamConfig{
		PollInterval: 10 * time.Millisecond, HeartbeatInterval: 50 * time.Millisecond,
		MaxDuration: time.Minute, WriteTimeout: 100 * time.Millisecond,
		BatchSize: 10, MaxEvents: 10,
	}
	ctx, cancelServer := context.WithCancel(context.Background())
	listener, err := ListenLoopback(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(fixture.api, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, listener) }()

	cursor := encodeEventStreamCursor(fixture.run.ID, timeline[len(timeline)-1].Sequence)
	request, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+
		"/api/v1/runs/"+fixture.run.ID+"/events/stream?cursor="+url.QueryEscape(cursor), nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testAccessToken)
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		t.Fatalf("stream status=%d", response.StatusCode)
	}
	cancelServer()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server shutdown failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server shutdown waited for the event-stream duration limit")
	}
	_ = response.Body.Close()
}

func TestEventStreamWriteDeadlineBoundsSlowConsumers(t *testing.T) {
	underlying := &deadlineBlockingWriter{header: make(http.Header)}
	tracked := &responseWriter{ResponseWriter: underlying}
	started := time.Now()
	err := writeEventStreamFrame(tracked, 20*time.Millisecond, []byte("data: {}\n\n"))
	elapsed := time.Since(started)
	if !errors.Is(err, os.ErrDeadlineExceeded) || elapsed < 15*time.Millisecond || elapsed > 250*time.Millisecond {
		t.Fatalf("slow write was not bounded: elapsed=%s err=%v", elapsed, err)
	}
	if underlying.deadlineSets < 2 || !underlying.deadline.IsZero() {
		t.Fatalf("write deadline was not installed and cleared: %#v", underlying)
	}
	if err := writeEventStreamFrame(httptest.NewRecorder(), time.Second,
		make([]byte, MaxEventStreamFrameBytes+1)); apperror.CodeOf(apperror.Normalize(err)) != apperror.CodeResourceExhausted {
		t.Fatalf("oversized stream frame was not rejected: %v", err)
	}
}

func TestRunEventStreamRejectsConnectionsAboveTheProcessLimit(t *testing.T) {
	fixture := newAPIFixture(t)
	config, err := normalizeEventStreamConfig(EventStreamConfig{
		PollInterval: 10 * time.Millisecond, HeartbeatInterval: 50 * time.Millisecond,
		MaxDuration: time.Second, WriteTimeout: 100 * time.Millisecond,
		BatchSize: 2, MaxEvents: 10, MaxConnections: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.api.eventStream = config
	fixture.api.eventStreamSlots = make(chan struct{}, config.MaxConnections)
	timeline, err := fixture.store.ListRunEvents(context.Background(), fixture.run.ID)
	if err != nil || len(timeline) == 0 {
		t.Fatalf("load timeline: len=%d err=%v", len(timeline), err)
	}
	cursor := encodeEventStreamCursor(fixture.run.ID, timeline[len(timeline)-1].Sequence)

	server := httptest.NewServer(fixture.api)
	defer server.Close()
	firstContext, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstRequest, err := http.NewRequestWithContext(firstContext, http.MethodGet,
		server.URL+"/api/v1/runs/"+fixture.run.ID+"/events/stream?cursor="+url.QueryEscape(cursor), nil)
	if err != nil {
		t.Fatal(err)
	}
	firstRequest.Header.Set("Authorization", "Bearer "+testAccessToken)
	firstResponse, err := server.Client().Do(firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer firstResponse.Body.Close()

	secondRequest, err := http.NewRequest(http.MethodGet,
		server.URL+"/api/v1/runs/"+fixture.run.ID+"/events/stream?cursor="+url.QueryEscape(cursor), nil)
	if err != nil {
		t.Fatal(err)
	}
	secondRequest.Header.Set("Authorization", "Bearer "+testAccessToken)
	secondResponse, err := server.Client().Do(secondRequest)
	if err != nil {
		t.Fatal(err)
	}
	secondBody, readErr := io.ReadAll(secondResponse.Body)
	_ = secondResponse.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if secondResponse.StatusCode != http.StatusTooManyRequests ||
		!bytes.Contains(secondBody, []byte(`"code":"RESOURCE_EXHAUSTED"`)) ||
		strings.HasPrefix(secondResponse.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("connection limit response is invalid: status=%d headers=%#v body=%s",
			secondResponse.StatusCode, secondResponse.Header, secondBody)
	}
	cancelFirst()
	_ = firstResponse.Body.Close()
	deadline := time.Now().Add(500 * time.Millisecond)
	for len(fixture.api.eventStreamSlots) != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(fixture.api.eventStreamSlots) != 0 {
		t.Fatal("cancelled event stream did not release its process slot")
	}
}

func TestEventStreamConfigurationIsBounded(t *testing.T) {
	defaults, err := normalizeEventStreamConfig(EventStreamConfig{})
	if err != nil || defaults.PollInterval != DefaultEventStreamPollInterval ||
		defaults.MaxEvents != DefaultEventStreamMaxEvents || defaults.BatchSize != DefaultEventStreamBatchSize ||
		defaults.MaxConnections != DefaultEventStreamMaxConnections ||
		defaults.WriteTimeout != DefaultEventStreamWriteTimeout {
		t.Fatalf("event stream defaults are invalid: %#v err=%v", defaults, err)
	}
	tests := []EventStreamConfig{
		{PollInterval: -time.Second},
		{HeartbeatInterval: time.Hour},
		{MaxDuration: time.Nanosecond},
		{WriteTimeout: time.Minute},
		{BatchSize: MaxEventStreamBatchSize + 1},
		{MaxEvents: 100001},
		{MaxConnections: 129},
	}
	fixture := newAPIFixture(t)
	for _, config := range tests {
		if _, err := New(fixture.store, Config{AccessToken: testAccessToken, EventStream: config}); apperror.CodeOf(apperror.Normalize(err)) != apperror.CodeInvalidArgument {
			t.Fatalf("invalid event stream config was accepted: %#v err=%v", config, err)
		}
	}
}

func testEventStreamConfig(maxEvents int, maxDuration time.Duration) EventStreamConfig {
	return EventStreamConfig{
		PollInterval: 5 * time.Millisecond, HeartbeatInterval: 100 * time.Millisecond,
		MaxDuration: maxDuration, WriteTimeout: 50 * time.Millisecond,
		BatchSize: 2, MaxEvents: maxEvents,
	}
}

func (f *apiFixture) streamRequest(t *testing.T, query string, lastEventID string) *httptest.ResponseRecorder {
	t.Helper()
	values := []string(nil)
	if lastEventID != "" {
		values = []string{lastEventID}
	}
	return f.streamRequestValues(t, query, values)
}

func (f *apiFixture) streamRequestValues(t *testing.T, query string, lastEventIDs []string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet,
		"http://127.0.0.1/api/v1/runs/"+f.run.ID+"/events/stream"+query, nil)
	request.Host = "127.0.0.1:8765"
	request.RemoteAddr = "127.0.0.1:45000"
	request.Header.Set("Authorization", "Bearer "+testAccessToken)
	for _, value := range lastEventIDs {
		request.Header.Add("Last-Event-ID", value)
	}
	response := httptest.NewRecorder()
	f.api.ServeHTTP(response, request)
	return response
}

func parseSSEEvents(t *testing.T, raw []byte) []parsedSSEEvent {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024), MaxEventStreamFrameBytes)
	current := parsedSSEEvent{}
	out := make([]parsedSSEEvent, 0)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if len(current.Data.Event.Payload) != 0 {
				out = append(out, current)
			}
			current = parsedSSEEvent{}
		case strings.HasPrefix(line, "id: "):
			current.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			current.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &current.Data); err != nil {
				t.Fatalf("decode SSE data: %v line=%s", err, line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func readOneSSEEvent(t *testing.T, reader io.Reader) parsedSSEEvent {
	t.Helper()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), MaxEventStreamFrameBytes)
	current := parsedSSEEvent{}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if len(current.Data.Event.Payload) != 0 {
				return current
			}
			current = parsedSSEEvent{}
		case strings.HasPrefix(line, "id: "):
			current.ID = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			current.Event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &current.Data); err != nil {
				t.Fatalf("decode SSE data: %v", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("event stream closed before a durable event arrived")
	return parsedSSEEvent{}
}

func assertSSESequences(t *testing.T, events []parsedSSEEvent, expected ...int64) {
	t.Helper()
	if len(events) != len(expected) {
		t.Fatalf("event count=%d want=%d events=%#v", len(events), len(expected), events)
	}
	for index, sequence := range expected {
		if events[index].Event != "run.event" || events[index].Data.Version != RunEventStreamVersion ||
			events[index].Data.Sequence != sequence {
			t.Fatalf("event %d is inconsistent: %#v", index, events[index])
		}
	}
}

type deadlineBlockingWriter struct {
	mu           sync.Mutex
	header       http.Header
	deadline     time.Time
	deadlineSets int
}

func (w *deadlineBlockingWriter) Header() http.Header { return w.header }
func (w *deadlineBlockingWriter) WriteHeader(int)     {}
func (w *deadlineBlockingWriter) Flush()              {}

func (w *deadlineBlockingWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	w.deadline = deadline
	w.deadlineSets++
	w.mu.Unlock()
	return nil
}

func (w *deadlineBlockingWriter) Write([]byte) (int, error) {
	w.mu.Lock()
	deadline := w.deadline
	w.mu.Unlock()
	if deadline.IsZero() {
		return 0, errors.New("write deadline was not set")
	}
	timer := time.NewTimer(max(time.Until(deadline), time.Nanosecond))
	defer timer.Stop()
	<-timer.C
	return 0, os.ErrDeadlineExceeded
}
