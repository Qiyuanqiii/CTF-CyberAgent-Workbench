package httpapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/domain"
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
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
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
	Get openAPIOperation `json:"get"`
}

type openAPIOperation struct {
	OperationID string             `json:"operationId"`
	Summary     string             `json:"summary"`
	Description string             `json:"description"`
	Tags        []string           `json:"tags"`
	Parameters  []openAPIParameter `json:"parameters,omitempty"`
	Responses   map[string]any     `json:"responses"`
	ReadOnly    bool               `json:"x-cyberagent-read-only"`
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
	OperationID string
	Summary     string
	Description string
	Tag         string
	DataType    reflect.Type
	Collection  bool
	Paginated   bool
	NotFound    bool
	RawDocument bool
	Parameters  []openAPIParameter
}

// GenerateOpenAPI creates the canonical client contract from the Go response DTOs.
func GenerateOpenAPI() ([]byte, error) {
	registry := newOpenAPISchemaRegistry()
	paths := make(map[string]openAPIPathItem)
	for _, spec := range openAPIOperationSpecs() {
		if _, exists := paths[spec.Path]; exists {
			return nil, fmt.Errorf("duplicate OpenAPI path %q", spec.Path)
		}
		operation, err := buildOpenAPIOperation(spec, registry)
		if err != nil {
			return nil, err
		}
		paths[spec.Path] = openAPIPathItem{Get: operation}
	}
	if registry.err != nil {
		return nil, registry.err
	}
	document := openAPIDocument{
		OpenAPI:           openAPISpecVersion,
		JSONSchemaDialect: openAPIJSONSchemaDialect,
		Info: openAPIInfo{
			Title: "CyberAgent Workbench Local API",
			Description: "Authenticated loopback-only read API owned by the Go control plane. " +
				"The contract exposes durable metadata, never tool execution or security-policy bypasses.",
			Version: Version,
		},
		Servers:  []openAPIServer{{URL: "http://127.0.0.1:8765", Description: "Default loopback server"}},
		Security: []map[string][]string{{"BearerAuth": {}}},
		Tags: []openAPITag{
			{Name: "System", Description: "API discovery, health, and contract metadata."},
			{Name: "Runs", Description: "Durable Run state and Run-scoped projections."},
			{Name: "Sessions", Description: "Persisted Session metadata and redacted messages."},
			{Name: "Memory", Description: "Structured WorkItems and Notes."},
			{Name: "Artifacts", Description: "Content-free Artifact descriptors."},
		},
		Paths: paths,
		Components: openAPIComponents{
			Schemas:   registry.schemas,
			Responses: standardOpenAPIErrorResponses(registry),
			SecuritySchemes: map[string]openAPISecurityType{
				"BearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "opaque",
					Description: "Process-scoped local administrator token; never persisted by CyberAgent."},
			},
		},
		ReadOnly: true,
	}
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode OpenAPI document: %w", err)
	}
	return append(encoded, '\n'), nil
}

