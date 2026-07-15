package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/policy"
	"cyberagent-workbench/internal/redact"
	"cyberagent-workbench/internal/runmutation"
	"cyberagent-workbench/internal/sandbox"
	"cyberagent-workbench/internal/toolbudget"
	"cyberagent-workbench/internal/tools"
)

const (
	sandboxApprovalToolName    = "sandbox.manifest"
	sandboxApprovalActionClass = "sandbox_execute"
)

type SandboxManifestStore interface {
	GetRun(ctx context.Context, id string) (domain.Run, error)
	GetMission(ctx context.Context, id string) (domain.Mission, error)
	GetSandboxWorkspace(ctx context.Context, id string) (sandbox.WorkspaceBinding, error)
	GetApproval(ctx context.Context, id string) (approval.Record, error)
	GetSandboxManifestOperation(ctx context.Context, keyDigest string) (sandbox.Operation, bool, error)
	CreateSandboxManifestIntent(ctx context.Context, preparation sandbox.Preparation,
		validation sandbox.Validation, operation sandbox.Operation) (sandbox.PreparedIntent, bool, error)
	GetSandboxManifestIntent(ctx context.Context, id string) (sandbox.PreparedIntent, error)
	ListSandboxManifestIntents(ctx context.Context, runID string, limit int) ([]sandbox.PreparedIntent, error)
	EnsureApproval(ctx context.Context, proposal approval.Proposal) (approval.Record, error)
	DecideApproval(ctx context.Context, request approval.DecisionRequest) (approval.DecisionResult, error)
	GetRunAgentUsage(ctx context.Context, runID string) (domain.RunAgentUsage, error)
	GetToolCallUsage(ctx context.Context, runID string) (toolbudget.Usage, error)
	GetRunExecutionLease(ctx context.Context, runID string) (domain.RunExecutionLease, bool, error)
	GetSandboxExecutionCandidateOperation(ctx context.Context, keyDigest string) (sandbox.CandidateOperation, bool, error)
	CreateSandboxExecutionCandidate(ctx context.Context, candidate sandbox.ExecutionCandidate,
		operation sandbox.CandidateOperation) (sandbox.ValidatedExecutionCandidate, bool, error)
	GetSandboxExecutionCandidate(ctx context.Context, id string) (sandbox.ValidatedExecutionCandidate, error)
	ListSandboxExecutionCandidates(ctx context.Context, runID string, limit int) ([]sandbox.ValidatedExecutionCandidate, error)
	GetRunArtifact(ctx context.Context, id string) (artifact.Blob, error)
	GetSandboxExecutionOperation(ctx context.Context, keyDigest string) (sandbox.ExecutionOperation, bool, error)
	CreateSandboxDisabledExecution(ctx context.Context, execution sandbox.DisabledExecution,
		inputs []sandbox.InputArtifactBinding, operation sandbox.ExecutionOperation,
		ownerID string, ttl time.Duration) (sandbox.Lifecycle, bool, error)
	GetSandboxDisabledExecution(ctx context.Context, id string) (sandbox.Lifecycle, error)
	ListSandboxDisabledExecutions(ctx context.Context, runID string, limit int) ([]sandbox.Lifecycle, error)
	AcquireSandboxExecutionLease(ctx context.Context, executionID, ownerID, leaseID string,
		ttl time.Duration) (sandbox.LeaseAcquisition, error)
	ReleaseSandboxExecutionLease(ctx context.Context, expected sandbox.ExecutionLease) (sandbox.ExecutionLease, bool, error)
	GetSandboxExecutionLease(ctx context.Context, executionID string) (sandbox.ExecutionLease, bool, error)
	GetSandboxCancellationOperation(ctx context.Context, keyDigest string) (sandbox.CancellationOperation, bool, error)
	CreateSandboxCancellation(ctx context.Context, request sandbox.CancellationRequest,
		operation sandbox.CancellationOperation) (sandbox.CancellationRequest, bool, error)
	GetSandboxCleanupOperation(ctx context.Context, keyDigest string) (sandbox.CleanupOperation, bool, error)
	CompleteSandboxCleanup(ctx context.Context, result sandbox.CleanupResult,
		operation sandbox.CleanupOperation, expectedLease sandbox.ExecutionLease) (sandbox.CleanupResult, bool, error)
	GetSandboxPreflightOperation(ctx context.Context, keyDigest string) (sandbox.PreflightOperation, bool, error)
	CreateSandboxDisabledPreflight(ctx context.Context, preflight sandbox.DisabledPreflight,
		operation sandbox.PreflightOperation) (sandbox.DisabledPreflight, bool, error)
	GetSandboxDisabledPreflight(ctx context.Context, id string) (sandbox.DisabledPreflight, error)
	ListSandboxDisabledPreflights(ctx context.Context, runID string, limit int) ([]sandbox.DisabledPreflight, error)
	GetSandboxBackendEvidenceOperation(ctx context.Context, keyDigest string) (sandbox.BackendEvidenceOperation, bool, error)
	CreateSandboxBackendEvidence(ctx context.Context, evidence sandbox.BackendEvidence,
		operation sandbox.BackendEvidenceOperation) (sandbox.BackendEvidence, bool, error)
	GetSandboxBackendEvidence(ctx context.Context, id string) (sandbox.BackendEvidence, error)
	ListSandboxBackendEvidence(ctx context.Context, runID string, limit int) ([]sandbox.BackendEvidence, error)
	GetSandboxOutputSimulationOperation(ctx context.Context, keyDigest string) (sandbox.OutputSimulationOperation, bool, error)
	CreateSandboxOutputSimulation(ctx context.Context, simulation sandbox.OutputSimulation,
		operation sandbox.OutputSimulationOperation) (sandbox.OutputSimulation, bool, error)
	GetSandboxOutputSimulation(ctx context.Context, id string) (sandbox.OutputSimulation, error)
	ListSandboxOutputSimulations(ctx context.Context, runID string, limit int) ([]sandbox.OutputSimulation, error)
	GetDockerObservationOperation(ctx context.Context, keyDigest string) (sandbox.DockerObservationOperation, bool, error)
	CreateDockerObservation(ctx context.Context, observation sandbox.DockerObservation,
		operation sandbox.DockerObservationOperation) (sandbox.DockerObservation, bool, error)
	GetDockerObservation(ctx context.Context, id string) (sandbox.DockerObservation, error)
	ListDockerObservations(ctx context.Context, runID string, limit int) ([]sandbox.DockerObservation, error)
	GetDockerContainerPlanOperation(ctx context.Context, keyDigest string) (sandbox.DockerContainerPlanOperation, bool, error)
	CreateDockerContainerPlan(ctx context.Context, plan sandbox.DockerContainerPlan,
		operation sandbox.DockerContainerPlanOperation) (sandbox.DockerContainerPlan, bool, error)
	GetDockerContainerPlan(ctx context.Context, id string) (sandbox.DockerContainerPlan, error)
	ListDockerContainerPlans(ctx context.Context, runID string, limit int) ([]sandbox.DockerContainerPlan, error)
	GetDockerContainerRehearsalOperation(ctx context.Context, keyDigest string) (sandbox.DockerContainerRehearsalOperation, bool, error)
	CreateDockerContainerRehearsal(ctx context.Context, rehearsal sandbox.DockerContainerRehearsal,
		operation sandbox.DockerContainerRehearsalOperation) (sandbox.DockerContainerRehearsal, bool, error)
	GetDockerContainerRehearsal(ctx context.Context, id string) (sandbox.DockerContainerRehearsal, error)
	ListDockerContainerRehearsals(ctx context.Context, runID string, limit int) ([]sandbox.DockerContainerRehearsal, error)
}

