import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi } from "vitest";
import type { CyberAgentClient } from "../api/client";
import { VerificationPlan } from "./verification-plan";

const emptyCoverage = () => ({
  protocol_version: "operator_verification_plan_coverage.v1", run_id: "run-1",
  session_id: "session-1", workspace_id: "workspace-1", plans: [], plan_count: 0,
  plan_item_count: 0, observed_plan_item_count: 0, associated_evidence_count: 0,
  associations: [], plans_truncated: false, associations_truncated: false,
  metadata_only: true, read_only: true, result_inferred: false, command_executed: false,
  model_assertion: false, record_rewritten: false, approval: false, authority_granted: false,
});

describe("VerificationPlan", () => {
  it("records a bounded operator checklist without a result field", async () => {
    const verificationPlans = vi.fn().mockResolvedValue({
      protocol_version: "operator_verification_plan_inventory.v1", run_id: "run-1",
      session_id: "session-1", workspace_id: "workspace-1", items: [], truncated: false,
    });
    const verificationPlanCoverage = vi.fn().mockResolvedValue(emptyCoverage());
    const recordVerificationPlan = vi.fn().mockResolvedValue({ id: "verification-plan-1" });
    const client = { hasVerificationEvidence: true, verificationPlans,
      verificationPlanCoverage, recordVerificationPlan } as unknown as CyberAgentClient;
    const user = userEvent.setup();
    render(<QueryClientProvider client={new QueryClient()}>
      <VerificationPlan client={client} runID="run-1" />
    </QueryClientProvider>);
    await screen.findByText("No verification plan recorded");
    await user.type(screen.getByLabelText("Title"), "Release checks");
    await user.type(screen.getByLabelText("Purpose"), "Operator guidance only");
    await user.type(screen.getByLabelText("Check 1 title"), "Focused tests");
    await user.type(screen.getByLabelText("Check 1 expected observation"), "Observe a pass");
    await user.click(screen.getByRole("button", { name: "Record plan" }));
    await waitFor(() => expect(recordVerificationPlan).toHaveBeenCalledTimes(1));
    const [runID, body, key] = recordVerificationPlan.mock.calls[0];
    expect(runID).toBe("run-1");
    expect(body).toEqual({ version: "operator_verification_plan.v1",
      title: "Release checks", summary: "Operator guidance only",
      items: [{ title: "Focused tests", expected_observation: "Observe a pass" }] });
    expect(body).not.toHaveProperty("outcome");
    expect(key).toMatch(/^web-verification-plan-/u);
  });

  it("reuses an uncertain retry key only while the plan intent is unchanged", async () => {
    const verificationPlans = vi.fn().mockResolvedValue({
      protocol_version: "operator_verification_plan_inventory.v1", run_id: "run-1",
      session_id: "session-1", workspace_id: "workspace-1", items: [], truncated: false,
    });
    const verificationPlanCoverage = vi.fn().mockResolvedValue(emptyCoverage());
    const recordVerificationPlan = vi.fn()
      .mockRejectedValueOnce(new Error("uncertain transport failure"))
      .mockRejectedValueOnce(new Error("uncertain transport failure"))
      .mockResolvedValueOnce({ id: "verification-plan-1" });
    const client = { hasVerificationEvidence: true, verificationPlans,
      verificationPlanCoverage, recordVerificationPlan } as unknown as CyberAgentClient;
    const user = userEvent.setup();
    render(<QueryClientProvider client={new QueryClient()}>
      <VerificationPlan client={client} runID="run-1" />
    </QueryClientProvider>);
    await screen.findByText("No verification plan recorded");
    await user.type(screen.getByLabelText("Title"), "Release checks");
    await user.type(screen.getByLabelText("Purpose"), "Operator guidance only");
    await user.type(screen.getByLabelText("Check 1 title"), "Focused tests");
    await user.type(screen.getByLabelText("Check 1 expected observation"), "Observe a pass");
    const submit = screen.getByRole("button", { name: "Record plan" });
    await user.click(submit);
    await waitFor(() => expect(recordVerificationPlan).toHaveBeenCalledTimes(1));
    await user.click(submit);
    await waitFor(() => expect(recordVerificationPlan).toHaveBeenCalledTimes(2));
    expect(recordVerificationPlan.mock.calls[1]?.[2])
      .toBe(recordVerificationPlan.mock.calls[0]?.[2]);
    await user.type(screen.getByLabelText("Title"), " updated");
    await user.click(submit);
    await waitFor(() => expect(recordVerificationPlan).toHaveBeenCalledTimes(3));
    expect(recordVerificationPlan.mock.calls[2]?.[2])
      .not.toBe(recordVerificationPlan.mock.calls[0]?.[2]);
  });

  it("shows contradictory explicit observations without inferring an overall result", async () => {
    const verificationPlans = vi.fn().mockResolvedValue({
      protocol_version: "operator_verification_plan_inventory.v1", run_id: "run-1",
      session_id: "session-1", workspace_id: "workspace-1", truncated: false,
      items: [{ id: "plan-1", title: "Release checks", summary: "Operator guidance",
        created_at: "2026-07-20T01:00:00Z", items: [{ ordinal: 1,
          title: "Focused tests", expected_observation: "Observe explicit results" }] }],
    });
    const verificationPlanCoverage = vi.fn().mockResolvedValue({
      ...emptyCoverage(), plan_count: 1, plan_item_count: 1, observed_plan_item_count: 1,
      associated_evidence_count: 2, plans: [{ plan_id: "plan-1", plan_sha256: "a".repeat(64),
        item_count: 1, observed_item_count: 1, associated_evidence_count: 2,
        items: [{ ordinal: 1, item_sha256: "b".repeat(64), associated_evidence_count: 2,
          pass_count: 1, fail_count: 1, unknown_count: 0,
          latest_association_event_sequence: 8 }] }],
    });
    const client = { hasVerificationEvidence: true, verificationPlans,
      verificationPlanCoverage, recordVerificationPlan: vi.fn() } as unknown as CyberAgentClient;
    render(<QueryClientProvider client={new QueryClient()}>
      <VerificationPlan client={client} runID="run-1" />
    </QueryClientProvider>);
    expect(await screen.findByText("1 pass")).toBeInTheDocument();
    expect(screen.getByText("1 fail")).toBeInTheDocument();
    expect(screen.queryByText("overall pass", { exact: false })).not.toBeInTheDocument();
  });
});
