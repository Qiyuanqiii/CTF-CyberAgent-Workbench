import { render, screen } from "@testing-library/react";
import type { PlanDeliveryStateView } from "../api/types";
import { PlanDeliveryPanel } from "./run-workspace";

const directions: NonNullable<PlanDeliveryStateView["proposal"]>["directions"] = [
  { ordinal: 1, title: "Conservative", summary: "Keep changes narrow.", tradeoffs: ["More sequential work"],
    modules: [{ ordinal: 1, title: "Inspect", objective: "Inspect current boundaries.",
      acceptance_criteria: ["Boundaries recorded"], dependencies: [] }] },
  { ordinal: 2, title: "Balanced", summary: "Deliver a vertical slice.", tradeoffs: ["Moderate breadth"],
    modules: [{ ordinal: 1, title: "Implement", objective: "Implement the core path.",
      acceptance_criteria: ["Focused tests pass"], dependencies: [] }] },
  { ordinal: 3, title: "Accelerated", summary: "Prepare independent slices.", tradeoffs: ["Higher review load"],
    modules: [{ ordinal: 1, title: "Prepare", objective: "Prepare independent work.",
      acceptance_criteria: ["Work stays bounded"], dependencies: [] }] },
];

describe("PlanDeliveryPanel", () => {
  it("renders the selected direction as a read-only projection", () => {
    const state: PlanDeliveryStateView = {
      operator_choice_needed: false,
      phase_change_needed: true,
      capability_grant: false,
      delivery_gate_enforced: true,
      required_checkpoints: 1,
      ready_checkpoints: 1,
      checkpoints: [{ id: "checkpoint-1", work_item_id: "work-1", module_ordinal: 1,
        module_count: 1, mode_revision: 5, work_item_version: 2,
        full_gate_required: true, handoff_note_id: "note-handoff",
        gate_ready: true, created_at: "2026-07-13T00:02:00Z" }],
      proposal: { id: "proposal-1", protocol_version: "plan_delivery.v1", status: "proposed",
        mode_revision: 4, directions, version: 1, created_at: "2026-07-13T00:00:00Z" },
      selection: { id: "selection-1", proposal_id: "proposal-1", direction_ordinal: 2,
        note_id: "note-1", items: [{ ordinal: 1, module_ordinal: 1, work_item_id: "work-1" }],
        version: 1, created_at: "2026-07-13T00:01:00Z" },
    };
    const { container } = render(<PlanDeliveryPanel state={state} />);
    expect(screen.getByText("Deliver phase required")).toBeInTheDocument();
    expect(screen.getByText("Capability grant: no")).toBeInTheDocument();
    expect(screen.getByText("Delivery gates 1 / 1")).toBeInTheDocument();
    expect(screen.getByText("Checkpoint history")).toBeInTheDocument();
    expect(screen.getByText("full gate")).toBeInTheDocument();
    expect(screen.getByText("Balanced")).toBeInTheDocument();
    expect(screen.getByText("Implement the core path.")).toBeInTheDocument();
    expect(container.querySelector("details.selected")?.getAttribute("open")).not.toBeNull();
    expect(container.querySelector("button")).toBeNull();
  });
});
