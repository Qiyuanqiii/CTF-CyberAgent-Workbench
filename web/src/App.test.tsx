import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import App from "./App";
import { useConnectionStore } from "./state/connection";

vi.mock("./components/resource-sidebar", () => ({ ResourceSidebar: () => null }));
vi.mock("./components/run-workspace", () => ({
  RunWorkspace: ({ client }: { client: { hasVerificationEvidence: boolean } }) =>
    <div data-testid="verification-capability">
      {String(client.hasVerificationEvidence)}
    </div>,
}));
vi.mock("./components/session-workspace", () => ({ SessionWorkspace: () => null }));
vi.mock("./components/desktop-skill-preview", () => ({
  DesktopSkillPreviewDialog: () => null,
}));
vi.mock("./components/model-availability-dialog", () => ({
  ModelAvailabilityDialog: () => null,
}));
vi.mock("./components/run-creation-dialog", () => ({ RunCreationDialog: () => null }));

describe("App capability wiring", () => {
  beforeEach(() => {
    useConnectionStore.getState().disconnect();
    useConnectionStore.getState().connect("read-token", {
      status: "ok",
      api_version: "api.v1",
      app_version: "test",
      schema_version: 78,
    }, "control-token", {
      verificationEvidenceEnabled: true,
    });
  });

  afterEach(() => {
    useConnectionStore.getState().disconnect();
    vi.unstubAllGlobals();
  });

  it("propagates verification evidence authority into the API client", () => {
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>(() => undefined)));
    render(<QueryClientProvider client={new QueryClient()}><App /></QueryClientProvider>);

    expect(screen.getByTestId("verification-capability")).toHaveTextContent("true");
    expect(document.querySelector(".prayu-shell.workspace-mode")).toBeInTheDocument();
    expect(screen.getByText("Prayu")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "设置" }));
    expect(document.querySelector(".prayu-shell.settings-mode")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "个人资料" })).toHaveClass("active");
  });
});
