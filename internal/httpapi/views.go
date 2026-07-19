package httpapi

import (
	"encoding/json"
	"time"

	"cyberagent-workbench/internal/application"
	"cyberagent-workbench/internal/artifact"
	"cyberagent-workbench/internal/domain"
	"cyberagent-workbench/internal/events"
	"cyberagent-workbench/internal/operationreceipt"
	"cyberagent-workbench/internal/operatoraction"
	"cyberagent-workbench/internal/session"
	"cyberagent-workbench/internal/toolbudget"
)

type IndexView struct {
	APIVersion string   `json:"api_version"`
	AppVersion string   `json:"app_version"`
	Resources  []string `json:"resources"`
}

type HealthView struct {
	Status        string `json:"status"`
	APIVersion    string `json:"api_version"`
	AppVersion    string `json:"app_version"`
	SchemaVersion int    `json:"schema_version"`
}

type ProviderAvailabilityView struct {
	Name               string   `json:"name"`
	Kind               string   `json:"kind"`
	Status             string   `json:"status"`
	Models             []string `json:"models"`
	CredentialSource   string   `json:"credential_source"`
	NetworkRequired    bool     `json:"network_required"`
	ConfigurationError bool     `json:"configuration_error"`
}

type ModelRouteAvailabilityView struct {
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Available bool   `json:"available"`
}

type ModelAvailabilityView struct {
	ProtocolVersion string                       `json:"protocol_version"`
	Generation      uint64                       `json:"generation"`
	Providers       []ProviderAvailabilityView   `json:"providers"`
	Routes          []ModelRouteAvailabilityView `json:"routes"`
}

type ProviderDiagnosticView struct {
	ProtocolVersion         string `json:"protocol_version"`
	Provider                string `json:"provider"`
	Model                   string `json:"model"`
	Status                  string `json:"status"`
	Outcome                 string `json:"outcome"`
	Retryable               bool   `json:"retryable"`
	NetworkRequestAttempted bool   `json:"network_request_attempted"`
	ModelCalled             bool   `json:"model_called"`
	ToolCalled              bool   `json:"tool_called"`
	ResponseContentReturned bool   `json:"response_content_returned"`
	DurationMillis          int64  `json:"duration_ms"`
}

type FileEditPreviewView struct {
	ID              string                             `json:"id"`
	SessionID       string                             `json:"session_id"`
	WorkspaceID     string                             `json:"workspace_id"`
	Path            string                             `json:"path"`
	Status          string                             `json:"status"`
	Diff            string                             `json:"diff"`
	OriginalHash    string                             `json:"original_hash"`
	ProposedHash    string                             `json:"proposed_hash"`
	Reason          string                             `json:"reason,omitempty"`
	SecretsRedacted bool                               `json:"secrets_redacted"`
	AllowedActions  []application.FileEditReviewAction `json:"allowed_actions"`
	CreatedAt       time.Time                          `json:"created_at"`
	UpdatedAt       time.Time                          `json:"updated_at"`
	ApplyEnabled    bool                               `json:"apply_enabled"`
}

type FileEditQueueView struct {
	ProtocolVersion string                `json:"protocol_version"`
	RunID           string                `json:"run_id"`
	Items           []FileEditPreviewView `json:"items"`
	Truncated       bool                  `json:"truncated"`
	ApplyEnabled    bool                  `json:"apply_enabled"`
}

type FileEditReviewView struct {
	ProtocolVersion string              `json:"protocol_version"`
	RunID           string              `json:"run_id"`
	Action          string              `json:"action"`
	Edit            FileEditPreviewView `json:"edit"`
	Replayed        bool                `json:"replayed"`
	FileWritten     bool                `json:"file_written"`
}

type OperationReceiptView struct {
	ProtocolVersion string                          `json:"protocol_version"`
	Kind            operationreceipt.Kind           `json:"kind"`
	Outcome         string                          `json:"outcome"`
	Durable         bool                            `json:"durable"`
	Replayed        bool                            `json:"replayed"`
	RetrySafe       bool                            `json:"retry_safe"`
	RetryStrategy   operationreceipt.RetryStrategy  `json:"retry_strategy"`
	RecoveryAction  operationreceipt.RecoveryAction `json:"recovery_action"`
	CleanupState    operationreceipt.CleanupState   `json:"cleanup_state"`
}

type OperationReceiptHistoryItemView struct {
	ID          string               `json:"id"`
	Scope       string               `json:"scope"`
	RunID       string               `json:"run_id,omitempty"`
	CompletedAt time.Time            `json:"completed_at"`
	Receipt     OperationReceiptView `json:"receipt"`
}

