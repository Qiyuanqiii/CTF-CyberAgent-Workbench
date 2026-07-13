package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
)

const testAccessToken = "api-test-token-0123456789-abcdefghijk"

const testControlToken = "api-control-token-0123456789-abcdefgh"

type apiTestEnvelope struct {
	Version   string          `json:"version"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
	Page      *Page           `json:"page,omitempty"`
	Error     *apiErrorView   `json:"error,omitempty"`
}

type apiFixture struct {
	api        *API
	store      *store.SQLiteStore
	dbPath     string
	run        domain.Run
	root       domain.AgentNode
	workItems  []domain.WorkItem
	notes      []domain.Note
	artifactID string
	secret     string
	leaseID    string
	checkpoint domain.SupervisorCheckpoint
	attempt    llm.ModelAttempt
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "http-api.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ctx := context.Background()
	runs := application.NewRunService(st)
	_, run, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "inspect durable agent state", Profile: "review", ModelRoute: "review",
		Budget: domain.Budget{MaxTurns: 8, MaxToolCalls: 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = runs.Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := st.RegisterRootAgent(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []session.Message{
		session.NewMessage(run.SessionID, "user", "inspect the project"),
		session.NewMessage(run.SessionID, "assistant", "inspection started"),
	} {
		if _, err := st.SaveSessionMessage(ctx, message); err != nil {
			t.Fatal(err)
		}
	}
	compacted := session.NewMessage(run.SessionID, "assistant", "old compacted context")
	compacted.Compacted = true
	if _, err := st.SaveSessionMessage(ctx, compacted); err != nil {
		t.Fatal(err)
	}

	workService := application.NewWorkItemService(st)
	workItems := make([]domain.WorkItem, 0, 3)
	for index := 1; index <= 3; index++ {
		item, err := workService.Create(ctx, application.CreateWorkItemRequest{
			RunID: run.ID, Title: fmt.Sprintf("API work item %d", index), Owner: "root",
			OwnerAgentID:       root.ID,
			AcceptanceCriteria: []string{fmt.Sprintf("criterion %d", index)},
		})
		if err != nil {
			t.Fatal(err)
		}
		workItems = append(workItems, item)
	}

	secret := "sk-" + strings.Repeat("q", 30)
	noteService := application.NewNoteService(st)
	notes := make([]domain.Note, 0, 3)
	for index := 1; index <= 3; index++ {
		content := fmt.Sprintf("durable observation %d", index)
		if index == 1 {
			content += " token=" + secret
		}
		note, err := noteService.Create(ctx, application.CreateNoteRequest{
			RunID: run.ID, Title: fmt.Sprintf("API note %d", index), Content: content,
			OwnerAgentID: root.ID, Tags: []string{"api", fmt.Sprintf("page-%d", index)}, Pinned: index == 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		notes = append(notes, note)
	}

	gateway := toolgateway.New(st, policy.NewDefaultChecker())
	proposed, err := gateway.Invoke(ctx, toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "echo api evidence"},
		RunID: run.ID, SessionID: run.SessionID, RequestedBy: "http_api_test",
	})
	if err != nil || proposed.Proposal == nil {
		t.Fatalf("artifact proposal failed: %#v err=%v", proposed, err)
	}
	reviewed, err := gateway.Review(ctx, toolgateway.ReviewRequest{
		Action: toolgateway.ReviewApprove, Tool: toolgateway.ShellTool,
		ProposalID: proposed.Proposal.ID, ReviewedBy: "http_api_test",
	})
	if err != nil || reviewed.Result == nil || reviewed.Result.Metadata["artifact_stdout_id"] == "" {
		t.Fatalf("artifact capture failed: %#v err=%v", reviewed, err)
	}
	acquiredLease, err := st.AcquireRunExecutionLease(ctx, domain.AcquireRunExecutionLeaseRequest{
		RunID: run.ID, OwnerID: "http-api-test-worker", TTL: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := st.BeginSupervisorTurn(ctx, acquiredLease.Lease, "pending input is deliberately private")
	if err != nil {
		t.Fatal(err)
	}
	attempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3, Provider: "http-api-test", Model: "test-model",
	}
	if inserted, err := st.RecordSupervisorModelStarted(ctx, turn.Checkpoint, attempt); err != nil || !inserted {
		t.Fatalf("fixture model start inserted=%t err=%v", inserted, err)
	}
	api, err := New(st, Config{
		AccessToken: testAccessToken, ControlToken: testControlToken, AppVersion: "test-version",
	})
	if err != nil {
		t.Fatal(err)
	}
	return &apiFixture{api: api, store: st, dbPath: dbPath, run: run, root: root,
		workItems: workItems, notes: notes,
		artifactID: reviewed.Result.Metadata["artifact_stdout_id"], secret: secret,
		leaseID: acquiredLease.Lease.LeaseID, checkpoint: turn.Checkpoint, attempt: attempt}
}

func TestReadAPIExposesDurableStateWithoutArtifactContentOrCheckpointInput(t *testing.T) {
	fixture := newAPIFixture(t)

	health := fixture.get(t, "/api/v1/health")
	if health.Code != http.StatusOK {
		t.Fatalf("health status=%d body=%s", health.Code, health.Body.String())
	}
	assertSecurityHeaders(t, health)
	var healthData struct {
		Status        string `json:"status"`
		SchemaVersion int    `json:"schema_version"`
	}
	decodeData(t, health, &healthData)
	if healthData.Status != "ok" || healthData.SchemaVersion != store.LatestSchemaVersion {
		t.Fatalf("unexpected health data: %#v", healthData)
	}

	runs := fixture.get(t, "/api/v1/runs?limit=100&status=running")
	var runViews []RunView
	decodeData(t, runs, &runViews)
	if len(runViews) != 1 || runViews[0].ID != fixture.run.ID {
		t.Fatalf("unexpected Run list: %#v", runViews)
	}
	runDetailResponse := fixture.get(t, "/api/v1/runs/"+fixture.run.ID)
	if strings.Contains(runDetailResponse.Body.String(), "pending input is deliberately private") {
		t.Fatal("Run detail exposed the durable pending model input")
	}
	var runDetail RunDetailView
	decodeData(t, runDetailResponse, &runDetail)
	if runDetail.Run.ID != fixture.run.ID || runDetail.Mission.Goal == "" || runDetail.Checkpoint == nil ||
		runDetail.Checkpoint.Phase != string(domain.SupervisorTurnStarted) || runDetail.ToolUsage.Consumed != 1 ||
		runDetail.Lease == nil || !runDetail.Lease.Active || runDetail.Lease.Generation != 1 ||
		runDetail.Lease.OwnerID != "http-api-test-worker" {
		t.Fatalf("unexpected Run detail: %#v", runDetail)
	}
	if strings.Contains(runDetailResponse.Body.String(), `"lease_id"`) {
		t.Fatal("Run detail exposed the execution fencing token")
	}

	graphResponse := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+"/agent-graph")
	var graph AgentGraphView
	decodeData(t, graphResponse, &graph)
	if graph.ProtocolVersion != domain.AgentGraphProtocolVersion || graph.RunID != fixture.run.ID ||
		graph.RootAgentID != fixture.root.ID || len(graph.Nodes) != 1 ||
		graph.Nodes[0].ID != fixture.root.ID || graph.Nodes[0].Role != string(domain.AgentRoleRoot) {
		t.Fatalf("unexpected Agent graph: %#v", graph)
	}
	for _, endpoint := range []string{"delegations", "fanout-plans", "reports"} {
		response := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+"/"+endpoint+"?limit=10")
		var values []json.RawMessage
		decodeData(t, response, &values)
		if len(values) != 0 {
			t.Fatalf("empty projection %s returned %#v", endpoint, values)
		}
		var envelope apiTestEnvelope
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil ||
			envelope.Page == nil || envelope.Page.Limit != 10 {
			t.Fatalf("projection %s omitted pagination: body=%s err=%v", endpoint, response.Body.String(), err)
		}
	}

	sessions := fixture.get(t, "/api/v1/sessions")
	var sessionViews []SessionView
	decodeData(t, sessions, &sessionViews)
	if len(sessionViews) != 1 || sessionViews[0].ID != fixture.run.SessionID {
		t.Fatalf("unexpected Session list: %#v", sessionViews)
	}
	var sessionDetail SessionDetailView
	decodeData(t, fixture.get(t, "/api/v1/sessions/"+fixture.run.SessionID), &sessionDetail)
	if sessionDetail.Run == nil || sessionDetail.Run.ID != fixture.run.ID {
		t.Fatalf("Session was not projected onto its Run: %#v", sessionDetail)
	}
	var messages []MessageView
	decodeData(t, fixture.get(t, "/api/v1/sessions/"+fixture.run.SessionID+"/messages"), &messages)
	if len(messages) != 2 {
		t.Fatalf("default message view included compacted history: %#v", messages)
	}
	decodeData(t, fixture.get(t,
		"/api/v1/sessions/"+fixture.run.SessionID+"/messages?include_compacted=true"), &messages)
	if len(messages) != 3 {
		t.Fatalf("compacted message view is incomplete: %#v", messages)
	}

	workResponse := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+"/work-items?owner=root")
	var workItems []WorkItemView
	decodeData(t, workResponse, &workItems)
	if len(workItems) != 3 || workItems[0].AcceptanceCriteria == nil || workItems[0].Dependencies == nil {
		t.Fatalf("unexpected WorkItem list: %#v", workItems)
	}
	var workItem WorkItemView
	decodeData(t, fixture.get(t, "/api/v1/work-items/"+fixture.workItems[0].ID), &workItem)
	if workItem.ID != fixture.workItems[0].ID {
		t.Fatalf("unexpected WorkItem detail: %#v", workItem)
	}
	agentWorkResponse := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+
		"/work-items?owner_agent_id="+url.QueryEscape(fixture.root.ID))
	var agentWorkItems []WorkItemView
	decodeData(t, agentWorkResponse, &agentWorkItems)
	if len(agentWorkItems) != 3 || agentWorkItems[0].OwnerAgentID != fixture.root.ID ||
		workItem.OwnerAgentID != fixture.root.ID {
		t.Fatalf("WorkItem Agent ownership is missing from API: list=%#v detail=%#v", agentWorkItems, workItem)
	}

	noteResponse := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+"/notes?tag=api")
	if strings.Contains(noteResponse.Body.String(), fixture.secret) || !strings.Contains(noteResponse.Body.String(), "[REDACTED:") {
		t.Fatalf("Note response was not redacted: %s", noteResponse.Body.String())
	}
	var notes []NoteView
	decodeData(t, noteResponse, &notes)
	if len(notes) != 3 || notes[0].Tags == nil || notes[0].SourceRefs == nil || notes[0].EvidenceIDs == nil {
		t.Fatalf("unexpected Note list: %#v", notes)
	}
	var note NoteView
	decodeData(t, fixture.get(t, "/api/v1/notes/"+fixture.notes[0].ID), &note)
	if note.ID != fixture.notes[0].ID {
		t.Fatalf("unexpected Note detail: %#v", note)
	}
	agentNoteResponse := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+
		"/notes?owner_agent_id="+url.QueryEscape(fixture.root.ID))
	var agentNotes []NoteView
	decodeData(t, agentNoteResponse, &agentNotes)
	if len(agentNotes) != 3 || agentNotes[0].OwnerAgentID != fixture.root.ID ||
		note.OwnerAgentID != fixture.root.ID {
		t.Fatalf("Note Agent ownership is missing from API: list=%#v detail=%#v", agentNotes, note)
	}

	artifactResponse := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+"/artifacts?stream=stdout")
	if strings.Contains(artifactResponse.Body.String(), "dry run: echo api evidence") ||
		strings.Contains(artifactResponse.Body.String(), `"content"`) {
		t.Fatalf("Artifact metadata endpoint exposed content: %s", artifactResponse.Body.String())
	}
	var artifacts []ArtifactView
	decodeData(t, artifactResponse, &artifacts)
	if len(artifacts) != 1 || artifacts[0].ID != fixture.artifactID || artifacts[0].SHA256 == "" {
		t.Fatalf("unexpected Artifact metadata: %#v", artifacts)
	}
	var artifact ArtifactView
	decodeData(t, fixture.get(t, "/api/v1/artifacts/"+fixture.artifactID), &artifact)
	if artifact.ID != fixture.artifactID {
		t.Fatalf("unexpected Artifact detail: %#v", artifact)
	}

	eventResponse := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+"/events?limit=100")
	if strings.Contains(eventResponse.Body.String(), fixture.leaseID) ||
		strings.Contains(eventResponse.Body.String(), `"lease_id"`) {
		t.Fatal("Run timeline exposed the execution fencing token")
	}
	var eventViews []EventView
	decodeData(t, eventResponse, &eventViews)
	if len(eventViews) < 10 {
		t.Fatalf("Run timeline is incomplete: %#v", eventViews)
	}
	for _, event := range eventViews {
		if !json.Valid(event.Payload) {
			t.Fatalf("event %s has invalid payload: %s", event.EventID, event.Payload)
		}
	}
	var rounds []SupervisorToolRoundView
	decodeData(t, fixture.get(t, "/api/v1/runs/"+fixture.run.ID+"/tool-rounds"), &rounds)
	if len(rounds) != 0 || rounds == nil {
		t.Fatalf("unexpected empty tool round view: %#v", rounds)
	}
	missing := fixture.get(t, "/api/v1/runs/missing/events")
	assertAPIError(t, missing, http.StatusNotFound, "NOT_FOUND")
}

func TestReadAPIPaginationCursorIsOpaqueScopedAndBounded(t *testing.T) {
	fixture := newAPIFixture(t)
	first := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+"/notes?limit=2")
	var firstNotes []NoteView
	firstEnvelope := decodeData(t, first, &firstNotes)
	if len(firstNotes) != 2 || firstEnvelope.Page == nil || firstEnvelope.Page.NextCursor == "" {
		t.Fatalf("first page is incomplete: notes=%#v page=%#v", firstNotes, firstEnvelope.Page)
	}
	second := fixture.get(t, "/api/v1/runs/"+fixture.run.ID+"/notes?limit=2&cursor="+
		url.QueryEscape(firstEnvelope.Page.NextCursor))
	var secondNotes []NoteView
	secondEnvelope := decodeData(t, second, &secondNotes)
	if len(secondNotes) != 1 || secondEnvelope.Page == nil || secondEnvelope.Page.NextCursor != "" ||
		secondNotes[0].ID == firstNotes[0].ID || secondNotes[0].ID == firstNotes[1].ID {
		t.Fatalf("second page is inconsistent: notes=%#v page=%#v", secondNotes, secondEnvelope.Page)
	}

	badCursorPaths := []string{
		"/api/v1/runs/" + fixture.run.ID + "/notes?limit=2&tag=api&cursor=" +
			url.QueryEscape(firstEnvelope.Page.NextCursor),
		"/api/v1/runs/" + fixture.run.ID + "/events?limit=2&cursor=" +
			url.QueryEscape(firstEnvelope.Page.NextCursor),
		"/api/v1/runs/" + fixture.run.ID + "/notes?cursor=not-a-cursor",
	}
	for _, requestPath := range badCursorPaths {
		assertAPIError(t, fixture.get(t, requestPath), http.StatusBadRequest, "INVALID_ARGUMENT")
	}

	invalidQueries := []string{
		"?limit=0", "?limit=101", "?limit=", "?limit=2&limit=3", "?unknown=true",
	}
	for _, query := range invalidQueries {
		assertAPIError(t, fixture.get(t, "/api/v1/sessions"+query), http.StatusBadRequest, "INVALID_ARGUMENT")
	}
}

func TestPageMarksTheBoundedCursorWindowAsTruncated(t *testing.T) {
	items, page := trimPage([]int{1, 2}, pageRequest{
		Limit: 1, Offset: maxStoreCursorOffset, Scope: "test-scope",
	})
	if len(items) != 1 || page == nil || page.NextCursor != "" || !page.Truncated {
		t.Fatalf("cursor window did not report its hard boundary: items=%#v page=%#v", items, page)
	}
}

func TestReadAPISecurityBoundaryAndInternalErrorRedaction(t *testing.T) {
	fixture := newAPIFixture(t)
	tests := []struct {
		name      string
		method    string
		token     string
		host      string
		remote    string
		body      io.Reader
		status    int
		errorCode string
	}{
		{name: "missing token", method: http.MethodGet, host: "127.0.0.1:8765", remote: "127.0.0.1:45000",
			status: http.StatusUnauthorized, errorCode: "POLICY_DENIED"},
		{name: "wrong token", method: http.MethodGet, token: strings.Repeat("x", 32), host: "127.0.0.1:8765",
			remote: "127.0.0.1:45000", status: http.StatusUnauthorized, errorCode: "POLICY_DENIED"},
		{name: "external Host", method: http.MethodGet, token: testAccessToken, host: "agent.example:8765",
			remote: "127.0.0.1:45000", status: http.StatusForbidden, errorCode: "POLICY_DENIED"},
		{name: "external client", method: http.MethodGet, token: testAccessToken, host: "127.0.0.1:8765",
			remote: "192.0.2.20:45000", status: http.StatusForbidden, errorCode: "POLICY_DENIED"},
		{name: "write method", method: http.MethodPost, token: testAccessToken, host: "127.0.0.1:8765",
			remote: "127.0.0.1:45000", status: http.StatusMethodNotAllowed, errorCode: "INVALID_ARGUMENT"},
		{name: "GET body", method: http.MethodGet, token: testAccessToken, host: "127.0.0.1:8765",
			remote: "127.0.0.1:45000", body: strings.NewReader("unexpected"),
			status: http.StatusBadRequest, errorCode: "INVALID_ARGUMENT"},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			response := fixture.request(t, current.method, "/api/v1/health", current.token,
				current.host, current.remote, current.body)
			assertAPIError(t, response, current.status, current.errorCode)
			assertSecurityHeaders(t, response)
			if current.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("missing Allow header: %#v", response.Header())
			}
		})
	}

	nonCanonical := fixture.request(t, http.MethodGet, "/api/v1//health", testAccessToken,
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
	assertAPIError(t, nonCanonical, http.StatusBadRequest, "INVALID_ARGUMENT")
	oversizedRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/health", nil)
	oversizedRequest.Host = "127.0.0.1:8765"
	oversizedRequest.RemoteAddr = "127.0.0.1:45000"
	oversizedRequest.Header.Set("Authorization", "Bearer "+testAccessToken)
	oversizedRequest.RequestURI = "/api/v1/health?x=" + strings.Repeat("a", MaxRequestTargetBytes)
	oversized := httptest.NewRecorder()
	fixture.api.ServeHTTP(oversized, oversizedRequest)
	assertAPIError(t, oversized, http.StatusRequestURITooLong, "RESOURCE_EXHAUSTED")

	closedStore, err := store.Open(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	closedAPI, err := New(closedStore, Config{AccessToken: testAccessToken})
	if err != nil {
		t.Fatal(err)
	}
	if err := closedStore.Close(); err != nil {
		t.Fatal(err)
	}
	closedResponse := performRequest(t, closedAPI, http.MethodGet, "/api/v1/health", testAccessToken,
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
	assertAPIError(t, closedResponse, http.StatusInternalServerError, "INTERNAL")
	if strings.Contains(strings.ToLower(closedResponse.Body.String()), "closed") ||
		strings.Contains(strings.ToLower(closedResponse.Body.String()), "sql") {
		t.Fatalf("internal Store error leaked through API: %s", closedResponse.Body.String())
	}
}

func TestReadAPIHandlesConcurrentSQLiteReaders(t *testing.T) {
	fixture := newAPIFixture(t)
	const readers = 32
	start := make(chan struct{})
	errorsFound := make(chan error, readers)
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(readers)
	done.Add(readers)
	for index := 0; index < readers; index++ {
		index := index
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			path := "/api/v1/health"
			if index%2 == 0 {
				path = "/api/v1/runs/" + fixture.run.ID + "/events?limit=5"
			}
			response := performRequest(t, fixture.api, http.MethodGet, path, testAccessToken,
				"127.0.0.1:8765", "127.0.0.1:45000", nil)
			if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) {
				errorsFound <- fmt.Errorf("reader %d: status=%d body=%s", index, response.Code, response.Body.String())
			}
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
}

func TestHTTPServerLifecycleAndLoopbackValidation(t *testing.T) {
	fixture := newAPIFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
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
	request, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+"/api/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testAccessToken)
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || !json.Valid(body) {
		t.Fatalf("network API response is invalid: status=%d body=%s err=%v", response.StatusCode, body, readErr)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graceful shutdown failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP API did not stop after context cancellation")
	}

	for _, address := range []string{"0.0.0.0:0", ":0", "agent.example:0", "127.0.0.1:-1", "127.0.0.1"} {
		if listener, err := ListenLoopback(context.Background(), address); err == nil {
			_ = listener.Close()
			t.Fatalf("unsafe or invalid listener address was accepted: %q", address)
		}
	}
	cancelled, cancelImmediately := context.WithCancel(context.Background())
	cancelImmediately()
	if listener, err := ListenLoopback(cancelled, "127.0.0.1:0"); listener != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled listener did not preserve context error: listener=%v err=%v", listener, err)
	}

	generated, err := GenerateAccessToken()
	if err != nil || len(generated) < MinAccessTokenBytes {
		t.Fatalf("generated token is invalid: len=%d err=%v", len(generated), err)
	}
	for _, token := range []string{"short", strings.Repeat("x", MaxAccessTokenBytes+1),
		" " + strings.Repeat("x", MinAccessTokenBytes), strings.Repeat("x", MinAccessTokenBytes-1) + "\n"} {
		if _, err := New(fixture.store, Config{AccessToken: token}); err == nil {
			t.Fatalf("invalid API token was accepted: %q", token)
		}
	}
}

func (f *apiFixture) get(t *testing.T, requestPath string) *httptest.ResponseRecorder {
	t.Helper()
	return f.request(t, http.MethodGet, requestPath, testAccessToken,
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
}

func (f *apiFixture) request(t *testing.T, method string, requestPath string, token string,
	host string, remote string, body io.Reader,
) *httptest.ResponseRecorder {
	t.Helper()
	return performRequest(t, f.api, method, requestPath, token, host, remote, body)
}

func performRequest(t *testing.T, api *API, method string, requestPath string, token string,
	host string, remote string, body io.Reader,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, "http://127.0.0.1"+requestPath, body)
	request.Host = host
	request.RemoteAddr = remote
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}

func decodeData[T any](t *testing.T, response *httptest.ResponseRecorder, target *T) apiTestEnvelope {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("API status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope apiTestEnvelope
	decoder := json.NewDecoder(bytes.NewReader(response.Body.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		t.Fatalf("decode API envelope: %v body=%s", err, response.Body.String())
	}
	if envelope.Version != Version || envelope.RequestID == "" || envelope.Error != nil {
		t.Fatalf("invalid success envelope: %#v", envelope)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatalf("decode API data: %v data=%s", err, envelope.Data)
	}
	return envelope
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("API status=%d, want %d body=%s", response.Code, status, response.Body.String())
	}
	var envelope apiTestEnvelope
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode API error: %v body=%s", err, response.Body.String())
	}
	if envelope.Version != Version || envelope.RequestID == "" || envelope.Error == nil ||
		envelope.Error.Code != code || envelope.Error.Message == "" {
		t.Fatalf("invalid error envelope: %#v", envelope)
	}
}

func assertSecurityHeaders(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	header := response.Header()
	if header.Get("Cache-Control") != "no-store" || header.Get("X-Content-Type-Options") != "nosniff" ||
		header.Get("X-Frame-Options") != "DENY" || header.Get("X-CyberAgent-API-Version") != Version ||
		header.Get("X-Request-ID") == "" || header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("HTTP security headers are incomplete: %#v", header)
	}
}
