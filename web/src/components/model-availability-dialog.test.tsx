import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

  it("keeps diagnostics and route selection behind explicit model control", async () => {
    const user = userEvent.setup();
    const diagnoseProvider = vi.fn().mockResolvedValue({
      protocol_version: "provider_diagnostic.v1", provider: "mock", model: "mock-code",
      status: "reachable", outcome: "success", retryable: false,
      network_request_attempted: false, model_called: true, tool_called: false,
      response_content_returned: false, duration_ms: 2,
    });
    const selectModelRoute = vi.fn().mockResolvedValue({
      name: "code", provider: "mock", model: "mock-fast", available: true,
    });
    const client = {
      hasModelControl: true, diagnoseProvider, selectModelRoute,
      modelAvailability: vi.fn().mockResolvedValue({
        protocol_version: "model_availability.v1",
        providers: [{ name: "mock", kind: "local", status: "available",
          models: ["mock-code", "mock-fast"], credential_source: "none",
          network_required: false, configuration_error: false }],
        routes: [{ name: "code", provider: "mock", model: "mock-code", available: true }],
      }),
    } as unknown as CyberAgentClient;
    const queryClient = new QueryClient({ defaultOptions: {
      queries: { retry: false }, mutations: { retry: false },
    } });
    render(<QueryClientProvider client={queryClient}>
      <ModelAvailabilityDialog client={client} open onClose={vi.fn()} />
    </QueryClientProvider>);
    await user.click(await screen.findByRole("button", { name: "Diagnose mock" }));
    await waitFor(() => expect(diagnoseProvider).toHaveBeenCalledWith({
      version: "provider_diagnostic.v1", provider: "mock", model: "mock-code",
      confirm_diagnostic: true,
    }));
    expect(await screen.findByText("reachable")).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("code model route"), "mock/mock-fast");
    await user.click(screen.getByRole("button", { name: "Save code route" }));
    await waitFor(() => expect(selectModelRoute).toHaveBeenCalledWith("code", {
      version: "model_route_control.v1", provider: "mock", model: "mock-fast",
    }));
  });
});
