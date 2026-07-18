import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CyberAgentClient } from "../api/client";
import { OperationReceiptHistory } from "./operation-receipt-history";

describe("OperationReceiptHistory", () => {
  it("renders refreshable terminal metadata without source paths or digests", async () => {
    const operationReceiptHistory = vi.fn().mockResolvedValue({
      protocol_version: "operation_receipt_history.v1", truncated: false,
      items: [{ id: "receipt-opaque", scope: "run", run_id: "run-1",
        completed_at: "2026-07-19T10:00:00Z",
        receipt: { protocol_version: "operation_receipt.v1", kind: "file_edit_apply",
          outcome: "failed", durable: true, replayed: false, retry_safe: true,
          retry_strategy: "same_operation_key", recovery_action: "none",
          cleanup_state: "complete" } }],
    });
    const client = { operationReceiptHistory } as unknown as CyberAgentClient;
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const user = userEvent.setup();
    render(<QueryClientProvider client={queryClient}>
      <OperationReceiptHistory client={client} runID="run-1" />
    </QueryClientProvider>);

    expect(await screen.findByText("file edit apply / durable")).toBeInTheDocument();
    expect(screen.getByText("run")).toBeInTheDocument();
    expect(screen.queryByText(/PRIVATE|README\.md|[a-f0-9]{64}/i)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Refresh operation receipts" }));
    await waitFor(() => expect(operationReceiptHistory).toHaveBeenCalledTimes(2));
    expect(operationReceiptHistory).toHaveBeenLastCalledWith("run-1", expect.any(AbortSignal));
  });
});
