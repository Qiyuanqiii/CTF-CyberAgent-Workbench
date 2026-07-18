import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban, Check, LoaderCircle, ShieldCheck } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type {
  ApprovalDecisionControlRequestView,
  ApprovalQueueItemView,
} from "../api/types";
import { formatDate, shortID } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

type ApprovalAction = ApprovalDecisionControlRequestView["action"];

export function ApprovalPanel({ client, runID }: { client: CyberAgentClient; runID: string }) {
  const queryClient = useQueryClient();
  const [reasons, setReasons] = useState<Record<string, string>>({});
  const operationKeys = useRef(new Map<string, string>());
  const query = useQuery({
    queryKey: ["run", runID, "approvals"],
    queryFn: ({ signal }) => client.approvalQueue(runID, signal),
    enabled: runID !== "",
  });
  const mutation = useMutation({
    mutationFn: ({ item, action, reason, intent }: {
      item: ApprovalQueueItemView;
      action: ApprovalAction;
      reason: string;
      intent: string;
    }) => {
      let operationKey = operationKeys.current.get(intent);
      if (!operationKey) {
        operationKey = `web-approval-${globalThis.crypto.randomUUID()}`;
        operationKeys.current.set(intent, operationKey);
      }
      const body: ApprovalDecisionControlRequestView = {
        version: "approval_control.v1",
        action,
        ...(action === "deny" && reason ? { reason } : {}),
      };
      return client.decideApproval(runID, item.id, body, operationKey);
    },
    onSuccess: (_result, variables) => {
      operationKeys.current.delete(variables.intent);
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "approvals"] });
      void queryClient.invalidateQueries({ queryKey: ["run", runID] });
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "events"] });
    },
  });
  if (query.isLoading) {
    return <LoadingState label="Loading approvals" />;
  }
  if (query.isError || !query.data) {
    return <ErrorState error={query.error} />;
  }
  const decide = (item: ApprovalQueueItemView, action: ApprovalAction) => {
    const reason = action === "deny" ? (reasons[item.id] ?? "").trim() : "";
    mutation.mutate({ item, action, reason,
      intent: `${item.id}:${action}:${reason}` });
  };
  return (
    <section className="approval-queue" aria-label="Pending approvals">
      <header className="approval-queue-header">
        <div><ShieldCheck aria-hidden="true" size={16} /><strong>Pending approvals</strong></div>
        <span>{query.data.items.length}{query.data.truncated ? "+" : ""}</span>
      </header>
      <div className="approval-boundary-line">
        <span>Process execution: off</span><span>Session grant: none</span><span>Capability grant: none</span>
      </div>
      {query.data.items.length === 0 ? <EmptyState>No pending approvals</EmptyState> : (
        <div className="approval-list">
          {query.data.items.map((item) => {
            const busy = mutation.isPending && mutation.variables?.item.id === item.id;
            const canApprove = client.hasApprovalControl &&
              item.allowed_actions.includes("approve_once");
            const canDeny = client.hasApprovalControl && item.allowed_actions.includes("deny");
            return (
              <article className="approval-row" key={item.id}>
                <div className="approval-row-main">
                  <span><strong>{item.tool_name}</strong><small>{item.action_class} / {item.mode}</small></span>
                  <code>{shortID(item.proposal_id)}</code>
                  <StatusBadge status={item.status} />
                  <time dateTime={item.created_at}>{formatDate(item.created_at)}</time>
                </div>
                {(canApprove || canDeny) && (
                  <div className="approval-actions">
                    {canDeny && <input aria-label={`Denial reason for ${item.tool_name}`}
                      disabled={busy} maxLength={2048}
                      onChange={(event) => setReasons((current) => ({ ...current,
                        [item.id]: event.target.value }))}
                      placeholder="Denial reason" value={reasons[item.id] ?? ""} />}
                    {canDeny && <button className="command-button danger" disabled={busy}
                      onClick={() => decide(item, "deny")} type="button">
                      {busy && mutation.variables?.action === "deny"
                        ? <LoaderCircle aria-hidden="true" className="spin" size={15} />
                        : <Ban aria-hidden="true" size={15} />}Deny
                    </button>}
                    {canApprove && <button className="command-button" disabled={busy}
                      onClick={() => decide(item, "approve_once")} type="button">
                      {busy && mutation.variables?.action === "approve_once"
                        ? <LoaderCircle aria-hidden="true" className="spin" size={15} />
                        : <Check aria-hidden="true" size={15} />}Approve once
                    </button>}
                  </div>
                )}
              </article>
            );
          })}
        </div>
      )}
      {mutation.isError && <div className="inline-warning" role="alert">
        {mutation.error instanceof Error ? mutation.error.message : "Approval decision failed"}
      </div>}
    </section>
  );
}
