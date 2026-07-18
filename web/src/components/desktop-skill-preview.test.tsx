import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { vi } from "vitest";
import { DesktopSkillPreviewDialog } from "./desktop-skill-preview";

const bridgeMocks = vi.hoisted(() => ({
  install: vi.fn(),
  select: vi.fn(),
}));

vi.mock("../lib/desktop-bridge", () => ({
  selectDesktopSkillPreview: bridgeMocks.select,
  installDesktopSkillPackage: bridgeMocks.install,
  desktopErrorMessage: (value: unknown) => value instanceof Error ? value.message : "failed",
}));

describe("DesktopSkillPreviewDialog", () => {
  beforeEach(() => {
    bridgeMocks.install.mockReset();
    bridgeMocks.select.mockReset();
  });

  it("renders bounded metadata with no path or install command", async () => {
    bridgeMocks.select.mockResolvedValue(skillPreview());
    const user = userEvent.setup();
    const { container } = render(<DesktopSkillPreviewDialog open onClose={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "选择 .zip" }));

    expect(await screen.findByText("review-helper")).toBeInTheDocument();
    expect(screen.getByText("已验证，未安装")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /安装/ })).not.toBeInTheDocument();
    expect(container.textContent).not.toContain("C:\\");
    expect(container.textContent).not.toContain("source_path");
  });

  it("requires confirmation and installs through the inert native boundary", async () => {
    bridgeMocks.select.mockResolvedValue(skillPreview());
    bridgeMocks.install.mockResolvedValue({
      protocol_version: "desktop_skill_package_install.v1",
      name: "review-helper", version: "1.0.0", surface: "code",
      trust_class: "operator_installed_untrusted", archive_sha256: "b".repeat(64),
      package_fingerprint: "c".repeat(64), replayed: false, recovered_pending: false,
      import_command_execution: false, import_network_access: false,
      import_provider_calls: false, tool_capability_grant: false,
      run_selection_authorized: false, context_injection_authorized: false,
      receipt: { protocol_version: "operation_receipt.v1", kind: "skill_package_install",
        outcome: "installed", durable: true, replayed: false, retry_safe: true,
        retry_strategy: "same_operation_key", recovery_action: "none",
        cleanup_state: "not_applicable" },
    });
    const user = userEvent.setup();
    render(<DesktopSkillPreviewDialog installationEnabled open onClose={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "选择 .zip" }));
    const install = await screen.findByRole("button", { name: "安装" });
    expect(install).toBeDisabled();
    await user.click(screen.getByRole("checkbox"));
    await user.click(install);
    expect(bridgeMocks.install).toHaveBeenCalledWith(expect.objectContaining({
      name: "review-helper", confirmation_handle: "D".repeat(43),
    }), "code", expect.stringMatching(/^desktop-skill-install-/));
    expect(await screen.findByText(/已登记到 code/)).toBeInTheDocument();
  });

  it("keeps cancellation inert and reports bounded errors", async () => {
    const user = userEvent.setup();
    bridgeMocks.select.mockResolvedValueOnce(null).mockRejectedValueOnce(new Error("selection unavailable"));
    render(<DesktopSkillPreviewDialog open onClose={vi.fn()} />);
    const choose = screen.getByRole("button", { name: "选择 .zip" });
    await user.click(choose);
    expect(screen.queryByText("已验证，未安装")).not.toBeInTheDocument();
    await user.click(choose);
    expect(await screen.findByRole("alert")).toHaveTextContent("selection unavailable");
  });
});

function skillPreview() {
  return {
    protocol_version: "desktop_skill_package_preview.v1",
    package_protocol: "skill_package.v1", skill_protocol: "skill.v1",
    name: "review-helper", version: "1.0.0", profiles: ["review"],
    declared_tools: ["workspace_list"], declared_tool_count: 1,
    content_bytes: 128, content_token_upper_bound: 32,
    archive_sha256: "b".repeat(64), package_fingerprint: "c".repeat(64),
    archive_bytes: 512, uncompressed_bytes: 384, entry_count: 2,
    trust_class: "operator_installed_untrusted", risk_codes: ["untrusted_instructions"],
    executable_asset_count: 0, install_hook_count: 0,
    import_command_execution: false, import_network_access: false,
    import_provider_calls: false, tool_capability_grant: false,
    installation_authorized: false, validated: true,
    confirmation_handle: "D".repeat(43),
    confirmation_expires_at: "2026-07-18T10:05:00Z",
  };
}
