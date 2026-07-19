import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { CyberAgentClient } from "../api/client";
import { RepositoryDiffPanel } from "./repository-diff-panel";

describe("RepositoryDiffPanel", () => {
  it("renders only the bounded redacted patch projection", async () => {
    const repositoryDiff = vi.fn().mockResolvedValue({
      protocol_version: "repository_diff.v1", workspace_id: "workspace-1",
      kind: "git", available: true, base_head: "1234567890ab",
      items: [{ path: "src/main.go", staging: "unmodified", worktree: "modified",
        content_state: "text", patch: "+token=[REDACTED:api-key]\n", patch_bytes: 31,
        added_lines: 1, deleted_lines: 0, redacted: true, truncated: false }],
      returned_count: 1, omitted_count: 0, redaction_count: 1, total_patch_bytes: 31,
      truncated: false, read_only: true, instruction_authorized: false,
      mutation_supported: false, authority_granted: false, root_path_exposed: false,
      raw_content_included: false, patch_content_included: true,
      remote_config_included: false, process_started: false, network_used: false,
      hooks_executed: false,
    });
    const client = { repositoryDiff } as unknown as CyberAgentClient;
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={queryClient}>
      <RepositoryDiffPanel client={client} workspaceID="workspace-1" />
    </QueryClientProvider>);
    expect(await screen.findByText("src/main.go")).toBeInTheDocument();
    expect(screen.getByText(/REDACTED:api-key/)).toBeInTheDocument();
    expect(screen.queryByText(/workspace root|authority granted/i)).not.toBeInTheDocument();
  });
});
