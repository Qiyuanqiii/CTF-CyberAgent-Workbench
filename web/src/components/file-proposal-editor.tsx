import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import Editor, { DiffEditor } from "@monaco-editor/react";
import { Check, FileDiff, LoaderCircle, Pencil, RefreshCw, X } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { FileEditProposalSourceView } from "../api/types";
import { ErrorState, StatusBadge } from "./common";

export function FileProposalEditor({ client, runID, source, onClose }: {
  client: CyberAgentClient;
  runID: string;
  source: FileEditProposalSourceView;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [activeSource, setActiveSource] = useState(source);
  const [draft, setDraft] = useState(source.content);
  const [expired, setExpired] = useState(Date.parse(source.expires_at) <= Date.now());
  const [mode, setMode] = useState<"edit" | "diff">("edit");
  const [editorState, setEditorState] = useState<"loading" | "ready" | "failed">("loading");
  useEffect(() => {
    let active = true;
    void import("../lib/monaco-local").then(({ configureLocalMonaco }) => {
      configureLocalMonaco();
      if (active) setEditorState("ready");
    }).catch(() => {
      if (active) setEditorState("failed");
    });
    return () => { active = false; };
  }, []);
  useEffect(() => {
    setActiveSource(source);
    setDraft(source.content);
  }, [source]);
  useEffect(() => {
    const remaining = Date.parse(activeSource.expires_at) - Date.now();
    setExpired(remaining <= 0);
    if (remaining <= 0) return;
    const timer = window.setTimeout(() => setExpired(true),
      Math.min(remaining, 2_147_483_647));
    return () => window.clearTimeout(timer);
  }, [activeSource.expires_at]);
  const mutation = useMutation({
    mutationFn: () => client.createFileEditProposal(runID, {
      version: "file_edit_proposal.v1",
      source_handle: activeSource.source_handle,
      proposed_text: draft,
    }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "file-edits"] });
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "approvals"] });
    },
  });
  const refresh = useMutation({
    mutationFn: () => client.reissueFileEditProposalSource(runID, activeSource.path,
      activeSource.content_sha256),
    onSuccess: (renewed) => {
      setActiveSource(renewed);
      mutation.reset();
    },
  });
  const bytes = new TextEncoder().encode(draft).length;
  const invalid = editorState !== "ready" || bytes > 256 * 1024 ||
    draft === activeSource.content || expired;
  const language = languageForPath(activeSource.path);

  return <section className="file-proposal-editor" aria-label={`Edit proposal for ${activeSource.path}`}>
    <header>
      <div><Pencil aria-hidden="true" size={16} /><strong>{activeSource.path}</strong>
        <StatusBadge status="proposal only" /></div>
      <div className="segmented-control" aria-label="Editor mode">
        <button aria-pressed={mode === "edit"} onClick={() => setMode("edit")} type="button">
          <Pencil aria-hidden="true" size={14} />Edit
        </button>
        <button aria-pressed={mode === "diff"} onClick={() => setMode("diff")} type="button">
          <FileDiff aria-hidden="true" size={14} />Diff
        </button>
      </div>
      <button aria-label="Close file proposal editor" className="icon-button"
        onClick={onClose} title="Close editor" type="button">
        <X aria-hidden="true" size={15} />
      </button>
    </header>
    <div className="monaco-proposal-surface">
      {editorState === "loading" && <div className="monaco-local-state" role="status">
        Loading local editor
      </div>}
      {editorState === "failed" && <div className="monaco-local-state inline-warning" role="alert">
        Local editor assets could not be loaded
      </div>}
      {editorState === "ready" && (mode === "edit" ?
        <Editor height="100%" language={language} onChange={(value) => setDraft(value ?? "")}
        options={{ automaticLayout: true, fontSize: 13, letterSpacing: 0, minimap: { enabled: false },
          renderWhitespace: "selection", scrollBeyondLastLine: false, tabSize: 2, wordWrap: "off" }}
        path={`proposal://${runID}/${activeSource.content_sha256}/${activeSource.path}`}
        theme="vs" value={draft} /> :
        <DiffEditor height="100%" language={language} modified={draft} original={activeSource.content}
          options={{ automaticLayout: true, fontSize: 13, letterSpacing: 0, minimap: { enabled: false },
            readOnly: true, renderSideBySide: true, scrollBeyondLastLine: false }}
          theme="vs" />)}
    </div>
    <footer>
      <span>{bytes.toLocaleString()} / {(256 * 1024).toLocaleString()} bytes</span>
      <span>Source {activeSource.content_sha256.slice(0, 12)}</span>
      {!mutation.data && (expired || mutation.isError) &&
        <button className="compact-command" disabled={refresh.isPending}
          onClick={() => refresh.mutate()} type="button">
          {refresh.isPending ? <LoaderCircle aria-hidden="true" className="spin" size={14} />
            : <RefreshCw aria-hidden="true" size={14} />}
          Refresh source
        </button>}
      {mutation.data ? <span className="proposal-created-status" role="status">
        <Check aria-hidden="true" size={14} />Pending review {mutation.data.edit.id.slice(0, 12)}
      </span> : <button className="compact-command" disabled={invalid || mutation.isPending}
        onClick={() => mutation.mutate()} type="button">
        {mutation.isPending ? <LoaderCircle aria-hidden="true" className="spin" size={14} />
          : <FileDiff aria-hidden="true" size={14} />}
        Create proposal
      </button>}
    </footer>
    {bytes > 256 * 1024 && <div className="inline-warning" role="alert">
      Proposal exceeds the 256 KiB Go boundary
    </div>}
    {mutation.isError && <ErrorState error={mutation.error} />}
    {refresh.isError && <ErrorState error={refresh.error} />}
  </section>;
}

function languageForPath(path: string): string {
  const extension = path.includes(".") ? path.slice(path.lastIndexOf(".") + 1).toLowerCase() : "";
  return ({ go: "go", ts: "typescript", tsx: "typescript", js: "javascript",
    jsx: "javascript", json: "json", md: "markdown", py: "python", rs: "rust",
    css: "css", html: "html", yaml: "yaml", yml: "yaml", sh: "shell" } as Record<string, string>)[extension] ?? "plaintext";
}