func openAPIOperationSpecs() []openAPIOperationSpec {
	runID := pathIdentityParameter("run_id", "Run identity")
	sessionID := pathIdentityParameter("session_id", "Session identity")
	workItemID := pathIdentityParameter("work_item_id", "WorkItem identity")
	noteID := pathIdentityParameter("note_id", "Note identity")
	artifactID := pathIdentityParameter("artifact_id", "Artifact identity")
	return []openAPIOperationSpec{
		{Path: "/api/v1", OperationID: "getAPIIndex", Summary: "Inspect API resources",
			Description: "Returns API and application versions plus top-level resources.", Tag: "System",
			DataType: reflect.TypeOf(IndexView{})},
		{Path: "/api/v1/health", OperationID: "getHealth", Summary: "Inspect local API health",
			Description: "Reads the current SQLite schema version without mutating state.", Tag: "System",
			DataType: reflect.TypeOf(HealthView{})},
		{Path: OpenAPIPath, OperationID: "getOpenAPI", Summary: "Read the OpenAPI contract",
			Description: "Returns the raw deterministic OpenAPI 3.1 JSON document under the same authentication boundary.",
			Tag:         "System", RawDocument: true},
		{Path: "/api/v1/runs", OperationID: "listRuns", Summary: "List Runs", Tag: "Runs",
			Description: "Returns a bounded cursor page of durable Runs.", DataType: reflect.TypeOf(RunView{}),
			Collection: true, Paginated: true, Parameters: append(paginationParameters(),
				stringQueryParameter("status", "Exact Run status filter", runStatuses()),
				identityQueryParameter("mission_id", "Exact Mission identity filter"))},
		{Path: "/api/v1/runs/{run_id}", OperationID: "getRun", Summary: "Inspect a Run", Tag: "Runs",
			Description: "Returns Run, Mission, checkpoint, tool usage, and token-free execution-lease metadata.",
			DataType:    reflect.TypeOf(RunDetailView{}), NotFound: true, Parameters: []openAPIParameter{runID}},
		{Path: "/api/v1/runs/{run_id}/events", OperationID: "listRunEvents", Summary: "List Run events",
			Tag: "Runs", Description: "Returns the ordered append-only Run event stream.",
			DataType: reflect.TypeOf(EventView{}), Collection: true, Paginated: true, NotFound: true,
			Parameters: append([]openAPIParameter{runID}, paginationParameters()...)},
		{Path: "/api/v1/runs/{run_id}/work-items", OperationID: "listRunWorkItems",
			Summary: "List Run WorkItems", Tag: "Memory", Description: "Returns structured Work Board items.",
			DataType: reflect.TypeOf(WorkItemView{}), Collection: true, Paginated: true, NotFound: true,
			Parameters: append(append([]openAPIParameter{runID}, paginationParameters()...),
				arrayQueryParameter("status", "Repeat or comma-separate WorkItem statuses", workItemStatusesOpenAPI(), 5),
				stringQueryParameter("owner", "Exact WorkItem owner filter", nil))},
		{Path: "/api/v1/runs/{run_id}/notes", OperationID: "listRunNotes", Summary: "List Run Notes",
			Tag: "Memory", Description: "Returns structured, redacted Run Notes.", DataType: reflect.TypeOf(NoteView{}),
			Collection: true, Paginated: true, NotFound: true,
			Parameters: append(append([]openAPIParameter{runID}, paginationParameters()...),
				arrayQueryParameter("status", "Repeat or comma-separate Note statuses", noteStatusesOpenAPI(), 2),
				arrayQueryParameter("category", "Repeat or comma-separate Note categories", noteCategoriesOpenAPI(), 5),
				arrayQueryParameter("visibility", "Repeat or comma-separate Note visibility values", noteVisibilitiesOpenAPI(), 3),
				stringQueryParameter("owner", "Exact Note owner filter", nil),
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
	responses := standardOperationResponses(spec.NotFound)
	if spec.RawDocument {
		responses["200"] = openAPIResponse{Description: "OpenAPI 3.1 document", Content: map[string]openAPIMediaType{
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
		responses["200"] = openAPIResponse{Description: "Successful read", Content: map[string]openAPIMediaType{
			"application/json": {Schema: successEnvelopeSchema(dataSchema, spec.Paginated, registry)},
		}}
	}
	return openAPIOperation{OperationID: spec.OperationID, Summary: spec.Summary,
		Description: spec.Description, Tags: []string{spec.Tag}, Parameters: spec.Parameters,
		Responses: responses, ReadOnly: true}, nil
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

func standardOperationResponses(notFound bool) map[string]any {
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
		"BadRequest":        makeResponse("Invalid path, query, method, or request body"),
		"Unauthorized":      makeResponse("Missing or invalid bearer token"),
		"Forbidden":         makeResponse("Request is outside the loopback security boundary"),
		"NotFound":          makeResponse("Requested durable resource was not found"),
		"RequestTooLarge":   makeResponse("Request target or query exceeded its hard limit"),
		"ResourceExhausted": makeResponse("Bounded response or resource limit was exhausted"),
		"InternalError":     makeResponse("Redacted internal server failure"),
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
	if strings.HasSuffix(fieldName, "_id") || fieldName == "id" || fieldName == "event_id" {
		if _, ref := schema["$ref"]; !ref {
			schema["minLength"] = 1
			schema["maxLength"] = 256
		}
	}
	if typeName == "ArtifactView" && fieldName == "sha256" {
		schema["pattern"] = "^[a-f0-9]{64}$"
	}
}

var openAPIFieldEnums = map[string][]string{
	"IndexView.api_version":                 {Version},
	"HealthView.status":                     {"ok"},
	"HealthView.api_version":                {Version},
	"ScopeView.network_mode":                {"disabled", "allowlist"},
	"MissionView.profile":                   {"code", "review", "learn", "script"},
	"RunView.status":                        runStatuses(),
	"SupervisorCheckpointView.phase":        {"idle", "turn_started", "turn_failed", "waiting", "run_completed", "run_failed"},
	"SupervisorCheckpointView.repair_phase": {"pending", "exhausted"},
	"RunExecutionLeaseView.status":          {string(domain.RunExecutionLeaseActive), string(domain.RunExecutionLeaseReleased)},
	"SessionView.status":                    {"active", "archived"},
	"WorkItemView.status":                   workItemStatusesOpenAPI(),
	"WorkItemView.priority":                 {"low", "normal", "high", "critical"},
	"NoteView.category":                     noteCategoriesOpenAPI(),
	"NoteView.visibility":                   noteVisibilitiesOpenAPI(),
	"NoteView.status":                       noteStatusesOpenAPI(),
	"ArtifactView.stream":                   artifactStreams(),
	"ArtifactView.encoding":                 {artifact.EncodingUTF8},
	"SupervisorToolCallView.status":         {"pending", "completed", "denied", "failed"},
}

var openAPIFieldMinimums = map[string]float64{
	"HealthView.schema_version":                 0,
	"BudgetView.max_turns":                      1,
	"BudgetView.max_tokens":                     0,
	"BudgetView.max_tool_calls":                 0,
	"BudgetView.max_cost_usd":                   0,
	"BudgetView.timeout_seconds":                0,
	"SupervisorCheckpointView.next_turn":        1,
	"SupervisorCheckpointView.input_tokens":     0,
	"SupervisorCheckpointView.output_tokens":    0,
	"SupervisorCheckpointView.total_tokens":     0,
	"SupervisorCheckpointView.execution_millis": 0,
	"ToolUsageView.consumed":                    0,
	"ToolUsageView.limit":                       0,
	"RunExecutionLeaseView.generation":          1,
	"MessageView.token_estimate":                0,
	"EventView.sequence":                        1,
	"WorkItemView.version":                      1,
	"NoteView.version":                          1,
	"ArtifactView.size_bytes":                   0,
	"SupervisorToolCallView.position":           0,
	"SupervisorToolCallView.model_attempt":      1,
	"SupervisorToolRoundView.turn":              1,
	"SupervisorToolRoundView.round":             1,
	"SupervisorToolRoundView.model_attempt":     1,
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
	paths := make([]string, 0, len(openAPIOperationSpecs()))
	for _, spec := range openAPIOperationSpecs() {
		paths = append(paths, spec.Path)
	}
	sort.Strings(paths)
	return paths
}
