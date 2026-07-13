import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FileSearch, GitBranch, Network, ScanSearch, ShieldAlert } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type {
  AgentGraphView,
  DelegationView,
  FanoutPlanView,
  FindingReportSummaryView,
  FindingReportView,
} from "../api/types";
import { usePagedResource } from "../hooks/use-paged-resource";
import { formatBytes, formatDate, formatNumber, shortID } from "../lib/format";
import { EmptyState, ErrorState, LoadMoreButton, LoadingState, StatusBadge } from "./common";

export function AgentGraphPanel({ client, runID }: ProjectionProps) {
  const query = useQuery({
    queryKey: ["run", runID, "agent-graph"],
    queryFn: ({ signal }) => client.get<AgentGraphView>(`/runs/${encodeURIComponent(runID)}/agent-graph`, {}, signal),
  });
  if (query.isLoading) return <LoadingState label="加载 Agent 图" />;
  if (query.isError || !query.data) return <ErrorState error={query.error} />;
  if (query.data.nodes.length === 0) return <EmptyState>暂无 Agent</EmptyState>;
  return (
    <div className="projection-stack agent-graph" aria-label="Agent graph">
      {query.data.nodes.map((node) => (
        <article className={`agent-node agent-depth-${node.depth}`} key={node.id}>
          <header>
            <span className="node-role"><GitBranch aria-hidden="true" size={15} />{node.role}</span>
            <strong>{shortID(node.id)}</strong>
            <StatusBadge status={node.status} />
          </header>
          <dl className="projection-metrics">
            <Metric label="Session" value={shortID(node.session_id)} />
            <Metric label="Profile" value={node.profile} />
            <Metric label="Turns" value={`${formatNumber(node.turns_used)} / ${formatNumber(node.turn_limit)}`} />
            <Metric label="Tokens" value={`${formatNumber(node.tokens_used)} / ${formatNumber(node.token_limit)}`} />
          </dl>
          <div className="tag-line">{node.skills.map((skill) => <code key={skill}>{skill}</code>)}</div>
          {node.completion && (
            <div className="completion-summary">
              <StatusBadge status={node.completion.outcome} />
              <span>{node.completion.summary}</span>
            </div>
          )}
        </article>
      ))}
    </div>
  );
}

export function DelegationsPanel({ client, runID }: ProjectionProps) {
  const query = usePagedResource<DelegationView>(client, ["run", runID, "delegations"],
    `/runs/${encodeURIComponent(runID)}/delegations`, { limit: 50 });
  const items = useMemo(() => query.data?.pages.flatMap((page) => page.items) ?? [], [query.data]);
  if (query.isLoading) return <LoadingState label="加载委派" />;
  if (query.isError) return <ErrorState error={query.error} />;
  if (items.length === 0) return <EmptyState>暂无委派提案</EmptyState>;
  return (
    <div className="projection-stack">
      {items.map((item) => (
        <article className="delegation-item" key={item.id}>
          <header className="projection-header">
            <div><span className="projection-kicker"><Network aria-hidden="true" size={14} />{shortID(item.id)}</span><strong>{item.assignments.map((entry) => entry.title).join(" / ")}</strong></div>
            <div className="status-line"><StatusBadge status={item.status} />{item.review && <StatusBadge status={item.review.decision} />}{item.application && <StatusBadge status={item.application.status} />}</div>
          </header>
          <div className="assignment-list">
            {item.assignments.map((assignment) => (
              <section key={assignment.ordinal}>
                <header><span>#{assignment.ordinal}</span><strong>{assignment.title}</strong>{assignment.application_status && <StatusBadge status={assignment.application_status} />}</header>
                <p>{assignment.goal}</p>
                <footer><span>{assignment.skills.join(" · ")}</span><span>{formatNumber(assignment.turn_limit)} turns / {formatNumber(assignment.token_limit)} tokens</span>{assignment.agent_id && <code>{shortID(assignment.agent_id)}</code>}</footer>
              </section>
            ))}
          </div>
          <dl className="projection-metrics">
            <Metric label="Review" value={item.review ? `${item.review.decision} · ${item.review.reviewed_by}` : "pending"} />
            <Metric label="Application" value={item.application?.status ?? "-"} />
            <Metric label="Schedule" value={item.latest_schedule?.status ?? (item.latest_schedule ? "requested" : "-")} />
            <Metric label="Created" value={formatDate(item.created_at)} />
          </dl>
        </article>
      ))}
      <LoadMoreButton hasNextPage={Boolean(query.hasNextPage)} isFetching={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()} />
    </div>
  );
}

