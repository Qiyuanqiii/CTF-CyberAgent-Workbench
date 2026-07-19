package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/credential"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/modelregistry"
	"cyberagent-workbench/internal/operationreceipt"
	"cyberagent-workbench/internal/operatoraction"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/workspace"
)

const (
	openAPISpecVersion       = "3.1.0"
	openAPIJSONSchemaDialect = "https://json-schema.org/draft/2020-12/schema"
	openAPIContentType       = "application/vnd.oai.openapi+json"
)

type openAPIDocument struct {
	OpenAPI           string                     `json:"openapi"`
	JSONSchemaDialect string                     `json:"jsonSchemaDialect"`
	Info              openAPIInfo                `json:"info"`
	Servers           []openAPIServer            `json:"servers"`
	Security          []map[string][]string      `json:"security"`
	Tags              []openAPITag               `json:"tags"`
	Paths             map[string]openAPIPathItem `json:"paths"`
	Components        openAPIComponents          `json:"components"`
	ReadOnly          bool                       `json:"x-cyberagent-read-only"`
}

type openAPIInfo struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Version     string         `json:"version"`
	License     openAPILicense `json:"license"`
}

type openAPILicense struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

type openAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type openAPITag struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type openAPIPathItem struct {
	Get  *openAPIOperation `json:"get,omitempty"`
	Post *openAPIOperation `json:"post,omitempty"`
}

type openAPIOperation struct {
	OperationID string                `json:"operationId"`
	Summary     string                `json:"summary"`
	Description string                `json:"description"`
	Tags        []string              `json:"tags"`
	Parameters  []openAPIParameter    `json:"parameters,omitempty"`
	Responses   map[string]any        `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
	RequestBody *openAPIRequestBody   `json:"requestBody,omitempty"`
	ReadOnly    bool                  `json:"x-cyberagent-read-only"`
	Streaming   bool                  `json:"x-cyberagent-streaming,omitempty"`
}

type openAPIRequestBody struct {
	Required bool                        `json:"required"`
	Content  map[string]openAPIMediaType `json:"content"`
}

type openAPIParameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"`
	Description string         `json:"description"`
	Required    bool           `json:"required,omitempty"`
	Style       string         `json:"style,omitempty"`
	Explode     *bool          `json:"explode,omitempty"`
	Schema      map[string]any `json:"schema"`
}

type openAPIComponents struct {
	Schemas         map[string]map[string]any      `json:"schemas"`
	Responses       map[string]openAPIResponse     `json:"responses"`
	SecuritySchemes map[string]openAPISecurityType `json:"securitySchemes"`
}

type openAPISecurityType struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme"`
	BearerFormat string `json:"bearerFormat"`
	Description  string `json:"description"`
}

type openAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]openAPIMediaType `json:"content"`
}

type openAPIMediaType struct {
	Schema map[string]any `json:"schema"`
}

type openAPIOperationSpec struct {
	Path        string
	Method      string
	OperationID string
	Summary     string
	Description string
	Tag         string
	DataType    reflect.Type
	Collection  bool
	Paginated   bool
	NotFound    bool
	RawDocument bool
	Streaming   bool
	Parameters  []openAPIParameter
	RequestType reflect.Type
	Control     bool
}

// GenerateOpenAPI creates the canonical client contract from the Go response DTOs.
func GenerateOpenAPI() ([]byte, error) {
	registry := newOpenAPISchemaRegistry()
	paths := make(map[string]openAPIPathItem)
	for _, spec := range openAPIOperationSpecs() {
		operation, err := buildOpenAPIOperation(spec, registry)
		if err != nil {
			return nil, err
		}
		item := paths[spec.Path]
		switch spec.Method {
		case "", http.MethodGet:
			if item.Get != nil {
				return nil, fmt.Errorf("duplicate OpenAPI GET path %q", spec.Path)
			}
			item.Get = &operation
		case http.MethodPost:
			if item.Post != nil {
				return nil, fmt.Errorf("duplicate OpenAPI POST path %q", spec.Path)
			}
			item.Post = &operation
		default:
			return nil, fmt.Errorf("unsupported OpenAPI method %q", spec.Method)
		}
		paths[spec.Path] = item
	}
	if registry.err != nil {
		return nil, registry.err
	}
	document := openAPIDocument{
		OpenAPI:           openAPISpecVersion,
		JSONSchemaDialect: openAPIJSONSchemaDialect,
		Info: openAPIInfo{
			Title: "CyberAgent Workbench Local API",
			Description: "Authenticated loopback-only API owned by the Go control plane. " +
				"Read operations expose durable metadata; separately authorized control capabilities permit exact active-call cancellation, non-authorizing execution-profile selection, idempotent creation of a closed workspace-bound Run, durable Run-bound Session steering, idempotent Run lifecycle transitions, and bounded execution through the Go Supervisor.",
			Version: Version,
			License: openAPILicense{Name: "Apache License 2.0", Identifier: "Apache-2.0"},
		},
		Servers:  []openAPIServer{{URL: "http://127.0.0.1:8765", Description: "Default loopback server"}},
		Security: []map[string][]string{{"BearerAuth": {}}},
		Tags: []openAPITag{
			{Name: "System", Description: "API discovery, health, and contract metadata."},
			{Name: "Models", Description: "Redacted Go-owned Provider and model-route availability."},
			{Name: "Runs", Description: "Durable Run state and Run-scoped projections."},
			{Name: "Agents", Description: "Bounded Agent graph and operator-gated delegation projections."},
			{Name: "Analysis", Description: "Read-only Fan-out plans and execution summaries."},
			{Name: "Reports", Description: "Finding reports and redacted lifecycle evidence summaries."},
			{Name: "Sessions", Description: "Persisted Session metadata and redacted messages."},
			{Name: "Workspaces", Description: "Registered Workspace identities without local root paths."},
			{Name: "Memory", Description: "Structured WorkItems and Notes."},
			{Name: "Artifacts", Description: "Content-free Artifact descriptors."},
			{Name: "Control", Description: "Separately authorized, audit-first control operations."},
		},
		Paths: paths,
		Components: openAPIComponents{
			Schemas:   registry.schemas,
			Responses: standardOpenAPIErrorResponses(registry),
			SecuritySchemes: map[string]openAPISecurityType{
				"BearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "opaque",
					Description: "Process-scoped read token; never persisted by CyberAgent."},
				"ControlBearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "opaque",
					Description: "Distinct optional control token; cannot authorize read operations and is never persisted."},
			},
		},
		ReadOnly: false,
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OpenAPI document: %w", err)
	}
	return append(encoded, '\n'), nil
}

