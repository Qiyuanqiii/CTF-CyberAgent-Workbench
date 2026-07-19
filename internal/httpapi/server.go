package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/approval"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/fileedit"
	"cyberagent-workbench/internal/idgen"
	"cyberagent-workbench/internal/modelregistry"
	"cyberagent-workbench/internal/operationreceipt"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/skills"
	"cyberagent-workbench/internal/toolbudget"
)

const (
	Version               = "api.v1"
	OpenAPIPath           = "/api/v1/openapi.json"
	DefaultListenAddress  = "127.0.0.1:8765"
	MaxRequestTargetBytes = 8 * 1024
	MaxQueryBytes         = 4 * 1024
	MaxResponseBytes      = 8 * 1024 * 1024
	MinAccessTokenBytes   = 32
	MaxAccessTokenBytes   = 512
)

type Store interface {
	SchemaVersion(ctx context.Context) (int, error)
	GetMission(ctx context.Context, id string) (domain.Mission, error)
	GetRun(ctx context.Context, id string) (domain.Run, error)
	GetRunMode(ctx context.Context, runID string) (domain.RunModeSnapshot, error)
	GetRunCreationOperation(ctx context.Context,
		keyDigest string) (domain.RunCreationOperation, bool, error)
	CreateMissionRunWithOperation(ctx context.Context, mission domain.Mission, run domain.Run,
		mode domain.RunModeSnapshot, linkedSession session.Session, initialEvents []events.Event,
		operation domain.RunCreationOperation) (domain.RunCreationOperation, bool, error)
	GetRunExecutionProfile(ctx context.Context,
		runID string) (domain.RunExecutionProfileSnapshot, error)
	GetRunExecutionProfileSnapshot(ctx context.Context,
		id string) (domain.RunExecutionProfileSnapshot, error)
	GetRunExecutionProfileOperation(ctx context.Context,
		keyDigest string) (domain.RunExecutionProfileOperation, bool, error)
	TransitionRunExecutionProfile(ctx context.Context,
		snapshot domain.RunExecutionProfileSnapshot,
		operation domain.RunExecutionProfileOperation,
		event events.Event) (domain.RunExecutionProfileSnapshot, bool, error)
	GetRunBySession(ctx context.Context, sessionID string) (domain.Run, bool, error)
	EnqueueOperatorSteering(ctx context.Context,
		request domain.EnqueueOperatorSteeringRequest) (domain.OperatorSteeringEnqueueResult, error)
	GetOperatorSteering(ctx context.Context, id string) (domain.OperatorSteeringMessage, error)
	CancelOperatorSteering(ctx context.Context,
		request domain.CancelOperatorSteeringRequest) (domain.OperatorSteeringCancellationResult, error)
	ListRuns(ctx context.Context, filter domain.RunFilter) ([]domain.Run, error)
	ListRunEventsPage(ctx context.Context, runID string, offset int, limit int) ([]events.Event, error)
	ListRunEventsAfterSequence(ctx context.Context, runID string, afterSequence int64, limit int) ([]events.Event, error)
	GetSupervisorCheckpoint(ctx context.Context, runID string) (domain.SupervisorCheckpoint, bool, error)
	GetRunExecutionLease(ctx context.Context, runID string) (domain.RunExecutionLease, bool, error)
	ListOperatorSteering(ctx context.Context, runID string,
		limit int) ([]domain.OperatorSteeringMessage, error)
	GetOperatorSteeringQueueSummary(ctx context.Context,
		runID string) (domain.OperatorSteeringQueueSummary, error)
	RequestSupervisorModelCancellation(ctx context.Context, request domain.RequestModelCancellation) (domain.ModelCancellationResult, error)
	RequestSpecialistModelCancellation(ctx context.Context,
		request domain.RequestSpecialistModelCancellation) (domain.SpecialistModelCancellationResult, error)
	GetToolCallUsage(ctx context.Context, runID string) (toolbudget.Usage, error)
	ListRunSupervisorToolRoundsPage(ctx context.Context, runID string, offset int, limit int) ([]domain.SupervisorToolRound, error)
	ListPlanDeliveryProposals(ctx context.Context, runID string, limit int) ([]domain.PlanDeliveryProposal, error)
	GetPlanDeliveryProposal(ctx context.Context, id string) (domain.PlanDeliveryProposal, error)
	GetPlanDeliverySelectionByRun(ctx context.Context, runID string) (domain.PlanDeliverySelection, bool, error)
	GetExternalSkillProjectionByRun(ctx context.Context, runID string) (skills.ExternalSkillProjection, bool, error)
	ListDeliveryCheckpoints(ctx context.Context, runID string, limit int) ([]domain.DeliveryCheckpoint, error)
	DeliveryGateEnforced(ctx context.Context, runID string) (bool, error)
	ListAgentNodes(ctx context.Context, runID string) ([]domain.AgentNode, error)
	GetAgentCompletion(ctx context.Context, agentID string) (domain.AgentCompletion, bool, error)
	ListSpecialistDelegationProposalsPage(ctx context.Context, runID string,
		offset int, limit int) ([]domain.SpecialistDelegationProposal, error)
	GetSpecialistDelegationReviewByProposal(ctx context.Context,
		proposalID string) (domain.SpecialistDelegationReview, bool, error)
	GetSpecialistDelegationApplicationByProposal(ctx context.Context,
		proposalID string) (domain.SpecialistDelegationApplication, bool, error)
	GetLatestSpecialistOperatorScheduleRequestByApplication(ctx context.Context,
		applicationID string) (domain.SpecialistOperatorScheduleRequest, bool, error)
	GetLatestSpecialistOperatorScheduleAttempt(ctx context.Context,
		requestID string) (domain.SpecialistSchedule, domain.SpecialistOperatorScheduleAttempt, bool, error)
	ListReadOnlyFanoutPlanSummariesPage(ctx context.Context, runID string,
		offset int, limit int) ([]domain.ReadOnlyFanoutPlanSummary, error)
	GetLatestReadOnlyFanoutExecutionSummary(ctx context.Context,
		planID string) (domain.ReadOnlyFanoutExecutionSummary, bool, error)
	ListFindingReportSummariesPage(ctx context.Context, runID string,
		offset int, limit int) ([]domain.FindingReportSummary, error)
	GetFindingReport(ctx context.Context, id string) (domain.FindingReport, error)
	GetApproval(ctx context.Context, id string) (approval.Record, error)
	ListApprovals(ctx context.Context, filter approval.ListFilter) ([]approval.Record, error)
	GetFileEditPreview(ctx context.Context, id string) (fileedit.Preview, error)
	ListFileEditPreviewsPage(ctx context.Context, filter fileedit.ListFilter,
		offset int, limit int) ([]fileedit.Preview, error)

	GetSession(ctx context.Context, id string) (session.Session, error)
	GetWorkspaceInfo(ctx context.Context, id string) (session.WorkspaceInfo, error)
	ListWorkspacesPage(ctx context.Context, offset int, limit int) ([]session.WorkspaceRecord, error)
	ListSessionsPage(ctx context.Context, offset int, limit int) ([]session.Session, error)
	ListSessionMessagesPage(ctx context.Context, sessionID string, includeCompacted bool,
		offset int, limit int) ([]session.Message, error)
	ListTerminalOperationRecords(context.Context, string, int) (
		[]operationreceipt.TerminalRecord, error)
	GetEvidenceAttachment(context.Context, string) (session.EvidenceAttachment,
		session.Message, bool, error)
	AttachEvidence(context.Context, session.EvidenceAttachment, session.Message) (
		session.EvidenceAttachment, session.Message, bool, error)

	GetWorkItem(ctx context.Context, id string) (domain.WorkItem, error)
	ListWorkItems(ctx context.Context, filter domain.WorkItemFilter) ([]domain.WorkItem, error)
	GetNote(ctx context.Context, id string) (domain.Note, error)
	ListNotes(ctx context.Context, filter domain.NoteFilter) ([]domain.Note, error)

	GetRunArtifactDescriptor(ctx context.Context, id string) (artifact.Descriptor, error)
	ListRunArtifacts(ctx context.Context, filter artifact.ListFilter) ([]artifact.Descriptor, error)
}

