import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useConnectionStore } from "../state/connection";
import { ConnectionGate } from "./connection-gate";

describe("ConnectionGate", () => {
  beforeEach(() => {
    useConnectionStore.getState().disconnect();
    delete window.go;
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

  it("auto-connects a closed-authority Desktop bootstrap without rendering its token", async () => {
    const bootstrap = vi.fn().mockResolvedValue({
      protocol_version: "desktop_connection_bootstrap.v1",
      api_base_url: "/api/v1",
      api_version: "api.v1",
      app_version: "v0.1.0",
      ui_digest: "a".repeat(64),
      read_token: "desktop-read-token-0123456789abcdef",
      control_token: "",
      control_enabled: false,
      run_creation_enabled: false,
      session_message_enabled: false,
      session_steering_control_enabled: false,
      run_lifecycle_enabled: false,
      run_execution_enabled: false,
      plan_delivery_control_enabled: false,
      approval_control_enabled: false,
      model_control_enabled: false,
      provider_credential_enabled: false,
      file_edit_review_enabled: false,
      file_edit_proposal_enabled: false,
      run_wake_control_enabled: false,
      file_edit_apply_enabled: false,
      run_wake_execution_enabled: false,
      run_wake_worker_enabled: false,
      read_only_default: true,
      process_execution_enabled: false,
      shell_execution_enabled: false,
      docker_execution_enabled: false,
      skill_installation_enabled: false,
      evidence_attachment_enabled: false,
      renderer_path_input_supported: false,
    });
    window.go = { desktop: { DesktopBridge: {
      Bootstrap: bootstrap,
      InstallSkillPackage: vi.fn(),
      SelectSkillPackage: vi.fn(),
      PreviewSkillPackage: vi.fn(),
    } } };
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      version: "api.v1",
      request_id: "req-desktop-health",
      data: { status: "ok", api_version: "api.v1", app_version: "test", schema_version: 71 },
    }), { status: 200, headers: { "Content-Type": "application/json" } })));

    render(<QueryClientProvider client={new QueryClient()}><ConnectionGate /></QueryClientProvider>);
    await waitFor(() => expect(useConnectionStore.getState().token)
      .toBe("desktop-read-token-0123456789abcdef"));
    expect(useConnectionStore.getState().controlToken).toBe("");
    expect(screen.queryByText("desktop-read-token-0123456789abcdef")).not.toBeInTheDocument();
    expect(bootstrap).toHaveBeenCalledTimes(1);
  });
});
