import { beforeEach, describe, expect, it, vi } from "vitest";

const bootstrap = {
  protocol_version: "desktop_connection_bootstrap.v1",
  api_base_url: "/api/v1",
  api_version: "api.v1",
  app_version: "v0.1.0",
  ui_digest: "a".repeat(64),
  read_token: "read-token-0123456789abcdefghijklmnop",
  control_token: "",
  control_enabled: false,
  run_creation_enabled: false,
  read_only_default: true,
  process_execution_enabled: false,
  shell_execution_enabled: false,
  docker_execution_enabled: false,
  skill_installation_enabled: false,
  renderer_path_input_supported: false,
};

const selection = {
  protocol_version: "desktop_file_selection.v1",
  handle: "A".repeat(43),
  expires_at: "2026-07-18T10:00:00Z",
};

const preview = {
  protocol_version: "desktop_skill_package_preview.v1",
  package_protocol: "skill_package.v1",
  skill_protocol: "skill.v1",
  name: "review-helper",
  version: "1.0.0",
  profiles: ["review"],
  declared_tools: ["workspace_list"],
  declared_tool_count: 1,
  content_bytes: 128,
  content_token_upper_bound: 32,
  archive_sha256: "b".repeat(64),
  package_fingerprint: "c".repeat(64),
  archive_bytes: 512,
  uncompressed_bytes: 384,
  entry_count: 2,
  trust_class: "operator_installed_untrusted",
  risk_codes: ["untrusted_instructions"],
  executable_asset_count: 0,
  install_hook_count: 0,
  import_command_execution: false,
  import_network_access: false,
  import_provider_calls: false,
  tool_capability_grant: false,
  installation_authorized: false,
  validated: true,
};

describe("desktop native bridge", () => {
  beforeEach(() => {
    vi.resetModules();
    delete window.go;
  });

  it("detects absence without changing ordinary browser behavior", async () => {
    const module = await import("./desktop-bridge");
    expect(module.desktopBridgeAvailable()).toBe(false);
    expect(module.desktopRuntimeActive()).toBe(false);
    await expect(module.loadDesktopBootstrap()).resolves.toBeNull();
  });

  it("accepts only a closed-authority same-origin bootstrap", async () => {
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(bootstrap) });
    const module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).resolves.toEqual(bootstrap);
    expect(module.desktopRuntimeActive()).toBe(true);
  });

  it("accepts Run creation without enabling existing Run controls", async () => {
    const creationOnly = {
      ...bootstrap,
      control_token: "control-token-0123456789abcdefghijkl",
      control_enabled: false,
      run_creation_enabled: true,
      read_only_default: false,
    };
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(creationOnly) });
    const module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).resolves.toEqual(creationOnly);
  });

  it("rejects authority widening and extra local-file fields", async () => {
    installBridge({ Bootstrap: vi.fn().mockResolvedValue({ ...bootstrap, process_execution_enabled: true }) });
    let module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).rejects.toThrow("rejected");

    vi.resetModules();
    installBridge({ Bootstrap: vi.fn().mockResolvedValue({ ...bootstrap, source_path: "C:\\PRIVATE" }) });
    module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).rejects.toThrow("rejected");

    vi.resetModules();
    installBridge({ Bootstrap: vi.fn().mockResolvedValue({
      ...bootstrap,
      control_token: bootstrap.read_token,
      control_enabled: true,
    run_creation_enabled: false,
      read_only_default: false,
    }) });
    module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).rejects.toThrow("rejected");
  });

  it("opens the native picker and consumes only its opaque handle", async () => {
    const select = vi.fn().mockResolvedValue({
      protocol_version: "desktop_skill_package_dialog.v1",
      status: "selected",
      selection,
    });
    const consume = vi.fn().mockResolvedValue(preview);
    installBridge({ SelectSkillPackage: select, PreviewSkillPackage: consume });
    const module = await import("./desktop-bridge");
    await expect(module.selectDesktopSkillPreview()).resolves.toEqual(preview);
    expect(select).toHaveBeenCalledWith();
    expect(consume).toHaveBeenCalledWith(selection.handle);
  });

  it("does not consume a cancelled selection or path-bearing preview", async () => {
    const consume = vi.fn().mockResolvedValue(preview);
    installBridge({
      SelectSkillPackage: vi.fn().mockResolvedValue({
        protocol_version: "desktop_skill_package_dialog.v1",
        status: "cancelled",
        selection: null,
      }),
      PreviewSkillPackage: consume,
    });
    let module = await import("./desktop-bridge");
    await expect(module.selectDesktopSkillPreview()).resolves.toBeNull();
    expect(consume).not.toHaveBeenCalled();

    vi.resetModules();
    installBridge({
      SelectSkillPackage: vi.fn().mockResolvedValue({
        protocol_version: "desktop_skill_package_dialog.v1",
        status: "selected",
        selection,
      }),
      PreviewSkillPackage: vi.fn().mockResolvedValue({ ...preview, source_path: "C:\\PRIVATE" }),
    });
    module = await import("./desktop-bridge");
    await expect(module.selectDesktopSkillPreview()).rejects.toThrow("rejected");
  });
});

function installBridge(overrides: Partial<{
  Bootstrap: () => Promise<unknown>;
  PreviewSkillPackage: (handle: string) => Promise<unknown>;
  SelectSkillPackage: () => Promise<unknown>;
}>) {
  window.go = {
    desktop: {
      DesktopBridge: {
        Bootstrap: vi.fn().mockResolvedValue(bootstrap),
        PreviewSkillPackage: vi.fn().mockResolvedValue(preview),
        SelectSkillPackage: vi.fn().mockResolvedValue({
          protocol_version: "desktop_skill_package_dialog.v1",
          status: "cancelled",
          selection: null,
        }),
        ...overrides,
      },
    },
  };
}
