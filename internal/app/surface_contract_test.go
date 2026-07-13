package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/headless"
	"cyberagent-workbench/internal/httpapi"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/tui"
)

const surfaceContractToken = "surface-contract-read-token-0123456789"

type surfaceLifecycleCase struct {
	name           string
	status         domain.RunStatus
	runID          string
	terminal       bool
	headlessReason string
	exitCode       int
}

type httpContractEnvelope[T any] struct {
	Data T             `json:"data"`
	Page *httpapi.Page `json:"page,omitempty"`
}

func TestReadSurfacesAgreeOnRunLifecycleAndPagination(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CYBERAGENT_HOME", home)
	t.Setenv("MIMO_API_KEY", "")
	t.Setenv("DEEPSEEK_API_KEY", "")
	t.Setenv("CYBERAGENT_ANTHROPIC_API_KEY", "")

	cases := []surfaceLifecycleCase{
		{name: "running", status: domain.RunRunning, headlessReason: "snapshot"},
		{name: "paused", status: domain.RunPaused, headlessReason: "snapshot"},
		{name: "completed", status: domain.RunCompleted, terminal: true,
			headlessReason: "terminal"},
		{name: "failed", status: domain.RunFailed, terminal: true,
			headlessReason: "terminal", exitCode: 4},
		{name: "cancelled", status: domain.RunCancelled, terminal: true,
			headlessReason: "terminal", exitCode: 7},
	}
	for index := range cases {
		cases[index].runID = createSurfaceLifecycleRun(t, cases[index].name, cases[index].status)
	}

	st, err := store.Open(filepath.Join(home, "cyberagent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	service := application.NewRunService(st)
	for index := len(cases); index < 53; index++ {
		if _, _, err := service.Create(ctx, application.CreateRunRequest{
			Goal: fmt.Sprintf("surface pagination filler %02d", index), Profile: "code",
		}); err != nil {
			t.Fatalf("create pagination filler %d: %v", index, err)
		}
	}

	sessionManager := session.NewManager(st, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	toolManager := toolgateway.New(st, policy.NewDefaultChecker()).ToolRuns()
	api, err := httpapi.New(st, httpapi.Config{
		AccessToken: surfaceContractToken, AppVersion: Version,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("lifecycle matrix", func(t *testing.T) {
		for _, testCase := range cases {
			t.Run(testCase.name, func(t *testing.T) {
				assertLifecycleSurfaceContract(t, st, sessionManager, toolManager, api, testCase)
			})
		}
	})

	t.Run("bounded list pagination", func(t *testing.T) {
		assertListPaginationContract(t, st, sessionManager, toolManager, api)
	})

	t.Run("event cursor and sequence resume", func(t *testing.T) {
		var pausedRunID string
		for _, testCase := range cases {
			if testCase.status == domain.RunPaused {
				pausedRunID = testCase.runID
				break
			}
		}
		assertEventPaginationContract(t, st, api, pausedRunID)
	})
}

func createSurfaceLifecycleRun(t *testing.T, name string, status domain.RunStatus) string {
	t.Helper()
	created, stderr, code := executeTestCommand(t, "run", "create",
		"read surface "+name+" contract", "--profile", "code")
	if code != 0 || stderr != "" {
		t.Fatalf("create %s contract Run: code=%d stderr=%s stdout=%s",
			name, code, stderr, created)
	}
	runID := runIDPattern.FindString(created)
	if runID == "" {
		t.Fatalf("%s contract Run id missing: %s", name, created)
	}
	if _, stderr, code = executeTestCommand(t, "run", "start", runID); code != 0 || stderr != "" {
		t.Fatalf("start %s contract Run: code=%d stderr=%s", name, code, stderr)
	}
	var transition []string
	switch status {
	case domain.RunRunning:
		return runID
	case domain.RunPaused:
		transition = []string{"run", "pause", runID}
	case domain.RunCompleted:
		transition = []string{"run", "finish", runID, "--summary", "surface contract complete"}
	case domain.RunFailed:
		transition = []string{"run", "fail", runID, "--reason", "surface contract failure"}
	case domain.RunCancelled:
		transition = []string{"run", "cancel", runID}
	default:
		t.Fatalf("unsupported surface lifecycle status %q", status)
	}
	if _, stderr, code = executeTestCommand(t, transition...); code != 0 || stderr != "" {
		t.Fatalf("transition %s contract Run: code=%d stderr=%s", name, code, stderr)
	}
	return runID
}

func assertLifecycleSurfaceContract(t *testing.T, st *store.SQLiteStore,
	sessionManager *session.Manager, toolManager tui.ToolManager, api http.Handler,
	testCase surfaceLifecycleCase,
) {
	t.Helper()
	cliShow, stderr, code := executeTestCommand(t, "run", "show", testCase.runID)
	if code != 0 || stderr != "" ||
		!strings.Contains(cliShow, "status: "+string(testCase.status)+"\n") {
		t.Fatalf("CLI Run projection drifted: code=%d stderr=%s stdout=%s",
			code, stderr, cliShow)
	}
	cliEvents, stderr, code := executeTestCommand(t, "run", "events", testCase.runID)
	if code != 0 || stderr != "" {
		t.Fatalf("CLI events projection failed: code=%d stderr=%s", code, stderr)
	}
	cliSequences := cliEventSequences(t, cliEvents)

	ctx := context.Background()
	run, err := st.GetRun(ctx, testCase.runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != testCase.status || run.Terminal() != testCase.terminal {
		t.Fatalf("Store lifecycle drifted: %#v", run)
	}
	sess, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	tuiModel, err := tui.NewModel(ctx, sess, sessionManager, toolManager, st)
	if err != nil {
		t.Fatal(err)
	}
	tuiProjection, found := tuiModel.CurrentRunProjection()
	if !found {
		t.Fatal("TUI omitted the contract Run")
	}

	var detailEnvelope httpContractEnvelope[httpapi.RunDetailView]
	readHTTPContract(t, api, surfaceContractToken, "/api/v1/runs/"+testCase.runID,
		&detailEnvelope)
	var eventEnvelope httpContractEnvelope[[]httpapi.EventView]
	readHTTPContract(t, api, surfaceContractToken,
		"/api/v1/runs/"+testCase.runID+"/events?limit=100", &eventEnvelope)
	if len(eventEnvelope.Data) == 0 || eventEnvelope.Page == nil ||
		eventEnvelope.Page.NextCursor != "" || eventEnvelope.Page.Truncated {
		t.Fatalf("HTTP event projection is incomplete: %#v", eventEnvelope.Page)
	}
	var agentEnvelope httpContractEnvelope[httpapi.AgentGraphView]
	readHTTPContract(t, api, surfaceContractToken,
		"/api/v1/runs/"+testCase.runID+"/agent-graph", &agentEnvelope)

	var headlessOutput bytes.Buffer
	exportErr := headless.NewExporter(st).Export(ctx, &headlessOutput,
		headless.Request{RunID: testCase.runID, MaxEvents: 100})
	exportExit := 0
	if exportErr != nil {
		exportExit = apperror.ExitCode(exportErr)
	}
	if exportExit != testCase.exitCode {
		t.Fatalf("headless exit drifted: got=%d want=%d err=%v",
			exportExit, testCase.exitCode, exportErr)
	}
	headlessRecords := decodeHeadlessCLIRecords(t, headlessOutput.String())
	if len(headlessRecords) < 2 {
		t.Fatalf("headless omitted lifecycle records: %#v", headlessRecords)
	}
	headlessEnd := headlessRecords[len(headlessRecords)-1]
	if headlessEnd.Kind != headless.EndRecordKind ||
		headlessEnd.Status != string(testCase.status) ||
		headlessEnd.Terminal != testCase.terminal ||
		headlessEnd.Reason != testCase.headlessReason ||
		headlessEnd.ExitCode != testCase.exitCode ||
		headlessEnd.HasMore || headlessEnd.Truncated ||
		headlessEnd.EventsEmitted != len(headlessRecords)-1 {
		t.Fatalf("headless lifecycle end record drifted: %#v", headlessEnd)
	}

	httpSequences := make([]int64, len(eventEnvelope.Data))
	for index, event := range eventEnvelope.Data {
		httpSequences[index] = event.Sequence
	}
	headlessSequences := make([]int64, len(headlessRecords)-1)
	for index, record := range headlessRecords[:len(headlessRecords)-1] {
		if record.Kind != headless.EventRecordKind || record.Version != headless.ProtocolVersion {
			t.Fatalf("headless event record drifted: %#v", record)
		}
		headlessSequences[index] = record.Sequence
	}
	cliTail := cliSequences[len(cliSequences)-1]
	if !slices.Equal(cliSequences, httpSequences) ||
		!slices.Equal(cliSequences, headlessSequences) ||
		string(tuiProjection.Status) != detailEnvelope.Data.Run.Status ||
		string(tuiProjection.Surface) != detailEnvelope.Data.Mode.Surface ||
		string(tuiProjection.Phase) != detailEnvelope.Data.Mode.Phase ||
		tuiProjection.ModeRevision != detailEnvelope.Data.Mode.Revision ||
		detailEnvelope.Data.Run.Status != headlessEnd.Status ||
		tuiProjection.EventSequence != cliTail ||
		cliTail != headlessEnd.LastSequence ||
		headlessEnd.SuggestedResume != cliTail ||
		tuiProjection.RunID != detailEnvelope.Data.Run.ID ||
		tuiProjection.MissionID != detailEnvelope.Data.Run.MissionID ||
		tuiProjection.SessionID != detailEnvelope.Data.Run.SessionID ||
		tuiProjection.AgentCount != len(agentEnvelope.Data.Nodes) {
		t.Fatalf("read surfaces disagree: cli_status=%q cli_events=%v tui=%#v http=%#v http_events=%v headless=%#v agents=%d",
			testCase.status, cliSequences, tuiProjection, detailEnvelope.Data.Run,
			httpSequences, headlessEnd, len(agentEnvelope.Data.Nodes))
	}
}

func assertListPaginationContract(t *testing.T, st *store.SQLiteStore,
	sessionManager *session.Manager, toolManager tui.ToolManager, api http.Handler,
) {
	t.Helper()
	ctx := context.Background()
	expectedRuns, err := st.ListRuns(ctx, domain.RunFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	expectedSessions, err := st.ListSessionsPage(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(expectedRuns) != 53 || len(expectedSessions) != 53 {
		t.Fatalf("pagination fixture drifted: runs=%d sessions=%d",
			len(expectedRuns), len(expectedSessions))
	}
	expectedRunIDs := make([]string, len(expectedRuns))
	expectedSessionIDs := make([]string, len(expectedSessions))
	for index, run := range expectedRuns {
		expectedRunIDs[index] = run.ID
	}
	for index, sess := range expectedSessions {
		expectedSessionIDs[index] = sess.ID
	}

	cliRuns, stderr, code := executeTestCommand(t, "run", "list", "--limit", "100")
	if code != 0 || stderr != "" {
		t.Fatalf("CLI Run list failed: code=%d stderr=%s", code, stderr)
	}
	cliSessions, stderr, code := executeTestCommand(t, "session", "list")
	if code != 0 || stderr != "" {
		t.Fatalf("CLI Session list failed: code=%d stderr=%s", code, stderr)
	}
	if got := cliRowIDs(t, cliRuns); !slices.Equal(got, expectedRunIDs) {
		t.Fatalf("CLI Run ordering drifted: got=%v want=%v", got, expectedRunIDs)
	}
	if got := cliRowIDs(t, cliSessions); !slices.Equal(got, expectedSessionIDs) {
		t.Fatalf("CLI Session ordering drifted: got=%v want=%v", got, expectedSessionIDs)
	}

	picker, err := tui.NewPicker(ctx, sessionManager, toolManager, "", "", "", st)
	if err != nil {
		t.Fatal(err)
	}
	pickerProjection := picker.CurrentProjection()
	if pickerProjection.View != "runs" || len(pickerProjection.Runs) != 50 ||
		len(pickerProjection.Sessions) != 50 || !pickerProjection.RunsTruncated ||
		!pickerProjection.SessionsTruncated {
		t.Fatalf("TUI bounded picker contract drifted: %#v", pickerProjection)
	}
	for index, projection := range pickerProjection.Runs {
		expected := expectedRuns[index]
		if projection.RunID != expected.ID || projection.MissionID != expected.MissionID ||
			projection.SessionID != expected.SessionID || projection.Status != expected.Status {
			t.Fatalf("TUI Run picker ordering drifted at %d: got=%#v want=%#v",
				index, projection, expected)
		}
	}
	for index, projection := range pickerProjection.Sessions {
		expected := expectedSessions[index]
		if projection.SessionID != expected.ID || projection.Status != expected.Status {
			t.Fatalf("TUI Session picker ordering drifted at %d: got=%#v want=%#v",
				index, projection, expected)
		}
	}

	httpRuns, runPageSizes := readHTTPContractPages[httpapi.RunView](t, api,
		surfaceContractToken, "/api/v1/runs", 20)
	httpSessions, sessionPageSizes := readHTTPContractPages[httpapi.SessionView](t, api,
		surfaceContractToken, "/api/v1/sessions", 20)
	if !slices.Equal(runPageSizes, []int{20, 20, 13}) ||
		!slices.Equal(sessionPageSizes, []int{20, 20, 13}) {
		t.Fatalf("HTTP page boundaries drifted: runs=%v sessions=%v",
			runPageSizes, sessionPageSizes)
	}
	if len(httpRuns) != len(expectedRuns) || len(httpSessions) != len(expectedSessions) {
		t.Fatalf("HTTP pagination omitted rows: runs=%d sessions=%d",
			len(httpRuns), len(httpSessions))
	}
	for index, view := range httpRuns {
		expected := expectedRuns[index]
		if view.ID != expected.ID || view.MissionID != expected.MissionID ||
			view.SessionID != expected.SessionID || view.Status != string(expected.Status) {
			t.Fatalf("HTTP Run pagination drifted at %d: got=%#v want=%#v",
				index, view, expected)
		}
	}
	for index, view := range httpSessions {
		expected := expectedSessions[index]
		if view.ID != expected.ID || view.Status != expected.Status {
			t.Fatalf("HTTP Session pagination drifted at %d: got=%#v want=%#v",
				index, view, expected)
		}
	}
	var emptyEnvelope httpContractEnvelope[[]httpapi.RunView]
	readHTTPContract(t, api, surfaceContractToken,
		"/api/v1/runs?limit=20&status=waiting_approval", &emptyEnvelope)
	if len(emptyEnvelope.Data) != 0 || emptyEnvelope.Page == nil ||
		emptyEnvelope.Page.Limit != 20 || emptyEnvelope.Page.NextCursor != "" ||
		emptyEnvelope.Page.Truncated {
		t.Fatalf("HTTP empty-page contract drifted: data=%#v page=%#v",
			emptyEnvelope.Data, emptyEnvelope.Page)
	}
}

func assertEventPaginationContract(t *testing.T, st *store.SQLiteStore,
	api http.Handler, runID string,
) {
	t.Helper()
	if runID == "" {
		t.Fatal("paused Run fixture is missing")
	}
	cliEvents, stderr, code := executeTestCommand(t, "run", "events", runID)
	if code != 0 || stderr != "" {
		t.Fatalf("CLI event list failed: code=%d stderr=%s", code, stderr)
	}
	expectedSequences := cliEventSequences(t, cliEvents)
	httpEvents, httpPageSizes := readHTTPContractPages[httpapi.EventView](t, api,
		surfaceContractToken, "/api/v1/runs/"+runID+"/events", 2)
	httpSequences := make([]int64, len(httpEvents))
	for index, event := range httpEvents {
		httpSequences[index] = event.Sequence
	}
	headlessSequences, headlessPageSizes := readHeadlessContractPages(t,
		headless.NewExporter(st), runID, 2)
	if len(httpPageSizes) < 2 || len(headlessPageSizes) < 2 {
		t.Fatalf("event fixture did not cross a page boundary: http=%v headless=%v",
			httpPageSizes, headlessPageSizes)
	}
	if !slices.Equal(expectedSequences, httpSequences) ||
		!slices.Equal(expectedSequences, headlessSequences) ||
		!slices.Equal(httpPageSizes, headlessPageSizes) {
		t.Fatalf("event pagination disagrees: cli=%v http=%v pages=%v headless=%v pages=%v",
			expectedSequences, httpSequences, httpPageSizes,
			headlessSequences, headlessPageSizes)
	}
	var emptyResume bytes.Buffer
	if err := headless.NewExporter(st).Export(context.Background(), &emptyResume,
		headless.Request{RunID: runID,
			AfterSequence: expectedSequences[len(expectedSequences)-1], MaxEvents: 2}); err != nil {
		t.Fatalf("headless empty resume failed: %v", err)
	}
	emptyRecords := decodeHeadlessCLIRecords(t, emptyResume.String())
	if len(emptyRecords) != 1 || emptyRecords[0].Kind != headless.EndRecordKind ||
		emptyRecords[0].EventsEmitted != 0 || emptyRecords[0].HasMore ||
		emptyRecords[0].Reason != "snapshot" ||
		emptyRecords[0].AfterSequence != expectedSequences[len(expectedSequences)-1] ||
		emptyRecords[0].LastSequence != expectedSequences[len(expectedSequences)-1] {
		t.Fatalf("headless empty-resume contract drifted: %#v", emptyRecords)
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
		t.Fatalf("decode HTTP contract %s: %v body=%s",
			path, err, response.Body.String())
	}
}

func readHTTPContractPages[T any](t *testing.T, api http.Handler, token string,
	resourcePath string, limit int,
) ([]T, []int) {
	t.Helper()
	var result []T
	var sizes []int
	cursor := ""
	seen := make(map[string]struct{})
	for pageIndex := 0; pageIndex < 100; pageIndex++ {
		values := url.Values{"limit": []string{strconv.Itoa(limit)}}
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		var envelope httpContractEnvelope[[]T]
		readHTTPContract(t, api, token, resourcePath+"?"+values.Encode(), &envelope)
		if envelope.Page == nil || envelope.Page.Limit != limit || envelope.Page.Truncated {
			t.Fatalf("HTTP page metadata drifted for %s: %#v", resourcePath, envelope.Page)
		}
		result = append(result, envelope.Data...)
		sizes = append(sizes, len(envelope.Data))
		next := envelope.Page.NextCursor
		if next == "" {
			return result, sizes
		}
		if len(envelope.Data) != limit {
			t.Fatalf("HTTP page advertised continuation after %d of %d rows",
				len(envelope.Data), limit)
		}
		if _, exists := seen[next]; exists {
			t.Fatalf("HTTP cursor loop detected for %s", resourcePath)
		}
		seen[next] = struct{}{}
		cursor = next
	}
	t.Fatalf("HTTP pagination exceeded its test page bound for %s", resourcePath)
	return nil, nil
}

func readHeadlessContractPages(t *testing.T, exporter *headless.Exporter,
	runID string, limit int,
) ([]int64, []int) {
	t.Helper()
	var sequences []int64
	var sizes []int
	var after int64
	for pageIndex := 0; pageIndex < 100; pageIndex++ {
		var output bytes.Buffer
		err := exporter.Export(context.Background(), &output, headless.Request{
			RunID: runID, AfterSequence: after, MaxEvents: limit,
		})
		records := decodeHeadlessCLIRecords(t, output.String())
		if len(records) == 0 {
			t.Fatal("headless page omitted its end record")
		}
		end := records[len(records)-1]
		if end.Kind != headless.EndRecordKind || end.AfterSequence != after ||
			end.EventsEmitted != len(records)-1 || end.LastSequence != end.SuggestedResume ||
			end.Truncated != end.HasMore {
			t.Fatalf("headless page metadata drifted: %#v", end)
		}
		for _, record := range records[:len(records)-1] {
			if record.Kind != headless.EventRecordKind || record.Sequence != after+1 {
				t.Fatalf("headless sequence is not contiguous: after=%d record=%#v",
					after, record)
			}
			sequences = append(sequences, record.Sequence)
			after = record.Sequence
		}
		sizes = append(sizes, len(records)-1)
		if end.HasMore {
			if err == nil || apperror.ExitCode(err) != 8 || end.ExitCode != 8 ||
				end.Reason != "max_events" || len(records) == 1 {
				t.Fatalf("headless continuation contract drifted: end=%#v err=%v", end, err)
			}
			continue
		}
		if err != nil || end.ExitCode != 0 || end.Terminal || end.Reason != "snapshot" {
			t.Fatalf("headless final snapshot drifted: end=%#v err=%v", end, err)
		}
		return sequences, sizes
	}
	t.Fatal("headless pagination exceeded its test page bound")
	return nil, nil
}

func cliEventSequences(t *testing.T, output string) []int64 {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		t.Fatal("CLI event output is empty")
	}
	sequences := make([]int64, 0, len(lines))
	for index, line := range lines {
		firstField := strings.SplitN(line, "\t", 2)[0]
		sequence, err := strconv.ParseInt(
			strings.TrimPrefix(strings.TrimSpace(firstField), "#"), 10, 64)
		if err != nil || sequence != int64(index+1) {
			t.Fatalf("parse contiguous CLI event sequence %q: %v", line, err)
		}
		sequences = append(sequences, sequence)
	}
	return sequences
}

func cliRowIDs(t *testing.T, output string) []string {
	t.Helper()
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || strings.HasPrefix(trimmed, "no ") {
		t.Fatalf("CLI list output is empty: %q", output)
	}
	lines := strings.Split(trimmed, "\n")
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(fields) != 2 || strings.TrimSpace(fields[0]) == "" {
			t.Fatalf("CLI list row is malformed: %q", line)
		}
		ids = append(ids, strings.TrimSpace(fields[0]))
	}
	return ids
}
