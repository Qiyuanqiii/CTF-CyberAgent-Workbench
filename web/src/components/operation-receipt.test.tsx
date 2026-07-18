import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { OperationReceipt } from "./operation-receipt";

describe("OperationReceipt", () => {
  it("does not present a durable failed FileEdit result as success", () => {
    render(<OperationReceipt receipt={{
      protocol_version: "operation_receipt.v1",
      kind: "file_edit_apply",
      outcome: "failed",
      durable: true,
      replayed: true,
      retry_safe: true,
      retry_strategy: "same_operation_key",
      recovery_action: "none",
      cleanup_state: "complete",
    }} />);

    expect(screen.getByRole("alert")).toHaveTextContent("failed");
    expect(screen.getByText(/will replay for the same operation key/i)).toBeInTheDocument();
  });
});
