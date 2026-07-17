import { consumeSSE } from "./sse";
import type {
  ErrorEnvelope,
  HealthView,
  PageResult,
  RunCreationControlRequestView,
  RunCreationControlView,
  RunEventPollView,
  RunEventStreamView,
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

function boundedIdentity(value: unknown): string {
  return typeof value === "string" && value.trim() === value && value.length > 0 && value.length <= 256
    ? value
    : "";
}

export class CyberAgentClient {
  readonly baseURL: string;
  readonly hasControl: boolean;
  readonly hasRunCreation: boolean;

  constructor(
    private readonly token: string,
    baseURL = import.meta.env.VITE_API_BASE_URL || "/api/v1",
    private readonly controlToken = "",
    capabilities: { runControlEnabled?: boolean; runCreationEnabled?: boolean } = {},
  ) {
    if (token.trim() === "") {
      throw new Error("A read bearer token is required");
    }
    this.baseURL = normalizeBaseURL(baseURL);
    const controlPresent = controlToken.trim() !== "";
    this.hasControl = controlPresent && (capabilities.runControlEnabled ?? true);
    this.hasRunCreation = controlPresent && (capabilities.runCreationEnabled ?? true);
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