type SandboxManifestService struct {
	store                SandboxManifestStore
	checker              policy.Checker
	inspector            sandbox.BackendInspector
	evidenceClient       sandbox.BackendEvidenceClient
	outputHarness        sandbox.OutputSimulationHarness
	dockerObserver       sandbox.DockerProductionObserver
	dockerWriter         sandbox.DockerContainerTransactionHarness
	dockerWriteTransport sandbox.DockerContainerWriteTransport
}

type PrepareSandboxManifestRequest struct {
	RunID        string
	Manifest     sandbox.Manifest
	ApprovalID   string
	OperationKey string
	RequestedBy  string
}

func NewSandboxManifestService(store SandboxManifestStore,
	checker policy.Checker,
) *SandboxManifestService {
	return &SandboxManifestService{
		store: store, checker: checker, inspector: sandbox.NewDisabledBackendInspector(),
		evidenceClient: sandbox.NewSimulationBackendClient(),
		outputHarness:  sandbox.NewInMemoryOutputHarness(),
		dockerObserver: sandbox.NewReadOnlyDockerProductionObserver(
			sandbox.NewLocalDockerReadOnlyTransport()),
		dockerWriter:         sandbox.NewInMemoryDockerWriteTransaction(),
		dockerWriteTransport: sandbox.NewUnavailableDockerContainerWriteTransport(),
	}
}

