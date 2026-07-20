import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import type { CyberAgentClient } from "../api/client";
import { RepositoryHistoryPanel } from "./repository-history-panel";

describe("RepositoryHistoryPanel", () => {
  it("renders bounded local branches and commit subjects", async () => {
    const repositoryHistory = vi.fn().mockResolvedValue({
      protocol_version: "repository_history.v1", workspace_id: "workspace-1",
      kind: "git", available: true, head: "abcdef123456", detached: false,
      commits: [{ hash: "abcdef123456", object_id: "abcdef1234567890abcdef1234567890abcdef12",
        subject: "bounded history", parent_count: 1,
        committed_at: "2026-07-19T01:00:00Z", redacted: false, subject_bounded: false }],
      branches: [{ name: "main", head: "abcdef123456", current: true }],
      returned_commit_count: 1, returned_branch_count: 1, omitted_branch_count: 0,
      redaction_count: 0, truncated: false, first_parent_only: true, read_only: true,
      root_path_exposed: false, author_identity_included: false,
      commit_body_included: false, remote_config_included: false,
      process_started: false, network_used: false, hooks_executed: false,
    });
    const repositoryCommit = vi.fn().mockResolvedValue({
      protocol_version: "repository_commit_detail.v1", workspace_id: "workspace-1",
      kind: "git", available: true, object_id: "abcdef1234567890abcdef1234567890abcdef12",
      hash: "abcdef123456", subject: "bounded history", parent_count: 1,
      committed_at: "2026-07-19T01:00:00Z", changes: [{ path: "internal/example.go",
        change: "modified", previous_kind: "regular", current_kind: "regular",
        content_changed: true, mode_changed: false }], changed_file_count: 1,
      returned_change_count: 1, omitted_change_count: 0, redaction_count: 0,
      truncated: false, first_parent_only: true, read_only: true, root_path_exposed: false,
      author_identity_included: false, commit_body_included: false, remote_config_included: false,
      file_content_included: false, patch_included: false, checkout_performed: false,
      reference_updated: false, process_started: false, network_used: false, hooks_executed: false,
    });
    const repositoryCommitFilePreview = vi.fn().mockResolvedValue({
      protocol_version: "repository_commit_file_preview.v1", workspace_id: "workspace-1",
      object_id: "abcdef1234567890abcdef1234567890abcdef12", hash: "abcdef123456",
      path: "internal/example.go", kind: "regular", content: "safe preview\n",
      total_bytes: 13, returned_bytes: 13, redaction_count: 0, redacted: false,
      provenance: { version: "context_provenance.v1", source_kind: "repository_commit_file",
        source_ref: "internal/example.go", content_sha256: "a".repeat(64),
        instruction_authorized: false }, read_only: true, mutation_supported: false,
      authority_granted: false, root_path_exposed: false, raw_blob_included: false,
      redacted_content_included: true, remote_config_included: false,
      checkout_performed: false, reference_updated: false, process_started: false,
      network_used: false, hooks_executed: false,
    });
    const historyObjectID = "1234567890abcdef1234567890abcdef12345678";
    const repositoryCommitComparison = vi.fn().mockResolvedValue({
      protocol_version: "repository_commit_comparison.v1", workspace_id: "workspace-1",
      kind: "git", available: true,
      base_object_id: "abcdef1234567890abcdef1234567890abcdef12",
      base_hash: "abcdef123456", base_subject: "bounded history",
      base_committed_at: "2026-07-19T01:00:00Z", base_redacted: false,
      base_subject_bounded: false, head_object_id: historyObjectID,
      head_hash: "1234567890ab", head_subject: "file changed",
      head_committed_at: "2026-07-18T01:00:00Z", head_redacted: false,
      head_subject_bounded: false, same_object: false,
      changes: [{ path: "internal/second.go", change: "added", previous_kind: "",
        current_kind: "regular", content_changed: true, mode_changed: false },
      { path: "internal/old.go", change: "deleted", previous_kind: "executable",
        current_kind: "", content_changed: true, mode_changed: false }],
      changed_file_count: 2, returned_change_count: 2, omitted_change_count: 0,
      redaction_count: 0, truncated: false, metadata_only: true, read_only: true,
      rename_inferred: false, ancestor_required: false, authority_granted: false,
      root_path_exposed: false, author_identity_included: false, commit_body_included: false,
      file_content_included: false, patch_included: false, remote_config_included: false,
      checkout_performed: false, reference_updated: false, process_started: false,
      network_used: false, hooks_executed: false,
    });
    const repositoryFileHistory = vi.fn().mockResolvedValue({
      protocol_version: "repository_file_history.v1", workspace_id: "workspace-1",
      kind: "git", available: true, head: "abcdef123456", path: "internal/example.go",
      entries: [{ object_id: historyObjectID,
        hash: "1234567890ab", subject: "file changed", committed_at: "2026-07-18T01:00:00Z",
        change: "modified", previous_kind: "regular", current_kind: "regular",
        content_changed: true, mode_changed: false, redacted: false, subject_bounded: false }],
      scanned_commit_count: 1, returned_entry_count: 1, redaction_count: 0,
      observed: true, truncated: false, first_parent_only: true, rename_inferred: false,
      metadata_only: true, read_only: true, authority_granted: false, root_path_exposed: false,
      author_identity_included: false, commit_body_included: false,
      file_content_included: false, patch_included: false, remote_config_included: false,
      checkout_performed: false, reference_updated: false, process_started: false,
      network_used: false, hooks_executed: false,
    });
    const client = { repositoryHistory, repositoryCommit, repositoryCommitComparison,
      repositoryCommitFilePreview, repositoryFileHistory } as unknown as CyberAgentClient;
    const user = userEvent.setup();
    const queryClient = new QueryClient();
    const { rerender } = render(<QueryClientProvider client={queryClient}>
      <RepositoryHistoryPanel client={client} workspaceID="workspace-1" />
    </QueryClientProvider>);
    expect(await screen.findByText("bounded history")).toBeInTheDocument();
    expect(screen.getByText("main")).toBeInTheDocument();
    expect(repositoryHistory).toHaveBeenCalledWith("workspace-1", expect.any(AbortSignal));
    await user.click(screen.getByRole("button", { name: "Inspect commit abcdef123456" }));
    expect(await screen.findByText("internal/example.go")).toBeInTheDocument();
    expect(repositoryCommit).toHaveBeenCalledWith("workspace-1",
      "abcdef1234567890abcdef1234567890abcdef12", expect.any(AbortSignal));
    await user.click(screen.getByRole("button", {
      name: "Use abcdef123456 as comparison base",
    }));
    await user.click(screen.getByRole("button", {
      name: "Preview internal/example.go at abcdef123456",
    }));
    expect(await screen.findByText("safe preview")).toBeInTheDocument();
    expect(repositoryCommitFilePreview).toHaveBeenCalledWith("workspace-1",
      "abcdef1234567890abcdef1234567890abcdef12", "internal/example.go",
      expect.any(AbortSignal));
    await user.click(screen.getByRole("button", { name: "Inspect history for internal/example.go" }));
    expect(await screen.findByText("file changed")).toBeInTheDocument();
    expect(repositoryFileHistory).toHaveBeenCalledWith("workspace-1", "internal/example.go",
      expect.any(AbortSignal));
    await user.click(screen.getByRole("button", {
      name: "Open commit 1234567890ab from history for internal/example.go",
    }));
    await waitFor(() => expect(repositoryCommit).toHaveBeenCalledWith("workspace-1",
      historyObjectID, expect.any(AbortSignal)));
    await waitFor(() => expect(repositoryCommitComparison).toHaveBeenCalledWith("workspace-1",
      "abcdef1234567890abcdef1234567890abcdef12", historyObjectID,
      expect.any(AbortSignal)));
    expect(await screen.findByRole("region", { name: "Exact commit comparison" }))
      .toHaveTextContent("internal/second.go");
    expect(screen.queryByRole("button", {
      name: "Preview internal/second.go at comparison base abcdef123456",
    })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", {
      name: "Preview internal/old.go at comparison head 1234567890ab",
    })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", {
      name: "Preview internal/second.go at comparison head 1234567890ab",
    }));
    await waitFor(() => expect(repositoryCommitFilePreview).toHaveBeenCalledWith("workspace-1",
      historyObjectID, "internal/second.go", expect.any(AbortSignal)));
    await user.click(screen.getByRole("button", {
      name: "Preview internal/old.go at comparison base abcdef123456",
    }));
    await waitFor(() => expect(repositoryCommitFilePreview).toHaveBeenCalledWith("workspace-1",
      "abcdef1234567890abcdef1234567890abcdef12", "internal/old.go",
      expect.any(AbortSignal)));
    await user.click(screen.getByRole("button", {
      name: "Preview internal/example.go at history commit 1234567890ab",
    }));
    await waitFor(() => expect(repositoryCommitFilePreview).toHaveBeenCalledWith("workspace-1",
      historyObjectID, "internal/example.go", expect.any(AbortSignal)));
    rerender(<QueryClientProvider client={queryClient}>
      <RepositoryHistoryPanel client={client} workspaceID="workspace-2" />
    </QueryClientProvider>);
    expect(screen.queryByRole("region", { name: "Exact commit metadata" })).not.toBeInTheDocument();
    expect(repositoryCommit).toHaveBeenCalledTimes(2);
    expect(repositoryCommitFilePreview).toHaveBeenCalledTimes(4);
    expect(screen.queryByRole("region", { name: "Exact commit comparison" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Exact file history" })).not.toBeInTheDocument();
  });
});
