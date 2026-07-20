package httpapi

import (
	"time"

	"cyberagent-workbench/internal/operatoraction"
)

type VerificationEvidenceRequestView struct {
	Version string `json:"version"`
	Outcome string `json:"outcome"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

type VerificationEvidenceItemView struct {
	ProtocolVersion  string    `json:"protocol_version"`
	ID               string    `json:"id"`
	RunID            string    `json:"run_id"`
	SessionID        string    `json:"session_id"`
	WorkspaceID      string    `json:"workspace_id"`
	Outcome          string    `json:"outcome"`
	Title            string    `json:"title"`
	Summary          string    `json:"summary"`
	SummarySHA256    string    `json:"summary_sha256"`
	Redacted         bool      `json:"redacted"`
	RecordedAt       time.Time `json:"recorded_at"`
	Immutable        bool      `json:"immutable"`
	OperatorSupplied bool      `json:"operator_supplied"`
	CommandExecuted  bool      `json:"command_executed"`
	ModelAssertion   bool      `json:"model_assertion"`
	Approval         bool      `json:"approval"`
	AuthorityGranted bool      `json:"authority_granted"`
}

type VerificationEvidenceControlView struct {
	ProtocolVersion  string    `json:"protocol_version"`
	ID               string    `json:"id"`
	RunID            string    `json:"run_id"`
	SessionID        string    `json:"session_id"`
	WorkspaceID      string    `json:"workspace_id"`
	Outcome          string    `json:"outcome"`
	Title            string    `json:"title"`
	Summary          string    `json:"summary"`
	SummarySHA256    string    `json:"summary_sha256"`
	Redacted         bool      `json:"redacted"`
	RecordedAt       time.Time `json:"recorded_at"`
	Immutable        bool      `json:"immutable"`
	OperatorSupplied bool      `json:"operator_supplied"`
	CommandExecuted  bool      `json:"command_executed"`
	ModelAssertion   bool      `json:"model_assertion"`
	Approval         bool      `json:"approval"`
	AuthorityGranted bool      `json:"authority_granted"`
	Replayed         bool      `json:"replayed"`
}

type VerificationEvidenceInventoryView struct {
	ProtocolVersion string                         `json:"protocol_version"`
	RunID           string                         `json:"run_id"`
	SessionID       string                         `json:"session_id"`
	WorkspaceID     string                         `json:"workspace_id"`
	Items           []VerificationEvidenceItemView `json:"items"`
	PassCount       int                            `json:"pass_count"`
	FailCount       int                            `json:"fail_count"`
	UnknownCount    int                            `json:"unknown_count"`
	Truncated       bool                           `json:"truncated"`
}

type VerificationPlanItemRequestView struct {
	Title               string `json:"title"`
	ExpectedObservation string `json:"expected_observation"`
}

type VerificationPlanRequestView struct {
	Version string                            `json:"version"`
	Title   string                            `json:"title"`
	Summary string                            `json:"summary"`
	Items   []VerificationPlanItemRequestView `json:"items"`
}

type VerificationPlanItemView struct {
	Ordinal             int    `json:"ordinal"`
	Title               string `json:"title"`
	ExpectedObservation string `json:"expected_observation"`
	ItemSHA256          string `json:"item_sha256"`
	Redacted            bool   `json:"redacted"`
}

type VerificationPlanView struct {
	ProtocolVersion  string                     `json:"protocol_version"`
	ID               string                     `json:"id"`
	RunID            string                     `json:"run_id"`
	SessionID        string                     `json:"session_id"`
	WorkspaceID      string                     `json:"workspace_id"`
	Title            string                     `json:"title"`
	Summary          string                     `json:"summary"`
	PlanSHA256       string                     `json:"plan_sha256"`
	Redacted         bool                       `json:"redacted"`
	CreatedAt        time.Time                  `json:"created_at"`
	Items            []VerificationPlanItemView `json:"items"`
	ItemCount        int                        `json:"item_count"`
	Immutable        bool                       `json:"immutable"`
	OperatorSupplied bool                       `json:"operator_supplied"`
	GuidanceOnly     bool                       `json:"guidance_only"`
	CommandExecuted  bool                       `json:"command_executed"`
	ModelAssertion   bool                       `json:"model_assertion"`
	ResultInferred   bool                       `json:"result_inferred"`
	Approval         bool                       `json:"approval"`
	AuthorityGranted bool                       `json:"authority_granted"`
}

type VerificationPlanControlView struct {
	ProtocolVersion  string                     `json:"protocol_version"`
	ID               string                     `json:"id"`
	RunID            string                     `json:"run_id"`
	SessionID        string                     `json:"session_id"`
	WorkspaceID      string                     `json:"workspace_id"`
	Title            string                     `json:"title"`
	Summary          string                     `json:"summary"`
	PlanSHA256       string                     `json:"plan_sha256"`
	Redacted         bool                       `json:"redacted"`
	CreatedAt        time.Time                  `json:"created_at"`
	Items            []VerificationPlanItemView `json:"items"`
	ItemCount        int                        `json:"item_count"`
	Immutable        bool                       `json:"immutable"`
	OperatorSupplied bool                       `json:"operator_supplied"`
	GuidanceOnly     bool                       `json:"guidance_only"`
	CommandExecuted  bool                       `json:"command_executed"`
	ModelAssertion   bool                       `json:"model_assertion"`
	ResultInferred   bool                       `json:"result_inferred"`
	Approval         bool                       `json:"approval"`
	AuthorityGranted bool                       `json:"authority_granted"`
	Replayed         bool                       `json:"replayed"`
}

type VerificationPlanInventoryView struct {
	ProtocolVersion string                 `json:"protocol_version"`
	RunID           string                 `json:"run_id"`
	SessionID       string                 `json:"session_id"`
	WorkspaceID     string                 `json:"workspace_id"`
	Items           []VerificationPlanView `json:"items"`
	Truncated       bool                   `json:"truncated"`
}

type VerificationAssociationRequestView struct {
	Version         string `json:"version"`
	PlanID          string `json:"plan_id"`
	PlanItemOrdinal int    `json:"plan_item_ordinal"`
	EvidenceID      string `json:"evidence_id"`
}

type VerificationAssociationControlView struct {
	ProtocolVersion          string    `json:"protocol_version"`
	ID                       string    `json:"id"`
	RunID                    string    `json:"run_id"`
	SessionID                string    `json:"session_id"`
	WorkspaceID              string    `json:"workspace_id"`
	PlanID                   string    `json:"plan_id"`
	PlanItemOrdinal          int       `json:"plan_item_ordinal"`
	PlanItemSHA256           string    `json:"plan_item_sha256"`
	EvidenceID               string    `json:"evidence_id"`
	EvidenceOutcome          string    `json:"evidence_outcome"`
	EvidenceEventSequence    int64     `json:"evidence_event_sequence"`
	AssociationEventSequence int64     `json:"association_event_sequence"`
	AssociatedAt             time.Time `json:"associated_at"`
	Immutable                bool      `json:"immutable"`
	OperatorSupplied         bool      `json:"operator_supplied"`
	MetadataOnly             bool      `json:"metadata_only"`
	CommandExecuted          bool      `json:"command_executed"`
	ModelAssertion           bool      `json:"model_assertion"`
	ResultInferred           bool      `json:"result_inferred"`
	RecordRewritten          bool      `json:"record_rewritten"`
	Approval                 bool      `json:"approval"`
	AuthorityGranted         bool      `json:"authority_granted"`
	Replayed                 bool      `json:"replayed"`
}

type VerificationPlanItemCoverageView struct {
	Ordinal                        int    `json:"ordinal"`
	ItemSHA256                     string `json:"item_sha256"`
	AssociatedEvidenceCount        int    `json:"associated_evidence_count"`
	PassCount                      int    `json:"pass_count"`
	FailCount                      int    `json:"fail_count"`
	UnknownCount                   int    `json:"unknown_count"`
	LatestAssociationEventSequence int64  `json:"latest_association_event_sequence"`
}

type VerificationPlanCoverageView struct {
	PlanID                  string                             `json:"plan_id"`
	PlanSHA256              string                             `json:"plan_sha256"`
	ItemCount               int                                `json:"item_count"`
	ObservedItemCount       int                                `json:"observed_item_count"`
	AssociatedEvidenceCount int                                `json:"associated_evidence_count"`
	Items                   []VerificationPlanItemCoverageView `json:"items"`
}

type VerificationAssociationReferenceView struct {
	ID                       string    `json:"id"`
	PlanID                   string    `json:"plan_id"`
	PlanItemOrdinal          int       `json:"plan_item_ordinal"`
	PlanItemSHA256           string    `json:"plan_item_sha256"`
	EvidenceID               string    `json:"evidence_id"`
	EvidenceOutcome          string    `json:"evidence_outcome"`
	EvidenceEventSequence    int64     `json:"evidence_event_sequence"`
	AssociationEventSequence int64     `json:"association_event_sequence"`
	AssociatedAt             time.Time `json:"associated_at"`
}

type VerificationPlanCoverageInventoryView struct {
	ProtocolVersion         string                                 `json:"protocol_version"`
	RunID                   string                                 `json:"run_id"`
	SessionID               string                                 `json:"session_id"`
	WorkspaceID             string                                 `json:"workspace_id"`
	Plans                   []VerificationPlanCoverageView         `json:"plans"`
	PlanCount               int                                    `json:"plan_count"`
	PlanItemCount           int                                    `json:"plan_item_count"`
	ObservedPlanItemCount   int                                    `json:"observed_plan_item_count"`
	AssociatedEvidenceCount int                                    `json:"associated_evidence_count"`
	Associations            []VerificationAssociationReferenceView `json:"associations"`
	PlansTruncated          bool                                   `json:"plans_truncated"`
	AssociationsTruncated   bool                                   `json:"associations_truncated"`
	MetadataOnly            bool                                   `json:"metadata_only"`
	ReadOnly                bool                                   `json:"read_only"`
	ResultInferred          bool                                   `json:"result_inferred"`
	CommandExecuted         bool                                   `json:"command_executed"`
	ModelAssertion          bool                                   `json:"model_assertion"`
	RecordRewritten         bool                                   `json:"record_rewritten"`
	Approval                bool                                   `json:"approval"`
	AuthorityGranted        bool                                   `json:"authority_granted"`
}

type VerificationPlanItemCoverageDetailView struct {
	ProtocolVersion                string                                 `json:"protocol_version"`
	RunID                          string                                 `json:"run_id"`
	SessionID                      string                                 `json:"session_id"`
	WorkspaceID                    string                                 `json:"workspace_id"`
	PlanID                         string                                 `json:"plan_id"`
	PlanSHA256                     string                                 `json:"plan_sha256"`
	PlanItemOrdinal                int                                    `json:"plan_item_ordinal"`
	PlanItemSHA256                 string                                 `json:"plan_item_sha256"`
	AssociatedEvidenceCount        int                                    `json:"associated_evidence_count"`
	PassCount                      int                                    `json:"pass_count"`
	FailCount                      int                                    `json:"fail_count"`
	UnknownCount                   int                                    `json:"unknown_count"`
	LatestAssociationEventSequence int64                                  `json:"latest_association_event_sequence"`
	Associations                   []VerificationAssociationReferenceView `json:"associations"`
	AssociationsTruncated          bool                                   `json:"associations_truncated"`
	MetadataOnly                   bool                                   `json:"metadata_only"`
	ReadOnly                       bool                                   `json:"read_only"`
	PrivatePlanBodyIncluded        bool                                   `json:"private_plan_body_included"`
	PrivateEvidenceBodiesIncluded  bool                                   `json:"private_evidence_bodies_included"`
	OperatorIdentityIncluded       bool                                   `json:"operator_identity_included"`
	ResultInferred                 bool                                   `json:"result_inferred"`
	CommandExecuted                bool                                   `json:"command_executed"`
	ModelAssertion                 bool                                   `json:"model_assertion"`
	RecordRewritten                bool                                   `json:"record_rewritten"`
	Approval                       bool                                   `json:"approval"`
	AuthorityGranted               bool                                   `json:"authority_granted"`
}

type VerificationSnapshotExportView struct {
	ProtocolVersion                string `json:"protocol_version"`
	SnapshotProtocolVersion        string `json:"snapshot_protocol_version"`
	Format                         string `json:"format"`
	Filename                       string `json:"filename"`
	MIMEType                       string `json:"mime_type"`
	RunID                          string `json:"run_id"`
	SessionID                      string `json:"session_id"`
	WorkspaceID                    string `json:"workspace_id"`
	PlanID                         string `json:"plan_id"`
	PlanSHA256                     string `json:"plan_sha256"`
	PlanItemOrdinal                int    `json:"plan_item_ordinal"`
	PlanItemSHA256                 string `json:"plan_item_sha256"`
	SnapshotHighWaterEventSequence int64  `json:"snapshot_high_water_event_sequence"`
	AssociatedEvidenceCount        int    `json:"associated_evidence_count"`
	PassCount                      int    `json:"pass_count"`
	FailCount                      int    `json:"fail_count"`
	UnknownCount                   int    `json:"unknown_count"`
	ReturnedAssociationCount       int    `json:"returned_association_count"`
	AssociationsTruncated          bool   `json:"associations_truncated"`
	ContentSHA256                  string `json:"content_sha256"`
	ContentBytes                   int    `json:"content_bytes"`
	Content                        string `json:"content"`
	MetadataOnly                   bool   `json:"metadata_only"`
	ReadOnly                       bool   `json:"read_only"`
	DownloadOnly                   bool   `json:"download_only"`
	PrivatePlanBodyIncluded        bool   `json:"private_plan_body_included"`
	PrivateEvidenceBodiesIncluded  bool   `json:"private_evidence_bodies_included"`
	OperatorIdentityIncluded       bool   `json:"operator_identity_included"`
	ResultInferred                 bool   `json:"result_inferred"`
	CommandExecuted                bool   `json:"command_executed"`
	ModelAssertion                 bool   `json:"model_assertion"`
	RecordRewritten                bool   `json:"record_rewritten"`
	Approval                       bool   `json:"approval"`
	AuthorityGranted               bool   `json:"authority_granted"`
	MutationSupported              bool   `json:"mutation_supported"`
	ExecutionStarted               bool   `json:"execution_started"`
}

type CodeHandoffPlanView struct {
	State             string `json:"state"`
	ProposalID        string `json:"proposal_id"`
	SelectionID       string `json:"selection_id"`
	DirectionCount    int    `json:"direction_count"`
	SelectedDirection int    `json:"selected_direction"`
	ModuleCount       int    `json:"module_count"`
	PendingCount      int    `json:"pending_count"`
	InProgressCount   int    `json:"in_progress_count"`
	BlockedCount      int    `json:"blocked_count"`
	CompletedCount    int    `json:"completed_count"`
	CancelledCount    int    `json:"cancelled_count"`
}

type CodeHandoffQueueView struct {
	Pending   int `json:"pending"`
	Prepared  int `json:"prepared"`
	Committed int `json:"committed"`
	Cancelled int `json:"cancelled"`
}

type CodeHandoffChangeSetView struct {
	Proposed       int  `json:"proposed"`
	Approved       int  `json:"approved"`
	Applied        int  `json:"applied"`
	Denied         int  `json:"denied"`
	Failed         int  `json:"failed"`
	ReturnedCount  int  `json:"returned_count"`
	TotalDiffBytes int  `json:"total_diff_bytes"`
	Truncated      bool `json:"truncated"`
}

type CodeHandoffVerificationReferenceView struct {
	ID         string    `json:"id"`
	Outcome    string    `json:"outcome"`
	Redacted   bool      `json:"redacted"`
	RecordedAt time.Time `json:"recorded_at"`
}

type CodeHandoffVerificationView struct {
	PassCount     int                                    `json:"pass_count"`
	FailCount     int                                    `json:"fail_count"`
	UnknownCount  int                                    `json:"unknown_count"`
	ReturnedCount int                                    `json:"returned_count"`
	Truncated     bool                                   `json:"truncated"`
	References    []CodeHandoffVerificationReferenceView `json:"references"`
}

type CodeHandoffVerificationPlanReferenceView struct {
	ID         string    `json:"id"`
	PlanSHA256 string    `json:"plan_sha256"`
	ItemCount  int       `json:"item_count"`
	Redacted   bool      `json:"redacted"`
	CreatedAt  time.Time `json:"created_at"`
}

type CodeHandoffVerificationPlansView struct {
	ReturnedCount int                                        `json:"returned_count"`
	Truncated     bool                                       `json:"truncated"`
	References    []CodeHandoffVerificationPlanReferenceView `json:"references"`
}

type CodeHandoffVerificationCoverageItemView struct {
	PlanID                         string `json:"plan_id"`
	PlanSHA256                     string `json:"plan_sha256"`
	Ordinal                        int    `json:"ordinal"`
	ItemSHA256                     string `json:"item_sha256"`
	AssociatedEvidenceCount        int    `json:"associated_evidence_count"`
	PassCount                      int    `json:"pass_count"`
	FailCount                      int    `json:"fail_count"`
	UnknownCount                   int    `json:"unknown_count"`
	LatestAssociationEventSequence int64  `json:"latest_association_event_sequence"`
}

type CodeHandoffVerificationCoverageView struct {
	ProtocolVersion         string                                    `json:"protocol_version"`
	PlanCount               int                                       `json:"plan_count"`
	PlanItemCount           int                                       `json:"plan_item_count"`
	ObservedPlanItemCount   int                                       `json:"observed_plan_item_count"`
	UnobservedPlanItemCount int                                       `json:"unobserved_plan_item_count"`
	AssociatedEvidenceCount int                                       `json:"associated_evidence_count"`
	ContradictoryItemCount  int                                       `json:"contradictory_item_count"`
	ReturnedItemCount       int                                       `json:"returned_item_count"`
	Truncated               bool                                      `json:"truncated"`
	Items                   []CodeHandoffVerificationCoverageItemView `json:"items"`
	MetadataOnly            bool                                      `json:"metadata_only"`
	ReadOnly                bool                                      `json:"read_only"`
	ResultInferred          bool                                      `json:"result_inferred"`
	PrivateBodiesIncluded   bool                                      `json:"private_bodies_included"`
}

type CodeHandoffActionReferenceView struct {
	ID          string                     `json:"id"`
	Kind        operatoraction.Kind        `json:"kind"`
	State       string                     `json:"state"`
	Destination operatoraction.Destination `json:"destination"`
	AvailableAt time.Time                  `json:"available_at"`
	DueAt       *time.Time                 `json:"due_at,omitempty"`
}

type CodeHandoffReportReferenceView struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	FindingCount int       `json:"finding_count"`
	CreatedAt    time.Time `json:"created_at"`
}

type CodeHandoffView struct {
	ProtocolVersion           string                              `json:"protocol_version"`
	RunID                     string                              `json:"run_id"`
	MissionID                 string                              `json:"mission_id"`
	SessionID                 string                              `json:"session_id"`
	WorkspaceID               string                              `json:"workspace_id"`
	RunStatus                 string                              `json:"run_status"`
	Surface                   string                              `json:"surface"`
	Phase                     string                              `json:"phase"`
	ModeRevision              int64                               `json:"mode_revision"`
	SourceEventSequence       int64                               `json:"source_event_sequence"`
	GeneratedAt               time.Time                           `json:"generated_at"`
	Plan                      CodeHandoffPlanView                 `json:"plan"`
	Queue                     CodeHandoffQueueView                `json:"queue"`
	ChangeSet                 CodeHandoffChangeSetView            `json:"change_set"`
	Verification              CodeHandoffVerificationView         `json:"verification"`
	VerificationPlans         CodeHandoffVerificationPlansView    `json:"verification_plans"`
	VerificationCoverage      CodeHandoffVerificationCoverageView `json:"verification_coverage"`
	PendingActionCount        int                                 `json:"pending_action_count"`
	PendingActionsTruncated   bool                                `json:"pending_actions_truncated"`
	PendingActions            []CodeHandoffActionReferenceView    `json:"pending_actions"`
	ReportReferencesTruncated bool                                `json:"report_references_truncated"`
	ReportReferences          []CodeHandoffReportReferenceView    `json:"report_references"`
	Regenerable               bool                                `json:"regenerable"`
	DurableSources            bool                                `json:"durable_sources"`
	PrivateBodiesIncluded     bool                                `json:"private_bodies_included"`
	CompositeMutation         bool                                `json:"composite_mutation"`
	ResumeAuthorized          bool                                `json:"resume_authorized"`
	ExecutionStarted          bool                                `json:"execution_started"`
}

type CodeHandoffExportView struct {
	ProtocolVersion     string    `json:"protocol_version"`
	Format              string    `json:"format"`
	Filename            string    `json:"filename"`
	MIMEType            string    `json:"mime_type"`
	RunID               string    `json:"run_id"`
	SourceEventSequence int64     `json:"source_event_sequence"`
	GeneratedAt         time.Time `json:"generated_at"`
	ContentSHA256       string    `json:"content_sha256"`
	ContentBytes        int       `json:"content_bytes"`
	Content             string    `json:"content"`
	ReadOnly            bool      `json:"read_only"`
	DownloadOnly        bool      `json:"download_only"`
	PrivateBodies       bool      `json:"private_bodies"`
	ResumeAuthorized    bool      `json:"resume_authorized"`
	MutationSupported   bool      `json:"mutation_supported"`
	ReportAcceptance    bool      `json:"report_acceptance"`
	ExecutionStarted    bool      `json:"execution_started"`
}
