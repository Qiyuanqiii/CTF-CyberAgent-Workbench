import {
  ChevronRight,
  BookOpenCheck,
  ClipboardList,
  FileDiff,
  GitBranch,
  ListChecks,
  Play,
  ShieldCheck,
} from "lucide-react";
import type { RunDetailView } from "../api/types";
import { StatusBadge } from "./common";

export type CodeJourneyDestination = "overview" | "actions" | "diffs" | "repository" |
  "verify" | "handoff";

interface JourneyStage {
  id: string;
  label: string;
  state: string;
  destination: CodeJourneyDestination;
  icon: typeof ClipboardList;
}

export function CodeJourney({ detail, onNavigate }: {
  detail: RunDetailView;
  onNavigate: (destination: CodeJourneyDestination) => void;
}) {
  const queued = detail.operator_steering.pending + detail.operator_steering.prepared;
  const planState = detail.mode.phase === "deliver" ? "deliver" :
    detail.plan_delivery?.selection ? "selected" :
      detail.plan_delivery?.operator_choice_needed ? "choice required" :
        detail.plan_delivery?.proposal ? "proposed" : "pending";
  const stages: JourneyStage[] = [
    { id: "scope", label: "Scope", state: detail.mission.workspace_id ? "bound" : "unbound",
      destination: "repository", icon: GitBranch },
    { id: "plan", label: "Plan", state: planState,
      destination: "overview", icon: ClipboardList },
    { id: "execute", label: "Queue and execute",
      state: queued > 0 ? `${queued} queued` : detail.run.status,
      destination: "overview", icon: Play },
    { id: "review", label: "Review", state: "per-file",
      destination: "actions", icon: ListChecks },
    { id: "verify", label: "Verify and report", state: "inspect",
      destination: "verify", icon: ShieldCheck },
    { id: "handoff", label: "Handoff", state: "regenerable",
      destination: "handoff", icon: BookOpenCheck },
  ];
  return <section aria-label="Code delivery journey" className="code-journey">
    <header className="projection-heading">
      <div><FileDiff aria-hidden="true" size={17} /><h2>Code Journey</h2></div>
      <div><StatusBadge status={detail.mode.surface} />
        <StatusBadge status={detail.mode.phase} /></div>
    </header>
    <div className="code-journey-list">
      {stages.map(({ id, label, state, destination, icon: Icon }, index) =>
        <div className="code-journey-stage" key={id}>
          <span className="journey-index">{index + 1}</span>
          <Icon aria-hidden="true" size={16} />
          <strong>{label}</strong>
          <StatusBadge status={state} />
          <button aria-label={`Open ${label}`} className="icon-button"
            onClick={() => onNavigate(destination)} title={`Open ${label}`} type="button">
            <ChevronRight aria-hidden="true" size={16} />
          </button>
        </div>)}
    </div>
    <footer>
      <span>Go control plane</span>
      <span>Independent mutations</span>
      <button className="compact-command" onClick={() => onNavigate("diffs")} type="button">
        <FileDiff aria-hidden="true" size={14} />Open diffs
      </button>
    </footer>
  </section>;
}
