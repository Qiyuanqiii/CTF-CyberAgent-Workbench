import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Boxes,
  ClipboardList,
  Database,
  FileArchive,
  Gauge,
  GitBranch,
  ListChecks,
  ListOrdered,
  Network,
  Radio,
  ScanSearch,
  ShieldAlert,
  StickyNote,
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
  SupervisorToolRoundView,
  WorkItemView,
} from "../api/types";
import { usePagedResource } from "../hooks/use-paged-resource";
import { useRunEventStream } from "../hooks/use-run-event-stream";
import { formatBytes, formatDate, formatNumber, shortID } from "../lib/format";
import { EmptyState, ErrorState, KeyValue, LoadMoreButton, LoadingState, StatusBadge } from "./common";
import { AgentGraphPanel, DelegationsPanel, FanoutPanel, FindingsPanel } from "./run-projections";

type RunTab = "overview" | "agents" | "delegations" | "fanout" | "findings" | "events" | "work" | "notes" | "artifacts" | "tools";

const tabs: Array<{ id: RunTab; label: string; icon: typeof Activity }> = [
  { id: "overview", label: "概览", icon: Gauge },
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
        {tab === "overview" && <RunOverview detail={detail} />}
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

function RunOverview({ detail }: { detail: RunDetailView }) {
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
      {detail.plan_delivery && <PlanDeliveryPanel state={detail.plan_delivery} />}
    </div>
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

export function PlanDeliveryPanel({ state }: { state: PlanDeliveryStateView }) {
  const selected = state.selection?.direction_ordinal;
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
            </div>
          </details>
        ))}
      </div>
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
