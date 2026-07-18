import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CyberAgentClient } from "../api/client";
import type { RunDetailView } from "../api/types";
import { RunWakePanel } from "./run-wake-panel";

describe("RunWakePanel", () => {
  it("schedules bounded intent with a memory-only operation key", async () => {
    const user = userEvent.setup();
    const scheduleRunWake = vi.fn().mockResolvedValue({
      protocol_version: "run_wake_control.v1", action: "schedule", replayed: false,
      execution_started: false, model_called: false, tool_called: false,
      intent: wakeIntent("queued"),
    });
    const client = wakeClient(scheduleRunWake);
    renderPanel(client, runDetail(1, 0));
    await screen.findByText("idle");
    await user.click(screen.getByRole("button", { name: "Schedule" }));
    await waitFor(() => expect(scheduleRunWake).toHaveBeenCalledTimes(1));
    expect(scheduleRunWake.mock.calls[0]?.[0]).toBe("run-1");
    expect(scheduleRunWake.mock.calls[0]?.[1]).toEqual({
      version: "run_wake_control.v1", max_attempts: 3, initial_delay_seconds: 0,
      base_backoff_seconds: 5, max_backoff_seconds: 60, max_elapsed_seconds: 300,
    });
    expect(scheduleRunWake.mock.calls[0]?.[2]).toMatch(/^web-wake-schedule-/);
    expect(await screen.findAllByText("disabled")).toHaveLength(2);
  });

  it("does not treat prepared-only work as schedulable pending input", async () => {
    renderPanel(wakeClient(vi.fn()), runDetail(0, 1));
    const button = await screen.findByRole("button", { name: "Schedule" });
    expect(button).toBeDisabled();
  });
});

function wakeClient(scheduleRunWake: ReturnType<typeof vi.fn>): CyberAgentClient {
  return {
    hasRunWakeControl: true,
    runWakeState: vi.fn().mockResolvedValue({
      protocol_version: "run_wake_intent.v1", run_id: "run-1", found: false,
    }),
    scheduleRunWake,
    cancelRunWake: vi.fn(),
  } as unknown as CyberAgentClient;
}

function runDetail(pending: number, prepared: number): RunDetailView {
  return {
    run: { id: "run-1", mission_id: "mission-1", session_id: "session-1", status: "running",
      config: { model_route: "code", interactive: true }, budget: { max_turns: 4 },
      created_at: "2026-07-18T00:00:00Z", updated_at: "2026-07-18T00:00:00Z" },
    operator_steering: { pending, prepared, committed: 0, cancelled: 0, messages: [] },
  } as unknown as RunDetailView;
}

function wakeIntent(status: "queued") {
  return {
    id: "wake-1", protocol_version: "run_wake_intent.v1", run_id: "run-1",
    session_id: "session-1", status, max_attempts: 3, attempt_count: 0,
    initial_delay_seconds: 0, base_backoff_seconds: 5, max_backoff_seconds: 60,
    max_elapsed_seconds: 300, next_wake_at: "2026-07-18T00:00:00Z",
    deadline_at: "2026-07-18T00:05:00Z", execution_enabled: false,
    background_loop_enabled: false, created_at: "2026-07-18T00:00:00Z",
    updated_at: "2026-07-18T00:00:00Z",
  };
}

function renderPanel(client: CyberAgentClient, detail: RunDetailView) {
  const queryClient = new QueryClient({ defaultOptions: {
    queries: { retry: false }, mutations: { retry: false },
  } });
  return render(<QueryClientProvider client={queryClient}>
    <RunWakePanel client={client} detail={detail} />
  </QueryClientProvider>);
}
