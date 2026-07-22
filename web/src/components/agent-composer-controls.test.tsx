import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { CyberAgentClient } from "../api/client";
import { AgentComposerControls } from "./agent-composer-controls";

function modelClient(overrides: Partial<CyberAgentClient> = {}): CyberAgentClient {
  return {
    hasModelControl: true,
    modelAvailability: vi.fn().mockResolvedValue({
      protocol_version: "model_availability.v1",
      generation: 1,
      providers: [{ name: "mock", kind: "local", status: "available",
        models: ["mock-code", "mock-fast"], credential_source: "none",
        network_required: false, configuration_error: false }],
      routes: [{ name: "code", provider: "mock", model: "mock-code", available: true }],
    }),
    selectModelRoute: vi.fn().mockResolvedValue({
      name: "code", provider: "mock", model: "mock-fast", available: true,
    }),
    ...overrides,
  } as unknown as CyberAgentClient;
}

function renderControls(client: CyberAgentClient, props: Record<string, unknown> = {}) {
  const queryClient = new QueryClient({ defaultOptions: {
    queries: { retry: false }, mutations: { retry: false },
  } });
  return render(<QueryClientProvider client={queryClient}>
    <AgentComposerControls client={client} route="code" {...props} />
  </QueryClientProvider>);
}

describe("AgentComposerControls", () => {
  it("keeps model discovery lazy and exposes task modes, files, plugins, and context", async () => {
    const user = userEvent.setup();
    const client = modelClient();
    const onOpenFiles = vi.fn();
    const onOpenPlugins = vi.fn();
    const onPlanModeChange = vi.fn();
    const onTargetModeChange = vi.fn();
    renderControls(client, { contextTokens: 8192, onOpenFiles, onOpenPlugins,
      onPlanModeChange, onTargetModeChange });

    expect(client.modelAvailability).not.toHaveBeenCalled();
    expect(screen.getByRole("status", { name: "上下文已用 25%" })).toBeInTheDocument();
    expect(screen.getByText(/已加载约 8.2k 标记/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "添加" }));
    await user.click(screen.getByRole("menuitem", { name: /文件和文件夹/ }));
    expect(onOpenFiles).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "添加" }));
    await user.click(screen.getByRole("menuitem", { name: /目标/ }));
    expect(onTargetModeChange).toHaveBeenCalledWith(true);

    await user.click(screen.getByRole("button", { name: "添加" }));
    await user.click(screen.getByRole("menuitem", { name: /计划模式/ }));
    expect(onPlanModeChange).toHaveBeenCalledWith(true);

    await user.click(screen.getByRole("button", { name: "添加" }));
    await user.click(screen.getByRole("menuitem", { name: /已安装插件/ }));
    expect(onOpenPlugins).toHaveBeenCalledTimes(1);
  });

  it("loads models on demand, persists a selected route, and does not fake reasoning support", async () => {
    const user = userEvent.setup();
    const client = modelClient();
    renderControls(client);

    await user.click(screen.getByRole("button", { name: "选择模型，当前 code" }));
    const fast = await screen.findByRole("menuitemradio", { name: /mock-fast/ });
    expect(client.modelAvailability).toHaveBeenCalledTimes(1);
    await user.click(fast);
    await waitFor(() => expect(client.selectModelRoute).toHaveBeenCalledWith("code", {
      version: "model_route_control.v1", provider: "mock", model: "mock-fast",
    }));

    await user.click(screen.getByRole("button", { name: "推理强度，当前标准" }));
    expect(screen.getByText("高").closest("button")).toBeDisabled();
    expect(screen.getByText("最高").closest("button")).toBeDisabled();
  });
});