func (s *SandboxManifestService) WithDockerContainerWriteTransport(
	transport sandbox.DockerContainerWriteTransport,
) *SandboxManifestService {
	if s != nil && transport != nil {
		s.dockerWriteTransport = transport
	}
	return s
}

func (s *SandboxManifestService) WithDockerContainerTransactionHarness(
	harness sandbox.DockerContainerTransactionHarness,
) *SandboxManifestService {
	if s != nil && harness != nil {
		s.dockerWriter = harness
	}
	return s
}

func (s *SandboxManifestService) WithDockerProductionObserver(
	observer sandbox.DockerProductionObserver,
) *SandboxManifestService {
	if s != nil && observer != nil {
		s.dockerObserver = observer
	}
	return s
}

func (s *SandboxManifestService) Prepare(ctx context.Context,
	request PrepareSandboxManifestRequest,
) (sandbox.PreparedIntent, error) {
	if s == nil || s.store == nil || s.checker == nil {
		return sandbox.PreparedIntent{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest store and policy checker are required")
	}
	normalizedRequest, err := normalizeSandboxManifestRequest(request)
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox manifest request is invalid", err)
	}
	manifest, err := sandbox.NewNoopRunner().ValidateManifest(ctx, normalizedRequest.Manifest)
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox manifest Noop validation failed", err)
	}
	manifestFingerprint, err := manifest.Fingerprint()
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Wrap(apperror.CodeInvalidArgument,
			"sandbox manifest fingerprint failed", err)
	}
	operationKeyDigest := runmutation.Fingerprint("sandbox_manifest_operation.v1",
		normalizedRequest.RunID, normalizedRequest.OperationKey)
	if existing, found, err := s.store.GetSandboxManifestOperation(ctx,
		operationKeyDigest); err != nil {
		return sandbox.PreparedIntent{}, apperror.Normalize(err)
	} else if found {
		return s.replay(ctx, normalizedRequest, manifestFingerprint, existing)
	}
	run, err := s.store.GetRun(ctx, normalizedRequest.RunID)
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Normalize(err)
	}
	if run.Terminal() {
		return sandbox.PreparedIntent{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest cannot be prepared for a terminal Run")
	}
	mission, err := s.store.GetMission(ctx, run.MissionID)
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Normalize(err)
	}
	if mission.WorkspaceID == "" || mission.Scope.WorkspaceID != mission.WorkspaceID {
		return sandbox.PreparedIntent{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest requires an exact non-empty Mission workspace scope")
	}
	workspace, err := s.store.GetSandboxWorkspace(ctx, mission.WorkspaceID)
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Normalize(err)
	}
	rootPath, err := validateSandboxWorkspaceBinding(workspace)
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"sandbox workspace binding is invalid", err)
	}
	normalizedScope, err := normalizeSandboxMissionScope(mission.Scope)
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Wrap(apperror.CodeFailedPrecondition,
			"Mission network scope is invalid", err)
	}
	if err := requireSandboxScopeSubset(manifest.Network, normalizedScope); err != nil {
		return sandbox.PreparedIntent{}, apperror.Wrap(apperror.CodePolicyDenied,
			"sandbox manifest attempted to widen the Mission network scope", err)
	}
	canonicalScope, err := json.Marshal(normalizedScope)
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Wrap(apperror.CodeInternal,
			"encode sandbox scope snapshot", err)
	}
	workspaceFingerprint := runmutation.Fingerprint("sandbox_workspace_binding.v1",
		workspace.ID, rootPath)
	scopeFingerprint := runmutation.Fingerprint("sandbox_scope_binding.v1",
		string(canonicalScope))
	authorizationFingerprint := runmutation.Fingerprint("sandbox_authorization.v1",
		run.ID, mission.ID, workspace.ID, manifestFingerprint, workspaceFingerprint,
		scopeFingerprint)
	canonicalManifest, err := manifest.CanonicalJSON()
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Wrap(apperror.CodeInternal,
			"encode normalized sandbox manifest", err)
	}
	decision := s.checker.CheckToolCall(tools.Call{
		Name: sandboxApprovalToolName,
		Args: map[string]string{"intent": string(canonicalManifest)},
	})
	decision = hardenSandboxDecision(decision, manifest)
	policyFingerprint := runmutation.Fingerprint("sandbox_policy_decision.v1",
		authorizationFingerprint, fmt.Sprintf("%t", decision.Allowed),
		fmt.Sprintf("%t", decision.NeedsApproval), decision.Risk, decision.Reason)
	approvalID, approvalStatus, err := s.bindSandboxApproval(ctx, run, mission,
		authorizationFingerprint, decision, normalizedRequest.ApprovalID)
	if err != nil {
		return sandbox.PreparedIntent{}, err
	}
	now := time.Now().UTC()
	preparation := sandbox.Preparation{
		ID: idgen.New("sandbox-manifest"), RunID: run.ID, MissionID: mission.ID,
		WorkspaceID: workspace.ID, CancellationID: idgen.New("sandbox-cancel"),
		ProtocolVersion: sandbox.ManifestProtocolVersion, Backend: manifest.Backend,
		ManifestFingerprint:      manifestFingerprint,
		AuthorizationFingerprint: authorizationFingerprint,
		WorkspaceFingerprint:     workspaceFingerprint, ScopeFingerprint: scopeFingerprint,
		CommandArgumentCount: len(manifest.Command.Arguments), MountCount: len(manifest.Mounts),
		WritableMountCount: manifest.WritableMountCount(),
		EnvironmentCount:   len(manifest.Environment), SecretReferenceCount: manifest.SecretReferenceCount(),
		NetworkMode: manifest.Network.Mode, AllowedTargetCount: len(manifest.Network.AllowedTargets),
		InputArtifactCount: len(manifest.InputArtifactIDs), OutputCount: manifest.OutputCount(),
		TimeoutSeconds:    manifest.TimeoutSeconds,
		GracePeriodMillis: manifest.Cancellation.GracePeriodMillis,
		CPUQuotaMillis:    manifest.Resources.CPUQuotaMillis,
		MemoryBytes:       manifest.Resources.MemoryBytes, PIDs: manifest.Resources.PIDs,
		MaxOutputBytes: manifest.Resources.MaxOutputBytes,
		RequestedBy:    normalizedRequest.RequestedBy, PreparedAt: now,
	}
	validation := sandbox.Validation{
		PreparationID: preparation.ID, RunID: run.ID,
		ProtocolVersion: sandbox.ValidationProtocolVersion,
		PolicyAllowed:   decision.Allowed, NeedsApproval: decision.NeedsApproval,
		Risk: decision.Risk, PolicyFingerprint: policyFingerprint,
		ApprovalID: approvalID, ApprovalStatus: approvalStatus, ValidatorName: "noop",
		BackendEnabled: false, ExecutionAuthorized: false, ValidatedAt: now,
	}
	operation := sandbox.Operation{
		KeyDigest:     operationKeyDigest,
		PreparationID: preparation.ID, RunID: run.ID,
		RequestedBy: normalizedRequest.RequestedBy, CreatedAt: now,
	}
	operation.RequestFingerprint = sandbox.IntentRequestFingerprint(preparation, validation)
	stored, replayed, err := s.store.CreateSandboxManifestIntent(ctx, preparation,
		validation, operation)
	stored.Replayed = replayed
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Normalize(err)
	}
	if !stored.Validation.PolicyAllowed {
		return stored, apperror.New(apperror.CodePolicyDenied,
			"sandbox manifest was recorded but permanently denied by policy")
	}
	return stored, nil
}

