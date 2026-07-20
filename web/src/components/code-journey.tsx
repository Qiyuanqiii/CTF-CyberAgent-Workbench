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
import { formatDate, shortID } from "../lib/format";
import { StatusBadge } from "./common";
import type { ReceiptReviewFacts, ReceiptReviewNavigationTarget } from "./receipt-review-navigation";

export type CodeJourneyDestination = "overview" | "actions" | "diffs" | "repository" |
  "verify" | "handoff";

interface JourneyStage {
  id: string;
  label: string;
  state: string;
  destination: CodeJourneyDestination;
  icon: typeof ClipboardList;
}

const maxJourneyReceiptReviews = 3;

export function CodeJourney({ detail, receiptReviewFacts, receiptReviewFactsState = "ready",
  onNavigate, onOpenReceiptReview }: {
  detail: RunDetailView;
  receiptReviewFacts?: ReceiptReviewFacts;
  receiptReviewFactsState?: "loading" | "ready" | "unavailable";
  onNavigate: (destination: CodeJourneyDestination) => void;
  onOpenReceiptReview: (target: ReceiptReviewNavigationTarget) => void;
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
    <section aria-label="Receipt review audit facts" className="journey-audit-facts">
      <header><div><strong>Receipt review audit</strong><StatusBadge status="metadata only" />
        <StatusBadge status="non-authorizing" /></div>
        {receiptReviewFacts && <span>{receiptReviewFacts.metadata_confirmed_count} confirmed / {receiptReviewFacts.metadata_disputed_count} disputed</span>}
      </header>
      {receiptReviewFactsState === "loading" && <p>Loading bounded audit facts</p>}
      {receiptReviewFactsState === "unavailable" && <p>Audit facts unavailable</p>}
      {receiptReviewFactsState === "ready" && receiptReviewFacts?.references.length === 0 &&
        <p>No receipt review facts</p>}
      {receiptReviewFactsState === "ready" && receiptReviewFacts &&
        receiptReviewFacts.references.length > 0 && <ul>
          {receiptReviewFacts.references.slice(0, maxJourneyReceiptReviews).map((item) =>
            <li key={item.id}><span><strong>{shortID(item.receipt_id)}</strong>
              <small>event {item.review_event_sequence} / {formatDate(item.reviewed_at)}</small></span>
              <StatusBadge status={item.decision.replaceAll("_", " ")} />
              <button aria-label={`Open receipt review ${item.id} in Verify`}
                className="icon-button" onClick={() => onOpenReceiptReview(item)}
                title="Open exact receipt review in Verify" type="button">
                <ChevronRight aria-hidden="true" size={15} />
              </button></li>)}
        </ul>}
      {receiptReviewFacts && (receiptReviewFacts.returned_count > maxJourneyReceiptReviews ||
        receiptReviewFacts.truncated) &&
        <footer>Showing {Math.min(maxJourneyReceiptReviews,
          receiptReviewFacts.references.length)} of {receiptReviewFacts.returned_count}
          {receiptReviewFacts.truncated && <StatusBadge status="source truncated" />}</footer>}
    </section>
    <footer>
      <span>Go control plane</span>
      <span>Independent mutations</span>
      <button className="compact-command" onClick={() => onNavigate("diffs")} type="button">
        <FileDiff aria-hidden="true" size={14} />Open diffs
      </button>
    </footer>
  </section>;
}
