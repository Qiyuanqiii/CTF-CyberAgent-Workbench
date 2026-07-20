import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { RunDetailView } from "../api/types";
import { CodeJourney, type CodeJourneyDestination } from "./code-journey";
import type { ReceiptReviewFacts } from "./receipt-review-navigation";

describe("CodeJourney", () => {
  it("navigates one Code journey through existing independently controlled surfaces", async () => {
    const user = userEvent.setup();
    const onNavigate = vi.fn<(destination: CodeJourneyDestination) => void>();
    const onOpenReceiptReview = vi.fn();
    render(<CodeJourney detail={detail()} receiptReviewFacts={receiptReviewFacts()}
      onNavigate={onNavigate} onOpenReceiptReview={onOpenReceiptReview} />);

    expect(screen.getByText("selected")).toBeInTheDocument();
    expect(screen.getByText("2 queued")).toBeInTheDocument();
    expect(screen.getByText("Independent mutations")).toBeInTheDocument();
    expect(screen.getByText("2 confirmed / 2 disputed")).toBeInTheDocument();
    expect(screen.getByText("Showing 3 of 4")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /Open receipt review/u })).toHaveLength(3);
    await user.click(screen.getByRole("button", {
      name: "Open receipt review review-1 in Verify",
    }));
    expect(onOpenReceiptReview).toHaveBeenCalledWith(expect.objectContaining({
      id: "review-1", receipt_id: "receipt-1", review_event_sequence: 21,
    }));
    await user.click(screen.getByRole("button", { name: "Open Scope" }));
    await user.click(screen.getByRole("button", { name: "Open Review" }));
    await user.click(screen.getByRole("button", { name: "Open diffs" }));
    expect(onNavigate.mock.calls.map(([destination]) => destination))
      .toEqual(["repository", "actions", "diffs"]);
  });

  it("keeps source truncation visible when all three displayed facts fit", () => {
    const facts = receiptReviewFacts();
    facts.references = facts.references.slice(0, 3);
    facts.returned_count = 3;
    render(<CodeJourney detail={detail()} receiptReviewFacts={facts}
      onNavigate={vi.fn()} onOpenReceiptReview={vi.fn()} />);

    expect(screen.getByText("Showing 3 of 3")).toBeInTheDocument();
    expect(screen.getByText("source truncated")).toBeInTheDocument();
  });
});

function receiptReviewFacts(): ReceiptReviewFacts {
  return {
    protocol_version: "operator_verification_plan_item_snapshot_receipt_review_inventory.v1",
    metadata_confirmed_count: 2, metadata_disputed_count: 2, returned_count: 4,
    truncated: true, metadata_only: true, read_only: true, review_non_authorizing: true,
    content_included: false, private_bodies_included: false,
    operator_identity_included: false, snapshot_accepted: false, result_accepted: false,
    result_inferred: false, record_rewritten: false, approval: false,
    authority_granted: false, execution_started: false,
    references: Array.from({ length: 4 }, (_, index) => ({
      id: `review-${index + 1}`, receipt_id: `receipt-${index + 1}`,
      receipt_content_sha256: String(index + 1).repeat(64),
      receipt_event_sequence: 10 + index, decision: index % 2 === 0 ?
        "metadata_confirmed" as const : "metadata_disputed" as const,
      review_event_sequence: 21 + index,
      reviewed_at: `2026-07-20T01:0${index}:00Z`,
    })),
  };
}

function detail(): RunDetailView {
  return {
    mission: { id: "mission-1", goal: "deliver a safe change", profile: "code",
      workspace_id: "workspace-1", scope: { network_mode: "disabled" },
      created_at: "2026-07-19T00:00:00Z", updated_at: "2026-07-19T00:00:00Z" },
    run: { id: "run-1", mission_id: "mission-1", session_id: "session-1",
      status: "running", budget: { max_turns: 8, max_tokens: 1000, max_tool_calls: 8,
        max_wall_time_seconds: 600 }, config: { interactive: true, model_route: "code" },
      created_at: "2026-07-19T00:00:00Z", updated_at: "2026-07-19T00:00:00Z" },
    mode: { protocol_version: "run_mode.v1", surface: "code", phase: "plan",
      profile: "code", policy_version: "mode_policy.v1", revision: 1,
      requested_by: "operator", reason: "test", capability_grant: false,
      scope: { network_mode: "disabled" }, created_at: "2026-07-19T00:00:00Z" },
    operator_steering: { pending: 1, prepared: 1, committed: 0, cancelled: 0,
      messages: [] },
    plan_delivery: { operator_choice_needed: false, phase_change_needed: true,
      delivery_gate_enforced: true, ready_checkpoints: 0, required_checkpoints: 1,
      capability_grant: false, checkpoints: [], selection: { id: "selection-1",
        proposal_id: "proposal-1", version: 1, direction_ordinal: 2, items: [],
        note_id: "note-1", created_at: "2026-07-19T00:00:00Z" } },
    execution_profile: { protocol_version: "run_execution_profile.v1", profile: "preview",
      backend: "noop", approval_policy: "none", filesystem_scope: "none",
      network_scope: "disabled", risk_tier: "minimal", required_gate: "none",
      policy_version: "execution_profile_policy.v1", capability_grant: false,
      execution_authorized: false, process_enabled: false, revision: 1,
      requested_by: "system", reason: "default", created_at: "2026-07-19T00:00:00Z" },
    tool_usage: { consumed: 0, remaining: 8, limit: 8 },
  } as RunDetailView;
}
