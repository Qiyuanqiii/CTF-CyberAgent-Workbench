import { useEffect, useMemo, useRef, useState, type CSSProperties,
  type Dispatch, type ReactNode, type RefObject, type SetStateAction } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BrainCircuit,
  Check,
  ChevronDown,
  Lightbulb,
  LoaderCircle,
  PackageSearch,
  Paperclip,
  Plus,
  Target,
} from "lucide-react";
import type { CyberAgentClient } from "../api/client";

export const defaultContextCapacityTokens = 32 * 1024;

export function AgentComposerControls({ client, route, contextTokens = 0,
  contextCapacity = defaultContextCapacityTokens, contextPartial = false,
  planMode = false, targetMode = false, onPlanModeChange, onTargetModeChange,
  onOpenFiles, onOpenPlugins, status, trailing }: {
  client: CyberAgentClient;
  route: string;
  contextTokens?: number;
  contextCapacity?: number;
  contextPartial?: boolean;
  planMode?: boolean;
  targetMode?: boolean;
  onPlanModeChange?: (selected: boolean) => void;
  onTargetModeChange?: (selected: boolean) => void;
  onOpenFiles?: () => void;
  onOpenPlugins?: () => void;
  status?: ReactNode;
  trailing?: ReactNode;
}) {
  return <div className="agent-composer-controls">
    <div className="agent-composer-left">
      <ComposerAddMenu onOpenFiles={onOpenFiles} onOpenPlugins={onOpenPlugins}
        onPlanModeChange={onPlanModeChange} onTargetModeChange={onTargetModeChange}
        planMode={planMode} targetMode={targetMode} />
      {targetMode && <span className="composer-mode-chip"><Target aria-hidden="true" size={13} />目标</span>}
      {planMode && <span className="composer-mode-chip"><Lightbulb aria-hidden="true" size={13} />计划</span>}
    </div>
    <div className="agent-composer-status">{status}</div>
    <div className="agent-composer-right">
      <ContextMeter capacity={contextCapacity} partial={contextPartial} used={contextTokens} />
      <ModelQuickPicker client={client} route={route} />
      <ReasoningPicker />
      {trailing}
    </div>
  </div>;
}

