import { CyberAgentClient } from "./client";
import type { RunEventStreamView } from "./types";

const healthEnvelope = {
  version: "api.v1",
  request_id: "req-health",
  data: { status: "ok", api_version: "api.v1", app_version: "test", schema_version: 37 },
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