type Config struct {
	AccessToken                   string
	ControlToken                  string
	RunControlEnabled             bool
	RunCreationEnabled            bool
	SessionMessageEnabled         bool
	SessionSteeringControlEnabled bool
	RunLifecycleEnabled           bool
	RunExecutionEnabled           bool
	PlanDeliveryControlEnabled    bool
	ApprovalControlEnabled        bool
	ModelControlEnabled           bool
	ProviderCredentialEnabled     bool
	FileEditReviewEnabled         bool
	FileEditProposalEnabled       bool
	RunWakeControlEnabled         bool
	FileEditApplyEnabled          bool
	RunWakeExecutionEnabled       bool
	RunWakeWorkerEnabled          bool
	SkillInstallationEnabled      bool
	EvidenceAttachmentEnabled     bool
	RunLifecycleController        RunLifecycleController
	RunExecutionController        RunExecutionController
	PlanDeliveryController        PlanDeliveryController
	ApprovalController            ApprovalController
	ModelControlController        ModelControlController
	ProviderCredentialController  ProviderCredentialController
	FileEditReviewController      FileEditReviewController
	FileEditProposalController    FileEditProposalController
	RunWakeController             RunWakeController
	FileEditApplyController       FileEditApplyController
	RunWakeExecutionController    RunWakeExecutionController
	RunWakeWorkerHealthSource     RunWakeWorkerHealthSource
	SkillInstallationController   SkillInstallationController
	ModelRegistry                 *modelregistry.Registry
	AppVersion                    string
	EventStream                   EventStreamConfig
	UIHandler                     http.Handler
}

