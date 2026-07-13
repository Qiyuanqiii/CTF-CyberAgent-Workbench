package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/redact"
)

type successEnvelope struct {
	Version   string `json:"version"`
	RequestID string `json:"request_id"`
	Data      any    `json:"data"`
	Page      *Page  `json:"page,omitempty"`
}

type errorEnvelope struct {
	Version   string       `json:"version"`
	RequestID string       `json:"request_id"`
	Error     apiErrorView `json:"error"`
}

type apiErrorView struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (a *API) route(request *http.Request) (any, *Page, error) {
	requestPath := request.URL.Path
	switch requestPath {
	case "/api/v1":
		if err := rejectQuery(request.URL.Query()); err != nil {
			return nil, nil, err
		}
		resources := []string{"runs", "sessions", "work-items", "notes", "artifacts",
			"agent-graph", "delegations", "readonly-fanout", "finding-reports",
			"event-stream", "openapi"}
		if a.controlEnabled {
			resources = append(resources, "model-cancellation-control",
				"specialist-model-cancellation-control")
		}
		return IndexView{APIVersion: Version, AppVersion: a.appVersion, Resources: resources}, nil, nil
	case "/api/v1/health":
		if err := rejectQuery(request.URL.Query()); err != nil {
			return nil, nil, err
		}
		return a.health(request)
	}
	if !strings.HasPrefix(requestPath, "/api/v1/") {
		return nil, nil, apperror.New(apperror.CodeNotFound, "HTTP API endpoint was not found")
	}
	segments := strings.Split(strings.TrimPrefix(requestPath, "/api/v1/"), "/")
	for _, segment := range segments {
		if err := validatePathIdentity(segment); err != nil {
			return nil, nil, err
		}
	}
	switch segments[0] {
	case "runs":
		return a.routeRuns(request, segments)
	case "sessions":
		return a.routeSessions(request, segments)
	case "work-items":
		if len(segments) == 2 {
			return a.workItem(request, segments[1])
		}
	case "notes":
		if len(segments) == 2 {
			return a.note(request, segments[1])
		}
	case "artifacts":
		if len(segments) == 2 {
			return a.artifact(request, segments[1])
		}
	}
	return nil, nil, apperror.New(apperror.CodeNotFound, "HTTP API endpoint was not found")
}

func (a *API) routeRuns(request *http.Request, segments []string) (any, *Page, error) {
	switch len(segments) {
	case 1:
		return a.runs(request)
	case 2:
		return a.run(request, segments[1])
	case 3:
		switch segments[2] {
		case "agent-graph":
			return a.runAgentGraph(request, segments[1])
		case "delegations":
			return a.runDelegations(request, segments[1])
		case "fanout-plans":
			return a.runFanoutPlans(request, segments[1])
		case "reports":
			return a.runFindingReports(request, segments[1])
		case "events":
			return a.runEvents(request, segments[1])
		case "work-items":
			return a.runWorkItems(request, segments[1])
		case "notes":
			return a.runNotes(request, segments[1])
		case "artifacts":
			return a.runArtifacts(request, segments[1])
		case "tool-rounds":
			return a.runToolRounds(request, segments[1])
		}
	case 4:
		if segments[2] == "reports" {
			return a.runFindingReport(request, segments[1], segments[3])
		}
	}
	return nil, nil, apperror.New(apperror.CodeNotFound, "Run HTTP API endpoint was not found")
}

func (a *API) routeSessions(request *http.Request, segments []string) (any, *Page, error) {
	switch len(segments) {
	case 1:
		return a.sessions(request)
	case 2:
		return a.session(request, segments[1])
	case 3:
		if segments[2] == "messages" {
			return a.sessionMessages(request, segments[1])
		}
	}
	return nil, nil, apperror.New(apperror.CodeNotFound, "Session HTTP API endpoint was not found")
}

func (a *API) health(request *http.Request) (any, *Page, error) {
	version, err := a.store.SchemaVersion(request.Context())
	if err != nil {
		return nil, nil, err
	}
	return HealthView{Status: "ok", APIVersion: Version, AppVersion: a.appVersion,
		SchemaVersion: version}, nil, nil
}