type OperationReceiptHistoryView struct {
	ProtocolVersion string                            `json:"protocol_version"`
	Items           []OperationReceiptHistoryItemView `json:"items"`
	Truncated       bool                              `json:"truncated"`
}

type OperatorActionItemView struct {
	ID          string                     `json:"id"`
	Kind        operatoraction.Kind        `json:"kind"`
	State       string                     `json:"state"`
	Destination operatoraction.Destination `json:"destination"`
	AvailableAt time.Time                  `json:"available_at"`
	DueAt       *time.Time                 `json:"due_at,omitempty"`
}

type OperatorActionCenterView struct {
	ProtocolVersion string                   `json:"protocol_version"`
	RunID           string                   `json:"run_id"`
	GeneratedAt     time.Time                `json:"generated_at"`
	Items           []OperatorActionItemView `json:"items"`
	Truncated       bool                     `json:"truncated"`
}

type EvidenceInventoryItemView struct {
	AttachmentID          string    `json:"attachment_id"`
	RunID                 string    `json:"run_id"`
	SessionID             string    `json:"session_id"`
	WorkspaceID           string    `json:"workspace_id"`
	SourceKind            string    `json:"source_kind"`
	SourceRef             string    `json:"source_ref"`
	ContentSHA256         string    `json:"content_sha256"`
	InstructionAuthorized bool      `json:"instruction_authorized"`
	AttachedAt            time.Time `json:"attached_at"`
}

type EvidenceInventoryView struct {
	ProtocolVersion string                      `json:"protocol_version"`
	RunID           string                      `json:"run_id"`
	Items           []EvidenceInventoryItemView `json:"items"`
	Truncated       bool                        `json:"truncated"`
}

type FileEditApplyView struct {
	ProtocolVersion string               `json:"protocol_version"`
	RunID           string               `json:"run_id"`
	Edit            FileEditPreviewView  `json:"edit"`
	Status          string               `json:"status"`
	Replayed        bool                 `json:"replayed"`
	FileWritten     bool                 `json:"file_written"`
	PolicyRechecked bool                 `json:"policy_rechecked"`
	Receipt         OperationReceiptView `json:"receipt"`
}