func openAPIOperationSpecs() []openAPIOperationSpec {
	runID := pathIdentityParameter("run_id", "Run identity")
	agentID := pathIdentityParameter("agent_id", "Specialist Agent identity")
	sessionID := pathIdentityParameter("session_id", "Session identity")
	messageID := pathIdentityParameter("message_id", "Operator steering message identity")
	workItemID := pathIdentityParameter("work_item_id", "WorkItem identity")
	noteID := pathIdentityParameter("note_id", "Note identity")
	artifactID := pathIdentityParameter("artifact_id", "Artifact identity")
	reportID := pathIdentityParameter("report_id", "Finding Report identity")
	approvalID := pathIdentityParameter("approval_id", "Approval identity")
	editID := pathIdentityParameter("edit_id", "File edit identity")
	workspaceID := pathIdentityParameter("workspace_id", "Workspace identity")
	routeName := pathIdentityParameter("route", "Model route name")
	providerName := pathIdentityParameter("provider", "Provider name")
	return []openAPIOperationSpec{
		{Path: "/api/v1", OperationID: "getAPIIndex", Summary: "Inspect API resources",
			Description: "Returns API and application versions plus top-level resources.", Tag: "System",
			DataType: reflect.TypeOf(IndexView{})},
		{Path: "/api/v1/health", OperationID: "getHealth", Summary: "Inspect local API health",
			Description: "Reads the current SQLite schema version without mutating state.", Tag: "System",
			DataType: reflect.TypeOf(HealthView{})},
		{Path: "/api/v1/capabilities", OperationID: "getRuntimeCapabilities",
			Summary: "Inspect process-local runtime capabilities", Tag: "System",
			Description: "Returns bounded enablement metadata and Run wake worker health without bearer tokens, owner or lease identities, private errors, runtime enable controls, or persistent-service authority.",
			DataType:    reflect.TypeOf(RuntimeCapabilitiesView{})},
		{Path: "/api/v1/models", OperationID: "getModelAvailability",
			Summary: "Inspect redacted model availability", Tag: "Models",
			Description: "Returns deterministic Provider registration and route metadata without API keys, base URLs, environment variable names, network probes, or model calls.",
			DataType:    reflect.TypeOf(ModelAvailabilityView{})},
		{Path: ModelRouteControlPathTemplate, Method: http.MethodPost,
			OperationID: "selectModelRoute", Summary: "Persist a model route selection",
			Tag:         "Control",
			Description: "Persists one available Provider/model selection before updating the in-process Router. It does not call a model or expose credentials.",
			DataType:    reflect.TypeOf(ModelRouteAvailabilityView{}),
			RequestType: reflect.TypeOf(ModelRouteControlRequestView{}), Control: true,
			Parameters: []openAPIParameter{routeName}},
		{Path: ProviderDiagnosticPath, Method: http.MethodPost,
			OperationID: "diagnoseProvider", Summary: "Run an explicit Provider diagnostic",
			Tag:         "Control",
			Description: "Makes at most one minimal no-tool model request after explicit confirmation. The response is content-free and never includes model text, raw Provider errors, credentials, or endpoint URLs.",
			DataType:    reflect.TypeOf(ProviderDiagnosticView{}),
			RequestType: reflect.TypeOf(ProviderDiagnosticRequestView{}), Control: true},
		{Path: ProviderCredentialsPath, OperationID: "listProviderCredentials",
			Summary: "Inspect Provider credential status", Tag: "Models",
			Description: "Returns only configured status and OS-store availability. It never reads credential plaintext into an API response.",
			DataType:    reflect.TypeOf(ProviderCredentialListView{})},
		{Path: ProviderCredentialPathTemplate, Method: http.MethodPost,
			OperationID: "changeProviderCredential", Summary: "Set or delete an OS-owned Provider credential",
			Tag:         "Control",
			Description: "Accepts one transient secret for direct storage in the OS credential manager, or deletes one exact Provider credential, then atomically reloads a Go-owned Provider Registry generation. Active calls retain their captured Provider and plaintext is never returned, persisted in SQLite, logged, or placed in an event.",
			DataType:    reflect.TypeOf(ProviderCredentialStatusView{}),
			RequestType: reflect.TypeOf(ProviderCredentialRequestView{}), Control: true,
			Parameters: []openAPIParameter{providerName}},
		{Path: OpenAPIPath, OperationID: "getOpenAPI", Summary: "Read the OpenAPI contract",
			Description: "Returns the raw deterministic OpenAPI 3.1 JSON document under the same authentication boundary.",
			Tag:         "System", RawDocument: true},
		{Path: "/api/v1/operation-receipts", OperationID: "listOperationReceipts",
			Summary: "List durable operation receipts", Tag: "Operations",
			Description: "Returns a refreshable bounded history derived from terminal FileEdit apply, foreground Run wake, and inert Skill installation facts. It omits operation keys, paths, content digests, requester identities, lease identities, and package archive metadata.",
			DataType:    reflect.TypeOf(OperationReceiptHistoryView{}),
			Parameters: []openAPIParameter{
				identityQueryParameter("run_id", "Optional exact Run filter"),
				{Name: "limit", In: "query", Description: "Maximum terminal receipts",
					Schema: map[string]any{"type": "integer", "minimum": 1,
						"maximum": operationreceipt.MaxHistoryItems,
						"default": application.DefaultOperationReceiptHistoryLimit}},
			}},
		{Path: "/api/v1/runs", OperationID: "listRuns", Summary: "List Runs", Tag: "Runs",
			Description: "Returns a bounded cursor page of durable Runs.", DataType: reflect.TypeOf(RunView{}),
			Collection: true, Paginated: true, Parameters: append(paginationParameters(),
				stringQueryParameter("status", "Exact Run status filter", runStatuses()),
				identityQueryParameter("mission_id", "Exact Mission identity filter"))},
		{Path: RunCreationControlPath, Method: http.MethodPost,
			OperationID: "createRun", Summary: "Create a controlled Run", Tag: "Control",
			Description: "Atomically creates one Mission, interactive Run, active Session, closed Run mode, preview execution profile, root Agent, and initial events. The request cannot select a model, budget, network target, existing Session, process backend, or capability grant.",
			DataType:    reflect.TypeOf(RunCreationControlView{}),
			RequestType: reflect.TypeOf(RunCreationControlRequestView{}), Control: true,
			Parameters: []openAPIParameter{
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: "/api/v1/workspaces", OperationID: "listWorkspaces", Summary: "List Workspaces",
			Tag: "Workspaces", Description: "Returns registered Workspace ids and names without local root paths.",
			DataType: reflect.TypeOf(WorkspaceView{}), Collection: true, Paginated: true,
			Parameters: paginationParameters()},
		{Path: "/api/v1/workspaces/{workspace_id}/explore",
			OperationID: "exploreWorkspace", Summary: "Inspect a bounded Workspace entry",
			Tag:         "Workspaces",
			Description: "Lists one directory level or returns a bounded redacted UTF-8 file preview. Go resolves the registered Workspace root, rejects traversal and symbolic links, omits internal staging files, and marks all content as non-authorizing evidence. Local root paths are never returned.",
			DataType:    reflect.TypeOf(WorkspaceExplorerView{}), NotFound: true,
			Parameters: []openAPIParameter{workspaceID,
				{Name: "path", In: "query", Description: "Canonical Workspace-relative path returned by a previous explorer response; defaults to the root",
					Schema: map[string]any{"type": "string", "maxLength": workspace.MaxExplorerPathRunes,
						"default": "."}}}},
		{Path: "/api/v1/workspaces/{workspace_id}/search",
			OperationID: "searchWorkspace", Summary: "Search bounded Workspace evidence",
			Tag:         "Workspaces",
			Description: "Performs one deterministic bounded scan over redacted UTF-8 Explorer projections. It follows no links, starts no indexer, returns canonical relative references and snippets only, and marks every result as non-authorizing evidence.",
			DataType:    reflect.TypeOf(WorkspaceSearchView{}), NotFound: true,
			Parameters: []openAPIParameter{workspaceID,
				{Name: "query", In: "query", Description: "Normalized case-insensitive filename or redacted text query",
					Required: true, Schema: map[string]any{"type": "string", "minLength": 1,
						"maxLength": workspace.MaxSearchQueryRunes}}}},
		{Path: EvidenceAttachmentPathTemplate, Method: http.MethodPost,
			OperationID: "attachRunEvidence", Summary: "Attach non-authorizing Workspace evidence",
			Tag:         "Control",
			Description: "Revalidates one exact redacted Workspace file projection and atomically appends it to the bound Session as tool-role evidence. Document text never becomes operator instruction and the operation starts no model, tool, process, or network call.",
			DataType:    reflect.TypeOf(EvidenceAttachmentView{}),
			RequestType: reflect.TypeOf(EvidenceAttachmentRequestView{}), Control: true,
			NotFound: true, Parameters: []openAPIParameter{runID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}}}},
		{Path: EvidenceAttachmentPathTemplate, OperationID: "listRunEvidence",
			Summary: "List attached non-authorizing evidence", Tag: "Runs",
			Description: "Returns a bounded metadata-only inventory of immutable Workspace evidence attachments. It omits document and Session message content, operation keys, request fingerprints, requester identities, event sequence, and capability metadata.",
			DataType:    reflect.TypeOf(EvidenceInventoryView{}), NotFound: true,
			Parameters: []openAPIParameter{runID}},
		{Path: "/api/v1/runs/{run_id}/operator-actions",
			OperationID: "listRunOperatorActions", Summary: "List pending operator actions",
			Tag:         "Runs",
			Description: "Returns one bounded Go-owned projection of pending steering, approval, file-edit review/apply, and due wake facts. Items contain opaque identities and navigation destinations only; no operation key, command, prompt, Diff, path, content, digest, or execution authority is exposed.",
			DataType:    reflect.TypeOf(OperatorActionCenterView{}), NotFound: true,
			Parameters: []openAPIParameter{runID}},
		{Path: "/api/v1/runs/{run_id}", OperationID: "getRun", Summary: "Inspect a Run", Tag: "Runs",
			Description: "Returns Run, Mission, checkpoint, tool usage, token-free execution-lease metadata, and read-only Plan/Delivery and external-Skill metadata projections when present.",
			DataType:    reflect.TypeOf(RunDetailView{}), NotFound: true, Parameters: []openAPIParameter{runID}},
		{Path: "/api/v1/runs/{run_id}/events", OperationID: "listRunEvents", Summary: "List Run events",
			Tag: "Runs", Description: "Returns the ordered append-only Run event stream.",
			DataType: reflect.TypeOf(EventView{}), Collection: true, Paginated: true, NotFound: true,
			Parameters: append([]openAPIParameter{runID}, paginationParameters()...)},
		{Path: "/api/v1/runs/{run_id}/agent-graph", OperationID: "getRunAgentGraph",
			Summary: "Inspect the bounded Agent graph", Tag: "Agents",
			Description: "Returns root and Specialist projections plus completion summaries without lease or fencing state.",
			DataType:    reflect.TypeOf(AgentGraphView{}), NotFound: true,
			Parameters: []openAPIParameter{runID}},
		{Path: "/api/v1/runs/{run_id}/external-skills", OperationID: "getRunExternalSkills",
			Summary: "Inspect bounded external Skill provenance", Tag: "Runs",
			Description: "Returns Run-pinned external Skill names, versions, trust and bounded delivery counts without package content, paths, digests, installation identities, requester identities, or operation identities.",
			DataType:    reflect.TypeOf(ExternalSkillProjectionView{}), NotFound: true,
			Parameters: []openAPIParameter{runID}},
		{Path: "/api/v1/runs/{run_id}/delegations", OperationID: "listRunDelegations",
			Summary: "List operator-gated delegations", Tag: "Agents",
			Description: "Returns proposal, review, application, and latest scheduling summaries without operation digests or review reasons.",
			DataType:    reflect.TypeOf(DelegationView{}), Collection: true, Paginated: true,
			NotFound: true, Parameters: append([]openAPIParameter{runID}, paginationParameters()...)},
		{Path: "/api/v1/runs/{run_id}/fanout-plans", OperationID: "listRunFanoutPlans",
			Summary: "List read-only Fan-out plans", Tag: "Analysis",
			Description: "Returns bounded plan metadata and the latest shard execution summary without file manifests or model report JSON.",
			DataType:    reflect.TypeOf(FanoutPlanView{}), Collection: true, Paginated: true,
			NotFound: true, Parameters: append([]openAPIParameter{runID}, paginationParameters()...)},
		{Path: "/api/v1/runs/{run_id}/reports", OperationID: "listRunFindingReports",
			Summary: "List Finding Report summaries", Tag: "Reports",
			Description: "Returns bounded report counts and severity summaries without Finding narratives.",
			DataType:    reflect.TypeOf(FindingReportSummaryView{}), Collection: true, Paginated: true,
			NotFound: true, Parameters: append([]openAPIParameter{runID}, paginationParameters()...)},
		{Path: "/api/v1/runs/{run_id}/reports/{report_id}", OperationID: "getRunFindingReport",
			Summary: "Inspect one Finding Report", Tag: "Reports",
			Description: "Returns Findings and evidence references while omitting operator reasons, Evidence notes, digests, and Artifact content.",
			DataType:    reflect.TypeOf(FindingReportView{}), NotFound: true,
			Parameters: []openAPIParameter{runID, reportID}},
		{Path: RunEventStreamPathTemplate, OperationID: "streamRunEvents", Summary: "Stream Run events",
			Tag: "Runs", Description: "Streams bounded durable Run events as SSE and resumes from a Run-bound cursor.",
			DataType: reflect.TypeOf(RunEventStreamView{}), Streaming: true, NotFound: true,
			Parameters: []openAPIParameter{
				runID,
				{Name: "cursor", In: "query", Description: "Opaque Run-bound cursor from a previous SSE id; cannot be combined with Last-Event-ID",
					Schema: map[string]any{"type": "string", "minLength": 1, "maxLength": MaxEventStreamCursorBytes}},
				{Name: "Last-Event-ID", In: "header", Description: "SSE reconnect cursor from the final received frame; cannot be combined with cursor",
					Schema: map[string]any{"type": "string", "minLength": 1, "maxLength": MaxEventStreamCursorBytes}},
			}},
		{Path: RunEventPollPathTemplate, OperationID: "pollRunEvents", Summary: "Poll Run events",
			Tag: "Runs", Description: "Returns one bounded append-only event batch using the same Run-bound high-water cursor as the SSE stream. Intended for embedded renderers without response streaming.",
			DataType: reflect.TypeOf(RunEventPollView{}), NotFound: true,
			Parameters: []openAPIParameter{runID,
				{Name: "cursor", In: "query", Description: "Opaque final cursor from a previous poll or SSE frame",
					Schema: map[string]any{"type": "string", "minLength": 1, "maxLength": MaxEventStreamCursorBytes}},
				{Name: "limit", In: "query", Description: "Maximum events returned in this bounded poll",
					Schema: map[string]any{"type": "integer", "minimum": 1, "maximum": MaxEventPollBatchSize,
						"default": DefaultEventPollBatchSize}},
			}},
		{Path: ModelCancellationPathTemplate, Method: http.MethodPost,
			OperationID: "requestModelCancellation", Summary: "Cancel an active model call", Tag: "Control",
			Description: "Persists an audit-first cancellation request bound to the exact active Supervisor and model attempt. The worker consumes it with its private execution lease; clients never provide a fencing token.",
			DataType:    reflect.TypeOf(ModelCancellationView{}), RequestType: reflect.TypeOf(ModelCancellationRequestView{}),
			Control: true, NotFound: true, Parameters: []openAPIParameter{
				runID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque operation key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string", "minLength": domain.MinModelCancellationKeyBytes,
						"maxLength": domain.MaxModelCancellationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: SpecialistModelCancellationPathTemplate, Method: http.MethodPost,
			OperationID: "requestSpecialistModelCancellation",
			Summary:     "Cancel an active Specialist model call", Tag: "Control",
			Description: "Persists an audit-first cancellation request bound to one exact Specialist AgentAttempt and model attempt. Only the worker holding that Attempt's private Run lease can observe and apply it.",
			DataType:    reflect.TypeOf(SpecialistModelCancellationView{}),
			RequestType: reflect.TypeOf(ModelCancellationRequestView{}),
			Control:     true, NotFound: true, Parameters: []openAPIParameter{
				runID, agentID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque operation key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string", "minLength": domain.MinModelCancellationKeyBytes,
						"maxLength": domain.MaxModelCancellationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: RunExecutionProfileControlPathTemplate, Method: http.MethodPost,
			OperationID: "selectRunExecutionProfile",
			Summary:     "Select a Run execution profile", Tag: "Control",
			Description: "Records operator intent for Preview, Docker, or local workspace execution. Selection never starts a process and never grants execution authority; backend-specific production gates remain mandatory.",
			DataType:    reflect.TypeOf(RunExecutionProfileControlView{}),
			RequestType: reflect.TypeOf(RunExecutionProfileControlRequestView{}),
			Control:     true, NotFound: true, Parameters: []openAPIParameter{
				runID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque operation key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: SessionSteeringCancellationPathTemplate, Method: http.MethodPost,
			OperationID: "cancelSessionSteering", Summary: "Cancel queued Session steering",
			Tag:         "Control",
			Description: "Cancels one exact pending operator-steering message for the bound Session. Prepared, committed, and already consumed input is immutable; this operation does not stop a model call or start execution.",
			DataType:    reflect.TypeOf(SessionSteeringCancellationView{}),
			RequestType: reflect.TypeOf(SessionSteeringCancellationRequestView{}),
			Control:     true, NotFound: true, Parameters: []openAPIParameter{
				sessionID, messageID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: RunLifecycleControlPathTemplate, Method: http.MethodPost,
			OperationID: "controlRunLifecycle", Summary: "Start, pause, or resume a Run",
			Tag:         "Control",
			Description: "Applies one exact idempotent Run lifecycle transition. Start atomically crosses created, preparing, and running; pause requires a quiescent Supervisor with no active execution lease; resume requires paused state. This operation never calls a model or tool.",
			DataType:    reflect.TypeOf(RunLifecycleControlView{}),
			RequestType: reflect.TypeOf(RunLifecycleControlRequestView{}),
			Control:     true, NotFound: true, Parameters: []openAPIParameter{
				runID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: PlanDirectionControlPathTemplate, Method: http.MethodPost,
			OperationID: "selectPlanDirection", Summary: "Select one Plan direction",
			Tag:         "Control",
			Description: "Selects exactly one of a persisted proposal's three directions and atomically creates its bounded WorkItems and handoff Note. It does not change phase, start execution, call a model, or grant capability.",
			DataType:    reflect.TypeOf(PlanDirectionControlView{}),
			RequestType: reflect.TypeOf(PlanDirectionControlRequestView{}),
			Control:     true, NotFound: true, Parameters: []openAPIParameter{
				runID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: PlanDeliveryControlPathTemplate, Method: http.MethodPost,
			OperationID: "enterPlanDelivery", Summary: "Enter Deliver after Plan selection",
			Tag:         "Control",
			Description: "Explicitly transitions a created or paused Run from Plan to Deliver only after an immutable operator selection exists. It does not resume the Run, start execution, call a model, or grant capability.",
			DataType:    reflect.TypeOf(PlanDeliveryTransitionControlView{}),
			RequestType: reflect.TypeOf(PlanDeliveryTransitionControlRequestView{}),
			Control:     true, NotFound: true, Parameters: []openAPIParameter{
				runID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: "/api/v1/runs/{run_id}/approvals", OperationID: "listRunApprovals",
			Summary: "List pending Run approvals", Tag: "Control",
			Description: "Returns at most one hundred pending approval metadata records and their bounded operator actions. Commands, file content, fingerprints, decision reasons, paths, capability grants, and execution authority are omitted.",
			DataType:    reflect.TypeOf(ApprovalQueueView{}), NotFound: true,
			Parameters: []openAPIParameter{runID}},
		{Path: FileEditQueuePathTemplate, OperationID: "listRunFileEdits",
			Summary: "List Run file edit previews", Tag: "Runs",
			Description: "Returns at most one hundred Run-bound metadata-only file edit previews. Original and proposed file bodies are omitted and apply authority is always false.",
			DataType:    reflect.TypeOf(FileEditQueueView{}), NotFound: true,
			Parameters: []openAPIParameter{runID}},
		{Path: FileEditProposalSourcePathTemplate,
			OperationID: "issueFileEditProposalSource", Summary: "Issue an exact interactive edit source",
			Tag:         "Runs",
			Description: "Returns a complete unredacted bounded UTF-8 file body plus a short-lived opaque handle bound to the Run, Session, Workspace, path, and current hash. Redacted or truncated files are refused; issuing the handle writes nothing.",
			DataType:    reflect.TypeOf(FileEditProposalSourceView{}), NotFound: true,
			Parameters: []openAPIParameter{runID,
				{Name: "path", In: "query", Description: "Canonical Go-projected Workspace-relative file path",
					Required: true, Schema: map[string]any{"type": "string", "minLength": 1,
						"maxLength": workspace.MaxExplorerPathRunes}},
				{Name: "expected_sha256", In: "query", Description: "Optional previously issued SHA-256 required when rotating an expired source handle",
					Schema: map[string]any{"type": "string", "pattern": `^[0-9a-f]{64}$`}}}},
		{Path: FileEditProposalRecoveryPathTemplate,
			OperationID: "recoverFileEditProposal", Summary: "Recover one durable pending FileEdit proposal",
			Tag:         "Runs",
			Description: "Returns integrity-checked original and proposed bodies for one exact pending Run-bound proposal, plus a stale-file flag. It issues no source handle and cannot mutate, approve, or apply the proposal.",
			DataType:    reflect.TypeOf(FileEditProposalRecoveryView{}), NotFound: true,
			Parameters: []openAPIParameter{runID, editID}},
		{Path: FileEditProposalPathTemplate, Method: http.MethodPost,
			OperationID: "createFileEditProposal", Summary: "Create a pending FileEdit from a Go-issued source",
			Tag:         "Control",
			Description: "Consumes only an opaque Go-issued source handle and bounded text. It rechecks the exact file hash and Policy, creates a pending approval-backed FileEdit, and cannot approve or write the file.",
			DataType:    reflect.TypeOf(FileEditProposalView{}),
			RequestType: reflect.TypeOf(FileEditProposalRequestView{}), Control: true,
			NotFound: true, Parameters: []openAPIParameter{runID}},
		{Path: "/api/v1/runs/{run_id}/file-edits/{edit_id}",
			OperationID: "getRunFileEdit", Summary: "Inspect a Run file edit preview", Tag: "Runs",
			Description: "Returns one exact Run-bound redacted diff without original or proposed file bodies.",
			DataType:    reflect.TypeOf(FileEditPreviewView{}), NotFound: true,
			Parameters: []openAPIParameter{runID, editID}},
		{Path: FileEditReviewPathTemplate, Method: http.MethodPost,
			OperationID: "reviewRunFileEdit", Summary: "Approve intent or deny a file edit",
			Tag:         "Control",
			Description: "Records an exact Run-bound review decision. Approve-intent never writes the file; applying an approved edit remains a separate non-HTTP operation.",
			DataType:    reflect.TypeOf(FileEditReviewView{}),
			RequestType: reflect.TypeOf(FileEditReviewRequestView{}), Control: true,
			NotFound: true, Parameters: []openAPIParameter{runID, editID}},
		{Path: FileEditApplyPathTemplate, Method: http.MethodPost,
			OperationID: "applyRunFileEdit", Summary: "Apply an approved file edit",
			Tag:         "Control",
			Description: "Separately applies one exact Run-bound approved edit after fresh Run, Workspace, approval, Policy, path, and current-hash checks. The renderer supplies no path or file content, and retries replay the durable operation result.",
			DataType:    reflect.TypeOf(FileEditApplyView{}),
			RequestType: reflect.TypeOf(FileEditApplyRequestView{}), Control: true,
			NotFound: true, Parameters: []openAPIParameter{runID, editID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}}}},
		{Path: RunWakeIntentPathTemplate, OperationID: "getRunWakeIntent",
			Summary: "Inspect the latest Run wake intent", Tag: "Runs",
			Description: "Returns a closed-authority wake projection without lease owner or fencing metadata.",
			DataType:    reflect.TypeOf(RunWakeStateView{}), NotFound: true,
			Parameters: []openAPIParameter{runID}},
		{Path: RunWakeIntentPathTemplate, Method: http.MethodPost,
			OperationID: "scheduleRunWake", Summary: "Schedule a bounded Run wake intent",
			Tag:         "Control",
			Description: "Persists bounded retry timing and single-owner intent for queued operator work. It does not start a background loop, model call, tool call, or Run execution.",
			DataType:    reflect.TypeOf(RunWakeControlView{}),
			RequestType: reflect.TypeOf(RunWakeScheduleRequestView{}), Control: true,
			NotFound: true, Parameters: []openAPIParameter{runID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}}}},
		{Path: RunWakeCancellationPathTemplate, Method: http.MethodPost,
			OperationID: "cancelRunWake", Summary: "Cancel an active Run wake intent",
			Tag:         "Control",
			Description: "Cancels one active wake intent and revokes any current ownership lease without starting execution.",
			DataType:    reflect.TypeOf(RunWakeControlView{}),
			RequestType: reflect.TypeOf(RunWakeCancelRequestView{}), Control: true,
			NotFound: true, Parameters: []openAPIParameter{runID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}}}},
		{Path: RunWakeExecutionPathTemplate, Method: http.MethodPost,
			OperationID: "consumeRunWake", Summary: "Consume one due Run wake intent",
			Tag:         "Control",
			Description: "Explicitly claims one due wake generation and hands its bounded queued selection to the existing Go RunSupervisor. It starts no hidden worker or background loop; retries replay the generation-fenced durable handoff.",
			DataType:    reflect.TypeOf(RunWakeExecutionView{}),
			RequestType: reflect.TypeOf(RunWakeExecutionRequestView{}), Control: true,
			NotFound: true, Parameters: []openAPIParameter{runID}},
		{Path: SkillPackageInstallPath, Method: http.MethodPost,
			OperationID: "installSkillPackage", Summary: "Install one inert Skill package",
			Tag:         "Control",
			Description: "Imports one explicitly confirmed, strictly validated, bounded archive into the content-addressed untrusted Skill Registry. Import executes no scripts, hooks, commands, tools, Provider calls, or network requests and grants no Run-selection or context-delivery authority.",
			DataType:    reflect.TypeOf(SkillPackageInstallView{}),
			RequestType: reflect.TypeOf(SkillPackageInstallRequestView{}), Control: true,
			Parameters: []openAPIParameter{
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: ApprovalDecisionControlPathTemplate, Method: http.MethodPost,
			OperationID: "decideRunApproval", Summary: "Approve once or deny a pending request",
			Tag:         "Control",
			Description: "Applies an idempotent operator decision to one exact Run-bound approval. Approve-once is limited to Policy-rechecked dry-run Shell and process-disabled ScriptProcess proposals; file edits can only be denied. It never creates a Session grant, starts a real process, writes a file, starts Docker, or grants capability.",
			DataType:    reflect.TypeOf(ApprovalDecisionControlView{}),
			RequestType: reflect.TypeOf(ApprovalDecisionControlRequestView{}),
			Control:     true, NotFound: true, Parameters: []openAPIParameter{
				runID, approvalID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: RunExecutionControlPathTemplate, Method: http.MethodPost,
			OperationID: "executeRunSelection", Summary: "Execute a bounded queued Run batch",
			Tag:         "Control",
			Description: "Freezes at most eight currently queued Session steering messages, then executes only those exact message identities through the Go RunSupervisor under one private execution lease. Retries replay the durable result and cannot consume messages appended after selection.",
			DataType:    reflect.TypeOf(RunExecutionControlView{}),
			RequestType: reflect.TypeOf(RunExecutionControlRequestView{}),
			Control:     true, NotFound: true, Parameters: []openAPIParameter{
				runID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: "/api/v1/runs/{run_id}/work-items", OperationID: "listRunWorkItems",
			Summary: "List Run WorkItems", Tag: "Memory", Description: "Returns structured Work Board items.",
			DataType: reflect.TypeOf(WorkItemView{}), Collection: true, Paginated: true, NotFound: true,
			Parameters: append(append([]openAPIParameter{runID}, paginationParameters()...),
				arrayQueryParameter("status", "Repeat or comma-separate WorkItem statuses", workItemStatusesOpenAPI(), 5),
				stringQueryParameter("owner", "Exact legacy WorkItem owner-label filter", nil),
				identityQueryParameter("owner_agent_id", "Exact WorkItem owner Agent identity"))},
		{Path: "/api/v1/runs/{run_id}/notes", OperationID: "listRunNotes", Summary: "List Run Notes",
			Tag: "Memory", Description: "Returns structured, redacted Run Notes.", DataType: reflect.TypeOf(NoteView{}),
			Collection: true, Paginated: true, NotFound: true,
			Parameters: append(append([]openAPIParameter{runID}, paginationParameters()...),
				arrayQueryParameter("status", "Repeat or comma-separate Note statuses", noteStatusesOpenAPI(), 2),
				arrayQueryParameter("category", "Repeat or comma-separate Note categories", noteCategoriesOpenAPI(), 5),
				arrayQueryParameter("visibility", "Repeat or comma-separate Note visibility values", noteVisibilitiesOpenAPI(), 3),
				stringQueryParameter("owner", "Exact legacy Note owner-label filter", nil),
				identityQueryParameter("owner_agent_id", "Exact Note owner Agent identity"),
				arrayQueryParameter("tag", "Repeat or comma-separate normalized Note tags", nil, domain.MaxNoteTags),
				booleanQueryParameter("pinned", "Filter by pinned state"))},
		{Path: "/api/v1/runs/{run_id}/artifacts", OperationID: "listRunArtifacts",
			Summary: "List Run Artifact descriptors", Tag: "Artifacts",
			Description: "Returns metadata and hashes only; Artifact content is never returned by HTTP.",
			DataType:    reflect.TypeOf(ArtifactView{}), Collection: true, Paginated: true, NotFound: true,
			Parameters: append(append([]openAPIParameter{runID}, paginationParameters()...),
				identityQueryParameter("source_id", "Exact source proposal or invocation identity"),
				stringQueryParameter("stream", "Artifact output stream", artifactStreams()))},
		{Path: "/api/v1/runs/{run_id}/tool-rounds", OperationID: "listRunToolRounds",
			Summary: "List Supervisor tool rounds", Tag: "Runs",
			Description: "Returns persisted redacted structured-memory tool rounds and calls.",
			DataType:    reflect.TypeOf(SupervisorToolRoundView{}), Collection: true, Paginated: true,
			NotFound: true, Parameters: append([]openAPIParameter{runID}, paginationParameters()...)},
		{Path: "/api/v1/sessions", OperationID: "listSessions", Summary: "List Sessions", Tag: "Sessions",
			Description: "Returns a bounded cursor page of persisted Sessions.", DataType: reflect.TypeOf(SessionView{}),
			Collection: true, Paginated: true, Parameters: paginationParameters()},
		{Path: "/api/v1/sessions/{session_id}", OperationID: "getSession", Summary: "Inspect a Session",
			Tag: "Sessions", Description: "Returns Session metadata and its optional bound Run.",
			DataType: reflect.TypeOf(SessionDetailView{}), NotFound: true, Parameters: []openAPIParameter{sessionID}},
		{Path: "/api/v1/sessions/{session_id}/messages", OperationID: "listSessionMessages",
			Summary: "List Session messages", Tag: "Sessions", Description: "Returns persisted redacted messages.",
			DataType: reflect.TypeOf(MessageView{}), Collection: true, Paginated: true, NotFound: true,
			Parameters: append(append([]openAPIParameter{sessionID}, paginationParameters()...),
				booleanQueryParameter("include_compacted", "Include compacted historical messages"))},
		{Path: SessionMessageControlPathTemplate, Method: http.MethodPost,
			OperationID: "submitSessionMessage", Summary: "Submit a Run-bound Session message",
			Tag:         "Control",
			Description: "Creates or replays one redacted durable operator-steering record for the exact Run-bound Session. It does not append Session history early, start or resume the Run, acquire a lease, call a model or tool, or grant a capability.",
			DataType:    reflect.TypeOf(SessionMessageControlView{}),
			RequestType: reflect.TypeOf(SessionMessageControlRequestView{}),
			Control:     true, NotFound: true,
			Parameters: []openAPIParameter{
				sessionID,
				{Name: "Idempotency-Key", In: "header", Description: "Opaque retry key; only a domain-separated digest is persisted",
					Required: true, Schema: map[string]any{"type": "string",
						"minLength": domain.MinAgentOperationKeyBytes,
						"maxLength": domain.MaxAgentOperationKeyBytes, "pattern": `^\S+$`}},
			}},
		{Path: "/api/v1/work-items/{work_item_id}", OperationID: "getWorkItem", Summary: "Inspect a WorkItem",
			Tag: "Memory", Description: "Returns one structured WorkItem.", DataType: reflect.TypeOf(WorkItemView{}),
			NotFound: true, Parameters: []openAPIParameter{workItemID}},
		{Path: "/api/v1/notes/{note_id}", OperationID: "getNote", Summary: "Inspect a Note", Tag: "Memory",
			Description: "Returns one redacted structured Note.", DataType: reflect.TypeOf(NoteView{}),
			NotFound: true, Parameters: []openAPIParameter{noteID}},
		{Path: "/api/v1/artifacts/{artifact_id}", OperationID: "getArtifact", Summary: "Inspect an Artifact descriptor",
			Tag: "Artifacts", Description: "Returns content-free Artifact metadata and its integrity hash.",
			DataType: reflect.TypeOf(ArtifactView{}), NotFound: true, Parameters: []openAPIParameter{artifactID}},
	}
}

func buildOpenAPIOperation(spec openAPIOperationSpec, registry *openAPISchemaRegistry) (openAPIOperation, error) {
	responses := standardOperationResponses(spec.NotFound, spec.Control)
	successStatus := "200"
	successDescription := "Successful read"
	if spec.Control {
		successStatus = "202"
		successDescription = "Control request accepted or idempotently replayed"
	}
	if spec.Streaming {
		responses[successStatus] = openAPIResponse{Description: "Bounded Server-Sent Event stream", Content: map[string]openAPIMediaType{
			"text/event-stream": {Schema: registry.ref(spec.DataType)},
		}}
	} else if spec.RawDocument {
		responses[successStatus] = openAPIResponse{Description: "OpenAPI 3.1 document", Content: map[string]openAPIMediaType{
			openAPIContentType: {Schema: map[string]any{"type": "object", "additionalProperties": true}},
		}}
	} else {
		if spec.DataType == nil {
			return openAPIOperation{}, fmt.Errorf("OpenAPI path %q has no response DTO", spec.Path)
		}
		dataSchema := registry.ref(spec.DataType)
		if spec.Collection {
			dataSchema = map[string]any{"type": "array", "items": dataSchema}
		}
		responses[successStatus] = openAPIResponse{Description: successDescription, Content: map[string]openAPIMediaType{
			"application/json": {Schema: successEnvelopeSchema(dataSchema, spec.Paginated, registry)},
		}}
	}
	operation := openAPIOperation{OperationID: spec.OperationID, Summary: spec.Summary,
		Description: spec.Description, Tags: []string{spec.Tag}, Parameters: spec.Parameters,
		Responses: responses, ReadOnly: !spec.Control, Streaming: spec.Streaming}
	if spec.Control {
		if spec.RequestType == nil {
			return openAPIOperation{}, fmt.Errorf("OpenAPI control path %q has no request DTO", spec.Path)
		}
		operation.Security = []map[string][]string{{"ControlBearerAuth": {}}}
		operation.RequestBody = &openAPIRequestBody{Required: true, Content: map[string]openAPIMediaType{
			"application/json": {Schema: registry.ref(spec.RequestType)},
		}}
	}
	return operation, nil
}

func successEnvelopeSchema(dataSchema map[string]any, paginated bool, registry *openAPISchemaRegistry) map[string]any {
	properties := map[string]any{
		"version":    map[string]any{"type": "string", "const": Version},
		"request_id": map[string]any{"type": "string", "minLength": 1, "maxLength": 256},
		"data":       dataSchema,
	}
	required := []string{"version", "request_id", "data"}
	if paginated {
		properties["page"] = registry.refNamed("Page", reflect.TypeOf(Page{}))
		required = append(required, "page")
	}
	return map[string]any{"type": "object", "additionalProperties": false,
		"properties": properties, "required": required}
}

func standardOperationResponses(notFound bool, control bool) map[string]any {
	responses := map[string]any{
		"400": responseReference("BadRequest"),
		"401": responseReference("Unauthorized"),
		"403": responseReference("Forbidden"),
		"414": responseReference("RequestTooLarge"),
		"429": responseReference("ResourceExhausted"),
		"500": responseReference("InternalError"),
	}
	if notFound {
		responses["404"] = responseReference("NotFound")
	}
	if control {
		responses["409"] = responseReference("Conflict")
		responses["412"] = responseReference("FailedPrecondition")
		responses["413"] = responseReference("RequestEntityTooLarge")
		responses["415"] = responseReference("UnsupportedMediaType")
	}
	return responses
}

func responseReference(name string) map[string]any {
	return map[string]any{"$ref": "#/components/responses/" + name}
}

func standardOpenAPIErrorResponses(registry *openAPISchemaRegistry) map[string]openAPIResponse {
	schema := registry.refNamed("ErrorEnvelope", reflect.TypeOf(errorEnvelope{}))
	makeResponse := func(description string) openAPIResponse {
		return openAPIResponse{Description: description,
			Content: map[string]openAPIMediaType{"application/json": {Schema: schema}}}
	}
	return map[string]openAPIResponse{
		"BadRequest":            makeResponse("Invalid path, query, method, or request body"),
		"Unauthorized":          makeResponse("Missing or invalid bearer token"),
		"Forbidden":             makeResponse("Request is outside the loopback security boundary"),
		"NotFound":              makeResponse("Requested durable resource was not found"),
		"Conflict":              makeResponse("Idempotency key, Run, or active attempt changed"),
		"FailedPrecondition":    makeResponse("The target model attempt is not active or cancellable"),
		"RequestEntityTooLarge": makeResponse("Control request body exceeded its hard limit"),
		"UnsupportedMediaType":  makeResponse("Control request must use application/json"),
		"RequestTooLarge":       makeResponse("Request target or query exceeded its hard limit"),
		"ResourceExhausted":     makeResponse("Bounded response or resource limit was exhausted"),
		"InternalError":         makeResponse("Redacted internal server failure"),
	}
}

func paginationParameters() []openAPIParameter {
	return []openAPIParameter{
		{Name: "limit", In: "query", Description: "Page size from 1 to 100; defaults to 50",
			Schema: map[string]any{"type": "integer", "minimum": 1, "maximum": MaxPageLimit, "default": DefaultPageLimit}},
		{Name: "cursor", In: "query", Description: "Opaque cursor bound to this route and exact filter set",
			Schema: map[string]any{"type": "string", "minLength": 1, "maxLength": MaxCursorBytes}},
	}
}

func pathIdentityParameter(name string, description string) openAPIParameter {
	return openAPIParameter{Name: name, In: "path", Description: description, Required: true,
		Schema: identitySchema()}
}

func identityQueryParameter(name string, description string) openAPIParameter {
	return openAPIParameter{Name: name, In: "query", Description: description, Schema: identitySchema()}
}

func identitySchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": 256,
		"pattern": `^[^/\\\x00-\x1f\x7f]+$`}
}

func stringQueryParameter(name string, description string, enum []string) openAPIParameter {
	schema := map[string]any{"type": "string", "minLength": 1}
	if len(enum) != 0 {
		schema["enum"] = enum
	}
	return openAPIParameter{Name: name, In: "query", Description: description, Schema: schema}
}

func arrayQueryParameter(name string, description string, enum []string, maxItems int) openAPIParameter {
	items := map[string]any{"type": "string", "minLength": 1}
	if len(enum) != 0 {
		items["enum"] = enum
	}
	explode := false
	return openAPIParameter{Name: name, In: "query", Description: description, Style: "form", Explode: &explode,
		Schema: map[string]any{"type": "array", "items": items, "minItems": 1, "maxItems": maxItems,
			"uniqueItems": true}}
}

func booleanQueryParameter(name string, description string) openAPIParameter {
	return openAPIParameter{Name: name, In: "query", Description: description,
		Schema: map[string]any{"type": "boolean"}}
}

type openAPISchemaRegistry struct {
	schemas map[string]map[string]any
	names   map[reflect.Type]string
	types   map[string]reflect.Type
	err     error
}

func newOpenAPISchemaRegistry() *openAPISchemaRegistry {
	registry := &openAPISchemaRegistry{schemas: map[string]map[string]any{},
		names: map[reflect.Type]string{}, types: map[string]reflect.Type{}}
	registry.refNamed("Page", reflect.TypeOf(Page{}))
	registry.refNamed("APIError", reflect.TypeOf(apiErrorView{}))
	registry.refNamed("ErrorEnvelope", reflect.TypeOf(errorEnvelope{}))
	return registry
}

func (r *openAPISchemaRegistry) ref(valueType reflect.Type) map[string]any {
	for valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	if valueType == reflect.TypeOf(time.Time{}) {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if valueType == reflect.TypeOf(json.RawMessage{}) {
		return map[string]any{}
	}
	if valueType.Kind() != reflect.Struct {
		return r.schema(valueType)
	}
	return r.refNamed(valueType.Name(), valueType)
}

func (r *openAPISchemaRegistry) refNamed(name string, valueType reflect.Type) map[string]any {
	for valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	if existing, ok := r.names[valueType]; ok {
		return map[string]any{"$ref": "#/components/schemas/" + existing}
	}
	if previous, ok := r.types[name]; ok && previous != valueType {
		r.err = fmt.Errorf("OpenAPI schema name %q is shared by %s and %s", name, previous, valueType)
		return map[string]any{}
	}
	r.names[valueType] = name
	r.types[name] = valueType
	r.schemas[name] = map[string]any{}
	r.schemas[name] = r.objectSchema(valueType)
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func (r *openAPISchemaRegistry) schema(valueType reflect.Type) map[string]any {
	for valueType.Kind() == reflect.Pointer {
		valueType = valueType.Elem()
	}
	if valueType == reflect.TypeOf(time.Time{}) {
		return map[string]any{"type": "string", "format": "date-time"}
	}
	if valueType == reflect.TypeOf(json.RawMessage{}) {
		return map[string]any{}
	}
	switch valueType.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return map[string]any{"type": "integer", "format": "int32"}
	case reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer", "format": "int64"}
	case reflect.Float32:
		return map[string]any{"type": "number", "format": "float"}
	case reflect.Float64:
		return map[string]any{"type": "number", "format": "double"}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": r.ref(valueType.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": r.ref(valueType.Elem())}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		return r.ref(valueType)
	default:
		r.err = fmt.Errorf("unsupported OpenAPI DTO type %s", valueType)
		return map[string]any{}
	}
}

func (r *openAPISchemaRegistry) objectSchema(valueType reflect.Type) map[string]any {
	properties := make(map[string]any)
	required := make([]string, 0, valueType.NumField())
	for index := 0; index < valueType.NumField(); index++ {
		field := valueType.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name, omitEmpty, skip := jsonField(field)
		if skip {
			continue
		}
		property := r.ref(field.Type)
		applyOpenAPIFieldMetadata(valueType.Name(), name, property)
		properties[name] = property
		if !omitEmpty {
			required = append(required, name)
		}
	}
	schema := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
	if len(required) != 0 {
		schema["required"] = required
	}
	return schema
}

func jsonField(field reflect.StructField) (string, bool, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = field.Name
	}
	omitEmpty := false
	for _, option := range parts[1:] {
		omitEmpty = omitEmpty || option == "omitempty"
	}
	return name, omitEmpty, false
}

