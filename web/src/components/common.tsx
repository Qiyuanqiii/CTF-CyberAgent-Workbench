import type { ReactNode } from "react";
import { ChevronDown, LoaderCircle } from "lucide-react";

export function StatusBadge({ status }: { status: string }) {
  const normalized = status.toLowerCase().replaceAll("_", "-");
  return <span className={`status-badge status-${normalized}`}>{status.replaceAll("_", " ")}</span>;
}

export function LoadingState({ label = "加载中" }: { label?: string }) {
  return (
    <div className="state-message" role="status">
      <LoaderCircle aria-hidden="true" className="spin" size={18} />
      <span>{label}</span>
    </div>
  );
}

export function ErrorState({ error }: { error: unknown }) {
  const message = error instanceof Error ? error.message : "请求失败";
  return <div className="state-message state-error" role="alert">{message}</div>;
}

export function EmptyState({ children }: { children: ReactNode }) {
  return <div className="empty-state">{children}</div>;
}

export function LoadMoreButton({
  hasNextPage,
  isFetching,
  onClick,
}: {
  hasNextPage: boolean;
  isFetching: boolean;
  onClick: () => void;
}) {
  if (!hasNextPage) {
    return null;
  }
  return (
    <button className="load-more" disabled={isFetching} onClick={onClick} type="button">
      {isFetching ? <LoaderCircle aria-hidden="true" className="spin" size={15} /> : <ChevronDown aria-hidden="true" size={15} />}
      加载更多
    </button>
  );
}

export function KeyValue({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="key-value">
      <dt>{label}</dt>
      <dd>{value || "-"}</dd>
    </div>
  );
}