func (a *API) runs(request *http.Request) (any, *Page, error) {
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "limit", "cursor", "status", "mission_id"); err != nil {
		return nil, nil, err
	}
	pageRequest, err := parsePage(values, request.URL.Path)
	if err != nil {
		return nil, nil, err
	}
	filter := domain.RunFilter{Limit: pageRequest.Limit + 1, Offset: pageRequest.Offset}
	if raw, ok := singleQueryValue(values, "mission_id"); ok {
		if err := validateIdentity(raw, "mission id"); err != nil {
			return nil, nil, err
		}
		filter.MissionID = raw
	}
	if raw, ok := singleQueryValue(values, "status"); ok {
		status := domain.RunStatus(strings.ToLower(raw))
		if !domain.ValidRunStatus(status) {
			return nil, nil, apperror.New(apperror.CodeInvalidArgument, "invalid Run status filter")
		}
		filter.Status = status
	}
	runs, err := a.store.ListRuns(request.Context(), filter)
	if err != nil {
		return nil, nil, err
	}
	views := make([]RunView, len(runs))
	for index := range runs {
		views[index] = runView(runs[index])
	}
	views, page := trimPage(views, pageRequest)
	return views, page, nil
}

func (a *API) run(request *http.Request, runID string) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	run, err := a.store.GetRun(request.Context(), runID)
	if err != nil {
		return nil, nil, err
	}
	mission, err := a.store.GetMission(request.Context(), run.MissionID)
	if err != nil {
		return nil, nil, err
	}
	mode, err := a.store.GetRunMode(request.Context(), run.ID)
	if err != nil {
		return nil, nil, err
	}
	usage, err := a.store.GetToolCallUsage(request.Context(), run.ID)
	if err != nil {
		return nil, nil, err
	}
	detail := RunDetailView{Run: runView(run), Mission: missionView(mission),
		Mode: runModeView(mode), ToolUsage: toolUsageView(usage)}
	checkpoint, found, err := a.store.GetSupervisorCheckpoint(request.Context(), run.ID)
	if err != nil {
		return nil, nil, err
	}
	if found {
		view := checkpointView(checkpoint)
		detail.Checkpoint = &view
	}
	lease, found, err := a.store.GetRunExecutionLease(request.Context(), run.ID)
	if err != nil {
		return nil, nil, err
	}
	if found {
		view := runExecutionLeaseView(lease, time.Now().UTC())
		detail.Lease = &view
	}
	return detail, nil, nil
}

func (a *API) runEvents(request *http.Request, runID string) (any, *Page, error) {
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "limit", "cursor"); err != nil {
		return nil, nil, err
	}
	if _, err := a.store.GetRun(request.Context(), runID); err != nil {
		return nil, nil, err
	}
	pageRequest, err := parsePage(values, request.URL.Path)
	if err != nil {
		return nil, nil, err
	}
	events, err := a.store.ListRunEventsPage(request.Context(), runID,
		pageRequest.Offset, pageRequest.Limit+1)
	if err != nil {
		return nil, nil, err
	}
	views := make([]EventView, len(events))
	for index := range events {
		views[index] = eventView(events[index])
	}
	views, page := trimPage(views, pageRequest)
	return views, page, nil
}

func (a *API) runWorkItems(request *http.Request, runID string) (any, *Page, error) {
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "limit", "cursor", "status", "owner", "owner_agent_id"); err != nil {
		return nil, nil, err
	}
	if _, err := a.store.GetRun(request.Context(), runID); err != nil {
		return nil, nil, err
	}
	pageRequest, err := parsePage(values, request.URL.Path)
	if err != nil {
		return nil, nil, err
	}
	statuses, err := workItemStatuses(values["status"])
	if err != nil {
		return nil, nil, err
	}
	filter := domain.WorkItemFilter{RunID: runID, Statuses: statuses,
		Limit: pageRequest.Limit + 1, Offset: pageRequest.Offset}
	if raw, ok := singleQueryValue(values, "owner"); ok {
		if err := validateOptionalLabel(raw, "work item owner", domain.MaxWorkItemOwnerRunes); err != nil {
			return nil, nil, err
		}
		filter.Owner = raw
	}
	if raw, ok := singleQueryValue(values, "owner_agent_id"); ok {
		if !domain.ValidAgentID(raw) {
			return nil, nil, apperror.New(apperror.CodeInvalidArgument,
				"work item owner_agent_id filter is invalid")
		}
		filter.OwnerAgentID = raw
	}
	items, err := a.store.ListWorkItems(request.Context(), filter)
	if err != nil {
		return nil, nil, err
	}
	views := make([]WorkItemView, len(items))
	for index := range items {
		views[index] = workItemView(items[index])
	}
	views, page := trimPage(views, pageRequest)
	return views, page, nil
}

