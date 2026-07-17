import { renderHook, waitFor } from "@testing-library/react";
import { vi } from "vitest";
import type { CyberAgentClient } from "../api/client";
import type { EventView } from "../api/types";
import { useRunEventStream } from "./use-run-event-stream";

vi.mock("../lib/desktop-bridge", () => ({ desktopRuntimeActive: () => true }));

describe("useRunEventStream Desktop fallback", () => {
  it("polls opaque event pages instead of opening an unsupported Windows stream", async () => {
    const first = event(1);
    const second = event(2);
    const getPage = vi.fn()
      .mockResolvedValueOnce({
        items: [first], requestID: "req-1", page: { limit: 100, next_cursor: "opaque-next" },
      })
      .mockResolvedValue({
        items: [second], requestID: "req-2", page: { limit: 100 },
      });
    const streamRunEvents = vi.fn();
    const client = { getPage, streamRunEvents } as unknown as CyberAgentClient;
    const { result, unmount } = renderHook(() => useRunEventStream(client, "run-desktop"));

    await waitFor(() => expect(result.current.frames.map((frame) => frame.sequence)).toEqual([1, 2]));
    expect(result.current.status).toBe("live");
    expect(streamRunEvents).not.toHaveBeenCalled();
    expect(getPage).toHaveBeenNthCalledWith(1, "/runs/run-desktop/events", { limit: 100 }, "", expect.any(AbortSignal));
    expect(getPage).toHaveBeenNthCalledWith(2, "/runs/run-desktop/events", { limit: 100 }, "opaque-next", expect.any(AbortSignal));
    unmount();
  });
});

function event(sequence: number): EventView {
  return {
    version: "event.v1",
    event_id: `event-${sequence}`,
    mission_id: "mission-desktop",
    run_id: "run-desktop",
    sequence,
    type: "run.updated",
    source: "test",
    payload: {},
    created_at: "2026-07-18T00:00:00Z",
  };
}
