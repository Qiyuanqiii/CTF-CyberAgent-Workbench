package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestOpenAPIDocumentIsDeterministicReadOnlyAndSecretFree(t *testing.T) {
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
		document.Info.Version != Version || !document.ReadOnly || len(document.Security) != 1 {
		t.Fatalf("OpenAPI metadata is incomplete: %#v", document)
	}
	expectedPaths := sortedOpenAPIPaths()
	actualPaths := make([]string, 0, len(document.Paths))
	operationIDs := make(map[string]struct{}, len(document.Paths))
	for path, item := range document.Paths {
		actualPaths = append(actualPaths, path)
		if item.Get.OperationID == "" || !item.Get.ReadOnly || item.Get.Responses["200"] == nil {
			t.Fatalf("path %s has an incomplete read operation: %#v", path, item.Get)
		}
		if _, duplicate := operationIDs[item.Get.OperationID]; duplicate {
			t.Fatalf("duplicate OpenAPI operation id %q", item.Get.OperationID)
		}
		operationIDs[item.Get.OperationID] = struct{}{}
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
			if method != "get" {
				t.Fatalf("OpenAPI path %s exposed non-read operation %q", path, method)
			}
		}
	}
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "RunExecutionLeaseView", "lease_id")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "SupervisorCheckpointView", "pending_input")
	assertOpenAPISchemaOmits(t, document.Components.Schemas, "ArtifactView", "content")
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
	replacements := map[string]string{
		"{run_id}":       fixture.run.ID,
		"{session_id}":   fixture.run.SessionID,
		"{work_item_id}": fixture.workItems[0].ID,
		"{note_id}":      fixture.notes[0].ID,
		"{artifact_id}":  fixture.artifactID,
	}
	for _, spec := range openAPIOperationSpecs() {
		requestPath := spec.Path
		for placeholder, value := range replacements {
			requestPath = strings.ReplaceAll(requestPath, placeholder, value)
		}
		t.Run(spec.OperationID, func(t *testing.T) {
			response := fixture.get(t, requestPath)
			if response.Code != http.StatusOK {
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
