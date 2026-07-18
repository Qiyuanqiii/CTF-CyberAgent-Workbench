package httpapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
	"cyberagent-workbench/internal/toolgateway"
)

func TestApprovalHTTPQueueIsMetadataOnlyAndApproveOnceIsClosedAuthority(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "approval-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	workspace := store.WorkspaceRecord{ID: "workspace-approval-http", Name: "approval-http",
		RootPath: t.TempDir(), CreatedAt: time.Now().UTC()}
	if err := st.SaveWorkspace(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	_, run, err := application.NewRunService(st).Create(t.Context(), application.CreateRunRequest{
		Goal: "inspect approval queue", Profile: "code", WorkspaceID: workspace.ID,
		Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = application.NewRunService(st).Start(t.Context(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	checker := policy.NewDefaultChecker()
	gateway := toolgateway.New(st, checker)
	const privateCommand = "echo private approval payload"
	proposed, err := gateway.Invoke(t.Context(), toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": privateCommand},
		RunID: run.ID, SessionID: run.SessionID, WorkspaceID: workspace.ID,
		RequestedBy: "approval_http_test",
	})
	if err != nil || proposed.Proposal == nil {
		t.Fatalf("proposal=%#v err=%v", proposed, err)
	}
	record, err := st.GetApprovalByProposal(t.Context(), proposed.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	controller := application.NewApprovalControlService(st, gateway, checker)
	api, err := New(st, Config{AccessToken: testAccessToken, ControlToken: testControlToken,
		ApprovalControlEnabled: true, ApprovalController: controller,
		AppVersion: "approval-control-test"})
	if err != nil {
		t.Fatal(err)
	}
	queuePath := "/api/v1/runs/" + run.ID + "/approvals"
	queue := performSessionMessageRequest(t, api, http.MethodGet, queuePath,
		testAccessToken, "", "", nil)
	if queue.Code != http.StatusOK || strings.Contains(queue.Body.String(), privateCommand) ||
		strings.Contains(queue.Body.String(), "request_fingerprint") ||
		strings.Contains(queue.Body.String(), "decision_reason") {
		t.Fatalf("approval queue leaked proposal content: status=%d body=%s", queue.Code, queue.Body.String())
	}
	var queueEnvelope struct {
		Data ApprovalQueueView `json:"data"`
	}
	if err := json.Unmarshal(queue.Body.Bytes(), &queueEnvelope); err != nil {
		t.Fatal(err)
	}
	if len(queueEnvelope.Data.Items) != 1 || queueEnvelope.Data.Items[0].ID != record.ID ||
		len(queueEnvelope.Data.Items[0].AllowedActions) != 2 ||
		queueEnvelope.Data.ProcessExecutionEnabled || queueEnvelope.Data.SessionGrantCreated ||
		queueEnvelope.Data.CapabilityGrant {
		t.Fatalf("unexpected approval queue: %#v", queueEnvelope.Data)
	}

	decisionPath := "/api/v1/runs/" + run.ID + "/approvals/" + record.ID + "/decision"
	decision := performSessionMessageRequest(t, api, http.MethodPost, decisionPath,
		testControlToken, "approval-http-approve-0001", "application/json",
		strings.NewReader(`{"version":"approval_control.v1","action":"approve_once"}`))
	if decision.Code != http.StatusAccepted || strings.Contains(decision.Body.String(), privateCommand) {
		t.Fatalf("approval decision status=%d body=%s", decision.Code, decision.Body.String())
	}
	var decisionEnvelope struct {
		Data ApprovalDecisionControlView `json:"data"`
	}
	if err := json.Unmarshal(decision.Body.Bytes(), &decisionEnvelope); err != nil {
		t.Fatal(err)
	}
	view := decisionEnvelope.Data
	if view.Status != string(approval.StatusApproved) || view.Replayed ||
		view.ProcessExecutionEnabled || view.ShellExecutionEnabled || view.DockerExecutionEnabled ||
		view.WorkspaceWriteApplied || view.SessionGrantCreated || view.CapabilityGrant {
		t.Fatalf("approval response widened authority: %#v", view)
	}
	replay := performSessionMessageRequest(t, api, http.MethodPost, decisionPath,
		testControlToken, "approval-http-approve-0001", "application/json",
		strings.NewReader(`{"version":"approval_control.v1","action":"approve_once"}`))
	if replay.Code != http.StatusAccepted || !strings.Contains(replay.Body.String(), `"replayed":true`) {
		t.Fatalf("approval replay status=%d body=%s", replay.Code, replay.Body.String())
	}
	empty := performSessionMessageRequest(t, api, http.MethodGet, queuePath,
		testAccessToken, "", "", nil)
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), `"items":[]`) {
		t.Fatalf("decided approval remained queued: status=%d body=%s", empty.Code, empty.Body.String())
	}
}

func TestApprovalHTTPControlCapabilityIsIndependentAndRequiresController(t *testing.T) {
	fixture := newAPIFixture(t)
	path := "/api/v1/runs/" + fixture.run.ID + "/approvals/missing-approval/decision"
	disabled := performSessionMessageRequest(t, fixture.api, http.MethodPost, path,
		testControlToken, "approval-http-disabled-0001", "application/json",
		strings.NewReader(`{"version":"approval_control.v1","action":"deny"}`))
	assertAPIError(t, disabled, http.StatusNotFound, "NOT_FOUND")
	if _, err := New(fixture.store, Config{AccessToken: testAccessToken,
		ControlToken: testControlToken, ApprovalControlEnabled: true,
		AppVersion: "approval-control-test"}); err == nil {
		t.Fatal("approval capability accepted a missing controller")
	}
}