type RunWakeIntentView struct {
	ID                    string     `json:"id"`
	ProtocolVersion       string     `json:"protocol_version"`
	RunID                 string     `json:"run_id"`
	SessionID             string     `json:"session_id"`
	Status                string     `json:"status"`
	MaxAttempts           int        `json:"max_attempts"`
	AttemptCount          int        `json:"attempt_count"`
	InitialDelaySeconds   int        `json:"initial_delay_seconds"`
	BaseBackoffSeconds    int        `json:"base_backoff_seconds"`
	MaxBackoffSeconds     int        `json:"max_backoff_seconds"`
	MaxElapsedSeconds     int        `json:"max_elapsed_seconds"`
	NextWakeAt            time.Time  `json:"next_wake_at"`
	DeadlineAt            time.Time  `json:"deadline_at"`
	ExecutionEnabled      bool       `json:"execution_enabled"`
	BackgroundLoopEnabled bool       `json:"background_loop_enabled"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	CancelledAt           *time.Time `json:"cancelled_at,omitempty"`
}

type RunWakeStateView struct {
	ProtocolVersion string             `json:"protocol_version"`
	RunID           string             `json:"run_id"`
	Found           bool               `json:"found"`
	Intent          *RunWakeIntentView `json:"intent,omitempty"`
}

type RunWakeControlView struct {
	ProtocolVersion  string            `json:"protocol_version"`
	Action           string            `json:"action"`
	Intent           RunWakeIntentView `json:"intent"`
	Replayed         bool              `json:"replayed"`
	ExecutionStarted bool              `json:"execution_started"`
	ModelCalled      bool              `json:"model_called"`
	ToolCalled       bool              `json:"tool_called"`
}

type RunWakeExecutionView struct {
	ProtocolVersion       string               `json:"protocol_version"`
	RunID                 string               `json:"run_id"`
	Intent                RunWakeIntentView    `json:"intent"`
	ConsumptionStatus     string               `json:"consumption_status"`
	StopReason            string               `json:"stop_reason,omitempty"`
	Replayed              bool                 `json:"replayed"`
	ExecutionStarted      bool                 `json:"execution_started"`
	ModelCalled           bool                 `json:"model_called"`
	ToolCalled            bool                 `json:"tool_called"`
	BackgroundLoopEnabled bool                 `json:"background_loop_enabled"`
	Receipt               OperationReceiptView `json:"receipt"`
}

type ApprovalQueueItemView struct {
	ID                      string                              `json:"id"`
	ProposalID              string                              `json:"proposal_id"`
	RunID                   string                              `json:"run_id"`
	SessionID               string                              `json:"session_id"`
	WorkspaceID             string                              `json:"workspace_id"`
	ToolName                string                              `json:"tool_name"`
	ActionClass             string                              `json:"action_class"`
	Mode                    string                              `json:"mode"`
	Status                  string                              `json:"status"`
	AllowedActions          []application.ApprovalControlAction `json:"allowed_actions"`
	Version                 int64                               `json:"version"`
	CreatedAt               time.Time                           `json:"created_at"`
	UpdatedAt               time.Time                           `json:"updated_at"`
	ProcessExecutionEnabled bool                                `json:"process_execution_enabled"`
	CapabilityGrant         bool                                `json:"capability_grant"`
}

type ApprovalQueueView struct {
	ProtocolVersion         string                  `json:"protocol_version"`
	RunID                   string                  `json:"run_id"`
	Items                   []ApprovalQueueItemView `json:"items"`
	Truncated               bool                    `json:"truncated"`
	ProcessExecutionEnabled bool                    `json:"process_execution_enabled"`
	SessionGrantCreated     bool                    `json:"session_grant_created"`
	CapabilityGrant         bool                    `json:"capability_grant"`
}

type WorkspaceView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type WorkspaceExplorerEntryView struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
	Readable  bool   `json:"readable"`
}

type WorkspaceExplorerProvenanceView struct {
	Version               string `json:"version"`
	SourceKind            string `json:"source_kind"`
	SourceRef             string `json:"source_ref"`
	ContentSHA256         string `json:"content_sha256"`
	InstructionAuthorized bool   `json:"instruction_authorized"`
}

type WorkspaceExplorerView struct {
	ProtocolVersion string                          `json:"protocol_version"`
	WorkspaceID     string                          `json:"workspace_id"`
	Path            string                          `json:"path"`
	Kind            string                          `json:"kind"`
	Entries         []WorkspaceExplorerEntryView    `json:"entries"`
	Content         string                          `json:"content"`
	TotalBytes      int64                           `json:"total_bytes"`
	ReturnedBytes   int                             `json:"returned_bytes"`
	Truncated       bool                            `json:"truncated"`
	RedactionCount  int                             `json:"redaction_count"`
	RootPathExposed bool                            `json:"root_path_exposed"`
	Provenance      WorkspaceExplorerProvenanceView `json:"provenance"`
}

type WorkspaceSearchResultView struct {
	Path             string                          `json:"path"`
	MatchKind        string                          `json:"match_kind"`
	Line             int                             `json:"line"`
	Snippet          string                          `json:"snippet"`
	ContentTruncated bool                            `json:"content_truncated"`
	Provenance       WorkspaceExplorerProvenanceView `json:"provenance"`
}

type WorkspaceSearchView struct {
	ProtocolVersion string                      `json:"protocol_version"`
	WorkspaceID     string                      `json:"workspace_id"`
	Results         []WorkspaceSearchResultView `json:"results"`
	ScannedEntries  int                         `json:"scanned_entries"`
	ScannedFiles    int                         `json:"scanned_files"`
	ScannedBytes    int64                       `json:"scanned_bytes"`
	Truncated       bool                        `json:"truncated"`
	RootPathExposed bool                        `json:"root_path_exposed"`
}

type ScopeView struct {
	WorkspaceID    string   `json:"workspace_id,omitempty"`
	NetworkMode    string   `json:"network_mode"`
	AllowedTargets []string `json:"allowed_targets,omitempty"`
}

type MissionView struct {
	ID          string    `json:"id"`
	Goal        string    `json:"goal"`
	Profile     string    `json:"profile"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	Scope       ScopeView `json:"scope"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RunConfigView struct {
	ModelRoute  string `json:"model_route"`
	Interactive bool   `json:"interactive"`
}

type BudgetView struct {
	MaxTurns       int     `json:"max_turns"`
	MaxTokens      int64   `json:"max_tokens,omitempty"`
	MaxToolCalls   int64   `json:"max_tool_calls,omitempty"`
	MaxCostUSD     float64 `json:"max_cost_usd,omitempty"`
	TimeoutSeconds int64   `json:"timeout_seconds,omitempty"`
}

type RunView struct {
	ID         string        `json:"id"`
	MissionID  string        `json:"mission_id"`
	SessionID  string        `json:"session_id,omitempty"`
	Status     string        `json:"status"`
	Config     RunConfigView `json:"config"`
	Budget     BudgetView    `json:"budget"`
	StartedAt  *time.Time    `json:"started_at,omitempty"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
}

