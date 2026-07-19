import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CyberAgentClient } from "../api/client";
import { VerificationEvidence } from "./verification-evidence";

describe("VerificationEvidence", () => {
  it("records one operator observation through the distinct capability", async () => {
    const verificationEvidence = vi.fn().mockResolvedValue({
      protocol_version: "operator_verification_inventory.v1", run_id: "run-1",
      session_id: "session-1", workspace_id: "workspace-1", items: [],
      pass_count: 0, fail_count: 0, unknown_count: 0, truncated: false,
    });
    const recordVerificationEvidence = vi.fn().mockResolvedValue({ id: "verification-1" });
    const client = { hasVerificationEvidence: true, verificationEvidence,
      recordVerificationEvidence } as unknown as CyberAgentClient;
    const queryClient = new QueryClient({ defaultOptions: {
      queries: { retry: false }, mutations: { retry: false },
    } });
    const user = userEvent.setup();
    render(<QueryClientProvider client={queryClient}>
      <VerificationEvidence client={client} runID="run-1" />
    </QueryClientProvider>);
    await screen.findByText("No verification evidence recorded");
    await user.selectOptions(screen.getByLabelText("Verification result"), "pass");
    await user.type(screen.getByLabelText("Title"), "Focused tests");
    await user.type(screen.getByLabelText("Summary"), "Go and React suites passed");
    await user.click(screen.getByRole("button", { name: "Record" }));
    await waitFor(() => expect(recordVerificationEvidence).toHaveBeenCalledTimes(1));
    expect(recordVerificationEvidence).toHaveBeenCalledWith("run-1", {
      version: "operator_verification_evidence.v1", outcome: "pass",
      title: "Focused tests", summary: "Go and React suites passed",
    }, expect.stringMatching(/^web-verification-/u));
  });
});
