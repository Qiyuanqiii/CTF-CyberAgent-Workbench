import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CyberAgentClient } from "../api/client";
import type { RunDetailView, RunExecutionControlView, RunLifecycleControlView } from "../api/types";
import { RunControlPanel } from "./run-workspace";

function detail(status: RunDetailView["run"]["status"] = "created",
  pending = 0): RunDetailView {
  return {
    run: {
      id: "run-1", mission_id: "mission-1", session_id: "sess-1", status,
      config: { model_route: "code", interactive: true }, budget: { max_turns: 4 },
      created_at: "2026-07-18T00:00:00Z", updated_at: "2026-07-18T00:00:00Z",
    },
    operator_steering: {
      pending, prepared: 0, committed: 0, cancelled: 0, messages: [],
    },
  } as unknown as RunDetailView;
}

function provider(children: React.ReactNode) {
  return <QueryClientProvider client={new QueryClient({ defaultOptions: {
    queries: { retry: false }, mutations: { retry: false },
  } })}>{children}</QueryClientProvider>;
}

describe("RunControlPanel", () => {
  it("reuses an in-memory lifecycle key after failure and rotates it after success", async () => {
    const current = detail();
    const result = {
      version: "run_lifecycle_control.v1", run: { ...current.run, status: "running" },
      action: "start", expected_status: "created", applied_status: "running",
      event_sequence_start: 5, event_sequence_end: 6, replayed: false,
      execution_started: false, model_called: false, tool_called: false,
      capability_grant: false,
    } as RunLifecycleControlView;
    const controlRunLifecycle = vi.fn()
      .mockRejectedValueOnce(new Error("temporary failure"))
      .mockResolvedValueOnce(result);
    const client = {
      hasRunLifecycle: true, hasRunExecution: false, controlRunLifecycle,
    } as unknown as CyberAgentClient;
    const user = userEvent.setup();
    render(provider(<RunControlPanel client={client} detail={current} />));
    await user.click(screen.getByRole("button", { name: "Start" }));
    await screen.findByRole("alert");
    await user.click(screen.getByRole("button", { name: "Start" }));
    await waitFor(() => expect(controlRunLifecycle).toHaveBeenCalledTimes(2));
    expect(controlRunLifecycle.mock.calls[0]?.[2]).toBe(
      controlRunLifecycle.mock.calls[1]?.[2]);
    expect(controlRunLifecycle.mock.calls[0]?.[1]).toEqual({
      version: "run_lifecycle_control.v1", action: "start",
    });
  });

  it("executes only the selected bounded queue size and renders durable result metadata", async () => {
    const current = detail("running", 3);
    const executeRun = vi.fn().mockResolvedValue({
      version: "run_execution_handoff.v1", operation_id: "run-handoff-1",
      run_id: "run-1", session_id: "sess-1", max_steps: 2, selected_count: 2,
      status: "completed", run_status: "running", stop_reason: "selection_drained",
      steps_completed: 2, pending_count: 0, prepared_count: 0, committed_count: 2,
      cancelled_count: 0, completion_event_sequence: 12, replayed: false,
      execution_started: true, model_called: true, tool_called: false,
      capability_grant: false,
    } as RunExecutionControlView);
    const client = {
      hasRunLifecycle: false, hasRunExecution: true, executeRun,
    } as unknown as CyberAgentClient;
    const user = userEvent.setup();
    render(provider(<RunControlPanel client={client} detail={current} />));
    const steps = screen.getByLabelText("Steps");
    fireEvent.change(steps, { target: { value: "2" } });
    await user.click(screen.getByRole("button", { name: "Run queue" }));
    await waitFor(() => expect(executeRun).toHaveBeenCalledTimes(1));
    expect(executeRun.mock.calls[0]?.[1]).toEqual({
      version: "run_execution_handoff.v1", max_steps: 2,
    });
    expect(await screen.findByText("selection_drained")).toBeInTheDocument();
    expect(screen.getByText("2/2 steps")).toBeInTheDocument();
  });

  it("rotates the execution key when the bounded request intent changes", async () => {
    const current = detail("running", 3);
    const executeRun = vi.fn().mockRejectedValue(new Error("response unavailable"));
    const client = {
      hasRunLifecycle: false, hasRunExecution: true, executeRun,
    } as unknown as CyberAgentClient;
    const user = userEvent.setup();
    render(provider(<RunControlPanel client={client} detail={current} />));
    await user.click(screen.getByRole("button", { name: "Run queue" }));
    await screen.findByRole("alert");
    fireEvent.change(screen.getByLabelText("Steps"), { target: { value: "2" } });
    await user.click(screen.getByRole("button", { name: "Run queue" }));
    await waitFor(() => expect(executeRun).toHaveBeenCalledTimes(2));
    expect(executeRun.mock.calls[0]?.[2]).not.toBe(executeRun.mock.calls[1]?.[2]);
  });
});
