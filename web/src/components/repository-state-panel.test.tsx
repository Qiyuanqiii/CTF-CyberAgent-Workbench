import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CyberAgentClient } from "../api/client";
import { RepositoryStatePanel } from "./repository-state-panel";

describe("RepositoryStatePanel", () => {
  it("renders status-only repository facts and refreshes without mutation", async () => {
    const user = userEvent.setup();
    const repositoryState = vi.fn().mockResolvedValue({
      protocol_version: "repository_state.v1", workspace_id: "workspace-1",
      kind: "git", available: true, clean: false, detached: false, branch: "main",
      head: "1234567890ab", changes: [{ path: "src/main.go", staging: "unmodified",
        worktree: "modified" }], staged_count: 0, worktree_count: 1,
      untracked_count: 0, conflicted_count: 0, redaction_count: 0, truncated: false,
      read_only: true, root_path_exposed: false, content_included: false,
      remote_config_included: false, process_started: false, network_used: false,
      hooks_executed: false,
    });
    renderPanel({ repositoryState } as unknown as CyberAgentClient);

    expect(await screen.findByText("src/main.go")).toBeInTheDocument();
    expect(screen.getByText("1234567890ab")).toBeInTheDocument();
    expect(screen.getByText("read-only / local metadata")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Refresh repository state" }));
    await waitFor(() => expect(repositoryState).toHaveBeenCalledTimes(2));
    expect(repositoryState).toHaveBeenLastCalledWith("workspace-1", expect.any(AbortSignal));
  });

  it("does not search parent directories when no root repository is available", async () => {
    renderPanel({ repositoryState: vi.fn().mockResolvedValue({
      protocol_version: "repository_state.v1", workspace_id: "workspace-1",
      kind: "none", available: false, clean: false, detached: false, branch: "", head: "",
      changes: [], staged_count: 0, worktree_count: 0, untracked_count: 0,
      conflicted_count: 0, redaction_count: 0, truncated: false, read_only: true,
      root_path_exposed: false, content_included: false, remote_config_included: false,
      process_started: false, network_used: false, hooks_executed: false,
    }) } as unknown as CyberAgentClient);
    expect(await screen.findByText("No Git repository at the registered Workspace root"))
      .toBeInTheDocument();
  });
});

function renderPanel(client: CyberAgentClient) {
  const queryClient = new QueryClient({ defaultOptions: {
    queries: { retry: false }, mutations: { retry: false },
  } });
  return render(<QueryClientProvider client={queryClient}>
    <RepositoryStatePanel client={client} workspaceID="workspace-1" />
  </QueryClientProvider>);
}
