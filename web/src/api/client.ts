import { consumeSSE } from "./sse";
import type {
  ErrorEnvelope,
  HealthView,
  PageResult,
  RunCreationControlRequestView,
  RunCreationControlView,
  RunExecutionControlRequestView,
  RunExecutionControlView,
  RunLifecycleControlRequestView,
  RunLifecycleControlView,
  RunEventPollView,
  RunEventStreamView,
  SessionMessageControlRequestView,
  SessionMessageControlView,
  SessionSteeringCancellationRequestView,
  SessionSteeringCancellationView,
  SuccessEnvelope,
} from "./types";

export type QueryValue = boolean | number | string | undefined;

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
    frame.event.version !== "event.v1" || frame.event.run_id !== expectedRunID ||
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

  constructor(
    private readonly token: string,
    baseURL = import.meta.env.VITE_API_BASE_URL || "/api/v1",
    private readonly controlToken = "",
    capabilities: { runControlEnabled?: boolean; runCreationEnabled?: boolean;
      sessionMessageEnabled?: boolean; sessionSteeringControlEnabled?: boolean;
      runLifecycleEnabled?: boolean; runExecutionEnabled?: boolean } = {},
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
  }

  async health(signal?: AbortSignal): Promise<HealthView> {
    return this.get<HealthView>("/health", {}, signal);
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

  private async sendControl<T>(path: string, body: unknown, idempotencyKey: string,
    signal?: AbortSignal): Promise<T> {
    if (idempotencyKey.trim() !== idempotencyKey || idempotencyKey.length < 16) {
      throw new Error("A normalized idempotency key is required");
    }
    const response = await fetch(this.url(path), {
      method: "POST",
      headers: {
        Accept: "application/json",
        Authorization: `Bearer ${this.controlToken}`,
        "Content-Type": "application/json",
        "Idempotency-Key": idempotencyKey,
      },
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
