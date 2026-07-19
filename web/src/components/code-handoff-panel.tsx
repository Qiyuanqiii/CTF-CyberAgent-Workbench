import { useQuery } from "@tanstack/react-query";
import { BookOpenCheck, RefreshCw } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { formatBytes, formatDate, shortID } from "../lib/format";
import { EmptyState, ErrorState, KeyValue, LoadingState, StatusBadge } from "./common";

export function CodeHandoffPanel({ client, runID }: {
  client: CyberAgentClient;
  runID: string;
}) {
  const query = useQuery({
    queryKey: ["run", runID, "code-handoff"],
    queryFn: ({ signal }) => client.codeHandoff(runID, signal),
    enabled: Boolean(runID),
  });
  return <section aria-label="Code handoff" className="code-handoff-panel">
    <header className="projection-heading">
      <div><BookOpenCheck aria-hidden="true" size={17} /><h2>Code handoff</h2></div>
      <div>{query.data && <StatusBadge status={query.data.run_status} />}
        <button aria-label="Refresh Code handoff" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh" type="button"><RefreshCw aria-hidden="true"
            className={query.isFetching ? "spin" : ""} size={15} /></button></div>
    </header>
    {query.isLoading && <LoadingState label="Building Code handoff" />}
    {query.isError && <ErrorState error={query.error} />}
    {query.data && <>
      <dl className="handoff-grid">
        <KeyValue label="Phase" value={query.data.phase} />
        <KeyValue label="Plan" value={query.data.plan.state} />
        <KeyValue label="Modules" value={`${query.data.plan.completed_count} / ${query.data.plan.module_count}`} />
        <KeyValue label="Queue" value={`${query.data.queue.pending} pending`} />
        <KeyValue label="Changes" value={`${query.data.change_set.returned_count} / ${formatBytes(query.data.change_set.total_diff_bytes)}`} />
        <KeyValue label="Verification" value={`${query.data.verification.pass_count} pass / ${query.data.verification.fail_count} fail`} />
        <KeyValue label="Actions" value={query.data.pending_action_count} />
        <KeyValue label="Generated" value={formatDate(query.data.generated_at)} />
      </dl>
      <div className="handoff-reference-columns">
        <section><h3>Pending actions</h3>
          {query.data.pending_actions.length === 0 ? <EmptyState>No pending actions</EmptyState> :
            <div className="handoff-reference-list">{query.data.pending_actions.map((item) =>
              <div key={item.id}><span><strong>{item.kind.replaceAll("_", " ")}</strong>
                <code>{shortID(item.id)}</code></span><StatusBadge status={item.state} /></div>)}</div>}
        </section>
        <section><h3>Reports</h3>
          {query.data.report_references.length === 0 ? <EmptyState>No reports</EmptyState> :
            <div className="handoff-reference-list">{query.data.report_references.map((item) =>
              <div key={item.id}><span><strong>{shortID(item.id)}</strong>
                <small>{item.finding_count} findings</small></span>
                <StatusBadge status={item.status} /></div>)}</div>}
        </section>
      </div>
    </>}
  </section>;
}
