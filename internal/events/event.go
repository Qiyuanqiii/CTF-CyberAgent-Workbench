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
	RunCreatedEvent                               = "run.created"
	RunModeSelectedEvent                          = "run.mode_selected"
	RunPhaseChangedEvent                          = "run.phase_changed"
	RunExecutionProfileSelectedEvent              = "run.execution_profile_selected"
	RunStatusChangedEvent                         = "run.status_changed"
	RunExecutionHandoffRequestedEvent             = "run.execution_handoff_requested"
	RunExecutionHandoffCompletedEvent             = "run.execution_handoff_completed"
	RunWakeScheduledEvent                         = "run.wake_scheduled"
	RunWakeClaimedEvent                           = "run.wake_claimed"
	RunWakeRetriedEvent                           = "run.wake_retried"
	RunWakeExhaustedEvent                         = "run.wake_exhausted"
	RunWakeCancelledEvent                         = "run.wake_cancelled"
	RunWakeHandoffPreparedEvent                   = "run.wake_handoff_prepared"
	RunWakeCompletedEvent                         = "run.wake_completed"
	FileEditApplyRequestedEvent                   = "file_edit.apply_requested"
	FileEditApplyCompletedEvent                   = "file_edit.apply_completed"
	RunExecutionLeaseAcquiredEvent                = "run.execution_lease_acquired"
	RunExecutionLeaseTakenOverEvent               = "run.execution_lease_taken_over"
	RunExecutionLeaseReleasedEvent                = "run.execution_lease_released"
	SessionAttachedEvent                          = "session.attached"
	SessionMessageEvent                           = "session.message_created"
	SessionEvidenceAttachedEvent                  = "session.evidence_attached"
	PolicyDecisionEvent                           = "policy.decision"
	ToolProposedEvent                             = "tool.proposed"
	ToolApprovedEvent                             = "tool.approved"
	ToolDeniedEvent                               = "tool.denied"
	ToolStartedEvent                              = "tool.started"
	ToolCompletedEvent                            = "tool.completed"
	ToolFailedEvent                               = "tool.failed"
	FileEditProposedEvent                         = "file_edit.proposed"
	FileEditApprovedEvent                         = "file_edit.approved"
	FileEditAppliedEvent                          = "file_edit.applied"
	FileEditDeniedEvent                           = "file_edit.denied"
	FileEditFailedEvent                           = "file_edit.failed"
	ApprovalRequestedEvent                        = "approval.requested"
	ApprovalBoundEvent                            = "approval.bound"
	ApprovalDecidedEvent                          = "approval.decided"
	ApprovalGrantCreatedEvent                     = "approval.grant_created"
	ApprovalGrantRevokedEvent                     = "approval.grant_revoked"
	ToolBudgetChargedEvent                        = "tool.budget_charged"
	ToolBudgetExhaustedEvent                      = "tool.budget_exhausted"
	ArtifactCreatedEvent                          = "artifact.created"
	LegacyTaskAdaptedEvent                        = "legacy.task_adapted"
	SupervisorCheckpointedEvent                   = "supervisor.checkpointed"
	AgentRegisteredEvent                          = "agent.registered"
	AgentCapacityReservedEvent                    = "agent.capacity_reserved"
	AgentStatusChangedEvent                       = "agent.status_changed"
	AgentMessageSentEvent                         = "agent.message_sent"
	AgentMessageConsumedEvent                     = "agent.message_consumed"
	AgentInboxContextPreparedEvent                = "agent.inbox_context_prepared"
	AgentInboxContextCommittedEvent               = "agent.inbox_context_committed"
	AgentInboxContextSupersededEvent              = "agent.inbox_context_superseded"
	AgentCompletionReportedEvent                  = "agent.completion_reported"
	AgentGraphSnapshottedEvent                    = "agent.graph_snapshotted"
	AgentTurnStartedEvent                         = "agent.turn_started"
	AgentAttemptUsageRecordedEvent                = "agent.usage_recorded"
	AgentTurnCompletedEvent                       = "agent.turn_completed"
	AgentTurnFailedEvent                          = "agent.turn_failed"
	AgentScheduleStartedEvent                     = "agent.schedule_started"
	AgentScheduleStoppedEvent                     = "agent.schedule_stopped"
	AgentOperatorScheduleRequestedEvent           = "agent.operator_schedule_requested"
	AgentDelegationProposedEvent                  = "agent.delegation_proposed"
	AgentDelegationReviewedEvent                  = "agent.delegation_reviewed"
	AgentDelegationApplicationStartedEvent        = "agent.delegation_application_started"
	AgentDelegationAssignmentAdmittedEvent        = "agent.delegation_assignment_admitted"
	AgentDelegationInstructionDeliveredEvent      = "agent.delegation_instruction_delivered"
	AgentDelegationAppliedEvent                   = "agent.delegation_applied"
	AgentDelegationApplicationAbortedEvent        = "agent.delegation_application_aborted"
	PlanDeliveryProposedEvent                     = "plan_delivery.proposed"
	PlanDeliveryDirectionSelectedEvent            = "plan_delivery.direction_selected"
	DeliveryCheckpointRecordedEvent               = "delivery.checkpoint_recorded"
	OperatorSteeringQueuedEvent                   = "operator.steering_queued"
	OperatorSteeringPreparedEvent                 = "operator.steering_prepared"
	OperatorSteeringCommittedEvent                = "operator.steering_committed"
	OperatorSteeringSupersededEvent               = "operator.steering_superseded"
	OperatorSteeringCancelledEvent                = "operator.steering_cancelled"
	OperatorSteeringActionDeferredEvent           = "operator.steering_action_deferred"
	ReadOnlyFanoutPlannedEvent                    = "readonly_fanout.planned"
	ReadOnlyFanoutExecutionStartedEvent           = "readonly_fanout.execution_started"
	ReadOnlyFanoutExecutionRecoveredEvent         = "readonly_fanout.execution_recovered"
	ReadOnlyFanoutShardStartedEvent               = "readonly_fanout.shard_started"
	ReadOnlyFanoutShardCompletedEvent             = "readonly_fanout.shard_completed"
	ReadOnlyFanoutShardFailedEvent                = "readonly_fanout.shard_failed"
	ReadOnlyFanoutShardCancelledEvent             = "readonly_fanout.shard_cancelled"
	ReadOnlyFanoutExecutionCompletedEvent         = "readonly_fanout.execution_completed"
	ReadOnlyFanoutExecutionFailedEvent            = "readonly_fanout.execution_failed"
	ReadOnlyFanoutExecutionCancelledEvent         = "readonly_fanout.execution_cancelled"
	FindingReportGeneratedEvent                   = "report.generated"
	FindingArtifactEvidenceAttachedEvent          = "finding.evidence_attached"
	FindingValidationDecidedEvent                 = "finding.validation_decided"
	FindingAcceptedEvent                          = "finding.accepted"
	FindingRemediationEvidenceAttachedEvent       = "finding.remediation_evidence_attached"
	FindingFixedEvent                             = "finding.fixed"
	SkillSelectionCreatedEvent                    = "skill.selection_created"
	SkillContextPreparedEvent                     = "skill.context_prepared"
	SkillContextCommittedEvent                    = "skill.context_committed"
	SpecialistSkillContextPreparedEvent           = "skill.specialist_context_prepared"
	SpecialistSkillContextCommittedEvent          = "skill.specialist_context_committed"
	ExternalSkillSelectionCreatedEvent            = "skill.external_selection_created"
	ExternalSkillContextPreparedEvent             = "skill.external_context_prepared"
	ExternalSkillContextCommittedEvent            = "skill.external_context_committed"
	ExternalSpecialistSkillContextPreparedEvent   = "skill.external_specialist_context_prepared"
	ExternalSpecialistSkillContextCommittedEvent  = "skill.external_specialist_context_committed"
	SandboxManifestPreparedEvent                  = "sandbox.manifest_prepared"
	SandboxManifestValidatedEvent                 = "sandbox.manifest_validated"
	SandboxExecutionCandidateValidatedEvent       = "sandbox.execution_candidate_validated"
	SandboxExecutionPreparedEvent                 = "sandbox.execution_prepared"
	SandboxExecutionLeaseAcquiredEvent            = "sandbox.execution_lease_acquired"
	SandboxExecutionLeaseTakenOverEvent           = "sandbox.execution_lease_taken_over"
	SandboxExecutionLeaseReleasedEvent            = "sandbox.execution_lease_released"
	SandboxExecutionCancelRequestedEvent          = "sandbox.execution_cancel_requested"
	SandboxExecutionCleanupCompletedEvent         = "sandbox.cleanup_completed"
	SandboxPreflightRecordedEvent                 = "sandbox.preflight_recorded"
	SandboxBackendEvidenceRecordedEvent           = "sandbox.backend_evidence_recorded"
	SandboxOutputSimulationRecordedEvent          = "sandbox.output_simulation_recorded"
	SandboxDockerObservationRecordedEvent         = "sandbox.docker_observation_recorded"
	SandboxDockerContainerPlanRecordedEvent       = "sandbox.docker_container_plan_recorded"
	SandboxDockerRehearsalRecordedEvent           = "sandbox.docker_container_rehearsal_recorded"
	SandboxDockerAttemptPreparedEvent             = "sandbox.docker_attempt_prepared"
	SandboxDockerAttemptAcquiredEvent             = "sandbox.docker_attempt_acquired"
	SandboxDockerAttemptTakenOverEvent            = "sandbox.docker_attempt_taken_over"
	SandboxDockerAttemptStagedEvent               = "sandbox.docker_attempt_staged"
	SandboxDockerAttemptCleanupEvent              = "sandbox.docker_attempt_cleanup_confirmed"
	SandboxDockerAttemptFailedEvent               = "sandbox.docker_attempt_failed"
	SandboxDockerAttemptCompletedEvent            = "sandbox.docker_attempt_completed"
	SandboxDockerHostInputRequirementEvent        = "sandbox.docker_host_input_requirement_recorded"
	SandboxDockerHostInputIntentEvent             = "sandbox.docker_host_input_intent_prepared"
	SandboxDockerHostInputStagedEvent             = "sandbox.docker_host_input_staged"
	SandboxDockerHostInputHandoffRequirementEvent = "sandbox.docker_host_input_handoff_requirement_recorded"
	SandboxDockerHostInputHandoffIntentEvent      = "sandbox.docker_host_input_handoff_intent_prepared"
	SandboxDockerHostInputHandoffEvent            = "sandbox.docker_host_input_handoff_completed"
	SandboxDockerRuntimeInputProjectionEvent      = "sandbox.docker_runtime_input_projection_planned"
	AgentProtocolRepairRequestedEvent             = "agent.protocol_repair_requested"
	AgentProtocolRepairStartedEvent               = "agent.protocol_repair_started"
	AgentProtocolRepairCompletedEvent             = "agent.protocol_repair_completed"
	AgentProtocolRepairFailedEvent                = "agent.protocol_repair_failed"
	ModelStartedEvent                             = "model.started"
	ModelCancelRequestedEvent                     = "model.cancel_requested"
	ModelCancelObservedEvent                      = "model.cancel_observed"
	ModelDeltaEvent                               = "model.delta"
	ModelCompletedEvent                           = "model.completed"
	ModelFailedEvent                              = "model.failed"
	ProtocolRepairRequestedEvent                  = "supervisor.protocol_repair_requested"
	ProtocolRepairStartedEvent                    = "supervisor.protocol_repair_started"
	ProtocolRepairCompletedEvent                  = "supervisor.protocol_repair_completed"
	ProtocolRepairFailedEvent                     = "supervisor.protocol_repair_failed"
	SupervisorToolBatchEvent                      = "supervisor.tool_batch_requested"
	SupervisorToolResultEvent                     = "supervisor.tool_result_recorded"
	SupervisorToolCompleteEvent                   = "supervisor.tool_batch_completed"
	SupervisorActionEvent                         = "supervisor.action_committed"
	SupervisorRunWaitingEvent                     = "supervisor.run_waiting"
	SupervisorRunCompletedEvent                   = "supervisor.run_completed"
	SupervisorRunFailedEvent                      = "supervisor.run_failed"
	WorkItemCreatedEvent                          = "work_item.created"
	WorkItemChangedEvent                          = "work_item.changed"
	NoteCreatedEvent                              = "note.created"
	NoteChangedEvent                              = "note.changed"
)