func (s *SandboxManifestService) replay(ctx context.Context,
	request PrepareSandboxManifestRequest, manifestFingerprint string,
	operation sandbox.Operation,
) (sandbox.PreparedIntent, error) {
	if operation.RunID != request.RunID || operation.RequestedBy != request.RequestedBy {
		return sandbox.PreparedIntent{}, apperror.New(apperror.CodeConflict,
			"sandbox manifest operation key was already used for different intent")
	}
	stored, err := s.store.GetSandboxManifestIntent(ctx, operation.PreparationID)
	if err != nil {
		return sandbox.PreparedIntent{}, apperror.Normalize(err)
	}
	if operation.PreparationID != stored.Preparation.ID ||
		operation.RequestFingerprint != sandbox.IntentRequestFingerprint(stored.Preparation,
			stored.Validation) || operation.RunID != stored.Preparation.RunID ||
		operation.RequestedBy != stored.Preparation.RequestedBy {
		return sandbox.PreparedIntent{}, apperror.New(apperror.CodeInternal,
			"stored sandbox manifest replay binding is invalid")
	}
	if stored.Preparation.ManifestFingerprint != manifestFingerprint ||
		stored.Validation.ApprovalID != request.ApprovalID {
		return sandbox.PreparedIntent{}, apperror.New(apperror.CodeConflict,
			"sandbox manifest operation key was already used for different intent")
	}
	stored.Replayed = true
	if !stored.Validation.PolicyAllowed {
		return stored, apperror.New(apperror.CodePolicyDenied,
			"sandbox manifest was recorded but permanently denied by policy")
	}
	return stored, nil
}

