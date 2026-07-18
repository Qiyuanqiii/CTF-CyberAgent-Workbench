import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  Boxes,
  Check,
  ClipboardList,
  Container,
  Database,
  FileArchive,
  FileDiff,
  FolderOpen,
  Gauge,
  GitBranch,
  History,
  ListChecks,
  ListOrdered,
  LoaderCircle,
  Network,
  Pause,
  Play,
  Radio,
  ScanSearch,
  ShieldAlert,
  ShieldCheck,
  StickyNote,
  Terminal,
  ChevronsRight,
  View,
  Wrench,
} from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type {
  ArtifactView,
  EventView,
  NoteView,
  OperatorSteeringQueueView,
  PlanDeliveryStateView,
  RunDetailView,
  RunExecutionProfileControlView,
  RunExecutionProfileView,
  RunExecutionControlView,
  RunLifecycleControlRequestView,
  RunLifecycleControlView,
  SupervisorToolRoundView,
  WorkItemView,
} from "../api/types";
import { usePagedResource } from "../hooks/use-paged-resource";
import { useRunEventStream } from "../hooks/use-run-event-stream";
import { formatBytes, formatDate, formatNumber, shortID } from "../lib/format";
import { EmptyState, ErrorState, KeyValue, LoadMoreButton, LoadingState, StatusBadge } from "./common";
import { ApprovalPanel } from "./approval-panel";
import { FileEditPanel } from "./file-edit-panel";
import { OperationReceiptHistory } from "./operation-receipt-history";
import { RunWakePanel } from "./run-wake-panel";
import { WorkspaceExplorer } from "./workspace-explorer";
import { AgentGraphPanel, DelegationsPanel, ExternalSkillsPanel, FanoutPanel, FindingsPanel } from "./run-projections";

type RunTab = "overview" | "approvals" | "diffs" | "files" | "receipts" | "agents" | "delegations" | "fanout" | "findings" | "events" | "work" | "notes" | "artifacts" | "tools";

const tabs: Array<{ id: RunTab; label: string; icon: typeof Activity }> = [
  { id: "overview", label: "概览", icon: Gauge },
  { id: "approvals", label: "Approvals", icon: ShieldCheck },
  { id: "diffs", label: "Diffs", icon: FileDiff },
  { id: "files", label: "Files", icon: FolderOpen },
  { id: "receipts", label: "Receipts", icon: History },
  { id: "agents", label: "Agents", icon: GitBranch },
  { id: "delegations", label: "委派", icon: Network },
  { id: "fanout", label: "Fan-out", icon: ScanSearch },
  { id: "findings", label: "发现", icon: ShieldAlert },
  { id: "events", label: "事件", icon: Activity },
  { id: "work", label: "任务", icon: ClipboardList },
  { id: "notes", label: "记忆", icon: StickyNote },
  { id: "artifacts", label: "产物", icon: FileArchive },
  { id: "tools", label: "工具", icon: Wrench },
];

