import { fireEvent, render, screen } from "@testing-library/react";
import { SettingsView, type SettingsCapability } from "./settings-view";

const capabilities: SettingsCapability[] = [
  { id: "run-control", label: "执行档位", enabled: true },
  { id: "plan-delivery", label: "计划交付", enabled: true },
  { id: "wake-worker", label: "Wake Worker", enabled: false },
];

const health = {
  status: "ok" as const,
  api_version: "api.v1" as const,
  app_version: "test",
  schema_version: 84,
};

describe("SettingsView", () => {
  beforeEach(() => {
    window.localStorage.clear();
    delete document.documentElement.dataset.prayuDensity;
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("projects real runtime facts and moves the selected brush state with navigation", () => {
    render(<SettingsView capabilities={capabilities} desktop health={health}
      onBack={vi.fn()} onOpenModels={vi.fn()} onOpenSkills={vi.fn()} />);

    expect(screen.getByRole("heading", { name: "Prayu" })).toBeInTheDocument();
    expect(screen.getByText("v84")).toBeInTheDocument();
    expect(screen.getByText("2/3")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "个人资料" })).toHaveClass("active");

    fireEvent.click(screen.getByRole("button", { name: "常规" }));
    expect(screen.getByRole("button", { name: "常规" })).toHaveClass("active");
    expect(screen.getByRole("button", { name: "个人资料" })).not.toHaveClass("active");
    expect(screen.getByRole("heading", { name: "常规" })).toBeInTheDocument();
  });

  it("keeps display density local and leaves model and Skill actions explicit", () => {
    const onOpenModels = vi.fn();
    const onOpenSkills = vi.fn();
    render(<SettingsView capabilities={capabilities} desktop health={health}
      onBack={vi.fn()} onOpenModels={onOpenModels} onOpenSkills={onOpenSkills} />);

    fireEvent.click(screen.getByRole("button", { name: "外观" }));
    fireEvent.click(screen.getByRole("button", { name: "紧凑" }));
    expect(document.documentElement.dataset.prayuDensity).toBe("compact");
    expect(window.localStorage.getItem("prayu.ui-density")).toBe("compact");

    fireEvent.click(screen.getByRole("button", { name: "模型与配置" }));
    fireEvent.click(screen.getByRole("button", { name: "Skill 包" }));
    expect(onOpenModels).toHaveBeenCalledTimes(1);
    expect(onOpenSkills).toHaveBeenCalledTimes(1);
  });

  it("falls back safely when browser storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("storage disabled", "SecurityError");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("storage disabled", "SecurityError");
    });

    expect(() => render(<SettingsView capabilities={capabilities} desktop health={health}
      onBack={vi.fn()} onOpenModels={vi.fn()} onOpenSkills={vi.fn()} />)).not.toThrow();
    fireEvent.click(screen.getByRole("button", { name: "外观" }));
    expect(screen.getByRole("button", { name: "舒展" })).toHaveAttribute("aria-pressed", "true");
  });
});
