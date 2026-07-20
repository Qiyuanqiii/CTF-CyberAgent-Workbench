package httpapi

import (
	"bytes"
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

func TestVerificationSnapshotReceiptHTTPRecordsExactDigestWithoutAcceptingResult(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "verification-snapshot-receipt-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	workspace := store.WorkspaceRecord{ID: "workspace-snapshot-receipt-http",
		Name: "snapshot-receipt", RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := st.SaveWorkspace(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(t.Context(),
		application.CreateRunRequest{Goal: "record an exact snapshot receipt", Profile: "code",
			ModelRoute: "mock/mock-code", WorkspaceID: workspace.ID,
			Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := application.NewVerificationPlanService(st).Record(t.Context(),
		application.RecordVerificationPlanRequest{Version: verification.PlanProtocolVersion,
			RunID: run.ID, Title: "Release checks", Summary: "Guidance only",
			Items: []application.VerificationPlanItemRequest{{Title: "Focused suite",
				ExpectedObservation: "Observe an explicit result"}},
			OperationKey: "http-snapshot-receipt-plan-0001", AuthoredBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	api, err := New(st, Config{AccessToken: testAccessToken, ControlToken: testControlToken,
		AppVersion: "snapshot-receipt-test", VerificationEvidenceEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	exportPath := strings.ReplaceAll(VerificationSnapshotExportPathTemplate, "{run_id}", run.ID)
	exportPath = strings.ReplaceAll(exportPath, "{plan_id}", plan.Plan.ID)
	exportPath = strings.ReplaceAll(exportPath, "{ordinal}", "1") + "?format=json"
	exportResponse := performSessionMessageRequest(t, api, http.MethodGet, exportPath,
		testAccessToken, "", "", nil)
	if exportResponse.Code != http.StatusOK {
		t.Fatalf("snapshot export status=%d body=%s", exportResponse.Code,
			exportResponse.Body.String())
	}
	var exported struct {
		Data VerificationSnapshotExportView `json:"data"`
	}
	if err := json.Unmarshal(exportResponse.Body.Bytes(), &exported); err != nil {
		t.Fatal(err)
	}
	requestView := VerificationSnapshotReceiptRequestView{
		Version: verification.SnapshotReceiptProtocolVersion, PlanID: plan.Plan.ID,
		PlanItemOrdinal: 1, Format: application.VerificationSnapshotExportFormatJSON,
		SnapshotHighWaterEventSequence: exported.Data.SnapshotHighWaterEventSequence,
		ContentSHA256:                  exported.Data.ContentSHA256, ConfirmMetadataSnapshot: true,
	}
	body, err := json.Marshal(requestView)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.ReplaceAll(VerificationSnapshotReceiptPathTemplate, "{run_id}", run.ID)
	operationKey := "http-snapshot-receipt-operation-0001"
	response := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", bytes.NewReader(body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("snapshot receipt status=%d body=%s", response.Code, response.Body.String())
	}
	var recorded struct {
		Data VerificationSnapshotReceiptControlView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	value := recorded.Data
	if value.ProtocolVersion != verification.SnapshotReceiptProtocolVersion ||
		value.RunID != run.ID || value.SessionID != run.SessionID ||
		value.WorkspaceID != workspace.ID || value.PlanID != plan.Plan.ID ||
		value.PlanItemOrdinal != 1 || value.ContentSHA256 != exported.Data.ContentSHA256 ||
		value.ContentBytes != exported.Data.ContentBytes || !value.Immutable ||
		!value.OperatorRecorded || !value.MetadataOnly || !value.ReadOnly ||
		value.ContentIncluded || value.PrivateBodiesIncluded || value.OperatorIdentityIncluded ||
		value.SnapshotAccepted || value.ResultAccepted || value.ResultInferred ||
		value.RecordRewritten || value.Approval || value.AuthorityGranted ||
		value.ExecutionStarted || value.Replayed ||
		value.ReceiptEventSequence <= value.SnapshotHighWaterEventSequence ||
		strings.Contains(response.Body.String(), "http_run_operator") {
		t.Fatalf("snapshot receipt widened acceptance or exposed identity: %#v", value)
	}
	replay := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", bytes.NewReader(body))
	if replay.Code != http.StatusAccepted {
		t.Fatalf("snapshot receipt replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	if !recorded.Data.Replayed || recorded.Data.ID != value.ID {
		t.Fatalf("snapshot receipt replay diverged: %#v", recorded.Data)
	}
	inventory := performSessionMessageRequest(t, api, http.MethodGet, path,
		testAccessToken, "", "", nil)
	if inventory.Code != http.StatusOK ||
		!strings.Contains(inventory.Body.String(),
			`"protocol_version":"operator_verification_plan_item_snapshot_receipt_inventory.v1"`) ||
		!strings.Contains(inventory.Body.String(), `"snapshot_accepted":false`) ||
		!strings.Contains(inventory.Body.String(), `"result_accepted":false`) ||
		strings.Contains(inventory.Body.String(), "http_run_operator") {
		t.Fatalf("snapshot receipt inventory status=%d body=%s", inventory.Code,
			inventory.Body.String())
	}
	requestView.ContentSHA256 = strings.Repeat("f", 64)
	changedBody, err := json.Marshal(requestView)
	if err != nil {
		t.Fatal(err)
	}
	changed := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", bytes.NewReader(changedBody))
	assertAPIError(t, changed, http.StatusConflict, "CONFLICT")
	readOnly, err := New(st, Config{AccessToken: testAccessToken,
		AppVersion: "snapshot-receipt-read-only"})
	if err != nil {
		t.Fatal(err)
	}
	disabled := performSessionMessageRequest(t, readOnly, http.MethodPost, path,
		testControlToken, "http-snapshot-receipt-disabled-0001", "application/json",
		bytes.NewReader(body))
	assertAPIError(t, disabled, http.StatusNotFound, "NOT_FOUND")
}
