import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import type { CyberAgentClient } from "../api/client";
import type { RunView, SessionMessageControlView } from "../api/types";
import { SessionComposer } from "./session-composer";

const result: SessionMessageControlView = {
  version: "session_message_submission.v1",
  run_id: "run-1",
  session_id: "sess-1",
  steering: {
    id: "steer-1",
    sequence: 3,
    status: "pending",
    created_at: "2026-07-18T00:00:00Z",
  },
  replayed: false,
  execution_started: false,
  model_called: false,
  tool_called: false,
  capability_grant: false,
};

const runningRun = { id: "run-1", status: "running" } as RunView;

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

function renderComposer(client: CyberAgentClient, run: RunView | null) {
  return render(withProvider(<SessionComposer client={client} sessionID="sess-1" run={run} />));
}

function withProvider(node: ReactNode) {
  return <QueryClientProvider client={new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })}>{node}</QueryClientProvider>;
}
