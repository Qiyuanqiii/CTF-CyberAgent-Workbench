import { useQuery } from "@tanstack/react-query";
import { GitBranch, RefreshCw } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

export function RepositoryStatePanel({ client, workspaceID }: {
  client: CyberAgentClient;
  workspaceID: string;
}) {
  const query = useQuery({
    queryKey: ["workspace", workspaceID, "repository-state"],
    queryFn: ({ signal }) => client.repositoryState(workspaceID, signal),
    enabled: Boolean(workspaceID),
  });
  if (!workspaceID) return <EmptyState>No Workspace is bound to this Run</EmptyState>;
  if (query.isLoading) return <LoadingState label="Loading repository state" />;
  if (query.isError || !query.data) return <ErrorState error={query.error} />;
  const state = query.data;
  if (!state.available) {
    return <section aria-label="Repository state" className="repository-state-panel">
      <header className="projection-heading">
        <div><GitBranch aria-hidden="true" size={17} /><h2>Repository</h2></div>
        <button aria-label="Refresh repository state" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh" type="button"><RefreshCw aria-hidden="true" size={15} /></button>
      </header>
      <EmptyState>No Git repository at the registered Workspace root</EmptyState>
    </section>;
  }
  return <section aria-label="Repository state" className="repository-state-panel">
    <header className="projection-heading">
      <div><GitBranch aria-hidden="true" size={17} /><h2>Repository</h2></div>
      <div>
        {state.truncated && <StatusBadge status="truncated" />}
        <StatusBadge status={state.clean ? "clean" : "changed"} />
        <button aria-label="Refresh repository state" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh" type="button"><RefreshCw aria-hidden="true" size={15} /></button>
      </div>
    </header>
    <div className="repository-reference">
      <span>{state.detached ? "detached" : state.branch || "unborn"}</span>
      {state.head && <code>{state.head}</code>}
      <small>read-only / local metadata</small>
    </div>
    <div aria-label="Repository change counts" className="repository-counts">
      <div><span>Staged</span><strong>{state.staged_count}</strong></div>
      <div><span>Worktree</span><strong>{state.worktree_count}</strong></div>
      <div><span>Untracked</span><strong>{state.untracked_count}</strong></div>
      <div><span>Conflicts</span><strong>{state.conflicted_count}</strong></div>
    </div>
    {state.changes.length === 0 ? <EmptyState>Working tree is clean</EmptyState> :
      <div className="repository-change-list" role="list">
        {state.changes.map((change) => <div key={change.path} role="listitem">
          <code>{change.path}</code>
          <span>{change.staging}</span>
          <span>{change.worktree}</span>
        </div>)}
      </div>}
  </section>;
}
