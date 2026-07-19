import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ClipboardList, LoaderCircle, Plus, RefreshCw, Trash2 } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { VerificationPlanRequestView } from "../api/types";
import { formatDate } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

type DraftItem = VerificationPlanRequestView["items"][number];

const emptyItem = (): DraftItem => ({ title: "", expected_observation: "" });

export function VerificationPlan({ client, runID }: {
  client: CyberAgentClient;
  runID: string;
}) {
  const queryClient = useQueryClient();
  const operationKey = useRef("");
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  const [items, setItems] = useState<DraftItem[]>([emptyItem()]);
  const query = useQuery({
    queryKey: ["run", runID, "verification-plan"],
    queryFn: ({ signal }) => client.verificationPlans(runID, signal),
    enabled: Boolean(runID),
  });
  const record = useMutation({
    mutationFn: (body: VerificationPlanRequestView) => {
      if (!operationKey.current) {
        operationKey.current = `web-verification-plan-${globalThis.crypto.randomUUID()}`;
      }
      return client.recordVerificationPlan(runID, body, operationKey.current);
    },
    onSuccess: () => {
      operationKey.current = "";
      setTitle("");
      setSummary("");
      setItems([emptyItem()]);
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "verification-plan"] });
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "events"] });
    },
  });
  const changeIntent = (change: () => void) => {
    operationKey.current = "";
    change();
  };
  const complete = title.trim() !== "" && summary.trim() !== "" &&
    items.every((item) => item.title.trim() && item.expected_observation.trim());
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!complete || record.isPending) return;
    record.mutate({ version: "operator_verification_plan.v1", title: title.trim(),
      summary: summary.trim(), items: items.map((item) => ({ title: item.title.trim(),
        expected_observation: item.expected_observation.trim() })) });
  };
  const updateItem = (index: number, key: keyof DraftItem, value: string) => {
    changeIntent(() => setItems((current) => current.map((item, ordinal) => ordinal === index
      ? { ...item, [key]: value } : item)));
  };
  return <section aria-label="Verification plan" className="verification-plan">
    <header className="operator-list-header">
      <div><ClipboardList aria-hidden="true" size={16} /><h2>Verification plan</h2></div>
      <div>{query.data && <StatusBadge status={`${query.data.items.length} plans`} />}
        <button aria-label="Refresh verification plans" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh" type="button"><RefreshCw aria-hidden="true"
            className={query.isFetching ? "spin" : ""} size={15} /></button></div>
    </header>
    {client.hasVerificationEvidence && <form className="verification-plan-form" onSubmit={submit}>
      <label>Title<input disabled={record.isPending} maxLength={160} required value={title}
        onChange={(event) => changeIntent(() => setTitle(event.target.value))} /></label>
      <label>Purpose<textarea disabled={record.isPending} maxLength={2048} required rows={2}
        value={summary}
        onChange={(event) => changeIntent(() => setSummary(event.target.value))} /></label>
      <div className="verification-plan-items">{items.map((item, index) =>
        <fieldset key={index}><legend>Check {index + 1}</legend>
          <label>Check<input aria-label={`Check ${index + 1} title`} disabled={record.isPending}
            maxLength={160} required
            value={item.title} onChange={(event) => updateItem(index, "title", event.target.value)} /></label>
          <label>Expected observation<textarea aria-label={`Check ${index + 1} expected observation`}
            disabled={record.isPending} maxLength={1024} required rows={2}
            value={item.expected_observation}
            onChange={(event) => updateItem(index, "expected_observation", event.target.value)} /></label>
          <button aria-label={`Remove check ${index + 1}`} className="icon-button"
            disabled={record.isPending || items.length === 1}
            onClick={() => changeIntent(() => setItems((current) =>
              current.filter((_, ordinal) => ordinal !== index)))} title="Remove check" type="button">
            <Trash2 aria-hidden="true" size={14} /></button>
        </fieldset>)}</div>
      <div className="verification-plan-actions">
        <button className="compact-command" disabled={record.isPending || items.length >= 32}
          onClick={() => changeIntent(() => setItems((current) => [...current, emptyItem()]))}
          type="button">
          <Plus aria-hidden="true" size={14} />Add check</button>
        <button className="command-button" disabled={record.isPending || !complete} type="submit">
          {record.isPending ? <LoaderCircle aria-hidden="true" className="spin" size={15} /> :
            <ClipboardList aria-hidden="true" size={15} />}Record plan</button>
      </div>
    </form>}
    {record.error && <ErrorState error={record.error} />}
    {query.isLoading && <LoadingState label="Loading verification plans" />}
    {query.isError && <ErrorState error={query.error} />}
    {query.data?.items.length === 0 && <EmptyState>No verification plan recorded</EmptyState>}
    {query.data && query.data.items.length > 0 && <div className="verification-plan-list">
      {query.data.items.map((plan) => <article key={plan.id}>
        <header><strong>{plan.title}</strong><StatusBadge status="guidance only" />
          <time dateTime={plan.created_at}>{formatDate(plan.created_at)}</time></header>
        <p>{plan.summary}</p><ol>{plan.items.map((item) => <li key={item.ordinal}>
          <strong>{item.title}</strong><span>{item.expected_observation}</span></li>)}</ol>
      </article>)}
    </div>}
  </section>;
}
