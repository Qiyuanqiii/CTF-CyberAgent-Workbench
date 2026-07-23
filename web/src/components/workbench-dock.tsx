import { useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import {
  Bot,
  ChevronDown,
  Code2,
  FileDiff,
  FileText,
  Folder,
  FolderOpen,
  Globe2,
  ListTree,
  LoaderCircle,
  PanelBottom,
  PanelRight,
  Plus,
  SquareTerminal,
  X,
} from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type {
  RepositoryStateView,
  RunDetailView,
  SessionDetailView,
  WorkItemView,
} from "../api/types";
import {
  desktopErrorMessage,
  listDesktopWorkspaceLaunchers,
  openDesktopWorkspace,
  type DesktopWorkspaceLauncher,
} from "../lib/desktop-bridge";
import { formatCompactDate, shortID } from "../lib/format";
import { usePagedResource } from "../hooks/use-paged-resource";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";
import { RepositoryDiffPanel } from "./repository-diff-panel";
import { WorkspaceExplorer } from "./workspace-explorer";

export type WorkbenchResourceKind = "run" | "session";
type SidecarTab = "review" | "terminal" | "browser" | "files" | "tasks";

interface WorkbenchContext {
  goal: string;
  mode: string;
  resourceLabel: string;
  status: string;
  updatedAt: string;
  workspaceID: string;
}

const sidecarItems: Array<{
  id: SidecarTab;
  label: string;
  shortcut: string;
  icon: typeof FileDiff;
}> = [
  { id: "review", label: "审阅", shortcut: "Ctrl+Shift+G", icon: FileDiff },
  { id: "terminal", label: "终端", shortcut: "", icon: SquareTerminal },
  { id: "browser", label: "浏览器", shortcut: "Ctrl+T", icon: Globe2 },
  { id: "files", label: "文件", shortcut: "Ctrl+P", icon: Folder },
  { id: "tasks", label: "侧边任务", shortcut: "Ctrl+Alt+S", icon: Bot },
];

export function WorkbenchDock({ children, client, desktop, resourceKind, runID, sessionID,
  title }: {
  children: ReactNode;
  client: CyberAgentClient;
  desktop: boolean;
  resourceKind: WorkbenchResourceKind;
  runID: string;
  sessionID: string;
  title: string;
}) {
  const [summaryVisible, setSummaryVisible] = useState(false);
  const [bottomVisible, setBottomVisible] = useState(false);
  const [sidecarVisible, setSidecarVisible] = useState(false);
  const [sidecarTab, setSidecarTab] = useState<SidecarTab>("files");
  const [sidecarMenuOpen, setSidecarMenuOpen] = useState(false);
  const sidecarMenuRef = useRef<HTMLDivElement>(null);
  const context = useWorkbenchContext(client, resourceKind, runID, sessionID);

  useDismissablePopover(sidecarMenuRef, sidecarMenuOpen, () => setSidecarMenuOpen(false));
  useEffect(() => {
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      if (!event.ctrlKey || event.metaKey) return;
      if (event.key.toLowerCase() === "j" && !event.altKey && !event.shiftKey) {
        event.preventDefault();
        setBottomVisible((visible) => !visible);
      } else if (event.key.toLowerCase() === "g" && event.shiftKey && !event.altKey) {
        event.preventDefault();
        setSidecarTab("review");
        setSidecarVisible(true);
      } else if (event.key.toLowerCase() === "t" && !event.altKey && !event.shiftKey) {
        event.preventDefault();
        setSidecarTab("browser");
        setSidecarVisible(true);
      } else if (event.key.toLowerCase() === "p" && !event.altKey && !event.shiftKey) {
        event.preventDefault();
        setSidecarTab("files");
        setSidecarVisible(true);
      } else if (event.key.toLowerCase() === "s" && event.altKey && !event.shiftKey) {
        event.preventDefault();
        setSidecarTab("tasks");
        setSidecarVisible(true);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const selectSidecar = (tab: SidecarTab) => {
    setSidecarTab(tab);
    setSidecarVisible(true);
    setSidecarMenuOpen(false);
  };

  return <main className="main-workspace prayu-conversation-panel">
    <header className="workspace-panel-toolbar">
      <div className="workspace-panel-location">
        <FolderOpen aria-hidden="true" size={16} />
        <strong>{title}</strong>
      </div>
      <div aria-label="工作台面板" className="workspace-panel-actions" role="toolbar">
        <WorkspaceOpenMenu desktop={desktop} workspaceID={context.workspaceID} />
        <button aria-label="切换摘要" aria-pressed={summaryVisible}
          className={`workspace-panel-icon ${summaryVisible ? "active" : ""}`}
          onClick={() => setSummaryVisible((visible) => !visible)}
          title="切换摘要" type="button">
          <ListTree aria-hidden="true" size={16} />
        </button>
        <button aria-label="切换底部面板显示" aria-pressed={bottomVisible}
          className={`workspace-panel-icon ${bottomVisible ? "active" : ""}`}
          onClick={() => setBottomVisible((visible) => !visible)}
          title="切换底部面板显示 (Ctrl+J)" type="button">
          <PanelBottom aria-hidden="true" size={16} />
        </button>
        <button aria-label="显示或隐藏右侧栏" aria-pressed={sidecarVisible}
          className={`workspace-panel-icon ${sidecarVisible ? "active" : ""}`}
          onClick={() => setSidecarVisible((visible) => !visible)}
          title="显示或隐藏右侧栏" type="button">
          <PanelRight aria-hidden="true" size={16} />
        </button>
      </div>
    </header>
    <div className={`workbench-dock-layout ${sidecarVisible ? "with-sidecar" : ""}`}>
      <div className="workbench-center-stack">
        <div className={`workbench-center-row ${summaryVisible ? "with-summary" : ""}`}>
          <div className="workspace-panel-stage">{children}</div>
          {summaryVisible && <WorkbenchSummary client={client} context={context} />}
        </div>
        {bottomVisible && <BottomPanel onClose={() => setBottomVisible(false)} />}
      </div>
      {sidecarVisible && <aside aria-label="右侧工具栏" className="workbench-sidecar">
        <header className="workbench-sidecar-tabs">
          <button className="workbench-sidecar-tab" type="button">
            {sidecarTabIcon(sidecarTab)}<span>{sidecarLabel(sidecarTab)}</span>
          </button>
          <div className="workbench-sidecar-add" ref={sidecarMenuRef}>
            <button aria-expanded={sidecarMenuOpen} aria-haspopup="menu"
              aria-label="添加右侧工具" className="workspace-panel-icon"
              onClick={() => setSidecarMenuOpen((open) => !open)} title="添加右侧工具"
              type="button"><Plus aria-hidden="true" size={17} /></button>
            {sidecarMenuOpen && <div aria-label="右侧工具" className="workbench-sidecar-menu" role="menu">
              {sidecarItems.map(({ id, label, shortcut, icon: Icon }) =>
                <button aria-checked={sidecarTab === id} key={id}
                  onClick={() => selectSidecar(id)} role="menuitemradio" type="button">
                  <Icon aria-hidden="true" size={16} /><span>{label}</span>
                  {shortcut && <kbd>{shortcut}</kbd>}
                </button>)}
            </div>}
          </div>
          <button aria-label="关闭右侧栏" className="workspace-panel-icon"
            onClick={() => setSidecarVisible(false)} title="关闭右侧栏" type="button">
            <X aria-hidden="true" size={16} />
          </button>
        </header>
        <div className="workbench-sidecar-body">
          <SidecarContent client={client} context={context} runID={runID} tab={sidecarTab} />
        </div>
      </aside>}
    </div>
  </main>;
}

function WorkspaceOpenMenu({ desktop, workspaceID }: { desktop: boolean; workspaceID: string }) {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [launchers, setLaunchers] = useState<DesktopWorkspaceLauncher[]>([]);
  const [message, setMessage] = useState("");
  const root = useRef<HTMLDivElement>(null);
  useDismissablePopover(root, open, () => setOpen(false));

  const toggle = async () => {
    if (!desktop || !workspaceID) return;
    const next = !open;
    setOpen(next);
    setMessage("");
    if (!next || launchers.length > 0) return;
    setLoading(true);
    try {
      setLaunchers(await listDesktopWorkspaceLaunchers(workspaceID));
    } catch (error) {
      setMessage(desktopErrorMessage(error));
    } finally {
      setLoading(false);
    }
  };
  const launch = async (launcher: DesktopWorkspaceLauncher) => {
    setLoading(true);
    setMessage("");
    try {
      const result = await openDesktopWorkspace(workspaceID, launcher.id);
      setMessage(result.status === "started" ? `已在 ${launcher.label} 中打开` : "已取消打开");
      if (result.status === "started") setOpen(false);
    } catch (error) {
      setMessage(desktopErrorMessage(error));
    } finally {
      setLoading(false);
    }
  };

  return <div className="workspace-open-root" ref={root}>
    <button aria-expanded={open} aria-haspopup="menu" aria-label="打开工作区"
      className="workspace-open-button" disabled={!desktop || !workspaceID || loading}
      onClick={() => void toggle()} title={desktop ? "打开工作区" : "桌面版支持打开工作区"}
      type="button">
      {loading ? <LoaderCircle aria-hidden="true" className="spin" size={15} /> :
        <FolderOpen aria-hidden="true" size={15} />}
      <ChevronDown aria-hidden="true" size={14} />
    </button>
    {open && <div aria-label="打开方式" className="workspace-open-menu" role="menu">
      {launchers.map((launcher) => <button key={launcher.id}
        onClick={() => void launch(launcher)} role="menuitem" type="button">
        {launcherIcon(launcher.kind)}<span>{launcher.label}</span>
      </button>)}
      {!loading && launchers.length === 0 && !message && <span>没有可用的打开方式</span>}
      {message && <span className="workspace-open-message" role="status">{message}</span>}
    </div>}
    {!open && message && <span className="workspace-open-live" role="status">{message}</span>}
  </div>;
}

function WorkbenchSummary({ client, context }: {
  client: CyberAgentClient;
  context: WorkbenchContext;
}) {
  const repository = useQuery({
    queryKey: ["workspace", context.workspaceID, "repository-state"],
    queryFn: ({ signal }) => client.repositoryState(context.workspaceID, signal),
    enabled: Boolean(context.workspaceID),
  });
  return <aside aria-label="摘要" className="workbench-summary">
    <header><span>环境信息</span></header>
    <SummaryRepository query={repository} workspaceID={context.workspaceID} />
    <section>
      <h2>当前任务</h2>
      {context.resourceLabel ? <>
        <div className="summary-item-heading"><strong>{context.resourceLabel}</strong>
          {context.status && <StatusBadge status={context.status} />}</div>
        {context.goal && <p>{context.goal}</p>}
        <dl><div><dt>模式</dt><dd>{context.mode || "-"}</dd></div>
          <div><dt>更新</dt><dd>{context.updatedAt ? formatCompactDate(context.updatedAt) : "-"}</dd></div></dl>
      </> : <EmptyState>尚未选择任务</EmptyState>}
    </section>
    <section>
      <h2>后台活动</h2>
      <div className="summary-inert-row"><SquareTerminal aria-hidden="true" size={15} />
        <span>没有由工作台启动的后台进程</span></div>
    </section>
  </aside>;
}

function SummaryRepository({ query, workspaceID }: {
  query: UseQueryResult<RepositoryStateView, Error>;
  workspaceID: string;
}) {
  if (!workspaceID) return <section><EmptyState>当前任务未绑定 Workspace</EmptyState></section>;
  if (query.isLoading) return <section><LoadingState label="加载仓库摘要" /></section>;
  if (query.isError || !query.data) return <section><ErrorState error={query.error} /></section>;
  if (!query.data.available) return <section><p>当前 Workspace 不是 Git 仓库</p></section>;
  const state = query.data;
  return <section>
    <div className="summary-item-heading"><strong>{state.detached ? "detached" : state.branch || "unborn"}</strong>
      <StatusBadge status={state.clean ? "clean" : "changed"} /></div>
    <dl>
      <div><dt>暂存</dt><dd>{state.staged_count}</dd></div>
      <div><dt>变更</dt><dd>{state.worktree_count}</dd></div>
      <div><dt>未跟踪</dt><dd>{state.untracked_count}</dd></div>
      <div><dt>冲突</dt><dd>{state.conflicted_count}</dd></div>
    </dl>
  </section>;
}

function BottomPanel({ onClose }: { onClose: () => void }) {
  return <section aria-label="底部面板" className="workbench-bottom-panel">
    <header><button className="active" type="button"><SquareTerminal aria-hidden="true" size={14} />终端</button>
      <button aria-label="关闭底部面板" className="workspace-panel-icon" onClick={onClose}
        title="关闭底部面板" type="button"><X aria-hidden="true" size={15} /></button></header>
    <div className="workbench-terminal-empty">
      <SquareTerminal aria-hidden="true" size={22} />
      <strong>终端尚未启用</strong>
      <span>当前权限档位不会启动本机 Shell</span>
    </div>
  </section>;
}

function SidecarContent({ client, context, runID, tab }: {
  client: CyberAgentClient;
  context: WorkbenchContext;
  runID: string;
  tab: SidecarTab;
}) {
  if (tab === "review") {
    return context.workspaceID ? <RepositoryDiffPanel client={client} workspaceID={context.workspaceID} /> :
      <EmptyState>当前任务未绑定 Workspace</EmptyState>;
  }
  if (tab === "files") {
    return <WorkspaceExplorer client={client} runID={runID} workspaceID={context.workspaceID} />;
  }
  if (tab === "tasks") {
    return <SideTaskPanel client={client} runID={runID} />;
  }
  if (tab === "browser") {
    return <div className="workbench-tool-empty"><Globe2 aria-hidden="true" size={29} />
      <strong>浏览器尚未启动</strong><span>浏览器运行时仍处于安全审查阶段</span></div>;
  }
  return <div className="workbench-tool-empty"><SquareTerminal aria-hidden="true" size={29} />
    <strong>终端尚未启动</strong><span>当前权限档位不允许工作台启动 Shell</span></div>;
}

function SideTaskPanel({ client, runID }: { client: CyberAgentClient; runID: string }) {
  const query = usePagedResource<WorkItemView>(client, ["run", runID, "sidecar-work-items"],
    `/runs/${encodeURIComponent(runID)}/work-items`, { limit: 50 }, Boolean(runID));
  const items = useMemo(() => query.data?.pages.flatMap((page) => page.items) ?? [], [query.data]);
  if (!runID) return <EmptyState>当前未选择 Run</EmptyState>;
  if (query.isLoading) return <LoadingState label="加载侧边任务" />;
  if (query.isError) return <ErrorState error={query.error} />;
  if (items.length === 0) return <EmptyState>暂无侧边任务</EmptyState>;
  return <div className="workbench-side-task-list">
    {items.map((item) => <article key={item.id}>
      <div><Bot aria-hidden="true" size={15} /><strong>{item.title}</strong></div>
      <StatusBadge status={item.status} />
      {item.owner && <small>{item.owner}</small>}
    </article>)}
  </div>;
}

function useWorkbenchContext(client: CyberAgentClient, resourceKind: WorkbenchResourceKind,
  runID: string, sessionID: string): WorkbenchContext {
  const run = useQuery({
    queryKey: ["run", runID],
    queryFn: ({ signal }) => client.get<RunDetailView>(`/runs/${encodeURIComponent(runID)}`, {}, signal),
    enabled: resourceKind === "run" && Boolean(runID),
  });
  const session = useQuery({
    queryKey: ["session", sessionID],
    queryFn: ({ signal }) => client.get<SessionDetailView>(
      `/sessions/${encodeURIComponent(sessionID)}`, {}, signal),
    enabled: resourceKind === "session" && Boolean(sessionID),
  });
  if (resourceKind === "run" && run.data) {
    return {
      goal: run.data.mission.goal,
      mode: `${run.data.mode.phase} / ${run.data.mission.profile}`,
      resourceLabel: `Run ${shortID(run.data.run.id)}`,
      status: run.data.run.status,
      updatedAt: run.data.run.updated_at,
      workspaceID: run.data.mission.workspace_id ?? "",
    };
  }
  if (resourceKind === "session" && session.data) {
    return {
      goal: session.data.session.title,
      mode: session.data.session.route,
      resourceLabel: `Session ${shortID(session.data.session.id)}`,
      status: session.data.session.status,
      updatedAt: session.data.session.updated_at,
      workspaceID: session.data.session.workspace_id ?? "",
    };
  }
  return { goal: "", mode: "", resourceLabel: "", status: "", updatedAt: "", workspaceID: "" };
}

function useDismissablePopover(root: RefObject<HTMLElement | null>, open: boolean,
  close: () => void) {
  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: globalThis.PointerEvent) => {
      if (event.target instanceof Node && !root.current?.contains(event.target)) close();
    };
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") close();
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [close, open, root]);
}

function sidecarLabel(tab: SidecarTab): string {
  return sidecarItems.find((item) => item.id === tab)?.label ?? "工具";
}

function sidecarTabIcon(tab: SidecarTab): ReactNode {
  const Icon = sidecarItems.find((item) => item.id === tab)?.icon ?? FileText;
  return <Icon aria-hidden="true" size={15} />;
}

function launcherIcon(kind: DesktopWorkspaceLauncher["kind"]): ReactNode {
  if (kind === "folder") return <FolderOpen aria-hidden="true" size={16} />;
  if (kind === "terminal") return <SquareTerminal aria-hidden="true" size={16} />;
  return <Code2 aria-hidden="true" size={16} />;
}