type API struct {
	store                         Store
	tokenHash                     [sha256.Size]byte
	controlTokenHash              [sha256.Size]byte
	controlEnabled                bool
	runCreationEnabled            bool
	sessionMessageEnabled         bool
	sessionSteeringControlEnabled bool
	runLifecycleEnabled           bool
	runExecutionEnabled           bool
	planDeliveryControlEnabled    bool
	approvalControlEnabled        bool
	modelControlEnabled           bool
	providerCredentialEnabled     bool
	fileEditReviewEnabled         bool
	fileEditProposalEnabled       bool
	runWakeControlEnabled         bool
	fileEditApplyEnabled          bool
	runWakeExecutionEnabled       bool
	runWakeWorkerEnabled          bool
	skillInstallationEnabled      bool
	evidenceAttachmentEnabled     bool
	runLifecycleController        RunLifecycleController
	runExecutionController        RunExecutionController
	planDeliveryController        PlanDeliveryController
	approvalController            ApprovalController
	modelControlController        ModelControlController
	providerCredentialController  ProviderCredentialController
	fileEditReviewController      FileEditReviewController
	fileEditProposalController    FileEditProposalController
	runWakeController             RunWakeController
	fileEditApplyController       FileEditApplyController
	runWakeExecutionController    RunWakeExecutionController
	runWakeWorkerHealthSource     RunWakeWorkerHealthSource
	skillInstallationController   SkillInstallationController
	modelRegistry                 *modelregistry.Registry
	appVersion                    string
	openAPI                       []byte
	eventStream                   EventStreamConfig
	eventStreamSlots              chan struct{}
	uiHandler                     http.Handler
}

