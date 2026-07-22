import { FolderOpen, X } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { WorkspaceExplorer } from "./workspace-explorer";

export function WorkspaceAttachmentDialog({ client, open, onClose, runID, workspaceID }: {
  client: CyberAgentClient;
  open: boolean;
  onClose: () => void;
  runID: string;
  workspaceID: string;
}) {
  if (!open) return null;
  return <div className="desktop-dialog-backdrop" role="presentation">
    <section aria-labelledby="workspace-attachment-title" aria-modal="true"
      className="desktop-dialog workspace-attachment-dialog" role="dialog">
      <header>
        <div><span className="dialog-icon"><FolderOpen aria-hidden="true" size={17} /></span>
          <div><h2 id="workspace-attachment-title">文件和文件夹</h2>
            <small>工作区证据浏览器</small></div></div>
        <button aria-label="关闭" className="icon-button" onClick={onClose}
          title="关闭" type="button"><X aria-hidden="true" size={16} /></button>
      </header>
      <div className="workspace-attachment-body">
        <WorkspaceExplorer client={client} runID={runID} workspaceID={workspaceID} />
      </div>
    </section>
  </div>;
}
