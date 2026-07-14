import { render, screen } from "@testing-library/react";
import type { OperatorSteeringQueueView } from "../api/types";
import { OperatorSteeringPanel } from "./run-workspace";

describe("OperatorSteeringPanel", () => {
  it("renders bounded metadata without a mutation control", () => {
    const state: OperatorSteeringQueueView = {
      pending: 1,
      prepared: 1,
      committed: 2,
      cancelled: 0,
      messages: [
        { id: "steer-20260714000000-000000000001", sequence: 3, status: "pending",
          created_at: "2026-07-14T00:00:00Z" },
        { id: "steer-20260714000000-000000000002", sequence: 2, status: "committed",
          created_at: "2026-07-13T23:59:00Z", committed_at: "2026-07-14T00:01:00Z" },
      ],
    };
    const { container } = render(<OperatorSteeringPanel state={state} />);
    expect(screen.getByText("Queued 1")).toBeInTheDocument();
    expect(screen.getByText("Prepared 1")).toBeInTheDocument();
    expect(screen.getByText("Committed 2")).toBeInTheDocument();
    expect(screen.getByText("#3")).toBeInTheDocument();
    expect(container.querySelector("button")).toBeNull();
    expect(container.textContent).not.toContain("content_sha256");
    expect(container.textContent).not.toContain("requested_by");
  });
});