func New(store Store, config Config) (*API, error) {
	if store == nil {
		return nil, errors.New("HTTP API store is required")
	}
	token := config.AccessToken
	if err := validateAccessToken(token); err != nil {
		return nil, err
	}
	controlToken := config.ControlToken
	controlTokenPresent := controlToken != ""
	var controlTokenHash [sha256.Size]byte
	if controlTokenPresent {
		if err := validateAccessToken(controlToken); err != nil {
			return nil, apperror.Wrap(apperror.CodeInvalidArgument, "invalid HTTP API control token", err)
		}
		controlTokenHash = sha256.Sum256([]byte(controlToken))
		accessTokenHash := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(accessTokenHash[:], controlTokenHash[:]) == 1 {
			return nil, apperror.New(apperror.CodeInvalidArgument,
				"HTTP API read and control tokens must be distinct")
		}
	}
	if (config.RunControlEnabled || config.RunCreationEnabled || config.SessionMessageEnabled ||
		config.SessionSteeringControlEnabled || config.RunLifecycleEnabled ||
		config.RunExecutionEnabled || config.PlanDeliveryControlEnabled ||
		config.ApprovalControlEnabled || config.ModelControlEnabled ||
		config.ProviderCredentialEnabled ||
		config.FileEditReviewEnabled || config.FileEditProposalEnabled ||
		config.RunWakeControlEnabled ||
		config.FileEditApplyEnabled || config.RunWakeExecutionEnabled ||
		config.RunWakeWorkerEnabled ||
		config.SkillInstallationEnabled || config.EvidenceAttachmentEnabled) &&
		!controlTokenPresent {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API control capabilities require a control token")
	}
	if config.RunLifecycleEnabled && config.RunLifecycleController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API Run lifecycle controller is required when enabled")
	}
	if config.RunExecutionEnabled && config.RunExecutionController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API Run execution controller is required when enabled")
	}
	if config.PlanDeliveryControlEnabled && config.PlanDeliveryController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API Plan/Delivery controller is required when enabled")
	}
	if config.ApprovalControlEnabled && config.ApprovalController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API approval controller is required when enabled")
	}
	if config.ModelControlEnabled && config.ModelControlController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API model controller is required when enabled")
	}
	if config.ProviderCredentialEnabled && config.ProviderCredentialController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API Provider credential controller is required when enabled")
	}
	if config.FileEditReviewEnabled && config.FileEditReviewController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API file edit review controller is required when enabled")
	}
	if config.FileEditProposalEnabled && config.FileEditProposalController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API file edit proposal controller is required when enabled")
	}
	if config.RunWakeControlEnabled && config.RunWakeController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API Run wake controller is required when enabled")
	}
	if config.FileEditApplyEnabled && config.FileEditApplyController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API FileEdit apply controller is required when enabled")
	}
	if config.RunWakeExecutionEnabled && config.RunWakeExecutionController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API foreground Run wake controller is required when enabled")
	}
	if config.RunWakeWorkerEnabled != (config.RunWakeWorkerHealthSource != nil) {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API Run wake worker health source must exactly match enablement")
	}
	if config.SkillInstallationEnabled && config.SkillInstallationController == nil {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API Skill installation controller is required when enabled")
	}
	version := strings.TrimSpace(config.AppVersion)
	if version == "" {
		version = "unknown"
	}
	modelRegistry := config.ModelRegistry
	if modelRegistry == nil {
		modelRegistry = modelregistry.New(nil)
	}
	document, err := GenerateOpenAPI()
	if err != nil {
		return nil, fmt.Errorf("generate OpenAPI document: %w", err)
	}
	eventStream, err := normalizeEventStreamConfig(config.EventStream)
	if err != nil {
		return nil, err
	}
	return &API{store: store, tokenHash: sha256.Sum256([]byte(token)),
		controlTokenHash:   controlTokenHash,
		controlEnabled:     controlTokenPresent && config.RunControlEnabled,
		runCreationEnabled: controlTokenPresent && config.RunCreationEnabled, appVersion: version,
		sessionMessageEnabled:         controlTokenPresent && config.SessionMessageEnabled,
		sessionSteeringControlEnabled: controlTokenPresent && config.SessionSteeringControlEnabled,
		runLifecycleEnabled:           controlTokenPresent && config.RunLifecycleEnabled,
		runExecutionEnabled:           controlTokenPresent && config.RunExecutionEnabled,
		planDeliveryControlEnabled:    controlTokenPresent && config.PlanDeliveryControlEnabled,
		approvalControlEnabled:        controlTokenPresent && config.ApprovalControlEnabled,
		modelControlEnabled:           controlTokenPresent && config.ModelControlEnabled,
		providerCredentialEnabled:     controlTokenPresent && config.ProviderCredentialEnabled,
		fileEditReviewEnabled:         controlTokenPresent && config.FileEditReviewEnabled,
		fileEditProposalEnabled:       controlTokenPresent && config.FileEditProposalEnabled,
		runWakeControlEnabled:         controlTokenPresent && config.RunWakeControlEnabled,
		fileEditApplyEnabled:          controlTokenPresent && config.FileEditApplyEnabled,
		runWakeExecutionEnabled:       controlTokenPresent && config.RunWakeExecutionEnabled,
		runWakeWorkerEnabled:          controlTokenPresent && config.RunWakeWorkerEnabled,
		skillInstallationEnabled:      controlTokenPresent && config.SkillInstallationEnabled,
		evidenceAttachmentEnabled:     controlTokenPresent && config.EvidenceAttachmentEnabled,
		runLifecycleController:        config.RunLifecycleController,
		runExecutionController:        config.RunExecutionController,
		planDeliveryController:        config.PlanDeliveryController,
		approvalController:            config.ApprovalController,
		modelControlController:        config.ModelControlController,
		providerCredentialController:  config.ProviderCredentialController,
		fileEditReviewController:      config.FileEditReviewController,
		fileEditProposalController:    config.FileEditProposalController,
		runWakeController:             config.RunWakeController,
		fileEditApplyController:       config.FileEditApplyController,
		runWakeExecutionController:    config.RunWakeExecutionController,
		runWakeWorkerHealthSource:     config.RunWakeWorkerHealthSource,
		skillInstallationController:   config.SkillInstallationController,
		modelRegistry:                 modelRegistry,
		openAPI:                       document, eventStream: eventStream,
		eventStreamSlots: make(chan struct{}, eventStream.MaxConnections),
		uiHandler:        config.UIHandler}, nil
}

func GenerateAccessToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate HTTP API access token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func validateAccessToken(token string) error {
	if token != strings.TrimSpace(token) || !utf8.ValidString(token) ||
		len([]byte(token)) < MinAccessTokenBytes || len([]byte(token)) > MaxAccessTokenBytes {
		return apperror.New(apperror.CodeInvalidArgument,
			fmt.Sprintf("HTTP API access token must be normalized UTF-8 between %d and %d bytes",
				MinAccessTokenBytes, MaxAccessTokenBytes))
	}
	for _, current := range token {
		if unicode.IsControl(current) || unicode.IsSpace(current) {
			return apperror.New(apperror.CodeInvalidArgument,
				"HTTP API access token cannot contain whitespace or control characters")
		}
	}
	return nil
}

func ListenLoopback(ctx context.Context, address string) (net.Listener, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	address = strings.TrimSpace(address)
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return nil, apperror.New(apperror.CodeInvalidArgument,
			"HTTP API listen address must use loopback-host:port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 {
		return nil, apperror.New(apperror.CodeInvalidArgument, "HTTP API listen port is invalid")
	}
	if !loopbackHost(host) {
		return nil, apperror.New(apperror.CodePolicyDenied,
			"HTTP API listen host must be localhost or a loopback IP address")
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, apperror.Wrap(apperror.CodeUnavailable, "HTTP API listen failed", err)
	}
	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok || tcpAddress.IP == nil || !tcpAddress.IP.IsLoopback() {
		_ = listener.Close()
		return nil, apperror.New(apperror.CodePolicyDenied,
			"HTTP API listener resolved outside the loopback interface")
	}
	return listener, nil
}

