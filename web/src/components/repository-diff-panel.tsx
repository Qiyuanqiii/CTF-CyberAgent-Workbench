import { useQuery } from "@tanstack/react-query";
import { FileDiff, RefreshCw } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { formatBytes } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

export function RepositoryDiffPanel({ client, workspaceID }: {
  client: CyberAgentClient;
  workspaceID: string;
}) {
  const query = useQuery({
    queryKey: ["workspace", workspaceID, "repository-diff"],
    queryFn: ({ signal }) => client.repositoryDiff(workspaceID, signal),
    enabled: Boolean(workspaceID),
  });
  if (!workspaceID) return null;
  return <section aria-label="Repository diff" className="repository-diff-panel">
    <header className="projection-heading">
      <div><FileDiff aria-hidden="true" size={17} /><h2>Working tree diff</h2></div>
      <div>
        {query.data?.truncated && <StatusBadge status="truncated" />}
        <button aria-label="Refresh repository diff" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh" type="button">
          <RefreshCw aria-hidden="true" className={query.isFetching ? "spin" : ""} size={15} />
        </button>
      </div>
    </header>
    {query.isLoading && <LoadingState label="Loading repository diff" />}
    {query.isError && <ErrorState error={query.error} />}
    {query.data && !query.data.available &&
      <EmptyState>No Git repository at the registered Workspace root</EmptyState>}
    {query.data?.available && <>
      <div className="repository-diff-summary">
        <span>{query.data.returned_count} files</span>
        <span>{formatBytes(query.data.total_patch_bytes)}</span>
        {query.data.omitted_count > 0 && <span>{query.data.omitted_count} omitted</span>}
        {query.data.redaction_count > 0 && <span>{query.data.redaction_count} redactions</span>}
      </div>
      {query.data.items.length === 0 ? <EmptyState>Working tree is clean</EmptyState> :
        <div className="repository-diff-list">
          {query.data.items.map((item) => <article key={item.path}>
            <header>
              <code>{item.path}</code>
              <span>{item.added_lines} added / {item.deleted_lines} deleted</span>
              {item.redacted && <StatusBadge status="redacted" />}
              <StatusBadge status={item.content_state} />
            </header>
            {item.patch ? <pre>{item.patch}</pre> :
              <div className="repository-diff-omitted">{item.content_state}</div>}
          </article>)}
        </div>}
    </>}
  </section>;
}