func applyOpenAPIFieldMetadata(typeName string, fieldName string, schema map[string]any) {
	if values := openAPIFieldEnums[typeName+"."+fieldName]; len(values) != 0 {
		schema["enum"] = values
	}
	if minimum, ok := openAPIFieldMinimums[typeName+"."+fieldName]; ok {
		schema["minimum"] = minimum
	}
	if maximum, ok := openAPIFieldMaximums[typeName+"."+fieldName]; ok {
		schema["maximum"] = maximum
	}
	if maximum, ok := openAPIFieldMaxLengths[typeName+"."+fieldName]; ok {
		schema["maxLength"] = maximum
	}
	if strings.HasSuffix(fieldName, "_id") || fieldName == "id" || fieldName == "event_id" {
		if _, ref := schema["$ref"]; !ref {
			schema["minLength"] = 1
			schema["maxLength"] = 256
		}
	}
	if typeName == "ArtifactView" && fieldName == "sha256" {
		schema["pattern"] = "^[a-f0-9]{64}$"
	}
	if fieldName == "content_sha256" {
		schema["pattern"] = "^[a-f0-9]{64}$"
	}
	if typeName == "WorkspaceExplorerView" && fieldName == "entries" {
		schema["maxItems"] = workspace.MaxExplorerEntries
	}
	if typeName == "WorkspaceSearchView" && fieldName == "results" {
		schema["maxItems"] = workspace.MaxSearchResults
	}
	if typeName == "OperationReceiptHistoryView" && fieldName == "items" {
		schema["maxItems"] = operationreceipt.MaxHistoryItems
	}
	if typeName == "OperatorActionCenterView" && fieldName == "items" {
		schema["maxItems"] = operatoraction.MaxItems
	}
	if typeName == "EvidenceInventoryView" && fieldName == "items" {
		schema["maxItems"] = session.MaxEvidenceInventoryItems
	}
	if typeName == "ProviderCredentialListView" && fieldName == "items" {
		schema["maxItems"] = 3
	}
	if typeName == "ProviderCredentialRequestView" && fieldName == "secret" {
		schema["writeOnly"] = true
	}
	if typeName == "FileEditProposalSourceView" && fieldName == "source_handle" {
		schema["minLength"] = 43
		schema["maxLength"] = 43
		schema["pattern"] = "^[A-Za-z0-9_-]{43}$"
	}
	if typeName == "FileEditProposalRecoveryView" &&
		(fieldName == "original_content" || fieldName == "proposed_content") {
		schema["maxLength"] = fileedit.MaxContentBytes
	}
	if typeName == "FileEditProposalRecoveryView" &&
		(fieldName == "original_sha256" || fieldName == "proposed_sha256") {
		schema["pattern"] = "^[0-9a-f]{64}$"
	}
	if typeName == "FileEditProposalRecoveryView" && fieldName == "original_sha256" {
		schema["pattern"] = "^([0-9a-f]{64}|missing)$"
	}
	if typeName == "FileEditProposalRecoveryView" && fieldName == "current_content_sha256" {
		schema["pattern"] = "^([0-9a-f]{64}|missing)$"
	}
	if typeName == "ModelAvailabilityView" && fieldName == "generation" {
		schema["minimum"] = 1
	}
	if typeName == "RunWakeWorkerHealthView" && fieldName == "concurrency" {
		schema["minimum"] = application.RunWakeWorkerConcurrency
		schema["maximum"] = application.RunWakeWorkerConcurrency
	}
	if typeName == "RunWakeWorkerHealthView" && fieldName == "max_steps" {
		schema["minimum"] = application.RunWakeWorkerMaxSteps
		schema["maximum"] = application.RunWakeWorkerMaxSteps
	}
}