export function FanoutPanel({ client, runID }: ProjectionProps) {
  const query = usePagedResource<FanoutPlanView>(client, ["run", runID, "fanout"],
    `/runs/${encodeURIComponent(runID)}/fanout-plans`, { limit: 50 });
  const items = useMemo(() => query.data?.pages.flatMap((page) => page.items) ?? [], [query.data]);
  if (query.isLoading) return <LoadingState label="加载 Fan-out" />;
  if (query.isError) return <ErrorState error={query.error} />;
  if (items.length === 0) return <EmptyState>暂无只读 Fan-out 计划</EmptyState>;
  return (
    <div className="projection-stack">
      {items.map((plan) => (
        <article className="fanout-item" key={plan.id}>
          <header className="projection-header">
            <div><span className="projection-kicker"><ScanSearch aria-hidden="true" size={14} />{shortID(plan.id)}</span><strong>{plan.goal}</strong></div>
            <StatusBadge status={plan.latest_execution?.status ?? plan.status} />
          </header>
          <dl className="projection-metrics">
            <Metric label="Tier" value={`${plan.requested_tier} → ${plan.effective_parallelism}`} />
            <Metric label="Files" value={formatNumber(plan.file_count)} />
            <Metric label="Input" value={formatBytes(plan.total_bytes)} />
            <Metric label="Excluded" value={formatNumber(plan.excluded_count)} />
          </dl>
          {plan.latest_execution ? <ShardTable execution={plan.latest_execution} /> : <div className="projection-placeholder">尚未执行</div>}
        </article>
      ))}
      <LoadMoreButton hasNextPage={Boolean(query.hasNextPage)} isFetching={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()} />
    </div>
  );
}

function ShardTable({ execution }: { execution: NonNullable<FanoutPlanView["latest_execution"]> }) {
  return (
    <div className="table-scroll shard-table"><table><thead><tr><th>Shard</th><th>状态</th><th>模型</th><th>Tokens</th><th>Findings</th><th>耗时</th></tr></thead><tbody>
      {execution.shards.map((shard) => <tr key={shard.ordinal}><td>#{shard.ordinal}</td><td><StatusBadge status={shard.status} /></td><td>{shard.provider && shard.model ? `${shard.provider}/${shard.model}` : "-"}</td><td>{formatNumber(shard.total_tokens)}</td><td>{formatNumber(shard.finding_count)}</td><td>{formatNumber(shard.elapsed_millis)} ms</td></tr>)}
    </tbody></table></div>
  );
}

export function FindingsPanel({ client, runID }: ProjectionProps) {
  const listQuery = usePagedResource<FindingReportSummaryView>(client, ["run", runID, "reports"],
    `/runs/${encodeURIComponent(runID)}/reports`, { limit: 50 });
  const reports = useMemo(() => listQuery.data?.pages.flatMap((page) => page.items) ?? [], [listQuery.data]);
  const [selectedID, setSelectedID] = useState("");
  useEffect(() => {
    if (reports.length > 0 && !reports.some((report) => report.id === selectedID)) setSelectedID(reports[0].id);
  }, [reports, selectedID]);
  const detailQuery = useQuery({
    queryKey: ["run", runID, "report", selectedID],
    queryFn: ({ signal }) => client.get<FindingReportView>(`/runs/${encodeURIComponent(runID)}/reports/${encodeURIComponent(selectedID)}`, {}, signal),
    enabled: selectedID !== "",
  });
  if (listQuery.isLoading) return <LoadingState label="加载 Finding 报告" />;
  if (listQuery.isError) return <ErrorState error={listQuery.error} />;
  if (reports.length === 0) return <EmptyState>暂无 Finding 报告</EmptyState>;
  return (
    <div className="finding-layout">
      <aside className="report-picker" aria-label="Finding reports">
        {reports.map((report) => (
          <button className={selectedID === report.id ? "selected" : ""} key={report.id} onClick={() => setSelectedID(report.id)} type="button">
            <span><FileSearch aria-hidden="true" size={14} />{shortID(report.id)}</span>
            <strong>{report.title}</strong>
            <small>{report.finding_count} findings · {report.severity.critical + report.severity.high} high+</small>
          </button>
        ))}
        <LoadMoreButton hasNextPage={Boolean(listQuery.hasNextPage)} isFetching={listQuery.isFetchingNextPage} onClick={() => void listQuery.fetchNextPage()} />
      </aside>
      <section className="finding-detail">
        {detailQuery.isLoading && <LoadingState label="加载报告详情" />}
        {detailQuery.isError && <ErrorState error={detailQuery.error} />}
        {detailQuery.data && <FindingList report={detailQuery.data} />}
      </section>
    </div>
  );
}

function FindingList({ report }: { report: FindingReportView }) {
  if (report.findings.length === 0) return <EmptyState>报告没有 Finding</EmptyState>;
  return <div className="projection-stack">{report.findings.map((finding) => (
    <article className="finding-item" key={finding.id}>
      <header className="projection-header"><div><span className="projection-kicker"><ShieldAlert aria-hidden="true" size={14} />#{finding.ordinal} · {finding.category}</span><strong>{finding.title}</strong></div><div className="status-line"><StatusBadge status={finding.severity} /><StatusBadge status={finding.status} /></div></header>
      <p>{finding.detail}</p>
      <dl className="projection-metrics">
        <Metric label="Location" value={`${finding.relative_path}:${finding.line_start || "-"}`} />
        <Metric label="Confidence" value={`${finding.confidence}%`} />
        <Metric label="Evidence" value={`${finding.evidence.length} model / ${finding.lifecycle.validation_evidence_count} validation`} />
        <Metric label="Remediation" value={formatNumber(finding.lifecycle.remediation_evidence_count)} />
      </dl>
    </article>
  ))}</div>;
}

function Metric({ label, value }: { label: string; value: string }) {
  return <div><dt>{label}</dt><dd>{value || "-"}</dd></div>;
}

interface ProjectionProps { client: CyberAgentClient; runID: string }
