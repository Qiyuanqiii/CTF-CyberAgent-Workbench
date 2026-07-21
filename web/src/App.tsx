import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  ArrowRight,
  Cpu,
  LogOut,
  PackageSearch,
  PanelLeft,
  Plus,
  RefreshCw,
  Settings,
} from "lucide-react";
import { CyberAgentClient } from "./api/client";
import { ConnectionGate } from "./components/connection-gate";
import { DesktopSkillPreviewDialog } from "./components/desktop-skill-preview";
import { ModelAvailabilityDialog } from "./components/model-availability-dialog";
import { RunCreationDialog } from "./components/run-creation-dialog";
import { ResourceSidebar } from "./components/resource-sidebar";
import { RunWorkspace } from "./components/run-workspace";
import { SessionWorkspace } from "./components/session-workspace";
import { SettingsView, type SettingsCapability } from "./components/settings-view";
import { desktopBridgeAvailable } from "./lib/desktop-bridge";
import { useConnectionStore } from "./state/connection";

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
  const [modelsOpen, setModelsOpen] = useState(false);
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
    setSkillPreviewOpen(false);
    setRunCreationOpen(false);
    setModelsOpen(false);
    queryClient.clear();
    disconnect();
  };

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
              <button disabled={!runCreationEnabled} onClick={() => setRunCreationOpen(true)}
                title="新建任务" type="button">文件</button>
              <button onClick={() => setModelsOpen(true)} title="模型与 Provider" type="button">编辑</button>
              <button disabled={surface === "settings"}
                onClick={() => setSidebarVisible((visible) => !visible)} title="切换侧栏" type="button">视图</button>
              <button onClick={() => setSurface("settings")} title="设置与关于" type="button">帮助</button>
            </nav>
          </div>
          <div className="titlebar-product"><span aria-hidden="true">P</span><strong>Prayu</strong></div>
          <div className="topbar-actions">
            <span className={`health-indicator ${healthQuery.isError ? "offline" : "online"}`}>
              <i />{healthQuery.isError ? "API error" : `api.v1 / schema ${healthQuery.data?.schema_version ?? "-"}`}
            </span>
            {runCreationEnabled &&
              <button aria-label="Create Run" className="icon-button" onClick={() => setRunCreationOpen(true)} title="Create Run" type="button">
                <Plus aria-hidden="true" size={17} />
              </button>}
            <button aria-label="Model availability" className="icon-button"
              onClick={() => setModelsOpen(true)} title="Models" type="button">
              <Cpu aria-hidden="true" size={16} />
            </button>
            {desktop &&
              <button aria-label="预览 Skill 包" className="icon-button" onClick={() => setSkillPreviewOpen(true)} title="预览 Skill 包" type="button">
                <PackageSearch aria-hidden="true" size={16} />
              </button>}
            <button aria-label="刷新" className="icon-button" disabled={healthQuery.isFetching} onClick={() => void healthQuery.refetch()} title="刷新" type="button">
              <RefreshCw aria-hidden="true" className={healthQuery.isFetching ? "spin" : ""} size={16} />
            </button>
            <button aria-label="设置" className="icon-button" onClick={() => setSurface("settings")} title="设置" type="button">
              <Settings aria-hidden="true" size={16} />
            </button>
            {!desktop && <button aria-label="断开连接" className="icon-button" onClick={leave} title="断开连接" type="button"><LogOut aria-hidden="true" size={16} /></button>}
          </div>
        </header>
        {surface === "workspace" ? <div className={`shell-body ${sidebarVisible ? "" : "sidebar-hidden"}`}>
          {sidebarVisible && <ResourceSidebar client={client}
            onCreateRun={runCreationEnabled ? () => setRunCreationOpen(true) : undefined}
            onOpenSettings={() => setSurface("settings")} />}
          <main className="main-workspace">
            {resourceKind === "run" ? <RunWorkspace client={client} runID={selectedRunID} /> : <SessionWorkspace client={client} sessionID={selectedSessionID} />}
          </main>
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
        onClose={() => setRunCreationOpen(false)} />
    </>
  );
}
