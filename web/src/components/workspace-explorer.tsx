import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, File, Folder, FolderOpen, ShieldCheck } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { formatBytes } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

export function WorkspaceExplorer({ client, workspaceID }: {
  client: CyberAgentClient;
  workspaceID: string;
}) {
  const [path, setPath] = useState(".");
  useEffect(() => setPath("."), [workspaceID]);
  const query = useQuery({
    queryKey: ["workspace", workspaceID, "explore", path],
    queryFn: ({ signal }) => client.workspaceExplore(workspaceID, path, signal),
    enabled: Boolean(workspaceID),
  });
  const parent = useMemo(() => parentPath(path), [path]);

  if (!workspaceID) return <EmptyState>No Workspace is bound to this Run</EmptyState>;
  if (query.isLoading) return <LoadingState label="Loading Workspace files" />;
  if (query.isError || !query.data) return <ErrorState error={query.error} />;
  const snapshot = query.data;

  return <section className="workspace-explorer" aria-label="Workspace files">
    <header className="explorer-toolbar">
      <button aria-label="Open parent directory" className="icon-button"
        disabled={path === "." || query.isFetching} onClick={() => setPath(parent)}
        title="Parent directory" type="button">
        <ArrowLeft aria-hidden="true" size={16} />
      </button>
      <FolderOpen aria-hidden="true" size={16} />
      <code>{snapshot.path}</code>
      {snapshot.truncated && <StatusBadge status="truncated" />}
    </header>
    <div className="explorer-provenance">
      <ShieldCheck aria-hidden="true" size={14} />
      <span>{snapshot.provenance.source_kind} / evidence only</span>
      {snapshot.redaction_count > 0 && <span>{snapshot.redaction_count} redacted</span>}
      <code>{snapshot.provenance.content_sha256.slice(0, 12)}</code>
    </div>
    {snapshot.kind === "directory" && snapshot.entries.length === 0 &&
      <EmptyState>Directory is empty</EmptyState>}
    {snapshot.kind === "directory" && snapshot.entries.length > 0 &&
      <div className="explorer-list" role="list">
        {snapshot.entries.map((entry) => {
          const Icon = entry.kind === "directory" ? Folder : File;
          return <div key={entry.path} role="listitem">
            <button disabled={!entry.readable || query.isFetching}
              onClick={() => setPath(entry.path)} type="button">
              <Icon aria-hidden="true" size={16} />
              <span>{entry.name}</span>
              <small>{entry.kind === "file" ? formatBytes(entry.size_bytes) : entry.kind}</small>
            </button>
          </div>;
        })}
      </div>}
    {snapshot.kind === "file" && <div className="explorer-file">
      <div><span>{formatBytes(snapshot.returned_bytes)} shown</span>
        <span>{formatBytes(snapshot.total_bytes)} total</span></div>
      <pre>{snapshot.content}</pre>
    </div>}
  </section>;
}

function parentPath(path: string): string {
  if (path === "." || !path.includes("/")) return ".";
  return path.slice(0, path.lastIndexOf("/")) || ".";
}
