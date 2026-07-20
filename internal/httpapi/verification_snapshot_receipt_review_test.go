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

func TestVerificationSnapshotReceiptReviewHTTPIsExplicitAndNonAuthorizing(t *testing.T) {
	ctx := t.Context()
	st, err := store.Open(filepath.Join(t.TempDir(), "verification-snapshot-review-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	workspace := store.WorkspaceRecord{ID: "workspace-snapshot-review-http",
		Name: "snapshot-review", RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := st.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(ctx,
		application.CreateRunRequest{Goal: "review exact receipt metadata", Profile: "code",
			ModelRoute: "mock/mock-code", WorkspaceID: workspace.ID,
			Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := application.NewVerificationPlanService(st).Record(ctx,
		application.RecordVerificationPlanRequest{Version: verification.PlanProtocolVersion,
			RunID: run.ID, Title: "Review checks", Summary: "Guidance only",
			Items: []application.VerificationPlanItemRequest{{Title: "Focused suite",
				ExpectedObservation: "Observe an explicit result"}},
			OperationKey: "http-snapshot-review-plan-0001", AuthoredBy: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	exported, err := application.NewVerificationSnapshotExportService(st).Build(ctx, run.ID,
		plan.Plan.ID, 1, application.VerificationSnapshotExportFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	receiptResult, err := application.NewVerificationSnapshotReceiptService(st).Record(ctx,
		application.RecordVerificationSnapshotReceiptRequest{
			Version: verification.SnapshotReceiptProtocolVersion, RunID: run.ID,
			PlanID: plan.Plan.ID, PlanItemOrdinal: 1, Format: exported.Format,
			SnapshotHighWaterEventSequence: exported.SnapshotHighWaterEventSequence,
			ContentSHA256:                  exported.ContentSHA256, ConfirmMetadataSnapshot: true,
			OperationKey: "http-snapshot-review-receipt-0001", RecordedBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	receipt := receiptResult.Receipt
	api, err := New(st, Config{AccessToken: testAccessToken, ControlToken: testControlToken,
		AppVersion: "snapshot-review-test", VerificationEvidenceEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	requestView := VerificationSnapshotReceiptReviewRequestView{
		Version: verification.SnapshotReceiptReviewProtocolVersion, ReceiptID: receipt.ID,
		ReceiptContentSHA256: receipt.ContentSHA256, ReceiptEventSequence: receipt.EventSequence,
		Decision:                    string(verification.SnapshotReceiptReviewMetadataConfirmed),
		ConfirmNonAuthorizingReview: true,
	}
	body, err := json.Marshal(requestView)
	if err != nil {
		t.Fatal(err)
	}
	path := strings.ReplaceAll(VerificationSnapshotReceiptReviewPathTemplate, "{run_id}", run.ID)
	operationKey := "http-snapshot-receipt-review-operation-0001"
	response := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", bytes.NewReader(body))
	if response.Code != http.StatusAccepted {
		t.Fatalf("snapshot receipt review status=%d body=%s", response.Code,
			response.Body.String())
	}
	var recorded struct {
		Data VerificationSnapshotReceiptReviewControlView `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	value := recorded.Data
	if value.ProtocolVersion != verification.SnapshotReceiptReviewProtocolVersion ||
		value.RunID != run.ID || value.SessionID != run.SessionID ||
		value.WorkspaceID != workspace.ID || value.ReceiptID != receipt.ID ||
		value.ReceiptContentSHA256 != receipt.ContentSHA256 ||
		value.ReceiptEventSequence != receipt.EventSequence ||
		value.Decision != string(verification.SnapshotReceiptReviewMetadataConfirmed) ||
		value.ReviewEventSequence <= value.ReceiptEventSequence || !value.Immutable ||
		!value.OperatorReviewed || !value.MetadataOnly || !value.ReadOnly ||
		!value.ReviewNonAuthorizing || value.ContentIncluded || value.PrivateBodiesIncluded ||
		value.OperatorIdentityIncluded || value.SnapshotAccepted || value.ResultAccepted ||
		value.ResultInferred || value.RecordRewritten || value.Approval ||
		value.AuthorityGranted || value.ExecutionStarted || value.Replayed ||
		strings.Contains(response.Body.String(), "http_run_operator") {
		t.Fatalf("snapshot receipt review widened authority or exposed identity: %#v", value)
	}
	replay := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", bytes.NewReader(body))
	if replay.Code != http.StatusAccepted {
		t.Fatalf("snapshot receipt review replay status=%d body=%s", replay.Code,
			replay.Body.String())
	}
	if err := json.Unmarshal(replay.Body.Bytes(), &recorded); err != nil {
		t.Fatal(err)
	}
	if !recorded.Data.Replayed || recorded.Data.ID != value.ID {
		t.Fatalf("snapshot receipt review replay diverged: %#v", recorded.Data)
	}
	inventory := performSessionMessageRequest(t, api, http.MethodGet, path,
		testAccessToken, "", "", nil)
	if inventory.Code != http.StatusOK ||
		!strings.Contains(inventory.Body.String(),
			`"protocol_version":"operator_verification_plan_item_snapshot_receipt_review_inventory.v1"`) ||
		!strings.Contains(inventory.Body.String(), `"review_non_authorizing":true`) ||
		!strings.Contains(inventory.Body.String(), `"snapshot_accepted":false`) ||
		!strings.Contains(inventory.Body.String(), `"result_accepted":false`) ||
		strings.Contains(inventory.Body.String(), "http_run_operator") {
		t.Fatalf("snapshot receipt review inventory status=%d body=%s", inventory.Code,
			inventory.Body.String())
	}
	requestView.Decision = string(verification.SnapshotReceiptReviewMetadataDisputed)
	changedBody, err := json.Marshal(requestView)
	if err != nil {
		t.Fatal(err)
	}
	changed := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, operationKey, "application/json", bytes.NewReader(changedBody))
	assertAPIError(t, changed, http.StatusConflict, "CONFLICT")
	second := performSessionMessageRequest(t, api, http.MethodPost, path,
		testControlToken, "http-snapshot-receipt-review-operation-0002", "application/json",
		bytes.NewReader(body))
	assertAPIError(t, second, http.StatusConflict, "CONFLICT")
	readOnly, err := New(st, Config{AccessToken: testAccessToken,
		AppVersion: "snapshot-review-read-only"})
	if err != nil {
		t.Fatal(err)
	}
	disabled := performSessionMessageRequest(t, readOnly, http.MethodPost, path,
		testControlToken, "http-snapshot-review-disabled-0001", "application/json",
		bytes.NewReader(body))
	assertAPIError(t, disabled, http.StatusNotFound, "NOT_FOUND")
}
