import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { CyberAgentClient } from "../api/client";
import { CodeHandoffPanel } from "./code-handoff-panel";

describe("CodeHandoffPanel", () => {
  it("renders durable summaries and opaque references without private bodies", async () => {
    const codeHandoff = vi.fn().mockResolvedValue({
      protocol_version: "code_handoff.v1", run_id: "run-1", mission_id: "mission-1",
      session_id: "session-1", workspace_id: "workspace-1", run_status: "paused",
      surface: "code", phase: "deliver", mode_revision: 2,
      generated_at: "2026-07-19T12:00:00Z",
      plan: { state: "selected", proposal_id: "proposal-1", selection_id: "selection-1",
        direction_count: 3, selected_direction: 2, module_count: 3, pending_count: 1,
        in_progress_count: 0, blocked_count: 0, completed_count: 2, cancelled_count: 0 },
      queue: { pending: 1, prepared: 0, committed: 2, cancelled: 0 },
      change_set: { proposed: 1, approved: 0, applied: 2, denied: 0, failed: 0,
        returned_count: 3, total_diff_bytes: 1024, truncated: false },
      verification: { pass_count: 1, fail_count: 0, unknown_count: 0,
        returned_count: 1, truncated: false,
        references: [{ id: "verification-1", outcome: "pass", redacted: false,
          recorded_at: "2026-07-19T11:00:00Z" }] },
      pending_action_count: 1, pending_actions_truncated: false,
      pending_actions: [{ id: "action-opaque-1", kind: "file_edit_review", state: "proposed",
        destination: "diffs", available_at: "2026-07-19T11:30:00Z" }],
      report_references_truncated: false,
      report_references: [{ id: "report-1", status: "generated", finding_count: 2,
        created_at: "2026-07-19T11:45:00Z" }],
      regenerable: true, durable_sources: true, private_bodies_included: false,
      composite_mutation: false, resume_authorized: false, execution_started: false,
    });
    const client = { codeHandoff } as unknown as CyberAgentClient;
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(<QueryClientProvider client={queryClient}>
      <CodeHandoffPanel client={client} runID="run-1" />
    </QueryClientProvider>);
    expect(await screen.findByText("selected")).toBeInTheDocument();
    expect(screen.getByText("file edit review")).toBeInTheDocument();
    expect(screen.getByText("2 findings")).toBeInTheDocument();
    expect(screen.queryByText(/proposal body|verification summary|command/i)).not.toBeInTheDocument();
  });
});
