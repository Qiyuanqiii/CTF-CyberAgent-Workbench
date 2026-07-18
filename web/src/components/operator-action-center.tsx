import { useQuery } from "@tanstack/react-query";
import { AlarmClock, FileDiff, ListChecks, MessageSquareMore, RefreshCw, ShieldCheck } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { OperatorActionItemView } from "../api/types";
import { formatDate } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

const actionPresentation: Record<OperatorActionItemView["kind"], {
  label: string;
  icon: typeof ListChecks;
}> = {
  steering_pending: { label: "Queued operator input", icon: MessageSquareMore },
  approval_pending: { label: "Approval review", icon: ShieldCheck },
  file_edit_review: { label: "File edit review", icon: FileDiff },
  file_edit_apply: { label: "Approved edit ready", icon: FileDiff },
  wake_due: { label: "Scheduled wake due", icon: AlarmClock },
};

export function OperatorActionCenter({ client, runID, onNavigate }: {
  client: CyberAgentClient;
  runID: string;
  onNavigate: (destination: OperatorActionItemView["destination"]) => void;
}) {
  const query = useQuery({
    queryKey: ["run", runID, "operator-actions"],
    queryFn: ({ signal }) => client.operatorActionCenter(runID, signal),
    enabled: Boolean(runID),
  });

  return <section className="operator-action-center" aria-label="Operator action center">
    <header className="operator-list-header">
      <div><ListChecks aria-hidden="true" size={16} /><h2>Operator actions</h2></div>
      <div>
        {query.data?.truncated && <StatusBadge status="truncated" />}
        <button aria-label="Refresh operator actions" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh actions" type="button">
          <RefreshCw aria-hidden="true" className={query.isFetching ? "spin" : ""} size={15} />
        </button>
      </div>
    </header>
    {query.isLoading && <LoadingState label="Loading operator actions" />}
    {query.isError && <ErrorState error={query.error} />}
    {query.data?.items.length === 0 && <EmptyState>No operator action is pending</EmptyState>}
    {query.data && query.data.items.length > 0 && <div className="operator-action-list">
      {query.data.items.map((item) => {
        const presentation = actionPresentation[item.kind];
        const Icon = presentation.icon;
        return <button key={item.id} onClick={() => onNavigate(item.destination)} type="button">
          <Icon aria-hidden="true" size={16} />
          <span><strong>{presentation.label}</strong><code>{item.id}</code></span>
          <StatusBadge status={item.state} />
          <time dateTime={item.due_at ?? item.available_at}>
            {formatDate(item.due_at ?? item.available_at)}
          </time>
        </button>;
      })}
    </div>}
  </section>;
}
