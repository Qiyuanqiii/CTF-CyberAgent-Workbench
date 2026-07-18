import { useQuery } from "@tanstack/react-query";
import { Eye, FileCheck2, RefreshCw, ShieldCheck } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { formatDate } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

export function EvidenceInventory({ client, runID, onOpenSource }: {
  client: CyberAgentClient;
  runID: string;
  onOpenSource: (sourceRef: string) => void;
}) {
  const query = useQuery({
    queryKey: ["run", runID, "evidence-inventory"],
    queryFn: ({ signal }) => client.evidenceInventory(runID, signal),
    enabled: Boolean(runID),
  });

  return <section className="evidence-inventory" aria-label="Attached evidence inventory">
    <header className="operator-list-header">
      <div><FileCheck2 aria-hidden="true" size={16} /><h2>Attached evidence</h2></div>
      <div>
        {query.data?.truncated && <StatusBadge status="truncated" />}
        <button aria-label="Refresh attached evidence" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh evidence" type="button">
          <RefreshCw aria-hidden="true" className={query.isFetching ? "spin" : ""} size={15} />
        </button>
      </div>
    </header>
    {query.isLoading && <LoadingState label="Loading attached evidence" />}
    {query.isError && <ErrorState error={query.error} />}
    {query.data?.items.length === 0 && <EmptyState>No evidence has been attached</EmptyState>}
    {query.data && query.data.items.length > 0 && <div className="evidence-inventory-list">
      {query.data.items.map((item) => <div key={item.attachment_id}>
        <ShieldCheck aria-hidden="true" size={16} />
        <span><strong>{item.source_ref}</strong><code>{item.content_sha256}</code></span>
        <time dateTime={item.attached_at}>{formatDate(item.attached_at)}</time>
        <button aria-label={`Open ${item.source_ref}`} className="icon-button"
          onClick={() => onOpenSource(item.source_ref)} title="Open source" type="button">
          <Eye aria-hidden="true" size={15} />
        </button>
      </div>)}
    </div>}
  </section>;
}