export function RunWorkspace({ client, runID }: { client: CyberAgentClient; runID: string }) {
  const [tab, setTab] = useState<RunTab>("overview");
  const detailQuery = useQuery({
    queryKey: ["run", runID],
    queryFn: ({ signal }) => client.get<RunDetailView>(`/runs/${encodeURIComponent(runID)}`, {}, signal),
    enabled: Boolean(runID),
  });
  const eventsQuery = usePagedResource<EventView>(client, ["run", runID, "events"],
    `/runs/${encodeURIComponent(runID)}/events`, { limit: 100 }, Boolean(runID) && tab === "events");
  const workQuery = usePagedResource<WorkItemView>(client, ["run", runID, "work"],
    `/runs/${encodeURIComponent(runID)}/work-items`, { limit: 100 }, Boolean(runID) && tab === "work");
  const notesQuery = usePagedResource<NoteView>(client, ["run", runID, "notes"],
    `/runs/${encodeURIComponent(runID)}/notes`, { limit: 100 }, Boolean(runID) && tab === "notes");
  const artifactsQuery = usePagedResource<ArtifactView>(client, ["run", runID, "artifacts"],
    `/runs/${encodeURIComponent(runID)}/artifacts`, { limit: 100 }, Boolean(runID) && tab === "artifacts");
  const toolsQuery = usePagedResource<SupervisorToolRoundView>(client, ["run", runID, "tools"],
    `/runs/${encodeURIComponent(runID)}/tool-rounds`, { limit: 100 }, Boolean(runID) && tab === "tools");
  const stream = useRunEventStream(client, runID);

  const events = useMemo(() => {
    const bySequence = new Map<number, EventView>();
    for (const page of eventsQuery.data?.pages ?? []) {
      for (const event of page.items) {
        bySequence.set(event.sequence, event);
      }
    }
    for (const frame of stream.frames) {
      bySequence.set(frame.event.sequence, frame.event);
    }
    return [...bySequence.values()].sort((left, right) => right.sequence - left.sequence);
  }, [eventsQuery.data, stream.frames]);
  const work = useMemo(() => workQuery.data?.pages.flatMap((page) => page.items) ?? [], [workQuery.data]);
  const notes = useMemo(() => notesQuery.data?.pages.flatMap((page) => page.items) ?? [], [notesQuery.data]);
  const artifacts = useMemo(() => artifactsQuery.data?.pages.flatMap((page) => page.items) ?? [], [artifactsQuery.data]);
  const rounds = useMemo(() => toolsQuery.data?.pages.flatMap((page) => page.items) ?? [], [toolsQuery.data]);

  if (!runID) {
    return <EmptyWorkspace icon={<Boxes aria-hidden="true" size={24} />} title="选择一个 Run" />;
  }
  if (detailQuery.isLoading) {
    return <LoadingState label="加载 Run" />;
  }
  if (detailQuery.isError || !detailQuery.data) {
    return <ErrorState error={detailQuery.error} />;
  }
  const detail = detailQuery.data;

  return (
    <div className="workspace-view">
      <header className="workspace-header">
        <div>
          <div className="workspace-kicker">Run {shortID(detail.run.id)}</div>
          <h1>{detail.mission.goal}</h1>
          <div className="header-meta">
            <StatusBadge status={detail.run.status} />
            <span>{detail.mission.profile}</span>
            <span>{detail.run.config.model_route}</span>
          </div>
        </div>
        <div className={`stream-indicator stream-${stream.status}`} title={stream.error || stream.status}>
          <Radio aria-hidden="true" size={15} />
          {stream.status}
        </div>
      </header>
      <nav aria-label="Run 视图" className="workspace-tabs" role="tablist">
        {tabs.map(({ id, label, icon: Icon }) => (
          <button aria-selected={tab === id} className={tab === id ? "active" : ""} key={id} onClick={() => setTab(id)} role="tab" type="button">
            <Icon aria-hidden="true" size={15} />{label}
          </button>
        ))}
      </nav>
      <div className="workspace-content">
        {tab === "overview" && <RunOverview client={client} detail={detail} />}
        {tab === "approvals" && <ApprovalPanel client={client} runID={runID} />}
        {tab === "diffs" && <FileEditPanel client={client} runID={runID} />}
        {tab === "files" && <WorkspaceExplorer client={client}
          key={detail.mission.workspace_id ?? "unbound"}
          runID={runID} workspaceID={detail.mission.workspace_id ?? ""} />}
        {tab === "receipts" && <OperationReceiptHistory client={client} runID={runID} />}
        {tab === "agents" && <AgentGraphPanel client={client} runID={runID} />}
        {tab === "delegations" && <DelegationsPanel client={client} runID={runID} />}
        {tab === "fanout" && <FanoutPanel client={client} runID={runID} />}
        {tab === "findings" && <FindingsPanel client={client} runID={runID} />}
        {tab === "events" && (
          <CollectionState query={eventsQuery} empty="暂无事件">
            {stream.error && <div className="inline-warning">SSE: {stream.error}</div>}
            <EventList events={events} />
            <LoadMoreButton hasNextPage={Boolean(eventsQuery.hasNextPage)} isFetching={eventsQuery.isFetchingNextPage} onClick={() => void eventsQuery.fetchNextPage()} />
          </CollectionState>
        )}
        {tab === "work" && (
          <CollectionState query={workQuery} empty="暂无任务">
            <WorkTable items={work} />
            <LoadMoreButton hasNextPage={Boolean(workQuery.hasNextPage)} isFetching={workQuery.isFetchingNextPage} onClick={() => void workQuery.fetchNextPage()} />
          </CollectionState>
        )}
        {tab === "notes" && (
          <CollectionState query={notesQuery} empty="暂无记忆">
            <NoteList notes={notes} />
            <LoadMoreButton hasNextPage={Boolean(notesQuery.hasNextPage)} isFetching={notesQuery.isFetchingNextPage} onClick={() => void notesQuery.fetchNextPage()} />
          </CollectionState>
        )}
        {tab === "artifacts" && (
          <CollectionState query={artifactsQuery} empty="暂无产物">
            <ArtifactTable artifacts={artifacts} />
            <LoadMoreButton hasNextPage={Boolean(artifactsQuery.hasNextPage)} isFetching={artifactsQuery.isFetchingNextPage} onClick={() => void artifactsQuery.fetchNextPage()} />
          </CollectionState>
        )}
        {tab === "tools" && (
          <CollectionState query={toolsQuery} empty="暂无工具轮次">
            <ToolRounds rounds={rounds} />
            <LoadMoreButton hasNextPage={Boolean(toolsQuery.hasNextPage)} isFetching={toolsQuery.isFetchingNextPage} onClick={() => void toolsQuery.fetchNextPage()} />
          </CollectionState>
        )}
      </div>
    </div>
  );
}

