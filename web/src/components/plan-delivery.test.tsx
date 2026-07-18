import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CyberAgentClient } from "../api/client";
import type { PlanDeliveryStateView, RunDetailView } from "../api/types";
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
    const { container } = renderWithQuery(<PlanDeliveryPanel state={state} />);
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

  it("keeps direction selection and Deliver as separate explicit controls", async () => {
    const user = userEvent.setup();
    const selectPlanDirection = vi.fn().mockResolvedValue({
      version: "plan_delivery_control.v1", run_id: "run-plan", proposal_id: "proposal-1",
      selection_id: "selection-1", note_id: "note-1", direction: 1, work_item_count: 1,
      replayed: false, phase_changed: false, execution_started: false, model_called: false,
      tool_called: false, capability_grant: false,
    });
    const enterPlanDelivery = vi.fn().mockResolvedValue({});
    const client = { hasPlanDelivery: true, selectPlanDirection,
      enterPlanDelivery } as unknown as CyberAgentClient;
    const detail = {
      run: { id: "run-plan", status: "paused" },
      mode: { phase: "plan" },
    } as unknown as RunDetailView;
    const choiceState: PlanDeliveryStateView = {
      operator_choice_needed: true, phase_change_needed: false, capability_grant: false,
      delivery_gate_enforced: true, required_checkpoints: 0, ready_checkpoints: 0,
      checkpoints: [],
      proposal: { id: "proposal-1", protocol_version: "plan_delivery.v1", status: "proposed",
        mode_revision: 4, directions, version: 1, created_at: "2026-07-13T00:00:00Z" },
    };
    const rendered = renderWithQuery(<PlanDeliveryPanel client={client} detail={detail}
      state={choiceState} />);
    await user.click(screen.getByRole("button", { name: "Choose direction 1" }));
    await waitFor(() => expect(selectPlanDirection).toHaveBeenCalledTimes(1));
    expect(selectPlanDirection.mock.calls[0]?.[0]).toBe("run-plan");
    expect(selectPlanDirection.mock.calls[0]?.[1]).toEqual({
      version: "plan_delivery_control.v1", proposal_id: "proposal-1", direction: 1,
    });
    expect(screen.queryByRole("button", { name: "Enter Deliver" })).not.toBeInTheDocument();

    rendered.rerender(<QueryClientProvider client={rendered.queryClient}>
      <PlanDeliveryPanel client={client} detail={detail} state={{ ...choiceState,
        operator_choice_needed: false, phase_change_needed: true,
        selection: { id: "selection-1", proposal_id: "proposal-1", direction_ordinal: 1,
          note_id: "note-1", items: [{ ordinal: 1, module_ordinal: 1, work_item_id: "work-1" }],
          version: 1, created_at: "2026-07-13T00:01:00Z" },
      }} />
    </QueryClientProvider>);
    await user.click(screen.getByRole("button", { name: "Enter Deliver" }));
    await waitFor(() => expect(enterPlanDelivery).toHaveBeenCalledTimes(1));
    expect(enterPlanDelivery.mock.calls[0]?.[0]).toBe("run-plan");
    expect(enterPlanDelivery.mock.calls[0]?.[1]).toEqual({
      version: "plan_delivery_control.v1",
    });
  });
});

function renderWithQuery(node: React.ReactNode) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false },
    mutations: { retry: false } } });
  return { ...render(<QueryClientProvider client={queryClient}>{node}</QueryClientProvider>),
    queryClient };
}
