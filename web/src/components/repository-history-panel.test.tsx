import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import type { CyberAgentClient } from "../api/client";
import { RepositoryHistoryPanel } from "./repository-history-panel";

describe("RepositoryHistoryPanel", () => {
  it("renders bounded local branches and commit subjects", async () => {
    const repositoryHistory = vi.fn().mockResolvedValue({
      protocol_version: "repository_history.v1", workspace_id: "workspace-1",
      kind: "git", available: true, head: "abcdef123456", detached: false,
      commits: [{ hash: "abcdef123456", subject: "bounded history", parent_count: 1,
        committed_at: "2026-07-19T01:00:00Z", redacted: false, subject_bounded: false }],
      branches: [{ name: "main", head: "abcdef123456", current: true }],
      returned_commit_count: 1, returned_branch_count: 1, omitted_branch_count: 0,
      redaction_count: 0, truncated: false, first_parent_only: true, read_only: true,
      root_path_exposed: false, author_identity_included: false,
      commit_body_included: false, remote_config_included: false,
      process_started: false, network_used: false, hooks_executed: false,
    });
    const client = { repositoryHistory } as unknown as CyberAgentClient;
    render(<QueryClientProvider client={new QueryClient()}>
      <RepositoryHistoryPanel client={client} workspaceID="workspace-1" />
    </QueryClientProvider>);
    expect(await screen.findByText("bounded history")).toBeInTheDocument();
    expect(screen.getByText("main")).toBeInTheDocument();
    expect(repositoryHistory).toHaveBeenCalledWith("workspace-1", expect.any(AbortSignal));
  });
});
