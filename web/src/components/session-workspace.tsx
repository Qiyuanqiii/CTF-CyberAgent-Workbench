import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { MessagesSquare } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { MessageView, SessionDetailView } from "../api/types";
import { usePagedResource } from "../hooks/use-paged-resource";
import { formatDate, formatNumber, shortID } from "../lib/format";
import { EmptyState, ErrorState, KeyValue, LoadMoreButton, LoadingState, StatusBadge } from "./common";
import { SessionComposer } from "./session-composer";

export function SessionWorkspace({ client, sessionID }: { client: CyberAgentClient; sessionID: string }) {
  const detailQuery = useQuery({
    queryKey: ["session", sessionID],
    queryFn: ({ signal }) => client.get<SessionDetailView>(`/sessions/${encodeURIComponent(sessionID)}`, {}, signal),
    enabled: Boolean(sessionID),
  });
  const messagesQuery = usePagedResource<MessageView>(client, ["session", sessionID, "messages"],
    `/sessions/${encodeURIComponent(sessionID)}/messages`, { limit: 100, include_compacted: true }, Boolean(sessionID));
  const messages = useMemo(() => messagesQuery.data?.pages.flatMap((page) => page.items) ?? [], [messagesQuery.data]);

  if (!sessionID) {
    return <div className="workspace-empty"><MessagesSquare aria-hidden="true" size={24} /><h1>选择一个 Session</h1></div>;
  }
  if (detailQuery.isLoading) {
    return <LoadingState label="加载 Session" />;
  }
  if (detailQuery.isError || !detailQuery.data) {
    return <ErrorState error={detailQuery.error} />;
  }
  const detail = detailQuery.data;

  return (
    <div className="workspace-view">
      <header className="workspace-header">
        <div>
          <div className="workspace-kicker">Session {shortID(detail.session.id)}</div>
          <h1>{detail.session.title}</h1>
          <div className="header-meta"><StatusBadge status={detail.session.status} /><span>{detail.session.route}</span></div>
        </div>
      </header>
      <div className="session-summary">
        <dl className="detail-grid">
          <KeyValue label="Workspace" value={detail.session.workspace_id} />
          <KeyValue label="Bound Run" value={detail.run ? shortID(detail.run.id) : "-"} />
          <KeyValue label="Created" value={formatDate(detail.session.created_at)} />
          <KeyValue label="Updated" value={formatDate(detail.session.updated_at)} />
        </dl>
      </div>
      <SessionComposer client={client} key={sessionID} run={detail.run ?? null}
        sessionID={sessionID} />
      <div className="workspace-content session-content">
        <div className="section-heading"><h2>Messages</h2><span>{formatNumber(messages.length)}</span></div>
        {messagesQuery.isLoading && <LoadingState />}
        {messagesQuery.isError && <ErrorState error={messagesQuery.error} />}
        {!messagesQuery.isLoading && !messagesQuery.isError && messages.length === 0 && <EmptyState>暂无消息</EmptyState>}
        <div className="message-list">
          {messages.map((message) => (
            <article className={`message-row role-${message.role}`} key={message.id}>
              <header><strong>{message.role}</strong><StatusBadge status={message.source_kind} /><span>{formatNumber(message.token_estimate)} tokens</span>{message.compacted && <StatusBadge status="compacted" />}<time dateTime={message.created_at}>{formatDate(message.created_at)}</time></header>
              <p>{message.content}</p>
            </article>
          ))}
        </div>
        <LoadMoreButton hasNextPage={Boolean(messagesQuery.hasNextPage)} isFetching={messagesQuery.isFetchingNextPage} onClick={() => void messagesQuery.fetchNextPage()} />
      </div>
    </div>
  );
}
