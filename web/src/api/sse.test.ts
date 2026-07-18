import { consumeSSE, type SSEMessage } from "./sse";

function streamChunks(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      for (const chunk of chunks) {
        controller.enqueue(encoder.encode(chunk));
      }
      controller.close();
    },
  });
}

describe("consumeSSE", () => {
  it("parses split CRLF frames, comments, and multiline data", async () => {
    const messages: SSEMessage[] = [];
    await consumeSSE(streamChunks([
      ": hello\r", "\nretry: 1000\r\n\r\n",
      "id: cursor-1\nevent: run.event\ndata: {\"part\":\ndata: true}\n", "\n",
    ]), (message) => messages.push(message));

    expect(messages).toEqual([{
      id: "cursor-1",
      event: "run.event",
      data: "{\"part\":\ntrue}",
    }]);
  });

  it("retains the last event id and ignores ids containing NUL", async () => {
    const messages: SSEMessage[] = [];
    await consumeSSE(streamChunks([
      "id: cursor-1\ndata: one\n\nid: bad\0id\ndata: two\n\n",
    ]), (message) => messages.push(message));

    expect(messages.map((message) => message.id)).toEqual(["cursor-1", "cursor-1"]);
  });

  it("rejects an event above the configured byte limit", async () => {
    await expect(consumeSSE(streamChunks(["data: 123456\n\n"]), () => undefined, 8))
      .rejects.toThrow("configured client limit");
  });

  it("cancels the response body when frame validation fails", async () => {
    let cancelled = false;
    const encoder = new TextEncoder();
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode("event: run.event\ndata: invalid\n\n"));
      },
      cancel() {
        cancelled = true;
      },
    });

    await expect(consumeSSE(stream, () => {
      throw new Error("invalid frame");
    })).rejects.toThrow("invalid frame");
    expect(cancelled).toBe(true);
  });
});
