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
      fileEditChangeSet: vi.fn().mockResolvedValue(changeSetFor(edit)),
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
      fileEditChangeSet: vi.fn().mockResolvedValue(changeSetFor(edit)),
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

  it("keeps mixed multi-file outcomes visible without a batch mutation", async () => {
    const applied = {
      id: "edit-applied", session_id: "session-1", workspace_id: "workspace-1",
      path: "applied.txt", status: "applied", diff: "+applied", original_hash: "missing",
      proposed_hash: "a".repeat(64), secrets_redacted: false, allowed_actions: [],
      apply_enabled: false, created_at: "2026-07-18T00:00:00Z",
      updated_at: "2026-07-18T00:00:00Z",
    } as const;
    const failed = { ...applied, id: "edit-failed", path: "failed.txt", status: "failed",
      diff: "+failed", proposed_hash: "b".repeat(64) } as const;
    const client = {
      hasFileEditReview: true, hasFileEditApply: true,
      fileEditQueue: vi.fn().mockResolvedValue({
        protocol_version: "file_edit_review.v1", run_id: "run-1",
        items: [applied, failed], truncated: false, apply_enabled: true,
      }),
      fileEditChangeSet: vi.fn().mockResolvedValue({
        protocol_version: "file_edit_change_set.v1", run_id: "run-1",
        session_id: "session-1", workspace_id: "workspace-1",
        items: [changeSetItem(applied), changeSetItem(failed)], proposed_count: 0,
        approved_count: 0, applied_count: 1, denied_count: 0, failed_count: 1,
        returned_count: 2, total_diff_bytes: 15, truncated: false,
        review_independent: true, apply_independent: true, atomic_apply: false,
        batch_mutation_supported: false, partial_apply_visible: true,
        diff_content_included: false,
      }),
      reviewFileEdit: vi.fn(), applyFileEdit: vi.fn(),
    } as unknown as CyberAgentClient;

    renderPanel(client);
    expect(await screen.findByText("partial")).toBeInTheDocument();
    expect(screen.getByText("applied.txt")).toBeInTheDocument();
    expect(screen.getByText("failed.txt")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /apply all/i })).not.toBeInTheDocument();
  });
});

function changeSetFor(edit: {
  id: string; path: string; status: string; diff: string; secrets_redacted: boolean;
  allowed_actions: readonly string[]; apply_enabled: boolean; updated_at: string;
}) {
  const item = changeSetItem(edit);
  return {
    protocol_version: "file_edit_change_set.v1", run_id: "run-1", session_id: "session-1",
    workspace_id: "workspace-1", items: [item], proposed_count: edit.status === "proposed" ? 1 : 0,
    approved_count: edit.status === "approved" ? 1 : 0,
    applied_count: edit.status === "applied" ? 1 : 0, denied_count: edit.status === "denied" ? 1 : 0,
    failed_count: edit.status === "failed" ? 1 : 0, returned_count: 1,
    total_diff_bytes: new TextEncoder().encode(edit.diff).length, truncated: false,
    review_independent: true, apply_independent: true, atomic_apply: false,
    batch_mutation_supported: false, partial_apply_visible: true, diff_content_included: false,
  };
}

function changeSetItem(edit: {
  id: string; path: string; status: string; diff: string; secrets_redacted: boolean;
  allowed_actions: readonly string[]; apply_enabled: boolean; updated_at: string;
}) {
  return { id: edit.id, path: edit.path, status: edit.status,
    diff_bytes: new TextEncoder().encode(edit.diff).length,
    secrets_redacted: edit.secrets_redacted, allowed_actions: [...edit.allowed_actions],
    apply_enabled: edit.apply_enabled, updated_at: edit.updated_at };
}

function renderPanel(client: CyberAgentClient) {
  const queryClient = new QueryClient({ defaultOptions: {
    queries: { retry: false }, mutations: { retry: false },
  } });
  return render(<QueryClientProvider client={queryClient}>
    <FileEditPanel client={client} runID="run-1" />
  </QueryClientProvider>);
}
