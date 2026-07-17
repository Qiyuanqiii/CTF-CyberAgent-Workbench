import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { CyberAgentClient } from "../api/client";
import type { AgentGraphView, ExternalSkillProjectionView, FindingReportView } from "../api/types";
import { AgentGraphPanel, DelegationsPanel, ExternalSkillsPanel, FanoutPanel, FindingsPanel } from "./run-projections";

function provider(children: React.ReactNode) {
  return <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>{children}</QueryClientProvider>;
}

describe("Run projection panels", () => {
  it("renders bounded external Skill metadata without a mutation control", () => {
    const projection: ExternalSkillProjectionView = {
      protocol_version: "external_skill_projection.v1",
      run_id: "run-1",
      mode_revision: 2,
      surface: "code",
      profile: "review",
      token_budget: 1024,
      token_upper_bound: 180,
      item_count: 1,
      operator_confirmed: true,
      context_delivery_authorized: true,
      tool_capability_grant: false,
      root_delivery: { prepared: 2, committed: 1 },
      specialist_delivery: { prepared: 1, committed: 1 },
      items: [{ ordinal: 1, name: "safe-review", version: "1.0.0",
        token_upper_bound: 180, trust_class: "operator_installed_untrusted",
        declared_tool_count: 2, specialist_eligible: true }],
      created_at: "2026-07-17T00:00:00Z",
    };
    const { container } = render(<ExternalSkillsPanel projection={projection} />);
    expect(screen.getByText("safe-review@1.0.0")).toBeInTheDocument();
    expect(screen.getByText("operator_installed_untrusted")).toBeInTheDocument();
    expect(screen.getByText("1 / 2")).toBeInTheDocument();
    expect(screen.getByText("Confirmed")).toBeInTheDocument();
    expect(screen.getByText("Authorized")).toBeInTheDocument();
    expect(screen.getByText("Closed")).toBeInTheDocument();
    expect(container.querySelector("button")).toBeNull();
    expect(container.textContent).not.toContain("SKILL.md");
    expect(container.textContent).not.toContain("sha256/");
  });

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