type RunModeView struct {
	ProtocolVersion string    `json:"protocol_version"`
	Revision        int64     `json:"revision"`
	Surface         string    `json:"surface"`
	Phase           string    `json:"phase"`
	Profile         string    `json:"profile"`
	Scope           ScopeView `json:"scope"`
	PolicyVersion   string    `json:"policy_version"`
	RequestedBy     string    `json:"requested_by"`
	Reason          string    `json:"reason"`
	CreatedAt       time.Time `json:"created_at"`
	CapabilityGrant bool      `json:"capability_grant"`
}

type RunExecutionProfileView struct {
	ProtocolVersion     string    `json:"protocol_version"`
	Revision            int64     `json:"revision"`
	Profile             string    `json:"profile"`
	Backend             string    `json:"backend"`
	ApprovalPolicy      string    `json:"approval_policy"`
	FilesystemScope     string    `json:"filesystem_scope"`
	NetworkScope        string    `json:"network_scope"`
	RiskTier            string    `json:"risk_tier"`
	RequiredGate        string    `json:"required_gate"`
	PolicyVersion       string    `json:"policy_version"`
	CreatedAt           time.Time `json:"created_at"`
	ProcessEnabled      bool      `json:"process_enabled"`
	ExecutionAuthorized bool      `json:"execution_authorized"`
	CapabilityGrant     bool      `json:"capability_grant"`
}

