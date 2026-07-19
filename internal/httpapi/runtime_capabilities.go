package httpapi

import (
	"net/http"

	"cyberagent-workbench/internal/apperror"
	"cyberagent-workbench/internal/application"
)

const RuntimeCapabilitiesProtocolVersion = "runtime_capabilities.v1"

type RunWakeWorkerHealthSource interface {
	Health() application.RunWakeWorkerHealth
}

type RunWakeWorkerHealthView struct {
	ProtocolVersion        string `json:"protocol_version"`
	Enabled                bool   `json:"enabled"`
	State                  string `json:"state"`
	Active                 bool   `json:"active"`
	PollIntervalMillis     int64  `json:"poll_interval_ms"`
	Concurrency            int    `json:"concurrency"`
	MaxSteps               int    `json:"max_steps"`
	RuntimeEnableSupported bool   `json:"runtime_enable_supported"`
	PersistentService      bool   `json:"persistent_service"`
}

type RuntimeCapabilitiesView struct {
	ProtocolVersion               string                  `json:"protocol_version"`
	RunControlEnabled             bool                    `json:"run_control_enabled"`
	RunCreationEnabled            bool                    `json:"run_creation_enabled"`
	SessionMessageEnabled         bool                    `json:"session_message_enabled"`
	SessionSteeringControlEnabled bool                    `json:"session_steering_control_enabled"`
	RunLifecycleEnabled           bool                    `json:"run_lifecycle_enabled"`
	RunExecutionEnabled           bool                    `json:"run_execution_enabled"`
	PlanDeliveryControlEnabled    bool                    `json:"plan_delivery_control_enabled"`
	ApprovalControlEnabled        bool                    `json:"approval_control_enabled"`
	ModelControlEnabled           bool                    `json:"model_control_enabled"`
	ProviderCredentialEnabled     bool                    `json:"provider_credential_enabled"`
	FileEditReviewEnabled         bool                    `json:"file_edit_review_enabled"`
	FileEditProposalEnabled       bool                    `json:"file_edit_proposal_enabled"`
	FileEditApplyEnabled          bool                    `json:"file_edit_apply_enabled"`
	RunWakeControlEnabled         bool                    `json:"run_wake_control_enabled"`
	RunWakeExecutionEnabled       bool                    `json:"run_wake_execution_enabled"`
	RunWakeWorkerEnabled          bool                    `json:"run_wake_worker_enabled"`
	SkillInstallationEnabled      bool                    `json:"skill_installation_enabled"`
	EvidenceAttachmentEnabled     bool                    `json:"evidence_attachment_enabled"`
	VerificationEvidenceEnabled   bool                    `json:"verification_evidence_enabled"`
	ProcessExecutionEnabled       bool                    `json:"process_execution_enabled"`
	ShellExecutionEnabled         bool                    `json:"shell_execution_enabled"`
	DockerExecutionEnabled        bool                    `json:"docker_execution_enabled"`
	WakeWorker                    RunWakeWorkerHealthView `json:"wake_worker"`
}

func (a *API) runtimeCapabilities(request *http.Request) (any, *Page, error) {
	if err := rejectQuery(request.URL.Query()); err != nil {
		return nil, nil, err
	}
	worker := RunWakeWorkerHealthView{
		ProtocolVersion: application.RunWakeWorkerHealthProtocolVersion,
		Enabled:         false, State: "disabled", Active: false,
		Concurrency:            application.RunWakeWorkerConcurrency,
		MaxSteps:               application.RunWakeWorkerMaxSteps,
		RuntimeEnableSupported: false, PersistentService: false,
	}
	if a.runWakeWorkerEnabled {
		if a.runWakeWorkerHealthSource == nil {
			return nil, nil, apperror.New(apperror.CodeInternal,
				"Run wake worker health source is unavailable")
		}
		health := a.runWakeWorkerHealthSource.Health()
		if health.ProtocolVersion != application.RunWakeWorkerHealthProtocolVersion ||
			!validRunWakeWorkerHealthState(health.State, health.Active) || health.PollIntervalMillis <
			application.MinRunWakeWorkerPollInterval.Milliseconds() ||
			health.PollIntervalMillis > application.MaxRunWakeWorkerPollInterval.Milliseconds() ||
			health.Concurrency != application.RunWakeWorkerConcurrency ||
			health.MaxSteps != application.RunWakeWorkerMaxSteps {
			return nil, nil, apperror.New(apperror.CodeInternal,
				"Run wake worker health violated its bounded contract")
		}
		worker.Enabled = true
		worker.State = string(health.State)
		worker.Active = health.Active
		worker.PollIntervalMillis = health.PollIntervalMillis
	}
	return RuntimeCapabilitiesView{
		ProtocolVersion:   RuntimeCapabilitiesProtocolVersion,
		RunControlEnabled: a.controlEnabled, RunCreationEnabled: a.runCreationEnabled,
		SessionMessageEnabled:         a.sessionMessageEnabled,
		SessionSteeringControlEnabled: a.sessionSteeringControlEnabled,
		RunLifecycleEnabled:           a.runLifecycleEnabled, RunExecutionEnabled: a.runExecutionEnabled,
		PlanDeliveryControlEnabled:  a.planDeliveryControlEnabled,
		ApprovalControlEnabled:      a.approvalControlEnabled,
		ModelControlEnabled:         a.modelControlEnabled,
		ProviderCredentialEnabled:   a.providerCredentialEnabled,
		FileEditReviewEnabled:       a.fileEditReviewEnabled,
		FileEditProposalEnabled:     a.fileEditProposalEnabled,
		FileEditApplyEnabled:        a.fileEditApplyEnabled,
		RunWakeControlEnabled:       a.runWakeControlEnabled,
		RunWakeExecutionEnabled:     a.runWakeExecutionEnabled,
		RunWakeWorkerEnabled:        a.runWakeWorkerEnabled,
		SkillInstallationEnabled:    a.skillInstallationEnabled,
		EvidenceAttachmentEnabled:   a.evidenceAttachmentEnabled,
		VerificationEvidenceEnabled: a.verificationEvidenceEnabled,
		ProcessExecutionEnabled:     false, ShellExecutionEnabled: false,
		DockerExecutionEnabled: false, WakeWorker: worker,
	}, nil, nil
}

func validRunWakeWorkerHealthState(state application.RunWakeWorkerState, active bool) bool {
	switch state {
	case application.RunWakeWorkerReady, application.RunWakeWorkerStopped:
		return !active
	case application.RunWakeWorkerRunning, application.RunWakeWorkerDraining:
		return true
	default:
		return false
	}
}
