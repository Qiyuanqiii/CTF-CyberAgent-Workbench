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
	ProtocolVersion           string                           `json:"protocol_version"`
	RunID                     string                           `json:"run_id"`
	MissionID                 string                           `json:"mission_id"`
	SessionID                 string                           `json:"session_id"`
	WorkspaceID               string                           `json:"workspace_id"`
	RunStatus                 string                           `json:"run_status"`
	Surface                   string                           `json:"surface"`
	Phase                     string                           `json:"phase"`
	ModeRevision              int64                            `json:"mode_revision"`
	GeneratedAt               time.Time                        `json:"generated_at"`
	Plan                      CodeHandoffPlanView              `json:"plan"`
	Queue                     CodeHandoffQueueView             `json:"queue"`
	ChangeSet                 CodeHandoffChangeSetView         `json:"change_set"`
	Verification              CodeHandoffVerificationView      `json:"verification"`
	PendingActionCount        int                              `json:"pending_action_count"`
	PendingActionsTruncated   bool                             `json:"pending_actions_truncated"`
	PendingActions            []CodeHandoffActionReferenceView `json:"pending_actions"`
	ReportReferencesTruncated bool                             `json:"report_references_truncated"`
	ReportReferences          []CodeHandoffReportReferenceView `json:"report_references"`
	Regenerable               bool                             `json:"regenerable"`
	DurableSources            bool                             `json:"durable_sources"`
	PrivateBodiesIncluded     bool                             `json:"private_bodies_included"`
	CompositeMutation         bool                             `json:"composite_mutation"`
	ResumeAuthorized          bool                             `json:"resume_authorized"`
	ExecutionStarted          bool                             `json:"execution_started"`
}