const (
	SandboxDockerRuntimeInputApplicationPreparedEvent      = "sandbox.docker_runtime_input_application_prepared"
	SandboxDockerRuntimeInputApplicationAcquiredEvent      = "sandbox.docker_runtime_input_application_acquired"
	SandboxDockerRuntimeInputApplicationTakenOverEvent     = "sandbox.docker_runtime_input_application_taken_over"
	SandboxDockerRuntimeInputApplicationFailedEvent        = "sandbox.docker_runtime_input_application_failed"
	SandboxDockerRuntimeInputApplicationCompletedEvent     = "sandbox.docker_runtime_input_application_completed"
	SandboxDockerRuntimeInputResourceInspectedEvent        = "sandbox.docker_runtime_input_resource_inspected"
	SandboxDockerRuntimeInputResourceCleanupPreparedEvent  = "sandbox.docker_runtime_input_resource_cleanup_prepared"
	SandboxDockerRuntimeInputResourceCleanupAcquiredEvent  = "sandbox.docker_runtime_input_resource_cleanup_acquired"
	SandboxDockerRuntimeInputResourceCleanupTakenOverEvent = "sandbox.docker_runtime_input_resource_cleanup_taken_over"
	SandboxDockerRuntimeInputResourceCleanupFailedEvent    = "sandbox.docker_runtime_input_resource_cleanup_failed"
	SandboxDockerRuntimeInputResourceCleanupCompletedEvent = "sandbox.docker_runtime_input_resource_cleanup_completed"
	SandboxDockerStartGateReviewedEvent                    = "sandbox.docker_start_gate_reviewed"
	SandboxDockerProductionEvidenceCapturedEvent           = "sandbox.docker_production_evidence_captured"
	SandboxDockerProductionEvidenceAttemptPreparedEvent    = "sandbox.docker_production_evidence_attempt_prepared"
	SandboxDockerProductionEvidenceAttemptAcquiredEvent    = "sandbox.docker_production_evidence_attempt_acquired"
	SandboxDockerProductionEvidenceAttemptTakenOverEvent   = "sandbox.docker_production_evidence_attempt_taken_over"
	SandboxDockerProductionEvidenceReconciledEvent         = "sandbox.docker_production_evidence_reconciled"
	SandboxDockerProductionEvidenceAttemptFailedEvent      = "sandbox.docker_production_evidence_attempt_failed"
	SandboxDockerProductionEvidenceAttemptCompletedEvent   = "sandbox.docker_production_evidence_attempt_completed"
	SandboxDockerProductionEvidenceHarnessPreparedEvent    = "sandbox.docker_production_evidence_harness_prepared"
	SandboxDockerProductionEvidenceHarnessReconciledEvent  = "sandbox.docker_production_evidence_harness_reconciled"
	SandboxDockerProductionEvidenceHarnessCompletedEvent   = "sandbox.docker_production_evidence_harness_completed"
	SandboxDockerProductionEvidenceReviewedEvent           = "sandbox.docker_production_evidence_reviewed"
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
