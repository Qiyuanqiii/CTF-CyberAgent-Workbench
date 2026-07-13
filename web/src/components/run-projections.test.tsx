import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { CyberAgentClient } from "../api/client";
import type { AgentGraphView, FindingReportView } from "../api/types";
import { AgentGraphPanel, DelegationsPanel, FanoutPanel, FindingsPanel } from "./run-projections";

function provider(children: React.ReactNode) {
  return <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{children}</QueryClientProvider>;
}

describe("Run projection panels", () => {
  it("renders the bounded Agent graph and completion summary", async () => {
    const graph: AgentGraphView = {
      protocol_version: "agent_graph.v1",
      run_id: "run-1",
      root_agent_id: "agent-root",
      nodes: [{ id: "agent-root", session_id: "session-root", role: "root", profile: "review",
        skills: ["model.chat"], status: "running", depth: 0, child_limit: 2,
        turn_limit: 8, token_limit: 2000, turns_used: 2, tokens_used: 320,
        version: 2, created_at: "2026-07-13T00:00:00Z", updated_at: "2026-07-13T00:01:00Z" },
      { id: "agent-child", parent_id: "agent-root", session_id: "session-child", role: "specialist",
        profile: "review", skills: ["note_create"], status: "completed", depth: 1, child_limit: 0,
        turn_limit: 2, token_limit: 300, turns_used: 1, tokens_used: 90, version: 3,
        created_at: "2026-07-13T00:00:10Z", updated_at: "2026-07-13T00:01:20Z",
        completion: { id: "completion-1", attempt_id: "attempt-1", outcome: "succeeded",
          summary: "Reviewed the parser boundary", work_item_ids: [], note_ids: [],
          created_at: "2026-07-13T00:01:20Z" } }],
    };
    const client = { get: vi.fn().mockResolvedValue(graph) } as unknown as CyberAgentClient;
    render(provider(<AgentGraphPanel client={client} runID="run-1" />));
    expect(await screen.findByText("Reviewed the parser boundary")).toBeInTheDocument();
    expect(screen.getByText("agent-root")).toBeInTheDocument();
    expect(screen.getByText("agent-child")).toBeInTheDocument();
  });

  it("renders delegation, Fan-out, and Finding projections from read-only pages", async () => {
    const detail: FindingReportView = {
      report: { id: "report-1", run_id: "run-1", source_kind: "readonly_fanout_execution",
        source_id: "execution-1", status: "generated", title: "Security review",
        finding_count: 1, evidence_count: 1,
        severity: { info: 0, low: 0, medium: 0, high: 1, critical: 0 },
        created_at: "2026-07-13T00:02:00Z" },
      findings: [{ id: "finding-1", ordinal: 1, status: "validated", severity: "high",
        category: "path", title: "Boundary issue", detail: "A bounded Finding detail",
        relative_path: "src/main.go", line_start: 12, line_end: 12, confidence: 92,
        evidence: [], artifact_evidence: [], remediation_evidence: [],
        lifecycle: { status: "validated", validation_evidence_count: 1, remediation_evidence_count: 0 } }],
    };
    const getPage = vi.fn().mockImplementation((path: string) => {
      if (path.endsWith("/delegations")) return Promise.resolve({ items: [{ id: "delegation-1", run_id: "run-1",
        root_agent_id: "agent-root", status: "proposed", requested_by: "run_supervisor",
        created_at: "2026-07-13T00:00:00Z", assignments: [{ ordinal: 1, title: "Parser review",
          goal: "Inspect parser boundaries", skills: ["model.chat"], turn_limit: 2, token_limit: 300 }] }],
        page: { limit: 50 }, requestID: "req-delegations" });
      if (path.endsWith("/fanout-plans")) return Promise.resolve({ items: [{ id: "fanout-1", run_id: "run-1",
        workspace_id: "ws-1", scope_path: ".", goal: "Audit source modules", protocol_version: "readonly_fanout.v1",
        requested_tier: "4", effective_parallelism: 4, status: "planned", file_count: 12,
        total_bytes: 4096, excluded_count: 3, shard_count: 4, requested_by: "operator",
        created_at: "2026-07-13T00:01:00Z" }], page: { limit: 50 }, requestID: "req-fanout" });
      return Promise.resolve({ items: [detail.report], page: { limit: 50 }, requestID: "req-reports" });
    });
    const client = { getPage, get: vi.fn().mockResolvedValue(detail) } as unknown as CyberAgentClient;
    render(provider(<><DelegationsPanel client={client} runID="run-1" /><FanoutPanel client={client} runID="run-1" /><FindingsPanel client={client} runID="run-1" /></>));
    expect(await screen.findByText("Inspect parser boundaries")).toBeInTheDocument();
    expect(await screen.findByText("Audit source modules")).toBeInTheDocument();
    expect(await screen.findByText("Boundary issue")).toBeInTheDocument();
    expect(screen.getByText("A bounded Finding detail")).toBeInTheDocument();
  });
});
