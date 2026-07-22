import { fireEvent, render, screen } from "@testing-library/react";
import { SidebarResizeHandle } from "./workbench-frame";

describe("SidebarResizeHandle", () => {
  it("supports bounded keyboard resizing and double-click reset", () => {
    const onChange = vi.fn();
    render(<SidebarResizeHandle onChange={onChange} value={286} />);
    const separator = screen.getByRole("separator", { name: "调整侧栏宽度" });

    fireEvent.keyDown(separator, { key: "ArrowLeft" });
    fireEvent.keyDown(separator, { key: "ArrowRight" });
    fireEvent.keyDown(separator, { key: "Home" });
    fireEvent.keyDown(separator, { key: "End" });
    fireEvent.doubleClick(separator);

    expect(onChange.mock.calls.map((call) => call[0])).toEqual([274, 298, 232, 420, 286]);
  });
});
