package httpapi

import (
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