func (a *API) runNotes(request *http.Request, runID string) (any, *Page, error) {
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "limit", "cursor", "status", "category",
		"visibility", "owner", "owner_agent_id", "tag", "pinned"); err != nil {
		return nil, nil, err
	}
	if _, err := a.store.GetRun(request.Context(), runID); err != nil {
		return nil, nil, err
	}
	pageRequest, err := parsePage(values, request.URL.Path)
	if err != nil {
		return nil, nil, err
	}
	filter := domain.NoteFilter{RunID: runID, Limit: pageRequest.Limit + 1, Offset: pageRequest.Offset}
	if filter.Statuses, err = noteStatuses(values["status"]); err != nil {
		return nil, nil, err
	}
	if filter.Categories, err = noteCategories(values["category"]); err != nil {
		return nil, nil, err
	}
	if filter.Visibilities, err = noteVisibilities(values["visibility"]); err != nil {
		return nil, nil, err
	}
	if raw, ok := singleQueryValue(values, "owner"); ok {
		if err := validateOptionalLabel(raw, "note owner", domain.MaxNoteOwnerRunes); err != nil {
			return nil, nil, err
		}
		filter.Owner = raw
	}
	if raw, ok := singleQueryValue(values, "owner_agent_id"); ok {
		if !domain.ValidAgentID(raw) {
			return nil, nil, apperror.New(apperror.CodeInvalidArgument,
				"note owner_agent_id filter is invalid")
		}
		filter.OwnerAgentID = raw
	}
	if filter.Tags, err = queryTokens(values["tag"], domain.MaxNoteTags, domain.MaxNoteTagRunes, "note tag"); err != nil {
		return nil, nil, err
	}
	if raw, ok := singleQueryValue(values, "pinned"); ok {
		value, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			return nil, nil, apperror.New(apperror.CodeInvalidArgument, "note pinned filter must be true or false")
		}
		filter.Pinned = &value
	}
	notes, err := a.store.ListNotes(request.Context(), filter)
	if err != nil {
		return nil, nil, err
	}
	views := make([]NoteView, len(notes))
	for index := range notes {
		views[index] = noteView(notes[index])
	}
	views, page := trimPage(views, pageRequest)
	return views, page, nil
}

func (a *API) runArtifacts(request *http.Request, runID string) (any, *Page, error) {
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "limit", "cursor", "source_id", "stream"); err != nil {
		return nil, nil, err
	}
	if _, err := a.store.GetRun(request.Context(), runID); err != nil {
		return nil, nil, err
	}
	pageRequest, err := parsePage(values, request.URL.Path)
	if err != nil {
		return nil, nil, err
	}
	filter := artifact.ListFilter{RunID: runID, Limit: pageRequest.Limit + 1, Offset: pageRequest.Offset}
	if raw, ok := singleQueryValue(values, "source_id"); ok {
		if err := validateIdentity(raw, "artifact source id"); err != nil {
			return nil, nil, err
		}
		filter.SourceID = raw
	}
	if raw, ok := singleQueryValue(values, "stream"); ok {
		filter.Stream = artifact.Stream(strings.ToLower(raw))
		if !filter.Stream.Valid() {
			return nil, nil, apperror.New(apperror.CodeInvalidArgument, "invalid artifact stream filter")
		}
	}
	descriptors, err := a.store.ListRunArtifacts(request.Context(), filter)
	if err != nil {
		return nil, nil, err
	}
	views := make([]ArtifactView, len(descriptors))
	for index := range descriptors {
		views[index] = artifactView(descriptors[index])
	}
	views, page := trimPage(views, pageRequest)
	return views, page, nil
}

func (a *API) runToolRounds(request *http.Request, runID string) (any, *Page, error) {
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "limit", "cursor"); err != nil {
		return nil, nil, err
	}
	if _, err := a.store.GetRun(request.Context(), runID); err != nil {
		return nil, nil, err
	}
	pageRequest, err := parsePage(values, request.URL.Path)
	if err != nil {
		return nil, nil, err
	}
	rounds, err := a.store.ListRunSupervisorToolRoundsPage(request.Context(), runID,
		pageRequest.Offset, pageRequest.Limit+1)
	if err != nil {
		return nil, nil, err
	}
	views := make([]SupervisorToolRoundView, len(rounds))
	for index := range rounds {
		views[index] = supervisorToolRoundView(rounds[index])
	}
	views, page := trimPage(views, pageRequest)
	return views, page, nil
}

