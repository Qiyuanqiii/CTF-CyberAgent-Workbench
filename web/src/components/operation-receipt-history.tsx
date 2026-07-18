import { useQuery } from "@tanstack/react-query";
import { History, RefreshCw } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { formatDate } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";
import { OperationReceipt } from "./operation-receipt";

export function OperationReceiptHistory({ client, runID }: {
  client: CyberAgentClient;
  runID: string;
}) {
  const query = useQuery({
    queryKey: ["operation-receipts", runID],
    queryFn: ({ signal }) => client.operationReceiptHistory(runID, signal),
    enabled: Boolean(runID),
  });

  return <section className="receipt-history" aria-label="Operation receipt history">
    <header>
      <div><History aria-hidden="true" size={16} /><h2>Operation receipts</h2></div>
      <div>
        {query.data?.truncated && <StatusBadge status="truncated" />}
        <button aria-label="Refresh operation receipts" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh receipts" type="button">
          <RefreshCw aria-hidden="true" className={query.isFetching ? "spin" : ""} size={15} />
        </button>
      </div>
    </header>
    {query.isLoading && <LoadingState label="Loading durable receipts" />}
    {query.isError && <ErrorState error={query.error} />}
    {query.data?.items.length === 0 && <EmptyState>No terminal operations recorded</EmptyState>}
    {query.data && query.data.items.length > 0 && <div className="receipt-history-list">
      {query.data.items.map((item) => <article key={item.id}>
        <header>
          <span>{item.scope.replaceAll("_", " ")}</span>
          <time dateTime={item.completed_at}>{formatDate(item.completed_at)}</time>
        </header>
        <OperationReceipt receipt={item.receipt} />
      </article>)}
    </div>}
  </section>;
}