function ComposerAddMenu({ planMode, targetMode, onPlanModeChange, onTargetModeChange,
  onOpenFiles, onOpenPlugins }: {
  planMode: boolean;
  targetMode: boolean;
  onPlanModeChange?: (selected: boolean) => void;
  onTargetModeChange?: (selected: boolean) => void;
  onOpenFiles?: () => void;
  onOpenPlugins?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  useDismissiblePopover(root, open, setOpen);
  const select = (action?: () => void) => {
    action?.();
    setOpen(false);
  };
  return <div className="composer-popover-root" ref={root}>
    <button aria-expanded={open} aria-haspopup="menu" aria-label="添加"
      className="composer-icon-button" onClick={() => setOpen((current) => !current)}
      title="添加" type="button"><Plus aria-hidden="true" size={19} /></button>
    {open && <div aria-label="添加到任务" className="composer-popover composer-add-popover" role="menu">
      <span className="composer-popover-heading">添加</span>
      <button disabled={!onOpenFiles} onClick={() => select(onOpenFiles)} role="menuitem" type="button">
        <Paperclip aria-hidden="true" size={16} /><span><strong>文件和文件夹</strong>
          <small>{onOpenFiles ? "浏览当前工作区" : "创建或选择任务后可用"}</small></span>
      </button>
      <button aria-pressed={targetMode} disabled={!onTargetModeChange}
        onClick={() => select(() => onTargetModeChange?.(!targetMode))} role="menuitem" type="button">
        <Target aria-hidden="true" size={16} /><span><strong>目标</strong><small>设置持续追求的任务目标</small></span>
        {targetMode && <Check aria-hidden="true" size={15} />}
      </button>
      <button aria-pressed={planMode} disabled={!onPlanModeChange}
        onClick={() => select(() => onPlanModeChange?.(!planMode))} role="menuitem" type="button">
        <Lightbulb aria-hidden="true" size={16} /><span><strong>计划模式</strong><small>先规划，再进入交付</small></span>
        {planMode && <Check aria-hidden="true" size={15} />}
      </button>
      <span className="composer-popover-heading">插件</span>
      <button disabled={!onOpenPlugins} onClick={() => select(onOpenPlugins)} role="menuitem" type="button">
        <PackageSearch aria-hidden="true" size={16} /><span><strong>已安装插件</strong>
          <small>打开 Go 管理的插件与 Skill</small></span>
      </button>
    </div>}
  </div>;
}

function ContextMeter({ used, capacity, partial }: {
  used: number;
  capacity: number;
  partial: boolean;
}) {
  const normalizedCapacity = Math.max(1, Math.round(capacity));
  const normalizedUsed = Math.max(0, Math.round(used));
  const percent = Math.min(100, Math.round((normalizedUsed / normalizedCapacity) * 100));
  return <div aria-label={`上下文已用 ${percent}%`} className="context-meter" role="status" tabIndex={0}>
    <i aria-hidden="true" style={{ "--context-percent": `${percent * 3.6}deg` } as CSSProperties} />
    <div className="context-meter-popover" role="tooltip">
      <strong>背景信息窗口：{percent}% 已用{partial ? "（部分）" : ""}</strong>
      <span>已加载约 {formatTokens(normalizedUsed)}，本地保守窗口 {formatTokens(normalizedCapacity)}</span>
    </div>
  </div>;
}

function ModelQuickPicker({ client, route }: { client: CyberAgentClient; route: string }) {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const queryClient = useQueryClient();
  useDismissiblePopover(root, open, setOpen);
  const query = useQuery({
    queryKey: ["models", "availability"],
    queryFn: ({ signal }) => client.modelAvailability(signal),
    enabled: open,
    staleTime: 30_000,
  });
  const mutation = useMutation({
    mutationFn: ({ provider, model }: { provider: string; model: string }) =>
      client.selectModelRoute(route, {
        version: "model_route_control.v1", provider, model,
      }),
    onSuccess: () => {
      setOpen(false);
      void queryClient.invalidateQueries({ queryKey: ["models", "availability"] });
    },
  });
  const selected = query.data?.routes.find((candidate) => candidate.name === route);
  const options = useMemo(() => query.data?.providers
    .filter((provider) => provider.status === "available")
    .flatMap((provider) => provider.models.map((model) => ({ provider: provider.name, model }))) ?? [],
  [query.data]);
  const label = selected ? selected.model : route || "模型";
  return <div className="composer-popover-root" ref={root}>
    <button aria-expanded={open} aria-haspopup="menu"
      aria-label={`选择模型，当前 ${label}`} className="composer-model-button"
      onClick={() => setOpen((current) => !current)} title={`模型路由：${route}`} type="button">
      {query.isFetching || mutation.isPending
        ? <LoaderCircle aria-hidden="true" className="spin" size={14} /> : null}
      <span>{label}</span><ChevronDown aria-hidden="true" size={14} />
    </button>
    {open && <div aria-label="选择模型" className="composer-popover composer-model-popover" role="menu">
      <span className="composer-popover-heading">模型 · {route}</span>
      {query.isLoading && <small className="composer-popover-note">正在读取可用模型...</small>}
      {query.isError && <small className="composer-popover-error">模型列表暂时不可用</small>}
      {options.map((option) => {
        const active = selected?.provider === option.provider && selected.model === option.model;
        return <button disabled={!client.hasModelControl || mutation.isPending}
          key={`${option.provider}/${option.model}`}
          onClick={() => mutation.mutate(option)} role="menuitemradio"
          aria-checked={active} type="button">
          <span><strong>{option.model}</strong><small>{option.provider}</small></span>
          {active && <Check aria-hidden="true" size={15} />}
        </button>;
      })}
      {!query.isLoading && !query.isError && options.length === 0 &&
        <small className="composer-popover-note">当前没有可用模型</small>}
      {!client.hasModelControl && <small className="composer-popover-note">当前启动未开启模型控制</small>}
      {mutation.isError && <small className="composer-popover-error">模型切换失败</small>}
    </div>}
  </div>;
}

function ReasoningPicker() {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  useDismissiblePopover(root, open, setOpen);
  return <div className="composer-popover-root reasoning-picker" ref={root}>
    <button aria-expanded={open} aria-haspopup="menu" aria-label="推理强度，当前标准"
      className="composer-reasoning-button"
      onClick={() => setOpen((current) => !current)} title="推理强度" type="button">
      <BrainCircuit aria-hidden="true" size={14} /><span>标准</span><ChevronDown aria-hidden="true" size={14} />
    </button>
    {open && <div aria-label="推理强度" className="composer-popover composer-reasoning-popover" role="menu">
      <span className="composer-popover-heading">推理强度</span>
      <button aria-checked="true" role="menuitemradio" type="button"><span><strong>标准</strong>
        <small>当前 Provider 合同</small></span><Check aria-hidden="true" size={15} /></button>
      <button disabled role="menuitemradio" title="Provider 协议尚未声明 reasoning_effort" type="button">
        <span><strong>高</strong><small>待 Provider 适配</small></span></button>
      <button disabled role="menuitemradio" title="Provider 协议尚未声明 reasoning_effort" type="button">
        <span><strong>最高</strong><small>待 Provider 适配</small></span></button>
    </div>}
  </div>;
}

function useDismissiblePopover(root: RefObject<HTMLDivElement | null>, open: boolean,
  setOpen: Dispatch<SetStateAction<boolean>>) {
  useEffect(() => {
    if (!open) return;
    const pointer = (event: MouseEvent) => {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    };
    const keyboard = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", pointer);
    window.addEventListener("keydown", keyboard);
    return () => {
      document.removeEventListener("mousedown", pointer);
      window.removeEventListener("keydown", keyboard);
    };
  }, [open, root, setOpen]);
}

function formatTokens(value: number): string {
  if (value >= 1000) return `${Math.round(value / 100) / 10}k 标记`;
  return `${value} 标记`;
}
