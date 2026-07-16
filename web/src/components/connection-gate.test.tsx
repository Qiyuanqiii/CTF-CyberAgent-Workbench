import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useConnectionStore } from "../state/connection";
import { ConnectionGate } from "./connection-gate";

describe("ConnectionGate", () => {
  beforeEach(() => {
    useConnectionStore.getState().disconnect();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("validates the token through health and does not render it as text", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1",
      request_id: "req-health",
      data: { status: "ok", api_version: "api.v1", app_version: "test", schema_version: 37 },
    }), { status: 200, headers: { "Content-Type": "application/json" } })));
    const user = userEvent.setup();
    render(<QueryClientProvider client={new QueryClient()}><ConnectionGate /></QueryClientProvider>);

    const input = screen.getByLabelText("Read bearer token");
    const controlInput = screen.getByLabelText(/Control bearer token/);
    expect(input).toHaveAttribute("type", "password");
    expect(controlInput).toHaveAttribute("type", "password");
    await user.type(input, "ephemeral-token");
    await user.type(controlInput, "ephemeral-control-token");
    expect(screen.queryByText("ephemeral-token")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "连接" }));

    expect(useConnectionStore.getState().token).toBe("ephemeral-token");
    expect(useConnectionStore.getState().controlToken).toBe("ephemeral-control-token");
    expect(input).toHaveValue("");
    expect(controlInput).toHaveValue("");
  });
});
