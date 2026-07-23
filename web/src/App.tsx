import { useEffect, useMemo, useState, type CSSProperties } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowRight,
  LogOut,
  Minus,
  PanelLeft,
  RefreshCw,
  Settings,
  Square,
  X,
} from "lucide-react";
import { CyberAgentClient } from "./api/client";
import { ConnectionGate } from "./components/connection-gate";
import { DesktopSkillPreviewDialog } from "./components/desktop-skill-preview";
import { ModelAvailabilityDialog, ModelAvailabilityWorkspace } from "./components/model-availability-dialog";
import { RunCreationDialog } from "./components/run-creation-dialog";
import { ResourceSidebar, type WorkbenchSection } from "./components/resource-sidebar";
import { RunWorkspace } from "./components/run-workspace";
import { SessionWorkspace } from "./components/session-workspace";
import { SettingsView, type SettingsCapability } from "./components/settings-view";
import { EmptyConversation, SidebarResizeHandle, UtilityWorkspace,
  WorkbenchFrame, clampSidebarWidth, defaultSidebarWidth,
  type NewRunDraft } from "./components/workbench-frame";
import { desktopBridgeAvailable } from "./lib/desktop-bridge";
import { closeDesktopWindow, minimiseDesktopWindow,
  toggleDesktopWindowMaximised } from "./lib/desktop-window";
import { useConnectionStore } from "./state/connection";

const sidebarWidthStorageKey = "prayu.sidebar.width.v1";

export default function App() {
  const token = useConnectionStore((state) => state.token);
  const controlToken = useConnectionStore((state) => state.controlToken);
  const runControlEnabled = useConnectionStore((state) => state.runControlEnabled);
  const runCreationEnabled = useConnectionStore((state) => state.runCreationEnabled);
  const sessionMessageEnabled = useConnectionStore((state) => state.sessionMessageEnabled);
  const sessionSteeringControlEnabled = useConnectionStore(
    (state) => state.sessionSteeringControlEnabled);
  const runLifecycleEnabled = useConnectionStore((state) => state.runLifecycleEnabled);
  const runExecutionEnabled = useConnectionStore((state) => state.runExecutionEnabled);
  const planDeliveryControlEnabled = useConnectionStore(
    (state) => state.planDeliveryControlEnabled);
  const approvalControlEnabled = useConnectionStore((state) => state.approvalControlEnabled);
  const modelControlEnabled = useConnectionStore((state) => state.modelControlEnabled);
  const providerCredentialEnabled = useConnectionStore((state) => state.providerCredentialEnabled);
  const fileEditReviewEnabled = useConnectionStore((state) => state.fileEditReviewEnabled);
  const fileEditProposalEnabled = useConnectionStore((state) => state.fileEditProposalEnabled);
  const fileEditApplyEnabled = useConnectionStore((state) => state.fileEditApplyEnabled);
  const runWakeControlEnabled = useConnectionStore((state) => state.runWakeControlEnabled);
  const runWakeExecutionEnabled = useConnectionStore((state) => state.runWakeExecutionEnabled);
  const runWakeWorkerEnabled = useConnectionStore((state) => state.runWakeWorkerEnabled);
  const skillInstallationEnabled = useConnectionStore((state) => state.skillInstallationEnabled);
  const evidenceAttachmentEnabled = useConnectionStore((state) => state.evidenceAttachmentEnabled);
  const verificationEvidenceEnabled = useConnectionStore(
    (state) => state.verificationEvidenceEnabled);
  if (!token) {
    return <ConnectionGate />;
  }
  return <ConnectedWorkbench token={token} controlToken={controlToken}
    runControlEnabled={runControlEnabled} runCreationEnabled={runCreationEnabled}
    sessionMessageEnabled={sessionMessageEnabled}
    sessionSteeringControlEnabled={sessionSteeringControlEnabled}
    runLifecycleEnabled={runLifecycleEnabled} runExecutionEnabled={runExecutionEnabled}
    planDeliveryControlEnabled={planDeliveryControlEnabled}
    approvalControlEnabled={approvalControlEnabled} modelControlEnabled={modelControlEnabled}
    providerCredentialEnabled={providerCredentialEnabled}
    fileEditReviewEnabled={fileEditReviewEnabled} fileEditApplyEnabled={fileEditApplyEnabled}
    fileEditProposalEnabled={fileEditProposalEnabled}
    runWakeControlEnabled={runWakeControlEnabled}
    runWakeExecutionEnabled={runWakeExecutionEnabled}
    runWakeWorkerEnabled={runWakeWorkerEnabled}
    skillInstallationEnabled={skillInstallationEnabled}
    evidenceAttachmentEnabled={evidenceAttachmentEnabled}
    verificationEvidenceEnabled={verificationEvidenceEnabled} />;
}

