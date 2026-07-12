package events

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"cyberagent-workbench/internal/idgen"
)

const EnvelopeVersion = "v1"

const (
	RunCreatedEvent                 = "run.created"
	RunStatusChangedEvent           = "run.status_changed"
	RunExecutionLeaseAcquiredEvent  = "run.execution_lease_acquired"
	RunExecutionLeaseTakenOverEvent = "run.execution_lease_taken_over"
	RunExecutionLeaseReleasedEvent  = "run.execution_lease_released"
	SessionAttachedEvent            = "session.attached"
	SessionMessageEvent             = "session.message_created"
	PolicyDecisionEvent             = "policy.decision"
	ToolProposedEvent               = "tool.proposed"
	ToolApprovedEvent               = "tool.approved"
	ToolDeniedEvent                 = "tool.denied"
	ToolStartedEvent                = "tool.started"
	ToolCompletedEvent              = "tool.completed"
	ToolFailedEvent                 = "tool.failed"
	FileEditProposedEvent           = "file_edit.proposed"
	FileEditApprovedEvent           = "file_edit.approved"
	FileEditAppliedEvent            = "file_edit.applied"
	FileEditDeniedEvent             = "file_edit.denied"
	FileEditFailedEvent             = "file_edit.failed"
	ApprovalRequestedEvent          = "approval.requested"
	ApprovalBoundEvent              = "approval.bound"
	ApprovalDecidedEvent            = "approval.decided"
	ApprovalGrantCreatedEvent       = "approval.grant_created"
	ApprovalGrantRevokedEvent       = "approval.grant_revoked"
	ToolBudgetChargedEvent          = "tool.budget_charged"
	ToolBudgetExhaustedEvent        = "tool.budget_exhausted"
	ArtifactCreatedEvent            = "artifact.created"
	LegacyTaskAdaptedEvent          = "legacy.task_adapted"
	SupervisorCheckpointedEvent     = "supervisor.checkpointed"
	AgentRegisteredEvent            = "agent.registered"
	AgentCapacityReservedEvent      = "agent.capacity_reserved"
	AgentStatusChangedEvent         = "agent.status_changed"
	AgentMessageSentEvent           = "agent.message_sent"
	AgentMessageConsumedEvent       = "agent.message_consumed"
	AgentCompletionReportedEvent    = "agent.completion_reported"
	AgentGraphSnapshottedEvent      = "agent.graph_snapshotted"
	AgentTurnStartedEvent           = "agent.turn_started"
	AgentTurnCompletedEvent         = "agent.turn_completed"
	AgentTurnFailedEvent            = "agent.turn_failed"
	ModelStartedEvent               = "model.started"
	ModelCancelRequestedEvent       = "model.cancel_requested"
	ModelCancelObservedEvent        = "model.cancel_observed"
	ModelDeltaEvent                 = "model.delta"
	ModelCompletedEvent             = "model.completed"
	ModelFailedEvent                = "model.failed"
	ProtocolRepairRequestedEvent    = "supervisor.protocol_repair_requested"
	ProtocolRepairStartedEvent      = "supervisor.protocol_repair_started"
	ProtocolRepairCompletedEvent    = "supervisor.protocol_repair_completed"
	ProtocolRepairFailedEvent       = "supervisor.protocol_repair_failed"
	SupervisorToolBatchEvent        = "supervisor.tool_batch_requested"
	SupervisorToolResultEvent       = "supervisor.tool_result_recorded"
	SupervisorToolCompleteEvent     = "supervisor.tool_batch_completed"
	SupervisorActionEvent           = "supervisor.action_committed"
	SupervisorRunWaitingEvent       = "supervisor.run_waiting"
	SupervisorRunCompletedEvent     = "supervisor.run_completed"
	SupervisorRunFailedEvent        = "supervisor.run_failed"
	WorkItemCreatedEvent            = "work_item.created"
	WorkItemChangedEvent            = "work_item.changed"
	NoteCreatedEvent                = "note.created"
	NoteChangedEvent                = "note.changed"
)

type Event struct {
	ID          int64
	EventID     string
	Version     string
	RunID       string
	MissionID   string
	Sequence    int64
	Type        string
	Source      string
	SubjectID   string
	PayloadJSON string
	CreatedAt   time.Time
}

func New(runID string, missionID string, eventType string, source string, subjectID string, payload any) (Event, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Event{}, err
	}
	event := Event{
		EventID:     idgen.New("evt"),
		Version:     EnvelopeVersion,
		RunID:       strings.TrimSpace(runID),
		MissionID:   strings.TrimSpace(missionID),
		Type:        strings.TrimSpace(eventType),
		Source:      strings.TrimSpace(source),
		SubjectID:   strings.TrimSpace(subjectID),
		PayloadJSON: string(encoded),
		CreatedAt:   time.Now().UTC(),
	}
	return event, event.Validate()
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.EventID) == "" || strings.TrimSpace(e.RunID) == "" || strings.TrimSpace(e.MissionID) == "" {
		return errors.New("event id, run id, and mission id are required")
	}
	if e.Version != EnvelopeVersion {
		return errors.New("unsupported event envelope version")
	}
	if strings.TrimSpace(e.Type) == "" || strings.TrimSpace(e.Source) == "" {
		return errors.New("event type and source are required")
	}
	if !json.Valid([]byte(e.PayloadJSON)) {
		return errors.New("event payload must be valid JSON")
	}
	if e.CreatedAt.IsZero() {
		return errors.New("event timestamp is required")
	}
	return nil
}
