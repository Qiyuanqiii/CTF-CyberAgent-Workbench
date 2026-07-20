import { useRef, useState, type FormEvent } from "react";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ClipboardList, Download, FileCheck2, ListTree, LoaderCircle, Plus, RefreshCw,
  Trash2 } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { VerificationPlanItemCoveragePage, VerificationPlanRequestView } from "../api/types";
import { downloadTextFile } from "../lib/download";
import { formatDate } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

type DraftItem = VerificationPlanRequestView["items"][number];

const emptyItem = (): DraftItem => ({ title: "", expected_observation: "" });
const coveragePageSize = 25;

function mergeCoveragePages(pages: VerificationPlanItemCoveragePage[]) {
  const first = pages[0]?.detail;
  if (!first) return { detail: undefined, associations: [], error: undefined };
  const associations: VerificationPlanItemCoveragePage["detail"]["associations"] = [];
  const associationIDs = new Set<string>();
  const evidenceIDs = new Set<string>();
  let previousSequence = Number.MAX_SAFE_INTEGER;
  let passCount = 0;
  let failCount = 0;
  let unknownCount = 0;
  for (const page of pages) {
    const detail = page.detail;
    if (page.page.limit !== pages[0]?.page.limit || detail.run_id !== first.run_id ||
      detail.session_id !== first.session_id || detail.workspace_id !== first.workspace_id ||
      detail.plan_id !== first.plan_id || detail.plan_sha256 !== first.plan_sha256 ||
      detail.plan_item_ordinal !== first.plan_item_ordinal ||
      detail.plan_item_sha256 !== first.plan_item_sha256 ||
      detail.associated_evidence_count !== first.associated_evidence_count ||
      detail.pass_count !== first.pass_count || detail.fail_count !== first.fail_count ||
      detail.unknown_count !== first.unknown_count ||
      detail.latest_association_event_sequence !== first.latest_association_event_sequence) {
      return { detail: first, associations: [],
        error: new Error("Evidence pages changed while loading; refresh the verification plan") };
    }
    for (const association of detail.associations) {
      if (associationIDs.has(association.id) || evidenceIDs.has(association.evidence_id) ||
        association.association_event_sequence >= previousSequence) {
        return { detail: first, associations: [],
          error: new Error("Evidence pages changed while loading; refresh the verification plan") };
      }
      associationIDs.add(association.id);
      evidenceIDs.add(association.evidence_id);
      previousSequence = association.association_event_sequence;
      if (association.evidence_outcome === "pass") passCount += 1;
      if (association.evidence_outcome === "fail") failCount += 1;
      if (association.evidence_outcome === "unknown") unknownCount += 1;
      associations.push(association);
    }
  }
  const finalPage = pages.at(-1)?.page;
  if (associations.length > first.associated_evidence_count || passCount > first.pass_count ||
    failCount > first.fail_count || unknownCount > first.unknown_count ||
    (!finalPage?.next_cursor && !finalPage?.truncated &&
      (associations.length !== first.associated_evidence_count || passCount !== first.pass_count ||
        failCount !== first.fail_count || unknownCount !== first.unknown_count))) {
    return { detail: first, associations: [],
      error: new Error("Evidence pages changed while loading; refresh the verification plan") };
  }
  return { detail: first, associations, error: undefined };
}