func (s *SandboxManifestService) Get(ctx context.Context,
	id string,
) (sandbox.PreparedIntent, error) {
	if s == nil || s.store == nil {
		return sandbox.PreparedIntent{}, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest store is required")
	}
	value, err := s.store.GetSandboxManifestIntent(ctx, strings.TrimSpace(id))
	return value, apperror.Normalize(err)
}

func (s *SandboxManifestService) List(ctx context.Context, runID string,
	limit int,
) ([]sandbox.PreparedIntent, error) {
	if s == nil || s.store == nil {
		return nil, apperror.New(apperror.CodeFailedPrecondition,
			"sandbox manifest store is required")
	}
	values, err := s.store.ListSandboxManifestIntents(ctx, strings.TrimSpace(runID), limit)
	return values, apperror.Normalize(err)
}

func normalizeSandboxManifestRequest(request PrepareSandboxManifestRequest,
) (PrepareSandboxManifestRequest, error) {
	originalOperationKey := request.OperationKey
	request.RunID = strings.TrimSpace(request.RunID)
	request.ApprovalID = strings.TrimSpace(request.ApprovalID)
	request.OperationKey = strings.TrimSpace(request.OperationKey)
	request.RequestedBy = strings.TrimSpace(redact.String(request.RequestedBy))
	if request.RequestedBy == "" {
		request.RequestedBy = "cli_operator"
	}
	if !domain.ValidAgentID(request.RunID) || !domain.ValidAgentID(request.RequestedBy) ||
		strings.ContainsRune(request.RunID, 0) || strings.ContainsRune(request.RequestedBy, 0) {
		return PrepareSandboxManifestRequest{}, errors.New("bounded Run and operator identities are required")
	}
	if request.ApprovalID != "" && (!domain.ValidAgentID(request.ApprovalID) ||
		strings.ContainsRune(request.ApprovalID, 0)) {
		return PrepareSandboxManifestRequest{}, errors.New("sandbox approval identity is invalid")
	}
	if request.OperationKey != originalOperationKey || !utf8.ValidString(request.OperationKey) {
		return PrepareSandboxManifestRequest{}, errors.New("sandbox operation key must be normalized UTF-8")
	}
	if _, err := domain.NormalizeAgentOperationKey(request.OperationKey); err != nil {
		return PrepareSandboxManifestRequest{}, err
	}
	for _, current := range request.OperationKey {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return PrepareSandboxManifestRequest{}, errors.New("sandbox operation key cannot contain whitespace or control characters")
		}
	}
	return request, nil
}

