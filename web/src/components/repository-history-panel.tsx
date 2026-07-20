import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Columns2, FileClock, FileCode2, FileDiff, FileInput,
  FileOutput, GitCommitHorizontal, GitCompareArrows, RefreshCw } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { formatDate } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

export function RepositoryHistoryPanel({ client, workspaceID }: {
  client: CyberAgentClient;
  workspaceID: string;
}) {
  const [selection, setSelection] = useState({ workspaceID: "", objectID: "" });
  const [fileSelection, setFileSelection] = useState({ workspaceID: "", objectID: "", path: "" });
  const [historySelection, setHistorySelection] = useState({ workspaceID: "", path: "" });
  const [comparisonBase, setComparisonBase] = useState({ workspaceID: "", objectID: "" });
  const [comparisonPreview, setComparisonPreview] = useState({ workspaceID: "", baseObjectID: "",
    baseHash: "", baseAvailable: false, headObjectID: "", headHash: "", headAvailable: false,
    path: "" });
  const selectedObjectID = selection.workspaceID === workspaceID ? selection.objectID : "";
  const comparisonBaseObjectID = comparisonBase.workspaceID === workspaceID ?
    comparisonBase.objectID : "";
  const comparisonHeadObjectID = selectedObjectID && selectedObjectID !== comparisonBaseObjectID ?
    selectedObjectID : "";
  const selectedPreviewObjectID = fileSelection.workspaceID === workspaceID ?
    fileSelection.objectID : "";
  const selectedFilePath = fileSelection.workspaceID === workspaceID ? fileSelection.path : "";
  const selectedHistoryPath = historySelection.workspaceID === workspaceID ?
    historySelection.path : "";
  const activeComparisonPreview = comparisonPreview.workspaceID === workspaceID &&
    comparisonPreview.baseObjectID === comparisonBaseObjectID &&
    comparisonPreview.headObjectID === comparisonHeadObjectID ? comparisonPreview : null;
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
  const comparisonQuery = useQuery({
    queryKey: ["workspace", workspaceID, "repository-commit-comparison",
      comparisonBaseObjectID, comparisonHeadObjectID],
    queryFn: ({ signal }) => client.repositoryCommitComparison(workspaceID,
      comparisonBaseObjectID, comparisonHeadObjectID, signal),
    enabled: Boolean(workspaceID && comparisonBaseObjectID && comparisonHeadObjectID),
  });
  const fileQuery = useQuery({
    queryKey: ["workspace", workspaceID, "repository-commit", selectedPreviewObjectID,
      "file-preview", selectedFilePath],
    queryFn: ({ signal }) => client.repositoryCommitFilePreview(workspaceID,
      selectedPreviewObjectID, selectedFilePath, signal),
    enabled: Boolean(workspaceID && selectedPreviewObjectID && selectedFilePath),
  });
  const fileHistoryQuery = useQuery({
    queryKey: ["workspace", workspaceID, "repository-file-history", selectedHistoryPath],
    queryFn: ({ signal }) => client.repositoryFileHistory(workspaceID, selectedHistoryPath, signal),
    enabled: Boolean(workspaceID && selectedHistoryPath),
  });
  const comparisonBasePreviewQuery = useQuery({
    queryKey: ["workspace", workspaceID, "repository-comparison-preview",
      activeComparisonPreview?.baseObjectID, activeComparisonPreview?.path, "base"],
    queryFn: ({ signal }) => client.repositoryCommitFilePreview(workspaceID,
      activeComparisonPreview?.baseObjectID ?? "", activeComparisonPreview?.path ?? "", signal),
    enabled: Boolean(workspaceID && activeComparisonPreview?.baseAvailable),
  });
  const comparisonHeadPreviewQuery = useQuery({
    queryKey: ["workspace", workspaceID, "repository-comparison-preview",
      activeComparisonPreview?.headObjectID, activeComparisonPreview?.path, "head"],
    queryFn: ({ signal }) => client.repositoryCommitFilePreview(workspaceID,
      activeComparisonPreview?.headObjectID ?? "", activeComparisonPreview?.path ?? "", signal),
    enabled: Boolean(workspaceID && activeComparisonPreview?.headAvailable),
  });
  const pairedPreviewCandidates = (comparisonQuery.data?.changes ?? []).filter((change) =>
    ["regular", "executable"].includes(change.previous_kind) ||
    ["regular", "executable"].includes(change.current_kind));
  const pairedPreviewIndex = activeComparisonPreview ? pairedPreviewCandidates.findIndex(
    (change) => change.path === activeComparisonPreview.path) : -1;
  const selectPairedPreview = (index: number, toggle = false) => {
    const comparison = comparisonQuery.data;
    const change = pairedPreviewCandidates[index];
    if (!comparison || !change) return;
    if (toggle && activeComparisonPreview?.path === change.path) {
      setComparisonPreview({ workspaceID: "", baseObjectID: "", baseHash: "",
        baseAvailable: false, headObjectID: "", headHash: "", headAvailable: false, path: "" });
      return;
    }
    setComparisonPreview({ workspaceID, baseObjectID: comparison.base_object_id,
      baseHash: comparison.base_hash,
      baseAvailable: ["regular", "executable"].includes(change.previous_kind),
      headObjectID: comparison.head_object_id, headHash: comparison.head_hash,
      headAvailable: ["regular", "executable"].includes(change.current_kind),
      path: change.path });
  };
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
                className="icon-button" onClick={() => {
                  setFileSelection({ workspaceID: "", objectID: "", path: "" });
                  setSelection((current) =>
                    current.workspaceID === workspaceID && current.objectID === commit.object_id ?
                      { workspaceID: "", objectID: "" } : { workspaceID, objectID: commit.object_id });
                }}
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
            {comparisonBaseObjectID === selectedObjectID && <StatusBadge status="comparison base" />}
            {detailQuery.data.truncated && <StatusBadge status="truncated" />}
            <button aria-label={comparisonBaseObjectID === selectedObjectID ?
              `Clear comparison base ${detailQuery.data.hash}` :
              `Use ${detailQuery.data.hash} as comparison base`}
              aria-pressed={comparisonBaseObjectID === selectedObjectID} className="icon-button"
              onClick={() => setComparisonBase((current) =>
                current.workspaceID === workspaceID && current.objectID === selectedObjectID ?
                  { workspaceID: "", objectID: "" } : { workspaceID, objectID: selectedObjectID })}
              title={comparisonBaseObjectID === selectedObjectID ?
                "Clear comparison base" : "Use as comparison base"} type="button">
              <GitCompareArrows aria-hidden="true" size={14} />
            </button></span></header>
        {detailQuery.data.changes.length === 0 ? <EmptyState>No changed files in this commit</EmptyState> :
          <div className="repository-commit-change-list">{detailQuery.data.changes.map((change) =>
            <div key={`${change.change}:${change.path}`}><StatusBadge status={change.change} />
              <code title={change.path}>{change.path}</code>
              <span>{change.previous_kind || "none"} to {change.current_kind || "none"}</span>
              <span>{change.content_changed ? "content" : "mode only"}</span>
              <button aria-label={`Inspect history for ${change.path}`}
                aria-pressed={selectedHistoryPath === change.path} className="icon-button"
                onClick={() => setHistorySelection((current) =>
                  current.workspaceID === workspaceID && current.path === change.path ?
                    { workspaceID: "", path: "" } : { workspaceID, path: change.path })}
                title="Inspect file history" type="button">
                <FileClock aria-hidden="true" size={14} />
              </button>
              {["regular", "executable"].includes(change.current_kind) ?
                <button aria-label={`Preview ${change.path} at ${detailQuery.data.hash}`}
                  aria-pressed={selectedPreviewObjectID === selectedObjectID &&
                    selectedFilePath === change.path} className="icon-button"
                  onClick={() => setFileSelection((current) =>
                    current.workspaceID === workspaceID && current.objectID === selectedObjectID &&
                      current.path === change.path ? { workspaceID: "", objectID: "", path: "" } :
                      { workspaceID, objectID: selectedObjectID, path: change.path })}
                  title="Preview redacted file" type="button">
                  <FileCode2 aria-hidden="true" size={14} />
                </button> : <span aria-hidden="true" className="repository-preview-placeholder" />}
            </div>)}</div>}
        {detailQuery.data.omitted_change_count > 0 &&
          <p className="repository-diff-omitted">{detailQuery.data.omitted_change_count} additional changes omitted</p>}
        {selectedFilePath && <section aria-label="Exact commit file preview"
          className="repository-commit-file-preview">
          {fileQuery.isLoading && <LoadingState label="Loading redacted commit file" />}
          {fileQuery.isError && <ErrorState error={fileQuery.error} />}
          {fileQuery.data && <>
            <header><code title={`${fileQuery.data.hash} / ${fileQuery.data.path}`}>
              {fileQuery.data.hash} / {fileQuery.data.path}</code>
              <span><StatusBadge status={fileQuery.data.kind} />
                {fileQuery.data.redacted && <StatusBadge status="redacted" />}</span></header>
            <pre>{fileQuery.data.content}</pre>
          </>}
        </section>}
      </>}
    </section>}
    {comparisonBaseObjectID && comparisonHeadObjectID &&
      <section aria-label="Exact commit comparison" className="repository-commit-comparison">
        {comparisonQuery.isLoading && <LoadingState label="Loading exact commit comparison" />}
        {comparisonQuery.isError && <ErrorState error={comparisonQuery.error} />}
        {comparisonQuery.data && <>
          <header><span><GitCompareArrows aria-hidden="true" size={14} />
            <code>{comparisonQuery.data.base_hash}</code><span>to</span>
            <code>{comparisonQuery.data.head_hash}</code></span>
            <span><StatusBadge status={`${comparisonQuery.data.changed_file_count} changed`} />
              {comparisonQuery.data.truncated && <StatusBadge status="truncated" />}</span></header>
          <div className="repository-commit-comparison-subjects">
            <span title={comparisonQuery.data.base_subject}>{comparisonQuery.data.base_subject}</span>
            <span title={comparisonQuery.data.head_subject}>{comparisonQuery.data.head_subject}</span>
          </div>
          {comparisonQuery.data.changes.length === 0 ?
            <EmptyState>No metadata changes between these commits</EmptyState> :
            <div className="repository-commit-comparison-list">{comparisonQuery.data.changes.map((change) =>
              <div key={`${change.change}:${change.path}`}><StatusBadge status={change.change} />
                <code title={change.path}>{change.path}</code>
                <span>{change.previous_kind || "none"} to {change.current_kind || "none"}</span>
                <span>{change.content_changed ? "content" : "mode only"}</span>
                <span className="repository-commit-comparison-actions">
                  {["regular", "executable"].includes(change.previous_kind) ?
                    <button aria-label={`Preview ${change.path} at comparison base ${comparisonQuery.data.base_hash}`}
                      aria-pressed={selectedPreviewObjectID === comparisonQuery.data.base_object_id &&
                        selectedFilePath === change.path} className="icon-button"
                      onClick={() => setFileSelection((current) =>
                        current.workspaceID === workspaceID &&
                          current.objectID === comparisonQuery.data.base_object_id &&
                          current.path === change.path ?
                          { workspaceID: "", objectID: "", path: "" } :
                          { workspaceID, objectID: comparisonQuery.data.base_object_id,
                            path: change.path })}
                      title="Preview redacted base file" type="button">
                      <FileInput aria-hidden="true" size={14} />
                    </button> : <span aria-hidden="true" className="repository-preview-placeholder" />}
                  {["regular", "executable"].includes(change.current_kind) ?
                    <button aria-label={`Preview ${change.path} at comparison head ${comparisonQuery.data.head_hash}`}
                      aria-pressed={selectedPreviewObjectID === comparisonQuery.data.head_object_id &&
                        selectedFilePath === change.path} className="icon-button"
                      onClick={() => setFileSelection((current) =>
                        current.workspaceID === workspaceID &&
                          current.objectID === comparisonQuery.data.head_object_id &&
                          current.path === change.path ?
                          { workspaceID: "", objectID: "", path: "" } :
                          { workspaceID, objectID: comparisonQuery.data.head_object_id,
                            path: change.path })}
                      title="Preview redacted head file" type="button">
                      <FileOutput aria-hidden="true" size={14} />
                    </button> : <span aria-hidden="true" className="repository-preview-placeholder" />}
                  {["regular", "executable"].includes(change.previous_kind) ||
                    ["regular", "executable"].includes(change.current_kind) ?
                    <button aria-label={`Compare redacted previews for ${change.path} between ${comparisonQuery.data.base_hash} and ${comparisonQuery.data.head_hash}`}
                      aria-pressed={activeComparisonPreview?.path === change.path}
                      className="icon-button" onClick={() => selectPairedPreview(
                        pairedPreviewCandidates.findIndex((candidate) => candidate.path === change.path),
                        true)}
                      title="Open paired redacted previews" type="button">
                      <Columns2 aria-hidden="true" size={14} />
                    </button> : <span aria-hidden="true" className="repository-preview-placeholder" />}
                </span>
              </div>)}</div>}
          {comparisonQuery.data.omitted_change_count > 0 &&
            <p className="repository-diff-omitted">
              {comparisonQuery.data.omitted_change_count} additional changes omitted
            </p>}
          {activeComparisonPreview &&
            <section aria-label="Paired redacted file preview"
              className="repository-comparison-preview-workspace">
              <header><span><Columns2 aria-hidden="true" size={14} />
                <code title={activeComparisonPreview.path}>{activeComparisonPreview.path}</code></span>
                <div className="repository-comparison-preview-controls">
                  <span aria-live="polite">{pairedPreviewIndex + 1} of {pairedPreviewCandidates.length}</span>
                  <button aria-label="Previous paired redacted preview" className="icon-button"
                    disabled={pairedPreviewIndex <= 0}
                    onClick={() => selectPairedPreview(pairedPreviewIndex - 1)}
                    title="Previous changed file" type="button">
                    <ChevronLeft aria-hidden="true" size={14} />
                  </button>
                  <button aria-label="Next paired redacted preview" className="icon-button"
                    disabled={pairedPreviewIndex < 0 ||
                      pairedPreviewIndex >= pairedPreviewCandidates.length - 1}
                    onClick={() => selectPairedPreview(pairedPreviewIndex + 1)}
                    title="Next changed file" type="button">
                    <ChevronRight aria-hidden="true" size={14} />
                  </button>
                  <StatusBadge status="read-only" />
                </div></header>
              <div>
                <section aria-label="Base redacted file preview">
                  <header><strong>Base</strong><code>{activeComparisonPreview.baseHash}</code></header>
                  {!activeComparisonPreview.baseAvailable ?
                    <EmptyState>File is absent at the base commit</EmptyState> : <>
                      {comparisonBasePreviewQuery.isLoading &&
                        <LoadingState label="Loading redacted base file" />}
                      {comparisonBasePreviewQuery.isError &&
                        <ErrorState error={comparisonBasePreviewQuery.error} />}
                      {comparisonBasePreviewQuery.data && <>
                        <div className="repository-comparison-preview-meta">
                          <code title={`${comparisonBasePreviewQuery.data.hash} / ${comparisonBasePreviewQuery.data.path}`}>
                            {comparisonBasePreviewQuery.data.hash} / {comparisonBasePreviewQuery.data.path}
                          </code><span><StatusBadge status={comparisonBasePreviewQuery.data.kind} />
                            {comparisonBasePreviewQuery.data.redacted &&
                              <StatusBadge status="redacted" />}</span></div>
                        <pre>{comparisonBasePreviewQuery.data.content}</pre>
                      </>}
                    </>}
                </section>
                <section aria-label="Head redacted file preview">
                  <header><strong>Head</strong><code>{activeComparisonPreview.headHash}</code></header>
                  {!activeComparisonPreview.headAvailable ?
                    <EmptyState>File is absent at the head commit</EmptyState> : <>
                      {comparisonHeadPreviewQuery.isLoading &&
                        <LoadingState label="Loading redacted head file" />}
                      {comparisonHeadPreviewQuery.isError &&
                        <ErrorState error={comparisonHeadPreviewQuery.error} />}
                      {comparisonHeadPreviewQuery.data && <>
                        <div className="repository-comparison-preview-meta">
                          <code title={`${comparisonHeadPreviewQuery.data.hash} / ${comparisonHeadPreviewQuery.data.path}`}>
                            {comparisonHeadPreviewQuery.data.hash} / {comparisonHeadPreviewQuery.data.path}
                          </code><span><StatusBadge status={comparisonHeadPreviewQuery.data.kind} />
                            {comparisonHeadPreviewQuery.data.redacted &&
                              <StatusBadge status="redacted" />}</span></div>
                        <pre>{comparisonHeadPreviewQuery.data.content}</pre>
                      </>}
                    </>}
                </section>
              </div>
            </section>}
        </>}
      </section>}
    {selectedHistoryPath && <section aria-label="Exact file history"
      className="repository-file-history">
      {fileHistoryQuery.isLoading && <LoadingState label="Loading exact file history" />}
      {fileHistoryQuery.isError && <ErrorState error={fileHistoryQuery.error} />}
      {fileHistoryQuery.data && <>
        <header><code title={fileHistoryQuery.data.path}>{fileHistoryQuery.data.path}</code>
          <span><StatusBadge status={`${fileHistoryQuery.data.returned_entry_count} changes`} />
            {fileHistoryQuery.data.truncated && <StatusBadge status="truncated" />}</span></header>
        {!fileHistoryQuery.data.observed ? <EmptyState>No changes found in the bounded history</EmptyState> :
          <div className="repository-file-history-list">{fileHistoryQuery.data.entries.map((entry) =>
            <div key={entry.object_id}><StatusBadge status={entry.change} />
              <code>{entry.hash}</code><span>{entry.subject}</span>
              <time dateTime={entry.committed_at}>{formatDate(entry.committed_at)}</time>
              <span>{entry.previous_kind || "none"} to {entry.current_kind || "none"}</span>
              <span className="repository-file-history-flags">
                {entry.redacted && <StatusBadge status="redacted" />}
              </span>
              <span className="repository-file-history-actions">
                <button aria-label={`Open commit ${entry.hash} from history for ${fileHistoryQuery.data.path}`}
                  aria-pressed={selectedObjectID === entry.object_id} className="icon-button"
                  onClick={() => {
                    setSelection({ workspaceID, objectID: entry.object_id });
                    setFileSelection({ workspaceID: "", objectID: "", path: "" });
                  }} title="Open exact commit" type="button">
                  <FileDiff aria-hidden="true" size={14} />
                </button>
                {["regular", "executable"].includes(entry.current_kind) ?
                  <button aria-label={`Preview ${fileHistoryQuery.data.path} at history commit ${entry.hash}`}
                    aria-pressed={selectedObjectID === entry.object_id &&
                      selectedFilePath === fileHistoryQuery.data.path} className="icon-button"
                    onClick={() => {
                      setSelection({ workspaceID, objectID: entry.object_id });
                      setFileSelection({ workspaceID, objectID: entry.object_id,
                        path: fileHistoryQuery.data.path });
                    }} title="Preview redacted file at exact commit" type="button">
                    <FileCode2 aria-hidden="true" size={14} />
                  </button> : <span aria-hidden="true" className="repository-preview-placeholder" />}
              </span>
            </div>)}</div>}
      </>}
    </section>}
  </section>;
}