var openAPIFieldEnums = map[string][]string{
	"EventView.version":                                 {events.EnvelopeVersion},
	"IndexView.api_version":                             {Version},
	"HealthView.status":                                 {"ok"},
	"HealthView.api_version":                            {Version},
	"RuntimeCapabilitiesView.protocol_version":          {RuntimeCapabilitiesProtocolVersion},
	"RunWakeWorkerHealthView.protocol_version":          {application.RunWakeWorkerHealthProtocolVersion},
	"RunWakeWorkerHealthView.state":                     {"disabled", string(application.RunWakeWorkerReady), string(application.RunWakeWorkerRunning), string(application.RunWakeWorkerDraining), string(application.RunWakeWorkerStopped)},
	"ModelAvailabilityView.protocol_version":            {modelregistry.ProtocolVersion},
	"ProviderDiagnosticRequestView.version":             {modelregistry.DiagnosticProtocolVersion},
	"ProviderDiagnosticView.protocol_version":           {modelregistry.DiagnosticProtocolVersion},
	"ProviderDiagnosticView.status":                     {modelregistry.DiagnosticReachable, modelregistry.DiagnosticUnreachable},
	"ProviderDiagnosticView.outcome":                    {string(llm.OutcomeSuccess), string(llm.OutcomeRetryable), string(llm.OutcomeRateLimited), string(llm.OutcomeInvalidResponse), string(llm.OutcomeCancelled), string(llm.OutcomePermanent)},
	"ModelRouteControlRequestView.version":              {modelregistry.RouteControlProtocolVersion},
	"ProviderCredentialListView.protocol_version":       {credential.ProtocolVersion},
	"ProviderCredentialStatusView.protocol_version":     {credential.ProtocolVersion},
	"ProviderCredentialRequestView.version":             {credential.ProtocolVersion},
	"ProviderCredentialRequestView.action":              {string(application.ProviderCredentialSet), string(application.ProviderCredentialDelete)},
	"FileEditProposalSourceView.protocol_version":       {application.FileEditProposalProtocolVersion},
	"FileEditProposalRequestView.version":               {application.FileEditProposalProtocolVersion},
	"FileEditProposalView.protocol_version":             {application.FileEditProposalProtocolVersion},
	"FileEditProposalRecoveryView.protocol_version":     {application.FileEditRecoveryProtocolVersion},
	"FileEditProposalRecoveryView.status":               {fileedit.StatusProposed},
	"FileEditQueueView.protocol_version":                {application.FileEditReviewProtocolVersion},
	"FileEditReviewRequestView.version":                 {application.FileEditReviewProtocolVersion},
	"FileEditReviewRequestView.action":                  {string(application.FileEditApproveIntent), string(application.FileEditDeny)},
	"FileEditReviewView.protocol_version":               {application.FileEditReviewProtocolVersion},
	"FileEditReviewView.action":                         {string(application.FileEditApproveIntent), string(application.FileEditDeny)},
	"FileEditPreviewView.status":                        {fileedit.StatusProposed, fileedit.StatusApproved, fileedit.StatusApplied, fileedit.StatusDenied, fileedit.StatusFailed},
	"FileEditApplyRequestView.version":                  {fileedit.FileEditApplyProtocolVersion},
	"FileEditApplyView.protocol_version":                {fileedit.FileEditApplyProtocolVersion},
	"FileEditApplyView.status":                          {string(fileedit.ApplyCompleted), string(fileedit.ApplyFailed)},
	"RunWakeScheduleRequestView.version":                {domain.RunWakeControlProtocolVersion},
	"RunWakeCancelRequestView.version":                  {domain.RunWakeControlProtocolVersion},
	"RunWakeStateView.protocol_version":                 {domain.RunWakeIntentProtocolVersion},
	"RunWakeIntentView.protocol_version":                {domain.RunWakeIntentProtocolVersion},
	"RunWakeIntentView.status":                          {string(domain.RunWakeQueued), string(domain.RunWakeLeased), string(domain.RunWakeCompleted), string(domain.RunWakeCancelled), string(domain.RunWakeExhausted)},
	"RunWakeControlView.protocol_version":               {domain.RunWakeControlProtocolVersion},
	"RunWakeControlView.action":                         {string(domain.RunWakeSchedule), string(domain.RunWakeCancel)},
	"RunWakeExecutionRequestView.version":               {domain.RunWakeConsumerProtocolVersion},
	"RunWakeExecutionView.protocol_version":             {domain.RunWakeConsumerProtocolVersion},
	"RunWakeExecutionView.consumption_status":           {string(domain.RunWakeConsumptionPrepared), string(domain.RunWakeConsumptionCompleted), string(domain.RunWakeConsumptionFailed)},
	"SkillPackageInstallRequestView.version":            {skills.PackageInstallationProtocolVersion},
	"SkillPackageInstallRequestView.surface":            {string(domain.ExecutionSurfaceCode), string(domain.ExecutionSurfaceCyber)},
	"SkillPackageInstallView.protocol_version":          {skills.PackageInstallationProtocolVersion},
	"SkillPackageInstallView.surface":                   {string(domain.ExecutionSurfaceCode), string(domain.ExecutionSurfaceCyber)},
	"SkillPackageInstallView.trust_class":               {string(skills.PackageTrustOperatorInstalledUntrusted)},
	"OperationReceiptView.protocol_version":             {operationreceipt.ProtocolVersion},
	"OperationReceiptView.kind":                         {string(operationreceipt.KindFileEditApply), string(operationreceipt.KindRunWakeConsume), string(operationreceipt.KindSkillPackageInstall)},
	"OperationReceiptView.outcome":                      {"applied", "failed", "completed", "installed"},
	"OperationReceiptView.retry_strategy":               {string(operationreceipt.RetrySameOperationKey), string(operationreceipt.RetrySameWakeGeneration)},
	"OperationReceiptView.recovery_action":              {string(operationreceipt.RecoveryNone), string(operationreceipt.RecoveryRetryAfterGrace)},
	"OperationReceiptView.cleanup_state":                {string(operationreceipt.CleanupNotApplicable), string(operationreceipt.CleanupComplete), string(operationreceipt.CleanupPendingReview)},
	"OperationReceiptHistoryView.protocol_version":      {operationreceipt.HistoryProtocolVersion},
	"OperationReceiptHistoryItemView.scope":             {"run", "skill_registry"},
	"WorkspaceExplorerView.protocol_version":            {workspace.ExplorerProtocolVersion},
	"WorkspaceExplorerView.kind":                        {"directory", "file"},
	"WorkspaceExplorerEntryView.kind":                   {"directory", "file", "blocked"},
	"WorkspaceExplorerProvenanceView.version":           {session.ContextProvenanceVersion},
	"WorkspaceExplorerProvenanceView.source_kind":       {session.SourceWorkspaceFile, session.SourceWorkspaceList},
	"WorkspaceSearchView.protocol_version":              {workspace.SearchProtocolVersion},
	"WorkspaceSearchResultView.match_kind":              {"filename", "content", "filename_and_content"},
	"EvidenceAttachmentRequestView.version":             {session.EvidenceAttachmentProtocolVersion},
	"EvidenceAttachmentRequestView.source_kind":         {session.SourceWorkspaceFile},
	"EvidenceAttachmentView.protocol_version":           {session.EvidenceAttachmentProtocolVersion},
	"EvidenceAttachmentView.source_kind":                {session.SourceWorkspaceFile},
	"EvidenceInventoryView.protocol_version":            {session.EvidenceInventoryProtocolVersion},
	"EvidenceInventoryItemView.source_kind":             {session.SourceWorkspaceFile},
	"OperatorActionCenterView.protocol_version":         {operatoraction.ProtocolVersion},
	"OperatorActionItemView.kind":                       {string(operatoraction.KindSteeringPending), string(operatoraction.KindApprovalPending), string(operatoraction.KindFileEditReview), string(operatoraction.KindFileEditApply), string(operatoraction.KindWakeDue)},
	"OperatorActionItemView.state":                      {"pending", "proposed", "approved", "queued"},
	"OperatorActionItemView.destination":                {string(operatoraction.DestinationQueue), string(operatoraction.DestinationApprovals), string(operatoraction.DestinationDiffs), string(operatoraction.DestinationWake)},
	"ProviderAvailabilityView.kind":                     {modelregistry.ProviderKindLocal, modelregistry.ProviderKindAnthropicCompatible},
	"ProviderAvailabilityView.status":                   {modelregistry.ProviderAvailable, modelregistry.ProviderNotConfigured, modelregistry.ProviderInvalidConfiguration},
	"ProviderAvailabilityView.credential_source":        {"none", "environment", "system"},
	"ScopeView.network_mode":                            {"disabled", "allowlist"},
	"MissionView.profile":                               {"code", "review", "learn", "script"},
	"RunView.status":                                    runStatuses(),
	"RunModeView.protocol_version":                      {domain.RunModeProtocolVersion},
	"RunModeView.surface":                               {string(domain.ExecutionSurfaceCode), string(domain.ExecutionSurfaceCyber)},
	"RunModeView.phase":                                 {string(domain.ExecutionPhasePlan), string(domain.ExecutionPhaseDeliver)},
	"RunModeView.profile":                               {"code", "review", "learn", "script"},
	"RunModeView.policy_version":                        {domain.RunModePolicyVersion},
	"RunExecutionProfileView.protocol_version":          {domain.RunExecutionProfileProtocolVersion},
	"RunExecutionProfileView.profile":                   {string(domain.RunExecutionProfilePreview), string(domain.RunExecutionProfileDocker), string(domain.RunExecutionProfileLocal)},
	"RunExecutionProfileView.backend":                   {string(domain.ExecutionBackendNoop), string(domain.ExecutionBackendDocker), string(domain.ExecutionBackendLocal)},
	"RunExecutionProfileView.approval_policy":           {string(domain.ExecutionApprovalNone), string(domain.ExecutionApprovalAlways)},
	"RunExecutionProfileView.filesystem_scope":          {string(domain.ExecutionFilesystemNone), string(domain.ExecutionFilesystemWorkspace)},
	"RunExecutionProfileView.network_scope":             {string(domain.ExecutionNetworkDisabled)},
	"RunExecutionProfileView.risk_tier":                 {string(domain.ExecutionRiskMinimal), string(domain.ExecutionRiskElevated), string(domain.ExecutionRiskHigh)},
	"RunExecutionProfileView.required_gate":             {string(domain.ExecutionGateNone), string(domain.ExecutionGateDockerProductionStart), string(domain.ExecutionGateLocalOSSandbox)},
	"RunExecutionProfileView.policy_version":            {domain.RunExecutionProfilePolicyVersion},
	"RunExecutionProfileControlRequestView.profile":     {string(domain.RunExecutionProfilePreview), string(domain.RunExecutionProfileDocker), string(domain.RunExecutionProfileLocal)},
	"RunCreationControlRequestView.version":             {domain.RunCreationProtocolVersion},
	"RunCreationControlRequestView.profile":             {string(domain.ProfileCode), string(domain.ProfileReview), string(domain.ProfileLearn), string(domain.ProfileScript)},
	"RunCreationControlRequestView.surface":             {string(domain.ExecutionSurfaceCode), string(domain.ExecutionSurfaceCyber)},
	"RunCreationControlRequestView.phase":               {string(domain.ExecutionPhasePlan), string(domain.ExecutionPhaseDeliver)},
	"SessionMessageControlRequestView.version":          {domain.SessionMessageSubmissionProtocolVersion},
	"SessionMessageControlView.version":                 {domain.SessionMessageSubmissionProtocolVersion},
	"SessionSteeringCancellationRequestView.version":    {domain.SessionSteeringCancellationProtocolVersion},
	"SessionSteeringCancellationView.version":           {domain.SessionSteeringCancellationProtocolVersion},
	"SessionSteeringCancellationView.cancellation_kind": {string(domain.OperatorSteeringCancellationOperator)},
	"RunLifecycleControlRequestView.version":            {domain.RunLifecycleControlProtocolVersion},
	"RunLifecycleControlRequestView.action":             {string(domain.RunLifecycleStart), string(domain.RunLifecyclePause), string(domain.RunLifecycleResume)},
	"RunLifecycleControlView.version":                   {domain.RunLifecycleControlProtocolVersion},
	"RunLifecycleControlView.action":                    {string(domain.RunLifecycleStart), string(domain.RunLifecyclePause), string(domain.RunLifecycleResume)},
	"RunLifecycleControlView.expected_status":           {string(domain.RunCreated), string(domain.RunRunning), string(domain.RunPaused)},
	"RunLifecycleControlView.applied_status":            {string(domain.RunRunning), string(domain.RunPaused)},
	"RunExecutionControlRequestView.version":            {domain.RunExecutionHandoffProtocolVersion},
	"RunExecutionControlView.version":                   {domain.RunExecutionHandoffProtocolVersion},
	"RunExecutionControlView.status":                    {string(domain.RunExecutionHandoffCompleted), string(domain.RunExecutionHandoffFailed)},
	"RunExecutionControlView.run_status":                runStatuses(),
	"PlanDirectionControlRequestView.version":           {application.PlanDeliveryControlProtocolVersion},
	"PlanDirectionControlView.version":                  {application.PlanDeliveryControlProtocolVersion},
	"PlanDeliveryTransitionControlRequestView.version":  {application.PlanDeliveryControlProtocolVersion},
	"PlanDeliveryTransitionControlView.version":         {application.PlanDeliveryControlProtocolVersion},
	"ApprovalQueueView.protocol_version":                {application.ApprovalQueueProtocolVersion},
	"ApprovalQueueItemView.status":                      {string(approval.StatusPending)},
	"ApprovalDecisionControlRequestView.version":        {application.ApprovalControlProtocolVersion},
	"ApprovalDecisionControlRequestView.action":         {string(application.ApprovalControlApproveOnce), string(application.ApprovalControlDeny)},
	"ApprovalDecisionControlView.version":               {application.ApprovalControlProtocolVersion},
	"ApprovalDecisionControlView.action":                {string(application.ApprovalControlApproveOnce), string(application.ApprovalControlDeny)},
	"ApprovalDecisionControlView.status":                {string(approval.StatusApproved), string(approval.StatusDenied)},
	"SupervisorCheckpointView.phase":                    {"idle", "turn_started", "turn_failed", "waiting", "run_completed", "run_failed"},
	"SupervisorCheckpointView.repair_phase":             {"pending", "exhausted"},
	"RunExecutionLeaseView.status":                      {string(domain.RunExecutionLeaseActive), string(domain.RunExecutionLeaseReleased)},
	"OperatorSteeringMessageView.status":                {string(domain.OperatorSteeringPending), string(domain.OperatorSteeringCommitted), string(domain.OperatorSteeringCancelled)},
	"PlanDeliveryProposalView.protocol_version":         {domain.PlanDeliveryProtocolVersion},
	"PlanDeliveryProposalView.status":                   {string(domain.PlanDeliveryProposalProposed)},
	"SessionView.status":                                {"active", "archived"},
	"WorkItemView.status":                               workItemStatusesOpenAPI(),
	"WorkItemView.priority":                             {"low", "normal", "high", "critical"},
	"NoteView.category":                                 noteCategoriesOpenAPI(),
	"NoteView.visibility":                               noteVisibilitiesOpenAPI(),
	"NoteView.status":                                   noteStatusesOpenAPI(),
	"ArtifactView.stream":                               artifactStreams(),
	"ArtifactView.encoding":                             {artifact.EncodingUTF8},
	"SupervisorToolCallView.status":                     {"pending", "completed", "denied", "failed"},
	"RunEventStreamView.version":                        {RunEventStreamVersion},
	"RunEventPollView.version":                          {RunEventPollVersion},
	"ModelCancellationView.status":                      {string(domain.ModelCancellationPending), string(domain.ModelCancellationObserved), string(domain.ModelCancellationResolved)},
	"SpecialistModelCancellationView.status":            {string(domain.ModelCancellationPending), string(domain.ModelCancellationObserved), string(domain.ModelCancellationResolved)},
	"AgentGraphView.protocol_version":                   {domain.AgentGraphProtocolVersion},
	"ExternalSkillProjectionView.protocol_version":      {skills.ExternalSkillProjectionProtocolVersion},
	"ExternalSkillProjectionView.surface":               {string(domain.ExecutionSurfaceCode), string(domain.ExecutionSurfaceCyber)},
	"ExternalSkillProjectionView.profile":               {string(domain.ProfileCode), string(domain.ProfileReview), string(domain.ProfileLearn), string(domain.ProfileScript)},
	"ExternalSkillProjectionItemView.trust_class":       {string(skills.PackageTrustOperatorInstalledUntrusted)},
	"AgentNodeView.role":                                {string(domain.AgentRoleRoot), string(domain.AgentRoleSpecialist)},
	"AgentNodeView.status":                              {string(domain.AgentReady), string(domain.AgentRunning), string(domain.AgentWaiting), string(domain.AgentCompleted), string(domain.AgentFailed), string(domain.AgentCancelled)},
	"DelegationReviewView.decision":                     {string(domain.SpecialistDelegationApproved), string(domain.SpecialistDelegationRejected)},
	"FanoutPlanView.status":                             {string(domain.ReadOnlyFanoutPlanned)},
	"FanoutExecutionView.status":                        {string(domain.ReadOnlyFanoutExecutionRunning), string(domain.ReadOnlyFanoutExecutionCompleted), string(domain.ReadOnlyFanoutExecutionFailed), string(domain.ReadOnlyFanoutExecutionCancelled)},
	"FindingReportSummaryView.status":                   {string(domain.FindingReportGenerated)},
	"FindingView.severity":                              {string(domain.FindingSeverityInfo), string(domain.FindingSeverityLow), string(domain.FindingSeverityMedium), string(domain.FindingSeverityHigh), string(domain.FindingSeverityCritical)},
	"FindingView.status":                                {string(domain.FindingStatusDraft), string(domain.FindingStatusValidated), string(domain.FindingStatusAccepted), string(domain.FindingStatusFixed), string(domain.FindingStatusRejected)},
	"MessageView.role":                                  {"user", "assistant", "system", "tool"},
	"MessageView.provenance_version":                    {session.LegacyContextProvenanceVersion, session.ContextProvenanceVersion},
	"MessageView.source_kind":                           {session.SourceOperatorMessage, session.SourceModelResponse, session.SourceGoControl, session.SourceWorkspaceFile, session.SourceWorkspaceList, session.SourceWorkspaceDiff, session.SourceToolResult, session.SourceGoCommandResult},
}

