import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Cpu, LogOut, PackageSearch, Plus, RefreshCw, ShieldCheck } from "lucide-react";
import { CyberAgentClient } from "./api/client";
import { ConnectionGate } from "./components/connection-gate";
import { DesktopSkillPreviewDialog } from "./components/desktop-skill-preview";
import { ModelAvailabilityDialog } from "./components/model-availability-dialog";
import { RunCreationDialog } from "./components/run-creation-dialog";
import { ResourceSidebar } from "./components/resource-sidebar";
import { RunWorkspace } from "./components/run-workspace";
import { SessionWorkspace } from "./components/session-workspace";
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
    evidenceAttachmentEnabled={evidenceAttachmentEnabled} />;
}

function ConnectedWorkbench({ token, controlToken, runControlEnabled, runCreationEnabled,
  sessionMessageEnabled, sessionSteeringControlEnabled, runLifecycleEnabled,
  runExecutionEnabled, planDeliveryControlEnabled, approvalControlEnabled,
  modelControlEnabled, providerCredentialEnabled, fileEditReviewEnabled,
  fileEditProposalEnabled, fileEditApplyEnabled, runWakeControlEnabled,
  runWakeExecutionEnabled, runWakeWorkerEnabled, skillInstallationEnabled,
  evidenceAttachmentEnabled }: {
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
}) {
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
  }), [token, controlToken, runControlEnabled, runCreationEnabled, sessionMessageEnabled,
    sessionSteeringControlEnabled, runLifecycleEnabled, runExecutionEnabled,
    planDeliveryControlEnabled, approvalControlEnabled, modelControlEnabled,
    providerCredentialEnabled, fileEditReviewEnabled, fileEditProposalEnabled,
    fileEditApplyEnabled, runWakeControlEnabled, runWakeExecutionEnabled,
    runWakeWorkerEnabled, skillInstallationEnabled, evidenceAttachmentEnabled]);
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

  useEffect(() => {
    if (healthQuery.data) {
      setHealth(healthQuery.data);
    }
  }, [healthQuery.data, setHealth]);

  const leave = () => {
    setSkillPreviewOpen(false);
    setRunCreationOpen(false);
    setModelsOpen(false);
    queryClient.clear();
    disconnect();
  };

  return (
    <>
      <div className="app-shell">
        <header className="topbar">
          <div className="brand-lockup compact">
            <span className="brand-mark"><ShieldCheck aria-hidden="true" size={20} /></span>
            <span><strong>CyberAgent Workbench</strong><small>Control console</small></span>
          </div>
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
            {!desktop && <button aria-label="断开连接" className="icon-button" onClick={leave} title="断开连接" type="button"><LogOut aria-hidden="true" size={16} /></button>}
          </div>
        </header>
        <div className="shell-body">
          <ResourceSidebar client={client} />
          <main className="main-workspace">
            {resourceKind === "run" ? <RunWorkspace client={client} runID={selectedRunID} /> : <SessionWorkspace client={client} sessionID={selectedSessionID} />}
          </main>
        </div>
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
