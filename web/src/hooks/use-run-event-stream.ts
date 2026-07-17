import { useEffect, useState } from "react";
import { APIRequestError, type CyberAgentClient } from "../api/client";
import type { RunEventStreamView } from "../api/types";
import { desktopRuntimeActive } from "../lib/desktop-bridge";

export type StreamStatus = "connecting" | "live" | "reconnecting" | "stopped";

const reconnectDelayMs = 1_000;
const maxLiveFrames = 500;
const maxRememberedRuns = 16;

interface RememberedDesktopRun {
  cursor: string;
  frames: RunEventStreamView[];
}

const rememberedDesktopRuns = new Map<string, RememberedDesktopRun>();

export function clearDesktopRunEventMemory(runID = ""): void {
  if (runID) {
    rememberedDesktopRuns.delete(runID);
    return;
  }
  rememberedDesktopRuns.clear();
}

function rememberDesktopRun(runID: string, cursor: string, frames: RunEventStreamView[]): void {
  rememberedDesktopRuns.delete(runID);
  rememberedDesktopRuns.set(runID, { cursor, frames: [...frames].slice(-maxLiveFrames) });
  while (rememberedDesktopRuns.size > maxRememberedRuns) {
    const oldest = rememberedDesktopRuns.keys().next().value as string | undefined;
    if (!oldest) {
      break;
    }
    rememberedDesktopRuns.delete(oldest);
  }
}

function mergeFrames(current: RunEventStreamView[], incoming: RunEventStreamView[]): RunEventStreamView[] {
  const bySequence = new Map(current.map((frame) => [frame.sequence, frame]));
  for (const frame of incoming) {
    bySequence.set(frame.sequence, frame);
  }
  return [...bySequence.values()]
    .sort((left, right) => left.sequence - right.sequence)
    .slice(-maxLiveFrames);
}

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
    setError("");
    if (!runID) {
      setFrames([]);
      setStatus("stopped");
      return;
    }

    const controller = new AbortController();
    if (desktopRuntimeActive()) {
      const remembered = rememberedDesktopRuns.get(runID);
      let cursor = remembered?.cursor ?? "";
      let currentFrames = remembered?.frames ?? [];
      let cursorResetUsed = false;
      setFrames(currentFrames);
      const poll = async () => {
        setStatus("connecting");
        let immediatePages = 0;
        while (!controller.signal.aborted) {
          try {
            const page = await client.pollRunEvents(
              runID,
              cursor,
              100,
              controller.signal,
            );
            if (controller.signal.aborted) {
              return;
            }
            cursor = page.cursor;
            currentFrames = mergeFrames(currentFrames, page.frames);
            rememberDesktopRun(runID, cursor, currentFrames);
            setFrames(currentFrames);
            setStatus("live");
            setError("");
            if (page.has_more) {
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
            if (caught instanceof APIRequestError && caught.status === 400 && cursor !== "" &&
              !cursorResetUsed) {
              cursorResetUsed = true;
              cursor = "";
              currentFrames = [];
              clearDesktopRunEventMemory(runID);
              setFrames([]);
              continue;
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
    setFrames([]);
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
