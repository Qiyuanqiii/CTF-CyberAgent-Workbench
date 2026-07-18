import { useRef, useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { LoaderCircle, SendHorizontal } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { RunView, SessionMessageControlRequestView,
  SessionMessageControlView } from "../api/types";
import { StatusBadge } from "./common";

const maximumContentBytes = 16 * 1024;

interface RetryIntent {
  fingerprint: string;
  key: string;
}

export function SessionComposer({ client, sessionID, run }: {
  client: CyberAgentClient;
  sessionID: string;
  run: RunView | null;
}) {
  const [content, setContent] = useState("");
  const [lastResult, setLastResult] = useState<SessionMessageControlView | null>(null);
  const retryIntent = useRef<RetryIntent | null>(null);
  const queryClient = useQueryClient();
  const mutation = useMutation({
    mutationFn: ({ request, key }: { request: SessionMessageControlRequestView; key: string }) =>
      client.submitSessionMessage(sessionID, request, key),
    onSuccess: (result) => {
      retryIntent.current = null;
      setContent("");
      setLastResult(result);
      void queryClient.invalidateQueries({ queryKey: ["run", result.run_id] });
      void queryClient.invalidateQueries({ queryKey: ["runs"] });
      void queryClient.invalidateQueries({ queryKey: ["session", sessionID] });
    },
  });

  if (!client.hasSessionMessages || !run) {
    return null;
  }

  const normalized = content.trim();
  const contentBytes = new TextEncoder().encode(normalized).byteLength;
  const contentTooLarge = contentBytes > maximumContentBytes;
  const mutable = run.status === "running" || run.status === "paused";
  const ready = mutable && contentBytes > 0 && !contentTooLarge && !mutation.isPending;

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready) {
      return;
    }
    const request: SessionMessageControlRequestView = {
      version: "session_message_submission.v1",
      content: normalized,
    };
    const fingerprint = JSON.stringify({ sessionID, request });
    if (retryIntent.current?.fingerprint !== fingerprint) {
      retryIntent.current = {
        fingerprint,
        key: `web-session-message-${globalThis.crypto.randomUUID()}`,
      };
    }
    mutation.mutate({ request, key: retryIntent.current.key });
  };

  const changeContent = (value: string) => {
    setContent(value);
    setLastResult(null);
    mutation.reset();
  };

  return (
    <form className="session-composer" onSubmit={submit}>
      <textarea aria-label="Session message" autoComplete="off"
        disabled={!mutable || mutation.isPending} onChange={(event) => changeContent(event.target.value)}
        placeholder="Message this Run" rows={3} spellCheck value={content} />
      <div className="session-composer-footer">
        <div className="session-composer-state" aria-live="polite">
          {!mutable && <><StatusBadge status={run.status} /><span>Run unavailable</span></>}
          {contentTooLarge && <span className="connection-error">Message exceeds 16384 UTF-8 bytes</span>}
          {mutation.isError && <span className="connection-error">{errorMessage(mutation.error)}</span>}
          {lastResult && <><StatusBadge status={lastResult.steering.status} />
            <span>Queued #{lastResult.steering.sequence}{lastResult.replayed ? " · replayed" : ""}</span></>}
        </div>
        <span className={contentTooLarge ? "byte-count invalid" : "byte-count"}>
          {contentBytes} / {maximumContentBytes} bytes
        </span>
        <button aria-label="Queue message" className="session-send-button" disabled={!ready}
          title="Queue message" type="submit">
          {mutation.isPending ? <LoaderCircle aria-hidden="true" className="spin" size={17} /> :
            <SendHorizontal aria-hidden="true" size={17} />}
        </button>
      </div>
    </form>
  );
}

function errorMessage(value: unknown): string {
  return value instanceof Error && value.message.trim() ? value.message : "Session message failed";
}
