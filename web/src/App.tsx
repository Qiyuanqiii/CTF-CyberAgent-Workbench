import { useEffect, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { LogOut, RefreshCw, ShieldCheck } from "lucide-react";
import { CyberAgentClient } from "./api/client";
import { ConnectionGate } from "./components/connection-gate";
import { ResourceSidebar } from "./components/resource-sidebar";
import { RunWorkspace } from "./components/run-workspace";
import { SessionWorkspace } from "./components/session-workspace";
import { useConnectionStore } from "./state/connection";

export default function App() {
  const token = useConnectionStore((state) => state.token);
  if (!token) {
    return <ConnectionGate />;
  }
  return <ConnectedWorkbench token={token} />;
}

function ConnectedWorkbench({ token }: { token: string }) {
  const client = useMemo(() => new CyberAgentClient(token), [token]);
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
    queryClient.clear();
    disconnect();
  };

  return (
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
          <button aria-label="刷新" className="icon-button" disabled={healthQuery.isFetching} onClick={() => void healthQuery.refetch()} title="刷新" type="button">
            <RefreshCw aria-hidden="true" className={healthQuery.isFetching ? "spin" : ""} size={16} />
          </button>
          <button aria-label="断开连接" className="icon-button" onClick={leave} title="断开连接" type="button"><LogOut aria-hidden="true" size={16} /></button>
        </div>
      </header>
      <div className="shell-body">
        <ResourceSidebar client={client} />
        <main className="main-workspace">
          {resourceKind === "run" ? <RunWorkspace client={client} runID={selectedRunID} /> : <SessionWorkspace client={client} sessionID={selectedSessionID} />}
        </main>
      </div>
    </div>
  );
}