export function VerificationPlan({ client, runID }: {
  client: CyberAgentClient;
  runID: string;
}) {
  const queryClient = useQueryClient();
  const operationKey = useRef("");
  const receiptOperationKeys = useRef(new Map<string, string>());
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  const [items, setItems] = useState<DraftItem[]>([emptyItem()]);
  const [coverageSelection, setCoverageSelection] = useState<{
    planID: string;
    ordinal: number;
  } | null>(null);
  const query = useQuery({
    queryKey: ["run", runID, "verification-plan"],
    queryFn: ({ signal }) => client.verificationPlans(runID, signal),
    enabled: Boolean(runID),
  });
  const coverageQuery = useQuery({
    queryKey: ["run", runID, "verification-plan-coverage"],
    queryFn: ({ signal }) => client.verificationPlanCoverage(runID, signal),
    enabled: Boolean(runID),
  });
  const coverageDetailQuery = useInfiniteQuery({
    queryKey: ["run", runID, "verification-plan-coverage", coverageSelection?.planID,
      coverageSelection?.ordinal],
    initialPageParam: "",
    queryFn: ({ signal, pageParam }) => {
      if (!coverageSelection) throw new Error("A verification plan item is required");
      return client.verificationPlanItemCoveragePage(runID, coverageSelection.planID,
        coverageSelection.ordinal, pageParam, coveragePageSize, signal);
    },
    getNextPageParam: (lastPage) => lastPage.page.next_cursor || undefined,
    enabled: Boolean(runID && coverageSelection),
  });
  const receiptQuery = useQuery({
    queryKey: ["run", runID, "verification-snapshot-receipts"],
    queryFn: ({ signal }) => client.verificationSnapshotReceipts(runID, signal),
    enabled: Boolean(runID && coverageSelection),
  });
  const mergedCoverage = mergeCoveragePages(coverageDetailQuery.data?.pages ?? []);
  const exportSnapshot = useMutation({
    mutationFn: ({ planID, ordinal, format }: { planID: string; ordinal: number;
      format: "json" | "markdown" }) =>
      client.verificationPlanItemSnapshotExport(runID, planID, ordinal, format),
    onSuccess: (value) => downloadTextFile(value.filename, value.mime_type, value.content),
  });
  const recordSnapshotReceipt = useMutation({
    mutationFn: async ({ planID, ordinal, format }: { planID: string; ordinal: number;
      format: "json" | "markdown" }) => {
      const snapshot = await client.verificationPlanItemSnapshotExport(runID, planID,
        ordinal, format);
      const intent = [planID, ordinal, format, snapshot.snapshot_high_water_event_sequence,
        snapshot.content_sha256].join(":");
      let key = receiptOperationKeys.current.get(intent);
      if (!key) {
        key = `web-verification-snapshot-receipt-${globalThis.crypto.randomUUID()}`;
        receiptOperationKeys.current.set(intent, key);
      }
      const receipt = await client.recordVerificationSnapshotReceipt(runID, {
        version: "operator_verification_plan_item_snapshot_receipt.v1",
        plan_id: planID, plan_item_ordinal: ordinal, format,
        snapshot_high_water_event_sequence: snapshot.snapshot_high_water_event_sequence,
        content_sha256: snapshot.content_sha256, confirm_metadata_snapshot: true,
      }, key);
      return { intent, receipt };
    },
    onSuccess: ({ intent }) => {
      receiptOperationKeys.current.delete(intent);
      void queryClient.invalidateQueries({
        queryKey: ["run", runID, "verification-snapshot-receipts"],
      });
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "events"] });
    },
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
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "verification-plan-coverage"] });
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
        {coverageQuery.data && <StatusBadge
          status={`${coverageQuery.data.observed_plan_item_count}/${coverageQuery.data.plan_item_count} observed`} />}
        {receiptQuery.data && <StatusBadge status={`${receiptQuery.data.items.length} receipts`} />}
        <button aria-label="Refresh verification plans" className="icon-button"
          disabled={query.isFetching || coverageQuery.isFetching || coverageDetailQuery.isFetching ||
            receiptQuery.isFetching}
          onClick={() => {
            void query.refetch();
            void coverageQuery.refetch();
            if (coverageSelection) {
              void coverageDetailQuery.refetch();
              void receiptQuery.refetch();
            }
          }}
          title="Refresh" type="button"><RefreshCw aria-hidden="true"
            className={query.isFetching || coverageQuery.isFetching ||
              coverageDetailQuery.isFetching || receiptQuery.isFetching ? "spin" : ""}
            size={15} /></button></div>
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
    {coverageQuery.isError && <ErrorState error={coverageQuery.error} />}
    {query.data?.items.length === 0 && <EmptyState>No verification plan recorded</EmptyState>}
    {query.data && query.data.items.length > 0 && <div className="verification-plan-list">
      {query.data.items.map((plan) => <article key={plan.id}>
        <header><strong>{plan.title}</strong><StatusBadge status="guidance only" />
          <time dateTime={plan.created_at}>{formatDate(plan.created_at)}</time></header>
        <p>{plan.summary}</p><ol>{plan.items.map((item) => {
          const coverage = coverageQuery.data?.plans.find((entry) => entry.plan_id === plan.id)
            ?.items.find((entry) => entry.ordinal === item.ordinal);
          const selected = coverageSelection?.planID === plan.id &&
            coverageSelection.ordinal === item.ordinal;
          const snapshotReceipts = receiptQuery.data?.items.filter((receipt) =>
            receipt.plan_id === plan.id && receipt.plan_item_ordinal === item.ordinal) ?? [];
          return <li key={item.ordinal}>
            <strong>{item.title}</strong><span>{item.expected_observation}</span>
            <div className="verification-coverage-row">
              {coverage && <div className="verification-coverage-badges">
                {coverage.associated_evidence_count === 0 ? <StatusBadge status="unobserved" /> : <>
                  {coverage.pass_count > 0 && <StatusBadge status={`${coverage.pass_count} pass`} />}
                  {coverage.fail_count > 0 && <StatusBadge status={`${coverage.fail_count} fail`} />}
                  {coverage.unknown_count > 0 && <StatusBadge status={`${coverage.unknown_count} unknown`} />}
                </>}
              </div>}
              <button aria-expanded={selected} aria-label={`Inspect evidence for check ${item.ordinal}`}
                className="icon-button" onClick={() => setCoverageSelection((current) =>
                  current?.planID === plan.id && current.ordinal === item.ordinal ? null :
                    { planID: plan.id, ordinal: item.ordinal })}
                title="Inspect evidence references" type="button">
                <ListTree aria-hidden="true" size={14} />
              </button>
            </div>
            {selected && <div aria-label={`Evidence references for check ${item.ordinal}`}
              className="verification-coverage-detail">
              {coverageDetailQuery.isLoading && <LoadingState label="Loading evidence references" />}
              {coverageDetailQuery.isError && <ErrorState error={coverageDetailQuery.error} />}
              {mergedCoverage.error && <ErrorState error={mergedCoverage.error} />}
              {mergedCoverage.detail && !mergedCoverage.error && <>
                <header><strong>Evidence references</strong>
                  <span>{mergedCoverage.associations.length} of {mergedCoverage.detail.associated_evidence_count}</span>
                  {coverageDetailQuery.data?.pages.at(-1)?.page.truncated &&
                    <StatusBadge status="page limit reached" />}
                  <div className="verification-snapshot-actions">
                    <button aria-label={`Download check ${item.ordinal} verification snapshot as Markdown`}
                      className="compact-command" disabled={exportSnapshot.isPending}
                      onClick={() => exportSnapshot.mutate({ planID: plan.id,
                        ordinal: item.ordinal, format: "markdown" })} type="button">
                      <Download aria-hidden="true" size={13} />Markdown</button>
                    <button aria-label={`Download check ${item.ordinal} verification snapshot as JSON`}
                      className="compact-command" disabled={exportSnapshot.isPending}
                      onClick={() => exportSnapshot.mutate({ planID: plan.id,
                        ordinal: item.ordinal, format: "json" })} type="button">
                      <Download aria-hidden="true" size={13} />JSON</button>
                    {client.hasVerificationEvidence && <>
                      <button aria-label={`Record check ${item.ordinal} Markdown snapshot receipt`}
                        className="compact-command" disabled={recordSnapshotReceipt.isPending}
                        onClick={() => recordSnapshotReceipt.mutate({ planID: plan.id,
                          ordinal: item.ordinal, format: "markdown" })} type="button">
                        <FileCheck2 aria-hidden="true" size={13} />Receipt MD</button>
                      <button aria-label={`Record check ${item.ordinal} JSON snapshot receipt`}
                        className="compact-command" disabled={recordSnapshotReceipt.isPending}
                        onClick={() => recordSnapshotReceipt.mutate({ planID: plan.id,
                          ordinal: item.ordinal, format: "json" })} type="button">
                        <FileCheck2 aria-hidden="true" size={13} />Receipt JSON</button>
                    </>}
                  </div>
                </header>
                {exportSnapshot.error && <ErrorState error={exportSnapshot.error} />}
                {recordSnapshotReceipt.error && <ErrorState error={recordSnapshotReceipt.error} />}
                {receiptQuery.isError && <ErrorState error={receiptQuery.error} />}
                {snapshotReceipts.length > 0 &&
                  <div className="verification-snapshot-receipts"><strong>Snapshot receipts</strong>
                    <ul>{snapshotReceipts.map((receipt) => <li key={receipt.id}>
                      <StatusBadge status={receipt.format} />
                      <code title={receipt.content_sha256}>{receipt.content_sha256.slice(0, 12)}</code>
                      <span>events {receipt.snapshot_high_water_event_sequence} / {receipt.receipt_event_sequence}</span>
                      <time dateTime={receipt.recorded_at}>{formatDate(receipt.recorded_at)}</time>
                      <StatusBadge status="record only" />
                    </li>)}</ul>
                  </div>}
                {mergedCoverage.associations.length === 0 ?
                  <span className="verification-coverage-empty">No explicit evidence associated</span> :
                  <ul>{mergedCoverage.associations.map((association) =>
                    <li key={association.id}><StatusBadge status={association.evidence_outcome} />
                      <code title={association.evidence_id}>{association.evidence_id}</code>
                      <span>events {association.evidence_event_sequence} / {association.association_event_sequence}</span>
                      <time dateTime={association.associated_at}>{formatDate(association.associated_at)}</time>
                    </li>)}</ul>}
                {coverageDetailQuery.hasNextPage && <div className="verification-coverage-pagination">
                  <button className="compact-command" disabled={coverageDetailQuery.isFetchingNextPage}
                    onClick={() => void coverageDetailQuery.fetchNextPage()} type="button">
                    {coverageDetailQuery.isFetchingNextPage ?
                      <LoaderCircle aria-hidden="true" className="spin" size={14} /> :
                      <ChevronDown aria-hidden="true" size={14} />}
                    Load older evidence
                  </button>
                </div>}
              </>}
            </div>}
          </li>;
        })}</ol>
      </article>)}
    </div>}
  </section>;
}
