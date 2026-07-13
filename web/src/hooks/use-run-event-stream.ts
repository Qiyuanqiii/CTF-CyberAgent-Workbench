import { useEffect, useState } from "react";
import { APIRequestError, type CyberAgentClient } from "../api/client";
import type { RunEventStreamView } from "../api/types";

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
