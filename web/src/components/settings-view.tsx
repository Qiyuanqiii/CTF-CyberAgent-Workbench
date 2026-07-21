import { useEffect, useMemo, useState } from "react";
import {
  ArrowLeft,
  CircleUserRound,
  Cpu,
  Info,
  Keyboard,
  PackageSearch,
  Palette,
  Search,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
} from "lucide-react";
import type { HealthView } from "../api/types";

export type SettingsCapability = {
  id: string;
  label: string;
  enabled: boolean;
};

type SettingsSection = "profile" | "general" | "appearance" | "shortcuts" | "about";
type Density = "comfortable" | "compact";

const densityStorageKey = "prayu.ui-density";

function readDensity(): Density {
  if (typeof window === "undefined") return "comfortable";
  try {
    return window.localStorage.getItem(densityStorageKey) === "compact"
      ? "compact" : "comfortable";
  } catch {
    return "comfortable";
  }
}

function persistDensity(density: Density) {
  try {
    window.localStorage.setItem(densityStorageKey, density);
  } catch {
    // Display preferences must never block the workbench.
  }
}

const navigation: Array<{
  id: SettingsSection;
  label: string;
  icon: typeof Settings;
}> = [
  { id: "general", label: "常规", icon: Settings },
  { id: "profile", label: "个人资料", icon: CircleUserRound },
  { id: "appearance", label: "外观", icon: Palette },
  { id: "shortcuts", label: "键盘快捷键", icon: Keyboard },
  { id: "about", label: "关于", icon: Info },
];

export function SettingsView({
  capabilities,
  desktop,
  health,
  onBack,
  onOpenModels,
  onOpenSkills,
}: {
  capabilities: SettingsCapability[];
  desktop: boolean;
  health: HealthView | null;
  onBack: () => void;
  onOpenModels: () => void;
  onOpenSkills: () => void;
}) {
  const [section, setSection] = useState<SettingsSection>("profile");
  const [query, setQuery] = useState("");
  const [density, setDensity] = useState<Density>(readDensity);
  const visibleNavigation = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase();
    return navigation.filter((item) => !normalized ||
      `${item.label} ${item.id}`.toLocaleLowerCase().includes(normalized));
  }, [query]);

  useEffect(() => {
    document.documentElement.dataset.prayuDensity = density;
    persistDensity(density);
  }, [density]);

  return (
    <div className="settings-shell">
      <aside className="settings-sidebar">
        <button className="settings-back" onClick={onBack} type="button">
          <ArrowLeft aria-hidden="true" size={17} />返回应用
        </button>
        <label className="settings-search">
          <Search aria-hidden="true" size={15} />
          <input aria-label="搜索设置" onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索设置..." type="search" value={query} />
        </label>
        <span className="settings-group-label">个人</span>
        <nav aria-label="Prayu 设置">
          {visibleNavigation.map(({ id, label, icon: Icon }) => (
            <button className={section === id ? "active" : ""} key={id}
              onClick={() => setSection(id)} type="button">
              <Icon aria-hidden="true" size={16} /><span>{label}</span>
            </button>
          ))}
        </nav>
        <span className="settings-group-label">集成</span>
        <nav aria-label="Prayu 集成">
          <button onClick={onOpenModels} type="button">
            <Cpu aria-hidden="true" size={16} /><span>模型与配置</span>
          </button>
          <button disabled={!desktop} onClick={onOpenSkills} type="button">
            <PackageSearch aria-hidden="true" size={16} /><span>Skill 包</span>
          </button>
        </nav>
      </aside>
      <main className="settings-main">
        <header className="settings-header">
          <strong>{navigation.find((item) => item.id === section)?.label}</strong>
          <div>
            <button className="settings-action" onClick={onOpenModels} type="button">
              <Cpu aria-hidden="true" size={15} />模型
            </button>
            {desktop && <button className="settings-action" onClick={onOpenSkills} type="button">
              <PackageSearch aria-hidden="true" size={15} />Skill
            </button>}
          </div>
        </header>
        <div className="settings-scroll">
          {section === "profile" && <ProfileSettings capabilities={capabilities}
            desktop={desktop} health={health} />}
          {section === "general" && <GeneralSettings capabilities={capabilities}
            desktop={desktop} health={health} />}
          {section === "appearance" && <AppearanceSettings density={density}
            onDensityChange={setDensity} />}
          {section === "shortcuts" && <ShortcutSettings />}
          {section === "about" && <AboutSettings desktop={desktop} health={health} />}
        </div>
      </main>
    </div>
  );
}