var openAPIFieldMinimums = map[string]float64{
	"HealthView.schema_version":                     0,
	"BudgetView.max_turns":                          1,
	"BudgetView.max_tokens":                         0,
	"BudgetView.max_tool_calls":                     0,
	"BudgetView.max_cost_usd":                       0,
	"BudgetView.timeout_seconds":                    0,
	"RunModeView.revision":                          1,
	"RunExecutionProfileView.revision":              1,
	"SupervisorCheckpointView.next_turn":            1,
	"SupervisorCheckpointView.input_tokens":         0,
	"SupervisorCheckpointView.output_tokens":        0,
	"SupervisorCheckpointView.total_tokens":         0,
	"SupervisorCheckpointView.execution_millis":     0,
	"ToolUsageView.consumed":                        0,
	"ToolUsageView.limit":                           0,
	"RunExecutionLeaseView.generation":              1,
	"OperatorSteeringMessageView.sequence":          1,
	"OperatorSteeringQueueView.pending":             0,
	"OperatorSteeringQueueView.prepared":            0,
	"OperatorSteeringQueueView.committed":           0,
	"OperatorSteeringQueueView.cancelled":           0,
	"PlanDeliveryModuleView.ordinal":                1,
	"PlanDeliveryDirectionView.ordinal":             1,
	"PlanDeliveryProposalView.mode_revision":        1,
	"PlanDeliveryProposalView.version":              1,
	"PlanDeliverySelectionItemView.ordinal":         1,
	"PlanDeliverySelectionItemView.module_ordinal":  1,
	"PlanDeliverySelectionView.direction_ordinal":   1,
	"PlanDeliverySelectionView.version":             1,
	"MessageView.token_estimate":                    0,
	"EventView.sequence":                            1,
	"WorkItemView.version":                          1,
	"NoteView.version":                              1,
	"ArtifactView.size_bytes":                       0,
	"SupervisorToolCallView.position":               0,
	"SupervisorToolCallView.model_attempt":          1,
	"SupervisorToolRoundView.turn":                  1,
	"SupervisorToolRoundView.round":                 1,
	"SupervisorToolRoundView.model_attempt":         1,
	"ModelCancellationRequestView.model_attempt":    1,
	"ModelCancellationView.model_attempt":           1,
	"SpecialistModelCancellationView.model_attempt": 1,
	"AgentNodeView.depth":                           0,
	"AgentNodeView.turn_limit":                      0,
	"AgentNodeView.token_limit":                     0,
	"AgentNodeView.turns_used":                      0,
	"AgentNodeView.tokens_used":                     0,
	"FindingReportSummaryView.finding_count":        0,
	"FindingReportSummaryView.evidence_count":       0,
	"FindingView.ordinal":                           1,
	"FindingView.confidence":                        0,

	"ExternalSkillProjectionView.mode_revision":           1,
	"ExternalSkillProjectionView.token_budget":            1,
	"ExternalSkillProjectionView.token_upper_bound":       1,
	"ExternalSkillProjectionView.item_count":              1,
	"ExternalSkillProjectionItemView.ordinal":             1,
	"ExternalSkillProjectionItemView.token_upper_bound":   1,
	"ExternalSkillProjectionItemView.declared_tool_count": 0,
	"ExternalSkillDeliveryView.prepared":                  0,
	"ExternalSkillDeliveryView.committed":                 0,
	"RunLifecycleControlView.event_sequence_start":        1,
	"RunLifecycleControlView.event_sequence_end":          1,
	"RunExecutionControlRequestView.max_steps":            1,
	"RunExecutionControlView.max_steps":                   1,
	"RunExecutionControlView.selected_count":              0,
	"RunExecutionControlView.steps_completed":             0,
	"RunExecutionControlView.pending_count":               0,
	"RunExecutionControlView.prepared_count":              0,
	"RunExecutionControlView.committed_count":             0,
	"RunExecutionControlView.cancelled_count":             0,
	"RunExecutionControlView.completion_event_sequence":   1,
	"PlanDirectionControlRequestView.direction":           1,
	"PlanDirectionControlView.direction":                  1,
	"PlanDirectionControlView.work_item_count":            1,
	"ApprovalQueueItemView.version":                       1,
	"ProviderDiagnosticView.duration_ms":                  0,
	"RunWakeScheduleRequestView.max_attempts":             1,
	"RunWakeScheduleRequestView.initial_delay_seconds":    0,
	"RunWakeScheduleRequestView.base_backoff_seconds":     domain.MinRunWakeBackoffSeconds,
	"RunWakeScheduleRequestView.max_backoff_seconds":      domain.MinRunWakeBackoffSeconds,
	"RunWakeScheduleRequestView.max_elapsed_seconds":      domain.MinRunWakeElapsedSeconds,
	"RunWakeIntentView.max_attempts":                      1,
	"RunWakeIntentView.attempt_count":                     0,
	"RunWakeExecutionRequestView.max_steps":               1,
	"WorkspaceExplorerEntryView.size_bytes":               0,
	"WorkspaceExplorerView.total_bytes":                   0,
	"WorkspaceExplorerView.returned_bytes":                0,
	"WorkspaceExplorerView.redaction_count":               0,
	"WorkspaceSearchResultView.line":                      0,
	"WorkspaceSearchView.scanned_entries":                 0,
	"WorkspaceSearchView.scanned_files":                   0,
	"WorkspaceSearchView.scanned_bytes":                   0,
	"EvidenceAttachmentView.session_message_id":           1,
}

