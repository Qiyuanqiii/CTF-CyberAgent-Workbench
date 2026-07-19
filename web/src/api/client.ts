import { consumeSSE } from "./sse";
import type {
  ApprovalDecisionControlRequestView,
  ApprovalDecisionControlView,
  ApprovalQueueView,
  CodeHandoffView,
  CodeHandoffExportView,
  ErrorEnvelope,
  EvidenceAttachmentRequestView,
  EvidenceAttachmentView,
  EvidenceInventoryView,
  FileEditApplyRequestView,
  FileEditApplyView,
  FileEditChangeSetView,
  FileEditProposalRequestView,
  FileEditProposalRecoveryView,
  FileEditProposalSourceView,
  FileEditProposalView,
  FileEditQueueView,
  FileEditReviewRequestView,
  FileEditReviewView,
  HealthView,
  ModelAvailabilityView,
  ModelRouteControlRequestView,
  OperationReceiptView,
  OperationReceiptHistoryView,
  OperatorActionCenterView,
  PageResult,
  PlanDeliveryTransitionControlRequestView,
  PlanDeliveryTransitionControlView,
  PlanDirectionControlRequestView,
  PlanDirectionControlView,
  ProviderDiagnosticRequestView,
  ProviderDiagnosticView,
  ProviderCredentialListView,
  ProviderCredentialRequestView,
  ProviderCredentialStatusView,
  RepositoryStateView,
  RepositoryDiffView,
  RepositoryHistoryView,
  RepositoryCommitDetailView,
  RunCreationControlRequestView,
  RunCreationControlView,
  RunExecutionControlRequestView,
  RunExecutionControlView,
  RunLifecycleControlRequestView,
  RunLifecycleControlView,
  RunWakeCancelRequestView,
  RunWakeControlView,
  RunWakeScheduleRequestView,
  RunWakeStateView,
  RunWakeExecutionRequestView,
  RunWakeExecutionView,
  RuntimeCapabilitiesView,
  SkillPackageInstallRequestView,
  SkillPackageInstallView,
  RunEventPollView,
  RunEventStreamView,
  SessionMessageControlRequestView,
  SessionMessageControlView,
  SessionSteeringCancellationRequestView,
  SessionSteeringCancellationView,
  SuccessEnvelope,
  WorkspaceExplorerView,
  WorkspaceSearchView,
  VerificationEvidenceControlView,
  VerificationEvidenceInventoryView,
  VerificationEvidenceRequestView,
  VerificationPlanControlView,
  VerificationPlanInventoryView,
  VerificationPlanRequestView,
  VerificationAssociationRequestView,
  VerificationAssociationControlView,
  VerificationPlanCoverageInventoryView,
} from "./types";

export type QueryValue = boolean | number | string | undefined;

export interface ClientCapabilities {
  runControlEnabled?: boolean;
  runCreationEnabled?: boolean;
  sessionMessageEnabled?: boolean;
  sessionSteeringControlEnabled?: boolean;
  runLifecycleEnabled?: boolean;
  runExecutionEnabled?: boolean;
  planDeliveryControlEnabled?: boolean;
  approvalControlEnabled?: boolean;
  modelControlEnabled?: boolean;
  providerCredentialEnabled?: boolean;
  fileEditReviewEnabled?: boolean;
  fileEditProposalEnabled?: boolean;
  fileEditApplyEnabled?: boolean;
  runWakeControlEnabled?: boolean;
  runWakeExecutionEnabled?: boolean;
  runWakeWorkerEnabled?: boolean;
  skillInstallationEnabled?: boolean;
  evidenceAttachmentEnabled?: boolean;
  verificationEvidenceEnabled?: boolean;
}

export class APIRequestError extends Error {
  constructor(
    message: string,
    readonly code: string,
    readonly status: number,
    readonly requestID = "",
  ) {
    super(message);
    this.name = "APIRequestError";
  }
}

function normalizeBaseURL(baseURL: string): string {
  const resolved = new URL(baseURL, window.location.origin);
  if (resolved.origin !== window.location.origin) {
    throw new Error("CyberAgent API must use the current browser origin");
  }
  const path = resolved.pathname.replace(/\/+$/, "");
  if (path !== "/api/v1") {
    throw new Error("CyberAgent API base path must be /api/v1");
  }
  return path;
}

function isErrorEnvelope(value: unknown): value is ErrorEnvelope {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Partial<ErrorEnvelope>;
  return candidate.version === "api.v1" && typeof candidate.request_id === "string" &&
    typeof candidate.error?.code === "string" && typeof candidate.error.message === "string";
}

function isSuccessEnvelope<T>(value: unknown): value is SuccessEnvelope<T> {
  if (typeof value !== "object" || value === null) {
    return false;
  }
  const candidate = value as Partial<SuccessEnvelope<T>>;
  return candidate.version === "api.v1" && typeof candidate.request_id === "string" &&
    Object.prototype.hasOwnProperty.call(candidate, "data");
}

function parseStreamFrame(value: unknown, expectedRunID: string, expectedRequestID = ""): RunEventStreamView {
  if (typeof value !== "object" || value === null) {
    throw new Error("SSE frame is not an object");
  }
  const frame = value as Partial<RunEventStreamView>;
  if (frame.version !== "run-events.v1" || frame.run_id !== expectedRunID ||
    typeof frame.request_id !== "string" || frame.request_id === "" ||
    (expectedRequestID !== "" && frame.request_id !== expectedRequestID) ||
    typeof frame.cursor !== "string" || frame.cursor === "" || frame.cursor.length > 512 ||
    typeof frame.sequence !== "number" || !Number.isSafeInteger(frame.sequence) || frame.sequence <= 0 ||
    typeof frame.event !== "object" || frame.event === null ||
    frame.event.version !== "v1" || frame.event.run_id !== expectedRunID ||
    frame.event.sequence !== frame.sequence || typeof frame.event.event_id !== "string" ||
    frame.event.event_id === "" || typeof frame.event.mission_id !== "string" ||
    typeof frame.event.type !== "string" || frame.event.type === "" ||
    typeof frame.event.source !== "string" || frame.event.source === "" ||
    typeof frame.event.created_at !== "string") {
    throw new Error("SSE frame does not match run-events.v1");
  }
  return frame as RunEventStreamView;
}

function parseEventPoll(value: unknown, expectedRunID: string, requestID: string): RunEventPollView {
  if (typeof value !== "object" || value === null) {
    throw new Error("Event poll response is not an object");
  }
  const poll = value as Partial<RunEventPollView>;
  if (poll.version !== "run-event-poll.v1" || poll.run_id !== expectedRunID ||
    typeof poll.cursor !== "string" || poll.cursor === "" || poll.cursor.length > 512 ||
    !Array.isArray(poll.frames) || typeof poll.has_more !== "boolean" ||
    (poll.has_more && poll.frames.length === 0) || poll.frames.length > 100) {
    throw new Error("Event poll response does not match run-event-poll.v1");
  }
  const frames = poll.frames.map((frame) => parseStreamFrame(frame, expectedRunID, requestID));
  for (let index = 1; index < frames.length; index++) {
    if (frames[index]!.sequence !== frames[index - 1]!.sequence + 1) {
      throw new Error("Event poll response contains a sequence gap");
    }
  }
  if (frames.length > 0 && frames[frames.length - 1]!.cursor !== poll.cursor) {
    throw new Error("Event poll response cursor does not match its final frame");
  }
  return { ...poll, frames } as RunEventPollView;
}

