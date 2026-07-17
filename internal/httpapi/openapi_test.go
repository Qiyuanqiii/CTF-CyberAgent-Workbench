package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/coordinator"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/llm"
)

func TestOpenAPIDocumentIsDeterministicCapabilitySeparatedAndSecretFree(t *testing.T) {
	first, err := GenerateOpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateOpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) || len(first) == 0 || first[len(first)-1] != '\n' || !json.Valid(first) {
		t.Fatal("OpenAPI generation is not deterministic canonical JSON")
	}
	for _, forbidden := range []string{`"lease_id"`, `"pending_input"`, `"fencing_token"`, `"api_key"`} {
		if bytes.Contains(first, []byte(forbidden)) {
			t.Fatalf("OpenAPI document exposed forbidden internal property %s", forbidden)
		}
	}

	var document openAPIDocument
	if err := json.Unmarshal(first, &document); err != nil {
		t.Fatal(err)
	}
	if document.OpenAPI != openAPISpecVersion || document.JSONSchemaDialect != openAPIJSONSchemaDialect ||
		document.Info.Version != Version || document.Info.License.Identifier != "Apache-2.0" ||
		document.ReadOnly || len(document.Security) != 1 {
		t.Fatalf("OpenAPI metadata is incomplete: %#v", document)
	}
	expectedPaths := sortedOpenAPIPaths()
	actualPaths := make([]string, 0, len(document.Paths))
	operationIDs := make(map[string]struct{}, len(document.Paths))
	for path, item := range document.Paths {
		actualPaths = append(actualPaths, path)
		operations := make([]*openAPIOperation, 0, 2)
		if item.Get != nil {
			if item.Get.OperationID == "" || !item.Get.ReadOnly || item.Get.Responses["200"] == nil ||
				item.Get.RequestBody != nil || len(item.Get.Security) != 0 {
				t.Fatalf("path %s has an incomplete read operation: %#v", path, item.Get)
			}
			operations = append(operations, item.Get)
		}
		if item.Post != nil {
			validControl := (path == ModelCancellationPathTemplate &&
				item.Post.OperationID == "requestModelCancellation") ||
				(path == SpecialistModelCancellationPathTemplate &&
					item.Post.OperationID == "requestSpecialistModelCancellation") ||
				(path == RunExecutionProfileControlPathTemplate &&
					item.Post.OperationID == "selectRunExecutionProfile")
			if !validControl ||
				item.Post.ReadOnly || item.Post.Responses["202"] == nil || item.Post.RequestBody == nil ||
				len(item.Post.Security) != 1 || item.Post.Security[0]["ControlBearerAuth"] == nil {
				t.Fatalf("path %s has an incomplete control operation: %#v", path, item.Post)
			}
			operations = append(operations, item.Post)
		}
		if len(operations) != 1 {
			t.Fatalf("path %s must expose exactly one operation: %#v", path, item)
		}
		if _, duplicate := operationIDs[operations[0].OperationID]; duplicate {
			t.Fatalf("duplicate OpenAPI operation id %q", operations[0].OperationID)
		}
		operationIDs[operations[0].OperationID] = struct{}{}
	}
	sort.Strings(actualPaths)
	if !reflect.DeepEqual(actualPaths, expectedPaths) {
		t.Fatalf("OpenAPI path catalog drifted:\n got %v\nwant %v", actualPaths, expectedPaths)
	}

	var raw struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(first, &raw); err != nil {
		t.Fatal(err)
	}
	for path, item := range raw.Paths {
		for method := range item {
			if method != "get" && !((path == ModelCancellationPathTemplate ||
				path == SpecialistModelCancellationPathTemplate ||
				path == RunExecutionProfileControlPathTemplate) && method == "post") {
				t.Fatalf("OpenAPI path %s exposed unexpected operation %q", path, method)
			}
		}
	}
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "RunExecutionLeaseView", "lease_id")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "SupervisorCheckpointView", "pending_input")
	for _, field := range []string{"content", "content_sha256", "requested_by", "session_id", "session_message_id"} {
		assertOpenAPISchemaOmits(t, document.Components.Schemas,
			"OperatorSteeringMessageView", field)
	}
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "ArtifactView", "content")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "AgentNodeView", "status_reason")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "DelegationReviewView", "reason")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "DelegationApplicationView", "policy_fingerprint")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "FanoutExecutionShardView", "report_json")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "FanoutExecutionShardView", "error_reason")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "FindingArtifactEvidenceView", "note")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "FindingArtifactEvidenceView", "attached_by")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "RunExecutionProfileView", "requested_by")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "RunExecutionProfileView", "reason")
	for _, field := range []string{"selection_id", "mission_id", "mode_snapshot_id", "requested_by",
		"operation_id", "fingerprint", "digest", "content", "path"} {
		assertOpenAPISchemaOmits(t, document.Components.Schemas,
			"ExternalSkillProjectionView", field)
	}
	for _, field := range []string{"selection_id", "installation_id", "fingerprint", "sha256",
		"object_key", "content", "path", "archive_bytes", "content_bytes"} {
		assertOpenAPISchemaOmits(t, document.Components.Schemas,
			"ExternalSkillProjectionItemView", field)
	}
	assertOpenAPISchemaOptional(t, document.Components.Schemas, "AgentGraphView", "root_agent_id")
}

