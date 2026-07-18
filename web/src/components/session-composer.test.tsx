import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import type { CyberAgentClient } from "../api/client";
import type { OperatorSteeringQueueView, RunView, SessionMessageControlView,
  SessionSteeringCancellationView } from "../api/types";
import { SessionComposer, SessionSteeringQueue } from "./session-composer";

const result: SessionMessageControlView = {
  version: "session_message_submission.v1",
  run_id: "run-1",
  session_id: "sess-1",
  steering: {
    id: "steer-1",
    sequence: 3,
    status: "pending",
    prepared: false,
    created_at: "2026-07-18T00:00:00Z",
  },
  replayed: false,
  execution_started: false,
  model_called: false,
  tool_called: false,
  capability_grant: false,
};

const runningRun = { id: "run-1", status: "running" } as RunView;

const cancellationResult: SessionSteeringCancellationView = {
  version: "session_steering_cancellation.v1",
  run_id: "run-1", session_id: "sess-1", cancellation_id: "cancel-1",
  cancellation_kind: "operator", replayed: false,
  steering: {
    id: "steer-1", sequence: 3, status: "cancelled", prepared: false,
    created_at: "2026-07-18T00:00:00Z", cancelled_at: "2026-07-18T00:01:00Z",
  },
  execution_started: false, model_called: false, tool_called: false, capability_grant: false,
};

describe("SessionComposer", () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
  });

  it("reuses an in-memory operation key after uncertain failure and clears on success", async () => {
    const submitSessionMessage = vi.fn()
      .mockRejectedValueOnce(new Error("response unavailable"))
      .mockResolvedValueOnce(result);
    const client = {
      hasSessionMessages: true,
      submitSessionMessage,
    } as unknown as CyberAgentClient;
    const user = userEvent.setup();
    renderComposer(client, runningRun);

    await user.type(screen.getByLabelText("Session message"), "Review the latest diff");
    await user.click(screen.getByRole("button", { name: "Queue message" }));
    await screen.findByText("response unavailable");
    await user.click(screen.getByRole("button", { name: "Queue message" }));
    await screen.findByText("Queued #3");

    expect(submitSessionMessage).toHaveBeenCalledTimes(2);
    const first = submitSessionMessage.mock.calls[0];
    const second = submitSessionMessage.mock.calls[1];
    expect(first?.[0]).toBe("sess-1");
    expect(first?.[1]).toEqual({
      version: "session_message_submission.v1", content: "Review the latest diff",
    });
    expect(first?.[2]).toBe(second?.[2]);
    expect(screen.getByLabelText("Session message")).toHaveValue("");
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });

  it("enforces the UTF-8 byte limit before issuing a request", async () => {
    const submitSessionMessage = vi.fn();
    const client = {
      hasSessionMessages: true,
      submitSessionMessage,
    } as unknown as CyberAgentClient;
    renderComposer(client, runningRun);

    fireEvent.change(screen.getByLabelText("Session message"), {
      target: { value: "测".repeat(6000) },
    });
    expect(screen.getByText("Message exceeds 16384 UTF-8 bytes")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Queue message" })).toBeDisabled();
    expect(submitSessionMessage).not.toHaveBeenCalled();
  });

  it("renders only for an enabled Run-bound capability and fails closed by Run status", async () => {
    const disabled = {
      hasSessionMessages: false,
      submitSessionMessage: vi.fn(),
    } as unknown as CyberAgentClient;
    const { rerender } = renderComposer(disabled, runningRun);
    expect(screen.queryByLabelText("Session message")).not.toBeInTheDocument();

    const enabled = {
      hasSessionMessages: true,
      submitSessionMessage: vi.fn(),
    } as unknown as CyberAgentClient;
    rerender(withProvider(<SessionComposer client={enabled} sessionID="sess-1"
      run={{ ...runningRun, status: "created" }} />));
    await waitFor(() => expect(screen.getByLabelText("Session message")).toBeDisabled());
    expect(screen.getByText("Run unavailable")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Queue message" })).toBeDisabled();
  });
});

describe("SessionSteeringQueue", () => {
  it("cancels only pending metadata and reuses the in-memory retry key", async () => {
    const cancelSessionSteering = vi.fn()
      .mockRejectedValueOnce(new Error("response unavailable"))
      .mockResolvedValueOnce(cancellationResult);
    const client = {
      hasSessionSteeringControl: true,
      cancelSessionSteering,
    } as unknown as CyberAgentClient;
    const state = {
      pending: 1, prepared: 0, committed: 1, cancelled: 0,
      messages: [
        { id: "steer-1", sequence: 3, status: "pending", prepared: false,
          created_at: "2026-07-18T00:00:00Z" },
        { id: "steer-2", sequence: 2, status: "committed", created_at: "2026-07-18T00:00:00Z",
          committed_at: "2026-07-18T00:00:30Z", prepared: false },
      ],
    } as OperatorSteeringQueueView;
    const user = userEvent.setup();
    render(withProvider(<SessionSteeringQueue client={client} sessionID="sess-1" state={state} />));

    expect(screen.queryByRole("button", { name: "Cancel queued message 2" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cancel queued message 3" }));
    await screen.findByText("response unavailable");
    await user.click(screen.getByRole("button", { name: "Cancel queued message 3" }));
    await waitFor(() => expect(cancelSessionSteering).toHaveBeenCalledTimes(2));

    expect(cancelSessionSteering.mock.calls[0]?.[0]).toBe("sess-1");
    expect(cancelSessionSteering.mock.calls[0]?.[1]).toBe("steer-1");
    expect(cancelSessionSteering.mock.calls[0]?.[2]).toEqual({
      version: "session_steering_cancellation.v1",
      reason: "operator cancelled queued Session message",
    });
    expect(cancelSessionSteering.mock.calls[0]?.[3]).toBe(
      cancelSessionSteering.mock.calls[1]?.[3]);
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });

  it("stays hidden without its distinct capability", () => {
    const client = { hasSessionSteeringControl: false } as CyberAgentClient;
    render(withProvider(<SessionSteeringQueue client={client} sessionID="sess-1" state={{
      pending: 1, prepared: 0, committed: 0, cancelled: 0,
      messages: [{ id: "steer-1", sequence: 1, status: "pending", prepared: false,
        created_at: "2026-07-18T00:00:00Z" }],
    }} />));
    expect(screen.queryByLabelText("Queued Session messages")).not.toBeInTheDocument();
  });

  it("does not offer cancellation for an already prepared message", () => {
    const client = { hasSessionSteeringControl: true } as CyberAgentClient;
    render(withProvider(<SessionSteeringQueue client={client} sessionID="sess-1" state={{
      pending: 0, prepared: 1, committed: 0, cancelled: 0,
      messages: [{ id: "steer-prepared", sequence: 1, status: "pending", prepared: true,
        created_at: "2026-07-18T00:00:00Z" }],
    }} />));
    expect(screen.queryByLabelText("Queued Session messages")).not.toBeInTheDocument();
  });
});

function renderComposer(client: CyberAgentClient, run: RunView | null) {
  return render(withProvider(<SessionComposer client={client} sessionID="sess-1" run={run} />));
}

function withProvider(node: ReactNode) {
  return <QueryClientProvider client={new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })}>{node}</QueryClientProvider>;
}