func loopbackHost(host string) bool {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (a *API) Handler() http.Handler {
	return a
}

func (a *API) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	requestID := idgen.New("req")
	tracked := &responseWriter{ResponseWriter: writer}
	uiRequest := a.uiHandler != nil && request != nil && request.URL != nil &&
		!reservedAPIPath(request.URL.Path)
	if uiRequest {
		setUISecurityHeaders(tracked.Header(), requestID)
	} else {
		setSecurityHeaders(tracked.Header(), requestID)
	}
	defer func() {
		if recover() != nil && !tracked.wrote {
			if uiRequest {
				writeUIError(tracked, request,
					apperror.New(apperror.CodeInternal, "internal server error"), 0)
			} else {
				a.writeError(tracked, requestID,
					apperror.New(apperror.CodeInternal, "internal server error"), 0)
			}
		}
	}()

	if status, err := validateRequestBoundary(request); err != nil {
		if uiRequest {
			writeUIError(tracked, request, err, status)
		} else {
			a.writeError(tracked, requestID, err, status)
		}
		return
	}
	if uiRequest {
		a.serveUI(tracked, request)
		return
	}
	if request.URL.Path == "/api/v1/runs" && request.Method != http.MethodGet {
		a.serveRunCreationControl(tracked, request, requestID)
		return
	}
	if sessionID, matched := matchSessionMessageControlPath(request.URL.Path); matched &&
		request.Method != http.MethodGet {
		a.serveSessionMessageControl(tracked, request, requestID, sessionID)
		return
	}
	if sessionID, messageID, matched := matchSessionSteeringCancellationPath(request.URL.Path); matched {
		a.serveSessionSteeringCancellation(tracked, request, requestID, sessionID, messageID)
		return
	}
	if runID, matched := matchRunLifecycleControlPath(request.URL.Path); matched {
		a.serveRunLifecycleControl(tracked, request, requestID, runID)
		return
	}
	if runID, matched := matchPlanDirectionControlPath(request.URL.Path); matched {
		a.servePlanDirectionControl(tracked, request, requestID, runID)
		return
	}
	if runID, matched := matchPlanDeliveryControlPath(request.URL.Path); matched {
		a.servePlanDeliveryControl(tracked, request, requestID, runID)
		return
	}
	if runID, approvalID, matched := matchApprovalDecisionControlPath(request.URL.Path); matched {
		a.serveApprovalDecisionControl(tracked, request, requestID, runID, approvalID)
		return
	}
	if route, diagnostic, matched := matchModelControlPath(request.URL.Path); matched {
		a.serveModelControl(tracked, request, requestID, route, diagnostic)
		return
	}
	if provider, matched := matchProviderCredentialControlPath(request.URL.Path); matched {
		a.serveProviderCredentialControl(tracked, request, requestID, provider)
		return
	}
	if runID, matched := matchFileEditProposalControlPath(request.URL.Path); matched {
		a.serveFileEditProposalControl(tracked, request, requestID, runID)
		return
	}
	if runID, editID, matched := matchFileEditReviewControlPath(request.URL.Path); matched {
		a.serveFileEditReviewControl(tracked, request, requestID, runID, editID)
		return
	}
	if runID, editID, matched := matchFileEditApplyControlPath(request.URL.Path); matched {
		a.serveFileEditApplyControl(tracked, request, requestID, runID, editID)
		return
	}
	if runID, cancel, matched := matchRunWakeControlPath(request.URL.Path); matched &&
		request.Method != http.MethodGet {
		a.serveRunWakeControl(tracked, request, requestID, runID, cancel)
		return
	}
	if runID, matched := matchRunWakeExecutionPath(request.URL.Path); matched {
		a.serveRunWakeExecutionControl(tracked, request, requestID, runID)
		return
	}
	if request.URL.Path == SkillPackageInstallPath {
		a.serveSkillPackageInstallControl(tracked, request, requestID)
		return
	}
	if runID, matched := matchEvidenceAttachmentPath(request.URL.Path); matched &&
		request.Method != http.MethodGet {
		a.serveEvidenceAttachmentControl(tracked, request, requestID, runID)
		return
	}
	if runID, matched := matchRunExecutionControlPath(request.URL.Path); matched {
		a.serveRunExecutionControl(tracked, request, requestID, runID)
		return
	}
	if runID, matched := matchRunExecutionProfileControlPath(request.URL.Path); matched {
		a.serveRunExecutionProfileControl(tracked, request, requestID, runID)
		return
	}
	if runID, agentID, matched := matchSpecialistModelCancellationPath(request.URL.Path); matched {
		a.serveSpecialistModelCancellation(tracked, request, requestID, runID, agentID)
		return
	}
	if runID, matched := matchModelCancellationPath(request.URL.Path); matched {
		a.serveModelCancellation(tracked, request, requestID, runID)
		return
	}
	if !a.authorized(request, a.tokenHash) {
		tracked.Header().Set("WWW-Authenticate", `Bearer realm="CyberAgent API"`)
		a.writeError(tracked, requestID,
			apperror.New(apperror.CodePolicyDenied, "valid bearer authorization is required"),
			http.StatusUnauthorized)
		return
	}
	if request.Method != http.MethodGet {
		tracked.Header().Set("Allow", http.MethodGet)
		a.writeError(tracked, requestID,
			apperror.New(apperror.CodeInvalidArgument, "HTTP API endpoint only supports GET"),
			http.StatusMethodNotAllowed)
		return
	}
	if request.ContentLength != 0 || len(request.TransferEncoding) != 0 {
		a.writeError(tracked, requestID,
			apperror.New(apperror.CodeInvalidArgument, "read-only HTTP API requests cannot contain a body"), 0)
		return
	}
	if request.URL.Path == OpenAPIPath {
		if err := rejectQuery(request.URL.Query()); err != nil {
			a.writeError(tracked, requestID, err, 0)
			return
		}
		a.writeOpenAPI(tracked, requestID)
		return
	}
	if runID, matched := matchRunEventStreamPath(request.URL.Path); matched {
		if err := validatePathIdentity(runID); err != nil {
			a.writeError(tracked, requestID, err, 0)
			return
		}
		a.serveRunEventStream(tracked, request, requestID, runID)
		return
	}
	if runID, matched := matchRunEventPollPath(request.URL.Path); matched {
		if err := validatePathIdentity(runID); err != nil {
			a.writeError(tracked, requestID, err, 0)
			return
		}
		a.serveRunEventPoll(tracked, request, requestID, runID)
		return
	}
	data, page, err := a.route(request)
	if err != nil {
		a.writeError(tracked, requestID, err, 0)
		return
	}
	a.writeSuccess(tracked, requestID, data, page)
}

