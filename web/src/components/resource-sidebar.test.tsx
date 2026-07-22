import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { CyberAgentClient } from "../api/client";
import type { RunView, SessionView } from "../api/types";
import { useConnectionStore } from "../state/connection";
import { ResourceSidebar } from "./resource-sidebar";

function run(id: string, status: RunView["status"]): RunView {
  return {
    id,
    mission_id: `mission-${id}`,
    session_id: `session-${id}`,
    status,
    config: { model_route: "code", interactive: false },
    budget: { max_turns: 8 },
    created_at: "2026-07-13T00:00:00Z",
    updated_at: "2026-07-13T00:01:00Z",
  };
}

function session(id: string, title: string): SessionView {
  return {
    id,
    route: "code",
    status: "active",
    title,
    workspace_id: "workspace-demo",
    created_at: "2026-07-13T00:00:00Z",
    updated_at: "2026-07-13T00:01:00Z",
  };
}

describe("ResourceSidebar", () => {
  beforeEach(() => {
    useConnectionStore.getState().disconnect();
  });

  it("renders lifecycle states and appends the next opaque cursor page", async () => {
    const firstPage = [
      run("run-paused", "paused"),
      run("run-completed", "completed"),
      run("run-failed", "failed"),
      run("run-cancelled", "cancelled"),
    ];
    const secondPage = [run("run-running", "running")];
    const getPage = vi.fn().mockImplementation((path: string, _query: unknown, cursor: string) => {
      if (path === "/sessions") {
        return Promise.resolve({
          items: [session("session-alpha", "修复登录回归")],
          page: { limit: 50 },
          requestID: "req-sessions-1",
        });
      }
      if (path !== "/runs") throw new Error(`unexpected path ${path}`);
      if (cursor === "cursor-terminal-page") {
        return Promise.resolve({
          items: secondPage,
          page: { limit: 50 },
          requestID: "req-runs-2",
        });
      }
      return Promise.resolve({
        items: firstPage,
        page: { limit: 50, next_cursor: "cursor-terminal-page" },
        requestID: "req-runs-1",
      });
    });
    const client = { getPage } as unknown as CyberAgentClient;
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    const onCreateRun = vi.fn();
    const onNavigate = vi.fn();
    const onOpenSettings = vi.fn();
    const { container } = render(
      <QueryClientProvider client={queryClient}>
        <ResourceSidebar activeSection="conversation" client={client}
          onCreateRun={onCreateRun} onNavigate={onNavigate} onOpenSettings={onOpenSettings} />
      </QueryClientProvider>,
    );

    await waitFor(() => {
      for (const status of ["paused", "completed", "failed", "cancelled"]) {
        expect(container.querySelector(`.history-status.status-${status}`)).toBeInTheDocument();
      }
    });
    const loadMore = Array.from(container.querySelectorAll<HTMLButtonElement>("button.load-more"))
      .find((button) => button.textContent?.includes("加载更多"));
    expect(loadMore).not.toBeNull();
    await act(async () => {
      fireEvent.click(loadMore!);
    });

    await waitFor(() => expect(container.querySelector(".history-status.status-running"))
      .toBeInTheDocument());
    await waitFor(() => expect(getPage.mock.calls.some((call) =>
      call[0] === "/runs" && call[2] === "cursor-terminal-page")).toBe(true));
    expect(useConnectionStore.getState().selectedRunID).toBe("run-paused");
    expect(screen.getByAltText("Prayu")).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "工作台导航" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "模型切换" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "自动定时" })).toBeInTheDocument();
    expect(screen.getByText("修复登录回归")).toBeInTheDocument();
    expect(container.querySelector(".resource-row.selected strong"))
      .toHaveTextContent("任务");

    fireEvent.click(screen.getByRole("button", { name: /新建任务/ }));
    fireEvent.click(screen.getByRole("button", { name: "模型切换" }));
    fireEvent.click(screen.getByRole("button", { name: /本地操作者/ }));
    expect(onCreateRun).toHaveBeenCalledTimes(1);
    expect(onNavigate).toHaveBeenCalledWith("models");
    expect(onOpenSettings).toHaveBeenCalledTimes(1);
  });
});
