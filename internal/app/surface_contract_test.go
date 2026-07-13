package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cyberagent-workbench/internal/headless"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/tui"
)

func TestSameRunStateAcrossCLI_TUI_HTTPAndHeadless(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	t.Setenv("MIMO_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("CYBERAGENT_ANTHROPIC_API_KEY", "")
	created, stderr, code := executeTestCommand(t, "run", "create",
		"read surface state contract", "--profile", "code")
	if code != 0 || stderr != "" {
		t.Fatalf("create contract Run: code=%d stderr=%s stdout=%s", code, stderr, created)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("contract Run id missing: %s", created)
	}
	if _, stderr, code = executeTestCommand(t, "run", "start", runID); code != 0 {
		t.Fatalf("start contract Run: code=%d stderr=%s", code, stderr)
	}
	cliShow, stderr, code := executeTestCommand(t, "run", "show", runID)
	if code != 0 || stderr != "" || !strings.Contains(cliShow, "status: running\n") {
		t.Fatalf("CLI Run projection drifted: code=%d stderr=%s stdout=%s", code, stderr, cliShow)
	}
	cliEvents, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || stderr != "" {
		t.Fatalf("CLI events projection failed: code=%d stderr=%s", code, stderr)
	}
	cliTail := lastCLIEventSequence(t, cliEvents)

	st, err := store.Open(filepath.Join(home, "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	run, err := st.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.GetSession(context.Background(), run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	tuiModel, err := tui.NewModel(context.Background(), sess, sessionManager,
		toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns(), st)
	if err != nil {
		t.Fatal(err)
	}
	tuiProjection, found := tuiModel.CurrentRunProjection()
	if !found {
		t.Fatal("TUI omitted the contract Run")
	}

	const accessToken = "surface-contract-read-token-0123456789"
	api, err := httpapi.New(st, httpapi.Config{AccessToken: accessToken, AppVersion: Version})
	if err != nil {
		t.Fatal(err)
	}
	var detailEnvelope struct {
		Data httpapi.RunDetailView `json:"data"`
	}
	readHTTPContract(t, api, accessToken, "/api/v1/runs/"+runID, &detailEnvelope)
	var eventEnvelope struct {
		Data []httpapi.EventView `json:"data"`
	}
	readHTTPContract(t, api, accessToken, "/api/v1/runs/"+runID+"/events?limit=100",
		&eventEnvelope)
	if len(eventEnvelope.Data) == 0 {
		t.Fatal("HTTP event projection is empty")
	}
	var agentEnvelope struct {
		Data httpapi.AgentGraphView `json:"data"`
	}
	readHTTPContract(t, api, accessToken, "/api/v1/runs/"+runID+"/agent-graph",
		&agentEnvelope)

	var headlessOutput bytes.Buffer
	if err := headless.NewExporter(st).Export(context.Background(), &headlessOutput,
		headless.Request{RunID: runID, MaxEvents: 100}); err != nil {
		t.Fatal(err)
	}
	headlessRecords := decodeHeadlessCLIRecords(t, headlessOutput.String())
	headlessEnd := headlessRecords[len(headlessRecords)-1]
	httpTail := eventEnvelope.Data[len(eventEnvelope.Data)-1].Sequence
	if string(tuiProjection.Status) != detailEnvelope.Data.Run.Status ||
		detailEnvelope.Data.Run.Status != headlessEnd.Status ||
		tuiProjection.EventSequence != cliTail || cliTail != httpTail ||
		httpTail != headlessEnd.LastSequence ||
		tuiProjection.RunID != detailEnvelope.Data.Run.ID ||
		tuiProjection.MissionID != detailEnvelope.Data.Run.MissionID ||
		tuiProjection.SessionID != detailEnvelope.Data.Run.SessionID ||
		tuiProjection.AgentCount != len(agentEnvelope.Data.Nodes) {
		t.Fatalf("read surfaces disagree: cli_status=%q cli_tail=%d tui=%#v http=%#v http_tail=%d headless=%#v agents=%d",
			"running", cliTail, tuiProjection, detailEnvelope.Data.Run, httpTail,
			headlessEnd, len(agentEnvelope.Data.Nodes))
	}
}

func readHTTPContract(t *testing.T, api http.Handler, token string, path string,
	target any,
) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+path, nil)
	request.RemoteAddr = "127.0.0.1:43210"
	request.Host = "127.0.0.1"
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("HTTP contract request %s failed: status=%d body=%s",
			path, response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode HTTP contract %s: %v body=%s", path, err, response.Body.String())
	}
}

func lastCLIEventSequence(t *testing.T, output string) int64 {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		t.Fatal("CLI event output is empty")
	}
	firstField := strings.SplitN(lines[len(lines)-1], "\t", 2)[0]
	sequence, err := strconv.ParseInt(strings.TrimPrefix(strings.TrimSpace(firstField), "#"), 10, 64)
	if err != nil || sequence <= 0 {
		t.Fatalf("parse CLI event tail %q: %v", lines[len(lines)-1], err)
	}
	return sequence
}
