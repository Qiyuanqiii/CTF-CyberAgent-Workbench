import { beforeEach, describe, expect, it, vi } from "vitest";

import type { DesktopSkillPreview } from "./desktop-bridge";

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
  verification_evidence_enabled: false,
  workspace_open_enabled: false,
  renderer_path_input_supported: false,
};

const selection = {
  protocol_version: "desktop_file_selection.v1",
  handle: "A".repeat(43),
  expires_at: "2026-07-18T10:00:00Z",
};

const preview: DesktopSkillPreview = {
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
  confirmation_handle: "D".repeat(43),
  confirmation_expires_at: "2026-07-18T10:05:00Z",
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

  it("accepts Session messages without enabling other control capabilities", async () => {
    const messagesOnly = {
      ...bootstrap,
      control_token: "control-token-0123456789abcdefghijkl",
      session_message_enabled: true,
      read_only_default: false,
    };
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(messagesOnly) });
    const module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).resolves.toEqual(messagesOnly);
  });

  it("accepts evidence attachment as an independent capability", async () => {
    const evidenceOnly = {
      ...bootstrap,
      control_token: "control-token-0123456789abcdefghijkl",
      evidence_attachment_enabled: true,
      read_only_default: false,
    };
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(evidenceOnly) });
    const module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).resolves.toEqual(evidenceOnly);
  });

  it("accepts verification evidence as an independent capability", async () => {
    const verificationOnly = {
      ...bootstrap,
      control_token: "control-token-0123456789abcdefghijkl",
      verification_evidence_enabled: true,
      read_only_default: false,
    };
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(verificationOnly) });
    const module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).resolves.toEqual(verificationOnly);
  });

  it("accepts Session steering cancellation without enabling sibling capabilities", async () => {
    const steeringOnly = {
      ...bootstrap,
      control_token: "control-token-0123456789abcdefghijkl",
      session_steering_control_enabled: true,
      read_only_default: false,
    };
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(steeringOnly) });
    const module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).resolves.toEqual(steeringOnly);
  });

  it("accepts Plan and approval controls as independent capabilities", async () => {
    const planOnly = {
      ...bootstrap,
      control_token: "control-token-0123456789abcdefghijkl",
      plan_delivery_control_enabled: true,
      read_only_default: false,
    };
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(planOnly) });
    let module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).resolves.toEqual(planOnly);

    vi.resetModules();
    const approvalOnly = {
      ...bootstrap,
      control_token: "control-token-0123456789abcdefghijkl",
      approval_control_enabled: true,
      read_only_default: false,
    };
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(approvalOnly) });
    module = await import("./desktop-bridge");
    await expect(module.loadDesktopBootstrap()).resolves.toEqual(approvalOnly);
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

  it("installs only after an enabled bootstrap and validates inert authority", async () => {
    const enabled = {
      ...bootstrap,
      control_token: "control-token-0123456789abcdefghijkl",
      skill_installation_enabled: true,
      read_only_default: false,
    };
    const result = {
      protocol_version: "desktop_skill_package_install.v1",
      name: preview.name, version: preview.version, surface: "code",
      trust_class: "operator_installed_untrusted",
      archive_sha256: preview.archive_sha256,
      package_fingerprint: preview.package_fingerprint,
      replayed: false, recovered_pending: false,
      import_command_execution: false, import_network_access: false,
      import_provider_calls: false, tool_capability_grant: false,
      run_selection_authorized: false, context_injection_authorized: false,
      receipt: { protocol_version: "operation_receipt.v1", kind: "skill_package_install",
        outcome: "installed", durable: true, replayed: false, retry_safe: true,
        retry_strategy: "same_operation_key", recovery_action: "none",
        cleanup_state: "not_applicable" },
    };
    const install = vi.fn().mockResolvedValue(result);
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(enabled), InstallSkillPackage: install });
    const module = await import("./desktop-bridge");
    await module.loadDesktopBootstrap();
    await expect(module.installDesktopSkillPackage(preview, "code",
      "desktop-install-operation-0001")).resolves.toEqual(result);
    expect(install).toHaveBeenCalledWith({
      protocol_version: "desktop_skill_package_install.v1",
      confirmation_handle: preview.confirmation_handle,
      surface: "code",
      operation_key: "desktop-install-operation-0001",
      confirm_untrusted: true,
    });
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

  it("opens a registered Workspace only through a strict pathless native contract", async () => {
    const enabled = { ...bootstrap, workspace_open_enabled: true };
    const launchers = {
      protocol_version: "desktop_workspace_launcher_list.v1",
      workspace_id: "workspace-1",
      launchers: [
        { id: "file-explorer", label: "File Explorer", kind: "folder" },
        { id: "terminal", label: "Terminal", kind: "terminal" },
      ],
      root_path_exposed: false,
      renderer_path_input_supported: false,
      arbitrary_arguments_accepted: false,
      agent_authority_granted: false,
    };
    const result = {
      protocol_version: "desktop_workspace_open.v1",
      workspace_id: "workspace-1",
      launcher_id: "file-explorer",
      status: "started",
      operator_confirmed: true,
      external_process_started: true,
      arbitrary_arguments_accepted: false,
      command_executed: false,
      root_path_exposed: false,
      agent_authority_granted: false,
    };
    const list = vi.fn().mockResolvedValue(launchers);
    const open = vi.fn().mockResolvedValue(result);
    installBridge({ Bootstrap: vi.fn().mockResolvedValue(enabled),
      WorkspaceLaunchers: list, OpenWorkspace: open });
    const module = await import("./desktop-bridge");
    await module.loadDesktopBootstrap();
    await expect(module.listDesktopWorkspaceLaunchers("workspace-1"))
      .resolves.toEqual(launchers.launchers);
    await expect(module.openDesktopWorkspace("workspace-1", "file-explorer"))
      .resolves.toEqual(result);
    expect(list).toHaveBeenCalledWith("workspace-1");
    expect(open).toHaveBeenCalledWith({
      protocol_version: "desktop_workspace_open.v1",
      workspace_id: "workspace-1",
      launcher_id: "file-explorer",
    });
  });

  it("rejects path disclosure, arbitrary arguments, and inconsistent open receipts", async () => {
    const enabled = { ...bootstrap, workspace_open_enabled: true };
    installBridge({
      Bootstrap: vi.fn().mockResolvedValue(enabled),
      WorkspaceLaunchers: vi.fn().mockResolvedValue({
        protocol_version: "desktop_workspace_launcher_list.v1",
        workspace_id: "workspace-1",
        launchers: [],
        root_path_exposed: false,
        renderer_path_input_supported: false,
        arbitrary_arguments_accepted: false,
        agent_authority_granted: false,
        root_path: "C:\\PRIVATE",
      }),
    });
    let module = await import("./desktop-bridge");
    await module.loadDesktopBootstrap();
    await expect(module.listDesktopWorkspaceLaunchers("workspace-1")).rejects.toThrow("rejected");

    vi.resetModules();
    installBridge({
      Bootstrap: vi.fn().mockResolvedValue(enabled),
      OpenWorkspace: vi.fn().mockResolvedValue({
        protocol_version: "desktop_workspace_open.v1",
        workspace_id: "workspace-1",
        launcher_id: "terminal",
        status: "started",
        operator_confirmed: false,
        external_process_started: true,
        arbitrary_arguments_accepted: false,
        command_executed: false,
        root_path_exposed: false,
        agent_authority_granted: false,
      }),
    });
    module = await import("./desktop-bridge");
    await module.loadDesktopBootstrap();
    await expect(module.openDesktopWorkspace("workspace-1", "terminal")).rejects.toThrow("rejected");
    await expect(module.openDesktopWorkspace("workspace-1", "terminal -- powershell"))
      .rejects.toThrow("request was rejected");
  });
});

function installBridge(overrides: Partial<{
  Bootstrap: () => Promise<unknown>;
  InstallSkillPackage: (request: unknown) => Promise<unknown>;
  PreviewSkillPackage: (handle: string) => Promise<unknown>;
  SelectSkillPackage: () => Promise<unknown>;
  OpenWorkspace: (request: unknown) => Promise<unknown>;
  WorkspaceLaunchers: (workspaceID: string) => Promise<unknown>;
}>) {
  window.go = {
    desktop: {
      DesktopBridge: {
        Bootstrap: vi.fn().mockResolvedValue(bootstrap),
        InstallSkillPackage: vi.fn().mockRejectedValue(new Error("disabled")),
        PreviewSkillPackage: vi.fn().mockResolvedValue(preview),
        OpenWorkspace: vi.fn().mockRejectedValue(new Error("disabled")),
        SelectSkillPackage: vi.fn().mockResolvedValue({
          protocol_version: "desktop_skill_package_dialog.v1",
          status: "cancelled",
          selection: null,
        }),
        WorkspaceLaunchers: vi.fn().mockRejectedValue(new Error("disabled")),
        ...overrides,
      },
    },
  };
}
