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
});

function renderPanel(client: CyberAgentClient) {
  const queryClient = new QueryClient({ defaultOptions: {
    queries: { retry: false }, mutations: { retry: false },
  } });
  return render(<QueryClientProvider client={queryClient}>
    <FileEditPanel client={client} runID="run-1" />
  </QueryClientProvider>);
}
