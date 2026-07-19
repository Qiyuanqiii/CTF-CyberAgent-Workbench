import { useQuery } from "@tanstack/react-query";
import { GitCommitHorizontal, RefreshCw } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { formatDate } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

export function RepositoryHistoryPanel({ client, workspaceID }: {
  client: CyberAgentClient;
  workspaceID: string;
}) {
  const query = useQuery({
    queryKey: ["workspace", workspaceID, "repository-history"],
    queryFn: ({ signal }) => client.repositoryHistory(workspaceID, signal),
    enabled: Boolean(workspaceID),
  });
  if (!workspaceID) return null;
  return <section aria-label="Repository history" className="repository-history-panel">
    <header className="projection-heading">
      <div><GitCommitHorizontal aria-hidden="true" size={17} /><h2>Local history</h2></div>
      <div>{query.data?.truncated && <StatusBadge status="truncated" />}
        <button aria-label="Refresh repository history" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh" type="button"><RefreshCw aria-hidden="true"
            className={query.isFetching ? "spin" : ""} size={15} /></button></div>
    </header>
    {query.isLoading && <LoadingState label="Loading repository history" />}
    {query.isError && <ErrorState error={query.error} />}
    {query.data && !query.data.available &&
      <EmptyState>No Git repository at the registered Workspace root</EmptyState>}
    {query.data?.available && <div className="repository-history-grid">
      <section><h3>Branches</h3>
        {query.data.branches.length === 0 ? <EmptyState>No local branches</EmptyState> :
          <div className="repository-branch-list">{query.data.branches.map((branch) =>
            <div key={branch.name}><span>{branch.name}</span><code>{branch.head}</code>
              {branch.current && <StatusBadge status="current" />}</div>)}</div>}
      </section>
      <section><h3>First-parent commits</h3>
        {query.data.commits.length === 0 ? <EmptyState>No commits</EmptyState> :
          <div className="repository-commit-list">{query.data.commits.map((commit) =>
            <div key={commit.hash}><code>{commit.hash}</code><span>{commit.subject}</span>
              <time dateTime={commit.committed_at}>{formatDate(commit.committed_at)}</time>
              {commit.redacted && <StatusBadge status="redacted" />}</div>)}</div>}
      </section>
    </div>}
  </section>;
}
