import { useEffect, useMemo, useState } from "react";
import { Archive, ListTree, RefreshCw, Search, TerminalSquare } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { RunView, SessionView } from "../api/types";
import { usePagedResource } from "../hooks/use-paged-resource";
import { formatCompactDate, shortID } from "../lib/format";
import { useConnectionStore } from "../state/connection";
import { EmptyState, ErrorState, LoadMoreButton, LoadingState, StatusBadge } from "./common";

const runStatuses = ["", "created", "preparing", "running", "waiting_approval", "paused", "completed", "failed", "cancelled"];

export function ResourceSidebar({ client }: { client: CyberAgentClient }) {
  const [status, setStatus] = useState("");
  const [search, setSearch] = useState("");
  const kind = useConnectionStore((state) => state.resourceKind);
  const selectedRunID = useConnectionStore((state) => state.selectedRunID);
  const selectedSessionID = useConnectionStore((state) => state.selectedSessionID);
  const selectRun = useConnectionStore((state) => state.selectRun);
  const selectSession = useConnectionStore((state) => state.selectSession);
  const setKind = useConnectionStore((state) => state.setResourceKind);

  const runsQuery = usePagedResource<RunView>(client, ["runs", status], "/runs", {
    limit: 50,
    status: status || undefined,
  }, kind === "run");
  const sessionsQuery = usePagedResource<SessionView>(client, ["sessions"], "/sessions", { limit: 50 }, kind === "session");
  const runs = useMemo(() => runsQuery.data?.pages.flatMap((page) => page.items) ?? [], [runsQuery.data]);
  const sessions = useMemo(() => sessionsQuery.data?.pages.flatMap((page) => page.items) ?? [], [sessionsQuery.data]);
  const normalizedSearch = search.trim().toLowerCase();
  const visibleRuns = runs.filter((run) => !normalizedSearch || `${run.id} ${run.mission_id} ${run.status}`.toLowerCase().includes(normalizedSearch));
  const visibleSessions = sessions.filter((session) => !normalizedSearch || `${session.id} ${session.title} ${session.route}`.toLowerCase().includes(normalizedSearch));

  useEffect(() => {
    if (kind === "run" && !runsQuery.isLoading && !runs.some((run) => run.id === selectedRunID)) {
      selectRun(runs[0]?.id ?? "");
    }
  }, [kind, runs, runsQuery.isLoading, selectRun, selectedRunID]);

  useEffect(() => {
    if (kind === "session" && !sessionsQuery.isLoading && !sessions.some((session) => session.id === selectedSessionID)) {
      selectSession(sessions[0]?.id ?? "");
    }
  }, [kind, selectSession, selectedSessionID, sessions, sessionsQuery.isLoading]);

  const activeQuery = kind === "run" ? runsQuery : sessionsQuery;

  return (
    <aside className="resource-sidebar">
      <div className="resource-tabs" role="tablist" aria-label="资源类型">
        <button aria-selected={kind === "run"} className={kind === "run" ? "active" : ""} onClick={() => setKind("run")} role="tab" type="button">
          <ListTree aria-hidden="true" size={16} />Runs
        </button>
        <button aria-selected={kind === "session"} className={kind === "session" ? "active" : ""} onClick={() => setKind("session")} role="tab" type="button">
          <TerminalSquare aria-hidden="true" size={16} />Sessions
        </button>
      </div>
      <div className="sidebar-tools">
        <label className="search-field">
          <Search aria-hidden="true" size={15} />
          <input aria-label="搜索" onChange={(event) => setSearch(event.target.value)} placeholder="搜索" type="search" value={search} />
        </label>
        <button aria-label="刷新列表" className="icon-button" disabled={activeQuery.isFetching} onClick={() => void activeQuery.refetch()} title="刷新列表" type="button">
          <RefreshCw aria-hidden="true" className={activeQuery.isFetching ? "spin" : ""} size={16} />
        </button>
      </div>
      {kind === "run" && (
        <select aria-label="Run 状态" className="status-filter" onChange={(event) => setStatus(event.target.value)} value={status}>
          {runStatuses.map((value) => <option key={value || "all"} value={value}>{value ? value.replaceAll("_", " ") : "全部状态"}</option>)}
        </select>
      )}
      <div className="resource-list">
        {activeQuery.isLoading && <LoadingState />}
        {activeQuery.isError && <ErrorState error={activeQuery.error} />}
        {!activeQuery.isLoading && !activeQuery.isError && kind === "run" && visibleRuns.map((run) => (
          <button className={`resource-row ${selectedRunID === run.id ? "selected" : ""}`} key={run.id} onClick={() => selectRun(run.id)} type="button">
            <span className="resource-row-top"><strong>{shortID(run.id)}</strong><StatusBadge status={run.status} /></span>
            <span>Mission {shortID(run.mission_id)}</span>
            <time dateTime={run.updated_at}>{formatCompactDate(run.updated_at)}</time>
          </button>
        ))}
        {!activeQuery.isLoading && !activeQuery.isError && kind === "session" && visibleSessions.map((session) => (
          <button className={`resource-row ${selectedSessionID === session.id ? "selected" : ""}`} key={session.id} onClick={() => selectSession(session.id)} type="button">
            <span className="resource-row-top"><strong>{session.title}</strong><StatusBadge status={session.status} /></span>
            <span>{session.route} / {shortID(session.id)}</span>
            <time dateTime={session.updated_at}>{formatCompactDate(session.updated_at)}</time>
          </button>
        ))}
        {!activeQuery.isLoading && !activeQuery.isError && ((kind === "run" && visibleRuns.length === 0) || (kind === "session" && visibleSessions.length === 0)) && (
          <EmptyState><Archive aria-hidden="true" size={19} />暂无数据</EmptyState>
        )}
        <LoadMoreButton hasNextPage={Boolean(activeQuery.hasNextPage)} isFetching={activeQuery.isFetchingNextPage} onClick={() => void activeQuery.fetchNextPage()} />
      </div>
    </aside>
  );
}
