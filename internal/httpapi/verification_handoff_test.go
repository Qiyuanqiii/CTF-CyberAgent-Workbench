package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/verification"
)

func TestVerificationEvidenceHTTPIsImmutableRedactedAndFeedsCodeHandoff(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "verification-handoff-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	workspace := store.WorkspaceRecord{ID: "workspace-verification-http", Name: "verification",
		RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := st.SaveWorkspace(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(t.Context(),
		application.CreateRunRequest{Goal: "verify the Code delivery", Profile: "code",
			ModelRoute: "mock/mock-code", WorkspaceID: workspace.ID,
			Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4}})
	if err != nil {
		t.Fatal(err)
	}
	api, err := New(st, Config{AccessToken: testAccessToken, ControlToken: testControlToken,
		AppVersion: "verification-test", VerificationEvidenceEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	path := strings.ReplaceAll(VerificationEvidencePathTemplate, "{run_id}", run.ID)
	secret := "sk-123456789012345678901234567890"
	body := `{"version":"operator_verification_evidence.v1","outcome":"pass",` +
		`"title":"Focused verification","summary":"tests passed with token ` + secret + `"}`
	operationKey := "http-verification-evidence-0001"
	response := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", strings.NewReader(body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("record status=%d body=%s", response.Code, response.Body.String())
	}
	var recorded struct {
		Data VerificationEvidenceControlView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	value := recorded.Data
	if value.ProtocolVersion != verification.EvidenceProtocolVersion || value.RunID != run.ID ||
		value.SessionID != run.SessionID || value.WorkspaceID != workspace.ID ||
		value.Outcome != string(verification.OutcomePass) || !value.Redacted ||
		!value.Immutable || !value.OperatorSupplied || value.CommandExecuted ||
		value.ModelAssertion || value.Approval || value.AuthorityGranted || value.Replayed ||
		strings.Contains(value.Summary, secret) || !strings.Contains(value.Summary, "[REDACTED:") {
		t.Fatalf("unsafe verification projection: %#v", value)
	}
	replay := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", strings.NewReader(body))
	if replay.Code != http.StatusAccepted {
		t.Fatalf("replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	if !recorded.Data.Replayed || recorded.Data.ID != value.ID {
		t.Fatalf("verification replay diverged: %#v", recorded.Data)
	}

	inventory := performSessionMessageRequest(t, api, http.MethodGet, path,
		testAccessToken, "", "", nil)
	if inventory.Code != http.StatusOK ||
		!strings.Contains(inventory.Body.String(), `"protocol_version":"operator_verification_inventory.v1"`) ||
		!strings.Contains(inventory.Body.String(), `"pass_count":1`) ||
		strings.Contains(inventory.Body.String(), secret) {
		t.Fatalf("inventory status=%d body=%s", inventory.Code, inventory.Body.String())
	}

	handoffPath := strings.ReplaceAll(CodeHandoffPathTemplate, "{run_id}", run.ID)
	handoff := performSessionMessageRequest(t, api, http.MethodGet, handoffPath,
		testAccessToken, "", "", nil)
	handoffBody := handoff.Body.String()
	if handoff.Code != http.StatusOK ||
		!strings.Contains(handoffBody, `"protocol_version":"code_handoff.v1"`) ||
		!strings.Contains(handoffBody, `"surface":"code"`) ||
		!strings.Contains(handoffBody, `"pass_count":1`) ||
		!strings.Contains(handoffBody, `"regenerable":true`) ||
		!strings.Contains(handoffBody, `"durable_sources":true`) ||
		!strings.Contains(handoffBody, `"private_bodies_included":false`) ||
		!strings.Contains(handoffBody, `"composite_mutation":false`) ||
		!strings.Contains(handoffBody, `"resume_authorized":false`) ||
		!strings.Contains(handoffBody, `"execution_started":false`) ||
		strings.Contains(handoffBody, "Focused verification") ||
		strings.Contains(handoffBody, "tests passed") || strings.Contains(handoffBody, secret) {
		t.Fatalf("handoff status=%d body=%s", handoff.Code, handoffBody)
	}

	readOnly, err := New(st, Config{AccessToken: testAccessToken,
		AppVersion: "verification-read-only"})
	if err != nil {
		t.Fatal(err)
	}
	disabled := performSessionMessageRequest(t, readOnly, http.MethodPost, path,
		testControlToken, "http-verification-disabled-0001", "application/json",
		strings.NewReader(body))
	assertAPIError(t, disabled, http.StatusNotFound, "NOT_FOUND")
}

func TestVerificationPlanHTTPPersistsGuidanceWithoutResultAuthority(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "verification-plan-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	workspace := store.WorkspaceRecord{ID: "workspace-verification-plan-http",
		Name: "verification-plan", RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := st.SaveWorkspace(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(t.Context(),
		application.CreateRunRequest{Goal: "plan the Code verification", Profile: "code",
			ModelRoute: "mock/mock-code", WorkspaceID: workspace.ID,
			Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4}})
	if err != nil {
		t.Fatal(err)
	}
	api, err := New(st, Config{AccessToken: testAccessToken, ControlToken: testControlToken,
		AppVersion: "verification-plan-test", VerificationEvidenceEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	path := strings.ReplaceAll(VerificationPlanPathTemplate, "{run_id}", run.ID)
	secret := "sk-123456789012345678901234567890"
	body := `{"version":"operator_verification_plan.v1","title":"Release checks",` +
		`"summary":"Operator guidance only","items":[` +
		`{"title":"Focused tests","expected_observation":"Observe tests passing"},` +
		`{"title":"Secret boundary","expected_observation":"Token ` + secret + ` is absent"}]}`
	operationKey := "http-verification-plan-0001"
	response := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", strings.NewReader(body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("record plan status=%d body=%s", response.Code, response.Body.String())
	}
	var recorded struct {
		Data VerificationPlanControlView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	value := recorded.Data
	if value.ProtocolVersion != verification.PlanProtocolVersion || value.RunID != run.ID ||
		value.SessionID != run.SessionID || value.WorkspaceID != workspace.ID ||
		!value.Redacted || !value.Immutable || !value.OperatorSupplied || !value.GuidanceOnly ||
		value.CommandExecuted || value.ModelAssertion || value.ResultInferred ||
		value.Approval || value.AuthorityGranted || value.Replayed || value.ItemCount != 2 ||
		len(value.Items) != 2 || strings.Contains(value.Items[1].ExpectedObservation, secret) {
		t.Fatalf("unsafe verification plan projection: %#v", value)
	}
	replay := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", strings.NewReader(body))
	if replay.Code != http.StatusAccepted {
		t.Fatalf("plan replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	if !recorded.Data.Replayed || recorded.Data.ID != value.ID {
		t.Fatalf("verification plan replay diverged: %#v", recorded.Data)
	}
	inventory := performSessionMessageRequest(t, api, http.MethodGet, path,
		testAccessToken, "", "", nil)
	if inventory.Code != http.StatusOK ||
		!strings.Contains(inventory.Body.String(),
			`"protocol_version":"operator_verification_plan_inventory.v1"`) ||
		!strings.Contains(inventory.Body.String(), `"guidance_only":true`) ||
		!strings.Contains(inventory.Body.String(), `"result_inferred":false`) ||
		strings.Contains(inventory.Body.String(), secret) ||
		strings.Contains(inventory.Body.String(), `"outcome"`) {
		t.Fatalf("plan inventory status=%d body=%s", inventory.Code, inventory.Body.String())
	}
	handoffPath := strings.ReplaceAll(CodeHandoffPathTemplate, "{run_id}", run.ID)
	handoff := performSessionMessageRequest(t, api, http.MethodGet, handoffPath,
		testAccessToken, "", "", nil)
	if handoff.Code != http.StatusOK ||
		!strings.Contains(handoff.Body.String(), `"source_event_sequence":`) ||
		!strings.Contains(handoff.Body.String(), `"verification_plans":{"returned_count":1`) ||
		strings.Contains(handoff.Body.String(), "Release checks") ||
		strings.Contains(handoff.Body.String(), "Focused tests") {
		t.Fatalf("plan-aware handoff status=%d body=%s", handoff.Code, handoff.Body.String())
	}
	exportPath := strings.ReplaceAll(CodeHandoffExportPathTemplate, "{run_id}", run.ID) +
		"?format=markdown"
	exportResponse := performSessionMessageRequest(t, api, http.MethodGet, exportPath,
		testAccessToken, "", "", nil)
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("handoff export status=%d body=%s", exportResponse.Code,
			exportResponse.Body.String())
	}
	var exported struct {
		Data CodeHandoffExportView `json:"data"`
	}
	if err := json.Unmarshal(exportResponse.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(exported.Data.Content))
	if exported.Data.ProtocolVersion != application.CodeHandoffExportProtocolVersion ||
		exported.Data.Format != application.CodeHandoffExportFormatMarkdown ||
		exported.Data.RunID != run.ID || exported.Data.SourceEventSequence <= 0 ||
		exported.Data.ContentBytes != len([]byte(exported.Data.Content)) ||
		exported.Data.ContentSHA256 != hex.EncodeToString(digest[:]) ||
		!exported.Data.ReadOnly || !exported.Data.DownloadOnly || exported.Data.PrivateBodies ||
		exported.Data.ResumeAuthorized || exported.Data.MutationSupported ||
		exported.Data.ReportAcceptance || exported.Data.ExecutionStarted ||
		strings.Contains(exported.Data.Content, "Release checks") ||
		strings.Contains(exported.Data.Content, "Focused tests") ||
		strings.Contains(exported.Data.Content, secret) {
		t.Fatalf("unsafe handoff export: %#v", exported.Data)
	}
	invalidExport := performSessionMessageRequest(t, api, http.MethodGet,
		strings.TrimSuffix(exportPath, "markdown")+"html", testAccessToken, "", "", nil)
	assertAPIError(t, invalidExport, http.StatusBadRequest, "INVALID_ARGUMENT")
	withOutcome := strings.TrimSuffix(body, "}") + `,"outcome":"pass"}`
	rejected := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, "http-verification-plan-outcome-0001", "application/json",
		strings.NewReader(withOutcome))
	assertAPIError(t, rejected, http.StatusBadRequest, "INVALID_ARGUMENT")

	readOnly, err := New(st, Config{AccessToken: testAccessToken,
		AppVersion: "verification-plan-read-only"})
	if err != nil {
		t.Fatal(err)
	}
	disabled := performSessionMessageRequest(t, readOnly, http.MethodPost, path,
		testControlToken, "http-verification-plan-disabled-0001", "application/json",
		strings.NewReader(body))
	assertAPIError(t, disabled, http.StatusNotFound, "NOT_FOUND")
}
