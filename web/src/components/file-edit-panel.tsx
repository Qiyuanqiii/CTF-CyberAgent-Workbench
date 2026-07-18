import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, FileDiff, LoaderCircle, X } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { FileEditReviewRequestView } from "../api/types";
import { formatDate, shortID } from "../lib/format";
import { EmptyState, ErrorState, LoadingState, StatusBadge } from "./common";

export function FileEditPanel({ client, runID }: { client: CyberAgentClient; runID: string }) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["run", runID, "file-edits"],
    queryFn: ({ signal }) => client.fileEditQueue(runID, signal),
  });
  const review = useMutation({
    mutationFn: ({ editID, action }: { editID: string;
      action: FileEditReviewRequestView["action"] }) =>
      client.reviewFileEdit(runID, editID, { version: "file_edit_review.v1", action }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "file-edits"] });
      void queryClient.invalidateQueries({ queryKey: ["run", runID, "events"] });
    },
  });
  if (query.isLoading) return <LoadingState label="Loading file edit previews" />;
  if (query.isError || !query.data) return <ErrorState error={query.error} />;
  if (query.data.items.length === 0) return <EmptyState>No file edit proposals</EmptyState>;
  return <section className="file-edit-panel" aria-label="File edit previews">
    <header className="projection-heading">
      <div><FileDiff aria-hidden="true" size={17} /><h2>Diff review</h2></div>
      <span>{query.data.items.length}{query.data.truncated ? "+" : ""}</span>
    </header>
    <div className="file-edit-list">
      {query.data.items.map((edit) => {
        const active = review.isPending && review.variables?.editID === edit.id;
        return <details className="file-edit-row" key={edit.id} open={edit.status === "proposed" || undefined}>
          <summary>
            <code>{edit.path}</code>
            <span>{shortID(edit.id)}</span>
            {edit.secrets_redacted && <span>redacted</span>}
            <StatusBadge status={edit.status} />
          </summary>
          <div className="file-edit-body">
            <pre>{edit.diff}</pre>
            <footer>
              <time dateTime={edit.updated_at}>{formatDate(edit.updated_at)}</time>
              <span>Apply authority: disabled</span>
              {client.hasFileEditReview && edit.allowed_actions.length > 0 && <div>
                {edit.allowed_actions.includes("approve_intent") &&
                  <button aria-label={`Approve intent ${edit.path}`} className="icon-button"
                    disabled={review.isPending}
                    onClick={() => review.mutate({ editID: edit.id, action: "approve_intent" })}
                    title="Approve intent without writing the file" type="button">
                    {active && review.variables?.action === "approve_intent"
                      ? <LoaderCircle aria-hidden="true" className="spin" size={15} />
                      : <Check aria-hidden="true" size={15} />}
                  </button>}
                {edit.allowed_actions.includes("deny") &&
                  <button aria-label={`Deny ${edit.path}`} className="icon-button"
                    disabled={review.isPending}
                    onClick={() => review.mutate({ editID: edit.id, action: "deny" })}
                    title="Deny file edit" type="button">
                    {active && review.variables?.action === "deny"
                      ? <LoaderCircle aria-hidden="true" className="spin" size={15} />
                      : <X aria-hidden="true" size={15} />}
                  </button>}
              </div>}
            </footer>
          </div>
        </details>;
      })}
    </div>
    {review.isError && <div className="inline-warning" role="alert">
      {review.error instanceof Error ? review.error.message : "File edit review failed"}
    </div>}
  </section>;
}
