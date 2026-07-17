import { useEffect, useState } from "react";
import { APIRequestError, type CyberAgentClient } from "../api/client";
import type { EventView, RunEventStreamView } from "../api/types";
import { desktopRuntimeActive } from "../lib/desktop-bridge";

export type StreamStatus = "connecting" | "live" | "reconnecting" | "stopped";

const reconnectDelayMs = 1_000;
const maxLiveFrames = 500;

function delay(signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timeout = window.setTimeout(resolve, reconnectDelayMs);
    signal.addEventListener("abort", () => {
      window.clearTimeout(timeout);
      resolve();
    }, { once: true });
  });
}

export function useRunEventStream(client: CyberAgentClient, runID: string) {
  const [frames, setFrames] = useState<RunEventStreamView[]>([]);
  const [status, setStatus] = useState<StreamStatus>("stopped");
  const [error, setError] = useState("");

  useEffect(() => {
    setFrames([]);
    setError("");
    if (!runID) {
      setStatus("stopped");
      return;
    }

    const controller = new AbortController();
    let cursor = "";
    if (desktopRuntimeActive()) {
      const poll = async () => {
        setStatus("connecting");
        let immediatePages = 0;
        while (!controller.signal.aborted) {
          try {
            const page = await client.getPage<EventView>(
              `/runs/${encodeURIComponent(runID)}/events`,
              { limit: 100 },
              cursor,
              controller.signal,
            );
            const polledFrames: RunEventStreamView[] = page.items.map((event) => ({
              version: "run-events.v1",
              request_id: page.requestID,
              run_id: runID,
              sequence: event.sequence,
              cursor: `desktop-poll-${event.sequence}`,
              event,
            }));
            setFrames((current) => {
              const bySequence = new Map(current.map((frame) => [frame.sequence, frame]));
              for (const frame of polledFrames) {
                bySequence.set(frame.sequence, frame);
              }
              return [...bySequence.values()]
                .sort((left, right) => left.sequence - right.sequence)
                .slice(-maxLiveFrames);
            });
            setStatus("live");
            setError("");
            if (page.page.next_cursor) {
              cursor = page.page.next_cursor;
              immediatePages++;
              if (immediatePages < 4) {
                continue;
              }
            }
            immediatePages = 0;
          } catch (caught) {
            if (controller.signal.aborted) {
              return;
            }
            setError(caught instanceof Error ? caught.message : "Event polling disconnected");
            if (caught instanceof APIRequestError && [400, 401, 403, 404].includes(caught.status)) {
              setStatus("stopped");
              return;
            }
            setStatus("reconnecting");
          }
          await delay(controller.signal);
        }
      };
      void poll();
      return () => {
        controller.abort();
        setStatus("stopped");
      };
    }
    const run = async () => {
      setStatus("connecting");
      while (!controller.signal.aborted) {
        try {
          await client.streamRunEvents(runID, {
            cursor,
            signal: controller.signal,
            onFrame: (frame) => {
              cursor = frame.cursor;
              setStatus("live");
              setError("");
              setFrames((current) => {
                if (current.some((item) => item.sequence === frame.sequence)) {
                  return current;
                }
                return [...current, frame].slice(-maxLiveFrames);
              });
            },
          });
          if (!controller.signal.aborted) {
            setStatus("reconnecting");
            await delay(controller.signal);
          }
        } catch (caught) {
          if (controller.signal.aborted) {
            return;
          }
          setError(caught instanceof Error ? caught.message : "Event stream disconnected");
          if (caught instanceof APIRequestError && [400, 401, 403, 404].includes(caught.status)) {
            setStatus("stopped");
            return;
          }
          setStatus("reconnecting");
          await delay(controller.signal);
        }
      }
    };
    void run();

    return () => {
      controller.abort();
      setStatus("stopped");
    };
  }, [client, runID]);

  return { frames, status, error };
}