function RunOverview({ client, detail }: { client: CyberAgentClient; detail: RunDetailView }) {
  const checkpoint = detail.checkpoint;
  const usage = detail.tool_usage;
  const percent = usage.limit > 0 ? Math.min(100, Math.round((usage.consumed / usage.limit) * 100)) : 0;
  const steering = detail.operator_steering;
  return (
    <div className="overview-layout">
      <section className="metric-strip" aria-label="Run 指标">
        <div><span>Next turn</span><strong>{checkpoint?.next_turn ?? 0}</strong></div>
        <div><span>Total tokens</span><strong>{formatNumber(checkpoint?.total_tokens)}</strong></div>
        <div><span>Tool calls</span><strong>{formatNumber(usage.consumed)} / {formatNumber(usage.limit)}</strong></div>
        <div><span>Execution</span><strong>{formatNumber(checkpoint?.execution_millis)} ms</strong></div>
      </section>
      <section className="detail-section">
        <h2>目标与范围</h2>
        <dl className="detail-grid">
          <KeyValue label="Mission" value={detail.mission.id} />
          <KeyValue label="Workspace" value={detail.mission.workspace_id} />
          <KeyValue label="Surface" value={detail.mode.surface} />
          <KeyValue label="Execution phase" value={detail.mode.phase} />
          <KeyValue label="Mode revision" value={formatNumber(detail.mode.revision)} />
          <KeyValue label="Network" value={detail.mission.scope.network_mode} />
          <KeyValue label="Allowed targets" value={detail.mission.scope.allowed_targets?.join(", ")} />
          <KeyValue label="Interactive" value={detail.run.config.interactive ? "yes" : "no"} />
          <KeyValue label="Created" value={formatDate(detail.run.created_at)} />
        </dl>
      </section>
      <RunControlPanel client={client} detail={detail} />
      <RunWakePanel client={client} detail={detail} />
      <ExecutionProfilePanel client={client} detail={detail} />
      <section className="detail-section">
        <h2>执行状态</h2>
        <dl className="detail-grid">
          <KeyValue label="Supervisor phase" value={checkpoint?.phase} />
          <KeyValue label="Attempt" value={checkpoint?.attempt_id} />
          <KeyValue label="Repair phase" value={checkpoint?.repair_phase} />
          <KeyValue label="Last error" value={checkpoint?.last_error} />
          <KeyValue label="Lease owner" value={detail.execution_lease?.owner_id} />
          <KeyValue label="Lease" value={detail.execution_lease ? <StatusBadge status={detail.execution_lease.active ? "active" : detail.execution_lease.status} /> : "-"} />
        </dl>
      </section>
      <section className="detail-section">
        <div className="section-heading"><h2>工具预算</h2><span>{percent}%</span></div>
        <progress aria-label="工具预算使用率" className="budget-track" max={100} value={percent}>{percent}%</progress>
        <dl className="detail-grid compact">
          <KeyValue label="Consumed" value={formatNumber(usage.consumed)} />
          <KeyValue label="Remaining" value={formatNumber(usage.remaining)} />
          <KeyValue label="Exhausted" value={formatDate(usage.exhausted_at)} />
        </dl>
      </section>
      <OperatorSteeringPanel state={steering} />
      {detail.plan_delivery && <PlanDeliveryPanel client={client} detail={detail}
        state={detail.plan_delivery} />}
      {detail.external_skills && <ExternalSkillsPanel projection={detail.external_skills} />}
    </div>
  );
}

