import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowRight, BookOpenCheck, Download, RefreshCw } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { formatBytes, formatDate, shortID } from "../lib/format";
import { downloadTextFile } from "../lib/download";
import { EmptyState, ErrorState, KeyValue, LoadingState, StatusBadge } from "./common";
import type { ReceiptReviewNavigationTarget } from "./receipt-review-navigation";

export function CodeHandoffPanel({ client, runID, onOpenReceiptReview }: {
  client: CyberAgentClient;
  runID: string;
  onOpenReceiptReview: (target: ReceiptReviewNavigationTarget) => void;
}) {
  const query = useQuery({
    queryKey: ["run", runID, "code-handoff"],
    queryFn: ({ signal }) => client.codeHandoff(runID, signal),
    enabled: Boolean(runID),
  });
  const exportHandoff = useMutation({
    mutationFn: (format: "markdown" | "json") => client.codeHandoffExport(runID, format),
    onSuccess: (value) => downloadTextFile(value.filename, value.mime_type, value.content),
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
        <KeyValue label="Checklists" value={query.data.verification_plans.returned_count} />
        <KeyValue label="Coverage" value={`${query.data.verification_coverage.observed_plan_item_count} / ${query.data.verification_coverage.plan_item_count}`} />
        <KeyValue label="Contradictions" value={query.data.verification_coverage.contradictory_item_count} />
        <KeyValue label="Receipt reviews" value={`${query.data.verification_snapshot_receipt_reviews.metadata_confirmed_count} confirmed / ${query.data.verification_snapshot_receipt_reviews.metadata_disputed_count} disputed`} />
        <KeyValue label="Actions" value={query.data.pending_action_count} />
        <KeyValue label="Event high-water" value={query.data.source_event_sequence} />
        <KeyValue label="Generated" value={formatDate(query.data.generated_at)} />
      </dl>
      <div className="handoff-export-actions">
        <span>SHA-256 verified download</span>
        <button className="compact-command" disabled={exportHandoff.isPending}
          onClick={() => exportHandoff.mutate("markdown")} type="button">
          <Download aria-hidden="true" size={14} />Markdown</button>
        <button className="compact-command" disabled={exportHandoff.isPending}
          onClick={() => exportHandoff.mutate("json")} type="button">
          <Download aria-hidden="true" size={14} />JSON</button>
      </div>
      {exportHandoff.error && <ErrorState error={exportHandoff.error} />}
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
        <section className="handoff-coverage"><h3>Verification coverage</h3>
          {query.data.verification_coverage.items.length === 0 ?
            <EmptyState>No checklist items</EmptyState> :
            <div className="handoff-reference-list">{query.data.verification_coverage.items.map((item) => {
              const contradictory = item.pass_count > 0 && item.fail_count > 0;
              return <div key={`${item.plan_id}:${item.ordinal}`}><span>
                <strong>{shortID(item.plan_id)} / item {item.ordinal}</strong>
                <small>{item.pass_count} pass / {item.fail_count} fail / {item.unknown_count} unknown</small>
              </span><StatusBadge status={contradictory ? "conflict" :
                item.associated_evidence_count > 0 ? "observed" : "unobserved"} /></div>;
            })}</div>}
        </section>
        <section><h3>Receipt metadata reviews</h3>
          {query.data.verification_snapshot_receipt_reviews.references.length === 0 ?
            <EmptyState>No receipt reviews</EmptyState> :
            <div className="handoff-reference-list">
              {query.data.verification_snapshot_receipt_reviews.references.map((item) =>
                <div key={item.id}><span><strong>{shortID(item.receipt_id)}</strong>
                  <small>event {item.review_event_sequence}</small></span>
                  <div className="handoff-reference-actions">
                    <StatusBadge status={item.decision.replaceAll("_", " ")} />
                    <button aria-label={`Open receipt review ${item.id} in Verify`}
                      className="icon-button" onClick={() => onOpenReceiptReview(item)}
                      title="Open exact receipt review in Verify" type="button">
                      <ArrowRight aria-hidden="true" size={14} />
                    </button>
                  </div></div>)}
            </div>}
        </section>
      </div>
    </>}
  </section>;
}