type SupervisorCheckpointView struct {
	RunID           string    `json:"run_id"`
	NextTurn        int       `json:"next_turn"`
	Phase           string    `json:"phase"`
	AttemptID       string    `json:"attempt_id,omitempty"`
	RepairPhase     string    `json:"repair_phase,omitempty"`
	RepairReason    string    `json:"repair_reason,omitempty"`
	LastError       string    `json:"last_error,omitempty"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
	ExecutionMillis int64     `json:"execution_millis"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ToolUsageView struct {
	Consumed    int64      `json:"consumed"`
	Limit       int64      `json:"limit"`
	Remaining   int64      `json:"remaining"`
	ExhaustedAt *time.Time `json:"exhausted_at,omitempty"`
}

type RunExecutionLeaseView struct {
	OwnerID    string     `json:"owner_id"`
	Generation int64      `json:"generation"`
	Status     string     `json:"status"`
	Active     bool       `json:"active"`
	AcquiredAt time.Time  `json:"acquired_at"`
	RenewedAt  time.Time  `json:"renewed_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
}

type OperatorSteeringMessageView struct {
	ID          string     `json:"id"`
	Sequence    int64      `json:"sequence"`
	Status      string     `json:"status"`
	Prepared    bool       `json:"prepared"`
	CreatedAt   time.Time  `json:"created_at"`
	CommittedAt *time.Time `json:"committed_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
}

type OperatorSteeringQueueView struct {
	Pending   int                           `json:"pending"`
	Prepared  int                           `json:"prepared"`
	Committed int                           `json:"committed"`
	Cancelled int                           `json:"cancelled"`
	Messages  []OperatorSteeringMessageView `json:"messages"`
}

type PlanDeliveryModuleView struct {
	Ordinal            int      `json:"ordinal"`
	Title              string   `json:"title"`
	Objective          string   `json:"objective"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Dependencies       []int    `json:"dependencies"`
}

type PlanDeliveryDirectionView struct {
	Ordinal   int                      `json:"ordinal"`
	Title     string                   `json:"title"`
	Summary   string                   `json:"summary"`
	Tradeoffs []string                 `json:"tradeoffs"`
	Modules   []PlanDeliveryModuleView `json:"modules"`
}

type PlanDeliveryProposalView struct {
	ID              string                      `json:"id"`
	ProtocolVersion string                      `json:"protocol_version"`
	Status          string                      `json:"status"`
	ModeRevision    int64                       `json:"mode_revision"`
	Directions      []PlanDeliveryDirectionView `json:"directions"`
	Version         int64                       `json:"version"`
	CreatedAt       time.Time                   `json:"created_at"`
}

type PlanDeliverySelectionItemView struct {
	Ordinal       int    `json:"ordinal"`
	ModuleOrdinal int    `json:"module_ordinal"`
	WorkItemID    string `json:"work_item_id"`
}

type PlanDeliverySelectionView struct {
	ID               string                          `json:"id"`
	ProposalID       string                          `json:"proposal_id"`
	DirectionOrdinal int                             `json:"direction_ordinal"`
	NoteID           string                          `json:"note_id"`
	Items            []PlanDeliverySelectionItemView `json:"items"`
	Version          int64                           `json:"version"`
	CreatedAt        time.Time                       `json:"created_at"`
}

type DeliveryCheckpointView struct {
	ID               string    `json:"id"`
	WorkItemID       string    `json:"work_item_id"`
	ModuleOrdinal    int       `json:"module_ordinal"`
	ModuleCount      int       `json:"module_count"`
	ModeRevision     int64     `json:"mode_revision"`
	WorkItemVersion  int64     `json:"work_item_version"`
	FullGateRequired bool      `json:"full_gate_required"`
	HandoffNoteID    string    `json:"handoff_note_id"`
	GateReady        bool      `json:"gate_ready"`
	CreatedAt        time.Time `json:"created_at"`
}

type PlanDeliveryStateView struct {
	Proposal             *PlanDeliveryProposalView  `json:"proposal,omitempty"`
	Selection            *PlanDeliverySelectionView `json:"selection,omitempty"`
	OperatorChoiceNeeded bool                       `json:"operator_choice_needed"`
	PhaseChangeNeeded    bool                       `json:"phase_change_needed"`
	CapabilityGrant      bool                       `json:"capability_grant"`
	DeliveryGateEnforced bool                       `json:"delivery_gate_enforced"`
	RequiredCheckpoints  int                        `json:"required_checkpoints"`
	ReadyCheckpoints     int                        `json:"ready_checkpoints"`
	Checkpoints          []DeliveryCheckpointView   `json:"checkpoints"`
}

type RunDetailView struct {
	Run              RunView                      `json:"run"`
	Mission          MissionView                  `json:"mission"`
	Mode             RunModeView                  `json:"mode"`
	ExecutionProfile RunExecutionProfileView      `json:"execution_profile"`
	Checkpoint       *SupervisorCheckpointView    `json:"checkpoint,omitempty"`
	Lease            *RunExecutionLeaseView       `json:"execution_lease,omitempty"`
	Steering         OperatorSteeringQueueView    `json:"operator_steering"`
	ToolUsage        ToolUsageView                `json:"tool_usage"`
	PlanDelivery     *PlanDeliveryStateView       `json:"plan_delivery,omitempty"`
	ExternalSkills   *ExternalSkillProjectionView `json:"external_skills,omitempty"`
}

type SessionView struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	Title       string    `json:"title"`
	Route       string    `json:"route"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SessionDetailView struct {
	Session SessionView `json:"session"`
	Run     *RunView    `json:"run,omitempty"`
}

type MessageView struct {
	ID                    int64     `json:"id"`
	SessionID             string    `json:"session_id"`
	Role                  string    `json:"role"`
	Content               string    `json:"content"`
	ProvenanceVersion     string    `json:"provenance_version"`
	SourceKind            string    `json:"source_kind"`
	SourceRef             string    `json:"source_ref,omitempty"`
	ContentSHA256         string    `json:"content_sha256"`
	InstructionAuthorized bool      `json:"instruction_authorized"`
	TokenEstimate         int       `json:"token_estimate"`
	Compacted             bool      `json:"compacted"`
	CreatedAt             time.Time `json:"created_at"`
}

type EventView struct {
	EventID   string          `json:"event_id"`
	Version   string          `json:"version"`
	RunID     string          `json:"run_id"`
	MissionID string          `json:"mission_id"`
	Sequence  int64           `json:"sequence"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	SubjectID string          `json:"subject_id,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

type WorkItemView struct {
	ID                 string     `json:"id"`
	RunID              string     `json:"run_id"`
	Title              string     `json:"title"`
	Description        string     `json:"description,omitempty"`
	Status             string     `json:"status"`
	Priority           string     `json:"priority"`
	Owner              string     `json:"owner,omitempty"`
	OwnerAgentID       string     `json:"owner_agent_id,omitempty"`
	AcceptanceCriteria []string   `json:"acceptance_criteria"`
	Dependencies       []string   `json:"dependencies"`
	BlockedReason      string     `json:"blocked_reason,omitempty"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

type NoteView struct {
	ID           string     `json:"id"`
	RunID        string     `json:"run_id"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	Category     string     `json:"category"`
	Visibility   string     `json:"visibility"`
	Owner        string     `json:"owner,omitempty"`
	OwnerAgentID string     `json:"owner_agent_id,omitempty"`
	Tags         []string   `json:"tags"`
	SourceRefs   []string   `json:"source_refs"`
	EvidenceIDs  []string   `json:"evidence_ids"`
	Status       string     `json:"status"`
	Pinned       bool       `json:"pinned"`
	Version      int64      `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ArchivedAt   *time.Time `json:"archived_at,omitempty"`
}

type ArtifactView struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id"`
	SessionID   string    `json:"session_id"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	SourceID    string    `json:"source_id"`
	ToolName    string    `json:"tool_name"`
	Stream      string    `json:"stream"`
	Kind        string    `json:"kind"`
	MIME        string    `json:"mime"`
	Encoding    string    `json:"encoding"`
	SHA256      string    `json:"sha256"`
	SizeBytes   int64     `json:"size_bytes"`
	Redacted    bool      `json:"redacted"`
	CreatedAt   time.Time `json:"created_at"`
}

type SupervisorToolCallView struct {
	Position     int             `json:"position"`
	ModelAttempt int             `json:"model_attempt"`
	CallID       string          `json:"call_id"`
	ToolName     string          `json:"tool_name"`
	Payload      json.RawMessage `json:"payload"`
	Status       string          `json:"status"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorCode    string          `json:"error_code,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

type SupervisorToolRoundView struct {
	RunID        string                   `json:"run_id"`
	Turn         int                      `json:"turn"`
	AttemptID    string                   `json:"attempt_id"`
	Round        int                      `json:"round"`
	ModelAttempt int                      `json:"model_attempt"`
	Calls        []SupervisorToolCallView `json:"calls"`
	CreatedAt    time.Time                `json:"created_at"`
	CompletedAt  *time.Time               `json:"completed_at,omitempty"`
}

func missionView(value domain.Mission) MissionView {
	return MissionView{
		ID: value.ID, Goal: value.Goal, Profile: string(value.Profile), WorkspaceID: value.WorkspaceID,
		Scope: ScopeView{WorkspaceID: value.Scope.WorkspaceID, NetworkMode: value.Scope.NetworkMode,
			AllowedTargets: append([]string{}, value.Scope.AllowedTargets...)},
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func runView(value domain.Run) RunView {
	return RunView{
		ID: value.ID, MissionID: value.MissionID, SessionID: value.SessionID, Status: string(value.Status),
		Config: RunConfigView{ModelRoute: value.Config.ModelRoute, Interactive: value.Config.Interactive},
		Budget: BudgetView{MaxTurns: value.Budget.MaxTurns, MaxTokens: value.Budget.MaxTokens,
			MaxToolCalls: value.Budget.MaxToolCalls, MaxCostUSD: value.Budget.MaxCostUSD,
			TimeoutSeconds: value.Budget.TimeoutSeconds},
		StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func runModeView(value domain.RunModeSnapshot) RunModeView {
	return RunModeView{
		ProtocolVersion: value.ProtocolVersion, Revision: value.Revision,
		Surface: string(value.Surface), Phase: string(value.Phase),
		Profile: string(value.Profile),
		Scope: ScopeView{WorkspaceID: value.Scope.WorkspaceID,
			NetworkMode:    value.Scope.NetworkMode,
			AllowedTargets: append([]string{}, value.Scope.AllowedTargets...)},
		PolicyVersion: value.PolicyVersion, RequestedBy: value.RequestedBy,
		Reason: value.Reason, CreatedAt: value.CreatedAt, CapabilityGrant: false,
	}
}

func runExecutionProfileView(
	value domain.RunExecutionProfileSnapshot,
) RunExecutionProfileView {
	return RunExecutionProfileView{
		ProtocolVersion: value.ProtocolVersion, Revision: value.Revision,
		Profile: string(value.Profile), Backend: string(value.Backend),
		ApprovalPolicy:  string(value.ApprovalPolicy),
		FilesystemScope: string(value.FilesystemScope),
		NetworkScope:    string(value.NetworkScope), RiskTier: string(value.RiskTier),
		RequiredGate: string(value.RequiredGate), PolicyVersion: value.PolicyVersion,
		CreatedAt:           value.CreatedAt,
		ProcessEnabled:      value.ProcessEnabled,
		ExecutionAuthorized: value.ExecutionAuthorized,
		CapabilityGrant:     value.CapabilityGrant,
	}
}

func checkpointView(value domain.SupervisorCheckpoint) SupervisorCheckpointView {
	return SupervisorCheckpointView{
		RunID: value.RunID, NextTurn: value.NextTurn, Phase: string(value.Phase), AttemptID: value.AttemptID,
		RepairPhase: string(value.RepairPhase), RepairReason: value.RepairReason, LastError: value.LastError,
		InputTokens: value.InputTokens, OutputTokens: value.OutputTokens, TotalTokens: value.TotalTokens,
		ExecutionMillis: value.ExecutionMillis, UpdatedAt: value.UpdatedAt,
	}
}

func toolUsageView(value toolbudget.Usage) ToolUsageView {
	return ToolUsageView{Consumed: value.Consumed, Limit: value.Limit,
		Remaining: value.Remaining, ExhaustedAt: value.ExhaustedAt}
}

func runExecutionLeaseView(value domain.RunExecutionLease, now time.Time) RunExecutionLeaseView {
	return RunExecutionLeaseView{
		OwnerID: value.OwnerID, Generation: value.Generation, Status: string(value.Status),
		Active: value.ActiveAt(now), AcquiredAt: value.AcquiredAt, RenewedAt: value.RenewedAt,
		ExpiresAt: value.ExpiresAt, ReleasedAt: value.ReleasedAt,
	}
}

func operatorSteeringQueueView(summary domain.OperatorSteeringQueueSummary,
	values []domain.OperatorSteeringMessage,
) OperatorSteeringQueueView {
	messages := make([]OperatorSteeringMessageView, len(values))
	for index, value := range values {
		messages[index] = operatorSteeringMessageView(value)
	}
	return OperatorSteeringQueueView{
		Pending: summary.Pending, Prepared: summary.Prepared,
		Committed: summary.Committed, Cancelled: summary.Cancelled,
		Messages: messages,
	}
}

func operatorSteeringMessageView(value domain.OperatorSteeringMessage) OperatorSteeringMessageView {
	return OperatorSteeringMessageView{
		ID: value.ID, Sequence: value.Sequence, Status: string(value.Status),
		Prepared:  value.Prepared,
		CreatedAt: value.CreatedAt, CommittedAt: value.CommittedAt,
		CancelledAt: value.CancelledAt,
	}
}

func planDeliveryProposalView(value domain.PlanDeliveryProposal) PlanDeliveryProposalView {
	directions := make([]PlanDeliveryDirectionView, len(value.Spec.Directions))
	for directionIndex, direction := range value.Spec.Directions {
		modules := make([]PlanDeliveryModuleView, len(direction.Modules))
		for moduleIndex, module := range direction.Modules {
			modules[moduleIndex] = PlanDeliveryModuleView{
				Ordinal: module.Ordinal, Title: module.Title, Objective: module.Objective,
				AcceptanceCriteria: append([]string{}, module.AcceptanceCriteria...),
				Dependencies:       append([]int{}, module.Dependencies...),
			}
		}
		directions[directionIndex] = PlanDeliveryDirectionView{
			Ordinal: direction.Ordinal, Title: direction.Title, Summary: direction.Summary,
			Tradeoffs: append([]string{}, direction.Tradeoffs...), Modules: modules,
		}
	}
	return PlanDeliveryProposalView{
		ID: value.ID, ProtocolVersion: value.Spec.Version, Status: string(value.Status),
		ModeRevision: value.ModeRevision, Directions: directions, Version: value.Version,
		CreatedAt: value.CreatedAt,
	}
}

func planDeliverySelectionView(value domain.PlanDeliverySelection) PlanDeliverySelectionView {
	items := make([]PlanDeliverySelectionItemView, len(value.Items))
	for index, item := range value.Items {
		items[index] = PlanDeliverySelectionItemView{
			Ordinal: item.Ordinal, ModuleOrdinal: item.ModuleOrdinal, WorkItemID: item.WorkItemID,
		}
	}
	return PlanDeliverySelectionView{
		ID: value.ID, ProposalID: value.ProposalID, DirectionOrdinal: value.DirectionOrdinal,
		NoteID: value.NoteID, Items: items, Version: value.Version, CreatedAt: value.CreatedAt,
	}
}

func deliveryCheckpointView(value domain.DeliveryCheckpoint,
	ready bool,
) DeliveryCheckpointView {
	return DeliveryCheckpointView{
		ID: value.ID, WorkItemID: value.WorkItemID,
		ModuleOrdinal: value.ModuleOrdinal, ModuleCount: value.ModuleCount,
		ModeRevision: value.ModeRevision, WorkItemVersion: value.WorkItemVersion,
		FullGateRequired: value.FullGateRequired, HandoffNoteID: value.HandoffNoteID,
		GateReady: ready, CreatedAt: value.CreatedAt,
	}
}

func sessionView(value session.Session) SessionView {
	return SessionView{ID: value.ID, WorkspaceID: value.WorkspaceID, Title: value.Title, Route: value.Route,
		Status: value.Status, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt}
}

func messageView(value session.Message) MessageView {
	return MessageView{ID: value.ID, SessionID: value.SessionID, Role: value.Role, Content: value.Content,
		ProvenanceVersion: value.Provenance.Version, SourceKind: value.Provenance.SourceKind,
		SourceRef: value.Provenance.SourceRef, ContentSHA256: value.Provenance.ContentSHA256,
		InstructionAuthorized: value.Provenance.InstructionAuthorized,
		TokenEstimate:         value.TokenEstimate, Compacted: value.Compacted, CreatedAt: value.CreatedAt}
}

func eventView(value events.Event) EventView {
	return EventView{EventID: value.EventID, Version: value.Version, RunID: value.RunID,
		MissionID: value.MissionID, Sequence: value.Sequence, Type: value.Type, Source: value.Source,
		SubjectID: value.SubjectID, Payload: json.RawMessage(value.PayloadJSON), CreatedAt: value.CreatedAt}
}

func workItemView(value domain.WorkItem) WorkItemView {
	return WorkItemView{ID: value.ID, RunID: value.RunID, Title: value.Title, Description: value.Description,
		Status: string(value.Status), Priority: string(value.Priority), Owner: value.Owner,
		OwnerAgentID:       value.OwnerAgentID,
		AcceptanceCriteria: append([]string{}, value.AcceptanceCriteria...),
		Dependencies:       append([]string{}, value.Dependencies...), BlockedReason: value.BlockedReason,
		Version: value.Version, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
		CompletedAt: value.CompletedAt}
}

func noteView(value domain.Note) NoteView {
	return NoteView{ID: value.ID, RunID: value.RunID, Title: value.Title, Content: value.Content,
		Category: string(value.Category), Visibility: string(value.Visibility), Owner: value.Owner,
		OwnerAgentID: value.OwnerAgentID,
		Tags:         append([]string{}, value.Tags...), SourceRefs: append([]string{}, value.SourceRefs...),
		EvidenceIDs: append([]string{}, value.EvidenceIDs...), Status: string(value.Status),
		Pinned: value.Pinned, Version: value.Version, CreatedAt: value.CreatedAt,
		UpdatedAt: value.UpdatedAt, ArchivedAt: value.ArchivedAt}
}

func artifactView(value artifact.Descriptor) ArtifactView {
	return ArtifactView{ID: value.ID, RunID: value.RunID, SessionID: value.SessionID,
		WorkspaceID: value.WorkspaceID, SourceID: value.SourceID, ToolName: value.ToolName,
		Stream: string(value.Stream), Kind: value.Kind, MIME: value.MIME, Encoding: value.Encoding,
		SHA256: value.SHA256, SizeBytes: value.SizeBytes, Redacted: value.Redacted, CreatedAt: value.CreatedAt}
}

func supervisorToolRoundView(value domain.SupervisorToolRound) SupervisorToolRoundView {
	calls := make([]SupervisorToolCallView, len(value.Calls))
	for index, call := range value.Calls {
		calls[index] = SupervisorToolCallView{
			Position: call.Position, ModelAttempt: call.ModelAttempt, CallID: call.CallID,
			ToolName: call.ToolName, Payload: json.RawMessage(call.PayloadJSON), Status: string(call.Status),
			Result: json.RawMessage(call.ResultJSON), ErrorCode: call.ErrorCode,
			CreatedAt: call.CreatedAt, CompletedAt: call.CompletedAt,
		}
	}
	return SupervisorToolRoundView{RunID: value.RunID, Turn: value.Turn, AttemptID: value.AttemptID,
		Round: value.Round, ModelAttempt: value.ModelAttempt, Calls: calls,
		CreatedAt: value.CreatedAt, CompletedAt: value.CompletedAt}
}
