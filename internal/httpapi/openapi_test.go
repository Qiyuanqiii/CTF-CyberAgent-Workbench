package httpapi

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	"cyberagent-workbench/internal/credential"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/llm"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/toolgateway"
	"cyberagent-workbench/internal/verification"
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
					item.Post.OperationID == "selectRunExecutionProfile") ||
				(path == RunCreationControlPath && item.Post.OperationID == "createRun") ||
				(path == SessionMessageControlPathTemplate &&
					item.Post.OperationID == "submitSessionMessage") ||
				(path == SessionSteeringCancellationPathTemplate &&
					item.Post.OperationID == "cancelSessionSteering") ||
				(path == RunLifecycleControlPathTemplate &&
					item.Post.OperationID == "controlRunLifecycle") ||
				(path == PlanDirectionControlPathTemplate &&
					item.Post.OperationID == "selectPlanDirection") ||
				(path == PlanDeliveryControlPathTemplate &&
					item.Post.OperationID == "enterPlanDelivery") ||
				(path == ApprovalDecisionControlPathTemplate &&
					item.Post.OperationID == "decideRunApproval") ||
				(path == RunExecutionControlPathTemplate &&
					item.Post.OperationID == "executeRunSelection") ||
				(path == ModelRouteControlPathTemplate &&
					item.Post.OperationID == "selectModelRoute") ||
				(path == ProviderDiagnosticPath &&
					item.Post.OperationID == "diagnoseProvider") ||
				(path == ProviderCredentialPathTemplate &&
					item.Post.OperationID == "changeProviderCredential") ||
				(path == FileEditProposalPathTemplate &&
					item.Post.OperationID == "createFileEditProposal") ||
				(path == FileEditReviewPathTemplate &&
					item.Post.OperationID == "reviewRunFileEdit") ||
				(path == FileEditApplyPathTemplate &&
					item.Post.OperationID == "applyRunFileEdit") ||
				(path == RunWakeIntentPathTemplate &&
					item.Post.OperationID == "scheduleRunWake") ||
				(path == RunWakeCancellationPathTemplate &&
					item.Post.OperationID == "cancelRunWake") ||
				(path == RunWakeExecutionPathTemplate &&
					item.Post.OperationID == "consumeRunWake") ||
				(path == SkillPackageInstallPath &&
					item.Post.OperationID == "installSkillPackage") ||
				(path == EvidenceAttachmentPathTemplate &&
					item.Post.OperationID == "attachRunEvidence") ||
				(path == VerificationEvidencePathTemplate &&
					item.Post.OperationID == "recordRunVerificationEvidence") ||
				(path == VerificationPlanPathTemplate &&
					item.Post.OperationID == "recordRunVerificationPlan") ||
				(path == VerificationAssociationPathTemplate &&
					item.Post.OperationID == "associateRunVerificationEvidence") ||
				(path == VerificationSnapshotReceiptPathTemplate &&
					item.Post.OperationID == "recordRunVerificationSnapshotReceipt") ||
				(path == VerificationSnapshotReceiptReviewPathTemplate &&
					item.Post.OperationID == "recordRunVerificationSnapshotReceiptReview")
			if !validControl ||
				item.Post.ReadOnly || item.Post.Responses["202"] == nil || item.Post.RequestBody == nil ||
				len(item.Post.Security) != 1 || item.Post.Security[0]["ControlBearerAuth"] == nil {
				t.Fatalf("path %s has an incomplete control operation: %#v", path, item.Post)
			}
			operations = append(operations, item.Post)
		}
		expectedOperations := 1
		if path == RunCreationControlPath || path == SessionMessageControlPathTemplate ||
			path == RunWakeIntentPathTemplate || path == EvidenceAttachmentPathTemplate ||
			path == VerificationEvidencePathTemplate || path == VerificationPlanPathTemplate ||
			path == VerificationSnapshotReceiptPathTemplate ||
			path == VerificationSnapshotReceiptReviewPathTemplate {
			expectedOperations = 2
		}
		if len(operations) != expectedOperations {
			t.Fatalf("path %s exposes %d operations, want %d: %#v",
				path, len(operations), expectedOperations, item)
		}
		for _, operation := range operations {
			if _, duplicate := operationIDs[operation.OperationID]; duplicate {
				t.Fatalf("duplicate OpenAPI operation id %q", operation.OperationID)
			}
			operationIDs[operation.OperationID] = struct{}{}
		}
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
				path == RunExecutionProfileControlPathTemplate ||
				path == RunCreationControlPath || path == SessionMessageControlPathTemplate ||
				path == SessionSteeringCancellationPathTemplate ||
				path == RunLifecycleControlPathTemplate ||
				path == PlanDirectionControlPathTemplate ||
				path == PlanDeliveryControlPathTemplate ||
				path == ApprovalDecisionControlPathTemplate ||
				path == RunExecutionControlPathTemplate ||
				path == ModelRouteControlPathTemplate ||
				path == ProviderDiagnosticPath || path == ProviderCredentialPathTemplate ||
				path == FileEditProposalPathTemplate || path == FileEditReviewPathTemplate ||
				path == FileEditApplyPathTemplate ||
				path == RunWakeIntentPathTemplate ||
				path == RunWakeCancellationPathTemplate ||
				path == RunWakeExecutionPathTemplate ||
				path == SkillPackageInstallPath ||
				path == EvidenceAttachmentPathTemplate ||
				path == VerificationEvidencePathTemplate ||
				path == VerificationPlanPathTemplate ||
				path == VerificationAssociationPathTemplate ||
				path == VerificationSnapshotReceiptPathTemplate ||
				path == VerificationSnapshotReceiptReviewPathTemplate) &&
				method == "post") {
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
	assertOpenAPISchemaOmits(t, document.Components.Schemas,
		"ProviderCredentialStatusView", "secret")
	assertOpenAPIPropertyFlag(t, document.Components.Schemas,
		"ProviderCredentialRequestView", "secret", "writeOnly", true)
	for _, field := range []string{"path", "content", "command", "hook"} {
		assertOpenAPISchemaOmits(t, document.Components.Schemas,
			"SkillPackageInstallRequestView", field)
	}
	for _, field := range []string{"command", "content", "path", "request_fingerprint",
		"decision_reason", "requested_by", "reviewed_by", "grant_id"} {
		assertOpenAPISchemaOmits(t, document.Components.Schemas,
			"ApprovalQueueItemView", field)
	}
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
	_, lifecycleRun, err := application.NewRunService(fixture.store).Create(t.Context(),
		application.CreateRunRequest{Goal: "OpenAPI lifecycle target", Profile: "code",
			Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	_, executionCreated, err := application.NewRunService(fixture.store).Create(t.Context(),
		application.CreateRunRequest{Goal: "OpenAPI execution target", Profile: "code",
			ModelRoute: "mock/mock-code", Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	executionRun, err := application.NewRunService(fixture.store).Start(t.Context(),
		executionCreated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.EnqueueOperatorSteering(t.Context(),
		domain.EnqueueOperatorSteeringRequest{
			RunID: executionRun.ID, SessionID: executionRun.SessionID,
			Content:      "OpenAPI execution input",
			OperationKey: "openapi-execution-queue-0001", RequestedBy: "openapi_test",
		}); err != nil {
		t.Fatal(err)
	}
	planRun, planProposal := prepareOpenAPIPlanControlTarget(t, fixture)
	checker := policy.NewDefaultChecker()
	gateway := toolgateway.New(fixture.store, checker)
	pendingApproval, err := gateway.Invoke(t.Context(), toolgateway.ToolCall{
		Name: toolgateway.ShellTool, Arguments: map[string]string{"command": "echo OpenAPI approval"},
		RunID: fixture.run.ID, SessionID: fixture.run.SessionID,
		WorkspaceID: fixture.workspace.ID, RequestedBy: "openapi_test",
	})
	if err != nil || pendingApproval.Proposal == nil {
		t.Fatalf("prepare OpenAPI approval=%#v err=%v", pendingApproval, err)
	}
	approvalRecord, err := fixture.store.GetApprovalByProposal(t.Context(), pendingApproval.Proposal.ID)
	if err != nil {
		t.Fatal(err)
	}
	fileEditRecord, err := fileedit.NewManager(fixture.store).Propose(t.Context(), fileedit.Proposal{
		SessionID: fixture.run.SessionID, WorkspaceID: fixture.workspace.ID,
		WorkspaceRoot: fixture.workspace.RootPath, Path: "openapi-review.txt",
		ProposedText: "bounded OpenAPI review\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, wakeCreated, err := application.NewRunService(fixture.store).Create(t.Context(),
		application.CreateRunRequest{Goal: "OpenAPI wake target", Profile: "code",
			Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	wakeRun, err := application.NewRunService(fixture.store).Start(t.Context(), wakeCreated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.EnqueueOperatorSteering(t.Context(),
		domain.EnqueueOperatorSteeringRequest{RunID: wakeRun.ID, SessionID: wakeRun.SessionID,
			Content: "OpenAPI wake input", OperationKey: "openapi-wake-queue-0001",
			RequestedBy: "openapi_test"}); err != nil {
		t.Fatal(err)
	}
	_, wakeExecutionCreated, err := application.NewRunService(fixture.store).Create(t.Context(),
		application.CreateRunRequest{Goal: "OpenAPI wake execution target", Profile: "code",
			ModelRoute: "mock/mock-code", Budget: domain.Budget{MaxTurns: 2}})
	if err != nil {
		t.Fatal(err)
	}
	wakeExecutionRun, err := application.NewRunService(fixture.store).Start(t.Context(),
		wakeExecutionCreated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.EnqueueOperatorSteering(t.Context(),
		domain.EnqueueOperatorSteeringRequest{RunID: wakeExecutionRun.ID,
			SessionID: wakeExecutionRun.SessionID, Content: "OpenAPI foreground wake input",
			OperationKey: "openapi-wake-execution-queue-0001",
			RequestedBy:  "openapi_test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.NewRunWakeControlService(fixture.store).Schedule(t.Context(),
		application.ScheduleRunWakeRequest{Version: domain.RunWakeControlProtocolVersion,
			RunID: wakeExecutionRun.ID, OperationKey: "openapi-wake-execution-schedule-0001",
			RequestedBy: "openapi_test", MaxAttempts: 2, BaseBackoffSeconds: 5,
			MaxBackoffSeconds: 30, MaxElapsedSeconds: 120}); err != nil {
		t.Fatal(err)
	}
	skillArchive := buildOpenAPISkillPackage(t)
	objects, err := skills.NewLocalPackageObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	builtins, err := skills.BuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	fixture.api.runLifecycleEnabled = true
	fixture.api.runExecutionEnabled = true
	fixture.api.planDeliveryControlEnabled = true
	fixture.api.approvalControlEnabled = true
	fixture.api.modelControlEnabled = true
	fixture.api.providerCredentialEnabled = true
	fixture.api.fileEditReviewEnabled = true
	fixture.api.fileEditProposalEnabled = true
	fixture.api.runWakeControlEnabled = true
	fixture.api.fileEditApplyEnabled = true
	fixture.api.runWakeExecutionEnabled = true
	fixture.api.skillInstallationEnabled = true
	fixture.api.evidenceAttachmentEnabled = true
	fixture.api.verificationEvidenceEnabled = true
	fixture.api.runLifecycleController = application.NewRunLifecycleControlService(fixture.store)
	executionController := application.NewRunExecutionHandoffService(
		fixture.store, llm.NewDefaultRouter(), policy.NewDefaultChecker())
	fixture.api.runExecutionController = executionController
	fixture.api.planDeliveryController = application.NewPlanDeliveryControlService(fixture.store)
	fixture.api.approvalController = application.NewApprovalControlService(fixture.store,
		gateway, checker)
	fixture.api.modelControlController = application.NewModelControlService(
		fixture.api.modelRegistry, fixture.store)
	credentialStore := credential.NewMemoryStore()
	fixture.api.providerCredentialController = application.NewProviderCredentialService(
		credentialStore)
	fixture.api.fileEditReviewController = application.NewFileEditReviewService(fixture.store)
	fileEditProposalController := application.NewFileEditProposalService(fixture.store,
		checker)
	fixture.api.fileEditProposalController = fileEditProposalController
	fixture.api.runWakeController = application.NewRunWakeControlService(fixture.store)
	fixture.api.fileEditApplyController = application.NewFileEditApplyService(fixture.store, checker)
	fixture.api.runWakeExecutionController = application.NewForegroundRunWakeConsumer(
		fixture.store, executionController)
	fixture.api.skillInstallationController = application.NewSkillPackageRegistryService(
		fixture.store, objects, builtins)
	steering, err := fixture.store.EnqueueOperatorSteering(t.Context(),
		domain.EnqueueOperatorSteeringRequest{
			RunID: fixture.run.ID, SessionID: fixture.run.SessionID,
			Content:      "OpenAPI cancellation target",
			OperationKey: "openapi-cancellation-target-0001",
			RequestedBy:  "openapi_test",
		})
	if err != nil {
		t.Fatal(err)
	}
	evidenceContent := "OpenAPI evidence\n"
	if err := os.WriteFile(filepath.Join(fixture.workspace.RootPath, "README.md"),
		[]byte(evidenceContent), 0o644); err != nil {
		t.Fatal(err)
	}
	evidenceDigest := sha256.Sum256([]byte(evidenceContent))
	proposalSource, err := fileEditProposalController.IssueSource(t.Context(),
		fixture.run.ID, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	recoverySource, err := fileEditProposalController.IssueSource(t.Context(),
		fixture.run.ID, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	recoveryProposal, err := fileEditProposalController.Propose(t.Context(),
		application.CreateFileEditProposalRequest{
			Version: application.FileEditProposalProtocolVersion, RunID: fixture.run.ID,
			SourceHandle: recoverySource.Handle, ProposedText: "OpenAPI recovery proposal\n",
		})
	if err != nil {
		t.Fatal(err)
	}
	verificationPlan, err := application.NewVerificationPlanService(fixture.store).Record(
		t.Context(), application.RecordVerificationPlanRequest{
			Version: verification.PlanProtocolVersion, RunID: fixture.run.ID,
			Title: "OpenAPI association plan", Summary: "Live route metadata",
			Items: []application.VerificationPlanItemRequest{{Title: "Live association",
				ExpectedObservation: "Observe an explicit operator result"}},
			OperationKey: "openapi-association-plan-operation-0001", AuthoredBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	verificationEvidence, err := application.NewVerificationEvidenceService(fixture.store).Record(
		t.Context(), application.RecordVerificationEvidenceRequest{
			Version: verification.EvidenceProtocolVersion, RunID: fixture.run.ID,
			Outcome: string(verification.OutcomePass), Title: "OpenAPI association evidence",
			Summary:      "Explicit live route observation",
			OperationKey: "openapi-association-evidence-operation-0001", RecordedBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	openAPISnapshot, err := application.NewVerificationSnapshotExportService(fixture.store).Build(
		t.Context(), fixture.run.ID, verificationPlan.Plan.ID, 1,
		application.VerificationSnapshotExportFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	openAPIReceipt, err := application.NewVerificationSnapshotReceiptService(fixture.store).Record(
		t.Context(), application.RecordVerificationSnapshotReceiptRequest{
			Version: verification.SnapshotReceiptProtocolVersion, RunID: fixture.run.ID,
			PlanID: verificationPlan.Plan.ID, PlanItemOrdinal: 1,
			Format:                         openAPISnapshot.Format,
			SnapshotHighWaterEventSequence: openAPISnapshot.SnapshotHighWaterEventSequence,
			ContentSHA256:                  openAPISnapshot.ContentSHA256, ConfirmMetadataSnapshot: true,
			OperationKey: "openapi-snapshot-review-receipt-0001", RecordedBy: "operator",
		})
	if err != nil {
		t.Fatal(err)
	}
	replacements := map[string]string{
		"{run_id}":       fixture.run.ID,
		"{workspace_id}": fixture.workspace.ID,
		"{agent_id}":     child.ID,
		"{session_id}":   fixture.run.SessionID,
		"{message_id}":   steering.Message.ID,
		"{work_item_id}": fixture.workItems[0].ID,
		"{note_id}":      fixture.notes[0].ID,
		"{artifact_id}":  fixture.artifactID,
		"{report_id}":    "report-openapi-missing-0001",
		"{approval_id}":  approvalRecord.ID,
		"{edit_id}":      fileEditRecord.ID,
		"{object_id}":    strings.Repeat("a", 40),
		"{plan_id}":      verificationPlan.Plan.ID,
		"{ordinal}":      "1",
		"{route}":        "code",
		"{provider}":     "mimo",
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
		} else if spec.Path == RunLifecycleControlPathTemplate {
			requestPath = strings.ReplaceAll(spec.Path, "{run_id}", lifecycleRun.ID)
		} else if spec.Path == PlanDirectionControlPathTemplate ||
			spec.Path == PlanDeliveryControlPathTemplate {
			requestPath = strings.ReplaceAll(spec.Path, "{run_id}", planRun.ID)
		} else if spec.Path == RunExecutionControlPathTemplate {
			requestPath = strings.ReplaceAll(spec.Path, "{run_id}", executionRun.ID)
		} else if spec.Path == RunWakeIntentPathTemplate ||
			spec.Path == RunWakeCancellationPathTemplate {
			requestPath = strings.ReplaceAll(spec.Path, "{run_id}", wakeRun.ID)
		} else if spec.Path == RunWakeExecutionPathTemplate {
			requestPath = strings.ReplaceAll(spec.Path, "{run_id}", wakeExecutionRun.ID)
		} else if spec.Path == FileEditProposalRecoveryPathTemplate {
			requestPath = strings.ReplaceAll(spec.Path, "{run_id}", fixture.run.ID)
			requestPath = strings.ReplaceAll(requestPath, "{edit_id}", recoveryProposal.Edit.ID)
		}
		if spec.OperationID == "searchWorkspace" {
			requestPath += "?query=README"
		} else if spec.OperationID == "getWorkspaceRepositoryFileHistory" {
			requestPath += "?path=README.md"
		} else if spec.OperationID == "getWorkspaceRepositoryCommitFilePreview" {
			requestPath += "?path=README.md"
		} else if spec.OperationID == "compareWorkspaceRepositoryCommits" {
			requestPath += "?base_object_id=" + strings.Repeat("a", 40) +
				"&head_object_id=" + strings.Repeat("b", 40)
		} else if spec.OperationID == "issueFileEditProposalSource" {
			requestPath += "?path=README.md"
		} else if spec.OperationID == "exportCodeHandoff" {
			requestPath += "?format=markdown"
		} else if spec.OperationID == "exportRunVerificationPlanItemSnapshot" {
			requestPath += "?format=json"
		}
		t.Run(spec.OperationID, func(t *testing.T) {
			var response *httptest.ResponseRecorder
			expectedStatus := http.StatusOK
			if spec.OperationID == "getRunFindingReport" {
				expectedStatus = http.StatusNotFound
			} else if spec.OperationID == "getWorkspaceRepositoryCommitFilePreview" {
				expectedStatus = http.StatusPreconditionFailed
			}
			if spec.Control {
				body := `{"profile":"docker"}`
				if spec.Path == RunCreationControlPath {
					body = `{"version":"run_creation.v1","goal":"OpenAPI live Run",` +
						`"workspace_id":"` + fixture.workspace.ID + `"}`
				} else if spec.Path == SessionMessageControlPathTemplate {
					body = `{"version":"session_message_submission.v1",` +
						`"content":"OpenAPI live Session message"}`
				} else if spec.Path == SessionSteeringCancellationPathTemplate {
					body = `{"version":"session_steering_cancellation.v1",` +
						`"reason":"OpenAPI live cancellation"}`
				} else if spec.Path == RunLifecycleControlPathTemplate {
					body = `{"version":"run_lifecycle_control.v1","action":"start"}`
				} else if spec.Path == PlanDirectionControlPathTemplate {
					body = `{"version":"plan_delivery_control.v1","proposal_id":"` +
						planProposal.ID + `","direction":1}`
				} else if spec.Path == PlanDeliveryControlPathTemplate {
					body = `{"version":"plan_delivery_control.v1"}`
				} else if spec.Path == ApprovalDecisionControlPathTemplate {
					body = `{"version":"approval_control.v1","action":"approve_once"}`
				} else if spec.Path == RunExecutionControlPathTemplate {
					body = `{"version":"run_execution_handoff.v1","max_steps":1}`
				} else if spec.Path == ModelRouteControlPathTemplate {
					body = `{"version":"model_route_control.v1","provider":"mock","model":"mock-code"}`
				} else if spec.Path == ProviderDiagnosticPath {
					body = `{"version":"provider_diagnostic.v1","provider":"mock","model":"mock-code","confirm_diagnostic":true}`
				} else if spec.Path == ProviderCredentialPathTemplate {
					body = `{"version":"provider_credential.v1","action":"set",` +
						`"secret":"temporary-openapi-key","confirm":true}`
				} else if spec.Path == FileEditProposalPathTemplate {
					body = `{"version":"file_edit_proposal.v1","source_handle":"` +
						proposalSource.Handle + `","proposed_text":"OpenAPI proposal\\n"}`
				} else if spec.Path == FileEditReviewPathTemplate {
					body = `{"version":"file_edit_review.v1","action":"approve_intent"}`
				} else if spec.Path == FileEditApplyPathTemplate {
					body = `{"version":"file_edit_apply.v1"}`
				} else if spec.Path == RunWakeIntentPathTemplate {
					body = `{"version":"run_wake_control.v1","max_attempts":3,` +
						`"initial_delay_seconds":0,"base_backoff_seconds":5,` +
						`"max_backoff_seconds":60,"max_elapsed_seconds":300}`
				} else if spec.Path == RunWakeCancellationPathTemplate {
					body = `{"version":"run_wake_control.v1"}`
				} else if spec.Path == RunWakeExecutionPathTemplate {
					body = `{"version":"run_wake_consumer.v1","max_steps":1}`
				} else if spec.Path == SkillPackageInstallPath {
					body = `{"version":"skill_package_installation.v1","archive_base64":"` +
						base64.StdEncoding.EncodeToString(skillArchive) +
						`","surface":"code","confirm_untrusted":true}`
				} else if spec.Path == EvidenceAttachmentPathTemplate {
					body = `{"version":"session_evidence_attachment.v1",` +
						`"source_kind":"workspace_file","source_ref":"README.md",` +
						`"content_sha256":"` + hex.EncodeToString(evidenceDigest[:]) + `"}`
				} else if spec.Path == VerificationEvidencePathTemplate {
					body = `{"version":"operator_verification_evidence.v1",` +
						`"outcome":"pass","title":"OpenAPI verification",` +
						`"summary":"Live route verified"}`
				} else if spec.Path == VerificationPlanPathTemplate {
					body = `{"version":"operator_verification_plan.v1",` +
						`"title":"OpenAPI verification plan","summary":"Operator guidance",` +
						`"items":[{"title":"Live route",` +
						`"expected_observation":"Observe a successful response"}]}`
				} else if spec.Path == VerificationAssociationPathTemplate {
					body = `{"version":"operator_verification_plan_evidence_association.v1",` +
						`"plan_id":"` + verificationPlan.Plan.ID + `",` +
						`"plan_item_ordinal":1,"evidence_id":"` +
						verificationEvidence.Evidence.ID + `"}`
				} else if spec.Path == VerificationSnapshotReceiptPathTemplate {
					snapshot, err := application.NewVerificationSnapshotExportService(
						fixture.store).Build(t.Context(), fixture.run.ID,
						verificationPlan.Plan.ID, 1, application.VerificationSnapshotExportFormatJSON)
					if err != nil {
						t.Fatal(err)
					}
					body = `{"version":"operator_verification_plan_item_snapshot_receipt.v1",` +
						`"plan_id":"` + verificationPlan.Plan.ID + `",` +
						`"plan_item_ordinal":1,"format":"json",` +
						`"snapshot_high_water_event_sequence":` +
						fmt.Sprint(snapshot.SnapshotHighWaterEventSequence) + `,` +
						`"content_sha256":"` + snapshot.ContentSHA256 + `",` +
						`"confirm_metadata_snapshot":true}`
				} else if spec.Path == VerificationSnapshotReceiptReviewPathTemplate {
					body = `{"version":"operator_verification_plan_item_snapshot_receipt_review.v1",` +
						`"receipt_id":"` + openAPIReceipt.Receipt.ID + `",` +
						`"receipt_content_sha256":"` + openAPIReceipt.Receipt.ContentSHA256 + `",` +
						`"receipt_event_sequence":` +
						fmt.Sprint(openAPIReceipt.Receipt.EventSequence) + `,` +
						`"decision":"metadata_confirmed",` +
						`"confirm_non_authorizing_review":true}`
				} else if spec.Path != RunExecutionProfileControlPathTemplate {
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

func buildOpenAPISkillPackage(t *testing.T) []byte {
	t.Helper()
	content := []byte("# OpenAPI external review\n\nInspect workspace evidence only.\n")
	digest := sha256.Sum256(content)
	manifest := skills.Manifest{
		Protocol: skills.ProtocolVersion, Name: "openapi-external-review", Version: "1.0.0",
		Description: "OpenAPI inert Skill installation fixture.",
		Profiles:    []domain.Profile{domain.ProfileReview},
		ToolDependencies: []toolgateway.ToolName{
			toolgateway.ListWorkspaceTool, toolgateway.ReadFileTool,
		},
		ContentPath: skills.PackageContentPath, ContentSHA256: hex.EncodeToString(digest[:]),
		ContentBytes: len(content), ContentTokenUpperBound: skills.ContentTokenUpperBound(content),
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range []struct {
		name string
		data []byte
	}{{skills.PackageManifestPath, manifestRaw}, {skills.PackageContentPath, content}} {
		file, createErr := writer.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Deflate})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := file.Write(entry.data); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func prepareOpenAPIPlanControlTarget(t *testing.T,
	fixture *apiFixture,
) (domain.Run, domain.PlanDeliveryProposal) {
	t.Helper()
	ctx := t.Context()
	runs := application.NewRunService(fixture.store)
	_, run, err := runs.Create(ctx, application.CreateRunRequest{
		Goal: "OpenAPI Plan control target", Profile: "review", Phase: "plan",
		ModelRoute: "http-plan/model",
		Budget:     domain.Budget{MaxTurns: 4, MaxTokens: 1000, MaxToolCalls: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err = runs.Start(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	provider := &httpPlanProvider{responses: []*llm.ChatResponse{
		{Provider: "openapi-plan", Model: "model",
			Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4},
			ToolCalls: []llm.ToolCall{{ID: "openapi-plan-control-call",
				Name: "plan_delivery_propose", Arguments: json.RawMessage(httpPlanDeliveryPayload)}}},
		{Text: httpRootWaitResponse(t), Provider: "openapi-plan", Model: "model",
			Usage: llm.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4}},
	}}
	router := llm.NewRouter(llm.ModelRef{Provider: provider.Name(), Model: "model"})
	router.RegisterProvider(provider)
	if _, err := application.NewRunSupervisor(fixture.store, router,
		policy.NewDefaultChecker()).Step(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	proposals, err := fixture.store.ListPlanDeliveryProposals(ctx, run.ID, 2)
	if err != nil || len(proposals) != 1 {
		t.Fatalf("OpenAPI Plan proposals=%#v err=%v", proposals, err)
	}
	run, err = fixture.store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	return run, proposals[0]
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

func assertOpenAPIPropertyFlag(t *testing.T, schemas map[string]map[string]any,
	name string, property string, flag string, expected any,
) {
	t.Helper()
	schema, ok := schemas[name]
	if !ok {
		t.Fatalf("OpenAPI component %s is missing", name)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI component %s has no properties", name)
	}
	value, ok := properties[property].(map[string]any)
	if !ok || !reflect.DeepEqual(value[flag], expected) {
		t.Fatalf("OpenAPI component %s property %s flag %s=%v want=%v",
			name, property, flag, value[flag], expected)
	}
}