export function RunControlPanel({ client, detail }: {
  client: CyberAgentClient;
  detail: RunDetailView;
}) {
  const queryClient = useQueryClient();
  const [maxSteps, setMaxSteps] = useState(1);
  const [lastExecution, setLastExecution] = useState<RunExecutionControlView | null>(null);
  const operationKeys = useRef(new Map<string, string>());
  const operationKey = (kind: string) => {
    const existing = operationKeys.current.get(kind);
    if (existing) {
      return existing;
    }
    const created = `web-run-${kind}-${globalThis.crypto.randomUUID()}`;
    operationKeys.current.set(kind, created);
    return created;
  };
  const lifecycle = useMutation({
    mutationFn: (action: RunLifecycleControlRequestView["action"]) =>
      client.controlRunLifecycle(detail.run.id, {
        version: "run_lifecycle_control.v1", action,
      }, operationKey(`lifecycle-${action}`)),
    onSuccess: (result: RunLifecycleControlView, action) => {
      operationKeys.current.delete(`lifecycle-${action}`);
      queryClient.setQueryData<RunDetailView>(["run", detail.run.id], (current) => current
        ? { ...current, run: result.run }
        : current);
      void queryClient.invalidateQueries({ queryKey: ["run", detail.run.id] });
      void queryClient.invalidateQueries({ queryKey: ["run", detail.run.id, "events"] });
    },
  });
  const execution = useMutation({
    mutationFn: () => client.executeRun(detail.run.id, {
      version: "run_execution_handoff.v1", max_steps: maxSteps,
    }, operationKey(`execute-${maxSteps}`)),
    onSuccess: (result) => {
      operationKeys.current.delete(`execute-${result.max_steps}`);
      setLastExecution(result);
      void queryClient.invalidateQueries({ queryKey: ["run", detail.run.id] });
      void queryClient.invalidateQueries({ queryKey: ["run", detail.run.id, "events"] });
    },
  });
  if (!client.hasRunLifecycle && !client.hasRunExecution) {
    return null;
  }
  const activeLease = Boolean(detail.execution_lease?.active);
  const queued = detail.operator_steering.pending + detail.operator_steering.prepared;
  const lifecycleAction: RunLifecycleControlRequestView["action"] | null =
    detail.run.status === "created" ? "start" :
      detail.run.status === "running" ? "pause" :
        detail.run.status === "paused" ? "resume" : null;
  const lifecycleDisabled = lifecycle.isPending || execution.isPending || activeLease ||
    lifecycleAction === null;
  const executionDisabled = execution.isPending || lifecycle.isPending || activeLease ||
    detail.run.status !== "running" || queued === 0;
  const LifecycleIcon = lifecycleAction === "pause" ? Pause : Play;
  const error = lifecycle.error ?? execution.error;
  return (
    <section className="detail-section run-control-section">
      <div className="section-heading">
        <h2><Play aria-hidden="true" size={15} />Run control</h2>
        <StatusBadge status={activeLease ? "busy" : detail.run.status} />
      </div>
      <div className="run-control-row">
        {client.hasRunLifecycle && lifecycleAction && (
          <button className="command-button" disabled={lifecycleDisabled}
            onClick={() => lifecycle.mutate(lifecycleAction)} type="button">
            {lifecycle.isPending
              ? <LoaderCircle aria-hidden="true" className="spin" size={16} />
              : <LifecycleIcon aria-hidden="true" size={16} />}
            {lifecycleAction === "start" ? "Start" : lifecycleAction === "pause" ? "Pause" : "Resume"}
          </button>
        )}
        {client.hasRunExecution && (
          <div className="run-execution-control">
            <label htmlFor={`run-max-steps-${detail.run.id}`}>Steps</label>
            <input id={`run-max-steps-${detail.run.id}`} max={8} min={1}
              onChange={(event) => setMaxSteps(Math.max(1, Math.min(8,
                Number.parseInt(event.target.value, 10) || 1)))} type="number" value={maxSteps} />
            <button className="command-button" disabled={executionDisabled}
              onClick={() => execution.mutate()} type="button">
              {execution.isPending
                ? <LoaderCircle aria-hidden="true" className="spin" size={16} />
                : <ChevronsRight aria-hidden="true" size={16} />}
              Run queue
            </button>
          </div>
        )}
      </div>
      {lastExecution && (
        <div className="run-control-result" role="status">
          <StatusBadge status={lastExecution.status} />
          <span>{lastExecution.stop_reason}</span>
          <span>{lastExecution.steps_completed}/{lastExecution.selected_count} steps</span>
        </div>
      )}
      {error && <div className="inline-warning" role="alert">
        {error instanceof Error ? error.message : "Run control failed"}
      </div>}
    </section>
  );
}

