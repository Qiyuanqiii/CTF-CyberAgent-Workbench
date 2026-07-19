package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/modelregistry"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/store"
)

func TestModelDiffAndWakeHTTPControlsRemainCapabilitySeparated(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "model-diff-wake-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	root := t.TempDir()
	workspace := store.WorkspaceRecord{ID: "workspace-model-diff-wake", Name: "control-test",
		RootPath: root, CreatedAt: time.Now().UTC()}
	if err := st.SaveWorkspace(t.Context(), workspace); err != nil {
		t.Fatal(err)
	}
	_, created, err := application.NewRunService(st).Create(t.Context(),
		application.CreateRunRequest{Goal: "test bounded controls", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4, MaxToolCalls: 4}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := application.NewRunService(st).Start(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnqueueOperatorSteering(t.Context(), domain.EnqueueOperatorSteeringRequest{
		RunID: run.ID, SessionID: run.SessionID, Content: "queued wake input",
		OperationKey: "http-wake-queue-0001", RequestedBy: "http_test",
	}); err != nil {
		t.Fatal(err)
	}
	edit, err := fileedit.NewManager(st).Propose(t.Context(), fileedit.Proposal{
		SessionID: run.SessionID, WorkspaceID: workspace.ID, WorkspaceRoot: root,
		Path: "review-only.txt", ProposedText: "reviewed but not written\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	models := modelregistry.New(nil)
	checker := policy.NewDefaultChecker()
	executionController := application.NewRunExecutionHandoffService(st,
		llm.NewDefaultRouter(), checker)
	api, err := New(st, Config{AccessToken: testAccessToken, ControlToken: testControlToken,
		ModelControlEnabled: true, FileEditReviewEnabled: true, RunWakeControlEnabled: true,
		FileEditApplyEnabled: true, RunWakeExecutionEnabled: true,
		ModelControlController:   application.NewModelControlService(models, st),
		FileEditReviewController: application.NewFileEditReviewService(st),
		FileEditApplyController:  application.NewFileEditApplyService(st, checker),
		RunWakeController:        application.NewRunWakeControlService(st), ModelRegistry: models,
		RunWakeExecutionController: application.NewForegroundRunWakeConsumer(st,
			executionController),
		AppVersion: "model-diff-wake-test"})
	if err != nil {
		t.Fatal(err)
	}

	route := performSessionMessageRequest(t, api, http.MethodPost,
		"/api/v1/models/routes/code", testControlToken, "", "application/json",
		strings.NewReader(`{"version":"model_route_control.v1","provider":"mock","model":"mock-code"}`))
	if route.Code != http.StatusAccepted || !strings.Contains(route.Body.String(), `"available":true`) {
		t.Fatalf("model route status=%d body=%s", route.Code, route.Body.String())
	}
	if persisted, found, err := st.GetProviderSetting(t.Context(), "route.code"); err != nil || !found || persisted != "mock/mock-code" {
		t.Fatalf("model route was not persisted first: value=%q found=%t err=%v", persisted, found, err)
	}
	diagnostic := performSessionMessageRequest(t, api, http.MethodPost,
		ProviderDiagnosticPath, testControlToken, "", "application/json",
		strings.NewReader(`{"version":"provider_diagnostic.v1","provider":"mock","model":"mock-code","confirm_diagnostic":true}`))
	if diagnostic.Code != http.StatusAccepted ||
		!strings.Contains(diagnostic.Body.String(), `"response_content_returned":false`) ||
		strings.Contains(diagnostic.Body.String(), `"response"`) ||
		strings.Contains(diagnostic.Body.String(), `"text"`) {
		t.Fatalf("diagnostic crossed its content-free boundary: %s", diagnostic.Body.String())
	}

	listPath := "/api/v1/runs/" + run.ID + "/file-edits"
	list := performSessionMessageRequest(t, api, http.MethodGet, listPath,
		testAccessToken, "", "", nil)
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), `"original_text"`) ||
		strings.Contains(list.Body.String(), `"proposed_text"`) ||
		!strings.Contains(list.Body.String(), `"apply_enabled":false`) {
		t.Fatalf("file edit preview leaked bodies or apply authority: %s", list.Body.String())
	}
	changeSet := performSessionMessageRequest(t, api, http.MethodGet,
		"/api/v1/runs/"+run.ID+"/file-edit-change-set",
		testAccessToken, "", "", nil)
	if changeSet.Code != http.StatusOK ||
		!strings.Contains(changeSet.Body.String(),
			`"protocol_version":"file_edit_change_set.v1"`) ||
		!strings.Contains(changeSet.Body.String(), `"proposed_count":1`) ||
		!strings.Contains(changeSet.Body.String(), `"review_independent":true`) ||
		!strings.Contains(changeSet.Body.String(), `"apply_independent":true`) ||
		!strings.Contains(changeSet.Body.String(), `"atomic_apply":false`) ||
		!strings.Contains(changeSet.Body.String(), `"batch_mutation_supported":false`) ||
		!strings.Contains(changeSet.Body.String(), `"partial_apply_visible":true`) ||
		!strings.Contains(changeSet.Body.String(), `"diff_content_included":false`) ||
		strings.Contains(changeSet.Body.String(), `"diff":`) ||
		strings.Contains(changeSet.Body.String(), `"original_hash":`) ||
		strings.Contains(changeSet.Body.String(), `"proposed_hash":`) {
		t.Fatalf("file edit change set widened batch authority: %s", changeSet.Body.String())
	}
	review := performSessionMessageRequest(t, api, http.MethodPost,
		listPath+"/"+edit.ID+"/review", testControlToken, "", "application/json",
		strings.NewReader(`{"version":"file_edit_review.v1","action":"approve_intent"}`))
	if review.Code != http.StatusAccepted ||
		!strings.Contains(review.Body.String(), `"file_written":false`) ||
		!strings.Contains(review.Body.String(), `"status":"approved"`) {
		t.Fatalf("file edit review status=%d body=%s", review.Code, review.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "review-only.txt")); !os.IsNotExist(err) {
		t.Fatalf("review-only approval wrote the Workspace file: %v", err)
	}
	applyPath := listPath + "/" + edit.ID + "/apply"
	apply := performSessionMessageRequest(t, api, http.MethodPost, applyPath,
		testControlToken, "http-file-apply-0001", "application/json",
		strings.NewReader(`{"version":"file_edit_apply.v1"}`))
	if apply.Code != http.StatusAccepted ||
		!strings.Contains(apply.Body.String(), `"status":"applied"`) ||
		!strings.Contains(apply.Body.String(), `"file_written":true`) ||
		!strings.Contains(apply.Body.String(), `"policy_rechecked":true`) {
		t.Fatalf("file edit apply status=%d body=%s", apply.Code, apply.Body.String())
	}
	written, err := os.ReadFile(filepath.Join(root, "review-only.txt"))
	if err != nil || string(written) != "reviewed but not written\n" {
		t.Fatalf("applied file=%q err=%v", written, err)
	}
	applyReplay := performSessionMessageRequest(t, api, http.MethodPost, applyPath,
		testControlToken, "http-file-apply-0001", "application/json",
		strings.NewReader(`{"version":"file_edit_apply.v1"}`))
	if applyReplay.Code != http.StatusAccepted ||
		!strings.Contains(applyReplay.Body.String(), `"replayed":true`) ||
		!strings.Contains(applyReplay.Body.String(), `"file_written":false`) {
		t.Fatalf("file edit apply replay status=%d body=%s",
			applyReplay.Code, applyReplay.Body.String())
	}

	wakePath := "/api/v1/runs/" + run.ID + "/wake-intent"
	wake := performSessionMessageRequest(t, api, http.MethodPost, wakePath,
		testControlToken, "http-wake-schedule-0001", "application/json",
		strings.NewReader(`{"version":"run_wake_control.v1","max_attempts":3,`+
			`"initial_delay_seconds":0,"base_backoff_seconds":5,`+
			`"max_backoff_seconds":60,"max_elapsed_seconds":300}`))
	if wake.Code != http.StatusAccepted ||
		!strings.Contains(wake.Body.String(), `"execution_started":false`) ||
		!strings.Contains(wake.Body.String(), `"background_loop_enabled":false`) ||
		strings.Contains(wake.Body.String(), "owner_id") || strings.Contains(wake.Body.String(), "lease_id") {
		t.Fatalf("wake schedule widened authority: %s", wake.Body.String())
	}
	cancel := performSessionMessageRequest(t, api, http.MethodPost, wakePath+"/cancel",
		testControlToken, "http-wake-cancel-0001", "application/json",
		strings.NewReader(`{"version":"run_wake_control.v1"}`))
	if cancel.Code != http.StatusAccepted ||
		!strings.Contains(cancel.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("wake cancellation status=%d body=%s", cancel.Code, cancel.Body.String())
	}

	_, wakeCreated, err := application.NewRunService(st).Create(t.Context(),
		application.CreateRunRequest{Goal: "consume one foreground wake", Profile: "code",
			WorkspaceID: workspace.ID, Budget: domain.Budget{MaxTurns: 4}})
	if err != nil {
		t.Fatal(err)
	}
	wakeRun, err := application.NewRunService(st).Start(t.Context(), wakeCreated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnqueueOperatorSteering(t.Context(), domain.EnqueueOperatorSteeringRequest{
		RunID: wakeRun.ID, SessionID: wakeRun.SessionID, Content: "foreground HTTP wake",
		OperationKey: "http-wake-consume-queue-0001", RequestedBy: "http_test",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewRunWakeControlService(st).Schedule(t.Context(),
		application.ScheduleRunWakeRequest{Version: domain.RunWakeControlProtocolVersion,
			RunID: wakeRun.ID, OperationKey: "http-wake-consume-schedule-0001",
			RequestedBy: "http_test", MaxAttempts: 2, BaseBackoffSeconds: 5,
			MaxBackoffSeconds: 30, MaxElapsedSeconds: 120}); err != nil {
		t.Fatal(err)
	}
	consumePath := "/api/v1/runs/" + wakeRun.ID + "/wake-intent/consume"
	consume := performSessionMessageRequest(t, api, http.MethodPost, consumePath,
		testControlToken, "", "application/json",
		strings.NewReader(`{"version":"run_wake_consumer.v1","max_steps":1}`))
	if consume.Code != http.StatusAccepted ||
		!strings.Contains(consume.Body.String(), `"consumption_status":"completed"`) ||
		!strings.Contains(consume.Body.String(), `"status":"completed"`) ||
		!strings.Contains(consume.Body.String(), `"background_loop_enabled":false`) ||
		strings.Contains(consume.Body.String(), "owner_id") ||
		strings.Contains(consume.Body.String(), "lease_id") {
		t.Fatalf("foreground wake status=%d body=%s", consume.Code, consume.Body.String())
	}
	consumeReplay := performSessionMessageRequest(t, api, http.MethodPost, consumePath,
		testControlToken, "", "application/json",
		strings.NewReader(`{"version":"run_wake_consumer.v1","max_steps":1}`))
	if consumeReplay.Code != http.StatusAccepted ||
		!strings.Contains(consumeReplay.Body.String(), `"replayed":true`) {
		t.Fatalf("foreground wake replay status=%d body=%s",
			consumeReplay.Code, consumeReplay.Body.String())
	}
}

func TestModelDiffAndWakeHTTPControlsRequireIndependentCapabilities(t *testing.T) {
	fixture := newAPIFixture(t)
	requests := []struct {
		path string
		body string
		key  string
	}{
		{path: ProviderDiagnosticPath,
			body: `{"version":"provider_diagnostic.v1","provider":"mock","model":"mock-code","confirm_diagnostic":true}`},
		{path: "/api/v1/runs/" + fixture.run.ID + "/file-edits/edit-missing/review",
			body: `{"version":"file_edit_review.v1","action":"deny"}`},
		{path: "/api/v1/runs/" + fixture.run.ID + "/file-edits/edit-missing/apply",
			body: `{"version":"file_edit_apply.v1"}`, key: "http-disabled-apply-0001"},
		{path: "/api/v1/runs/" + fixture.run.ID + "/wake-intent",
			body: `{"version":"run_wake_control.v1","max_attempts":3,"initial_delay_seconds":0,` +
				`"base_backoff_seconds":5,"max_backoff_seconds":60,"max_elapsed_seconds":300}`,
			key: "http-disabled-wake-0001"},
		{path: "/api/v1/runs/" + fixture.run.ID + "/wake-intent/consume",
			body: `{"version":"run_wake_consumer.v1","max_steps":1}`},
		{path: SkillPackageInstallPath,
			body: `{"version":"skill_package_installation.v1","archive_base64":"AA==",` +
				`"surface":"code","confirm_untrusted":true}`,
			key: "http-disabled-skill-0001"},
	}
	for _, current := range requests {
		response := performSessionMessageRequest(t, fixture.api, http.MethodPost,
			current.path, testControlToken, current.key, "application/json",
			strings.NewReader(current.body))
		assertAPIError(t, response, http.StatusNotFound, "NOT_FOUND")
	}
}