func validateSandboxWorkspaceBinding(workspace sandbox.WorkspaceBinding) (string, error) {
	if !domain.ValidAgentID(workspace.ID) || strings.ContainsRune(workspace.ID, 0) {
		return "", errors.New("workspace identity is invalid")
	}
	if workspace.RootPath == "" || strings.TrimSpace(workspace.RootPath) != workspace.RootPath ||
		!utf8.ValidString(workspace.RootPath) || strings.ContainsRune(workspace.RootPath, 0) {
		return "", errors.New("workspace root must be normalized UTF-8")
	}
	rootPath, err := filepath.Abs(workspace.RootPath)
	if err != nil {
		return "", err
	}
	rootPath = filepath.Clean(rootPath)
	info, err := os.Stat(rootPath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("workspace root is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func normalizeSandboxMissionScope(scope domain.Scope) (domain.Scope, error) {
	normalized := domain.Scope{WorkspaceID: strings.TrimSpace(scope.WorkspaceID),
		NetworkMode: strings.TrimSpace(scope.NetworkMode)}
	if normalized.WorkspaceID != scope.WorkspaceID || normalized.NetworkMode != scope.NetworkMode {
		return domain.Scope{}, errors.New("mission scope must be normalized before sandbox binding")
	}
	if normalized.WorkspaceID == "" {
		return domain.Scope{}, errors.New("mission scope workspace is required")
	}
	if normalized.NetworkMode != "disabled" && normalized.NetworkMode != "allowlist" {
		return domain.Scope{}, fmt.Errorf("unsupported mission network mode %q", normalized.NetworkMode)
	}
	if len(scope.AllowedTargets) > sandbox.MaxNetworkTargets {
		return domain.Scope{}, errors.New("mission network allowlist exceeds sandbox protocol bounds")
	}
	for _, target := range scope.AllowedTargets {
		value, err := sandbox.NormalizeAllowedTarget(target)
		if err != nil {
			return domain.Scope{}, err
		}
		normalized.AllowedTargets = append(normalized.AllowedTargets, value)
	}
	sort.Strings(normalized.AllowedTargets)
	for index := 1; index < len(normalized.AllowedTargets); index++ {
		if normalized.AllowedTargets[index] == normalized.AllowedTargets[index-1] {
			return domain.Scope{}, errors.New("mission network allowlist contains duplicate targets")
		}
	}
	if normalized.NetworkMode == "disabled" && len(normalized.AllowedTargets) != 0 {
		return domain.Scope{}, errors.New("disabled Mission network scope cannot contain targets")
	}
	if normalized.NetworkMode == "allowlist" && len(normalized.AllowedTargets) == 0 {
		return domain.Scope{}, errors.New("allowlist Mission network scope requires targets")
	}
	return normalized, nil
}

func requireSandboxScopeSubset(request sandbox.NetworkScope, mission domain.Scope) error {
	if request.Mode == "disabled" {
		return nil
	}
	if request.Mode != "allowlist" || mission.NetworkMode != "allowlist" {
		return errors.New("mission scope does not permit sandbox network access")
	}
	allowed := make(map[string]struct{}, len(mission.AllowedTargets))
	for _, target := range mission.AllowedTargets {
		allowed[target] = struct{}{}
	}
	for _, target := range request.AllowedTargets {
		if _, ok := allowed[target]; !ok {
			return fmt.Errorf("target %q is outside the mission allowlist", target)
		}
	}
	return nil
}

func hardenSandboxDecision(decision policy.Decision, manifest sandbox.Manifest) policy.Decision {
	decision.Reason = strings.TrimSpace(decision.Reason)
	decision.Risk = strings.ToLower(strings.TrimSpace(decision.Risk))
	if decision.Reason == "" {
		decision.Reason = "sandbox policy checker returned no reason"
	}
	if !decision.Allowed {
		decision.NeedsApproval = false
		if riskRank(decision.Risk) < riskRank("high") {
			decision.Risk = "high"
		}
		return decision
	}
	requiresApproval := decision.NeedsApproval || riskRank(decision.Risk) >= riskRank("medium") ||
		manifest.Backend != sandbox.BackendNoop ||
		manifest.Network.Mode == "allowlist" || manifest.HasWritableMount() ||
		manifest.SecretReferenceCount() > 0
	if requiresApproval {
		decision.NeedsApproval = true
		if riskRank(decision.Risk) < riskRank("medium") {
			decision.Risk = "medium"
		}
		decision.Reason += "; sandbox backend, write, network, or secret capability requires approval"
	} else if riskRank(decision.Risk) == 0 {
		decision.Risk = "low"
	}
	return decision
}

func riskRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

func (s *SandboxManifestService) bindSandboxApproval(ctx context.Context, run domain.Run,
	mission domain.Mission, authorizationFingerprint string, decision policy.Decision,
	requestedApprovalID string,
) (string, sandbox.ApprovalStatus, error) {
	if !decision.Allowed {
		if requestedApprovalID != "" {
			return "", "", apperror.New(apperror.CodePolicyDenied,
				"policy-denied sandbox manifest cannot be overridden by approval")
		}
		return "", sandbox.ApprovalNotApplicable, nil
	}
	if !decision.NeedsApproval {
		if requestedApprovalID != "" {
			return "", "", apperror.New(apperror.CodeInvalidArgument,
				"sandbox manifest does not require an approval binding")
		}
		return "", sandbox.ApprovalNotRequired, nil
	}
	if requestedApprovalID == "" {
		return "", sandbox.ApprovalRequired, nil
	}
	record, err := s.store.GetApproval(ctx, requestedApprovalID)
	if err != nil {
		return "", "", apperror.Normalize(err)
	}
	if err := record.Validate(); err != nil {
		return "", "", apperror.Wrap(apperror.CodeInternal,
			"stored sandbox approval is invalid", err)
	}
	if record.RunID != run.ID || record.SessionID != run.SessionID ||
		record.WorkspaceID != mission.WorkspaceID || record.ToolName != sandboxApprovalToolName ||
		record.ActionClass != sandboxApprovalActionClass ||
		record.RequestFingerprint != authorizationFingerprint {
		return "", "", apperror.New(apperror.CodeConflict,
			"sandbox approval does not match the exact Run, workspace, action, and manifest intent")
	}
	switch record.Status {
	case approval.StatusPending:
		return record.ID, sandbox.ApprovalPending, nil
	case approval.StatusApproved:
		return record.ID, sandbox.ApprovalApproved, nil
	case approval.StatusDenied:
		return record.ID, sandbox.ApprovalDenied, nil
	default:
		return "", "", apperror.New(apperror.CodeInternal,
			"stored sandbox approval status is unsupported")
	}
}
