import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ClipboardCheck, LoaderCircle, RefreshCw } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { VerificationEvidenceRequestView } from "../api/types";
import { formatDate } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

export function VerificationEvidence({ client, runID }: {
  client: CyberAgentClient;
  runID: string;
}) {
  const queryClient = useQueryClient();
  const operationKey = useRef("");
  const [outcome, setOutcome] = useState<VerificationEvidenceRequestView["outcome"]>("unknown");
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  const query = useQuery({
    queryKey: ["run", runID, "verification-evidence"],
    queryFn: ({ signal }) => client.verificationEvidence(runID, signal),
    enabled: Boolean(runID),
  });
  const record = useMutation({
    mutationFn: (body: VerificationEvidenceRequestView) => {
      if (!operationKey.current) {
        operationKey.current = `web-verification-${globalThis.crypto.randomUUID()}`;
      }
      return client.recordVerificationEvidence(runID, body, operationKey.current);
    },
    onSuccess: () => {
      operationKey.current = "";
      setTitle("");
      setSummary("");
      setOutcome("unknown");
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "verification-evidence"] });
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "code-handoff"] });
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "events"] });
    },
  });
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedTitle = title.trim();
    const normalizedSummary = summary.trim();
    if (!normalizedTitle || !normalizedSummary || record.isPending) return;
    record.mutate({ version: "operator_verification_evidence.v1", outcome,
      title: normalizedTitle, summary: normalizedSummary });
  };
  return <section aria-label="Verification evidence" className="verification-evidence">
    <header className="operator-list-header">
      <div><ClipboardCheck aria-hidden="true" size={16} /><h2>Verification</h2></div>
      <div>
        {query.data && <>
          <StatusBadge status={`${query.data.pass_count} pass`} />
          {query.data.fail_count > 0 && <StatusBadge status={`${query.data.fail_count} fail`} />}
        </>}
        <button aria-label="Refresh verification evidence" className="icon-button"
          disabled={query.isFetching} onClick={() => void query.refetch()}
          title="Refresh" type="button">
          <RefreshCw aria-hidden="true" className={query.isFetching ? "spin" : ""} size={15} />
        </button>
      </div>
    </header>
    {client.hasVerificationEvidence && <form className="verification-form" onSubmit={submit}>
      <label>Result<select aria-label="Verification result" value={outcome}
        onChange={(event) => setOutcome(event.target.value as VerificationEvidenceRequestView["outcome"])}>
        <option value="pass">Pass</option><option value="fail">Fail</option>
        <option value="unknown">Unknown</option>
      </select></label>
      <label>Title<input maxLength={160} required value={title}
        onChange={(event) => setTitle(event.target.value)} /></label>
      <label className="verification-summary">Summary<textarea maxLength={2048} required rows={3}
        value={summary} onChange={(event) => setSummary(event.target.value)} /></label>
      <button className="command-button" disabled={record.isPending || !title.trim() || !summary.trim()}
        type="submit">{record.isPending ? <LoaderCircle aria-hidden="true" className="spin" size={15} /> :
          <ClipboardCheck aria-hidden="true" size={15} />}Record</button>
    </form>}
    {record.error && <ErrorState error={record.error} />}
    {query.isLoading && <LoadingState label="Loading verification evidence" />}
    {query.isError && <ErrorState error={query.error} />}
    {query.data?.items.length === 0 && <EmptyState>No verification evidence recorded</EmptyState>}
    {query.data && query.data.items.length > 0 && <div className="verification-list">
      {query.data.items.map((item) => <article key={item.id}>
        <header><StatusBadge status={item.outcome} /><strong>{item.title}</strong>
          <time dateTime={item.recorded_at}>{formatDate(item.recorded_at)}</time></header>
        <p>{item.summary}</p>
      </article>)}
    </div>}
  </section>;
}