function ProfileSettings({ capabilities, desktop, health }: {
  capabilities: SettingsCapability[];
  desktop: boolean;
  health: HealthView | null;
}) {
  const enabled = capabilities.filter((capability) => capability.enabled);
  return (
    <div className="profile-settings">
      <section className="profile-identity">
        <span className="profile-avatar" aria-hidden="true">P</span>
        <h1>Prayu</h1>
        <p>@local-operator <span>Local</span></p>
      </section>
      <dl className="profile-metrics">
        <div><dt>Schema</dt><dd>v{health?.schema_version ?? "-"}</dd></div>
        <div><dt>API</dt><dd>{health?.api_version ?? "api.v1"}</dd></div>
        <div><dt>版本</dt><dd>{health?.app_version ?? "dev"}</dd></div>
        <div><dt>控制能力</dt><dd>{enabled.length}/{capabilities.length}</dd></div>
        <div><dt>运行界面</dt><dd>{desktop ? "Desktop" : "Web"}</dd></div>
      </dl>
      <section className="capability-activity" aria-label="能力状态">
        <header>
          <div><h2>能力状态</h2><span>{enabled.length} 项已启用</span></div>
          <span className="capability-legend"><i />启用</span>
        </header>
        <div className="capability-grid">
          {capabilities.map((capability) => <span aria-label={`${capability.label}: ${capability.enabled ? "启用" : "关闭"}`}
            className={capability.enabled ? "enabled" : ""} key={capability.id}
            role="img" title={`${capability.label}: ${capability.enabled ? "启用" : "关闭"}`} />)}
        </div>
      </section>
      <div className="profile-detail-columns">
        <section>
          <h2>运行时</h2>
          <dl className="settings-values">
            <div><dt>状态</dt><dd>{health?.status ?? "connecting"}</dd></div>
            <div><dt>控制平面</dt><dd>Go</dd></div>
            <div><dt>界面</dt><dd>React / Vite</dd></div>
            <div><dt>本地存储</dt><dd>SQLite</dd></div>
          </dl>
        </section>
        <section>
          <h2>当前能力</h2>
          <ul className="enabled-capability-list">
            {enabled.slice(0, 5).map((capability) => <li key={capability.id}>
              <ShieldCheck aria-hidden="true" size={15} />
              <span>{capability.label}</span>
            </li>)}
            {enabled.length === 0 && <li><SlidersHorizontal aria-hidden="true" size={15} />只读模式</li>}
          </ul>
        </section>
      </div>
    </div>
  );
}

function GeneralSettings({ capabilities, desktop, health }: {
  capabilities: SettingsCapability[];
  desktop: boolean;
  health: HealthView | null;
}) {
  return <section className="settings-page-section">
    <h1>常规</h1>
    <dl className="settings-row-list">
      <div><dt>连接状态</dt><dd><span className="settings-online-dot" />{health?.status ?? "connecting"}</dd></div>
      <div><dt>运行界面</dt><dd>{desktop ? "Windows Desktop" : "Web console"}</dd></div>
      <div><dt>控制能力</dt><dd>{capabilities.filter((item) => item.enabled).length} / {capabilities.length}</dd></div>
      <div><dt>数据边界</dt><dd>Local-first</dd></div>
    </dl>
  </section>;
}

function AppearanceSettings({ density, onDensityChange }: {
  density: Density;
  onDensityChange: (density: Density) => void;
}) {
  return <section className="settings-page-section">
    <h1>外观</h1>
    <div className="appearance-setting-row">
      <div><strong>界面密度</strong><span>Workspace density</span></div>
      <div className="prayu-segmented" role="group" aria-label="界面密度">
        <button aria-pressed={density === "comfortable"}
          onClick={() => onDensityChange("comfortable")} type="button">舒展</button>
        <button aria-pressed={density === "compact"}
          onClick={() => onDensityChange("compact")} type="button">紧凑</button>
      </div>
    </div>
    <div className="appearance-preview" aria-label="Prayu 水墨主题预览">
      <span /><div><strong>Prayu Ink</strong><small>Orange / Ivory / Charcoal</small></div>
      <ShieldCheck aria-hidden="true" size={17} />
    </div>
  </section>;
}

function ShortcutSettings() {
  return <section className="settings-page-section">
    <h1>键盘快捷键</h1>
    <dl className="shortcut-list">
      <div><dt>打开命令面板</dt><dd><kbd>Ctrl</kbd><kbd>K</kbd></dd></div>
      <div><dt>关闭对话框或预览</dt><dd><kbd>Esc</kbd></dd></div>
      <div><dt>选择上一项</dt><dd><kbd>↑</kbd></dd></div>
      <div><dt>选择下一项</dt><dd><kbd>↓</kbd></dd></div>
      <div><dt>确认当前操作</dt><dd><kbd>Enter</kbd></dd></div>
    </dl>
  </section>;
}

function AboutSettings({ desktop, health }: { desktop: boolean; health: HealthView | null }) {
  return <section className="settings-page-section about-prayu">
    <span className="about-mark">P</span>
    <h1>Prayu</h1>
    <p>Local-first AI Agent Workbench</p>
    <dl className="settings-row-list">
      <div><dt>应用版本</dt><dd>{health?.app_version ?? "dev"}</dd></div>
      <div><dt>API 协议</dt><dd>{health?.api_version ?? "api.v1"}</dd></div>
      <div><dt>数据库</dt><dd>schema v{health?.schema_version ?? "-"}</dd></div>
      <div><dt>Surface</dt><dd>{desktop ? "Desktop" : "Web"}</dd></div>
    </dl>
  </section>;
}
