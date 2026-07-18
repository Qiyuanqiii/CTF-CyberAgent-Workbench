export const desktopConnectionProtocol = "desktop_connection_bootstrap.v1";
export const desktopSkillDialogProtocol = "desktop_skill_package_dialog.v1";
export const desktopSkillSelectionProtocol = "desktop_file_selection.v1";
export const desktopSkillPreviewProtocol = "desktop_skill_package_preview.v1";

export interface DesktopConnectionBootstrap {
  protocol_version: typeof desktopConnectionProtocol;
  api_base_url: "/api/v1";
  api_version: "api.v1";
  app_version: string;
  ui_digest: string;
  read_token: string;
  control_token: string;
  control_enabled: boolean;
  run_creation_enabled: boolean;
  session_message_enabled: boolean;
  session_steering_control_enabled: boolean;
  run_lifecycle_enabled: boolean;
  run_execution_enabled: boolean;
  read_only_default: boolean;
  process_execution_enabled: false;
  shell_execution_enabled: false;
  docker_execution_enabled: false;
  skill_installation_enabled: false;
  renderer_path_input_supported: false;
}

export interface DesktopSkillSelection {
  protocol_version: typeof desktopSkillSelectionProtocol;
  handle: string;
  expires_at: string;
}

export interface DesktopSkillDialogResult {
  protocol_version: typeof desktopSkillDialogProtocol;
  status: "cancelled" | "selected";
  selection: DesktopSkillSelection | null;
}

export interface DesktopSkillPreview {
  protocol_version: typeof desktopSkillPreviewProtocol;
  package_protocol: string;
  skill_protocol: string;
  name: string;
  version: string;
  profiles: string[];
  declared_tools: string[];
  declared_tool_count: number;
  content_bytes: number;
  content_token_upper_bound: number;
  archive_sha256: string;
  package_fingerprint: string;
  archive_bytes: number;
  uncompressed_bytes: number;
  entry_count: number;
  trust_class: string;
  risk_codes: string[];
  executable_asset_count: number;
  install_hook_count: number;
  import_command_execution: false;
  import_network_access: false;
  import_provider_calls: false;
  tool_capability_grant: false;
  installation_authorized: false;
  validated: true;
}

interface NativeDesktopBridge {
  Bootstrap: () => Promise<unknown>;
  PreviewSkillPackage: (handle: string) => Promise<unknown>;
  SelectSkillPackage: () => Promise<unknown>;
}

declare global {
  interface Window {
    go?: {
      desktop?: {
        DesktopBridge?: Partial<NativeDesktopBridge>;
      };
    };
  }
}

let activeBootstrap: DesktopConnectionBootstrap | null = null;
let bootstrapPromise: Promise<DesktopConnectionBootstrap> | null = null;

export function desktopBridgeAvailable(): boolean {
  return getBridge() !== null;
}

export function desktopRuntimeActive(): boolean {
  return activeBootstrap !== null;
}

export async function loadDesktopBootstrap(): Promise<DesktopConnectionBootstrap | null> {
  if (activeBootstrap) {
    return activeBootstrap;
  }
  const bridge = getBridge();
  if (!bridge) {
    return null;
  }
  if (!bootstrapPromise) {
    bootstrapPromise = bridge.Bootstrap().then((value) => {
      if (!validBootstrap(value)) {
        throw new Error("Desktop connection bootstrap was rejected");
      }
      activeBootstrap = value;
      return value;
    });
  }
  try {
    return await bootstrapPromise;
  } catch (error) {
    bootstrapPromise = null;
    throw error;
  }
}

export async function selectDesktopSkillPreview(): Promise<DesktopSkillPreview | null> {
  const bridge = getBridge();
  if (!bridge) {
    throw new Error("Desktop native bridge is unavailable");
  }
  const dialogValue = await bridge.SelectSkillPackage();
  if (!validDialogResult(dialogValue)) {
    throw new Error("Desktop Skill selection result was rejected");
  }
  if (dialogValue.status === "cancelled") {
    return null;
  }
  if (!dialogValue.selection) {
    throw new Error("Desktop Skill selection result was rejected");
  }
  const previewValue = await bridge.PreviewSkillPackage(dialogValue.selection.handle);
  if (!validPreview(previewValue)) {
    throw new Error("Desktop Skill preview was rejected");
  }
  return previewValue;
}

export function desktopErrorMessage(value: unknown): string {
  if (value instanceof Error && value.message.trim()) {
    return value.message;
  }
  if (isRecord(value) && typeof value.message === "string" && value.message.trim()) {
    return value.message;
  }
  return "Desktop operation failed";
}

function getBridge(): NativeDesktopBridge | null {
  const candidate = window.go?.desktop?.DesktopBridge;
  if (!candidate || typeof candidate.Bootstrap !== "function" ||
    typeof candidate.SelectSkillPackage !== "function" ||
    typeof candidate.PreviewSkillPackage !== "function") {
    return null;
  }
  return candidate as NativeDesktopBridge;
}

