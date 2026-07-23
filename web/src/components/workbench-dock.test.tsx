import { fireEvent, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { CyberAgentClient } from "../api/client";
import { WorkbenchDock } from "./workbench-dock";

describe("WorkbenchDock", () => {
  it("keeps summary, bottom panel, and sidecar independently controllable", () => {
    renderDock();

    expect(screen.getByText("conversation body")).toBeInTheDocument();
    const summary = screen.getByRole("button", { name: "切换摘要" });
    const bottom = screen.getByRole("button", { name: "切换底部面板显示" });
    const sidecar = screen.getByRole("button", { name: "显示或隐藏右侧栏" });
    expect(summary).toHaveAttribute("aria-pressed", "false");
    expect(bottom).toHaveAttribute("aria-pressed", "false");
    expect(sidecar).toHaveAttribute("aria-pressed", "false");

    fireEvent.click(summary);
    expect(screen.getByRole("complementary", { name: "摘要" })).toBeInTheDocument();
    expect(summary).toHaveAttribute("aria-pressed", "true");

    fireEvent.click(bottom);
    expect(screen.getByRole("region", { name: "底部面板" })).toBeInTheDocument();
    expect(screen.getByText("终端尚未启用")).toBeInTheDocument();

    fireEvent.click(sidecar);
    expect(screen.getByRole("complementary", { name: "右侧工具栏" })).toBeInTheDocument();
    expect(screen.getByText("当前任务未绑定 Workspace")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "关闭右侧栏" }));
    expect(screen.queryByRole("complementary", { name: "右侧工具栏" })).not.toBeInTheDocument();
    expect(screen.getByRole("complementary", { name: "摘要" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "底部面板" })).toBeInTheDocument();
  });

  it("matches the tool shortcuts and add-menu behavior without starting tools", () => {
    renderDock();

    fireEvent.keyDown(window, { key: "g", ctrlKey: true, shiftKey: true });
    expect(screen.getByText("审阅")).toBeInTheDocument();
    expect(screen.getByText("当前任务未绑定 Workspace")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "添加右侧工具" }));
    fireEvent.click(screen.getByRole("menuitemradio", { name: /浏览器/ }));
    expect(screen.getByText("浏览器尚未启动")).toBeInTheDocument();
    expect(screen.getByText("浏览器运行时仍处于安全审查阶段")).toBeInTheDocument();

    fireEvent.keyDown(window, { key: "p", ctrlKey: true });
    expect(screen.getByText("文件")).toBeInTheDocument();
    expect(screen.getByText("No Workspace is bound to this Run")).toBeInTheDocument();

    fireEvent.keyDown(window, { key: "j", ctrlKey: true });
    expect(screen.getByRole("region", { name: "底部面板" })).toBeInTheDocument();
    fireEvent.keyDown(window, { key: "j", ctrlKey: true });
    expect(screen.queryByRole("region", { name: "底部面板" })).not.toBeInTheDocument();
  });

  it("does not offer native Workspace opening in an ordinary browser", () => {
    renderDock();
    expect(screen.getByRole("button", { name: "打开工作区" })).toBeDisabled();
  });
});

function renderDock() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const client = {} as CyberAgentClient;
  return render(
    <QueryClientProvider client={queryClient}>
      <WorkbenchDock client={client} desktop={false} resourceKind="run"
        runID="" sessionID="" title="Prayu 工作台">
        <div>conversation body</div>
      </WorkbenchDock>
    </QueryClientProvider>,
  );
}