var openAPIFieldMaximums = map[string]float64{
	"RunExecutionControlRequestView.max_steps":         domain.MaxRunExecutionHandoffSteps,
	"RunExecutionControlView.max_steps":                domain.MaxRunExecutionHandoffSteps,
	"RunExecutionControlView.selected_count":           domain.MaxRunExecutionHandoffSteps,
	"RunExecutionControlView.steps_completed":          domain.MaxRunExecutionHandoffSteps,
	"RunExecutionControlView.pending_count":            domain.MaxRunExecutionHandoffSteps,
	"RunExecutionControlView.prepared_count":           domain.MaxRunExecutionHandoffSteps,
	"RunExecutionControlView.committed_count":          domain.MaxRunExecutionHandoffSteps,
	"RunExecutionControlView.cancelled_count":          domain.MaxRunExecutionHandoffSteps,
	"PlanDirectionControlRequestView.direction":        domain.PlanDeliveryDirectionCount,
	"PlanDirectionControlView.direction":               domain.PlanDeliveryDirectionCount,
	"PlanDirectionControlView.work_item_count":         domain.MaxPlanDeliveryModules,
	"RunWakeScheduleRequestView.max_attempts":          domain.MaxRunWakeAttempts,
	"RunWakeScheduleRequestView.initial_delay_seconds": domain.MaxRunWakeInitialDelaySeconds,
	"RunWakeScheduleRequestView.base_backoff_seconds":  domain.MaxRunWakeBackoffSeconds,
	"RunWakeScheduleRequestView.max_backoff_seconds":   domain.MaxRunWakeBackoffSeconds,
	"RunWakeScheduleRequestView.max_elapsed_seconds":   domain.MaxRunWakeElapsedSeconds,
	"RunWakeExecutionRequestView.max_steps":            domain.MaxRunExecutionHandoffSteps,
	"WorkspaceExplorerView.returned_bytes":             workspace.MaxExplorerProjectedBytes,
	"WorkspaceSearchView.scanned_entries":              workspace.MaxSearchEntries,
	"WorkspaceSearchView.scanned_files":                workspace.MaxSearchFiles,
	"WorkspaceSearchView.scanned_bytes":                workspace.MaxSearchReadBytes,
}

