import { CyberAgentClient } from "./client";
import type { RunEventStreamView } from "./types";

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
    id: "steer-1", sequence: 1, status: "pending", created_at: "2026-07-18T00:00:00Z",
  },
  replayed: false,
  execution_started: false,
  model_called: false,
  tool_called: false,
  capability_grant: false,
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

  it("polls Run events with a stream-compatible opaque cursor and validates the envelope", async () => {
    const frame: RunEventStreamView = {
      version: "run-events.v1",
      request_id: "req-poll",
      run_id: "run-1",
      cursor: "opaque-2",
      sequence: 2,
      event: {
        event_id: "event-2",
        version: "event.v1",
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
            event_id: "event-1", version: "event.v1", run_id: "run-1", mission_id: "mission-1",
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
        version: "event.v1",
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
        version: "event.v1",
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
