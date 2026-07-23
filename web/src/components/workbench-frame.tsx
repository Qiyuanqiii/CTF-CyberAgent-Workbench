import { useRef, useState, type FormEvent, type KeyboardEvent, type PointerEvent,
  type ReactNode } from "react";
import {
  ArrowUp,
  CalendarClock,
  GitPullRequest,
  MessagesSquare,
  PackageSearch,
} from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { RunCreationControlRequestView } from "../api/types";
import { AgentComposerControls } from "./agent-composer-controls";
import { WorkbenchDock, type WorkbenchResourceKind } from "./workbench-dock";

export const minimumSidebarWidth = 232;
export const maximumSidebarWidth = 420;
export const defaultSidebarWidth = 286;

export function clampSidebarWidth(width: number): number {
  return Math.min(maximumSidebarWidth, Math.max(minimumSidebarWidth, Math.round(width)));
}

export function SidebarResizeHandle({ value, onChange }: {
  value: number;
  onChange: (value: number) => void;
}) {
  const drag = useRef<{ pointerID: number; originX: number; originWidth: number } | null>(null);
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
      event.preventDefault();
      onChange(clampSidebarWidth(value + (event.key === "ArrowLeft" ? -12 : 12)));
    } else if (event.key === "Home") {
      event.preventDefault();
      onChange(minimumSidebarWidth);
    } else if (event.key === "End") {
      event.preventDefault();
      onChange(maximumSidebarWidth);
    }
  };
  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    drag.current = { pointerID: event.pointerId, originX: event.clientX, originWidth: value };
    event.currentTarget.setPointerCapture?.(event.pointerId);
  };
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    if (!drag.current || drag.current.pointerID !== event.pointerId) return;
    onChange(clampSidebarWidth(drag.current.originWidth + event.clientX - drag.current.originX));
  };
  const finishDrag = (event: PointerEvent<HTMLDivElement>) => {
    if (!drag.current || drag.current.pointerID !== event.pointerId) return;
    drag.current = null;
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId);
    }
  };
  return <div aria-label="调整侧栏宽度" aria-orientation="vertical"
    aria-valuemax={maximumSidebarWidth} aria-valuemin={minimumSidebarWidth}
    aria-valuenow={value} className="sidebar-resize-handle"
    onDoubleClick={() => onChange(defaultSidebarWidth)} onKeyDown={onKeyDown}
    onPointerCancel={finishDrag} onPointerDown={onPointerDown}
    onPointerMove={onPointerMove} onPointerUp={finishDrag} role="separator" tabIndex={0} />;
}

export function WorkbenchFrame({ title, children, client, desktop, resourceKind, runID,
  sessionID }: {
  title: string;
  children: ReactNode;
  client: CyberAgentClient;
  desktop: boolean;
  resourceKind: WorkbenchResourceKind;
  runID: string;
  sessionID: string;
}) {
  return <WorkbenchDock client={client} desktop={desktop} resourceKind={resourceKind}
    runID={runID} sessionID={sessionID} title={title}>
    {children}
  </WorkbenchDock>;
}

export interface NewRunDraft {
  goal: string;
  phase: NonNullable<RunCreationControlRequestView["phase"]>;
}

export function EmptyConversation({ client, onCreateRun, creationEnabled, onOpenPlugins }: {
  client: CyberAgentClient;
  onCreateRun: (draft: NewRunDraft) => void;
  creationEnabled: boolean;
  onOpenPlugins?: () => void;
}) {
  const [goal, setGoal] = useState("");
  const [planMode, setPlanMode] = useState(false);
  const [targetMode, setTargetMode] = useState(false);
  const normalized = goal.trim();
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!creationEnabled || !normalized) return;
    onCreateRun({ goal: normalized, phase: planMode ? "plan" : "deliver" });
  };
  return (
    <div className="prayu-empty-conversation">
      <div className="prayu-empty-heading">
        <MessagesSquare aria-hidden="true" size={24} />
        <h1>开始一项任务</h1>
      </div>
      <form className="prayu-starter-composer" onSubmit={submit}>
        <textarea aria-label="描述任务" disabled={!creationEnabled}
          onChange={(event) => setGoal(event.target.value)} placeholder="描述你想完成的工作"
          rows={2} value={goal} />
        <AgentComposerControls client={client} onOpenPlugins={onOpenPlugins}
          onPlanModeChange={setPlanMode} onTargetModeChange={setTargetMode}
          planMode={planMode} route="code" targetMode={targetMode}
          trailing={<button aria-label="创建任务" className="composer-send-button"
            disabled={!creationEnabled || !normalized} title="创建任务" type="submit">
            <ArrowUp aria-hidden="true" size={16} />
          </button>} />
      </form>
    </div>
  );
}

type UtilityKind = "pull-requests" | "schedule" | "plugins";

const utilityViews: Record<UtilityKind, {
  title: string;
  empty: string;
  icon: typeof GitPullRequest;
}> = {
  "pull-requests": { title: "拉取请求", empty: "暂无拉取请求", icon: GitPullRequest },
  schedule: { title: "自动定时", empty: "暂无自动任务", icon: CalendarClock },
  plugins: { title: "插件", empty: "暂无已打开的插件", icon: PackageSearch },
};

export function UtilityWorkspace({ kind, onOpenPlugins }: {
  kind: UtilityKind;
  onOpenPlugins?: () => void;
}) {
  const view = utilityViews[kind];
  const Icon = view.icon;
  return (
    <section className="utility-workspace">
      <header><Icon aria-hidden="true" size={18} /><h1>{view.title}</h1></header>
      <div className="utility-empty-state">
        <Icon aria-hidden="true" size={25} />
        <strong>{view.empty}</strong>
        {kind === "plugins" && onOpenPlugins &&
          <button className="command-button" onClick={onOpenPlugins} type="button">打开插件管理</button>}
      </div>
    </section>
  );
}
