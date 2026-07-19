import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FileDiff, GitCommitHorizontal, RefreshCw } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { formatDate } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

export function RepositoryHistoryPanel({ client, workspaceID }: {
  client: CyberAgentClient;
  workspaceID: string;
}) {
  const [selection, setSelection] = useState({ workspaceID: "", objectID: "" });
  const selectedObjectID = selection.workspaceID === workspaceID ? selection.objectID : "";
  const query = useQuery({
    queryKey: ["workspace", workspaceID, "repository-history"],
    queryFn: ({ signal }) => client.repositoryHistory(workspaceID, signal),
    enabled: Boolean(workspaceID),
  });
  const detailQuery = useQuery({
    queryKey: ["workspace", workspaceID, "repository-commit", selectedObjectID],
    queryFn: ({ signal }) => client.repositoryCommit(workspaceID, selectedObjectID, signal),
    enabled: Boolean(workspaceID && selectedObjectID),
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
            <div key={commit.object_id}><code>{commit.hash}</code><span>{commit.subject}</span>
              <time dateTime={commit.committed_at}>{formatDate(commit.committed_at)}</time>
              <span className="repository-commit-flags">
                {commit.redacted && <StatusBadge status="redacted" />}
              </span>
              <button aria-label={`Inspect commit ${commit.hash}`} aria-pressed={selectedObjectID === commit.object_id}
                className="icon-button" onClick={() => setSelection((current) =>
                  current.workspaceID === workspaceID && current.objectID === commit.object_id ?
                    { workspaceID: "", objectID: "" } : { workspaceID, objectID: commit.object_id })}
                title="Inspect changed files" type="button">
                <FileDiff aria-hidden="true" size={14} />
              </button></div>)}</div>}
      </section>
    </div>}
    {selectedObjectID && <section aria-label="Exact commit metadata" className="repository-commit-detail">
      {detailQuery.isLoading && <LoadingState label="Loading exact commit metadata" />}
      {detailQuery.isError && <ErrorState error={detailQuery.error} />}
      {detailQuery.data && <>
        <header><span><code>{detailQuery.data.hash}</code><strong>{detailQuery.data.subject}</strong></span>
          <span><StatusBadge status={`${detailQuery.data.changed_file_count} changed`} />
            {detailQuery.data.truncated && <StatusBadge status="truncated" />}</span></header>
        {detailQuery.data.changes.length === 0 ? <EmptyState>No changed files in this commit</EmptyState> :
          <div className="repository-commit-change-list">{detailQuery.data.changes.map((change) =>
            <div key={`${change.change}:${change.path}`}><StatusBadge status={change.change} />
              <code title={change.path}>{change.path}</code>
              <span>{change.previous_kind || "none"} to {change.current_kind || "none"}</span>
              <span>{change.content_changed ? "content" : "mode only"}</span></div>)}</div>}
        {detailQuery.data.omitted_change_count > 0 &&
          <p className="repository-diff-omitted">{detailQuery.data.omitted_change_count} additional changes omitted</p>}
      </>}
    </section>}
  </section>;
}