func (a *API) sessions(request *http.Request) (any, *Page, error) {
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "limit", "cursor"); err != nil {
		return nil, nil, err
	}
	pageRequest, err := parsePage(values, request.URL.Path)
	if err != nil {
		return nil, nil, err
	}
	sessions, err := a.store.ListSessionsPage(request.Context(), pageRequest.Offset, pageRequest.Limit+1)
	if err != nil {
		return nil, nil, err
	}
	views := make([]SessionView, len(sessions))
	for index := range sessions {
		views[index] = sessionView(sessions[index])
	}
	views, page := trimPage(views, pageRequest)
	return views, page, nil
}

func (a *API) session(request *http.Request, sessionID string) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	sess, err := a.store.GetSession(request.Context(), sessionID)
	if err != nil {
		return nil, nil, err
	}
	detail := SessionDetailView{Session: sessionView(sess)}
	run, found, err := a.store.GetRunBySession(request.Context(), sess.ID)
	if err != nil {
		return nil, nil, err
	}
	if found {
		view := runView(run)
		detail.Run = &view
	}
	return detail, nil, nil
}

func (a *API) sessionMessages(request *http.Request, sessionID string) (any, *Page, error) {
	values := request.URL.Query()
	if err := validateSingleQueryValues(values, "limit", "cursor", "include_compacted"); err != nil {
		return nil, nil, err
	}
	if _, err := a.store.GetSession(request.Context(), sessionID); err != nil {
		return nil, nil, err
	}
	pageRequest, err := parsePage(values, request.URL.Path)
	if err != nil {
		return nil, nil, err
	}
	includeCompacted := false
	if raw, ok := singleQueryValue(values, "include_compacted"); ok {
		includeCompacted, err = strconv.ParseBool(raw)
		if err != nil {
			return nil, nil, apperror.New(apperror.CodeInvalidArgument,
				"include_compacted must be true or false")
		}
	}
	messages, err := a.store.ListSessionMessagesPage(request.Context(), sessionID, includeCompacted,
		pageRequest.Offset, pageRequest.Limit+1)
	if err != nil {
		return nil, nil, err
	}
	views := make([]MessageView, len(messages))
	for index := range messages {
		views[index] = messageView(messages[index])
	}
	views, page := trimPage(views, pageRequest)
	return views, page, nil
}

func (a *API) workItem(request *http.Request, id string) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	item, err := a.store.GetWorkItem(request.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	return workItemView(item), nil, nil
}

func (a *API) note(request *http.Request, id string) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	note, err := a.store.GetNote(request.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	return noteView(note), nil, nil
}

func (a *API) artifact(request *http.Request, id string) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	descriptor, err := a.store.GetRunArtifactDescriptor(request.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	return artifactView(descriptor), nil, nil
}

func (a *API) writeSuccess(writer http.ResponseWriter, requestID string, data any, page *Page) {
	a.writeSuccessStatus(writer, requestID, data, page, http.StatusOK)
}

