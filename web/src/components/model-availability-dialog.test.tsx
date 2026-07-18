import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import type { CyberAgentClient } from "../api/client";
import { ModelAvailabilityDialog } from "./model-availability-dialog";

describe("ModelAvailabilityDialog", () => {
  it("renders redacted provider and route status without configuration secrets", async () => {
    const client = { modelAvailability: vi.fn().mockResolvedValue({
      protocol_version: "model_availability.v1",
      providers: [{ name: "mock", kind: "local", status: "available", models: ["mock-code"],
        credential_source: "none", network_required: false, configuration_error: false }],
      routes: [{ name: "code", provider: "mock", model: "mock-code", available: true }],
    }) } as unknown as CyberAgentClient;
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { container } = render(<QueryClientProvider client={queryClient}>
      <ModelAvailabilityDialog client={client} open onClose={vi.fn()} />
    </QueryClientProvider>);
    expect(await screen.findByText("mock-code")).toBeInTheDocument();
    expect(screen.getByText("code")).toBeInTheDocument();
    expect(container.textContent).not.toContain("api_key");
    expect(container.textContent).not.toContain("base_url");
  });
});
