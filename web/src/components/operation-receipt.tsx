import { CheckCircle2, History, ShieldAlert } from "lucide-react";
import type { OperationReceiptView } from "../api/types";

export function OperationReceipt({ receipt }: { receipt: OperationReceiptView }) {
  const pending = receipt.cleanup_state === "pending_review";
  const failed = receipt.outcome === "failed";
  const warning = pending || failed;
  const Icon = warning ? ShieldAlert : receipt.replayed ? History : CheckCircle2;
  return <div className={`operation-receipt ${warning ? "receipt-warning" : ""}`}
    role={failed ? "alert" : "status"}>
    <Icon aria-hidden="true" size={15} />
    <div>
      <strong>{receipt.outcome}</strong>
      <span>{receipt.kind.replaceAll("_", " ")} / durable{receipt.replayed ? " / replayed" : ""}</span>
      {failed && <small>The durable failed result will replay for the same operation key.</small>}
      {pending && <small>Staging cleanup is pending. Retry the same operation after the cleanup grace period.</small>}
    </div>
  </div>;
}