function validBootstrap(value: unknown): value is DesktopConnectionBootstrap {
  if (!hasExactKeys(value, [
    "api_base_url", "api_version", "app_version", "control_enabled", "control_token",
    "docker_execution_enabled", "process_execution_enabled", "protocol_version", "read_only_default",
    "read_token", "renderer_path_input_supported", "run_creation_enabled", "shell_execution_enabled",
    "run_execution_enabled", "run_lifecycle_enabled",
    "session_message_enabled", "skill_installation_enabled", "ui_digest",
    "session_steering_control_enabled",
  ])) {
    return false;
  }
  return value.protocol_version === desktopConnectionProtocol && value.api_base_url === "/api/v1" &&
    value.api_version === "api.v1" && boundedText(value.app_version, 1, 64) &&
    isSHA256(value.ui_digest) && validToken(value.read_token) &&
    typeof value.control_token === "string" && typeof value.control_enabled === "boolean" &&
    typeof value.run_creation_enabled === "boolean" &&
    typeof value.session_message_enabled === "boolean" &&
    typeof value.session_steering_control_enabled === "boolean" &&
    typeof value.run_lifecycle_enabled === "boolean" &&
    typeof value.run_execution_enabled === "boolean" &&
    (value.control_token !== "") === (value.control_enabled || value.run_creation_enabled ||
      value.session_message_enabled || value.session_steering_control_enabled ||
      value.run_lifecycle_enabled || value.run_execution_enabled) &&
    (value.control_token === "" || validToken(value.control_token)) &&
    value.control_token !== value.read_token &&
    value.read_only_default === !(value.control_enabled || value.run_creation_enabled ||
      value.session_message_enabled || value.session_steering_control_enabled ||
      value.run_lifecycle_enabled || value.run_execution_enabled) &&
    value.process_execution_enabled === false && value.shell_execution_enabled === false &&
    value.docker_execution_enabled === false && value.skill_installation_enabled === false &&
    value.renderer_path_input_supported === false;
}

function validDialogResult(value: unknown): value is DesktopSkillDialogResult {
  if (!hasExactKeys(value, ["protocol_version", "selection", "status"]) ||
    value.protocol_version !== desktopSkillDialogProtocol ||
    (value.status !== "cancelled" && value.status !== "selected")) {
    return false;
  }
  if (value.status === "cancelled") {
    return value.selection === null;
  }
  return validSelection(value.selection);
}

function validSelection(value: unknown): value is DesktopSkillSelection {
  return hasExactKeys(value, ["expires_at", "handle", "protocol_version"]) &&
    value.protocol_version === desktopSkillSelectionProtocol &&
    typeof value.handle === "string" && /^[A-Za-z0-9_-]{43}$/.test(value.handle) &&
    typeof value.expires_at === "string" && Number.isFinite(Date.parse(value.expires_at));
}

function validPreview(value: unknown): value is DesktopSkillPreview {
  if (!hasExactKeys(value, [
    "archive_bytes", "archive_sha256", "content_bytes", "content_token_upper_bound",
    "declared_tool_count", "declared_tools", "entry_count", "executable_asset_count",
    "import_command_execution", "import_network_access", "import_provider_calls",
    "install_hook_count", "installation_authorized", "name", "package_fingerprint",
    "package_protocol", "profiles", "protocol_version", "risk_codes", "skill_protocol",
    "tool_capability_grant", "trust_class", "uncompressed_bytes", "validated", "version",
  ])) {
    return false;
  }
  return value.protocol_version === desktopSkillPreviewProtocol &&
    boundedText(value.package_protocol, 1, 64) && boundedText(value.skill_protocol, 1, 64) &&
    boundedText(value.name, 1, 128) && boundedText(value.version, 1, 64) &&
    boundedStringArray(value.profiles, 8, 64) && boundedStringArray(value.declared_tools, 16, 128) &&
    value.declared_tool_count === value.declared_tools.length &&
    safeCount(value.content_bytes) && safeCount(value.content_token_upper_bound) &&
    isSHA256(value.archive_sha256) && isSHA256(value.package_fingerprint) &&
    safeCount(value.archive_bytes) && safeCount(value.uncompressed_bytes) && safeCount(value.entry_count) &&
    boundedText(value.trust_class, 1, 64) && boundedStringArray(value.risk_codes, 16, 128) &&
    value.executable_asset_count === 0 && value.install_hook_count === 0 &&
    value.import_command_execution === false && value.import_network_access === false &&
    value.import_provider_calls === false && value.tool_capability_grant === false &&
    value.installation_authorized === false && value.validated === true;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasExactKeys(value: unknown, expected: string[]): value is Record<string, unknown> {
  if (!isRecord(value)) {
    return false;
  }
  const keys = Object.keys(value).sort();
  const wanted = [...expected].sort();
  return keys.length === wanted.length && keys.every((key, index) => key === wanted[index]);
}

function boundedText(value: unknown, minimum: number, maximum: number): value is string {
  return typeof value === "string" && value.trim() === value &&
    value.length >= minimum && value.length <= maximum;
}

function validToken(value: unknown): value is string {
  return boundedText(value, 32, 512) && !/\s/u.test(value);
}

function isSHA256(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
}

function safeCount(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
}

function boundedStringArray(value: unknown, maximumItems: number, maximumLength: number): value is string[] {
  return Array.isArray(value) && value.length <= maximumItems &&
    value.every((item) => boundedText(item, 1, maximumLength));
}