function parseRunCreationControl(value: unknown,
  request: RunCreationControlRequestView): RunCreationControlView {
  if (!hasExactKeys(value, ["mission", "mode", "replayed", "run", "session"]) ||
    typeof value.replayed !== "boolean" || !isRecord(value.mission) ||
    !isRecord(value.mode) || !isRecord(value.run) || !isRecord(value.session) ||
    !isRecord(value.run.config) || !isRecord(value.run.budget) || !isRecord(value.mission.scope) ||
    !isRecord(value.mode.scope)) {
    throw new APIRequestError("Run creation response is invalid", "INVALID_RESPONSE", 502);
  }
  const missionID = boundedIdentity(value.mission.id);
  const runID = boundedIdentity(value.run.id);
  const sessionID = boundedIdentity(value.session.id);
  const workspaceID = boundedIdentity(value.mission.workspace_id);
  const expectedProfile = request.profile ?? "code";
  const expectedSurface = request.surface ?? "code";
  const expectedPhase = request.phase ?? "deliver";
  if (!missionID || !runID || !sessionID || !workspaceID ||
    workspaceID !== request.workspace_id || value.mission.goal !== request.goal ||
    value.run.mission_id !== missionID || value.run.session_id !== sessionID ||
    value.session.workspace_id !== workspaceID ||
    value.mission.scope.workspace_id !== workspaceID || value.mode.scope.workspace_id !== workspaceID ||
    value.run.status !== "created" || value.session.status !== "active" ||
    value.run.config.interactive !== true || value.mission.profile !== expectedProfile ||
    value.mode.profile !== expectedProfile ||
    value.run.config.model_route !== expectedProfile || value.session.route !== expectedProfile ||
    value.session.title !== value.mission.goal ||
    value.mission.scope.network_mode !== "disabled" ||
    value.mode.scope.network_mode !== "disabled" ||
    !hasNoAllowedTargets(value.mission.scope) || !hasNoAllowedTargets(value.mode.scope) ||
    value.run.budget.max_turns !== 100 || value.run.budget.max_tool_calls !== 100 ||
    (value.run.budget.max_tokens ?? 0) !== 0 || (value.run.budget.max_cost_usd ?? 0) !== 0 ||
    (value.run.budget.timeout_seconds ?? 0) !== 0 ||
    value.mode.capability_grant !== false || value.mode.protocol_version !== "run_mode.v1" ||
    value.mode.policy_version !== "mode_policy.v1" || value.mode.revision !== 1 ||
    value.mode.surface !== expectedSurface || value.mode.phase !== expectedPhase) {
    throw new APIRequestError("Run creation response violated its closed authority contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as RunCreationControlView;
}

function parseSessionMessageControl(value: unknown,
  expectedSessionID: string): SessionMessageControlView {
  if (!hasExactKeys(value, ["capability_grant", "execution_started", "model_called", "replayed",
    "run_id", "session_id", "steering", "tool_called", "version"]) ||
    value.version !== "session_message_submission.v1" ||
    value.session_id !== expectedSessionID || !boundedIdentity(value.run_id) ||
    !boundedIdentity(value.session_id) || typeof value.replayed !== "boolean" ||
    value.execution_started !== false || value.model_called !== false ||
    value.tool_called !== false || value.capability_grant !== false ||
    !isRecord(value.steering) || !hasOnlyKeys(value.steering,
      ["cancelled_at", "committed_at", "created_at", "id", "prepared", "sequence", "status"]) ||
    !boundedIdentity(value.steering.id) ||
    value.steering.prepared !== false ||
    typeof value.steering.sequence !== "number" ||
    !Number.isSafeInteger(value.steering.sequence) || value.steering.sequence <= 0 ||
    typeof value.steering.created_at !== "string" ||
    !Number.isFinite(Date.parse(value.steering.created_at)) ||
    (value.steering.status !== "pending" && value.steering.status !== "committed" &&
      value.steering.status !== "cancelled")) {
    throw new APIRequestError("Session message response is invalid", "INVALID_RESPONSE", 502);
  }
  const committedAt = value.steering.committed_at;
  const cancelledAt = value.steering.cancelled_at;
  if ((committedAt !== undefined && (typeof committedAt !== "string" ||
      !Number.isFinite(Date.parse(committedAt)))) ||
    (cancelledAt !== undefined && (typeof cancelledAt !== "string" ||
      !Number.isFinite(Date.parse(cancelledAt)))) ||
    (value.steering.status === "pending" && (committedAt !== undefined || cancelledAt !== undefined)) ||
    (value.steering.status === "committed" && (committedAt === undefined || cancelledAt !== undefined)) ||
    (value.steering.status === "cancelled" && (cancelledAt === undefined || committedAt !== undefined))) {
    throw new APIRequestError("Session message response violated its steering state contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as SessionMessageControlView;
}

function parseSessionSteeringCancellation(value: unknown,
  expectedSessionID: string, expectedMessageID: string): SessionSteeringCancellationView {
  if (!hasExactKeys(value, ["cancellation_id", "cancellation_kind", "capability_grant",
    "execution_started", "model_called", "replayed", "run_id", "session_id", "steering",
    "tool_called", "version"]) ||
    value.version !== "session_steering_cancellation.v1" ||
    value.session_id !== expectedSessionID || !boundedIdentity(value.run_id) ||
    !boundedIdentity(value.session_id) || !boundedIdentity(value.cancellation_id) ||
    value.cancellation_kind !== "operator" || typeof value.replayed !== "boolean" ||
    value.execution_started !== false || value.model_called !== false ||
    value.tool_called !== false || value.capability_grant !== false ||
    !isRecord(value.steering) || !hasOnlyKeys(value.steering,
      ["cancelled_at", "committed_at", "created_at", "id", "prepared", "sequence", "status"]) ||
    value.steering.id !== expectedMessageID || value.steering.status !== "cancelled" ||
    value.steering.prepared !== false ||
    typeof value.steering.sequence !== "number" ||
    !Number.isSafeInteger(value.steering.sequence) || value.steering.sequence <= 0 ||
    typeof value.steering.created_at !== "string" ||
    !Number.isFinite(Date.parse(value.steering.created_at)) ||
    typeof value.steering.cancelled_at !== "string" ||
    !Number.isFinite(Date.parse(value.steering.cancelled_at)) ||
    value.steering.committed_at !== undefined) {
    throw new APIRequestError("Session steering cancellation response is invalid",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as SessionSteeringCancellationView;
}

function parseRunLifecycleControl(value: unknown, expectedRunID: string,
  request: RunLifecycleControlRequestView): RunLifecycleControlView {
  if (!hasExactKeys(value, ["action", "applied_status", "capability_grant",
    "event_sequence_end", "event_sequence_start", "execution_started", "expected_status",
    "model_called", "replayed", "run", "tool_called", "version"]) ||
    value.version !== "run_lifecycle_control.v1" || value.action !== request.action ||
    !isRecord(value.run) || value.run.id !== expectedRunID || !boundedIdentity(value.run.id) ||
    typeof value.run.status !== "string" || typeof value.replayed !== "boolean" ||
    value.execution_started !== false || value.model_called !== false ||
    value.tool_called !== false || value.capability_grant !== false ||
    !safePositiveInteger(value.event_sequence_start) ||
    !safePositiveInteger(value.event_sequence_end)) {
    throw new APIRequestError("Run lifecycle response is invalid", "INVALID_RESPONSE", 502);
  }
  const transitions = {
    start: ["created", "running", 2],
    pause: ["running", "paused", 1],
    resume: ["paused", "running", 1],
  } as const;
  const transition = transitions[request.action];
  if (!transition || value.expected_status !== transition[0] ||
    value.applied_status !== transition[1] ||
    (!value.replayed && value.run.status !== transition[1]) ||
    (value.replayed && !isRunStatus(value.run.status)) ||
    value.event_sequence_end - value.event_sequence_start + 1 !== transition[2]) {
    throw new APIRequestError("Run lifecycle response violated its transition contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as RunLifecycleControlView;
}

function isRunStatus(value: unknown): boolean {
  return typeof value === "string" && ["created", "preparing", "running",
    "waiting_approval", "paused", "completed", "failed", "cancelled"].includes(value);
}

function parseRunExecutionControl(value: unknown, expectedRunID: string,
  request: RunExecutionControlRequestView): RunExecutionControlView {
  if (!isRecord(value) || !hasOnlyKeys(value, ["cancelled_count", "capability_grant",
    "committed_count", "completion_event_sequence", "error_code", "execution_started",
    "max_steps", "model_called", "operation_id", "pending_count", "prepared_count",
    "replayed", "run_id", "run_status", "selected_count", "session_id", "status",
    "steps_completed", "stop_reason", "tool_called", "version"]) ||
    value.version !== "run_execution_handoff.v1" || value.run_id !== expectedRunID ||
    !boundedIdentity(value.run_id) || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.operation_id) || value.max_steps !== request.max_steps ||
    !safeBoundedCount(value.selected_count, request.max_steps) ||
    !safeBoundedCount(value.steps_completed, value.selected_count) ||
    !safeBoundedCount(value.pending_count, value.selected_count) ||
    !safeBoundedCount(value.prepared_count, value.selected_count) ||
    !safeBoundedCount(value.committed_count, value.selected_count) ||
    !safeBoundedCount(value.cancelled_count, value.selected_count) ||
    value.pending_count + value.prepared_count + value.committed_count +
      value.cancelled_count !== value.selected_count ||
    !safePositiveInteger(value.completion_event_sequence) ||
    (value.status !== "completed" && value.status !== "failed") ||
    typeof value.run_status !== "string" || typeof value.stop_reason !== "string" ||
    value.stop_reason.length === 0 || value.stop_reason.length > 64 ||
    typeof value.replayed !== "boolean" || typeof value.execution_started !== "boolean" ||
    typeof value.model_called !== "boolean" || typeof value.tool_called !== "boolean" ||
    value.capability_grant !== false || (value.tool_called && !value.model_called) ||
    value.execution_started !== (value.selected_count > 0) ||
    (value.status === "completed" && value.error_code !== undefined) ||
    (value.status === "failed" && (typeof value.error_code !== "string" ||
      value.error_code.length === 0 || value.error_code.length > 64))) {
    throw new APIRequestError("Run execution response is invalid", "INVALID_RESPONSE", 502);
  }
  return value as unknown as RunExecutionControlView;
}

function parseModelAvailability(value: unknown): ModelAvailabilityView {
  if (!hasExactKeys(value, ["generation", "protocol_version", "providers", "routes"]) ||
    value.protocol_version !== "model_availability.v1" || !Array.isArray(value.providers) ||
    !safeBoundedCount(value.generation, Number.MAX_SAFE_INTEGER) || value.generation < 1 ||
    !Array.isArray(value.routes) || value.providers.length > 64 || value.routes.length > 64) {
    throw new APIRequestError("Model availability response is invalid", "INVALID_RESPONSE", 502);
  }
  const providerNames = new Set<string>();
  const availableProviderNames = new Set<string>();
  for (const provider of value.providers) {
    if (!hasExactKeys(provider, ["configuration_error", "credential_source", "kind", "models",
      "name", "network_required", "status"]) || !boundedText(provider.name, 128) ||
      (provider.kind !== "local" && provider.kind !== "anthropic_compatible") ||
      (provider.status !== "available" && provider.status !== "not_configured" &&
        provider.status !== "invalid_configuration") ||
      (provider.credential_source !== "none" && provider.credential_source !== "environment" &&
        provider.credential_source !== "system") ||
      typeof provider.network_required !== "boolean" ||
      typeof provider.configuration_error !== "boolean" || !Array.isArray(provider.models) ||
      provider.models.length > 64 || !provider.models.every((model) => boundedText(model, 256)) ||
      providerNames.has(provider.name)) {
      throw new APIRequestError("Model Provider availability is invalid", "INVALID_RESPONSE", 502);
    }
    providerNames.add(provider.name);
    if (provider.status === "available") {
      availableProviderNames.add(provider.name);
    }
  }
  const routeNames = new Set<string>();
  for (const route of value.routes) {
    if (!hasExactKeys(route, ["available", "model", "name", "provider"]) ||
      !boundedText(route.name, 128) || !boundedText(route.provider, 128) ||
      !boundedText(route.model, 256) || typeof route.available !== "boolean" ||
      (route.available && !availableProviderNames.has(route.provider)) ||
      routeNames.has(route.name)) {
      throw new APIRequestError("Model route availability is invalid", "INVALID_RESPONSE", 502);
    }
    routeNames.add(route.name);
  }
  return value as unknown as ModelAvailabilityView;
}

function parsePlanDirectionControl(value: unknown, expectedRunID: string,
  request: PlanDirectionControlRequestView): PlanDirectionControlView {
  if (!hasExactKeys(value, ["capability_grant", "direction", "execution_started", "model_called",
    "note_id", "phase_changed", "proposal_id", "replayed", "run_id", "selection_id",
    "tool_called", "version", "work_item_count"]) ||
    value.version !== "plan_delivery_control.v1" || value.run_id !== expectedRunID ||
    value.proposal_id !== request.proposal_id || value.direction !== request.direction ||
    !boundedIdentity(value.run_id) || !boundedIdentity(value.proposal_id) ||
    !boundedIdentity(value.selection_id) || !boundedIdentity(value.note_id) ||
    !safeBoundedCount(value.work_item_count, 32) || value.work_item_count < 1 ||
    typeof value.replayed !== "boolean" || value.phase_changed !== false ||
    value.execution_started !== false || value.model_called !== false ||
    value.tool_called !== false || value.capability_grant !== false) {
    throw new APIRequestError("Plan direction response violated its closed authority contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as PlanDirectionControlView;
}

function parsePlanDeliveryTransition(value: unknown,
  expectedRunID: string): PlanDeliveryTransitionControlView {
  if (!hasExactKeys(value, ["applied_mode", "capability_grant", "current_mode",
    "execution_started", "model_called", "replayed", "run_id", "selection_id", "tool_called",
    "version"]) || value.version !== "plan_delivery_control.v1" ||
    value.run_id !== expectedRunID || !boundedIdentity(value.run_id) ||
    !boundedIdentity(value.selection_id) || !isRecord(value.applied_mode) ||
    !isRecord(value.current_mode) || value.applied_mode.phase !== "deliver" ||
    value.current_mode.phase !== "deliver" || value.applied_mode.capability_grant !== false ||
    value.current_mode.capability_grant !== false || typeof value.replayed !== "boolean" ||
    value.execution_started !== false || value.model_called !== false ||
    value.tool_called !== false || value.capability_grant !== false) {
    throw new APIRequestError("Plan delivery response violated its closed authority contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as PlanDeliveryTransitionControlView;
}

function parseApprovalQueue(value: unknown, expectedRunID: string): ApprovalQueueView {
  if (!hasExactKeys(value, ["capability_grant", "items", "process_execution_enabled",
    "protocol_version", "run_id", "session_grant_created", "truncated"]) ||
    value.protocol_version !== "approval_queue.v1" || value.run_id !== expectedRunID ||
    !boundedIdentity(value.run_id) || !Array.isArray(value.items) || value.items.length > 100 ||
    typeof value.truncated !== "boolean" || value.process_execution_enabled !== false ||
    value.session_grant_created !== false || value.capability_grant !== false) {
    throw new APIRequestError("Approval queue response is invalid", "INVALID_RESPONSE", 502);
  }
  const identities = new Set<string>();
  for (const item of value.items) {
    const itemID = isRecord(item) ? boundedIdentity(item.id) : "";
    if (!hasExactKeys(item, ["action_class", "allowed_actions", "capability_grant", "created_at",
      "id", "mode", "process_execution_enabled", "proposal_id", "run_id", "session_id",
      "status", "tool_name", "updated_at", "version", "workspace_id"]) ||
      item.run_id !== expectedRunID || item.status !== "pending" || !itemID ||
      !boundedIdentity(item.proposal_id) || !boundedIdentity(item.run_id) ||
      !boundedIdentity(item.session_id) || !boundedIdentity(item.workspace_id) ||
      !boundedText(item.tool_name, 128) || !boundedText(item.action_class, 128) ||
      !boundedText(item.mode, 64) || !Array.isArray(item.allowed_actions) ||
      item.allowed_actions.length > 2 ||
      !item.allowed_actions.every((action) => action === "approve_once" || action === "deny") ||
      new Set(item.allowed_actions).size !== item.allowed_actions.length ||
      !safePositiveInteger(item.version) || !validDate(item.created_at) ||
      !validDate(item.updated_at) || item.process_execution_enabled !== false ||
      item.capability_grant !== false || identities.has(itemID)) {
      throw new APIRequestError("Approval queue item is invalid", "INVALID_RESPONSE", 502);
    }
    if (item.tool_name === "replace_file" && item.allowed_actions.includes("approve_once")) {
      throw new APIRequestError("File approval exposed write authority", "INVALID_RESPONSE", 502);
    }
    identities.add(itemID);
  }
  return value as unknown as ApprovalQueueView;
}

function parseApprovalDecision(value: unknown, expectedRunID: string, expectedApprovalID: string,
  request: ApprovalDecisionControlRequestView): ApprovalDecisionControlView {
  const expectedStatus = request.action === "approve_once" ? "approved" : "denied";
  if (!hasExactKeys(value, ["action", "approval_id", "capability_grant",
    "docker_execution_enabled", "process_execution_enabled", "proposal_id", "replayed", "run_id",
    "session_grant_created", "shell_execution_enabled", "status", "tool_name", "version",
    "workspace_write_applied"]) || value.version !== "approval_control.v1" ||
    value.run_id !== expectedRunID || value.approval_id !== expectedApprovalID ||
    value.action !== request.action || value.status !== expectedStatus ||
    !boundedIdentity(value.run_id) || !boundedIdentity(value.approval_id) ||
    !boundedIdentity(value.proposal_id) || !boundedText(value.tool_name, 128) ||
    typeof value.replayed !== "boolean" || value.process_execution_enabled !== false ||
    value.shell_execution_enabled !== false || value.docker_execution_enabled !== false ||
    value.workspace_write_applied !== false || value.session_grant_created !== false ||
    value.capability_grant !== false) {
    throw new APIRequestError("Approval decision violated its closed authority contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as ApprovalDecisionControlView;
}

function parseModelRouteControl(value: unknown, route: string,
  request: ModelRouteControlRequestView): ModelAvailabilityView["routes"][number] {
  if (!hasExactKeys(value, ["available", "model", "name", "provider"]) ||
    value.name !== route || value.provider !== request.provider || value.model !== request.model ||
    value.available !== true) {
    throw new APIRequestError("Model route response violated its exact binding",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as ModelAvailabilityView["routes"][number];
}

function parseProviderDiagnostic(value: unknown, request: ProviderDiagnosticRequestView): ProviderDiagnosticView {
  if (!hasExactKeys(value, ["duration_ms", "model", "model_called",
    "network_request_attempted", "outcome", "protocol_version", "provider",
    "response_content_returned", "retryable", "status", "tool_called"]) ||
    value.protocol_version !== "provider_diagnostic.v1" || value.provider !== request.provider ||
    value.model !== request.model || (value.status !== "reachable" && value.status !== "unreachable") ||
    !boundedText(value.outcome, 64) || typeof value.retryable !== "boolean" ||
    typeof value.network_request_attempted !== "boolean" || value.model_called !== true ||
    value.tool_called !== false || value.response_content_returned !== false ||
    !safeBoundedCount(value.duration_ms, 60_000)) {
    throw new APIRequestError("Provider diagnostic response violated its content-free contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as ProviderDiagnosticView;
}

function parseProviderCredentialStatus(value: unknown,
  expectedProvider = ""): ProviderCredentialStatusView {
  if (!hasExactKeys(value, ["configured", "plaintext_returned", "protocol_version",
    "provider", "registry_generation", "registry_reloaded", "restart_required",
    "store_available", "store_kind"]) ||
    value.protocol_version !== "provider_credential.v1" ||
    !["anthropic", "deepseek", "mimo"].includes(String(value.provider)) ||
    (expectedProvider !== "" && value.provider !== expectedProvider) ||
    typeof value.configured !== "boolean" || typeof value.store_available !== "boolean" ||
    !boundedText(value.store_kind, 128) || value.plaintext_returned !== false ||
    typeof value.restart_required !== "boolean" || typeof value.registry_reloaded !== "boolean" ||
    !safeBoundedCount(value.registry_generation, Number.MAX_SAFE_INTEGER) ||
    (value.registry_reloaded && (value.restart_required || value.registry_generation < 1))) {
    throw new APIRequestError("Provider credential status violated its plaintext-free contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as ProviderCredentialStatusView;
}

function parseProviderCredentialList(value: unknown): ProviderCredentialListView {
  if (!hasExactKeys(value, ["items", "protocol_version"]) ||
    value.protocol_version !== "provider_credential.v1" || !Array.isArray(value.items) ||
    value.items.length !== 3) {
    throw new APIRequestError("Provider credential list is invalid", "INVALID_RESPONSE", 502);
  }
  const items = value.items.map((item) => parseProviderCredentialStatus(item));
  if (new Set(items.map((item) => item.provider)).size !== items.length ||
    items.some((item) => item.restart_required || item.registry_reloaded) ||
    new Set(items.map((item) => item.registry_generation)).size !== 1) {
    throw new APIRequestError("Provider credential list widened status authority",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, items } as unknown as ProviderCredentialListView;
}

function parseRuntimeCapabilities(value: unknown): RuntimeCapabilitiesView {
  const capabilityKeys = ["approval_control_enabled", "docker_execution_enabled",
    "evidence_attachment_enabled", "verification_evidence_enabled",
    "file_edit_apply_enabled", "file_edit_proposal_enabled",
    "file_edit_review_enabled", "model_control_enabled", "plan_delivery_control_enabled",
    "process_execution_enabled", "provider_credential_enabled", "protocol_version",
    "run_control_enabled", "run_creation_enabled", "run_execution_enabled",
    "run_lifecycle_enabled", "run_wake_control_enabled", "run_wake_execution_enabled",
    "run_wake_worker_enabled", "session_message_enabled",
    "session_steering_control_enabled", "shell_execution_enabled",
    "skill_installation_enabled", "wake_worker"];
  if (!hasExactKeys(value, capabilityKeys) || value.protocol_version !== "runtime_capabilities.v1") {
    throw new APIRequestError("Runtime capability response is invalid", "INVALID_RESPONSE", 502);
  }
  for (const key of capabilityKeys) {
    if (key !== "protocol_version" && key !== "wake_worker" && typeof value[key] !== "boolean") {
      throw new APIRequestError("Runtime capability flag is invalid", "INVALID_RESPONSE", 502);
    }
  }
  const worker = value.wake_worker;
  if (!hasExactKeys(worker, ["active", "concurrency", "enabled", "max_steps",
    "persistent_service", "poll_interval_ms", "protocol_version",
    "runtime_enable_supported", "state"]) ||
    worker.protocol_version !== "run_wake_worker_health.v1" ||
    typeof worker.enabled !== "boolean" || typeof worker.active !== "boolean" ||
    worker.concurrency !== 1 || worker.max_steps !== 1 ||
    worker.runtime_enable_supported !== false || worker.persistent_service !== false ||
    value.run_wake_worker_enabled !== worker.enabled ||
    (worker.enabled && (!["ready", "running", "draining", "stopped"].includes(String(worker.state)) ||
      !safeBoundedCount(worker.poll_interval_ms, 60_000) || worker.poll_interval_ms < 250)) ||
    ((worker.state === "ready" || worker.state === "stopped") && worker.active) ||
    ((worker.state === "running" || worker.state === "draining") && !worker.active) ||
    (!worker.enabled && (worker.state !== "disabled" || worker.active || worker.poll_interval_ms !== 0)) ||
    value.process_execution_enabled !== false || value.shell_execution_enabled !== false ||
    value.docker_execution_enabled !== false) {
    throw new APIRequestError("Run wake worker capability response is invalid",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as RuntimeCapabilitiesView;
}

export function clientCapabilitiesFromRuntime(value: RuntimeCapabilitiesView): ClientCapabilities {
  return {
    runControlEnabled: value.run_control_enabled,
    runCreationEnabled: value.run_creation_enabled,
    sessionMessageEnabled: value.session_message_enabled,
    sessionSteeringControlEnabled: value.session_steering_control_enabled,
    runLifecycleEnabled: value.run_lifecycle_enabled,
    runExecutionEnabled: value.run_execution_enabled,
    planDeliveryControlEnabled: value.plan_delivery_control_enabled,
    approvalControlEnabled: value.approval_control_enabled,
    modelControlEnabled: value.model_control_enabled,
    providerCredentialEnabled: value.provider_credential_enabled,
    fileEditReviewEnabled: value.file_edit_review_enabled,
    fileEditProposalEnabled: value.file_edit_proposal_enabled,
    fileEditApplyEnabled: value.file_edit_apply_enabled,
    runWakeControlEnabled: value.run_wake_control_enabled,
    runWakeExecutionEnabled: value.run_wake_execution_enabled,
    runWakeWorkerEnabled: value.run_wake_worker_enabled,
    skillInstallationEnabled: value.skill_installation_enabled,
    evidenceAttachmentEnabled: value.evidence_attachment_enabled,
    verificationEvidenceEnabled: value.verification_evidence_enabled,
  };
}

function parseFileEditProposalSource(value: unknown, runID: string,
  expectedPath: string): FileEditProposalSourceView {
  if (!hasExactKeys(value, ["content", "content_sha256", "editable", "expires_at",
    "file_write", "path", "protocol_version", "run_id", "source_handle", "workspace_id"]) ||
    value.protocol_version !== "file_edit_proposal.v1" || value.run_id !== runID ||
    value.path !== expectedPath || !boundedIdentity(value.run_id) ||
    !boundedIdentity(value.workspace_id) || !validWorkspaceRelativePath(value.path) ||
    typeof value.content !== "string" || new TextEncoder().encode(value.content).length > 131_072 ||
    !isSHA256(value.content_sha256) || typeof value.source_handle !== "string" ||
    !/^[A-Za-z0-9_-]{43}$/u.test(value.source_handle) || !validDate(value.expires_at) ||
    Date.parse(value.expires_at) <= Date.now() || value.editable !== true ||
    value.file_write !== false) {
    throw new APIRequestError("File edit source violated its exact no-write contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as FileEditProposalSourceView;
}

function parseFileEditProposal(value: unknown, runID: string): FileEditProposalView {
  if (!hasExactKeys(value, ["approval_required", "edit", "file_written",
    "protocol_version", "replayed", "run_id"]) ||
    value.protocol_version !== "file_edit_proposal.v1" || value.run_id !== runID ||
    value.approval_required !== true || value.file_written !== false ||
    typeof value.replayed !== "boolean") {
    throw new APIRequestError("File edit proposal widened write authority", "INVALID_RESPONSE", 502);
  }
  const edit = parseFileEditPreview(value.edit);
  if (edit.status !== "proposed" || edit.apply_enabled !== false ||
    edit.allowed_actions.length > 2) {
    throw new APIRequestError("File edit proposal result is not pending review",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, edit } as unknown as FileEditProposalView;
}

function parseFileEditProposalRecovery(value: unknown, runID: string,
  editID: string): FileEditProposalRecoveryView {
  if (!hasExactKeys(value, ["current_content_sha256", "edit_id", "editable", "file_write",
    "original_content", "original_sha256", "path", "proposed_content", "proposed_sha256",
    "protocol_version", "review_required", "run_id", "stale", "status", "workspace_id"]) ||
    value.protocol_version !== "file_edit_proposal_recovery.v1" || value.run_id !== runID ||
    value.edit_id !== editID || !boundedIdentity(value.workspace_id) ||
    !validWorkspaceRelativePath(value.path) || typeof value.original_content !== "string" ||
    typeof value.proposed_content !== "string" ||
    new TextEncoder().encode(value.original_content).length > 256 * 1024 ||
    new TextEncoder().encode(value.proposed_content).length > 256 * 1024 ||
    !(isSHA256(value.original_sha256) ||
      (value.original_sha256 === "missing" && value.original_content === "")) ||
    !isSHA256(value.proposed_sha256) ||
    !(isSHA256(value.current_content_sha256) || value.current_content_sha256 === "missing") ||
    value.status !== "proposed" || value.stale !==
      (value.current_content_sha256 !== value.original_sha256) ||
    value.review_required !== true || value.editable !== false || value.file_write !== false) {
    throw new APIRequestError("File edit proposal recovery widened authority",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as FileEditProposalRecoveryView;
}

function parseFileEditPreview(value: unknown): FileEditQueueView["items"][number] {
  if (!isRecord(value) || !hasOnlyKeys(value, ["allowed_actions", "apply_enabled", "created_at",
    "diff", "id", "original_hash", "path", "proposed_hash", "reason", "secrets_redacted",
    "session_id", "status", "updated_at", "workspace_id"]) ||
    !["proposed", "approved", "applied", "denied", "failed"].includes(String(value.status)) ||
    !boundedIdentity(value.id) || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.workspace_id) || !boundedText(value.path, 4096) ||
    typeof value.diff !== "string" || value.diff.length > 1_100_000 ||
    !boundedText(value.original_hash, 128) || !boundedText(value.proposed_hash, 128) ||
    typeof value.secrets_redacted !== "boolean" || typeof value.apply_enabled !== "boolean" ||
    !Array.isArray(value.allowed_actions) || value.allowed_actions.length > 2 ||
    !value.allowed_actions.every((action) => action === "approve_intent" || action === "deny") ||
    !validDate(value.created_at) || !validDate(value.updated_at) ||
    (value.reason !== undefined && typeof value.reason !== "string") ||
    (value.apply_enabled === true &&
      (value.status !== "approved" || value.allowed_actions.length !== 0))) {
    throw new APIRequestError("File edit preview violated its metadata-only contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as FileEditQueueView["items"][number];
}

function parseFileEditQueue(value: unknown, runID: string): FileEditQueueView {
  if (!hasExactKeys(value, ["apply_enabled", "items", "protocol_version", "run_id", "truncated"]) ||
    value.protocol_version !== "file_edit_review.v1" || value.run_id !== runID ||
    typeof value.apply_enabled !== "boolean" || typeof value.truncated !== "boolean" ||
    !Array.isArray(value.items) || value.items.length > 100) {
    throw new APIRequestError("File edit queue response is invalid", "INVALID_RESPONSE", 502);
  }
  const items = value.items.map(parseFileEditPreview);
  if (!value.apply_enabled && items.some((item) => item.apply_enabled)) {
    throw new APIRequestError("File edit queue widened apply authority", "INVALID_RESPONSE", 502);
  }
  return { ...value, items } as unknown as FileEditQueueView;
}

function parseFileEditChangeSet(value: unknown, runID: string): FileEditChangeSetView {
  const keys = ["applied_count", "apply_independent", "approved_count", "atomic_apply",
    "batch_mutation_supported", "denied_count", "diff_content_included", "failed_count",
    "items", "partial_apply_visible", "proposed_count", "protocol_version",
    "returned_count", "review_independent", "run_id", "session_id", "total_diff_bytes",
    "truncated", "workspace_id"];
  if (!hasExactKeys(value, keys) || value.protocol_version !== "file_edit_change_set.v1" ||
    value.run_id !== runID || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.workspace_id) || !Array.isArray(value.items) ||
    value.items.length > 100 || !safeBoundedCount(value.proposed_count, 100) ||
    !safeBoundedCount(value.approved_count, 100) ||
    !safeBoundedCount(value.applied_count, 100) ||
    !safeBoundedCount(value.denied_count, 100) ||
    !safeBoundedCount(value.failed_count, 100) ||
    !safeBoundedCount(value.returned_count, 100) ||
    !safeBoundedCount(value.total_diff_bytes, 100 * 1_064_960) ||
    typeof value.truncated !== "boolean" || value.review_independent !== true ||
    value.apply_independent !== true || value.atomic_apply !== false ||
    value.batch_mutation_supported !== false || value.partial_apply_visible !== true ||
    value.diff_content_included !== false) {
    throw new APIRequestError("File edit change set widened batch mutation authority",
      "INVALID_RESPONSE", 502);
  }
  const items = value.items.map((item) => {
    if (!hasExactKeys(item, ["allowed_actions", "apply_enabled", "diff_bytes", "id", "path",
      "secrets_redacted", "status", "updated_at"]) || !boundedIdentity(item.id) ||
      !validWorkspaceRelativePath(item.path) || item.path === "." ||
      !["proposed", "approved", "applied", "denied", "failed"].includes(String(item.status)) ||
      !safeBoundedCount(item.diff_bytes, 1_064_960) ||
      typeof item.secrets_redacted !== "boolean" || typeof item.apply_enabled !== "boolean" ||
      !Array.isArray(item.allowed_actions) || item.allowed_actions.length > 2 ||
      !item.allowed_actions.every((action) => action === "approve_intent" || action === "deny") ||
      !validDate(item.updated_at) ||
      (item.status === "proposed" ?
        !(item.allowed_actions.length === 0 ||
          (item.allowed_actions.length === 2 &&
            item.allowed_actions.includes("approve_intent") &&
            item.allowed_actions.includes("deny"))) : item.allowed_actions.length !== 0) ||
      (item.apply_enabled && item.status !== "approved")) {
      throw new APIRequestError("File edit change set item violated per-file authority",
        "INVALID_RESPONSE", 502);
    }
    return item;
  });
  const counts = new Map<string, number>([
    ["proposed", value.proposed_count], ["approved", value.approved_count],
    ["applied", value.applied_count], ["denied", value.denied_count],
    ["failed", value.failed_count],
  ]);
  if (value.returned_count !== items.length ||
    [...counts.values()].reduce((sum, count) => sum + count, 0) !== items.length ||
    items.some((item) => counts.get(String(item.status)) === undefined) ||
    value.total_diff_bytes !== items.reduce((sum, item) => sum + Number(item.diff_bytes), 0)) {
    throw new APIRequestError("File edit change set contains inconsistent partial state",
      "INVALID_RESPONSE", 502);
  }
  for (const status of counts.keys()) {
    if (items.filter((item) => item.status === status).length !== counts.get(status)) {
      throw new APIRequestError("File edit change set contains inconsistent partial state",
        "INVALID_RESPONSE", 502);
    }
  }
  return { ...value, items } as unknown as FileEditChangeSetView;
}

function parseFileEditReview(value: unknown, runID: string, editID: string,
  request: FileEditReviewRequestView): FileEditReviewView {
  if (!hasExactKeys(value, ["action", "edit", "file_written", "protocol_version", "replayed", "run_id"]) ||
    value.protocol_version !== "file_edit_review.v1" || value.run_id !== runID ||
    value.action !== request.action || value.file_written !== false || typeof value.replayed !== "boolean") {
    throw new APIRequestError("File edit review response violated its no-write contract",
      "INVALID_RESPONSE", 502);
  }
  const edit = parseFileEditPreview(value.edit);
  const expected = request.action === "approve_intent" ? "approved" : "denied";
  if (edit.id !== editID || edit.status !== expected || edit.apply_enabled !== false) {
    throw new APIRequestError("File edit review result does not match the requested decision",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, edit } as unknown as FileEditReviewView;
}

function parseFileEditApply(value: unknown, runID: string, editID: string): FileEditApplyView {
  if (!hasExactKeys(value, ["edit", "file_written", "policy_rechecked", "protocol_version",
    "receipt", "replayed", "run_id", "status"]) || value.protocol_version !== "file_edit_apply.v1" ||
    value.run_id !== runID || (value.status !== "applied" && value.status !== "failed") ||
    typeof value.replayed !== "boolean" || typeof value.file_written !== "boolean" ||
    value.policy_rechecked !== true) {
    throw new APIRequestError("File edit apply response violated its audited contract",
      "INVALID_RESPONSE", 502);
  }
  const edit = parseFileEditPreview(value.edit);
  if (edit.id !== editID || edit.apply_enabled !== false ||
    (value.status === "applied" && edit.status !== "applied") ||
    (value.status === "failed" && edit.status !== "failed")) {
    throw new APIRequestError("File edit apply result does not match the requested edit",
      "INVALID_RESPONSE", 502);
  }
  const receipt = parseOperationReceipt(value.receipt, "file_edit_apply", value.status,
    value.replayed);
  return { ...value, edit, receipt } as unknown as FileEditApplyView;
}

function parseRunWakeIntent(value: unknown, runID: string): NonNullable<RunWakeStateView["intent"]> {
  if (!isRecord(value) || !hasOnlyKeys(value, ["attempt_count", "background_loop_enabled",
    "base_backoff_seconds", "cancelled_at", "created_at", "deadline_at", "execution_enabled",
    "id", "initial_delay_seconds", "max_attempts", "max_backoff_seconds",
    "max_elapsed_seconds", "next_wake_at", "protocol_version", "run_id", "session_id",
    "status", "updated_at"]) || value.protocol_version !== "run_wake_intent.v1" ||
    value.run_id !== runID || !boundedIdentity(value.id) || !boundedIdentity(value.session_id) ||
    !["queued", "leased", "completed", "cancelled", "exhausted"].includes(String(value.status)) ||
    !safeBoundedCount(value.attempt_count, 8) || !safePositiveInteger(value.max_attempts) ||
    Number(value.attempt_count) > Number(value.max_attempts) ||
    !safeBoundedCount(value.initial_delay_seconds, 3600) ||
    !safeBoundedCount(value.base_backoff_seconds, 21_600) ||
    !safeBoundedCount(value.max_backoff_seconds, 21_600) ||
    !safeBoundedCount(value.max_elapsed_seconds, 86_400) || value.execution_enabled !== false ||
    value.background_loop_enabled !== false || !validDate(value.next_wake_at) ||
    !validDate(value.deadline_at) || !validDate(value.created_at) || !validDate(value.updated_at) ||
    (value.cancelled_at !== undefined && !validDate(value.cancelled_at))) {
    throw new APIRequestError("Run wake intent violated its closed authority contract",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as NonNullable<RunWakeStateView["intent"]>;
}

function parseRunWakeState(value: unknown, runID: string): RunWakeStateView {
  if (!isRecord(value) || !hasOnlyKeys(value, ["found", "intent", "protocol_version", "run_id"]) ||
    value.protocol_version !== "run_wake_intent.v1" || value.run_id !== runID ||
    typeof value.found !== "boolean" || (value.found !== (value.intent !== undefined))) {
    throw new APIRequestError("Run wake state response is invalid", "INVALID_RESPONSE", 502);
  }
  return value.found
    ? { ...value, intent: parseRunWakeIntent(value.intent, runID) } as unknown as RunWakeStateView
    : value as unknown as RunWakeStateView;
}

function parseRunWakeControl(value: unknown, runID: string,
  expectedAction: "cancel" | "schedule"): RunWakeControlView {
  if (!hasExactKeys(value, ["action", "execution_started", "intent", "model_called",
    "protocol_version", "replayed", "tool_called"]) ||
    value.protocol_version !== "run_wake_control.v1" || value.action !== expectedAction ||
    typeof value.replayed !== "boolean" || value.execution_started !== false ||
    value.model_called !== false || value.tool_called !== false) {
    throw new APIRequestError("Run wake response widened execution authority",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, intent: parseRunWakeIntent(value.intent, runID) } as unknown as RunWakeControlView;
}

function parseRunWakeExecution(value: unknown, runID: string): RunWakeExecutionView {
  if (!isRecord(value) || !hasOnlyKeys(value, ["background_loop_enabled", "consumption_status",
    "execution_started", "intent", "model_called", "protocol_version", "receipt", "replayed", "run_id",
    "stop_reason", "tool_called"]) || value.protocol_version !== "run_wake_consumer.v1" ||
    value.run_id !== runID || value.consumption_status !== "completed" ||
    value.execution_started !== true || typeof value.model_called !== "boolean" ||
    typeof value.tool_called !== "boolean" || value.background_loop_enabled !== false ||
    typeof value.replayed !== "boolean" ||
    (value.stop_reason !== undefined && !boundedText(value.stop_reason, 64))) {
    throw new APIRequestError("Foreground Run wake response violated its bounded contract",
      "INVALID_RESPONSE", 502);
  }
  const intent = parseRunWakeIntent(value.intent, runID);
  if (intent.status !== "completed") {
    throw new APIRequestError("Foreground Run wake did not settle its exact intent",
      "INVALID_RESPONSE", 502);
  }
  const receipt = parseOperationReceipt(value.receipt, "run_wake_consume", "completed",
    value.replayed);
  return { ...value, intent, receipt } as unknown as RunWakeExecutionView;
}

function parseSkillPackageInstall(value: unknown,
  request: SkillPackageInstallRequestView): SkillPackageInstallView {
  if (!hasExactKeys(value, ["archive_sha256", "context_injection_authorized",
    "import_command_execution", "import_network_access", "import_provider_calls", "name",
    "package_fingerprint", "protocol_version", "receipt", "recovered_pending", "replayed",
    "run_selection_authorized", "surface", "tool_capability_grant", "trust_class", "version"]) ||
    value.protocol_version !== "skill_package_installation.v1" ||
    value.surface !== request.surface || value.trust_class !== "operator_installed_untrusted" ||
    !boundedText(value.name, 128) || !boundedText(value.version, 64) ||
    !isSHA256(value.archive_sha256) || !isSHA256(value.package_fingerprint) ||
    typeof value.replayed !== "boolean" || typeof value.recovered_pending !== "boolean" ||
    value.import_command_execution !== false || value.import_network_access !== false ||
    value.import_provider_calls !== false || value.tool_capability_grant !== false ||
    value.run_selection_authorized !== false || value.context_injection_authorized !== false) {
    throw new APIRequestError("Skill package installation widened inert Registry authority",
      "INVALID_RESPONSE", 502);
  }
  const receipt = parseOperationReceipt(value.receipt, "skill_package_install", "installed",
    value.replayed);
  return { ...value, receipt } as unknown as SkillPackageInstallView;
}

function parseOperationReceipt(value: unknown, kind: OperationReceiptView["kind"],
  outcome: OperationReceiptView["outcome"], replayed: boolean): OperationReceiptView {
  if (!hasExactKeys(value, ["cleanup_state", "durable", "kind", "outcome", "protocol_version",
    "recovery_action", "replayed", "retry_safe", "retry_strategy"]) ||
    value.protocol_version !== "operation_receipt.v1" || value.kind !== kind ||
    value.outcome !== outcome || value.durable !== true || value.replayed !== replayed ||
    value.retry_safe !== true) {
    throw new APIRequestError("Operation receipt violated its durable recovery contract",
      "INVALID_RESPONSE", 502);
  }
  const apply = kind === "file_edit_apply";
  if ((apply && value.retry_strategy !== "same_operation_key") ||
    (kind === "run_wake_consume" && value.retry_strategy !== "same_wake_generation") ||
    (kind === "skill_package_install" && value.retry_strategy !== "same_operation_key") ||
    (apply && !["complete", "pending_review"].includes(String(value.cleanup_state))) ||
    (!apply && value.cleanup_state !== "not_applicable") ||
    (value.cleanup_state === "pending_review" &&
      value.recovery_action !== "retry_after_cleanup_grace") ||
    (value.cleanup_state !== "pending_review" && value.recovery_action !== "none")) {
    throw new APIRequestError("Operation receipt widened recovery authority",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as OperationReceiptView;
}

function parseWorkspaceExplorer(value: unknown, workspaceID: string,
  expectedPath: string): WorkspaceExplorerView {
  if (!hasExactKeys(value, ["content", "entries", "kind", "path", "protocol_version",
    "provenance", "redaction_count", "returned_bytes", "root_path_exposed", "total_bytes",
    "truncated", "workspace_id"]) || value.protocol_version !== "workspace_explorer.v1" ||
    value.workspace_id !== workspaceID || value.path !== expectedPath ||
    (value.kind !== "directory" && value.kind !== "file") || !Array.isArray(value.entries) ||
    value.entries.length > 200 || typeof value.content !== "string" ||
    value.content.length > 131_072 || !safeBoundedCount(value.total_bytes, Number.MAX_SAFE_INTEGER) ||
    !safeBoundedCount(value.returned_bytes, 131_072) ||
    !safeBoundedCount(value.redaction_count, 65_536) || typeof value.truncated !== "boolean" ||
    value.root_path_exposed !== false || !isRecord(value.provenance) ||
    !hasExactKeys(value.provenance, ["content_sha256", "instruction_authorized", "source_kind",
      "source_ref", "version"]) || value.provenance.version !== "context_provenance.v1" ||
    value.provenance.source_ref !== expectedPath || !isSHA256(value.provenance.content_sha256) ||
    value.provenance.instruction_authorized !== false ||
    (value.kind === "directory" && (value.content !== "" || value.total_bytes !== 0 ||
      value.returned_bytes !== 0 || value.provenance.source_kind !== "workspace_listing")) ||
    (value.kind === "file" && (value.entries.length !== 0 ||
      value.provenance.source_kind !== "workspace_file" ||
      new TextEncoder().encode(value.content).length !== value.returned_bytes))) {
    throw new APIRequestError("Workspace explorer response violated its bounded evidence contract",
      "INVALID_RESPONSE", 502);
  }
  const entries = value.entries.map((entry) => {
    if (!hasExactKeys(entry, ["kind", "name", "path", "readable", "size_bytes"])) {
      throw new APIRequestError("Workspace explorer entry widened renderer path authority",
        "INVALID_RESPONSE", 502);
    }
    const expectedEntryPath = expectedPath === "." ? String(entry.name) :
      `${expectedPath}/${String(entry.name)}`;
    if (!validWorkspaceEntryName(entry.name) || !validWorkspaceRelativePath(entry.path) ||
      entry.path !== expectedEntryPath ||
      !["directory", "file", "blocked"].includes(String(entry.kind)) ||
      !safeBoundedCount(entry.size_bytes, Number.MAX_SAFE_INTEGER) ||
      typeof entry.readable !== "boolean" ||
      (entry.kind === "blocked" ? entry.readable !== false : entry.readable !== true) ||
      String(entry.name).startsWith(".cyberagent-edit-")) {
      throw new APIRequestError("Workspace explorer entry widened renderer path authority",
        "INVALID_RESPONSE", 502);
    }
    return entry;
  });
  return { ...value, entries } as unknown as WorkspaceExplorerView;
}

function parseWorkspaceSearch(value: unknown, workspaceID: string): WorkspaceSearchView {
  if (!hasExactKeys(value, ["protocol_version", "results", "root_path_exposed",
    "scanned_bytes", "scanned_entries", "scanned_files", "truncated", "workspace_id"]) ||
    value.protocol_version !== "workspace_search.v1" || value.workspace_id !== workspaceID ||
    !Array.isArray(value.results) || value.results.length > 50 ||
    !safeBoundedCount(value.scanned_entries, 1000) ||
    !safeBoundedCount(value.scanned_files, 64) ||
    !safeBoundedCount(value.scanned_bytes, 64 * (64 * 1024 + 4)) ||
    typeof value.truncated !== "boolean" || value.root_path_exposed !== false) {
    throw new APIRequestError("Workspace search response violated its bounded evidence contract",
      "INVALID_RESPONSE", 502);
  }
  const results = value.results.map((item) => {
    if (!hasExactKeys(item, ["content_truncated", "line", "match_kind", "path",
      "provenance", "snippet"]) || !validWorkspaceRelativePath(item.path) ||
      !["filename", "content", "filename_and_content"].includes(String(item.match_kind)) ||
      !safeBoundedCount(item.line, Number.MAX_SAFE_INTEGER) ||
      typeof item.snippet !== "string" || new TextEncoder().encode(item.snippet).length > 512 ||
      typeof item.content_truncated !== "boolean" || !hasExactKeys(item.provenance,
        ["content_sha256", "instruction_authorized", "source_kind", "source_ref", "version"]) ||
      item.provenance.version !== "context_provenance.v1" ||
      item.provenance.source_kind !== "workspace_file" ||
      item.provenance.source_ref !== item.path || !isSHA256(item.provenance.content_sha256) ||
      item.provenance.instruction_authorized !== false ||
      (item.match_kind === "filename" ? item.line !== 0 || item.snippet !== "" : item.line < 1)) {
      throw new APIRequestError("Workspace search result widened renderer or instruction authority",
        "INVALID_RESPONSE", 502);
    }
    return item;
  });
  return { ...value, results } as unknown as WorkspaceSearchView;
}

function parseRepositoryState(value: unknown, workspaceID: string): RepositoryStateView {
  const keys = ["available", "branch", "changes", "clean", "conflicted_count",
    "content_included", "detached", "head", "hooks_executed", "kind", "network_used",
    "process_started", "protocol_version", "read_only", "redaction_count",
    "remote_config_included", "root_path_exposed", "staged_count", "truncated",
    "untracked_count", "workspace_id", "worktree_count"];
  if (!hasExactKeys(value, keys) || value.protocol_version !== "repository_state.v1" ||
    value.workspace_id !== workspaceID || !["none", "git"].includes(String(value.kind)) ||
    typeof value.available !== "boolean" || typeof value.clean !== "boolean" ||
    typeof value.detached !== "boolean" || typeof value.truncated !== "boolean" ||
    value.read_only !== true || value.root_path_exposed !== false ||
    value.content_included !== false || value.remote_config_included !== false ||
    value.process_started !== false || value.network_used !== false ||
    value.hooks_executed !== false || typeof value.branch !== "string" ||
    value.branch.length > 255 || /[\u0000-\u001f\u007f]/u.test(value.branch) ||
    typeof value.head !== "string" || !/^(?:[0-9a-f]{12})?$/u.test(value.head) ||
    !safeBoundedCount(value.staged_count, 10_000) ||
    !safeBoundedCount(value.worktree_count, 10_000) ||
    !safeBoundedCount(value.untracked_count, 10_000) ||
    !safeBoundedCount(value.conflicted_count, 10_000) ||
    !safeBoundedCount(value.redaction_count, 10_000) || !Array.isArray(value.changes) ||
    value.changes.length > 200) {
    throw new APIRequestError("Repository state violated its read-only bounded contract",
      "INVALID_RESPONSE", 502);
  }
  const changes = value.changes.map((change) => {
    if (!hasExactKeys(change, ["path", "staging", "worktree"]) ||
      !validWorkspaceRelativePath(change.path) || change.path === "." ||
      !["unmodified", "untracked", "modified", "added", "deleted", "renamed", "copied",
        "conflicted"].includes(String(change.staging)) ||
      !["unmodified", "untracked", "modified", "added", "deleted", "renamed", "copied",
        "conflicted"].includes(String(change.worktree)) ||
      (change.staging === "unmodified" && change.worktree === "unmodified")) {
      throw new APIRequestError("Repository change widened path or status authority",
        "INVALID_RESPONSE", 502);
    }
    return change;
  });
  const total = value.staged_count + value.worktree_count + value.untracked_count;
  if ((value.available !== (value.kind === "git")) ||
    (!value.available && (value.clean || value.detached || value.branch !== "" ||
      value.head !== "" || changes.length !== 0 || total !== 0 ||
      value.conflicted_count !== 0 || value.redaction_count !== 0 || value.truncated)) ||
    (value.available && value.clean !== (total === 0)) ||
    value.conflicted_count > value.staged_count + value.worktree_count) {
    throw new APIRequestError("Repository state contains inconsistent status facts",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, changes } as unknown as RepositoryStateView;
}

function parseRepositoryDiff(value: unknown, workspaceID: string): RepositoryDiffView {
  const keys = ["authority_granted", "available", "base_head", "hooks_executed",
    "instruction_authorized", "items", "kind", "mutation_supported", "network_used",
    "omitted_count", "patch_content_included", "process_started", "protocol_version",
    "raw_content_included", "read_only", "redaction_count", "remote_config_included",
    "returned_count", "root_path_exposed", "total_patch_bytes", "truncated", "workspace_id"];
  if (!hasExactKeys(value, keys) || value.protocol_version !== "repository_diff.v1" ||
    value.workspace_id !== workspaceID || !["none", "git"].includes(String(value.kind)) ||
    typeof value.available !== "boolean" || typeof value.truncated !== "boolean" ||
    value.read_only !== true || value.instruction_authorized !== false ||
    value.mutation_supported !== false || value.authority_granted !== false ||
    value.root_path_exposed !== false || value.raw_content_included !== false ||
    value.patch_content_included !== value.available || value.remote_config_included !== false ||
    value.process_started !== false || value.network_used !== false || value.hooks_executed !== false ||
    typeof value.base_head !== "string" || !/^(?:[0-9a-f]{12})?$/u.test(value.base_head) ||
    !Array.isArray(value.items) || value.items.length > 50 ||
    !safeBoundedCount(value.returned_count, 50) || value.returned_count !== value.items.length ||
    !safeBoundedCount(value.omitted_count, 10_000) ||
    !safeBoundedCount(value.redaction_count, 10_000) ||
    !safeBoundedCount(value.total_patch_bytes, 512 * 1024) ||
    value.available !== (value.kind === "git")) {
    throw new APIRequestError("Repository diff violated its bounded read-only contract",
      "INVALID_RESPONSE", 502);
  }
  const status = ["unmodified", "untracked", "modified", "added", "deleted", "renamed",
    "copied", "conflicted"];
  const contentStates = ["text", "binary_or_unsupported", "size_limited", "linked", "unavailable"];
  let totalBytes = 0;
  let redactedItems = 0;
  const paths = new Set<string>();
  const items = value.items.map((item) => {
    if (!hasExactKeys(item, ["added_lines", "content_state", "deleted_lines", "patch",
      "patch_bytes", "path", "redacted", "staging", "truncated", "worktree"]) ||
      !validWorkspaceRelativePath(item.path) || item.path === "." || paths.has(String(item.path)) ||
      !status.includes(String(item.staging)) || !status.includes(String(item.worktree)) ||
      !contentStates.includes(String(item.content_state)) || typeof item.patch !== "string" ||
      !safeBoundedCount(item.patch_bytes, 64 * 1024) ||
      new TextEncoder().encode(item.patch).length !== item.patch_bytes ||
      !safeBoundedCount(item.added_lines, 64 * 1024) ||
      !safeBoundedCount(item.deleted_lines, 64 * 1024) ||
      typeof item.redacted !== "boolean" || typeof item.truncated !== "boolean" ||
      (item.content_state !== "text" &&
        (item.patch !== "" || item.patch_bytes !== 0 || item.added_lines !== 0 ||
          item.deleted_lines !== 0 || item.truncated))) {
      throw new APIRequestError("Repository diff item widened content or path authority",
        "INVALID_RESPONSE", 502);
    }
    paths.add(String(item.path));
    totalBytes += Number(item.patch_bytes);
    redactedItems += item.redacted ? 1 : 0;
    return item;
  });
  if (totalBytes !== value.total_patch_bytes || redactedItems > value.redaction_count ||
    ((items.some((item) => item.truncated) || value.omitted_count > 0) && !value.truncated) ||
    (!value.available && (items.length !== 0 || value.base_head !== "" ||
      value.total_patch_bytes !== 0 || value.omitted_count !== 0 || value.redaction_count !== 0))) {
    throw new APIRequestError("Repository diff contains inconsistent bounded facts",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, items } as unknown as RepositoryDiffView;
}

function parseRepositoryHistory(value: unknown, workspaceID: string): RepositoryHistoryView {
  const keys = ["author_identity_included", "available", "branches", "commit_body_included",
    "commits", "detached", "first_parent_only", "head", "hooks_executed", "kind",
    "network_used", "omitted_branch_count", "process_started", "protocol_version",
    "read_only", "redaction_count", "remote_config_included", "returned_branch_count",
    "returned_commit_count", "root_path_exposed", "truncated", "workspace_id"];
  if (!hasExactKeys(value, keys) || value.protocol_version !== "repository_history.v1" ||
    value.workspace_id !== workspaceID || !["none", "git"].includes(String(value.kind)) ||
    typeof value.available !== "boolean" || value.available !== (value.kind === "git") ||
    typeof value.detached !== "boolean" || typeof value.truncated !== "boolean" ||
    value.first_parent_only !== true || value.read_only !== true ||
    value.root_path_exposed !== false || value.author_identity_included !== false ||
    value.commit_body_included !== false || value.remote_config_included !== false ||
    value.process_started !== false || value.network_used !== false ||
    value.hooks_executed !== false || typeof value.head !== "string" ||
    !/^(?:[0-9a-f]{12})?$/u.test(value.head) || !Array.isArray(value.commits) ||
    !Array.isArray(value.branches) || value.commits.length > 50 || value.branches.length > 64 ||
    !safeBoundedCount(value.returned_commit_count, 50) ||
    value.returned_commit_count !== value.commits.length ||
    !safeBoundedCount(value.returned_branch_count, 64) ||
    value.returned_branch_count !== value.branches.length ||
    !safeBoundedCount(value.omitted_branch_count, 1024) ||
    !safeBoundedCount(value.redaction_count, 10_000)) {
    throw new APIRequestError("Repository history violated its bounded local contract",
      "INVALID_RESPONSE", 502);
  }
  const hashes = new Set<string>();
  const objectIDs = new Set<string>();
  const commits = value.commits.map((commit) => {
    if (!hasExactKeys(commit, ["committed_at", "hash", "object_id", "parent_count", "redacted",
      "subject", "subject_bounded"]) || typeof commit.hash !== "string" ||
      !/^[0-9a-f]{12}$/u.test(commit.hash) || hashes.has(commit.hash) ||
      typeof commit.object_id !== "string" || !/^[0-9a-f]{40}$/u.test(commit.object_id) ||
      !commit.object_id.startsWith(commit.hash) || objectIDs.has(commit.object_id) ||
      typeof commit.subject !== "string" || commit.subject.length === 0 ||
      [...commit.subject].length > 512 || /[\u0000-\u001f\u007f]/u.test(commit.subject) ||
      !safeBoundedCount(commit.parent_count, 10_000) || !validDate(commit.committed_at) ||
      typeof commit.redacted !== "boolean" || typeof commit.subject_bounded !== "boolean") {
      throw new APIRequestError("Repository commit widened history or privacy authority",
        "INVALID_RESPONSE", 502);
    }
    hashes.add(commit.hash);
    objectIDs.add(commit.object_id);
    return commit;
  });
  const branchNames = new Set<string>();
  let currentBranches = 0;
  const branches = value.branches.map((branch) => {
    if (!hasExactKeys(branch, ["current", "head", "name"]) ||
      typeof branch.name !== "string" || branch.name.length === 0 ||
      [...branch.name].length > 255 || /[\u0000-\u001f\u007f]/u.test(branch.name) ||
      branchNames.has(branch.name) || typeof branch.head !== "string" ||
      !/^[0-9a-f]{12}$/u.test(branch.head) || typeof branch.current !== "boolean") {
      throw new APIRequestError("Repository branch widened local metadata authority",
        "INVALID_RESPONSE", 502);
    }
    branchNames.add(branch.name);
    currentBranches += branch.current ? 1 : 0;
    return branch;
  });
  if (currentBranches > 1 ||
    (!value.available && (value.head !== "" || value.detached || commits.length !== 0 ||
      branches.length !== 0 || value.omitted_branch_count !== 0 ||
      value.redaction_count !== 0 || value.truncated)) ||
    (value.detached && currentBranches !== 0) ||
    (value.head === "" && commits.length !== 0)) {
    throw new APIRequestError("Repository history contains inconsistent local facts",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, commits, branches } as unknown as RepositoryHistoryView;
}

function parseRepositoryCommitDetail(value: unknown, workspaceID: string,
  objectID: string): RepositoryCommitDetailView {
  const keys = ["author_identity_included", "available", "changed_file_count", "changes",
    "checkout_performed", "commit_body_included", "committed_at", "file_content_included",
    "first_parent_only", "hash", "hooks_executed", "kind", "network_used", "object_id",
    "omitted_change_count", "parent_count", "patch_included", "process_started",
    "protocol_version", "read_only", "redaction_count", "reference_updated",
    "remote_config_included", "returned_change_count", "root_path_exposed", "subject",
    "truncated", "workspace_id"];
  if (!hasExactKeys(value, keys) || value.protocol_version !== "repository_commit_detail.v1" ||
    value.workspace_id !== workspaceID || value.object_id !== objectID ||
    !/^[0-9a-f]{40}$/u.test(String(value.object_id)) ||
    !["none", "git"].includes(String(value.kind)) ||
    typeof value.available !== "boolean" || value.available !== (value.kind === "git") ||
    value.first_parent_only !== true || value.read_only !== true ||
    value.root_path_exposed !== false || value.author_identity_included !== false ||
    value.commit_body_included !== false || value.file_content_included !== false ||
    value.patch_included !== false || value.remote_config_included !== false ||
    value.checkout_performed !== false || value.reference_updated !== false ||
    value.process_started !== false || value.network_used !== false ||
    value.hooks_executed !== false || typeof value.truncated !== "boolean" ||
    !Array.isArray(value.changes) || value.changes.length > 200 ||
    !safeBoundedCount(value.changed_file_count, 40_000) ||
    !safeBoundedCount(value.returned_change_count, 200) ||
    value.returned_change_count !== value.changes.length ||
    !safeBoundedCount(value.omitted_change_count, 40_000) ||
    value.changed_file_count !== value.returned_change_count + value.omitted_change_count ||
    !safeBoundedCount(value.redaction_count, 40_000) ||
    !safeBoundedCount(value.parent_count, 1024)) {
    throw new APIRequestError("Repository commit detail violated its exact read-only contract",
      "INVALID_RESPONSE", 502);
  }
  const paths = new Set<string>();
  const kinds = ["", "regular", "executable", "symlink", "submodule"];
  const changes = value.changes.map((change) => {
    if (!hasExactKeys(change, ["change", "content_changed", "current_kind", "mode_changed",
      "path", "previous_kind"]) || !validWorkspaceRelativePath(change.path) ||
      change.path === "." || paths.has(String(change.path)) ||
      !["added", "modified", "deleted"].includes(String(change.change)) ||
      !kinds.includes(String(change.previous_kind)) || !kinds.includes(String(change.current_kind)) ||
      typeof change.content_changed !== "boolean" || typeof change.mode_changed !== "boolean" ||
      (!change.content_changed && !change.mode_changed) ||
      (change.change === "added" && (change.previous_kind !== "" || change.current_kind === "")) ||
      (change.change === "deleted" && (change.previous_kind === "" || change.current_kind !== "")) ||
      (change.change === "modified" &&
        (change.previous_kind === "" || change.current_kind === ""))) {
      throw new APIRequestError("Repository commit file metadata widened content authority",
        "INVALID_RESPONSE", 502);
    }
    paths.add(String(change.path));
    return change;
  });
  if ((!value.available && (value.hash !== "" || value.subject !== "" ||
    value.committed_at !== "0001-01-01T00:00:00Z" || value.parent_count !== 0 ||
    value.changed_file_count !== 0 || value.omitted_change_count !== 0 ||
    value.redaction_count !== 0 || value.truncated)) ||
    (value.available && (typeof value.hash !== "string" || !/^[0-9a-f]{12}$/u.test(value.hash) ||
      !objectID.startsWith(value.hash) || typeof value.subject !== "string" ||
      value.subject.length === 0 || [...value.subject].length > 512 ||
      /[\u0000-\u001f\u007f]/u.test(value.subject) || !validDate(value.committed_at))) ||
    (value.omitted_change_count > 0 && !value.truncated)) {
    throw new APIRequestError("Repository commit detail contains inconsistent bounded facts",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, changes } as unknown as RepositoryCommitDetailView;
}

function validVerificationText(value: unknown, maximum: number, multiline: boolean): value is string {
  if (typeof value !== "string" || value === "" || value.trim() !== value ||
    [...value].length > maximum || value.includes("\0")) {
    return false;
  }
  return multiline ? !/[\u0001-\u0008\u000b-\u001f\u007f]/u.test(value) :
    !/[\u0000-\u001f\u007f]/u.test(value);
}

function parseVerificationEvidenceItem(value: unknown, runID: string,
  sessionID = "", workspaceID = "", control = false): VerificationEvidenceControlView {
  const keys = ["approval", "authority_granted", "command_executed", "id", "immutable",
    "model_assertion", "operator_supplied", "outcome", "protocol_version", "recorded_at",
    "redacted", "run_id", "session_id", "summary", "summary_sha256", "title", "workspace_id"];
  if (control) keys.push("replayed");
  if (!hasExactKeys(value, keys) || value.protocol_version !== "operator_verification_evidence.v1" ||
    value.run_id !== runID || !boundedIdentity(value.id) || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.workspace_id) || (sessionID !== "" && value.session_id !== sessionID) ||
    (workspaceID !== "" && value.workspace_id !== workspaceID) ||
    !["pass", "fail", "unknown"].includes(String(value.outcome)) ||
    !validVerificationText(value.title, 160, false) ||
    !validVerificationText(value.summary, 2048, true) || !isSHA256(value.summary_sha256) ||
    typeof value.redacted !== "boolean" || !validDate(value.recorded_at) ||
    value.immutable !== true || value.operator_supplied !== true ||
    value.command_executed !== false || value.model_assertion !== false ||
    value.approval !== false || value.authority_granted !== false ||
    (control && typeof value.replayed !== "boolean")) {
    throw new APIRequestError("Verification evidence widened observation authority",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as VerificationEvidenceControlView;
}

function parseVerificationEvidenceInventory(value: unknown,
  runID: string): VerificationEvidenceInventoryView {
  if (!hasExactKeys(value, ["fail_count", "items", "pass_count", "protocol_version", "run_id",
    "session_id", "truncated", "unknown_count", "workspace_id"]) ||
    value.protocol_version !== "operator_verification_inventory.v1" || value.run_id !== runID ||
    !boundedIdentity(value.session_id) || !boundedIdentity(value.workspace_id) ||
    !Array.isArray(value.items) || value.items.length > 100 ||
    !safeBoundedCount(value.pass_count, 100) || !safeBoundedCount(value.fail_count, 100) ||
    !safeBoundedCount(value.unknown_count, 100) || typeof value.truncated !== "boolean" ||
    value.pass_count + value.fail_count + value.unknown_count !== value.items.length) {
    throw new APIRequestError("Verification inventory violated its immutable bounded contract",
      "INVALID_RESPONSE", 502);
  }
  const ids = new Set<string>();
  const items = value.items.map((item) => {
    const parsed = parseVerificationEvidenceItem(item, runID, String(value.session_id),
      String(value.workspace_id));
    if (ids.has(parsed.id)) {
      throw new APIRequestError("Verification inventory repeated an immutable identity",
        "INVALID_RESPONSE", 502);
    }
    ids.add(parsed.id);
    return parsed;
  });
  if (value.truncated && items.length !== 100) {
    throw new APIRequestError("Verification inventory truncation is inconsistent",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, items } as unknown as VerificationEvidenceInventoryView;
}

function parseVerificationPlan(value: unknown, runID: string, sessionID = "",
  workspaceID = "", control = false): VerificationPlanControlView {
  const keys = ["approval", "authority_granted", "command_executed", "created_at",
    "guidance_only", "id", "immutable", "item_count", "items", "model_assertion",
    "operator_supplied", "plan_sha256", "protocol_version", "redacted", "result_inferred",
    "run_id", "session_id", "summary", "title", "workspace_id"];
  if (control) keys.push("replayed");
  if (!hasExactKeys(value, keys) || value.protocol_version !== "operator_verification_plan.v1" ||
    value.run_id !== runID || !boundedIdentity(value.id) || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.workspace_id) || (sessionID !== "" && value.session_id !== sessionID) ||
    (workspaceID !== "" && value.workspace_id !== workspaceID) ||
    !validVerificationText(value.title, 160, false) ||
    !validVerificationText(value.summary, 2048, true) || !isSHA256(value.plan_sha256) ||
    typeof value.redacted !== "boolean" || !validDate(value.created_at) ||
    !Array.isArray(value.items) || value.items.length < 1 || value.items.length > 32 ||
    value.item_count !== value.items.length || value.immutable !== true ||
    value.operator_supplied !== true || value.guidance_only !== true ||
    value.command_executed !== false || value.model_assertion !== false ||
    value.result_inferred !== false || value.approval !== false ||
    value.authority_granted !== false || (control && typeof value.replayed !== "boolean")) {
    throw new APIRequestError("Verification plan widened guidance or result authority",
      "INVALID_RESPONSE", 502);
  }
  let itemRedacted = false;
  const items = value.items.map((item, index) => {
    if (!hasExactKeys(item, ["expected_observation", "item_sha256", "ordinal", "redacted",
      "title"]) || item.ordinal !== index + 1 ||
      !validVerificationText(item.title, 160, false) ||
      !validVerificationText(item.expected_observation, 1024, true) ||
      !isSHA256(item.item_sha256) || typeof item.redacted !== "boolean") {
      throw new APIRequestError("Verification plan item violated its bounded checklist contract",
        "INVALID_RESPONSE", 502);
    }
    itemRedacted = itemRedacted || item.redacted;
    return item;
  });
  if (itemRedacted && !value.redacted) {
    throw new APIRequestError("Verification plan redaction state is inconsistent",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, items } as unknown as VerificationPlanControlView;
}

function parseVerificationPlanInventory(value: unknown,
  runID: string): VerificationPlanInventoryView {
  if (!hasExactKeys(value, ["items", "protocol_version", "run_id", "session_id", "truncated",
    "workspace_id"]) || value.protocol_version !== "operator_verification_plan_inventory.v1" ||
    value.run_id !== runID || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.workspace_id) || !Array.isArray(value.items) ||
    value.items.length > 50 || typeof value.truncated !== "boolean") {
    throw new APIRequestError("Verification plan inventory violated its immutable bounded contract",
      "INVALID_RESPONSE", 502);
  }
  const identities = new Set<string>();
  const items = value.items.map((item) => {
    const parsed = parseVerificationPlan(item, runID, String(value.session_id),
      String(value.workspace_id));
    if (identities.has(parsed.id)) {
      throw new APIRequestError("Verification plan inventory repeated an immutable identity",
        "INVALID_RESPONSE", 502);
    }
    identities.add(parsed.id);
    return parsed;
  });
  if (value.truncated && items.length !== 50) {
	throw new APIRequestError("Verification plan inventory truncation is inconsistent",
	  "INVALID_RESPONSE", 502);
  }
  return { ...value, items } as unknown as VerificationPlanInventoryView;
}

function parseVerificationAssociation(value: unknown, runID: string,
  control = false): VerificationAssociationControlView {
  const keys = ["approval", "associated_at", "association_event_sequence",
    "authority_granted", "command_executed", "evidence_event_sequence", "evidence_id",
    "evidence_outcome", "id", "immutable", "metadata_only", "model_assertion",
    "operator_supplied", "plan_id", "plan_item_ordinal", "plan_item_sha256",
    "protocol_version", "record_rewritten", "result_inferred", "run_id", "session_id",
    "workspace_id"];
  if (control) keys.push("replayed");
  if (!hasExactKeys(value, keys) ||
    value.protocol_version !== "operator_verification_plan_evidence_association.v1" ||
    value.run_id !== runID || !boundedIdentity(value.id) || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.workspace_id) || !boundedIdentity(value.plan_id) ||
    !safePositiveInteger(value.plan_item_ordinal) || value.plan_item_ordinal > 32 ||
    !isSHA256(value.plan_item_sha256) || !boundedIdentity(value.evidence_id) ||
    !["pass", "fail", "unknown"].includes(String(value.evidence_outcome)) ||
    !safePositiveInteger(value.evidence_event_sequence) ||
    !safePositiveInteger(value.association_event_sequence) ||
    value.association_event_sequence <= value.evidence_event_sequence ||
    !validDate(value.associated_at) || value.immutable !== true ||
    value.operator_supplied !== true || value.metadata_only !== true ||
    value.command_executed !== false || value.model_assertion !== false ||
    value.result_inferred !== false || value.record_rewritten !== false ||
    value.approval !== false || value.authority_granted !== false ||
    (control && typeof value.replayed !== "boolean")) {
    throw new APIRequestError("Verification association widened result or mutation authority",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as VerificationAssociationControlView;
}

function parseVerificationPlanCoverage(value: unknown,
  runID: string): VerificationPlanCoverageInventoryView {
  const keys = ["approval", "associated_evidence_count", "associations",
    "associations_truncated", "authority_granted", "command_executed", "metadata_only",
    "model_assertion", "observed_plan_item_count", "plan_count", "plan_item_count", "plans",
    "plans_truncated", "protocol_version", "read_only", "record_rewritten",
    "result_inferred", "run_id", "session_id", "workspace_id"];
  if (!hasExactKeys(value, keys) ||
    value.protocol_version !== "operator_verification_plan_coverage.v1" ||
    value.run_id !== runID || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.workspace_id) || !Array.isArray(value.plans) ||
    value.plans.length > 50 || value.plan_count !== value.plans.length ||
    !safeBoundedCount(value.plan_item_count, 1600) ||
    !safeBoundedCount(value.observed_plan_item_count, 1600) ||
    value.observed_plan_item_count > value.plan_item_count ||
    !safeBoundedCount(value.associated_evidence_count, 1_000_000_000) ||
    !Array.isArray(value.associations) || value.associations.length > 100 ||
    typeof value.plans_truncated !== "boolean" ||
    typeof value.associations_truncated !== "boolean" || value.metadata_only !== true ||
    value.read_only !== true || value.result_inferred !== false ||
    value.command_executed !== false || value.model_assertion !== false ||
    value.record_rewritten !== false || value.approval !== false ||
    value.authority_granted !== false) {
    throw new APIRequestError("Verification coverage widened metadata-only authority",
      "INVALID_RESPONSE", 502);
  }
  const planIDs = new Set<string>();
  const planItems = new Map<string, Map<number, string>>();
  let itemTotal = 0;
  let observedTotal = 0;
  let associationTotal = 0;
  const plans = value.plans.map((plan) => {
    if (!hasExactKeys(plan, ["associated_evidence_count", "item_count", "items",
      "observed_item_count", "plan_id", "plan_sha256"]) ||
      !boundedIdentity(plan.plan_id) || planIDs.has(String(plan.plan_id)) ||
      !isSHA256(plan.plan_sha256) || !Array.isArray(plan.items) ||
      plan.items.length < 1 || plan.items.length > 32 || plan.item_count !== plan.items.length ||
      !safeBoundedCount(plan.observed_item_count, 32) ||
      !safeBoundedCount(plan.associated_evidence_count, 1_000_000_000)) {
      throw new APIRequestError("Verification coverage plan metadata is invalid",
        "INVALID_RESPONSE", 502);
    }
    const ordinals = new Map<number, string>();
    let observed = 0;
    let associated = 0;
    const items = plan.items.map((item, index) => {
      if (!hasExactKeys(item, ["associated_evidence_count", "fail_count", "item_sha256",
        "latest_association_event_sequence", "ordinal", "pass_count", "unknown_count"]) ||
        item.ordinal !== index + 1 || !isSHA256(item.item_sha256) ||
        !safeBoundedCount(item.associated_evidence_count, 1_000_000_000) ||
        !safeBoundedCount(item.pass_count, 1_000_000_000) ||
        !safeBoundedCount(item.fail_count, 1_000_000_000) ||
        !safeBoundedCount(item.unknown_count, 1_000_000_000) ||
        item.pass_count + item.fail_count + item.unknown_count !==
          item.associated_evidence_count ||
        !safeBoundedCount(item.latest_association_event_sequence, Number.MAX_SAFE_INTEGER) ||
        ((item.associated_evidence_count === 0) !==
          (item.latest_association_event_sequence === 0))) {
        throw new APIRequestError("Verification coverage item inferred a non-explicit result",
          "INVALID_RESPONSE", 502);
      }
      if (item.associated_evidence_count > 0) observed += 1;
      associated += Number(item.associated_evidence_count);
      ordinals.set(Number(item.ordinal), String(item.item_sha256));
      return item;
    });
    if (plan.observed_item_count !== observed || plan.associated_evidence_count !== associated) {
      throw new APIRequestError("Verification coverage plan counts are inconsistent",
        "INVALID_RESPONSE", 502);
    }
    planIDs.add(String(plan.plan_id));
    planItems.set(String(plan.plan_id), ordinals);
    itemTotal += Number(plan.item_count);
    observedTotal += observed;
    associationTotal += associated;
    return { ...plan, items };
  });
  const associationIDs = new Set<string>();
  const evidenceIDs = new Set<string>();
  const associations = value.associations.map((association) => {
    if (!hasExactKeys(association, ["associated_at", "association_event_sequence",
      "evidence_event_sequence", "evidence_id", "evidence_outcome", "id", "plan_id",
      "plan_item_ordinal", "plan_item_sha256"]) || !boundedIdentity(association.id) ||
      associationIDs.has(String(association.id)) || !boundedIdentity(association.plan_id) ||
      !safePositiveInteger(association.plan_item_ordinal) || association.plan_item_ordinal > 32 ||
      !isSHA256(association.plan_item_sha256) || !boundedIdentity(association.evidence_id) ||
      evidenceIDs.has(String(association.evidence_id)) ||
      !["pass", "fail", "unknown"].includes(String(association.evidence_outcome)) ||
      !safePositiveInteger(association.evidence_event_sequence) ||
      !safePositiveInteger(association.association_event_sequence) ||
      association.association_event_sequence <= association.evidence_event_sequence ||
      !validDate(association.associated_at)) {
      throw new APIRequestError("Verification coverage association metadata is invalid",
        "INVALID_RESPONSE", 502);
    }
    const knownItems = planItems.get(String(association.plan_id));
    if (knownItems && knownItems.get(Number(association.plan_item_ordinal)) !==
      association.plan_item_sha256) {
      throw new APIRequestError("Verification association escaped its exact plan item",
        "INVALID_RESPONSE", 502);
    }
    associationIDs.add(String(association.id));
    evidenceIDs.add(String(association.evidence_id));
    return association;
  });
  if (itemTotal !== value.plan_item_count || observedTotal !== value.observed_plan_item_count ||
    associationTotal !== value.associated_evidence_count ||
    (value.plans_truncated && plans.length !== 50) ||
    (value.associations_truncated && associations.length !== 100)) {
    throw new APIRequestError("Verification coverage aggregate is inconsistent",
      "INVALID_RESPONSE", 502);
  }
  return { ...value, plans, associations } as unknown as VerificationPlanCoverageInventoryView;
}

function parseCodeHandoff(value: unknown, runID: string): CodeHandoffView {
  const keys = ["change_set", "composite_mutation", "durable_sources", "execution_started",
    "generated_at", "mission_id", "mode_revision", "pending_action_count", "pending_actions",
    "pending_actions_truncated", "phase", "plan", "private_bodies_included", "protocol_version",
    "queue", "regenerable", "report_references", "report_references_truncated",
    "resume_authorized", "run_id", "run_status", "session_id", "source_event_sequence",
    "surface", "verification", "verification_plans", "workspace_id"];
  if (!hasExactKeys(value, keys) || value.protocol_version !== "code_handoff.v1" ||
    value.run_id !== runID || !boundedIdentity(value.mission_id) || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.workspace_id) || value.surface !== "code" ||
    !["plan", "deliver"].includes(String(value.phase)) ||
    !["created", "preparing", "running", "waiting_approval", "paused", "completed", "failed",
      "cancelled"].includes(String(value.run_status)) || !safePositiveInteger(value.mode_revision) ||
    !safePositiveInteger(value.source_event_sequence) || !validDate(value.generated_at) ||
    value.regenerable !== true || value.durable_sources !== true ||
    value.private_bodies_included !== false || value.composite_mutation !== false ||
    value.resume_authorized !== false || value.execution_started !== false ||
    !isRecord(value.plan) || !isRecord(value.queue) || !isRecord(value.change_set) ||
    !isRecord(value.verification) || !isRecord(value.verification_plans) ||
    !Array.isArray(value.pending_actions) ||
    value.pending_actions.length > 20 || !Array.isArray(value.report_references) ||
    value.report_references.length > 20 || !safeBoundedCount(value.pending_action_count, 100) ||
    typeof value.pending_actions_truncated !== "boolean" ||
    typeof value.report_references_truncated !== "boolean") {
    throw new APIRequestError("Code handoff violated its metadata-only boundary",
      "INVALID_RESPONSE", 502);
  }
  const planKeys = ["blocked_count", "cancelled_count", "completed_count", "direction_count",
    "in_progress_count", "module_count", "pending_count", "proposal_id", "selected_direction",
    "selection_id", "state"];
  const plan = value.plan;
  const planCounts = [plan.pending_count, plan.in_progress_count, plan.blocked_count,
    plan.completed_count, plan.cancelled_count];
  if (!hasExactKeys(plan, planKeys) || !["none", "proposed", "selected"].includes(String(plan.state)) ||
    typeof plan.proposal_id !== "string" || typeof plan.selection_id !== "string" ||
    !safeBoundedCount(plan.direction_count, 3) || !safeBoundedCount(plan.selected_direction, 3) ||
    !safeBoundedCount(plan.module_count, 8) ||
    planCounts.some((count) => !safeBoundedCount(count, 8)) ||
    planCounts.reduce<number>((total, count) => total + Number(count), 0) !== plan.module_count ||
    (plan.state === "none" && (plan.proposal_id !== "" || plan.selection_id !== "" ||
      plan.direction_count !== 0 || plan.selected_direction !== 0 || plan.module_count !== 0)) ||
    (plan.state === "proposed" && (!boundedIdentity(plan.proposal_id) || plan.selection_id !== "" ||
      plan.direction_count !== 3 || plan.selected_direction !== 0 || plan.module_count !== 0)) ||
    (plan.state === "selected" && (!boundedIdentity(plan.proposal_id) ||
      !boundedIdentity(plan.selection_id) || plan.direction_count !== 3 ||
      plan.selected_direction < 1))) {
    throw new APIRequestError("Code handoff Plan summary is inconsistent", "INVALID_RESPONSE", 502);
  }
  const queue = value.queue;
  if (!hasExactKeys(queue, ["cancelled", "committed", "pending", "prepared"]) ||
    [queue.pending, queue.prepared, queue.committed, queue.cancelled]
      .some((count) => !safeBoundedCount(count, Number.MAX_SAFE_INTEGER))) {
    throw new APIRequestError("Code handoff queue summary is invalid", "INVALID_RESPONSE", 502);
  }
  const changeSet = value.change_set;
  const changeCounts = [changeSet.proposed, changeSet.approved, changeSet.applied,
    changeSet.denied, changeSet.failed];
  if (!hasExactKeys(changeSet, ["applied", "approved", "denied", "failed", "proposed",
    "returned_count", "total_diff_bytes", "truncated"]) ||
    changeCounts.some((count) => !safeBoundedCount(count, 100)) ||
    !safeBoundedCount(changeSet.returned_count, 100) ||
    changeCounts.reduce<number>((total, count) => total + Number(count), 0) !==
      changeSet.returned_count ||
    !safeBoundedCount(changeSet.total_diff_bytes, 110 * 1024 * 1024) ||
    typeof changeSet.truncated !== "boolean") {
    throw new APIRequestError("Code handoff change-set summary is invalid", "INVALID_RESPONSE", 502);
  }
  const verification = value.verification;
  if (!hasExactKeys(verification, ["fail_count", "pass_count", "references", "returned_count",
    "truncated", "unknown_count"]) || !Array.isArray(verification.references) ||
    verification.references.length > 20 || !safeBoundedCount(verification.returned_count, 100) ||
    !safeBoundedCount(verification.pass_count, 100) || !safeBoundedCount(verification.fail_count, 100) ||
    !safeBoundedCount(verification.unknown_count, 100) ||
    verification.pass_count + verification.fail_count + verification.unknown_count !==
      verification.returned_count || typeof verification.truncated !== "boolean" ||
    verification.references.length !== Math.min(Number(verification.returned_count), 20) ||
    verification.truncated !== (verification.returned_count > 20)) {
    throw new APIRequestError("Code handoff verification summary is invalid", "INVALID_RESPONSE", 502);
  }
  const verificationIDs = new Set<string>();
  for (const reference of verification.references) {
    if (!hasExactKeys(reference, ["id", "outcome", "recorded_at", "redacted"]) ||
      !boundedIdentity(reference.id) || verificationIDs.has(String(reference.id)) ||
      !["pass", "fail", "unknown"].includes(String(reference.outcome)) ||
      typeof reference.redacted !== "boolean" || !validDate(reference.recorded_at)) {
      throw new APIRequestError("Code handoff verification reference is invalid",
        "INVALID_RESPONSE", 502);
    }
    verificationIDs.add(String(reference.id));
  }
  const verificationPlans = value.verification_plans;
  if (!hasExactKeys(verificationPlans, ["references", "returned_count", "truncated"]) ||
    !Array.isArray(verificationPlans.references) || verificationPlans.references.length > 20 ||
    !safeBoundedCount(verificationPlans.returned_count, 50) ||
    typeof verificationPlans.truncated !== "boolean" ||
    verificationPlans.references.length !== Math.min(Number(verificationPlans.returned_count), 20) ||
    verificationPlans.truncated !== (verificationPlans.returned_count > 20)) {
    throw new APIRequestError("Code handoff verification plan summary is invalid",
      "INVALID_RESPONSE", 502);
  }
  const verificationPlanIDs = new Set<string>();
  for (const reference of verificationPlans.references) {
    if (!hasExactKeys(reference, ["created_at", "id", "item_count", "plan_sha256", "redacted"]) ||
      !boundedIdentity(reference.id) || verificationPlanIDs.has(String(reference.id)) ||
      !isSHA256(reference.plan_sha256) || !safeBoundedCount(reference.item_count, 32) ||
      reference.item_count < 1 || typeof reference.redacted !== "boolean" ||
      !validDate(reference.created_at)) {
      throw new APIRequestError("Code handoff verification plan reference is invalid",
        "INVALID_RESPONSE", 502);
    }
    verificationPlanIDs.add(String(reference.id));
  }
  const actionMapping = {
    steering_pending: ["pending", "queue"], approval_pending: ["pending", "approvals"],
    file_edit_review: ["proposed", "diffs"], file_edit_apply: ["approved", "diffs"],
    wake_due: ["queued", "wake"],
  } as const;
  if (value.pending_actions.length !== Math.min(Number(value.pending_action_count), 20) ||
    value.pending_actions_truncated !== (value.pending_action_count > 20) ||
    (value.report_references_truncated && value.report_references.length !== 20)) {
    throw new APIRequestError("Code handoff reference summary is invalid", "INVALID_RESPONSE", 502);
  }
  const actionIDs = new Set<string>();
  for (const action of value.pending_actions) {
    const hasDueAt = Object.prototype.hasOwnProperty.call(action, "due_at");
    if (!hasExactKeys(action, hasDueAt ?
      ["available_at", "destination", "due_at", "id", "kind", "state"] :
      ["available_at", "destination", "id", "kind", "state"]) ||
      !boundedIdentity(action.id) || !String(action.id).startsWith("action-") ||
      actionIDs.has(String(action.id)) ||
      !Object.prototype.hasOwnProperty.call(actionMapping, String(action.kind)) ||
      !validDate(action.available_at)) {
      throw new APIRequestError("Code handoff action reference is invalid",
        "INVALID_RESPONSE", 502);
    }
    const expected = actionMapping[action.kind as keyof typeof actionMapping];
    if (action.state !== expected[0] || action.destination !== expected[1] ||
      (action.kind === "wake_due" ? !validDate(action.due_at) : hasDueAt)) {
      throw new APIRequestError("Code handoff action reference widened navigation",
        "INVALID_RESPONSE", 502);
    }
    actionIDs.add(String(action.id));
  }
  const reportIDs = new Set<string>();
  for (const report of value.report_references) {
    if (!hasExactKeys(report, ["created_at", "finding_count", "id", "status"]) ||
      !boundedIdentity(report.id) || reportIDs.has(String(report.id)) ||
      report.status !== "generated" || !safeBoundedCount(report.finding_count, 10_000) ||
      !validDate(report.created_at)) {
      throw new APIRequestError("Code handoff report reference is invalid",
        "INVALID_RESPONSE", 502);
    }
    reportIDs.add(String(report.id));
  }
  return value as unknown as CodeHandoffView;
}

async function parseCodeHandoffExport(value: unknown, runID: string,
  format: "json" | "markdown"): Promise<CodeHandoffExportView> {
  const keys = ["content", "content_bytes", "content_sha256", "download_only",
    "execution_started", "filename", "format", "generated_at", "mime_type",
    "mutation_supported", "private_bodies", "protocol_version", "read_only",
    "report_acceptance", "resume_authorized", "run_id", "source_event_sequence"];
  if (!hasExactKeys(value, keys) || value.protocol_version !== "code_handoff_export.v1" ||
    value.run_id !== runID || value.format !== format || !validDate(value.generated_at) ||
    !safePositiveInteger(value.source_event_sequence) || typeof value.filename !== "string" ||
    value.filename.length < 1 || value.filename.length > 255 || /[\\/:*?"<>|\u0000-\u001f]/u.test(value.filename) ||
    typeof value.mime_type !== "string" || typeof value.content !== "string" ||
    !safePositiveInteger(value.content_bytes) || value.content_bytes > 256 * 1024 ||
    !isSHA256(value.content_sha256) || value.read_only !== true || value.download_only !== true ||
    value.private_bodies !== false || value.resume_authorized !== false ||
    value.mutation_supported !== false || value.report_acceptance !== false ||
    value.execution_started !== false) {
    throw new APIRequestError("Code handoff export violated its download-only boundary",
      "INVALID_RESPONSE", 502);
  }
  const expectedMIME = format === "json" ? "application/json" : "text/markdown; charset=utf-8";
  const expectedSuffix = format === "json" ? ".json" : ".md";
  const encoded = new TextEncoder().encode(value.content);
  if (value.mime_type !== expectedMIME || !value.filename.endsWith(expectedSuffix) ||
    encoded.length !== value.content_bytes) {
    throw new APIRequestError("Code handoff export metadata does not match its content",
      "INVALID_RESPONSE", 502);
  }
  const digest = new Uint8Array(await globalThis.crypto.subtle.digest("SHA-256", encoded));
  const digestHex = [...digest].map((byte) => byte.toString(16).padStart(2, "0")).join("");
  if (digestHex !== value.content_sha256) {
    throw new APIRequestError("Code handoff export digest verification failed",
      "INVALID_RESPONSE", 502);
  }
  if (format === "json") {
    let document: unknown;
    try {
      document = JSON.parse(value.content);
    } catch {
      throw new APIRequestError("Code handoff JSON export is invalid", "INVALID_RESPONSE", 502);
    }
    if (!isRecord(document) || document.protocol_version !== "code_handoff.v1" ||
      document.run_id !== runID || document.source_event_sequence !== value.source_event_sequence) {
      throw new APIRequestError("Code handoff JSON export escaped its source binding",
        "INVALID_RESPONSE", 502);
    }
  } else if (!value.content.startsWith("# CyberAgent Code Handoff\n") ||
    !value.content.includes(`Source event high-water: \`${value.source_event_sequence}\``)) {
    throw new APIRequestError("Code handoff Markdown export omitted its source binding",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as CodeHandoffExportView;
}

function parseEvidenceAttachment(value: unknown, runID: string,
  request: EvidenceAttachmentRequestView): EvidenceAttachmentView {
  if (!hasExactKeys(value, ["attachment_id", "capability_grant", "content_sha256",
    "execution_started", "instruction_authorized", "model_called", "protocol_version",
    "replayed", "run_id", "session_id", "session_message_id", "source_kind", "source_ref",
    "tool_called", "workspace_id"]) ||
    value.protocol_version !== "session_evidence_attachment.v1" || value.run_id !== runID ||
    value.source_kind !== "workspace_file" || value.source_kind !== request.source_kind ||
    value.source_ref !== request.source_ref || value.content_sha256 !== request.content_sha256 ||
    !boundedIdentity(value.attachment_id) || !boundedIdentity(value.session_id) ||
    !boundedIdentity(value.workspace_id) || !safePositiveInteger(value.session_message_id) ||
    value.instruction_authorized !== false || typeof value.replayed !== "boolean" ||
    value.execution_started !== false || value.model_called !== false ||
    value.tool_called !== false || value.capability_grant !== false) {
    throw new APIRequestError("Evidence attachment response widened document authority",
      "INVALID_RESPONSE", 502);
  }
  return value as unknown as EvidenceAttachmentView;
}

function parseEvidenceInventory(value: unknown, runID: string): EvidenceInventoryView {
  if (!hasExactKeys(value, ["items", "protocol_version", "run_id", "truncated"]) ||
    value.protocol_version !== "session_evidence_inventory.v1" || value.run_id !== runID ||
    !Array.isArray(value.items) || value.items.length > 100 ||
    typeof value.truncated !== "boolean") {
    throw new APIRequestError("Evidence inventory response violated its metadata-only contract",
      "INVALID_RESPONSE", 502);
  }
  const identities = new Set<string>();
  const items = value.items.map((item) => {
    if (!hasExactKeys(item, ["attached_at", "attachment_id", "content_sha256",
      "instruction_authorized", "run_id", "session_id", "source_kind", "source_ref",
      "workspace_id"]) || !boundedIdentity(item.attachment_id) ||
      identities.has(String(item.attachment_id)) || item.run_id !== runID ||
      !boundedIdentity(item.session_id) || !boundedIdentity(item.workspace_id) ||
      item.source_kind !== "workspace_file" || !validWorkspaceRelativePath(item.source_ref) ||
      !isSHA256(item.content_sha256) || item.instruction_authorized !== false ||
      !validDate(item.attached_at)) {
      throw new APIRequestError("Evidence inventory item widened document or renderer authority",
        "INVALID_RESPONSE", 502);
    }
    identities.add(String(item.attachment_id));
    return item;
  });
  return { ...value, items } as unknown as EvidenceInventoryView;
}

function parseOperatorActionCenter(value: unknown, runID: string): OperatorActionCenterView {
  if (!hasExactKeys(value, ["generated_at", "items", "protocol_version", "run_id",
    "truncated"]) || value.protocol_version !== "operator_action_center.v1" ||
    value.run_id !== runID || !validDate(value.generated_at) || !Array.isArray(value.items) ||
    value.items.length > 100 || typeof value.truncated !== "boolean") {
    throw new APIRequestError("Operator action center response is invalid", "INVALID_RESPONSE", 502);
  }
  const mapping = {
    steering_pending: ["pending", "queue"],
    approval_pending: ["pending", "approvals"],
    file_edit_review: ["proposed", "diffs"],
    file_edit_apply: ["approved", "diffs"],
    wake_due: ["queued", "wake"],
  } as const;
  const identities = new Set<string>();
  const generatedAt = Date.parse(value.generated_at);
  const items = value.items.map((item) => {
    if (!isRecord(item) || !hasOnlyKeys(item, ["available_at", "destination", "due_at", "id",
      "kind", "state"]) || !boundedIdentity(item.id) || !String(item.id).startsWith("action-") ||
      identities.has(String(item.id)) || !validDate(item.available_at) ||
      !Object.prototype.hasOwnProperty.call(mapping, String(item.kind))) {
      throw new APIRequestError("Operator action item exposed invalid metadata",
        "INVALID_RESPONSE", 502);
    }
    const expected = mapping[item.kind as keyof typeof mapping];
    const dueAt = item.due_at;
    if (item.state !== expected[0] || item.destination !== expected[1] ||
      (item.kind === "wake_due"
        ? !validDate(dueAt) || Date.parse(dueAt) > generatedAt
        : dueAt !== undefined)) {
      throw new APIRequestError("Operator action item widened its closed navigation contract",
        "INVALID_RESPONSE", 502);
    }
    identities.add(String(item.id));
    return item;
  });
  return { ...value, items } as unknown as OperatorActionCenterView;
}

function parseOperationReceiptHistory(value: unknown,
  expectedRunID: string): OperationReceiptHistoryView {
  if (!hasExactKeys(value, ["items", "protocol_version", "truncated"]) ||
    value.protocol_version !== "operation_receipt_history.v1" || !Array.isArray(value.items) ||
    value.items.length > 100 || typeof value.truncated !== "boolean") {
    throw new APIRequestError("Operation receipt history response is invalid",
      "INVALID_RESPONSE", 502);
  }
  const identities = new Set<string>();
  const items = value.items.map((item) => {
    if (!isRecord(item) || !hasOnlyKeys(item, ["completed_at", "id", "receipt", "run_id", "scope"]) ||
      !boundedIdentity(item.id) || identities.has(String(item.id)) || !validDate(item.completed_at) ||
      (item.scope !== "run" && item.scope !== "skill_registry") ||
      (item.scope === "run" && (!boundedIdentity(item.run_id) ||
        (expectedRunID !== "" && item.run_id !== expectedRunID))) ||
      (item.scope === "skill_registry" && item.run_id !== undefined)) {
      throw new APIRequestError("Operation receipt history item exposed invalid scope metadata",
        "INVALID_RESPONSE", 502);
    }
    const receiptValue = item.receipt;
    if (!isRecord(receiptValue) || typeof receiptValue.kind !== "string" ||
      typeof receiptValue.outcome !== "string") {
      throw new APIRequestError("Operation receipt history item omitted its durable receipt",
        "INVALID_RESPONSE", 502);
    }
    const validTerminalOutcome =
      (receiptValue.kind === "file_edit_apply" &&
        (receiptValue.outcome === "applied" || receiptValue.outcome === "failed")) ||
      (receiptValue.kind === "run_wake_consume" &&
        (receiptValue.outcome === "completed" || receiptValue.outcome === "failed")) ||
      (receiptValue.kind === "skill_package_install" && receiptValue.outcome === "installed");
    if (!validTerminalOutcome) {
      throw new APIRequestError("Operation receipt history contains an unsupported terminal result",
        "INVALID_RESPONSE", 502);
    }
    const receipt = parseOperationReceipt(receiptValue,
      receiptValue.kind as OperationReceiptView["kind"],
      receiptValue.outcome as OperationReceiptView["outcome"], false);
    if ((item.scope === "skill_registry") !== (receipt.kind === "skill_package_install")) {
      throw new APIRequestError("Operation receipt history scope and kind diverged",
        "INVALID_RESPONSE", 502);
    }
    identities.add(String(item.id));
    return { ...item, receipt };
  });
  return { ...value, items } as unknown as OperationReceiptHistoryView;
}

function validWorkspaceRelativePath(value: unknown): value is string {
  if (typeof value !== "string" || value.length === 0 || Array.from(value).length > 512 ||
    value.trim() !== value || value.startsWith("/") || value.includes("\\") ||
    value.includes(":") || /[\u0000-\u001f\u007f]/u.test(value)) {
    return false;
  }
  if (value === ".") return true;
  return value.split("/").every((part) => part !== "" && part !== "." && part !== "..");
}

function validWorkspaceEntryName(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && Array.from(value).length <= 255 &&
    value.trim() === value && !value.includes("/") && !value.includes("\\") &&
    !value.includes(":") && !/[\u0000-\u001f\u007f]/u.test(value);
}

function hasNoAllowedTargets(scope: Record<string, unknown>): boolean {
  return scope.allowed_targets === undefined ||
    (Array.isArray(scope.allowed_targets) && scope.allowed_targets.length === 0);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasExactKeys(value: unknown, expected: string[]): value is Record<string, unknown> {
  if (!isRecord(value)) {
    return false;
  }
  const actual = Object.keys(value).sort();
  const wanted = [...expected].sort();
  return actual.length === wanted.length && actual.every((key, index) => key === wanted[index]);
}

function hasOnlyKeys(value: Record<string, unknown>, allowed: string[]): boolean {
  const accepted = new Set(allowed);
  return Object.keys(value).every((key) => accepted.has(key));
}

function boundedIdentity(value: unknown): string {
  return typeof value === "string" && value.trim() === value && value.length > 0 && value.length <= 256
    ? value
    : "";
}

function boundedText(value: unknown, maximum: number): value is string {
  return typeof value === "string" && value.trim() === value && value.length > 0 &&
    value.length <= maximum;
}

function validDate(value: unknown): value is string {
  return typeof value === "string" && Number.isFinite(Date.parse(value));
}

function isSHA256(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
}

function safePositiveInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value > 0;
}

function safeBoundedCount(value: unknown, maximum: number): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) &&
    value >= 0 && value <= maximum;
}

export class CyberAgentClient {
  readonly baseURL: string;
  readonly hasControl: boolean;
  readonly hasRunCreation: boolean;
  readonly hasSessionMessages: boolean;
  readonly hasSessionSteeringControl: boolean;
  readonly hasRunLifecycle: boolean;
  readonly hasRunExecution: boolean;
  readonly hasPlanDelivery: boolean;
  readonly hasApprovalControl: boolean;
  readonly hasModelControl: boolean;
  readonly hasProviderCredentials: boolean;
  readonly hasFileEditReview: boolean;
  readonly hasFileEditProposals: boolean;
  readonly hasFileEditApply: boolean;
  readonly hasRunWakeControl: boolean;
  readonly hasRunWakeExecution: boolean;
  readonly hasRunWakeWorker: boolean;
  readonly hasSkillInstallation: boolean;
  readonly hasEvidenceAttachment: boolean;
  readonly hasVerificationEvidence: boolean;

  constructor(
    private readonly token: string,
    baseURL = import.meta.env.VITE_API_BASE_URL || "/api/v1",
    private readonly controlToken = "",
    capabilities: ClientCapabilities = {},
  ) {
    if (token.trim() === "") {
      throw new Error("A read bearer token is required");
    }
    this.baseURL = normalizeBaseURL(baseURL);
    const controlPresent = controlToken.trim() !== "";
    this.hasControl = controlPresent && (capabilities.runControlEnabled ?? true);
    this.hasRunCreation = controlPresent && (capabilities.runCreationEnabled ?? true);
    this.hasSessionMessages = controlPresent && (capabilities.sessionMessageEnabled ?? true);
    this.hasSessionSteeringControl = controlPresent &&
      (capabilities.sessionSteeringControlEnabled ?? true);
    this.hasRunLifecycle = controlPresent && (capabilities.runLifecycleEnabled ?? true);
    this.hasRunExecution = controlPresent && (capabilities.runExecutionEnabled ?? true);
    this.hasPlanDelivery = controlPresent && (capabilities.planDeliveryControlEnabled ?? true);
    this.hasApprovalControl = controlPresent && (capabilities.approvalControlEnabled ?? true);
    this.hasModelControl = controlPresent && (capabilities.modelControlEnabled ?? true);
    this.hasProviderCredentials = controlPresent &&
      (capabilities.providerCredentialEnabled ?? false);
    this.hasFileEditReview = controlPresent && (capabilities.fileEditReviewEnabled ?? true);
    this.hasFileEditProposals = controlPresent &&
      (capabilities.fileEditProposalEnabled ?? false);
    this.hasFileEditApply = controlPresent && (capabilities.fileEditApplyEnabled ?? true);
    this.hasRunWakeControl = controlPresent && (capabilities.runWakeControlEnabled ?? true);
    this.hasRunWakeExecution = controlPresent && (capabilities.runWakeExecutionEnabled ?? true);
    this.hasRunWakeWorker = controlPresent && (capabilities.runWakeWorkerEnabled ?? false);
    this.hasSkillInstallation = controlPresent && (capabilities.skillInstallationEnabled ?? true);
    this.hasEvidenceAttachment = controlPresent &&
      (capabilities.evidenceAttachmentEnabled ?? true);
    this.hasVerificationEvidence = controlPresent &&
      (capabilities.verificationEvidenceEnabled ?? false);
  }

  async health(signal?: AbortSignal): Promise<HealthView> {
    return this.get<HealthView>("/health", {}, signal);
  }

  async runtimeCapabilities(signal?: AbortSignal): Promise<RuntimeCapabilitiesView> {
    return parseRuntimeCapabilities(await this.get<unknown>("/capabilities", {}, signal));
  }

  async modelAvailability(signal?: AbortSignal): Promise<ModelAvailabilityView> {
    const value = await this.get<unknown>("/models", {}, signal);
    return parseModelAvailability(value);
  }

  async workspaceExplore(workspaceID: string, path = ".",
    signal?: AbortSignal): Promise<WorkspaceExplorerView> {
    if (!boundedIdentity(workspaceID) || !validWorkspaceRelativePath(path)) {
      throw new Error("A normalized Workspace identity and Go-issued relative path are required");
    }
    return parseWorkspaceExplorer(await this.get<unknown>(
      `/workspaces/${encodeURIComponent(workspaceID)}/explore`, { path }, signal,
    ), workspaceID, path);
  }

  async workspaceSearch(workspaceID: string, query: string,
    signal?: AbortSignal): Promise<WorkspaceSearchView> {
    if (!boundedIdentity(workspaceID) || !boundedText(query, 128) ||
      /[\u0000-\u001f\u007f]/u.test(query)) {
      throw new Error("A normalized Workspace identity and bounded query are required");
    }
    return parseWorkspaceSearch(await this.get<unknown>(
      `/workspaces/${encodeURIComponent(workspaceID)}/search`, { query }, signal,
    ), workspaceID);
  }

  async repositoryState(workspaceID: string,
    signal?: AbortSignal): Promise<RepositoryStateView> {
    if (!boundedIdentity(workspaceID) || workspaceID.trim() !== workspaceID) {
      throw new Error("A normalized Workspace identity is required");
    }
    return parseRepositoryState(await this.get<unknown>(
      `/workspaces/${encodeURIComponent(workspaceID)}/repository-state`, {}, signal,
    ), workspaceID);
  }

  async repositoryDiff(workspaceID: string,
    signal?: AbortSignal): Promise<RepositoryDiffView> {
    if (!boundedIdentity(workspaceID) || workspaceID.trim() !== workspaceID) {
      throw new Error("A normalized Workspace identity is required");
    }
    return parseRepositoryDiff(await this.get<unknown>(
      `/workspaces/${encodeURIComponent(workspaceID)}/repository-diff`, {}, signal,
    ), workspaceID);
  }

  async repositoryHistory(workspaceID: string,
    signal?: AbortSignal): Promise<RepositoryHistoryView> {
    if (!boundedIdentity(workspaceID) || workspaceID.trim() !== workspaceID) {
      throw new Error("A normalized Workspace identity is required");
    }
    return parseRepositoryHistory(await this.get<unknown>(
      `/workspaces/${encodeURIComponent(workspaceID)}/repository-history`, {}, signal,
    ), workspaceID);
  }

  async repositoryCommit(workspaceID: string, objectID: string,
    signal?: AbortSignal): Promise<RepositoryCommitDetailView> {
    if (!boundedIdentity(workspaceID) || workspaceID.trim() !== workspaceID ||
      !/^[0-9a-f]{40}$/u.test(objectID)) {
      throw new Error("A normalized Workspace identity and exact commit object are required");
    }
    return parseRepositoryCommitDetail(await this.get<unknown>(
      `/workspaces/${encodeURIComponent(workspaceID)}/repository-commits/${objectID}`, {}, signal,
    ), workspaceID, objectID);
  }

  async operationReceiptHistory(runID = "",
    signal?: AbortSignal): Promise<OperationReceiptHistoryView> {
    if (runID !== "" && (!boundedIdentity(runID) || runID.trim() !== runID)) {
      throw new Error("A normalized Run identity is required");
    }
    return parseOperationReceiptHistory(await this.get<unknown>(
      "/operation-receipts", { run_id: runID || undefined, limit: 100 }, signal,
    ), runID);
  }

  async operatorActionCenter(runID: string,
    signal?: AbortSignal): Promise<OperatorActionCenterView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    return parseOperatorActionCenter(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/operator-actions`, {}, signal,
    ), runID);
  }

  async evidenceInventory(runID: string,
    signal?: AbortSignal): Promise<EvidenceInventoryView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    return parseEvidenceInventory(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/evidence-attachments`, {}, signal,
    ), runID);
  }

  async verificationEvidence(runID: string,
    signal?: AbortSignal): Promise<VerificationEvidenceInventoryView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    return parseVerificationEvidenceInventory(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/verification-evidence`, {}, signal,
    ), runID);
  }

  async recordVerificationEvidence(runID: string, body: VerificationEvidenceRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<VerificationEvidenceControlView> {
    if (!this.hasVerificationEvidence || !boundedIdentity(runID) ||
      body.version !== "operator_verification_evidence.v1" ||
      !["pass", "fail", "unknown"].includes(body.outcome) ||
      !validVerificationText(body.title, 160, false) ||
      !validVerificationText(body.summary, 2048, true)) {
      throw new Error("Verification evidence capability and a bounded observation are required");
    }
    return parseVerificationEvidenceItem(await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/verification-evidence`, body, idempotencyKey, signal,
    ), runID, "", "", true);
  }

  async verificationPlans(runID: string,
    signal?: AbortSignal): Promise<VerificationPlanInventoryView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    return parseVerificationPlanInventory(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/verification-plan`, {}, signal,
    ), runID);
  }

  async recordVerificationPlan(runID: string, body: VerificationPlanRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<VerificationPlanControlView> {
    if (!this.hasVerificationEvidence || !boundedIdentity(runID) ||
      body.version !== "operator_verification_plan.v1" ||
      !validVerificationText(body.title, 160, false) ||
      !validVerificationText(body.summary, 2048, true) || !Array.isArray(body.items) ||
      body.items.length < 1 || body.items.length > 32 || body.items.some((item) =>
        !validVerificationText(item.title, 160, false) ||
        !validVerificationText(item.expected_observation, 1024, true))) {
      throw new Error("Verification capability and a bounded operator checklist are required");
    }
    return parseVerificationPlan(await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/verification-plan`, body,
      idempotencyKey, signal), runID, "", "", true);
  }

  async verificationPlanCoverage(runID: string,
    signal?: AbortSignal): Promise<VerificationPlanCoverageInventoryView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    return parseVerificationPlanCoverage(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/verification-plan-coverage`, {}, signal,
    ), runID);
  }

  async associateVerificationEvidence(runID: string, body: VerificationAssociationRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<VerificationAssociationControlView> {
    if (!this.hasVerificationEvidence || !boundedIdentity(runID) ||
      body.version !== "operator_verification_plan_evidence_association.v1" ||
      !boundedIdentity(body.plan_id) || !safePositiveInteger(body.plan_item_ordinal) ||
      body.plan_item_ordinal > 32 || !boundedIdentity(body.evidence_id)) {
      throw new Error("Verification capability and exact plan/evidence identities are required");
    }
    return parseVerificationAssociation(await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/verification-plan-associations`, body,
      idempotencyKey, signal), runID, true);
  }

  async codeHandoff(runID: string, signal?: AbortSignal): Promise<CodeHandoffView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    return parseCodeHandoff(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/code-handoff`, {}, signal,
    ), runID);
  }

  async codeHandoffExport(runID: string, format: "json" | "markdown",
    signal?: AbortSignal): Promise<CodeHandoffExportView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID ||
      !["json", "markdown"].includes(format)) {
      throw new Error("A normalized Run identity and supported handoff format are required");
    }
    return parseCodeHandoffExport(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/code-handoff/export`, { format }, signal,
    ), runID, format);
  }

  async attachEvidence(runID: string, body: EvidenceAttachmentRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<EvidenceAttachmentView> {
    if (!this.hasEvidenceAttachment || !boundedIdentity(runID) ||
      body.version !== "session_evidence_attachment.v1" ||
      body.source_kind !== "workspace_file" || !validWorkspaceRelativePath(body.source_ref) ||
      !isSHA256(body.content_sha256)) {
      throw new Error("Evidence attachment capability and exact Workspace provenance are required");
    }
    return parseEvidenceAttachment(await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/evidence-attachments`, body, idempotencyKey, signal,
    ), runID, body);
  }

  async selectModelRoute(route: string, body: ModelRouteControlRequestView,
    signal?: AbortSignal): Promise<ModelAvailabilityView["routes"][number]> {
    if (!this.hasModelControl || !boundedIdentity(route) || route.trim() !== route) {
      throw new Error("Model control capability and a normalized route are required");
    }
    const result = await this.sendControlRequest<unknown>(
      `/models/routes/${encodeURIComponent(route)}`, body, signal,
    );
    return parseModelRouteControl(result, route, body);
  }

  async diagnoseProvider(body: ProviderDiagnosticRequestView,
    signal?: AbortSignal): Promise<ProviderDiagnosticView> {
    if (!this.hasModelControl || body.confirm_diagnostic !== true) {
      throw new Error("Explicit Provider diagnostic confirmation is required");
    }
    const result = await this.sendControlRequest<unknown>("/models/diagnostics", body, signal);
    return parseProviderDiagnostic(result, body);
  }

  async providerCredentialStatuses(signal?: AbortSignal): Promise<ProviderCredentialListView> {
    if (!this.hasProviderCredentials) {
      throw new Error("Provider credential capability is required");
    }
    return parseProviderCredentialList(await this.get<unknown>("/models/credentials", {}, signal));
  }

  async changeProviderCredential(provider: string, body: ProviderCredentialRequestView,
    signal?: AbortSignal): Promise<ProviderCredentialStatusView> {
    if (!this.hasProviderCredentials || !["anthropic", "deepseek", "mimo"].includes(provider) ||
      body.version !== "provider_credential.v1" || body.confirm !== true ||
      (body.action === "set" ? typeof body.secret !== "string" || body.secret.length < 8 :
        body.action !== "delete" || body.secret !== "")) {
      throw new Error("Confirmed Provider credential capability is required");
    }
    const result = await this.sendControlRequest<unknown>(
      `/models/credentials/${encodeURIComponent(provider)}`, body, signal,
    );
    const status = parseProviderCredentialStatus(result, provider);
    const applied = status.registry_reloaded && !status.restart_required &&
      status.registry_generation > 0;
    const deferred = !status.registry_reloaded && status.restart_required;
    if (status.configured !== (body.action === "set") || (!applied && !deferred)) {
      throw new APIRequestError("Provider credential change returned the wrong status",
        "INVALID_RESPONSE", 502);
    }
    return status;
  }

  async fileEditQueue(runID: string, signal?: AbortSignal): Promise<FileEditQueueView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    return parseFileEditQueue(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/file-edits`, {}, signal,
    ), runID);
  }

  async fileEditChangeSet(runID: string,
    signal?: AbortSignal): Promise<FileEditChangeSetView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    return parseFileEditChangeSet(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/file-edit-change-set`, {}, signal,
    ), runID);
  }

  async issueFileEditProposalSource(runID: string, path: string,
    signal?: AbortSignal): Promise<FileEditProposalSourceView> {
    if (!this.hasFileEditProposals || !boundedIdentity(runID) ||
      !validWorkspaceRelativePath(path)) {
      throw new Error("File edit proposal capability and a Go-issued path are required");
    }
    return parseFileEditProposalSource(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/file-edit-proposal-source`, { path }, signal,
    ), runID, path);
  }

  async reissueFileEditProposalSource(runID: string, path: string, expectedSHA256: string,
    signal?: AbortSignal): Promise<FileEditProposalSourceView> {
    if (!this.hasFileEditProposals || !boundedIdentity(runID) ||
      !validWorkspaceRelativePath(path) || !isSHA256(expectedSHA256)) {
      throw new Error("An exact previously issued file digest is required");
    }
    return parseFileEditProposalSource(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/file-edit-proposal-source`,
      { path, expected_sha256: expectedSHA256 }, signal,
    ), runID, path);
  }

  async recoverFileEditProposal(runID: string, editID: string,
    signal?: AbortSignal): Promise<FileEditProposalRecoveryView> {
    if (!this.hasFileEditProposals || !boundedIdentity(runID) || !boundedIdentity(editID)) {
      throw new Error("File edit recovery requires exact Run and proposal identities");
    }
    return parseFileEditProposalRecovery(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/file-edit-proposal-recovery/${encodeURIComponent(editID)}`,
      {}, signal,
    ), runID, editID);
  }

  async createFileEditProposal(runID: string, body: FileEditProposalRequestView,
    signal?: AbortSignal): Promise<FileEditProposalView> {
    if (!this.hasFileEditProposals || !boundedIdentity(runID) ||
      body.version !== "file_edit_proposal.v1" ||
      !/^[A-Za-z0-9_-]{43}$/u.test(body.source_handle) ||
      typeof body.proposed_text !== "string" ||
      new TextEncoder().encode(body.proposed_text).length > 256 * 1024) {
      throw new Error("A bounded Go-issued file edit proposal is required");
    }
    return parseFileEditProposal(await this.sendControlRequest<unknown>(
      `/runs/${encodeURIComponent(runID)}/file-edit-proposals`, body, signal,
    ), runID);
  }

  async reviewFileEdit(runID: string, editID: string, body: FileEditReviewRequestView,
    signal?: AbortSignal): Promise<FileEditReviewView> {
    if (!this.hasFileEditReview || !boundedIdentity(runID) || !boundedIdentity(editID)) {
      throw new Error("File edit review capability and normalized identities are required");
    }
    const result = await this.sendControlRequest<unknown>(
      `/runs/${encodeURIComponent(runID)}/file-edits/${encodeURIComponent(editID)}/review`,
      body, signal,
    );
    return parseFileEditReview(result, runID, editID, body);
  }

  async applyFileEdit(runID: string, editID: string, body: FileEditApplyRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<FileEditApplyView> {
    if (!this.hasFileEditApply || !boundedIdentity(runID) || !boundedIdentity(editID)) {
      throw new Error("File edit apply capability and normalized identities are required");
    }
    const result = await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/file-edits/${encodeURIComponent(editID)}/apply`,
      body, idempotencyKey, signal,
    );
    return parseFileEditApply(result, runID, editID);
  }

  async runWakeState(runID: string, signal?: AbortSignal): Promise<RunWakeStateView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    return parseRunWakeState(await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/wake-intent`, {}, signal,
    ), runID);
  }

  async scheduleRunWake(runID: string, body: RunWakeScheduleRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<RunWakeControlView> {
    if (!this.hasRunWakeControl) {
      throw new Error("Run wake control capability is required");
    }
    return parseRunWakeControl(await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/wake-intent`, body, idempotencyKey, signal,
    ), runID, "schedule");
  }

  async cancelRunWake(runID: string, body: RunWakeCancelRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<RunWakeControlView> {
    if (!this.hasRunWakeControl) {
      throw new Error("Run wake control capability is required");
    }
    return parseRunWakeControl(await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/wake-intent/cancel`, body, idempotencyKey, signal,
    ), runID, "cancel");
  }

  async consumeRunWake(runID: string, body: RunWakeExecutionRequestView,
    signal?: AbortSignal): Promise<RunWakeExecutionView> {
    if (!this.hasRunWakeExecution || !boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("Foreground Run wake capability and a normalized Run are required");
    }
    return parseRunWakeExecution(await this.sendControlRequest<unknown>(
      `/runs/${encodeURIComponent(runID)}/wake-intent/consume`, body, signal,
    ), runID);
  }

  async installSkillPackage(body: SkillPackageInstallRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<SkillPackageInstallView> {
    if (!this.hasSkillInstallation || body.confirm_untrusted !== true) {
      throw new Error("Explicit untrusted Skill installation capability is required");
    }
    return parseSkillPackageInstall(await this.sendControl<unknown>(
      "/skills/packages/install", body, idempotencyKey, signal,
    ), body);
  }

  async get<T>(path: string, query: Record<string, QueryValue> = {}, signal?: AbortSignal): Promise<T> {
    const envelope = await this.request<T>(path, query, signal);
    return envelope.data;
  }

  async getPage<T>(
    path: string,
    query: Record<string, QueryValue> = {},
    cursor = "",
    signal?: AbortSignal,
  ): Promise<PageResult<T>> {
    const envelope = await this.request<T[]>(path, { ...query, cursor: cursor || undefined }, signal);
    if (!envelope.page || !Array.isArray(envelope.data)) {
      throw new APIRequestError("API collection response omitted pagination metadata", "INVALID_RESPONSE", 502,
        envelope.request_id);
    }
    return { items: envelope.data, page: envelope.page, requestID: envelope.request_id };
  }

  async postControl<T>(
    path: string,
    body: unknown,
    idempotencyKey: string,
    signal?: AbortSignal,
  ): Promise<T> {
    if (!this.hasControl) {
      throw new Error("A control bearer token is required for this operation");
    }
    return this.sendControl<T>(path, body, idempotencyKey, signal);
  }

  async createRun(body: RunCreationControlRequestView, idempotencyKey: string,
    signal?: AbortSignal): Promise<RunCreationControlView> {
    if (!this.hasRunCreation) {
      throw new Error("Run creation capability is required for this operation");
    }
    const result = await this.sendControl<unknown>("/runs", body, idempotencyKey, signal);
    return parseRunCreationControl(result, body);
  }

  async submitSessionMessage(sessionID: string, body: SessionMessageControlRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<SessionMessageControlView> {
    if (!this.hasSessionMessages) {
      throw new Error("Session message capability is required for this operation");
    }
    const normalizedSessionID = boundedIdentity(sessionID);
    if (!normalizedSessionID || normalizedSessionID !== sessionID) {
      throw new Error("A normalized Session identity is required");
    }
    const result = await this.sendControl<unknown>(
      `/sessions/${encodeURIComponent(sessionID)}/messages`, body, idempotencyKey, signal,
    );
    return parseSessionMessageControl(result, sessionID);
  }

  async cancelSessionSteering(sessionID: string, messageID: string,
    body: SessionSteeringCancellationRequestView, idempotencyKey: string,
    signal?: AbortSignal): Promise<SessionSteeringCancellationView> {
    if (!this.hasSessionSteeringControl) {
      throw new Error("Session steering cancellation capability is required for this operation");
    }
    const normalizedSessionID = boundedIdentity(sessionID);
    const normalizedMessageID = boundedIdentity(messageID);
    if (!normalizedSessionID || normalizedSessionID !== sessionID ||
      !normalizedMessageID || normalizedMessageID !== messageID) {
      throw new Error("Normalized Session and steering identities are required");
    }
    const result = await this.sendControl<unknown>(
      `/sessions/${encodeURIComponent(sessionID)}/messages/${encodeURIComponent(messageID)}/cancel`,
      body, idempotencyKey, signal,
    );
    return parseSessionSteeringCancellation(result, sessionID, messageID);
  }

  async controlRunLifecycle(runID: string, body: RunLifecycleControlRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<RunLifecycleControlView> {
    if (!this.hasRunLifecycle) {
      throw new Error("Run lifecycle capability is required for this operation");
    }
    const normalizedRunID = boundedIdentity(runID);
    if (!normalizedRunID || normalizedRunID !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    const result = await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/lifecycle`, body, idempotencyKey, signal,
    );
    return parseRunLifecycleControl(result, runID, body);
  }

  async executeRun(runID: string, body: RunExecutionControlRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<RunExecutionControlView> {
    if (!this.hasRunExecution) {
      throw new Error("Run execution capability is required for this operation");
    }
    const normalizedRunID = boundedIdentity(runID);
    if (!normalizedRunID || normalizedRunID !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    const result = await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/execute`, body, idempotencyKey, signal,
    );
    return parseRunExecutionControl(result, runID, body);
  }

  async selectPlanDirection(runID: string, body: PlanDirectionControlRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<PlanDirectionControlView> {
    if (!this.hasPlanDelivery) {
      throw new Error("Plan/Delivery control capability is required for this operation");
    }
    if (!boundedIdentity(runID) || runID.trim() !== runID || body.direction < 1 ||
      body.direction > 3 || !boundedIdentity(body.proposal_id)) {
      throw new Error("A normalized Run, proposal, and direction are required");
    }
    const result = await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/plan/direction`, body, idempotencyKey, signal,
    );
    return parsePlanDirectionControl(result, runID, body);
  }

  async enterPlanDelivery(runID: string, body: PlanDeliveryTransitionControlRequestView,
    idempotencyKey: string, signal?: AbortSignal): Promise<PlanDeliveryTransitionControlView> {
    if (!this.hasPlanDelivery) {
      throw new Error("Plan/Delivery control capability is required for this operation");
    }
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    const result = await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/plan/deliver`, body, idempotencyKey, signal,
    );
    return parsePlanDeliveryTransition(result, runID);
  }

  async approvalQueue(runID: string, signal?: AbortSignal): Promise<ApprovalQueueView> {
    if (!boundedIdentity(runID) || runID.trim() !== runID) {
      throw new Error("A normalized Run identity is required");
    }
    const value = await this.get<unknown>(
      `/runs/${encodeURIComponent(runID)}/approvals`, {}, signal,
    );
    return parseApprovalQueue(value, runID);
  }

  async decideApproval(runID: string, approvalID: string,
    body: ApprovalDecisionControlRequestView, idempotencyKey: string,
    signal?: AbortSignal): Promise<ApprovalDecisionControlView> {
    if (!this.hasApprovalControl) {
      throw new Error("Approval control capability is required for this operation");
    }
    if (!boundedIdentity(runID) || runID.trim() !== runID ||
      !boundedIdentity(approvalID) || approvalID.trim() !== approvalID) {
      throw new Error("Normalized Run and approval identities are required");
    }
    const result = await this.sendControl<unknown>(
      `/runs/${encodeURIComponent(runID)}/approvals/${encodeURIComponent(approvalID)}/decision`,
      body, idempotencyKey, signal,
    );
    return parseApprovalDecision(result, runID, approvalID, body);
  }

  private async sendControl<T>(path: string, body: unknown, idempotencyKey: string,
    signal?: AbortSignal): Promise<T> {
    if (idempotencyKey.trim() !== idempotencyKey || idempotencyKey.length < 16) {
      throw new Error("A normalized idempotency key is required");
    }
    return this.sendControlRequest<T>(path, body, signal, idempotencyKey);
  }

  private async sendControlRequest<T>(path: string, body: unknown, signal?: AbortSignal,
    idempotencyKey = ""): Promise<T> {
    const headers: Record<string, string> = {
      Accept: "application/json",
      Authorization: `Bearer ${this.controlToken}`,
      "Content-Type": "application/json",
    };
    if (idempotencyKey) {
      headers["Idempotency-Key"] = idempotencyKey;
    }
    const response = await fetch(this.url(path), {
      method: "POST",
      headers,
      body: JSON.stringify(body),
      signal,
      cache: "no-store",
      credentials: "omit",
      referrerPolicy: "no-referrer",
    });
    const payload = await this.readJSON(response);
    if (!response.ok) {
      if (isErrorEnvelope(payload)) {
        throw new APIRequestError(payload.error.message, payload.error.code, response.status, payload.request_id);
      }
      throw new APIRequestError("CyberAgent control request failed", "INVALID_RESPONSE", response.status,
        response.headers.get("x-request-id") || "");
    }
    if (!isSuccessEnvelope<T>(payload)) {
      throw new APIRequestError("CyberAgent API returned an invalid control envelope", "INVALID_RESPONSE",
        response.status, response.headers.get("x-request-id") || "");
    }
    return payload.data;
  }

  async streamRunEvents(
    runID: string,
    options: {
      cursor?: string;
      signal: AbortSignal;
      onFrame: (frame: RunEventStreamView) => void;
    },
  ): Promise<void> {
    const headers = this.headers();
    headers.Accept = "text/event-stream";
    if (options.cursor) {
      headers["Last-Event-ID"] = options.cursor;
    }
    const response = await fetch(this.url(`/runs/${encodeURIComponent(runID)}/events/stream`), {
      method: "GET",
      headers,
      signal: options.signal,
      cache: "no-store",
      credentials: "omit",
      referrerPolicy: "no-referrer",
    });
    if (!response.ok) {
      throw await this.responseError(response);
    }
    if (!response.headers.get("content-type")?.toLowerCase().startsWith("text/event-stream") || !response.body) {
      throw new APIRequestError("API returned an invalid event stream", "INVALID_RESPONSE", response.status,
        response.headers.get("x-request-id") || "");
    }
    await consumeSSE(response.body, (message) => {
      if (message.event !== "run.event") {
        return;
      }
      const frame = parseStreamFrame(JSON.parse(message.data) as unknown, runID);
      if (message.id === "" || message.id !== frame.cursor) {
        throw new Error("SSE id does not match the frame cursor");
      }
      options.onFrame(frame);
    });
  }

  async pollRunEvents(
    runID: string,
    cursor = "",
    limit = 100,
    signal?: AbortSignal,
  ): Promise<RunEventPollView> {
    if (!Number.isSafeInteger(limit) || limit <= 0 || limit > 100) {
      throw new Error("Event poll limit must be between 1 and 100");
    }
    const envelope = await this.request<unknown>(
      `/runs/${encodeURIComponent(runID)}/events/poll`,
      { cursor: cursor || undefined, limit },
      signal,
    );
    return parseEventPoll(envelope.data, runID, envelope.request_id);
  }

  private async request<T>(
    path: string,
    query: Record<string, QueryValue>,
    signal?: AbortSignal,
  ): Promise<SuccessEnvelope<T>> {
    const response = await fetch(this.url(path, query), {
      method: "GET",
      headers: this.headers(),
      signal,
      cache: "no-store",
      credentials: "omit",
      referrerPolicy: "no-referrer",
    });
    const payload = await this.readJSON(response);
    if (!response.ok) {
      if (isErrorEnvelope(payload)) {
        throw new APIRequestError(payload.error.message, payload.error.code, response.status, payload.request_id);
      }
      throw new APIRequestError("CyberAgent API request failed", "INVALID_RESPONSE", response.status,
        response.headers.get("x-request-id") || "");
    }
    if (!isSuccessEnvelope<T>(payload)) {
      throw new APIRequestError("CyberAgent API returned an invalid envelope", "INVALID_RESPONSE", response.status,
        response.headers.get("x-request-id") || "");
    }
    return payload;
  }

  private async responseError(response: Response): Promise<APIRequestError> {
    const payload = await this.readJSON(response);
    if (isErrorEnvelope(payload)) {
      return new APIRequestError(payload.error.message, payload.error.code, response.status, payload.request_id);
    }
    return new APIRequestError("CyberAgent API request failed", "INVALID_RESPONSE", response.status,
      response.headers.get("x-request-id") || "");
  }

  private async readJSON(response: Response): Promise<unknown> {
    try {
      return await response.json() as unknown;
    } catch {
      return null;
    }
  }

  private headers(): Record<string, string> {
    return {
      Accept: "application/json",
      Authorization: `Bearer ${this.token}`,
    };
  }

  private url(path: string, query: Record<string, QueryValue> = {}): string {
    if (!path.startsWith("/") || path.startsWith("//")) {
      throw new Error("API path must be relative to the configured base path");
    }
    const url = new URL(`${this.baseURL}${path}`, window.location.origin);
    if (!url.pathname.startsWith(`${this.baseURL}/`)) {
      throw new Error("API path escaped the configured base path");
    }
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== "") {
        url.searchParams.set(key, String(value));
      }
    }
    return `${url.pathname}${url.search}`;
  }
}