func reservedAPIPath(value string) bool {
	return value == "/api" || strings.HasPrefix(value, "/api/")
}

func (a *API) serveUI(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		writeUIError(writer, request,
			apperror.New(apperror.CodeInvalidArgument, "Web UI only supports GET and HEAD"),
			http.StatusMethodNotAllowed)
		return
	}
	if request.ContentLength != 0 || len(request.TransferEncoding) != 0 {
		writeUIError(writer, request,
			apperror.New(apperror.CodeInvalidArgument, "Web UI requests cannot contain a body"), 0)
		return
	}
	if request.URL.RawQuery != "" {
		writeUIError(writer, request,
			apperror.New(apperror.CodeInvalidArgument, "Web UI requests cannot contain query parameters"), 0)
		return
	}
	if len(request.Header.Values("Authorization")) != 0 {
		writeUIError(writer, request,
			apperror.New(apperror.CodeInvalidArgument, "Web UI asset requests cannot contain authorization"), 0)
		return
	}
	a.uiHandler.ServeHTTP(writer, request)
}

func validateRequestBoundary(request *http.Request) (int, error) {
	if request == nil || request.URL == nil {
		return 0, apperror.New(apperror.CodeInvalidArgument, "HTTP request is required")
	}
	if len(request.RequestURI) > MaxRequestTargetBytes {
		return http.StatusRequestURITooLong,
			apperror.New(apperror.CodeResourceExhausted, "HTTP request target exceeds its limit")
	}
	if len(request.URL.RawQuery) > MaxQueryBytes {
		return http.StatusRequestURITooLong,
			apperror.New(apperror.CodeResourceExhausted, "HTTP query exceeds its limit")
	}
	if request.URL.Path == "" || request.URL.Path != path.Clean(request.URL.Path) ||
		strings.Contains(request.URL.Path, `\`) {
		return 0, apperror.New(apperror.CodeInvalidArgument, "HTTP path is not canonical")
	}
	if !loopbackRequestHost(request.Host) {
		return 0, apperror.New(apperror.CodePolicyDenied, "HTTP Host must identify a loopback address")
	}
	remoteHost, _, err := net.SplitHostPort(strings.TrimSpace(request.RemoteAddr))
	if err != nil || !loopbackHost(remoteHost) {
		return 0, apperror.New(apperror.CodePolicyDenied, "HTTP client must connect from loopback")
	}
	return 0, nil
}

func loopbackRequestHost(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		host = value
		if strings.Contains(strings.Trim(host, "[]"), ":") && net.ParseIP(strings.Trim(host, "[]")) == nil {
			return false
		}
	}
	return loopbackHost(host)
}

func (a *API) authorized(request *http.Request, tokenHash [sha256.Size]byte) bool {
	values := request.Header.Values("Authorization")
	if len(values) != 1 {
		return false
	}
	parts := strings.SplitN(strings.TrimSpace(values[0]), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return false
	}
	candidate := sha256.Sum256([]byte(parts[1]))
	return subtle.ConstantTimeCompare(candidate[:], tokenHash[:]) == 1
}

func setSecurityHeaders(header http.Header, requestID string) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("X-CyberAgent-API-Version", Version)
	header.Set("X-Request-ID", requestID)
}

func setUISecurityHeaders(header http.Header, requestID string) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; connect-src 'self'; "+
		"font-src 'self'; form-action 'none'; frame-ancestors 'none'; img-src 'self' data:; "+
		"manifest-src 'self'; object-src 'none'; script-src 'self'; style-src 'self'; worker-src 'self'")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), payment=(), usb=()")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("X-CyberAgent-UI-Version", "web-ui.v1")
	header.Set("X-Request-ID", requestID)
}

func writeUIError(writer http.ResponseWriter, request *http.Request, err error, statusOverride int) {
	status := statusOverride
	if status == 0 {
		status = apperror.HTTPStatus(apperror.Normalize(err))
	}
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(status)
	if request == nil || request.Method != http.MethodHead {
		_, _ = io.WriteString(writer, http.StatusText(status)+"\n")
	}
}

type Server struct {
	httpServer     *http.Server
	cancelRequests context.CancelFunc
}

func NewServer(api *API, errorLog *log.Logger) (*Server, error) {
	if api == nil {
		return nil, errors.New("HTTP API is required")
	}
	if errorLog == nil {
		errorLog = log.New(io.Discard, "", 0)
	}
	requestContext, cancelRequests := context.WithCancel(context.Background())
	return &Server{httpServer: &http.Server{
		Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 * 1024,
		ErrorLog: errorLog, BaseContext: func(net.Listener) context.Context { return requestContext },
	}, cancelRequests: cancelRequests}, nil
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if s == nil || s.httpServer == nil || s.cancelRequests == nil || listener == nil {
		return apperror.New(apperror.CodeFailedPrecondition, "HTTP API server and listener are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer s.cancelRequests()
	done := make(chan error, 1)
	go func() { done <- s.httpServer.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return apperror.Wrap(apperror.CodeUnavailable, "HTTP API server stopped", err)
	case <-ctx.Done():
		s.cancelRequests()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr := s.httpServer.Shutdown(shutdownCtx)
		serveErr := <-done
		if shutdownErr != nil {
			return apperror.Wrap(apperror.CodeUnavailable, "HTTP API shutdown failed", shutdownErr)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return apperror.Wrap(apperror.CodeUnavailable, "HTTP API server stopped", serveErr)
		}
		return nil
	}
}

type responseWriter struct {
	http.ResponseWriter
	wrote bool
}

func (w *responseWriter) WriteHeader(status int) {
	if !w.wrote {
		w.wrote = true
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