function ConnectedWorkbench({ token, controlToken, runControlEnabled, runCreationEnabled,
  sessionMessageEnabled, sessionSteeringControlEnabled, runLifecycleEnabled,
  runExecutionEnabled, planDeliveryControlEnabled, approvalControlEnabled,
  modelControlEnabled, providerCredentialEnabled, fileEditReviewEnabled,
  fileEditProposalEnabled, fileEditApplyEnabled, runWakeControlEnabled,
  runWakeExecutionEnabled, runWakeWorkerEnabled, skillInstallationEnabled,
  evidenceAttachmentEnabled, verificationEvidenceEnabled }: {
  token: string;
  controlToken: string;
  runControlEnabled: boolean;
  runCreationEnabled: boolean;
  sessionMessageEnabled: boolean;
  sessionSteeringControlEnabled: boolean;
  runLifecycleEnabled: boolean;
  runExecutionEnabled: boolean;
  planDeliveryControlEnabled: boolean;
  approvalControlEnabled: boolean;
  modelControlEnabled: boolean;
  providerCredentialEnabled: boolean;
  fileEditReviewEnabled: boolean;
  fileEditProposalEnabled: boolean;
  fileEditApplyEnabled: boolean;
  runWakeControlEnabled: boolean;
  runWakeExecutionEnabled: boolean;
  runWakeWorkerEnabled: boolean;
  skillInstallationEnabled: boolean;
  evidenceAttachmentEnabled: boolean;
  verificationEvidenceEnabled: boolean;
}) {
  const [surface, setSurface] = useState<"workspace" | "settings">("workspace");
  const [sidebarVisible, setSidebarVisible] = useState(true);
  const [skillPreviewOpen, setSkillPreviewOpen] = useState(false);
  const [runCreationOpen, setRunCreationOpen] = useState(false);
  const [runDraft, setRunDraft] = useState<Partial<NewRunDraft>>({});
  const [modelsOpen, setModelsOpen] = useState(false);
  const [sidebarWidth, setSidebarWidth] = useState(readSidebarWidth);
  const [workspaceSection, setWorkspaceSection] = useState<Exclude<WorkbenchSection, "new-task">>(
    "conversation");
  const desktop = desktopBridgeAvailable();
  const client = useMemo(() => new CyberAgentClient(token, undefined, controlToken, {
    runControlEnabled, runCreationEnabled, sessionMessageEnabled,
    sessionSteeringControlEnabled,
    runLifecycleEnabled, runExecutionEnabled,
    planDeliveryControlEnabled, approvalControlEnabled, modelControlEnabled,
    providerCredentialEnabled, fileEditReviewEnabled, fileEditProposalEnabled,
    fileEditApplyEnabled, runWakeControlEnabled, runWakeExecutionEnabled,
    runWakeWorkerEnabled, skillInstallationEnabled, evidenceAttachmentEnabled,
    verificationEvidenceEnabled,
  }), [token, controlToken, runControlEnabled, runCreationEnabled, sessionMessageEnabled,
    sessionSteeringControlEnabled, runLifecycleEnabled, runExecutionEnabled,
    planDeliveryControlEnabled, approvalControlEnabled, modelControlEnabled,
    providerCredentialEnabled, fileEditReviewEnabled, fileEditProposalEnabled,
    fileEditApplyEnabled, runWakeControlEnabled, runWakeExecutionEnabled,
    runWakeWorkerEnabled, skillInstallationEnabled, evidenceAttachmentEnabled,
    verificationEvidenceEnabled]);
  const queryClient = useQueryClient();
  const health = useConnectionStore((state) => state.health);
  const setHealth = useConnectionStore((state) => state.setHealth);
  const disconnect = useConnectionStore((state) => state.disconnect);
  const resourceKind = useConnectionStore((state) => state.resourceKind);
  const selectedRunID = useConnectionStore((state) => state.selectedRunID);
  const selectedSessionID = useConnectionStore((state) => state.selectedSessionID);
  const healthQuery = useQuery({
    queryKey: ["health"],
    queryFn: ({ signal }) => client.health(signal),
    initialData: health ?? undefined,
    refetchInterval: 30_000,
  });
  const settingsCapabilities = useMemo<SettingsCapability[]>(() => [
    { id: "run-control", label: "执行档位", enabled: runControlEnabled },
    { id: "run-creation", label: "创建任务", enabled: runCreationEnabled },
    { id: "session-message", label: "会话消息", enabled: sessionMessageEnabled },
    { id: "steering", label: "队列引导", enabled: sessionSteeringControlEnabled },
    { id: "lifecycle", label: "Run 生命周期", enabled: runLifecycleEnabled },
    { id: "execution", label: "有界执行", enabled: runExecutionEnabled },
    { id: "plan-delivery", label: "计划交付", enabled: planDeliveryControlEnabled },
    { id: "approval", label: "审批", enabled: approvalControlEnabled },
    { id: "model", label: "模型配置", enabled: modelControlEnabled },
    { id: "credentials", label: "系统凭证", enabled: providerCredentialEnabled },
    { id: "edit-review", label: "编辑审阅", enabled: fileEditReviewEnabled },
    { id: "edit-proposal", label: "编辑提案", enabled: fileEditProposalEnabled },
    { id: "edit-apply", label: "编辑应用", enabled: fileEditApplyEnabled },
    { id: "wake", label: "Wake 队列", enabled: runWakeControlEnabled },
    { id: "wake-execution", label: "Wake 执行", enabled: runWakeExecutionEnabled },
    { id: "wake-worker", label: "Wake Worker", enabled: runWakeWorkerEnabled },
    { id: "skill-install", label: "Skill 安装", enabled: skillInstallationEnabled },
    { id: "evidence", label: "证据挂载", enabled: evidenceAttachmentEnabled },
    { id: "verification", label: "验证证据", enabled: verificationEvidenceEnabled },
  ], [approvalControlEnabled, evidenceAttachmentEnabled, fileEditApplyEnabled,
    fileEditProposalEnabled, fileEditReviewEnabled, modelControlEnabled,
    planDeliveryControlEnabled, providerCredentialEnabled, runControlEnabled,
    runCreationEnabled, runExecutionEnabled, runLifecycleEnabled,
    runWakeControlEnabled, runWakeExecutionEnabled, runWakeWorkerEnabled,
    sessionMessageEnabled, sessionSteeringControlEnabled, skillInstallationEnabled,
    verificationEvidenceEnabled]);

  useEffect(() => {
    if (healthQuery.data) {
      setHealth(healthQuery.data);
    }
  }, [healthQuery.data, setHealth]);

  const leave = () => {
    setSurface("workspace");
    setWorkspaceSection("conversation");
    setSkillPreviewOpen(false);
    setRunCreationOpen(false);
    setModelsOpen(false);
    queryClient.clear();
    disconnect();
  };

  const openRunCreation = (draft: Partial<NewRunDraft> = {}) => {
    setSurface("workspace");
    setWorkspaceSection("conversation");
    setRunDraft(draft);
    setRunCreationOpen(true);
  };

  const resizeSidebar = (value: number) => {
    const normalized = clampSidebarWidth(value);
    setSidebarWidth(normalized);
    try {
      localStorage.setItem(sidebarWidthStorageKey, String(normalized));
    } catch {
      // Window geometry remains usable when browser storage is unavailable.
    }
  };

  const navigateWorkspace = (section: WorkbenchSection) => {
    setSurface("workspace");
    if (section === "new-task") {
      openRunCreation();
      return;
    }
    setWorkspaceSection(section);
    if (section === "plugins" && desktop) setSkillPreviewOpen(true);
  };

  const selectedResourceID = resourceKind === "run" ? selectedRunID : selectedSessionID;
  const panelTitle = workspaceSection === "conversation"
    ? selectedResourceID
      ? `${resourceKind === "run" ? "任务" : "对话"} / ${selectedResourceID.slice(0, 18)}`
      : "Prayu 工作台"
    : workspaceSection === "pull-requests" ? "拉取请求"
      : workspaceSection === "models" ? "模型切换"
        : workspaceSection === "schedule" ? "自动定时" : "插件";

  const workspaceContent = workspaceSection === "models"
    ? <ModelAvailabilityWorkspace client={client} />
    : workspaceSection === "pull-requests" || workspaceSection === "schedule" ||
      workspaceSection === "plugins"
      ? <UtilityWorkspace kind={workspaceSection}
        onOpenPlugins={desktop ? () => setSkillPreviewOpen(true) : undefined} />
      : selectedResourceID
        ? resourceKind === "run"
          ? <RunWorkspace client={client} onOpenPlugins={desktop ? () => setSkillPreviewOpen(true) : undefined}
            runID={selectedRunID} />
          : <SessionWorkspace client={client} onOpenPlugins={desktop ? () => setSkillPreviewOpen(true) : undefined}
            sessionID={selectedSessionID} />
        : <EmptyConversation client={client} creationEnabled={runCreationEnabled}
          onCreateRun={openRunCreation}
          onOpenPlugins={desktop ? () => setSkillPreviewOpen(true) : undefined} />;

  return (
    <>
      <div className={`app-shell prayu-shell ${surface === "settings" ? "settings-mode" : "workspace-mode"}`}>
        <header className="topbar prayu-titlebar">
          <div className="titlebar-navigation">
            <button aria-label="显示或隐藏侧栏" className="titlebar-icon" disabled={surface === "settings"}
              onClick={() => setSidebarVisible((visible) => !visible)} title="显示或隐藏侧栏" type="button">
              <PanelLeft aria-hidden="true" size={16} />
            </button>
            <button aria-label="返回工作台" className="titlebar-icon" disabled={surface === "workspace"}
              onClick={() => setSurface("workspace")} title="返回工作台" type="button">
              <ArrowLeft aria-hidden="true" size={16} />
            </button>
            <button aria-label="前进" className="titlebar-icon" disabled title="前进" type="button">
              <ArrowRight aria-hidden="true" size={16} />
            </button>
            <nav aria-label="应用菜单" className="titlebar-menu">
              <button disabled={!runCreationEnabled} onClick={() => openRunCreation()}
                title="新建任务" type="button">文件</button>
              <button onClick={() => navigateWorkspace("models")} title="模型与 Provider"
                type="button">编辑</button>
              <button disabled={surface === "settings"}
                onClick={() => setSidebarVisible((visible) => !visible)} title="切换侧栏" type="button">视图</button>
              <button onClick={() => setSurface("settings")} title="设置与关于" type="button">帮助</button>
            </nav>
          </div>
          <div className="topbar-actions">
            <span className={`health-indicator ${healthQuery.isError ? "offline" : "online"}`}>
              <i />{healthQuery.isError ? "API error" : `api.v1 / schema ${healthQuery.data?.schema_version ?? "-"}`}
            </span>
            <button aria-label="刷新" className="icon-button" disabled={healthQuery.isFetching} onClick={() => void healthQuery.refetch()} title="刷新" type="button">
              <RefreshCw aria-hidden="true" className={healthQuery.isFetching ? "spin" : ""} size={16} />
            </button>
            <button aria-label="设置" className="icon-button" onClick={() => setSurface("settings")} title="设置" type="button">
              <Settings aria-hidden="true" size={16} />
            </button>
            {!desktop && <button aria-label="断开连接" className="icon-button" onClick={leave} title="断开连接" type="button"><LogOut aria-hidden="true" size={16} /></button>}
            {desktop && <div aria-label="窗口控制" className="desktop-window-controls" role="group">
              <button aria-label="最小化" onClick={minimiseDesktopWindow} title="最小化" type="button">
                <Minus aria-hidden="true" size={15} />
              </button>
              <button aria-label="最大化或还原" onClick={toggleDesktopWindowMaximised}
                title="最大化或还原" type="button">
                <Square aria-hidden="true" size={12} />
              </button>
              <button aria-label="关闭" className="desktop-window-close" onClick={closeDesktopWindow}
                title="关闭" type="button">
                <X aria-hidden="true" size={16} />
              </button>
            </div>}
          </div>
        </header>
        {surface === "workspace" ? <div className={`shell-body ${sidebarVisible ? "" : "sidebar-hidden"}`}
          style={{ "--prayu-sidebar-width": `${sidebarWidth}px` } as CSSProperties}>
          {sidebarVisible && <ResourceSidebar client={client}
            activeSection={runCreationOpen ? "new-task" : workspaceSection}
            onCreateRun={runCreationEnabled ? openRunCreation : undefined}
            onNavigate={navigateWorkspace}
            onOpenSettings={() => setSurface("settings")} />}
          {sidebarVisible && <SidebarResizeHandle onChange={resizeSidebar} value={sidebarWidth} />}
          <WorkbenchFrame client={client} desktop={desktop} resourceKind={resourceKind}
            runID={selectedRunID} sessionID={selectedSessionID}
            title={panelTitle}>
            {workspaceContent}
          </WorkbenchFrame>
        </div> : <SettingsView capabilities={settingsCapabilities} desktop={desktop}
          health={healthQuery.data ?? health ?? null} onBack={() => setSurface("workspace")}
          onOpenModels={() => setModelsOpen(true)}
          onOpenSkills={() => setSkillPreviewOpen(true)} />}
      </div>
      <DesktopSkillPreviewDialog installationEnabled={skillInstallationEnabled}
        open={skillPreviewOpen} onClose={() => setSkillPreviewOpen(false)} />
      <ModelAvailabilityDialog client={client} open={modelsOpen}
        onClose={() => setModelsOpen(false)} />
      <RunCreationDialog client={client} open={runCreationOpen}
        initialGoal={runDraft.goal} initialPhase={runDraft.phase}
        onClose={() => setRunCreationOpen(false)} />
    </>
  );
}

function readSidebarWidth(): number {
  try {
    const stored = Number(localStorage.getItem(sidebarWidthStorageKey));
    return Number.isFinite(stored) && stored > 0
      ? clampSidebarWidth(stored) : defaultSidebarWidth;
  } catch {
    return defaultSidebarWidth;
  }
}
