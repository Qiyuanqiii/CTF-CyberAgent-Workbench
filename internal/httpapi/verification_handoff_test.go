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
		!strings.Contains(handoffBody,
			`"verification_coverage":{"protocol_version":"operator_verification_plan_coverage.v1"`) ||
		!strings.Contains(handoffBody, `"result_inferred":false`) ||
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
		strings.Contains(exported.Data.Content, secret) ||
		!strings.Contains(exported.Data.Content, "Coverage: 0/2 items observed") {
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

func TestVerificationAssociationHTTPPreservesExplicitCausalityAndMetadataOnlyCoverage(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "verification-association-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	workspace := store.WorkspaceRecord{ID: "workspace-verification-association-http",
		Name: "verification-association", RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := st.SaveWorkspace(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(t.Context(),
		application.CreateRunRequest{Goal: "associate explicit verification evidence", Profile: "code",
			ModelRoute: "mock/mock-code", WorkspaceID: workspace.ID,
			Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4}})
	if err != nil {
		t.Fatal(err)
	}
	planResult, err := application.NewVerificationPlanService(st).Record(t.Context(),
		application.RecordVerificationPlanRequest{Version: verification.PlanProtocolVersion,
			RunID: run.ID, Title: "Release checks", Summary: "Operator guidance only",
			Items: []application.VerificationPlanItemRequest{{Title: "Focused tests",
				ExpectedObservation: "Observe an explicit result"}},
			OperationKey: "http-association-plan-operation-0001", AuthoredBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	evidenceResult, err := application.NewVerificationEvidenceService(st).Record(t.Context(),
		application.RecordVerificationEvidenceRequest{Version: verification.EvidenceProtocolVersion,
			RunID: run.ID, Outcome: string(verification.OutcomePass), Title: "Focused tests",
			Summary:      "Observed a passing suite",
			OperationKey: "http-association-evidence-operation-0001", RecordedBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	api, err := New(st, Config{AccessToken: testAccessToken, ControlToken: testControlToken,
		AppVersion: "verification-association-test", VerificationEvidenceEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	path := strings.ReplaceAll(VerificationAssociationPathTemplate, "{run_id}", run.ID)
	body := `{"version":"operator_verification_plan_evidence_association.v1","plan_id":"` +
		planResult.Plan.ID + `","plan_item_ordinal":1,"evidence_id":"` +
		evidenceResult.Evidence.ID + `"}`
	operationKey := "http-verification-association-0001"
	response := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", strings.NewReader(body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("association status=%d body=%s", response.Code, response.Body.String())
	}
	var recorded struct {
		Data VerificationAssociationControlView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	value := recorded.Data
	if value.ProtocolVersion != verification.PlanEvidenceAssociationProtocolVersion ||
		value.RunID != run.ID || value.SessionID != run.SessionID ||
		value.WorkspaceID != workspace.ID || value.PlanID != planResult.Plan.ID ||
		value.PlanItemOrdinal != 1 || value.PlanItemSHA256 != planResult.Plan.Items[0].ItemSHA256 ||
		value.EvidenceID != evidenceResult.Evidence.ID ||
		value.EvidenceOutcome != string(verification.OutcomePass) ||
		value.EvidenceEventSequence != evidenceResult.Evidence.EventSequence ||
		value.AssociationEventSequence <= value.EvidenceEventSequence || !value.Immutable ||
		!value.OperatorSupplied || !value.MetadataOnly || value.CommandExecuted ||
		value.ModelAssertion || value.ResultInferred || value.RecordRewritten || value.Approval ||
		value.AuthorityGranted || value.Replayed {
		t.Fatalf("association widened authority or lost causality: %#v", value)
	}
	replay := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", strings.NewReader(body))
	if replay.Code != http.StatusAccepted {
		t.Fatalf("association replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	if !recorded.Data.Replayed || recorded.Data.ID != value.ID {
		t.Fatalf("association replay diverged: %#v", recorded.Data)
	}

	coveragePath := strings.ReplaceAll(VerificationCoveragePathTemplate, "{run_id}", run.ID)
	coverage := performSessionMessageRequest(t, api, http.MethodGet, coveragePath,
		testAccessToken, "", "", nil)
	if coverage.Code != http.StatusOK {
		t.Fatalf("coverage status=%d body=%s", coverage.Code, coverage.Body.String())
	}
	var inventory struct {
		Data VerificationPlanCoverageInventoryView `json:"data"`
	}
	if err := json.Unmarshal(coverage.Body.Bytes(), &inventory); err != nil {
		t.Fatal(err)
	}
	projected := inventory.Data
	if projected.ProtocolVersion != verification.PlanCoverageProtocolVersion ||
		projected.PlanCount != 1 || projected.PlanItemCount != 1 ||
		projected.ObservedPlanItemCount != 1 || projected.AssociatedEvidenceCount != 1 ||
		len(projected.Plans) != 1 || len(projected.Plans[0].Items) != 1 ||
		projected.Plans[0].Items[0].PassCount != 1 ||
		projected.Plans[0].Items[0].FailCount != 0 ||
		len(projected.Associations) != 1 || !projected.MetadataOnly || !projected.ReadOnly ||
		projected.ResultInferred || projected.CommandExecuted || projected.ModelAssertion ||
		projected.RecordRewritten || projected.Approval || projected.AuthorityGranted ||
		strings.Contains(coverage.Body.String(), "Release checks") ||
		strings.Contains(coverage.Body.String(), "Observed a passing suite") {
		t.Fatalf("coverage inferred or exposed private text: %#v", projected)
	}
	handoffPath := strings.ReplaceAll(CodeHandoffPathTemplate, "{run_id}", run.ID)
	handoff := performSessionMessageRequest(t, api, http.MethodGet, handoffPath,
		testAccessToken, "", "", nil)
	if handoff.Code != http.StatusOK {
		t.Fatalf("coverage handoff status=%d body=%s", handoff.Code, handoff.Body.String())
	}
	var handoffEnvelope struct {
		Data CodeHandoffView `json:"data"`
	}
	if err := json.Unmarshal(handoff.Body.Bytes(), &handoffEnvelope); err != nil {
		t.Fatal(err)
	}
	handoffCoverage := handoffEnvelope.Data.VerificationCoverage
	if handoffCoverage.ProtocolVersion != verification.PlanCoverageProtocolVersion ||
		handoffCoverage.PlanCount != 1 || handoffCoverage.PlanItemCount != 1 ||
		handoffCoverage.ObservedPlanItemCount != 1 ||
		handoffCoverage.UnobservedPlanItemCount != 0 ||
		handoffCoverage.AssociatedEvidenceCount != 1 ||
		handoffCoverage.ContradictoryItemCount != 0 ||
		handoffCoverage.ReturnedItemCount != 1 || len(handoffCoverage.Items) != 1 ||
		handoffCoverage.Items[0].PassCount != 1 || !handoffCoverage.MetadataOnly ||
		!handoffCoverage.ReadOnly || handoffCoverage.ResultInferred ||
		handoffCoverage.PrivateBodiesIncluded ||
		strings.Contains(handoff.Body.String(), "Release checks") ||
		strings.Contains(handoff.Body.String(), "Observed a passing suite") {
		t.Fatalf("handoff coverage widened or lost explicit metadata: %#v", handoffCoverage)
	}

	changed := strings.Replace(body, evidenceResult.Evidence.ID, planResult.Plan.ID, 1)
	conflict := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", strings.NewReader(changed))
	assertAPIError(t, conflict, http.StatusConflict, "CONFLICT")
	unknownField := strings.TrimSuffix(body, "}") + `,"outcome":"pass"}`
	rejected := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, "http-verification-association-unknown-0001", "application/json",
		strings.NewReader(unknownField))
	assertAPIError(t, rejected, http.StatusBadRequest, "INVALID_ARGUMENT")
	wrongMethod := performSessionMessageRequest(t, api, http.MethodGet, path,
		testAccessToken, "", "", nil)
	assertAPIError(t, wrongMethod, http.StatusMethodNotAllowed, "INVALID_ARGUMENT")
	readOnly, err := New(st, Config{AccessToken: testAccessToken,
		AppVersion: "verification-association-read-only"})
	if err != nil {
		t.Fatal(err)
	}
	disabled := performSessionMessageRequest(t, readOnly, http.MethodPost, path,
		testControlToken, "http-verification-association-disabled-0001", "application/json",
		strings.NewReader(body))
	assertAPIError(t, disabled, http.StatusNotFound, "NOT_FOUND")
}