func (a *API) writeSuccessStatus(writer http.ResponseWriter, requestID string, data any, page *Page, status int) {
	encoded, err := json.Marshal(successEnvelope{Version: Version, RequestID: requestID, Data: data, Page: page})
	if err != nil {
		a.writeError(writer, requestID, apperror.New(apperror.CodeInternal, "internal server error"), 0)
		return
	}
	if len(encoded) > MaxResponseBytes {
		a.writeError(writer, requestID,
			apperror.New(apperror.CodeResourceExhausted, "HTTP API response exceeds its limit"), 0)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write(append(encoded, '\n'))
}

func (a *API) writeOpenAPI(writer http.ResponseWriter, requestID string) {
	if len(a.openAPI) == 0 || len(a.openAPI) > MaxResponseBytes {
		a.writeError(writer, requestID,
			apperror.New(apperror.CodeInternal, "internal server error"), 0)
		return
	}
	writer.Header().Set("Content-Type", "application/vnd.oai.openapi+json;version=3.1; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(a.openAPI)
}

func (a *API) writeError(writer http.ResponseWriter, requestID string, err error, statusOverride int) {
	classified := apperror.Normalize(err)
	code := apperror.CodeOf(classified)
	message := "internal server error"
	if code != apperror.CodeInternal {
		message = redact.String(strings.Join(strings.Fields(classified.Error()), " "))
		if runes := []rune(message); len(runes) > 1024 {
			message = string(runes[:1024])
		}
	}
	status := statusOverride
	if status == 0 {
		status = apperror.HTTPStatus(classified)
	}
	encoded, marshalErr := json.Marshal(errorEnvelope{Version: Version, RequestID: requestID,
		Error: apiErrorView{Code: string(code), Message: message}})
	if marshalErr != nil {
		encoded = []byte(`{"version":"api.v1","error":{"code":"INTERNAL","message":"internal server error"}}`)
		status = http.StatusInternalServerError
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write(append(encoded, '\n'))
}

func rejectQuery(values url.Values) error {
	if len(values) != 0 {
		return apperror.New(apperror.CodeInvalidArgument, "HTTP API endpoint does not accept query parameters")
	}
	return nil
}

func validatePathIdentity(value string) error {
	if value == "" || value == "." || value == ".." {
		return apperror.New(apperror.CodeNotFound, "HTTP API endpoint was not found")
	}
	return validateIdentity(value, "path identity")
}

func validateIdentity(value string, label string) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		len([]rune(value)) > 256 || strings.ContainsAny(value, "/\\") {
		return apperror.New(apperror.CodeInvalidArgument, label+" is invalid")
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return apperror.New(apperror.CodeInvalidArgument, label+" is invalid")
		}
	}
	return nil
}

func validateOptionalLabel(value string, label string, maxRunes int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		len([]rune(value)) > maxRunes {
		return apperror.New(apperror.CodeInvalidArgument, label+" is invalid")
	}
	for _, current := range value {
		if unicode.IsControl(current) {
			return apperror.New(apperror.CodeInvalidArgument, label+" is invalid")
		}
	}
	return nil
}

func queryTokens(values []string, maxItems int, maxRunes int, label string) ([]string, error) {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, token := range strings.Split(value, ",") {
			token = strings.TrimSpace(token)
			if err := validateOptionalLabel(token, label, maxRunes); err != nil {
				return nil, err
			}
			if _, exists := seen[token]; exists {
				continue
			}
			seen[token] = struct{}{}
			out = append(out, token)
			if len(out) > maxItems {
				return nil, apperror.New(apperror.CodeInvalidArgument,
					fmt.Sprintf("%s filter exceeds %d values", label, maxItems))
			}
		}
	}
	return out, nil
}

func workItemStatuses(values []string) ([]domain.WorkItemStatus, error) {
	tokens, err := queryTokens(values, 5, 32, "work item status")
	if err != nil {
		return nil, err
	}
	out := make([]domain.WorkItemStatus, len(tokens))
	for index, token := range tokens {
		status, err := domain.ParseWorkItemStatus(token)
		if err != nil {
			return nil, apperror.New(apperror.CodeInvalidArgument, "invalid work item status filter")
		}
		out[index] = status
	}
	return out, nil
}

func noteStatuses(values []string) ([]domain.NoteStatus, error) {
	tokens, err := queryTokens(values, 2, 32, "note status")
	if err != nil {
		return nil, err
	}
	out := make([]domain.NoteStatus, len(tokens))
	for index, token := range tokens {
		status, err := domain.ParseNoteStatus(token)
		if err != nil {
			return nil, apperror.New(apperror.CodeInvalidArgument, "invalid note status filter")
		}
		out[index] = status
	}
	return out, nil
}

func noteCategories(values []string) ([]domain.NoteCategory, error) {
	tokens, err := queryTokens(values, 5, 32, "note category")
	if err != nil {
		return nil, err
	}
	out := make([]domain.NoteCategory, len(tokens))
	for index, token := range tokens {
		category, err := domain.ParseNoteCategory(token)
		if err != nil {
			return nil, apperror.New(apperror.CodeInvalidArgument, "invalid note category filter")
		}
		out[index] = category
	}
	return out, nil
}

func noteVisibilities(values []string) ([]domain.NoteVisibility, error) {
	tokens, err := queryTokens(values, 3, 32, "note visibility")
	if err != nil {
		return nil, err
	}
	out := make([]domain.NoteVisibility, len(tokens))
	for index, token := range tokens {
		visibility, err := domain.ParseNoteVisibility(token)
		if err != nil {
			return nil, apperror.New(apperror.CodeInvalidArgument, "invalid note visibility filter")
		}
		out[index] = visibility
	}
	return out, nil
}
