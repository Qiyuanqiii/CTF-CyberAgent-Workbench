import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CyberAgentClient } from "../api/client";
import { FileEditPanel } from "./file-edit-panel";

describe("FileEditPanel", () => {
  it("renders the bounded Diff and approves intent without an apply operation", async () => {
    const user = userEvent.setup();
    const edit = {
      id: "edit-1", session_id: "session-1", workspace_id: "workspace-1",
      path: "README.md", status: "proposed", diff: "--- README.md\n+++ README.md\n-old\n+new",
      original_hash: "a".repeat(64), proposed_hash: "b".repeat(64),
      secrets_redacted: true, allowed_actions: ["approve_intent", "deny"],
      created_at: "2026-07-18T00:00:00Z", updated_at: "2026-07-18T00:00:00Z",
      apply_enabled: false,
    } as const;
    const reviewFileEdit = vi.fn().mockResolvedValue({
      protocol_version: "file_edit_review.v1", run_id: "run-1",
      action: "approve_intent", edit: { ...edit, status: "approved", allowed_actions: [] },
      replayed: false, file_written: false,
    });
    const client = {
      hasFileEditReview: true,
      fileEditQueue: vi.fn().mockResolvedValue({
        protocol_version: "file_edit_review.v1", run_id: "run-1", items: [edit],
        truncated: false, apply_enabled: false,
      }),
      reviewFileEdit,
    } as unknown as CyberAgentClient;

    renderPanel(client);
    expect(await screen.findByText("Apply authority: disabled")).toBeInTheDocument();
    expect(screen.getByText(/\+new/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Approve intent README.md" }));
    await waitFor(() => expect(reviewFileEdit).toHaveBeenCalledTimes(1));
    expect(reviewFileEdit.mock.calls[0]?.slice(0, 3)).toEqual([
      "run-1", "edit-1", { version: "file_edit_review.v1", action: "approve_intent" },
    ]);
  });

  it("applies only an approved edit with a memory-only retry key", async () => {
    const user = userEvent.setup();
    const edit = {
      id: "edit-approved", session_id: "session-1", workspace_id: "workspace-1",
      path: "safe.txt", status: "approved", diff: "--- safe.txt\n+++ safe.txt\n+value",
      original_hash: "a".repeat(64), proposed_hash: "b".repeat(64),
      secrets_redacted: false, allowed_actions: [], apply_enabled: true,
      created_at: "2026-07-18T00:00:00Z", updated_at: "2026-07-18T00:00:00Z",
    } as const;
    const applyFileEdit = vi.fn().mockResolvedValue({
      protocol_version: "file_edit_apply.v1", run_id: "run-1", status: "applied",
      edit: { ...edit, status: "applied", apply_enabled: false },
      replayed: false, file_written: true, policy_rechecked: true,
      receipt: { protocol_version: "operation_receipt.v1", kind: "file_edit_apply",
        outcome: "applied", durable: true, replayed: false, retry_safe: true,
        retry_strategy: "same_operation_key", recovery_action: "none",
        cleanup_state: "complete" },
    });
    const client = {
      hasFileEditReview: true, hasFileEditApply: true,
      fileEditQueue: vi.fn().mockResolvedValue({
        protocol_version: "file_edit_review.v1", run_id: "run-1", items: [edit],
        truncated: false, apply_enabled: true,
      }),
      reviewFileEdit: vi.fn(), applyFileEdit,
    } as unknown as CyberAgentClient;
    renderPanel(client);
    await user.click(await screen.findByRole("button", { name: "Apply safe.txt" }));
    await waitFor(() => expect(applyFileEdit).toHaveBeenCalledTimes(1));
    expect(applyFileEdit.mock.calls[0]?.slice(0, 3)).toEqual([
      "run-1", "edit-approved", { version: "file_edit_apply.v1" },
    ]);
    expect(applyFileEdit.mock.calls[0]?.[3]).toMatch(/^web-file-apply-/);
    expect(await screen.findByText("file edit apply / durable")).toBeInTheDocument();
  });
});

function renderPanel(client: CyberAgentClient) {
  const queryClient = new QueryClient({ defaultOptions: {
    queries: { retry: false }, mutations: { retry: false },
  } });
  return render(<QueryClientProvider client={queryClient}>
    <FileEditPanel client={client} runID="run-1" />
  </QueryClientProvider>);
}
