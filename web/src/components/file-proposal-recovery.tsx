import { useEffect, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { DiffEditor } from "@monaco-editor/react";
import { AlertTriangle, History, X } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { ErrorState, LoadingState, StatusBadge } from "./common";

export function FileProposalRecovery({ client, runID, editID, onClose }: {
  client: CyberAgentClient;
  runID: string;
  editID: string;
  onClose: () => void;
}) {
  const [editorState, setEditorState] = useState<"loading" | "ready" | "failed">("loading");
  const query = useQuery({
    queryKey: ["run", runID, "file-edit-recovery", editID],
    queryFn: ({ signal }) => client.recoverFileEditProposal(runID, editID, signal),
  });
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
  if (query.isLoading) return <RecoveryStatus onClose={onClose}>
    <LoadingState label="Recovering pending proposal" />
  </RecoveryStatus>;
  if (query.isError || !query.data) return <RecoveryStatus onClose={onClose}>
    <ErrorState error={query.error} />
  </RecoveryStatus>;
  const recovery = query.data;
  return <div className="file-proposal-editor file-proposal-recovery"
    aria-label={`Recovered proposal for ${recovery.path}`}>
    <header>
      <div><History aria-hidden="true" size={16} /><strong>{recovery.path}</strong>
        <StatusBadge status={recovery.stale ? "stale" : "pending review"} /></div>
      <button aria-label="Close recovered proposal" className="icon-button"
        onClick={onClose} title="Close recovery" type="button">
        <X aria-hidden="true" size={15} />
      </button>
    </header>
    <div className="monaco-proposal-surface">
      {editorState === "ready" ? <DiffEditor height="100%" language={languageForPath(recovery.path)}
        modified={recovery.proposed_content} original={recovery.original_content}
        options={{ automaticLayout: true, fontSize: 13, letterSpacing: 0,
          minimap: { enabled: false }, readOnly: true, renderSideBySide: true,
          scrollBeyondLastLine: false }} theme="vs" /> :
        <div className={`monaco-local-state${editorState === "failed" ? " inline-warning" : ""}`}
          role={editorState === "failed" ? "alert" : "status"}>
          {editorState === "failed" ? "Local editor assets could not be loaded" : "Loading local editor"}
        </div>}
    </div>
    <footer>
      {recovery.stale && <span className="stale-proposal-status">
        <AlertTriangle aria-hidden="true" size={14} />Workspace source changed</span>}
      <span>Original {recovery.original_sha256.slice(0, 12)}</span>
      <span>Proposal {recovery.proposed_sha256.slice(0, 12)}</span>
    </footer>
  </div>;
}

function RecoveryStatus({ children, onClose }: {
  children: ReactNode;
  onClose: () => void;
}) {
  return <section aria-label="File proposal recovery status"
    className="file-proposal-editor file-proposal-recovery">
    <header>
      <div><History aria-hidden="true" size={16} /><strong>Proposal recovery</strong></div>
      <button aria-label="Close recovered proposal" className="icon-button"
        onClick={onClose} title="Close recovery" type="button">
        <X aria-hidden="true" size={15} />
      </button>
    </header>
    {children}
  </section>;
}

function languageForPath(path: string): string {
  const extension = path.includes(".") ? path.slice(path.lastIndexOf(".") + 1).toLowerCase() : "";
  return ({ go: "go", ts: "typescript", tsx: "typescript", js: "javascript",
    jsx: "javascript", json: "json", md: "markdown", py: "python", rs: "rust",
    css: "css", html: "html", yaml: "yaml", yml: "yaml", sh: "shell" } as Record<string, string>)[extension] ?? "plaintext";
}