const executionProfiles: Array<{
  id: RunExecutionProfileView["profile"];
  label: string;
  detail: string;
  icon: typeof View;
}> = [
  { id: "preview", label: "Preview", detail: "No process", icon: View },
  { id: "docker", label: "Docker", detail: "Isolated gate", icon: Container },
  { id: "local", label: "Host workspace", detail: "OS sandbox gate", icon: Terminal },
];

export function ExecutionProfilePanel({ client, detail }: {
  client: CyberAgentClient;
  detail: RunDetailView;
}) {
  const queryClient = useQueryClient();
  const profile = detail.execution_profile;
  const mutableStatus = detail.run.status === "created" || detail.run.status === "paused";
  const mutable = client.hasControl && mutableStatus && !detail.execution_lease?.active;
  const mutation = useMutation({
    mutationFn: (target: RunExecutionProfileView["profile"]) => client.postControl<RunExecutionProfileControlView>(
      `/runs/${encodeURIComponent(detail.run.id)}/execution-profile`,
      { profile: target, reason: "web console execution profile selection" },
      `web-execution-profile-${globalThis.crypto.randomUUID()}`,
    ),
    onSuccess: (result) => {
      queryClient.setQueryData<RunDetailView>(["run", detail.run.id], (current) => current
        ? { ...current, execution_profile: result.execution_profile }
        : current);
      void queryClient.invalidateQueries({ queryKey: ["run", detail.run.id, "events"] });
    },
  });
  let boundary = "Selection is intent only";
  if (!client.hasControl) {
    boundary = "Read-only connection";
  } else if (!mutableStatus) {
    boundary = "Pause the Run before changing profile";
  } else if (detail.execution_lease?.active) {
    boundary = "Active execution lease";
  }
  return (
    <section className="detail-section execution-profile-section">
      <div className="section-heading">
        <h2><Container aria-hidden="true" size={15} />Execution environment / 执行环境</h2>
        <StatusBadge status={profile.risk_tier} />
      </div>
      <div aria-label="Run execution profile" className="execution-profile-segments" role="group">
        {executionProfiles.map(({ id, label, detail: optionDetail, icon: Icon }) => (
          <button
            aria-pressed={profile.profile === id}
            className={profile.profile === id ? "selected" : ""}
            disabled={!mutable || mutation.isPending || profile.profile === id}
            key={id}
            onClick={() => mutation.mutate(id)}
            title={`Select ${label}`}
            type="button"
          >
            <Icon aria-hidden="true" size={16} />
            <span><strong>{label}</strong><small>{optionDetail}</small></span>
          </button>
        ))}
      </div>
      <div className="execution-profile-boundary">
        <span>{boundary}</span>
        <span>Backend: {profile.backend}</span>
        <span>Approval: {profile.approval_policy}</span>
        <span>Gate: {profile.required_gate}</span>
        <span>Process enabled: no</span>
        <span>Execution authorized: no</span>
      </div>
      {mutation.isError && (
        <div className="inline-warning" role="alert">
          {mutation.error instanceof Error ? mutation.error.message : "Execution profile selection failed"}
        </div>
      )}
    </section>
  );
}

