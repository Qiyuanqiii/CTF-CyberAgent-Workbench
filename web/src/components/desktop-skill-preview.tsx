import { useEffect, useRef, useState } from "react";
import { FileArchive, FolderOpen, LoaderCircle, PackageCheck, ShieldCheck, X } from "lucide-react";
import {
  desktopErrorMessage,
  installDesktopSkillPackage,
  selectDesktopSkillPreview,
  type DesktopSkillInstallResult,
  type DesktopSkillPreview,
} from "../lib/desktop-bridge";
import { formatBytes, formatNumber } from "../lib/format";
import { OperationReceipt } from "./operation-receipt";

export function DesktopSkillPreviewDialog({ open, onClose, installationEnabled = false }: {
  open: boolean;
  onClose: () => void;
  installationEnabled?: boolean;
}) {
  const [preview, setPreview] = useState<DesktopSkillPreview | null>(null);
  const [installed, setInstalled] = useState<DesktopSkillInstallResult | null>(null);
  const [surface, setSurface] = useState<"code" | "cyber">("code");
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const operationKey = useRef("");

  useEffect(() => {
    if (!open) {
      setPreview(null);
      setInstalled(null);
      setConfirmed(false);
      setError("");
      setLoading(false);
      return;
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !loading) {
        onClose();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [loading, onClose, open]);

  if (!open) {
    return null;
  }

  const select = async () => {
    if (loading) {
      return;
    }
    setLoading(true);
    setError("");
    try {
      const selected = await selectDesktopSkillPreview();
      if (selected) {
        setPreview(selected);
        setInstalled(null);
        setConfirmed(false);
        operationKey.current = `desktop-skill-install-${globalThis.crypto.randomUUID()}`;
      }
    } catch (caught) {
      setError(desktopErrorMessage(caught));
    } finally {
      setLoading(false);
    }
  };

  const install = async () => {
    if (!preview || !installationEnabled || !confirmed || loading || installed) return;
    setLoading(true);
    setError("");
    try {
      setInstalled(await installDesktopSkillPackage(preview, surface, operationKey.current));
    } catch (caught) {
      setError(desktopErrorMessage(caught));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="desktop-dialog-backdrop" role="presentation">
      <section aria-labelledby="desktop-skill-title" aria-modal="true" className="desktop-dialog" role="dialog">
        <header>
          <div>
            <span className="dialog-icon"><FileArchive aria-hidden="true" size={18} /></span>
            <div><h2 id="desktop-skill-title">Skill 包预览</h2><small>本地结构校验</small></div>
          </div>
          <button aria-label="关闭" autoFocus className="icon-button" disabled={loading} onClick={onClose} title="关闭" type="button">
            <X aria-hidden="true" size={17} />
          </button>
        </header>

        <div className="desktop-dialog-body">
          {!preview && (
            <div className="desktop-package-empty">
              <FileArchive aria-hidden="true" size={28} />
              <button className="desktop-select-command" disabled={loading} onClick={() => void select()} type="button">
                {loading ? <LoaderCircle aria-hidden="true" className="spin" size={17} /> : <FolderOpen aria-hidden="true" size={17} />}
                选择 .zip
              </button>
            </div>
          )}
          {preview && <SkillPreviewDetails preview={preview} />}
          {preview && installationEnabled && !installed && <div className="desktop-install-controls">
            <div aria-label="Skill surface" className="segmented-control" role="group">
              <button aria-pressed={surface === "code"} onClick={() => setSurface("code")}
                type="button">Code</button>
              <button aria-pressed={surface === "cyber"} onClick={() => setSurface("cyber")}
                type="button">Cyber</button>
            </div>
            <label className="desktop-install-confirmation">
              <input checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)}
                type="checkbox" />
              <span>确认按不受信任包登记，不授予执行权</span>
            </label>
          </div>}
          {installed && <div className="desktop-install-success" role="status">
            <PackageCheck aria-hidden="true" size={17} />
            <span>{installed.name} {installed.version} 已登记到 {installed.surface}</span>
          </div>}
          {installed && <OperationReceipt receipt={installed.receipt} />}
          {error && <div className="connection-error" role="alert">{error}</div>}
        </div>

        {preview && (
          <footer>
            <span><ShieldCheck aria-hidden="true" size={15} />{installed ? "已安全登记" : "已验证，未安装"}</span>
            <div className="desktop-dialog-actions">
              {installationEnabled && !installed &&
                <button className="desktop-select-command" disabled={loading || !confirmed}
                  onClick={() => void install()} type="button">
                  {loading ? <LoaderCircle aria-hidden="true" className="spin" size={16} />
                    : <PackageCheck aria-hidden="true" size={16} />}安装
                </button>}
              <button className="desktop-select-command" disabled={loading} onClick={() => void select()} type="button">
                <FolderOpen aria-hidden="true" size={16} />重新选择
              </button>
            </div>
          </footer>
        )}
      </section>
    </div>
  );
}

function SkillPreviewDetails({ preview }: { preview: DesktopSkillPreview }) {
  return (
    <div className="desktop-package-preview">
      <div className="desktop-package-heading">
        <div><strong>{preview.name}</strong><span>{preview.version}</span></div>
        <code>{preview.trust_class}</code>
      </div>
      <dl className="desktop-package-metrics">
        <div><dt>Profiles</dt><dd>{preview.profiles.join(", ")}</dd></div>
        <div><dt>Tools</dt><dd>{formatNumber(preview.declared_tool_count)}</dd></div>
        <div><dt>Content</dt><dd>{formatBytes(preview.content_bytes)}</dd></div>
        <div><dt>Tokens</dt><dd>{formatNumber(preview.content_token_upper_bound)}</dd></div>
        <div><dt>Archive</dt><dd>{formatBytes(preview.archive_bytes)}</dd></div>
        <div><dt>Entries</dt><dd>{formatNumber(preview.entry_count)}</dd></div>
      </dl>
      <div className="desktop-package-list">
        <span>Declared tools</span>
        <div>{preview.declared_tools.length > 0
          ? preview.declared_tools.map((tool) => <code key={tool}>{tool}</code>)
          : <small>none</small>}</div>
      </div>
      <div className="desktop-package-list">
        <span>Risk codes</span>
        <div>{preview.risk_codes.map((risk) => <code key={risk}>{risk}</code>)}</div>
      </div>
      <div className="desktop-package-digests">
        <span>Archive <code>{preview.archive_sha256.slice(0, 12)}</code></span>
        <span>Package <code>{preview.package_fingerprint.slice(0, 12)}</code></span>
      </div>
    </div>
  );
}
