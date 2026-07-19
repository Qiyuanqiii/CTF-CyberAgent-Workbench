import { useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, type UseQueryResult } from "@tanstack/react-query";
import { ArrowLeft, File, Folder, FolderOpen, Paperclip, Pencil, Search, ShieldCheck, X } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { WorkspaceSearchView } from "../api/types";
import { formatBytes } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";
import { FileProposalEditor } from "./file-proposal-editor";

export function WorkspaceExplorer({ client, workspaceID, runID = "", initialPath = "." }: {
  client: CyberAgentClient;
  workspaceID: string;
  runID?: string;
  initialPath?: string;
}) {
  const [path, setPath] = useState(initialPath);
  const [searchInput, setSearchInput] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const operationKeys = useRef(new Map<string, string>());
  useEffect(() => {
    setPath(initialPath);
    setSearchInput("");
    setSearchQuery("");
    operationKeys.current.clear();
  }, [workspaceID, runID, initialPath]);
  const query = useQuery({
    queryKey: ["workspace", workspaceID, "explore", path],
    queryFn: ({ signal }) => client.workspaceExplore(workspaceID, path, signal),
    enabled: Boolean(workspaceID),
  });
  const search = useQuery({
    queryKey: ["workspace", workspaceID, "search", searchQuery],
    queryFn: ({ signal }) => client.workspaceSearch(workspaceID, searchQuery, signal),
    enabled: Boolean(workspaceID && searchQuery),
  });
  const attachment = useMutation({
    mutationFn: ({ sourceRef, contentSHA256 }: {
      sourceRef: string;
      contentSHA256: string;
    }) => client.attachEvidence(runID, {
      version: "session_evidence_attachment.v1",
      source_kind: "workspace_file",
      source_ref: sourceRef,
      content_sha256: contentSHA256,
    }, evidenceOperationKey(operationKeys.current, sourceRef, contentSHA256)),
  });
  const proposalSource = useMutation({
    mutationFn: (sourcePath: string) => client.issueFileEditProposalSource(runID, sourcePath),
  });
  const parent = useMemo(() => parentPath(path), [path]);

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalized = searchInput.trim();
    if (normalized && normalized === searchInput && normalized.length <= 128) {
      setSearchQuery(normalized);
    }
  };

  const attach = (sourceRef: string, contentSHA256: string) => {
    attachment.mutate({ sourceRef, contentSHA256 });
  };

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
      {snapshot.kind === "file" && client.hasEvidenceAttachment && runID &&
        <button className="compact-command" disabled={attachment.isPending}
          onClick={() => attach(snapshot.path, snapshot.provenance.content_sha256)} type="button">
          <Paperclip aria-hidden="true" size={14} />Attach evidence
        </button>}
      {snapshot.kind === "file" && client.hasFileEditProposals && runID &&
        !snapshot.truncated && snapshot.redaction_count === 0 &&
        <button className="compact-command" disabled={proposalSource.isPending}
          onClick={() => proposalSource.mutate(snapshot.path)} type="button">
          <Pencil aria-hidden="true" size={14} />Edit proposal
        </button>}
    </header>
    <form className="explorer-search" onSubmit={submitSearch} role="search">
      <Search aria-hidden="true" size={15} />
      <input aria-label="Search Workspace evidence" maxLength={128}
        onChange={(event) => setSearchInput(event.target.value)}
        placeholder="Search files" type="search" value={searchInput} />
      <button aria-label="Search Workspace" className="icon-button"
        disabled={!searchInput.trim() || searchInput.trim() !== searchInput || search.isFetching}
        title="Search" type="submit"><Search aria-hidden="true" size={15} /></button>
    </form>
    <div className="explorer-provenance">
      <ShieldCheck aria-hidden="true" size={14} />
      <span>{snapshot.provenance.source_kind} / evidence only</span>
      {snapshot.redaction_count > 0 && <span>{snapshot.redaction_count} redacted</span>}
      <code>{snapshot.provenance.content_sha256.slice(0, 12)}</code>
    </div>
    {attachment.isError && <ErrorState error={attachment.error} />}
    {proposalSource.isError && <ErrorState error={proposalSource.error} />}
    {attachment.data && <div className="explorer-attachment-status" role="status">
      <ShieldCheck aria-hidden="true" size={14} />
      Evidence attached as non-authorizing context
      {attachment.data.replayed && <StatusBadge status="replayed" />}
    </div>}
    {searchQuery && <WorkspaceSearchResults client={client} runID={runID}
      pending={attachment.isPending} query={search} onAttach={attach}
      onClear={() => setSearchQuery("")} onOpen={(resultPath) => setPath(resultPath)} />}
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
    {proposalSource.data && proposalSource.data.path === snapshot.path ?
      <FileProposalEditor client={client} onClose={() => proposalSource.reset()}
        runID={runID} source={proposalSource.data} /> :
    snapshot.kind === "file" && <div className="explorer-file">
      <div><span>{formatBytes(snapshot.returned_bytes)} shown</span>
        <span>{formatBytes(snapshot.total_bytes)} total</span></div>
      <pre>{snapshot.content}</pre>
    </div>}
  </section>;
}

function WorkspaceSearchResults({ client, runID, pending, query, onAttach, onClear, onOpen }: {
  client: CyberAgentClient;
  runID: string;
  pending: boolean;
  query: UseQueryResult<WorkspaceSearchView, Error>;
  onAttach: (path: string, digest: string) => void;
  onClear: () => void;
  onOpen: (path: string) => void;
}) {
  if (query.isLoading) return <LoadingState label="Searching Workspace" />;
  if (query.isError || !query.data) return <ErrorState error={query.error} />;
  const data = query.data;
  return <section className="explorer-search-results" aria-label="Workspace search results">
    <header><strong>{data.results.length} results</strong>
      {data.truncated && <StatusBadge status="truncated" />}
      <button aria-label="Close search results" className="icon-button" onClick={onClear}
        title="Close search" type="button"><X aria-hidden="true" size={14} /></button>
    </header>
    {data.results.length === 0 && <EmptyState>No matching evidence</EmptyState>}
    {data.results.map((result) => <div className="explorer-search-result" key={result.path}>
      <button className="search-result-open" onClick={() => onOpen(result.path)} type="button">
        <File aria-hidden="true" size={15} />
        <span><strong>{result.path}</strong>
          <small>{result.line > 0 ? `line ${result.line}` : result.match_kind}</small>
          {result.snippet && <code>{result.snippet}</code>}</span>
      </button>
      {client.hasEvidenceAttachment && runID &&
        <button aria-label={`Attach ${result.path} as evidence`} className="icon-button"
          disabled={pending} onClick={() => onAttach(result.path,
            result.provenance.content_sha256)} title="Attach non-authorizing evidence" type="button">
          <Paperclip aria-hidden="true" size={14} />
        </button>}
    </div>)}
  </section>;
}

function evidenceOperationKey(keys: Map<string, string>, path: string, digest: string): string {
  const identity = `${path}:${digest}`;
  const existing = keys.get(identity);
  if (existing) return existing;
  const key = `evidence-${crypto.randomUUID()}`;
  keys.set(identity, key);
  return key;
}

function parentPath(path: string): string {
  if (path === "." || !path.includes("/")) return ".";
  return path.slice(0, path.lastIndexOf("/")) || ".";
}