var openAPIFieldMaxLengths = map[string]int{
	"ModelCancellationRequestView.reason":            domain.MaxModelCancellationReasonRunes,
	"RunExecutionProfileControlRequestView.reason":   domain.MaxRunExecutionProfileReasonRunes,
	"RunCreationControlRequestView.goal":             domain.MaxRunCreationGoalBytes,
	"SessionMessageControlRequestView.content":       domain.MaxOperatorSteeringContentBytes,
	"SessionSteeringCancellationRequestView.reason":  domain.MaxOperatorSteeringReasonBytes,
	"ApprovalDecisionControlRequestView.reason":      approval.MaxReasonRunes,
	"ProviderCredentialRequestView.secret":           credential.MaxSecretBytes,
	"FileEditProposalSourceView.path":                workspace.MaxExplorerPathRunes,
	"FileEditProposalSourceView.content":             workspace.MaxExplorerProjectedBytes,
	"FileEditProposalSourceView.content_sha256":      64,
	"FileEditProposalRequestView.source_handle":      43,
	"FileEditProposalRequestView.proposed_text":      fileedit.MaxContentBytes,
	"MessageView.source_ref":                         session.MaxContextSourceRefRunes,
	"MessageView.content_sha256":                     64,
	"SkillPackageInstallRequestView.archive_base64":  base64.StdEncoding.EncodedLen(skills.MaxPackageArchiveBytes),
	"WorkspaceExplorerView.path":                     workspace.MaxExplorerPathRunes,
	"WorkspaceExplorerProvenanceView.source_ref":     workspace.MaxExplorerPathRunes,
	"WorkspaceExplorerProvenanceView.content_sha256": 64,
	"WorkspaceExplorerEntryView.name":                255,
	"WorkspaceExplorerEntryView.path":                workspace.MaxExplorerPathRunes,
	"WorkspaceSearchResultView.path":                 workspace.MaxExplorerPathRunes,
	"WorkspaceSearchResultView.snippet":              workspace.MaxSearchSnippetBytes,
	"EvidenceAttachmentRequestView.source_ref":       workspace.MaxExplorerPathRunes,
	"EvidenceAttachmentRequestView.content_sha256":   64,
	"EvidenceAttachmentView.source_ref":              workspace.MaxExplorerPathRunes,
	"EvidenceAttachmentView.content_sha256":          64,
	"EvidenceInventoryItemView.source_ref":           workspace.MaxExplorerPathRunes,
	"EvidenceInventoryItemView.content_sha256":       64,
}