func TestOpenAPIGoldenDocumentMatchesGoDTOs(t *testing.T) {
	generated, err := GenerateOpenAPI()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("..", "..", "docs", "openapi.json")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed OpenAPI document: %v", err)
	}
	if !bytes.Equal(committed, generated) {
		t.Fatalf("%s is stale; regenerate it with `cyberagent api openapi --output docs/openapi.json`", path)
	}
}

func TestOpenAPIRoutesMatchAuthenticatedLiveHandlers(t *testing.T) {
	fixture := newAPIFixture(t)
	fixture.api.eventStream = testEventStreamConfig(1, 100*time.Millisecond)
	childRun, child, childAttempt, childModel :=
		prepareOpenAPISpecialistCancellationTarget(t, fixture)
	_, profileRun, err := application.NewRunService(fixture.store).Create(t.Context(),
		application.CreateRunRequest{Goal: "OpenAPI execution profile target", Profile: "code",
			Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	replacements := map[string]string{
		"{run_id}":       fixture.run.ID,
		"{agent_id}":     child.ID,
		"{session_id}":   fixture.run.SessionID,
		"{work_item_id}": fixture.workItems[0].ID,
		"{note_id}":      fixture.notes[0].ID,
		"{artifact_id}":  fixture.artifactID,
		"{report_id}":    "report-openapi-missing-0001",
	}
	for _, spec := range openAPIOperationSpecs() {
		requestPath := spec.Path
		for placeholder, value := range replacements {
			requestPath = strings.ReplaceAll(requestPath, placeholder, value)
		}
		if spec.Path == SpecialistModelCancellationPathTemplate {
			requestPath = strings.ReplaceAll(spec.Path, "{run_id}", childRun.ID)
			requestPath = strings.ReplaceAll(requestPath, "{agent_id}", child.ID)
		} else if spec.Path == RunExecutionProfileControlPathTemplate {
			requestPath = strings.ReplaceAll(spec.Path, "{run_id}", profileRun.ID)
		}
		t.Run(spec.OperationID, func(t *testing.T) {
			var response *httptest.ResponseRecorder
			expectedStatus := http.StatusOK
			if spec.OperationID == "getRunFindingReport" {
				expectedStatus = http.StatusNotFound
			}
			if spec.Control {
				body := `{"profile":"docker"}`
				if spec.Path != RunExecutionProfileControlPathTemplate {
					attemptID := fixture.checkpoint.AttemptID
					modelAttempt := 1
					if spec.Path == SpecialistModelCancellationPathTemplate {
						attemptID = childAttempt.ID
						modelAttempt = childModel.Number
					}
					body = `{"attempt_id":"` + attemptID + `","model_attempt":` +
						fmt.Sprint(modelAttempt) + `}`
				}
				response = performControlPathRequest(t, fixture.api, requestPath,
					"openapi-live-operation-012345-"+spec.OperationID,
					strings.NewReader(body))
				expectedStatus = http.StatusAccepted
			} else {
				response = fixture.get(t, requestPath)
			}
			if response.Code != expectedStatus {
				t.Fatalf("documented route is not live: path=%s status=%d body=%s",
					requestPath, response.Code, response.Body.String())
			}
			assertSecurityHeaders(t, response)
			contentType := response.Header().Get("Content-Type")
			if spec.Streaming {
				streamEvents := parseSSEEvents(t, response.Body.Bytes())
				if !strings.HasPrefix(contentType, "text/event-stream") || len(streamEvents) != 1 {
					t.Fatalf("SSE response is invalid: content-type=%q body=%s", contentType, response.Body.String())
				}
			} else if spec.RawDocument {
				if !strings.HasPrefix(contentType, openAPIContentType) ||
					!bytes.Contains(response.Body.Bytes(), []byte(`"openapi": "3.1.0"`)) {
					t.Fatalf("raw OpenAPI response is invalid: content-type=%q body=%s", contentType, response.Body.String())
				}
			} else if !strings.HasPrefix(contentType, "application/json") || !json.Valid(response.Body.Bytes()) {
				t.Fatalf("API envelope has wrong content type %q", contentType)
			}
		})
	}

	unauthorized := fixture.request(t, http.MethodGet, OpenAPIPath, "",
		"127.0.0.1:8765", "127.0.0.1:45000", nil)
	assertAPIError(t, unauthorized, http.StatusUnauthorized, "POLICY_DENIED")
	assertAPIError(t, fixture.get(t, OpenAPIPath+"?format=yaml"), http.StatusBadRequest, "INVALID_ARGUMENT")
}

func prepareOpenAPISpecialistCancellationTarget(t *testing.T,
	fixture *apiFixture,
) (domain.Run, domain.AgentNode, domain.AgentAttempt, llm.ModelAttempt) {
	t.Helper()
	ctx := t.Context()
	runs := application.NewRunService(fixture.store)
	_, run, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "OpenAPI Specialist cancellation target", Profile: "code",
		Budget: domain.Budget{MaxTurns: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = runs.Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, found, err := fixture.store.GetRootAgent(ctx, run.ID)
	if err != nil || !found {
		t.Fatalf("OpenAPI Specialist root missing: found=%t err=%v", found, err)
	}
	coord, err := coordinator.NewWithSpecialistAdmission(fixture.store,
		coordinator.SpecialistAdmissionPolicy{
			MaxChildren: 1, MaxTurnsPerChild: 2, MaxTokensPerChild: 32,
		})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := coord.AdmitSpecialist(ctx, coordinator.AdmitSpecialistRequest{
		RunID: run.ID, ParentAgentID: root.ID,
		Title: "OpenAPI cancellation target", Skills: []string{"model.chat"},
		TurnLimit: 2, TokenLimit: 32,
		IdempotencyKey: "openapi-specialist-admission-012345",
	})
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := fixture.store.AcquireRunExecutionLease(ctx,
		domain.AcquireRunExecutionLeaseRequest{
			RunID: run.ID, OwnerID: "openapi-specialist-worker", TTL: time.Minute,
		})
	if err != nil {
		t.Fatal(err)
	}
	attemptID := "attempt-openapi-specialist-0001"
	attempt, _, err := fixture.store.BeginSpecialistAttempt(ctx, domain.AgentAttemptStart{
		AttemptID: attemptID, RunID: run.ID, AgentID: admitted.Agent.ID,
		ParentAgentID: root.ID, Lease: acquired.Lease, StartedAt: time.Now().UTC(),
	}, "openapi-specialist-start-012345")
	if err != nil {
		t.Fatal(err)
	}
	modelAttempt := llm.ModelAttempt{
		Number: 1, TransportAttempt: 1, MaxAttempts: 3,
		Provider: "openapi-specialist", Model: "test-model",
	}
	if inserted, err := fixture.store.RecordSpecialistModelStarted(ctx,
		domain.AgentAttemptRef{RunID: attempt.RunID, AgentID: attempt.AgentID,
			AttemptID: attempt.ID}, modelAttempt); err != nil || !inserted {
		t.Fatalf("OpenAPI Specialist model start inserted=%t err=%v", inserted, err)
	}
	return run, admitted.Agent, attempt, modelAttempt
}

func assertOpenAPISchemaOmits(t *testing.T, schemas map[string]map[string]any, name string, property string) {
	t.Helper()
	schema, ok := schemas[name]
	if !ok {
		t.Fatalf("OpenAPI component %s is missing", name)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI component %s has no properties", name)
	}
	if _, exposed := properties[property]; exposed {
		t.Fatalf("OpenAPI component %s exposed forbidden property %s", name, property)
	}
}

func assertOpenAPISchemaOptional(t *testing.T, schemas map[string]map[string]any,
	name string, property string,
) {
	t.Helper()
	schema, ok := schemas[name]
	if !ok {
		t.Fatalf("OpenAPI component %s is missing", name)
	}
	required, _ := schema["required"].([]any)
	for _, current := range required {
		if current == property {
			t.Fatalf("OpenAPI component %s unexpectedly requires %s", name, property)
		}
	}
}
