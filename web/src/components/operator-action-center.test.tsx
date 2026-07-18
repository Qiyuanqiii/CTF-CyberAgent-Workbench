import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CyberAgentClient } from "../api/client";
import { OperatorActionCenter } from "./operator-action-center";

describe("OperatorActionCenter", () => {
  it("renders opaque actions and navigates without exposing private facts", async () => {
    const operatorActionCenter = vi.fn().mockResolvedValue({
      protocol_version: "operator_action_center.v1", run_id: "run-1",
      generated_at: "2026-07-19T12:00:00Z", truncated: false,
      items: [{ id: "action-opaque", kind: "approval_pending", state: "pending",
        destination: "approvals", available_at: "2026-07-19T11:00:00Z" }],
    });
    const client = { operatorActionCenter } as unknown as CyberAgentClient;
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const onNavigate = vi.fn();
    const user = userEvent.setup();
    render(<QueryClientProvider client={queryClient}>
      <OperatorActionCenter client={client} onNavigate={onNavigate} runID="run-1" />
    </QueryClientProvider>);

    await user.click(await screen.findByRole("button", { name: /Approval review/ }));
    expect(onNavigate).toHaveBeenCalledWith("approvals");
    expect(screen.getByText("action-opaque")).toBeInTheDocument();
    expect(screen.queryByText(/PRIVATE|command|operation.key/i)).not.toBeInTheDocument();
  });
});