func runStatuses() []string {
	return []string{string(domain.RunCreated), string(domain.RunPreparing), string(domain.RunRunning),
		string(domain.RunWaitingApproval), string(domain.RunPaused), string(domain.RunCompleted),
		string(domain.RunFailed), string(domain.RunCancelled)}
}

func workItemStatusesOpenAPI() []string {
	return []string{string(domain.WorkItemPending), string(domain.WorkItemInProgress),
		string(domain.WorkItemBlocked), string(domain.WorkItemCompleted), string(domain.WorkItemCancelled)}
}

func noteStatusesOpenAPI() []string {
	return []string{string(domain.NoteActive), string(domain.NoteArchived)}
}

func noteCategoriesOpenAPI() []string {
	return []string{string(domain.NoteObservation), string(domain.NoteHypothesis), string(domain.NoteDecision),
		string(domain.NoteSummary), string(domain.NoteReference)}
}

func noteVisibilitiesOpenAPI() []string {
	return []string{string(domain.NoteVisibilityRun), string(domain.NoteVisibilityRoot),
		string(domain.NoteVisibilityOwner)}
}

func artifactStreams() []string {
	return []string{string(artifact.StreamStdout), string(artifact.StreamStderr)}
}

func sortedOpenAPIPaths() []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(openAPIOperationSpecs()))
	for _, spec := range openAPIOperationSpecs() {
		if _, exists := seen[spec.Path]; exists {
			continue
		}
		seen[spec.Path] = struct{}{}
		paths = append(paths, spec.Path)
	}
	sort.Strings(paths)
	return paths
}
