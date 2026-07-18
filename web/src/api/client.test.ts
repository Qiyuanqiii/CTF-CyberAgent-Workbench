import { CyberAgentClient } from "./client";
import type { RunEventStreamView, RunLifecycleControlView } from "./types";

const healthEnvelope = {
  version: "api.v1",
  request_id: "req-health",
  data: { status: "ok", api_version: "api.v1", app_version: "test", schema_version: 37 },
};

const runCreationData = {
  mission: {
    id: "mission-created", goal: "Create parser", workspace_id: "workspace-1", profile: "code",
    scope: { workspace_id: "workspace-1", network_mode: "disabled" },
  },
  run: {
    id: "run-created", mission_id: "mission-created", session_id: "sess-created",
    status: "created",
    config: { interactive: true, model_route: "code" }, budget: { max_turns: 100, max_tool_calls: 100 },
  },
  session: { id: "sess-created", workspace_id: "workspace-1", title: "Create parser", route: "code", status: "active" },
  mode: {
    protocol_version: "run_mode.v1", policy_version: "mode_policy.v1", revision: 1,
    profile: "code", surface: "code", phase: "deliver",
    scope: { workspace_id: "workspace-1", network_mode: "disabled" }, capability_grant: false,
  },
  replayed: false,
};

const sessionMessageData = {
  version: "session_message_submission.v1",
  run_id: "run-1",
  session_id: "sess-1",
  steering: {
    id: "steer-1", sequence: 1, status: "pending", prepared: false,
    created_at: "2026-07-18T00:00:00Z",
  },
  replayed: false,
  execution_started: false,
  model_called: false,
  tool_called: false,
  capability_grant: false,
};

const sessionSteeringCancellationData = {
  version: "session_steering_cancellation.v1",
  run_id: "run-1", session_id: "sess-1",
  steering: {
    id: "steer-1", sequence: 1, status: "cancelled", prepared: false,
    created_at: "2026-07-18T00:00:00Z", cancelled_at: "2026-07-18T00:01:00Z",
  },
  cancellation_id: "cancel-1", cancellation_kind: "operator", replayed: false,
  execution_started: false, model_called: false, tool_called: false, capability_grant: false,
};

const runLifecycleData = {
  version: "run_lifecycle_control.v1",
  run: {
    id: "run-1", mission_id: "mission-1", session_id: "sess-1", status: "running",
    config: { model_route: "code", interactive: true }, budget: { max_turns: 4 },
    created_at: "2026-07-18T00:00:00Z", updated_at: "2026-07-18T00:01:00Z",
  },
  action: "start", expected_status: "created", applied_status: "running",
  event_sequence_start: 5, event_sequence_end: 6, replayed: false,
  execution_started: false, model_called: false, tool_called: false, capability_grant: false,
};

const runExecutionData = {
  version: "run_execution_handoff.v1", operation_id: "run-handoff-1",
  run_id: "run-1", session_id: "sess-1", max_steps: 2, selected_count: 2,
  status: "completed", run_status: "running", stop_reason: "selection_drained",
  steps_completed: 1, pending_count: 0, prepared_count: 0, committed_count: 1,
  cancelled_count: 1, completion_event_sequence: 12, replayed: false,
  execution_started: true, model_called: true, tool_called: false, capability_grant: false,
};