export function OperatorSteeringPanel({ state }: { state: OperatorSteeringQueueView }) {
  return (
    <section className="detail-section steering-section">
      <div className="section-heading">
        <h2><ListOrdered aria-hidden="true" size={15} />Operator steering</h2>
        <StatusBadge status={state.pending + state.prepared > 0 ? "pending" : "idle"} />
      </div>
      <div className="steering-state-line">
        <span>Queued {formatNumber(state.pending)}</span>
        <span>Prepared {formatNumber(state.prepared)}</span>
        <span>Committed {formatNumber(state.committed)}</span>
        <span>Cancelled {formatNumber(state.cancelled)}</span>
      </div>
      <div className="steering-list" aria-label="Operator steering metadata">
        {state.messages.length === 0 ? <p>No operator guidance recorded</p> :
          state.messages.map((message) => (
            <div className="steering-row" key={message.id}>
              <span>#{message.sequence}</span>
              <code>{shortID(message.id)}</code>
              <StatusBadge status={message.status} />
              <time dateTime={message.created_at}>{formatDate(message.created_at)}</time>
            </div>
          ))}
      </div>
    </section>
  );
}

export function PlanDeliveryPanel({ state, client, detail }: {
  state: PlanDeliveryStateView;
  client?: CyberAgentClient;
  detail?: RunDetailView;
}) {
  const queryClient = useQueryClient();
  const operationKeys = useRef(new Map<string, string>());
  const operationKey = (intent: string) => {
    const existing = operationKeys.current.get(intent);
    if (existing) {
      return existing;
    }
    const created = `web-plan-${globalThis.crypto.randomUUID()}`;
    operationKeys.current.set(intent, created);
    return created;
  };
  const refresh = () => {
    if (!detail) return;
    void queryClient.invalidateQueries({ queryKey: ["run", detail.run.id] });
    void queryClient.invalidateQueries({ queryKey: ["run", detail.run.id, "events"] });
    void queryClient.invalidateQueries({ queryKey: ["run", detail.run.id, "work"] });
    void queryClient.invalidateQueries({ queryKey: ["run", detail.run.id, "notes"] });
  };
  const directionMutation = useMutation({
    mutationFn: (direction: number) => {
      if (!client || !detail || !state.proposal) {
        throw new Error("Plan direction control is unavailable");
      }
      const intent = `${state.proposal.id}:direction:${direction}`;
      return client.selectPlanDirection(detail.run.id, {
        version: "plan_delivery_control.v1", proposal_id: state.proposal.id, direction,
      }, operationKey(intent)).then((result) => ({ result, intent }));
    },
    onSuccess: ({ intent }) => {
      operationKeys.current.delete(intent);
      refresh();
    },
  });
  const deliveryMutation = useMutation({
    mutationFn: () => {
      if (!client || !detail || !state.selection) {
        throw new Error("Plan delivery control is unavailable");
      }
      const intent = `${state.selection.id}:deliver`;
      return client.enterPlanDelivery(detail.run.id, {
        version: "plan_delivery_control.v1",
      }, operationKey(intent)).then((result) => ({ result, intent }));
    },
    onSuccess: ({ intent }) => {
      operationKeys.current.delete(intent);
      refresh();
    },
  });
  const selected = state.selection?.direction_ordinal;
  const mutable = Boolean(client?.hasPlanDelivery && detail &&
    (detail.run.status === "created" || detail.run.status === "paused") &&
    detail.mode.phase === "plan" && !detail.execution_lease?.active);
  const selecting = directionMutation.isPending || deliveryMutation.isPending;
  const controlError = directionMutation.error ?? deliveryMutation.error;
  const status = state.operator_choice_needed
    ? "Operator choice required"
    : state.phase_change_needed
      ? "Deliver phase required"
      : "Direction selected";
  return (
    <section className="detail-section plan-delivery-section">
      <div className="section-heading">
        <h2><ListChecks aria-hidden="true" size={15} />Plan / Delivery</h2>
        <StatusBadge status={state.operator_choice_needed ? "pending" : "accepted"} />
      </div>
      <div className="plan-state-line">
        <span>{status}</span>
        <span>Mode revision {formatNumber(state.proposal?.mode_revision)}</span>
        <span>Delivery gates {formatNumber(state.ready_checkpoints)} / {formatNumber(state.required_checkpoints)}</span>
        <span>Gate enforcement: {state.delivery_gate_enforced ? "on" : "legacy exempt"}</span>
        <span>Capability grant: no</span>
      </div>
      <div className="plan-direction-list">
        {state.proposal?.directions.map((direction) => (
          <details className={selected === direction.ordinal ? "plan-direction selected" : "plan-direction"}
            key={direction.ordinal} open={selected === direction.ordinal || undefined}>
            <summary>
              <span className="plan-ordinal">{direction.ordinal}</span>
              <span><strong>{direction.title}</strong><small>{direction.summary}</small></span>
              <span>{direction.modules.length} slices</span>
              {selected === direction.ordinal && <StatusBadge status="selected" />}
            </summary>
            <div className="plan-direction-body">
              <div><h3>Tradeoffs</h3><ul>{direction.tradeoffs.map((item) => <li key={item}>{item}</li>)}</ul></div>
              <div><h3>Delivery slices</h3><ol>{direction.modules.map((module) => (
                <li key={module.ordinal}>
                  <strong>{module.title}</strong>
                  <p>{module.objective}</p>
                  <small>{module.dependencies.length > 0 ? `Depends on ${module.dependencies.join(", ")}` : "No dependencies"}</small>
                </li>
              ))}</ol></div>
              {mutable && state.operator_choice_needed && state.proposal && (
                <button className="command-button plan-choice-button" disabled={selecting}
                  onClick={() => directionMutation.mutate(direction.ordinal)} type="button">
                  {directionMutation.isPending && directionMutation.variables === direction.ordinal
                    ? <LoaderCircle aria-hidden="true" className="spin" size={15} />
                    : <Check aria-hidden="true" size={15} />}Choose direction {direction.ordinal}
                </button>
              )}
            </div>
          </details>
        ))}
      </div>
      {mutable && state.selection && state.phase_change_needed && (
        <div className="plan-delivery-actions">
          <button className="command-button" disabled={selecting}
            onClick={() => deliveryMutation.mutate()} type="button">
            {deliveryMutation.isPending
              ? <LoaderCircle aria-hidden="true" className="spin" size={15} />
              : <ChevronsRight aria-hidden="true" size={15} />}Enter Deliver
          </button>
        </div>
      )}
      {(directionMutation.isError || deliveryMutation.isError) && (
        <div className="inline-warning" role="alert">
          {controlError instanceof Error
            ? controlError.message
            : "Plan/Delivery control failed"}
        </div>
      )}
      {state.selection && (
        <div className="delivery-checkpoint-list" aria-label="Delivery checkpoint history">
          <h3>Checkpoint history</h3>
          {state.checkpoints.length === 0 ? <p>No checkpoints recorded</p> : state.checkpoints.map((checkpoint) => (
            <div className="delivery-checkpoint-row" key={checkpoint.id}>
              <span>Slice {checkpoint.module_ordinal}/{checkpoint.module_count}</span>
              <code>{shortID(checkpoint.work_item_id)}</code>
              <span>mode r{checkpoint.mode_revision} / work v{checkpoint.work_item_version}</span>
              {checkpoint.full_gate_required && <span>full gate</span>}
              <StatusBadge status={checkpoint.gate_ready ? "ready" : "stale"} />
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function EventList({ events }: { events: EventView[] }) {
  if (events.length === 0) {
    return <EmptyState>暂无事件</EmptyState>;
  }
  return (
    <div className="event-list">
      {events.map((event) => (
        <details className="event-row" key={event.event_id}>
          <summary>
            <span className="event-sequence">#{event.sequence}</span>
            <strong>{event.type}</strong>
            <span>{event.source}</span>
            <time dateTime={event.created_at}>{formatDate(event.created_at)}</time>
          </summary>
          <pre>{JSON.stringify(event.payload, null, 2)}</pre>
        </details>
      ))}
    </div>
  );
}

function WorkTable({ items }: { items: WorkItemView[] }) {
  if (items.length === 0) {
    return <EmptyState>暂无任务</EmptyState>;
  }
  return (
    <div className="table-scroll"><table><thead><tr><th>任务</th><th>状态</th><th>优先级</th><th>Owner</th><th>版本</th></tr></thead><tbody>
      {items.map((item) => <tr key={item.id}><td><strong>{item.title}</strong>{item.description && <small>{item.description}</small>}</td><td><StatusBadge status={item.status} /></td><td>{item.priority}</td><td>{item.owner_agent_id || item.owner || "-"}</td><td>v{item.version}</td></tr>)}
    </tbody></table></div>
  );
}

function NoteList({ notes }: { notes: NoteView[] }) {
  if (notes.length === 0) {
    return <EmptyState>暂无记忆</EmptyState>;
  }
  return <div className="note-list">{notes.map((note) => (
    <article className="note-item" key={note.id}>
      <header><div><strong>{note.title}</strong><span>{note.category} / {note.visibility}</span></div><StatusBadge status={note.status} /></header>
      <p>{note.content}</p>
      <footer><span>{note.tags.join(" · ") || "untagged"}</span><time dateTime={note.updated_at}>{formatDate(note.updated_at)}</time></footer>
    </article>
  ))}</div>;
}

function ArtifactTable({ artifacts }: { artifacts: ArtifactView[] }) {
  if (artifacts.length === 0) {
    return <EmptyState>暂无产物</EmptyState>;
  }
  return (
    <div className="table-scroll"><table><thead><tr><th>Descriptor</th><th>Tool / stream</th><th>MIME</th><th>大小</th><th>SHA-256</th></tr></thead><tbody>
      {artifacts.map((item) => <tr key={item.id}><td><strong>{shortID(item.id)}</strong><small>{item.kind}{item.redacted ? " / redacted" : ""}</small></td><td>{item.tool_name}<small>{item.stream}</small></td><td>{item.mime}</td><td>{formatBytes(item.size_bytes)}</td><td><code>{shortID(item.sha256)}</code></td></tr>)}
    </tbody></table></div>
  );
}

function ToolRounds({ rounds }: { rounds: SupervisorToolRoundView[] }) {
  if (rounds.length === 0) {
    return <EmptyState>暂无工具轮次</EmptyState>;
  }
  return <div className="tool-rounds">{rounds.map((round) => (
    <section className="tool-round" key={`${round.attempt_id}-${round.turn}-${round.round}`}>
      <header><strong>Turn {round.turn} / Round {round.round}</strong><span>{round.attempt_id}</span><time dateTime={round.created_at}>{formatDate(round.created_at)}</time></header>
      {round.calls.map((call) => (
        <details className="tool-call" key={`${call.position}-${call.call_id}`}>
          <summary><span>{call.position}</span><strong>{call.tool_name}</strong><StatusBadge status={call.status} /></summary>
          <div className="tool-json"><div><label>Payload</label><pre>{JSON.stringify(call.payload, null, 2)}</pre></div><div><label>Result</label><pre>{JSON.stringify(call.result ?? null, null, 2)}</pre></div></div>
        </details>
      ))}
    </section>
  ))}</div>;
}

function CollectionState({ query, empty, children }: { query: { isLoading: boolean; isError: boolean; error: unknown; data?: unknown }; empty: string; children: React.ReactNode }) {
  if (query.isLoading) {
    return <LoadingState />;
  }
  if (query.isError) {
    return <ErrorState error={query.error} />;
  }
  if (!query.data) {
    return <EmptyState>{empty}</EmptyState>;
  }
  return <>{children}</>;
}

function EmptyWorkspace({ icon, title }: { icon: React.ReactNode; title: string }) {
  return <div className="workspace-empty">{icon}<h1>{title}</h1></div>;
}