describe("CyberAgentClient", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("keeps the bearer out of the URL and sends it only in Authorization", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(healthEnvelope), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetchMock);

    const result = await new CyberAgentClient("read-secret").health();

    expect(result.schema_version).toBe(37);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/health");
    expect(url).not.toContain("read-secret");
    expect(init.headers).toMatchObject({ Authorization: "Bearer read-secret" });
    expect(init.credentials).toBe("omit");
  });

  it("rejects a cross-origin API base before issuing a request", () => {
    expect(() => new CyberAgentClient("read-secret", "https://example.com/api/v1"))
      .toThrow("current browser origin");
    expect(() => new CyberAgentClient("read-secret", "/api/v10"))
      .toThrow("must be /api/v1");
  });

  it("rejects paths that escape the API base", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    await expect(new CyberAgentClient("read-secret").get("/../health"))
      .rejects.toThrow("escaped the configured base path");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("returns typed API errors without exposing request headers", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1",
      request_id: "req-denied",
      error: { code: "POLICY_DENIED", message: "valid bearer authorization is required" },
    }), { status: 401, headers: { "Content-Type": "application/json" } })));

    const request = new CyberAgentClient("wrong-secret").health();
    await expect(request).rejects.toMatchObject({
      code: "POLICY_DENIED",
      status: 401,
      requestID: "req-denied",
    });
  });

  it("forwards an opaque collection cursor without leaking the bearer", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1",
        request_id: "req-runs-1",
        data: [{ id: "run-paused", status: "paused" }],
        page: { limit: 1, next_cursor: "opaque+/cursor=one" },
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1",
        request_id: "req-runs-2",
        data: [{ id: "run-completed", status: "completed" }],
        page: { limit: 1 },
      }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret");

    const first = await client.getPage<{ id: string; status: string }>("/runs", { limit: 1 });
    const second = await client.getPage<{ id: string; status: string }>(
      "/runs", { limit: 1 }, first.page.next_cursor,
    );

    expect(first.items[0]?.status).toBe("paused");
    expect(second.items[0]?.status).toBe("completed");
    const [firstURL] = fetchMock.mock.calls[0] as [string, RequestInit];
    const [secondURL, secondInit] = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(firstURL).toBe("/api/v1/runs?limit=1");
    expect(secondURL).toContain("limit=1");
    expect(secondURL).toContain("cursor=opaque%2B%2Fcursor%3Done");
    expect(secondURL).not.toContain("read-secret");
    expect(secondInit.headers).toMatchObject({ Authorization: "Bearer read-secret" });
  });

  it("keeps the optional control token in Authorization and out of URLs and bodies", async () => {
    const responseEnvelope = {
      version: "api.v1",
      request_id: "req-profile",
      data: { replayed: false, execution_profile: { profile: "docker" } },
    };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(responseEnvelope), {
      status: 202,
      headers: { "Content-Type": "application/json" },
    }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret");

    const result = await client.postControl<{ execution_profile: { profile: string } }>(
      "/runs/run-1/execution-profile",
      { profile: "docker" },
      "web-execution-profile-test-0001",
    );

    expect(result.execution_profile.profile).toBe("docker");
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/runs/run-1/execution-profile");
    expect(url).not.toContain("control-secret");
    expect(init.headers).toMatchObject({
      Authorization: "Bearer control-secret",
      "Content-Type": "application/json",
      "Idempotency-Key": "web-execution-profile-test-0001",
    });
    expect(init.body).toBe(JSON.stringify({ profile: "docker" }));
    expect(String(init.body)).not.toContain("control-secret");
  });

  it("does not expose control operations without a distinct control token", async () => {
    vi.stubGlobal("fetch", vi.fn());
    const client = new CyberAgentClient("read-secret");
    await expect(client.postControl("/runs/run-1/execution-profile", { profile: "docker" },
      "web-execution-profile-test-0002")).rejects.toThrow("control bearer token");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("separates Run creation from existing Run controls and validates closed authority", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-create", data: runCreationData,
    }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false,
      runCreationEnabled: true,
    });
    expect(client.hasControl).toBe(false);
    expect(client.hasRunCreation).toBe(true);
    await expect(client.postControl("/runs/run-1/execution-profile", { profile: "docker" },
      "web-profile-separated-0001")).rejects.toThrow("control bearer token");
    const result = await client.createRun({
      version: "run_creation.v1", goal: "Create parser", workspace_id: "workspace-1",
    }, "web-run-create-operation-0001");
    expect(result.run.id).toBe("run-created");
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/runs");
    expect(url).not.toContain("control-secret");
    expect(init.headers).toMatchObject({
      Authorization: "Bearer control-secret",
      "Idempotency-Key": "web-run-create-operation-0001",
    });
    expect(String(init.body)).not.toContain("control-secret");
  });

  it("rejects a Run creation response that widens authority", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1",
      request_id: "req-create-forged",
      data: { ...runCreationData, mode: { ...runCreationData.mode, capability_grant: true } },
    }), { status: 202, headers: { "Content-Type": "application/json" } })));
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false,
      runCreationEnabled: true,
    });
    await expect(client.createRun({ version: "run_creation.v1", goal: "Create parser",
      workspace_id: "workspace-1" }, "web-run-create-operation-0002"))
      .rejects.toThrow("closed authority");
  });

  it("rejects a Run creation response bound to a different requested workspace", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1",
      request_id: "req-create-forged-workspace",
      data: runCreationData,
    }), { status: 202, headers: { "Content-Type": "application/json" } })));
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false,
      runCreationEnabled: true,
    });
    await expect(client.createRun({ version: "run_creation.v1", goal: "Create parser",
      workspace_id: "workspace-other" }, "web-run-create-operation-0003"))
      .rejects.toThrow("closed authority");
  });

  it("rejects a Run creation response with a cross-Workspace Mission scope", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1",
      request_id: "req-create-forged-scope",
      data: {
        ...runCreationData,
        mission: { ...runCreationData.mission,
          scope: { ...runCreationData.mission.scope, workspace_id: "workspace-other" } },
      },
    }), { status: 202, headers: { "Content-Type": "application/json" } })));
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false,
      runCreationEnabled: true,
    });
    await expect(client.createRun({ version: "run_creation.v1", goal: "Create parser",
      workspace_id: "workspace-1" }, "web-run-create-operation-scope"))
      .rejects.toThrow("closed authority");
  });

  it("rejects a Run creation response bound to a different requested goal", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1",
      request_id: "req-create-forged-goal",
      data: runCreationData,
    }), { status: 202, headers: { "Content-Type": "application/json" } })));
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false,
      runCreationEnabled: true,
    });
    await expect(client.createRun({ version: "run_creation.v1", goal: "Different goal",
      workspace_id: "workspace-1" }, "web-run-create-operation-0004"))
      .rejects.toThrow("closed authority");
  });

  it("separates Session messages and validates the closed submission response", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-session-message", data: sessionMessageData,
    }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false,
      runCreationEnabled: false,
      sessionMessageEnabled: true,
    });
    expect(client.hasControl).toBe(false);
    expect(client.hasRunCreation).toBe(false);
    expect(client.hasSessionMessages).toBe(true);
    const result = await client.submitSessionMessage("sess-1", {
      version: "session_message_submission.v1", content: "Review the latest change",
    }, "web-session-message-operation-0001");
    expect(result.steering.id).toBe("steer-1");
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/sessions/sess-1/messages");
    expect(url).not.toContain("control-secret");
    expect(init.headers).toMatchObject({
      Authorization: "Bearer control-secret",
      "Idempotency-Key": "web-session-message-operation-0001",
    });
    expect(init.body).toBe(JSON.stringify({
      version: "session_message_submission.v1", content: "Review the latest change",
    }));
  });

  it("rejects forged Session message authority and cross-Session responses", async () => {
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false,
      runCreationEnabled: false,
      sessionMessageEnabled: true,
    });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-session-forged",
      data: { ...sessionMessageData, model_called: true },
    }), { status: 202, headers: { "Content-Type": "application/json" } })).mockResolvedValueOnce(
      new Response(JSON.stringify({
        version: "api.v1", request_id: "req-session-cross",
        data: { ...sessionMessageData, session_id: "sess-other" },
      }), { status: 202, headers: { "Content-Type": "application/json" } }),
    ));
    const request = { version: "session_message_submission.v1" as const, content: "Review" };
    await expect(client.submitSessionMessage("sess-1", request,
      "web-session-message-operation-0002")).rejects.toThrow("invalid");
    await expect(client.submitSessionMessage("sess-1", request,
      "web-session-message-operation-0003")).rejects.toThrow("invalid");
  });

  it("does not expose Session messages without their distinct capability", async () => {
    vi.stubGlobal("fetch", vi.fn());
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false,
      runCreationEnabled: true,
      sessionMessageEnabled: false,
    });
    await expect(client.submitSessionMessage("sess-1", {
      version: "session_message_submission.v1", content: "Review",
    }, "web-session-message-operation-0004")).rejects.toThrow("capability");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("separates pending Session steering cancellation and validates its authority", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-session-cancel",
      data: sessionSteeringCancellationData,
    }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, runCreationEnabled: false, sessionMessageEnabled: false,
      sessionSteeringControlEnabled: true,
    });
    expect(client.hasControl).toBe(false);
    expect(client.hasSessionMessages).toBe(false);
    expect(client.hasSessionSteeringControl).toBe(true);
    await expect(client.cancelSessionSteering("sess-1", "steer-1", {
      version: "session_steering_cancellation.v1", reason: "operator cancelled",
    }, "web-session-steering-cancel-0001")).resolves.toEqual(sessionSteeringCancellationData);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/sessions/sess-1/messages/steer-1/cancel");
    expect(init.headers).toMatchObject({
      Authorization: "Bearer control-secret",
      "Idempotency-Key": "web-session-steering-cancel-0001",
    });
  });

  it("rejects forged or cross-message Session steering cancellation responses", async () => {
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, runCreationEnabled: false, sessionMessageEnabled: false,
      sessionSteeringControlEnabled: true,
    });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-session-cancel-forged",
      data: { ...sessionSteeringCancellationData, execution_started: true },
    }), { status: 202, headers: { "Content-Type": "application/json" } })).mockResolvedValueOnce(
      new Response(JSON.stringify({
        version: "api.v1", request_id: "req-session-cancel-cross",
        data: { ...sessionSteeringCancellationData,
          steering: { ...sessionSteeringCancellationData.steering, id: "steer-other" } },
      }), { status: 202, headers: { "Content-Type": "application/json" } }),
    ));
    const body = { version: "session_steering_cancellation.v1" as const, reason: "cancel" };
    await expect(client.cancelSessionSteering("sess-1", "steer-1", body,
      "web-session-steering-cancel-0002")).rejects.toThrow("invalid");
    await expect(client.cancelSessionSteering("sess-1", "steer-1", body,
      "web-session-steering-cancel-0003")).rejects.toThrow("invalid");
  });

  it("does not expose Session steering cancellation without its capability", async () => {
    vi.stubGlobal("fetch", vi.fn());
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      sessionMessageEnabled: true, sessionSteeringControlEnabled: false,
    });
    await expect(client.cancelSessionSteering("sess-1", "steer-1", {
      version: "session_steering_cancellation.v1", reason: "cancel",
    }, "web-session-steering-cancel-0004")).rejects.toThrow("capability");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("separates Run lifecycle and bounded execution capabilities", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-lifecycle", data: runLifecycleData,
      }), { status: 202, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-execute", data: runExecutionData,
      }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, runCreationEnabled: false, sessionMessageEnabled: false,
      sessionSteeringControlEnabled: false, runLifecycleEnabled: true,
      runExecutionEnabled: true,
    });
    expect(client.hasControl).toBe(false);
    expect(client.hasRunLifecycle).toBe(true);
    expect(client.hasRunExecution).toBe(true);
    await expect(client.controlRunLifecycle("run-1", {
      version: "run_lifecycle_control.v1", action: "start",
    }, "web-run-lifecycle-operation-0001")).resolves.toEqual(runLifecycleData);
    await expect(client.executeRun("run-1", {
      version: "run_execution_handoff.v1", max_steps: 2,
    }, "web-run-execution-operation-0001")).resolves.toEqual(runExecutionData);
    const [lifecycleURL, lifecycleInit] = fetchMock.mock.calls[0] as [string, RequestInit];
    const [executionURL, executionInit] = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(lifecycleURL).toBe("/api/v1/runs/run-1/lifecycle");
    expect(executionURL).toBe("/api/v1/runs/run-1/execute");
    expect(lifecycleInit.headers).toMatchObject({ Authorization: "Bearer control-secret",
      "Idempotency-Key": "web-run-lifecycle-operation-0001" });
    expect(executionInit.headers).toMatchObject({ Authorization: "Bearer control-secret",
      "Idempotency-Key": "web-run-execution-operation-0001" });
  });

  it("rejects forged Run lifecycle and execution metadata", async () => {
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runLifecycleEnabled: true, runExecutionEnabled: true,
    });
    vi.stubGlobal("fetch", vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-lifecycle-forged",
        data: { ...runLifecycleData, model_called: true },
      }), { status: 202, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-execute-forged",
        data: { ...runExecutionData, committed_count: 2 },
      }), { status: 202, headers: { "Content-Type": "application/json" } })));
    await expect(client.controlRunLifecycle("run-1", {
      version: "run_lifecycle_control.v1", action: "start",
    }, "web-run-lifecycle-operation-0002")).rejects.toThrow("invalid");
    await expect(client.executeRun("run-1", {
      version: "run_execution_handoff.v1", max_steps: 2,
    }, "web-run-execution-operation-0002")).rejects.toThrow("invalid");
  });

  it("accepts an exact lifecycle replay after the Run has advanced", async () => {
    const delayedReplay = {
      ...runLifecycleData, replayed: true,
      run: { ...runLifecycleData.run, status: "paused" },
    } as RunLifecycleControlView;
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-lifecycle-delayed", data: delayedReplay,
    }), { status: 202, headers: { "Content-Type": "application/json" } })));
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runLifecycleEnabled: true,
    });
    await expect(client.controlRunLifecycle("run-1", {
      version: "run_lifecycle_control.v1", action: "start",
    }, "web-run-lifecycle-delayed-0001")).resolves.toEqual(delayedReplay);
  });

  it("does not expose Run operations without their distinct capabilities", async () => {
    vi.stubGlobal("fetch", vi.fn());
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runLifecycleEnabled: false, runExecutionEnabled: false,
    });
    await expect(client.controlRunLifecycle("run-1", {
      version: "run_lifecycle_control.v1", action: "start",
    }, "web-run-lifecycle-operation-0003")).rejects.toThrow("capability");
    await expect(client.executeRun("run-1", {
      version: "run_execution_handoff.v1", max_steps: 1,
    }, "web-run-execution-operation-0003")).rejects.toThrow("capability");
    expect(fetch).not.toHaveBeenCalled();
  });

  it("validates redacted model availability without probing through the client", async () => {
    const data = {
      protocol_version: "model_availability.v1",
      providers: [{ name: "mock", kind: "local", status: "available", models: ["mock-code"],
        credential_source: "none", network_required: false, configuration_error: false }],
      routes: [{ name: "code", provider: "mock", model: "mock-code", available: true }],
    };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-models", data,
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    await expect(new CyberAgentClient("read-secret").modelAvailability()).resolves.toEqual(data);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/api/v1/models");
    expect(init.method).toBe("GET");

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-models-forged",
      data: { ...data, providers: [{ ...data.providers[0], base_url: "https://private.invalid" }] },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    await expect(new CyberAgentClient("read-secret").modelAvailability()).rejects.toThrow("invalid");

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-models-unbound",
      data: { ...data, routes: [{ ...data.routes[0], provider: "missing" }] },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    await expect(new CyberAgentClient("read-secret").modelAvailability()).rejects.toThrow("invalid");
  });

  it("keeps Plan direction and Deliver as independently validated controls", async () => {
    const direction = {
      version: "plan_delivery_control.v1", run_id: "run-1", proposal_id: "proposal-1",
      selection_id: "selection-1", note_id: "note-1", direction: 2, work_item_count: 1,
      replayed: false, phase_changed: false, execution_started: false, model_called: false,
      tool_called: false, capability_grant: false,
    };
    const delivery = {
      version: "plan_delivery_control.v1", run_id: "run-1", selection_id: "selection-1",
      applied_mode: { phase: "deliver", capability_grant: false },
      current_mode: { phase: "deliver", capability_grant: false }, replayed: false,
      execution_started: false, model_called: false, tool_called: false, capability_grant: false,
    };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-plan-direction", data: direction,
      }), { status: 202, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-plan-deliver", data: delivery,
      }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, runCreationEnabled: false, sessionMessageEnabled: false,
      sessionSteeringControlEnabled: false, runLifecycleEnabled: false, runExecutionEnabled: false,
      planDeliveryControlEnabled: true, approvalControlEnabled: false,
    });
    expect(client.hasControl).toBe(false);
    expect(client.hasPlanDelivery).toBe(true);
    expect(client.hasApprovalControl).toBe(false);
    await expect(client.selectPlanDirection("run-1", {
      version: "plan_delivery_control.v1", proposal_id: "proposal-1", direction: 2,
    }, "web-plan-direction-operation-0001")).resolves.toEqual(direction);
    await expect(client.enterPlanDelivery("run-1", {
      version: "plan_delivery_control.v1",
    }, "web-plan-deliver-operation-0001")).resolves.toEqual(delivery);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/runs/run-1/plan/direction");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/v1/runs/run-1/plan/deliver");
  });

  it("validates a metadata-only approval queue and closed approve-once response", async () => {
    const queue = {
      protocol_version: "approval_queue.v1", run_id: "run-1", truncated: false,
      process_execution_enabled: false, session_grant_created: false, capability_grant: false,
      items: [{ id: "approval-1", proposal_id: "proposal-1", run_id: "run-1",
        session_id: "session-1", workspace_id: "workspace-1", tool_name: "shell",
        action_class: "shell", mode: "per_call", status: "pending",
        allowed_actions: ["approve_once", "deny"], version: 1,
        created_at: "2026-07-18T00:00:00Z", updated_at: "2026-07-18T00:00:00Z",
        process_execution_enabled: false, capability_grant: false }],
    };
    const decision = {
      version: "approval_control.v1", run_id: "run-1", approval_id: "approval-1",
      proposal_id: "proposal-1", tool_name: "shell", action: "approve_once",
      status: "approved", replayed: false, process_execution_enabled: false,
      shell_execution_enabled: false, docker_execution_enabled: false,
      workspace_write_applied: false, session_grant_created: false, capability_grant: false,
    };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-approval-queue", data: queue,
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-approval-decision", data: decision,
      }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, approvalControlEnabled: true,
    });
    await expect(client.approvalQueue("run-1")).resolves.toEqual(queue);
    await expect(client.decideApproval("run-1", "approval-1", {
      version: "approval_control.v1", action: "approve_once",
    }, "web-approval-operation-0001")).resolves.toEqual(decision);
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/runs/run-1/approvals");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/v1/runs/run-1/approvals/approval-1/decision");
    const decisionInit = fetchMock.mock.calls[1]?.[1] as RequestInit;
    expect(decisionInit.headers).toMatchObject({ Authorization: "Bearer control-secret" });

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-approval-unbound",
      data: { ...queue, items: [{ ...queue.items[0], session_id: "" }] },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    await expect(client.approvalQueue("run-1")).rejects.toThrow("invalid");
  });

  it("validates content-free model diagnostics and exact persisted routes", async () => {
    const route = { name: "code", provider: "mock", model: "mock-code", available: true };
    const diagnostic = {
      protocol_version: "provider_diagnostic.v1", provider: "mock", model: "mock-code",
      status: "reachable", outcome: "success", retryable: false,
      network_request_attempted: false, model_called: true, tool_called: false,
      response_content_returned: false, duration_ms: 2,
    };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-route", data: route,
      }), { status: 202, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-diagnostic", data: diagnostic,
      }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, modelControlEnabled: true,
    });
    await expect(client.selectModelRoute("code", {
      version: "model_route_control.v1", provider: "mock", model: "mock-code",
    })).resolves.toEqual(route);
    await expect(client.diagnoseProvider({
      version: "provider_diagnostic.v1", provider: "mock", model: "mock-code",
      confirm_diagnostic: true,
    })).resolves.toEqual(diagnostic);
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).headers)
      .not.toHaveProperty("Idempotency-Key");

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-diagnostic-forged",
      data: { ...diagnostic, response_content_returned: true, response: "private" },
    }), { status: 202, headers: { "Content-Type": "application/json" } }));
    await expect(client.diagnoseProvider({
      version: "provider_diagnostic.v1", provider: "mock", model: "mock-code",
      confirm_diagnostic: true,
    })).rejects.toThrow("content-free");
  });

  it("rejects FileEdit body leakage and validates review-only decisions", async () => {
    const edit = { id: "edit-1", session_id: "session-1", workspace_id: "workspace-1",
      path: "README.md", status: "proposed", diff: "--- a/README.md\n+++ b/README.md\n",
      original_hash: "missing", proposed_hash: "a".repeat(64), secrets_redacted: false,
      allowed_actions: ["approve_intent", "deny"], created_at: "2026-07-18T00:00:00Z",
      updated_at: "2026-07-18T00:00:00Z", apply_enabled: false };
    const queue = { protocol_version: "file_edit_review.v1", run_id: "run-1",
      items: [edit], truncated: false, apply_enabled: false };
    const decided = { protocol_version: "file_edit_review.v1", run_id: "run-1",
      action: "approve_intent", edit: { ...edit, status: "approved", allowed_actions: [] },
      replayed: false, file_written: false };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-edits", data: queue,
      }), { status: 200, headers: { "Content-Type": "application/json" } }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        version: "api.v1", request_id: "req-edit-review", data: decided,
      }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, fileEditReviewEnabled: true,
    });
    await expect(client.fileEditQueue("run-1")).resolves.toEqual(queue);
    await expect(client.reviewFileEdit("run-1", "edit-1", {
      version: "file_edit_review.v1", action: "approve_intent",
    })).resolves.toEqual(decided);

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-edit-leak",
      data: { ...queue, items: [{ ...edit, proposed_text: "private body" }] },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    await expect(client.fileEditQueue("run-1")).rejects.toThrow("metadata-only");
  });

  it("validates bounded wake scheduling without accepting execution authority", async () => {
    const intent = { id: "wake-1", protocol_version: "run_wake_intent.v1", run_id: "run-1",
      session_id: "session-1", status: "queued", max_attempts: 3, attempt_count: 0,
      initial_delay_seconds: 0, base_backoff_seconds: 5, max_backoff_seconds: 60,
      max_elapsed_seconds: 300, next_wake_at: "2026-07-18T00:00:00Z",
      deadline_at: "2026-07-18T00:05:00Z", execution_enabled: false,
      background_loop_enabled: false, created_at: "2026-07-18T00:00:00Z",
      updated_at: "2026-07-18T00:00:00Z" };
    const result = { protocol_version: "run_wake_control.v1", action: "schedule", intent,
      replayed: false, execution_started: false, model_called: false, tool_called: false };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-wake", data: result,
    }), { status: 202, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, runWakeControlEnabled: true,
    });
    await expect(client.scheduleRunWake("run-1", {
      version: "run_wake_control.v1", max_attempts: 3, initial_delay_seconds: 0,
      base_backoff_seconds: 5, max_backoff_seconds: 60, max_elapsed_seconds: 300,
    }, "web-wake-operation-0001")).resolves.toEqual(result);
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).headers)
      .toMatchObject({ "Idempotency-Key": "web-wake-operation-0001" });

    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({
      version: "api.v1", request_id: "req-wake-forged",
      data: { ...result, execution_started: true },
    }), { status: 202, headers: { "Content-Type": "application/json" } }));
    await expect(client.scheduleRunWake("run-1", {
      version: "run_wake_control.v1", max_attempts: 3, initial_delay_seconds: 0,
      base_backoff_seconds: 5, max_backoff_seconds: 60, max_elapsed_seconds: 300,
    }, "web-wake-operation-0002")).rejects.toThrow("authority");
  });

  it("validates apply, foreground wake, and inert Skill installation boundaries", async () => {
    const appliedEdit = { id: "edit-1", session_id: "session-1", workspace_id: "workspace-1",
      path: "safe.txt", status: "applied", diff: "--- safe.txt\n+++ safe.txt\n+ok\n",
      original_hash: "missing", proposed_hash: "a".repeat(64), secrets_redacted: false,
      allowed_actions: [], created_at: "2026-07-18T00:00:00Z",
      updated_at: "2026-07-18T00:00:01Z", apply_enabled: false };
    const applyResult = { protocol_version: "file_edit_apply.v1", run_id: "run-1",
      edit: appliedEdit, status: "applied", replayed: false, file_written: true,
      policy_rechecked: true, receipt: operationReceipt("file_edit_apply", "applied",
        "same_operation_key", "complete") };
    const completedIntent = { id: "wake-1", protocol_version: "run_wake_intent.v1",
      run_id: "run-1", session_id: "session-1", status: "completed", max_attempts: 3,
      attempt_count: 1, initial_delay_seconds: 0, base_backoff_seconds: 5,
      max_backoff_seconds: 60, max_elapsed_seconds: 300,
      next_wake_at: "2026-07-18T00:05:00Z", deadline_at: "2026-07-18T00:05:00Z",
      execution_enabled: false, background_loop_enabled: false,
      created_at: "2026-07-18T00:00:00Z", updated_at: "2026-07-18T00:00:01Z" };
    const wakeResult = { protocol_version: "run_wake_consumer.v1", run_id: "run-1",
      intent: completedIntent, consumption_status: "completed", stop_reason: "waiting",
      replayed: false, execution_started: true, model_called: true, tool_called: false,
      background_loop_enabled: false, receipt: operationReceipt("run_wake_consume", "completed",
        "same_wake_generation", "not_applicable") };
    const skillResult = { protocol_version: "skill_package_installation.v1",
      name: "review-helper", version: "1.0.0", surface: "code",
      trust_class: "operator_installed_untrusted", archive_sha256: "b".repeat(64),
      package_fingerprint: "c".repeat(64), replayed: false, recovered_pending: false,
      import_command_execution: false, import_network_access: false,
      import_provider_calls: false, tool_capability_grant: false,
      run_selection_authorized: false, context_injection_authorized: false,
      receipt: operationReceipt("skill_package_install", "installed",
        "same_operation_key", "not_applicable") };
    const envelope = (requestID: string, data: unknown) => new Response(JSON.stringify({
      version: "api.v1", request_id: requestID, data,
    }), { status: 202, headers: { "Content-Type": "application/json" } });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(envelope("req-apply", applyResult))
      .mockResolvedValueOnce(envelope("req-consume", wakeResult))
      .mockResolvedValueOnce(envelope("req-skill", skillResult))
      .mockResolvedValueOnce(envelope("req-skill-forged", {
        ...skillResult, import_command_execution: true,
      }))
      .mockResolvedValueOnce(envelope("req-apply-mismatch", {
        ...applyResult, status: "failed", edit: { ...appliedEdit, status: "failed" },
      }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, fileEditApplyEnabled: true,
      runWakeExecutionEnabled: true, skillInstallationEnabled: true,
    });
    await expect(client.applyFileEdit("run-1", "edit-1", {
      version: "file_edit_apply.v1",
    }, "web-file-apply-operation-0001")).resolves.toEqual(applyResult);
    await expect(client.consumeRunWake("run-1", {
      version: "run_wake_consumer.v1", max_steps: 1,
    })).resolves.toEqual(wakeResult);
    const skillRequest = { version: "skill_package_installation.v1" as const,
      archive_base64: "UEsDBA==", surface: "code" as const, confirm_untrusted: true };
    await expect(client.installSkillPackage(skillRequest,
      "web-skill-install-operation-0001")).resolves.toEqual(skillResult);
    expect((fetchMock.mock.calls[0]?.[1] as RequestInit).headers)
      .toMatchObject({ "Idempotency-Key": "web-file-apply-operation-0001" });
    expect((fetchMock.mock.calls[1]?.[1] as RequestInit).headers)
      .not.toHaveProperty("Idempotency-Key");
    await expect(client.installSkillPackage(skillRequest,
      "web-skill-install-operation-0002")).rejects.toThrow("inert Registry authority");
    await expect(client.applyFileEdit("run-1", "edit-1", {
      version: "file_edit_apply.v1",
    }, "web-file-apply-operation-0002")).rejects.toThrow("durable recovery contract");
  });

  it("validates bounded Workspace evidence without accepting local root authority", async () => {
    const snapshot = { protocol_version: "workspace_explorer.v1", workspace_id: "workspace-1",
      path: "src", kind: "directory", entries: [{ name: "main.go", path: "src/main.go",
        kind: "file", size_bytes: 120, readable: true }], content: "", total_bytes: 0,
      returned_bytes: 0, truncated: false, redaction_count: 0, root_path_exposed: false,
      provenance: { version: "context_provenance.v1", source_kind: "workspace_listing",
        source_ref: "src", content_sha256: "a".repeat(64), instruction_authorized: false } };
    const envelope = (data: unknown) => new Response(JSON.stringify({
      version: "api.v1", request_id: "req-explorer", data,
    }), { status: 200, headers: { "Content-Type": "application/json" } });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(envelope(snapshot))
      .mockResolvedValueOnce(envelope({ ...snapshot, root_path: "C:\\private" }))
      .mockResolvedValueOnce(envelope({ ...snapshot, entries: [
        { ...snapshot.entries[0], path: "other/main.go" },
      ] }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret");
    await expect(client.workspaceExplore("workspace-1", "src")).resolves.toEqual(snapshot);
    expect(String(fetchMock.mock.calls[0]?.[0])).toContain("path=src");
    await expect(client.workspaceExplore("workspace-1", "src"))
      .rejects.toThrow("bounded evidence contract");
    await expect(client.workspaceExplore("workspace-1", "src"))
      .rejects.toThrow("renderer path authority");
    await expect(client.workspaceExplore("workspace-1", "../private"))
      .rejects.toThrow("Go-issued relative path");
    await expect(client.workspaceExplore("workspace-1", "C:private"))
      .rejects.toThrow("Go-issued relative path");
  });

  it("validates Workspace search, evidence attachment, and metadata-only receipt history", async () => {
    const provenance = { version: "context_provenance.v1", source_kind: "workspace_file",
      source_ref: "README.md", content_sha256: "d".repeat(64),
      instruction_authorized: false };
    const search = { protocol_version: "workspace_search.v1", workspace_id: "workspace-1",
      results: [{ path: "README.md", match_kind: "content", line: 2,
        snippet: "Notes for automated assistants", content_truncated: false, provenance }],
      scanned_entries: 1, scanned_files: 1, scanned_bytes: 80,
      truncated: false, root_path_exposed: false };
    const attachment = { protocol_version: "session_evidence_attachment.v1",
      attachment_id: "evidence-1", run_id: "run-1", session_id: "session-1",
      workspace_id: "workspace-1", source_kind: "workspace_file", source_ref: "README.md",
      content_sha256: provenance.content_sha256, session_message_id: 8,
      instruction_authorized: false, replayed: false, execution_started: false,
      model_called: false, tool_called: false, capability_grant: false };
    const receipt = operationReceipt("file_edit_apply", "applied",
      "same_operation_key", "complete");
    const history = { protocol_version: "operation_receipt_history.v1", truncated: false,
      items: [{ id: "receipt-opaque", scope: "run", run_id: "run-1",
        completed_at: "2026-07-19T10:00:00Z", receipt }] };
    const envelope = (requestID: string, data: unknown, status = 200) =>
      new Response(JSON.stringify({ version: "api.v1", request_id: requestID, data }),
        { status, headers: { "Content-Type": "application/json" } });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(envelope("req-search", search))
      .mockResolvedValueOnce(envelope("req-evidence", attachment, 202))
      .mockResolvedValueOnce(envelope("req-history", history))
      .mockResolvedValueOnce(envelope("req-history-forged", {
        ...history, items: [{ ...history.items[0], receipt: {
          ...receipt, kind: "shell_execute", outcome: "completed",
        } }],
      }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret", "/api/v1", "control-secret", {
      runControlEnabled: false, evidenceAttachmentEnabled: true,
    });

    await expect(client.workspaceSearch("workspace-1", "automated assistants"))
      .resolves.toEqual(search);
    await expect(client.attachEvidence("run-1", {
      version: "session_evidence_attachment.v1", source_kind: "workspace_file",
      source_ref: "README.md", content_sha256: provenance.content_sha256,
    }, "web-evidence-operation-0001")).resolves.toEqual(attachment);
    await expect(client.operationReceiptHistory("run-1")).resolves.toEqual(history);
    const [searchURL] = fetchMock.mock.calls[0] as [string, RequestInit];
    const [attachURL, attachInit] = fetchMock.mock.calls[1] as [string, RequestInit];
    const [historyURL] = fetchMock.mock.calls[2] as [string, RequestInit];
    expect(searchURL).toContain("/workspaces/workspace-1/search?query=automated+assistants");
    expect(attachURL).toBe("/api/v1/runs/run-1/evidence-attachments");
    expect(attachInit.headers).toMatchObject({ Authorization: "Bearer control-secret",
      "Idempotency-Key": "web-evidence-operation-0001" });
    expect(String(attachInit.body)).not.toContain("control-secret");
    expect(historyURL).toContain("/operation-receipts?run_id=run-1&limit=100");
    await expect(client.operationReceiptHistory("run-1"))
      .rejects.toThrow("unsupported terminal result");
  });

  it("validates bounded operator actions and metadata-only evidence inventory", async () => {
    const inventory = { protocol_version: "session_evidence_inventory.v1", run_id: "run-1",
      truncated: false, items: [{ attachment_id: "evidence-1", run_id: "run-1",
        session_id: "session-1", workspace_id: "workspace-1", source_kind: "workspace_file",
        source_ref: "README.md", content_sha256: "c".repeat(64),
        instruction_authorized: false, attached_at: "2026-07-19T10:00:00Z" }] };
    const actions = { protocol_version: "operator_action_center.v1", run_id: "run-1",
      generated_at: "2026-07-19T12:00:00Z", truncated: false,
      items: [{ id: "action-opaque", kind: "wake_due", state: "queued",
        destination: "wake", available_at: "2026-07-19T11:00:00Z",
        due_at: "2026-07-19T11:00:00Z" }] };
    const envelope = (requestID: string, data: unknown) => new Response(JSON.stringify({
      version: "api.v1", request_id: requestID, data,
    }), { status: 200, headers: { "Content-Type": "application/json" } });
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(envelope("req-inventory", inventory))
      .mockResolvedValueOnce(envelope("req-actions", actions))
      .mockResolvedValueOnce(envelope("req-forged-actions", {
        ...actions, items: [{ ...actions.items[0], due_at: "2026-07-20T11:00:00Z" }],
      }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new CyberAgentClient("read-secret");

    await expect(client.evidenceInventory("run-1")).resolves.toEqual(inventory);
    await expect(client.operatorActionCenter("run-1")).resolves.toEqual(actions);
    expect(String(fetchMock.mock.calls[0]?.[0])).toBe(
      "/api/v1/runs/run-1/evidence-attachments");
    expect(String(fetchMock.mock.calls[1]?.[0])).toBe(
      "/api/v1/runs/run-1/operator-actions");
    await expect(client.operatorActionCenter("run-1"))
      .rejects.toThrow("closed navigation contract");
  });

  it("polls Run events with a stream-compatible opaque cursor and validates the envelope", async () => {
    const frame: RunEventStreamView = {
      version: "run-events.v1",
      request_id: "req-poll",
      run_id: "run-1",
      cursor: "opaque-2",
      sequence: 2,
      event: {
        event_id: "event-2",
        version: "v1",
        run_id: "run-1",
        mission_id: "mission-1",
        sequence: 2,
        type: "run.updated",
        source: "test",
        payload: {},
        created_at: "2026-07-18T00:00:00Z",
      },
    };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1",
      request_id: "req-poll",
      data: {
        version: "run-event-poll.v1",
        run_id: "run-1",
        cursor: "opaque-2",
        frames: [frame],
        has_more: false,
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    vi.stubGlobal("fetch", fetchMock);

    const result = await new CyberAgentClient("read-secret").pollRunEvents("run-1", "opaque-1", 25);

    expect(result.frames).toEqual([frame]);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain("/api/v1/runs/run-1/events/poll?");
    expect(url).toContain("cursor=opaque-1");
    expect(url).toContain("limit=25");
    expect(url).not.toContain("read-secret");
    expect(init.headers).toMatchObject({ Authorization: "Bearer read-secret" });
  });

  it("rejects a poll cursor that does not match the final validated frame", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1",
      request_id: "req-poll",
      data: {
        version: "run-event-poll.v1",
        run_id: "run-1",
        cursor: "forged-final",
        has_more: false,
        frames: [{
          version: "run-events.v1",
          request_id: "req-poll",
          run_id: "run-1",
          cursor: "actual-final",
          sequence: 1,
          event: {
            event_id: "event-1", version: "v1", run_id: "run-1", mission_id: "mission-1",
            sequence: 1, type: "run.created", source: "test", payload: {},
            created_at: "2026-07-18T00:00:00Z",
          },
        }],
      },
    }), { status: 200, headers: { "Content-Type": "application/json" } })));

    await expect(new CyberAgentClient("read-secret").pollRunEvents("run-1"))
      .rejects.toThrow("final frame");
  });

  it("resumes SSE with Last-Event-ID and validates the matching cursor", async () => {
    const frame: RunEventStreamView = {
      version: "run-events.v1",
      request_id: "req-stream",
      run_id: "run-1",
      cursor: "cursor-2",
      sequence: 2,
      event: {
        event_id: "event-2",
        version: "v1",
        run_id: "run-1",
        mission_id: "mission-1",
        sequence: 2,
        type: "run.updated",
        source: "test",
        payload: {},
        created_at: "2026-07-13T00:00:00Z",
      },
    };
    const body = `id: cursor-2\nevent: run.event\ndata: ${JSON.stringify(frame)}\n\n`;
    const fetchMock = vi.fn().mockResolvedValue(new Response(body, {
      status: 200,
      headers: { "Content-Type": "text/event-stream" },
    }));
    vi.stubGlobal("fetch", fetchMock);
    const received: RunEventStreamView[] = [];
    const controller = new AbortController();

    await new CyberAgentClient("read-secret").streamRunEvents("run-1", {
      cursor: "cursor-1",
      signal: controller.signal,
      onFrame: (value) => received.push(value),
    });

    expect(received).toEqual([frame]);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).not.toContain("cursor");
    expect(init.headers).toMatchObject({
      Accept: "text/event-stream",
      Authorization: "Bearer read-secret",
      "Last-Event-ID": "cursor-1",
    });
  });

  it("rejects a run event frame without the matching SSE id", async () => {
    const frame = {
      version: "run-events.v1",
      request_id: "req-stream",
      run_id: "run-1",
      cursor: "cursor-1",
      sequence: 1,
      event: {
        event_id: "event-1",
        version: "v1",
        run_id: "run-1",
        mission_id: "mission-1",
        sequence: 1,
        type: "run.created",
        source: "test",
        payload: {},
        created_at: "2026-07-13T00:00:00Z",
      },
    };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
      `event: run.event\ndata: ${JSON.stringify(frame)}\n\n`,
      { status: 200, headers: { "Content-Type": "text/event-stream" } },
    )));

    await expect(new CyberAgentClient("read-secret").streamRunEvents("run-1", {
      signal: new AbortController().signal,
      onFrame: () => undefined,
    })).rejects.toThrow("id does not match");
  });
});

function operationReceipt(kind: "file_edit_apply" | "run_wake_consume" | "skill_package_install",
  outcome: "applied" | "completed" | "installed",
  retryStrategy: "same_operation_key" | "same_wake_generation",
  cleanupState: "complete" | "not_applicable") {
  return { protocol_version: "operation_receipt.v1", kind, outcome, durable: true,
    replayed: false, retry_safe: true, retry_strategy: retryStrategy,
    recovery_action: "none", cleanup_state: cleanupState };
}
