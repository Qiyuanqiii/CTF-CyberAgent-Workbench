# ADR 0005: Durable Operator Steering Queue

- Status: Accepted
- Date: 2026-07-14

## Context

An operator must be able to add requirements while a Run is executing. Writing directly into the active model/tool transaction would race with PendingInput, Session history, execution-lease fencing, lifecycle actions, and budget commits. Rejecting every busy input also loses the queue-oriented workflow expected from modern coding agents.

## Decision

Go owns one durable, Run-scoped operator steering queue.

- Enqueue persists bounded, redacted, normalized operator text under a digest-only idempotency operation. It never interrupts an active Provider or tool call.
- Each Run has a stable arrival sequence. The Supervisor may prepare only the oldest pending item and only while opening a root turn.
- A delivery binds one message to one exact Supervisor attempt and turn. Failed attempts supersede that delivery without consuming the message.
- A successful lifecycle transaction commits the operator Session message, assistant response, delivery, and queue state together. There is no early Session write.
- Pending successor input defers model-requested `finish` or `wait` to an effective Go-owned `continue`. Run completion fails closed while pending steering exists.
- Failed or cancelled Runs cancel outstanding steering. Models and child Agents cannot enqueue or mutate the queue.
- Local CLI detail may read content. HTTP/OpenAPI, React, and TUI projections expose metadata only and have no queue mutation.

## Invariants

1. Steering is operator authority only after Go validates its provenance; queue contents never become system authority merely because they are stored.
2. Queueing grants no tool, Shell, network, workspace-write, approval, Sandbox, or child-Agent capability.
3. One message can have at most one prepared delivery and one committed delivery. A committed message binds exactly one authorized Session user message.
4. Queue order is immutable. Schema v45 provides no edit, reorder, or pending-cancellation operation.
5. Execution-lease fencing and the existing Supervisor transaction remain the only commit boundary; the queue is not a second execution engine.

## Consequences

Busy Session/TUI input can be acknowledged durably without fabricating a model response. Recovery and replay are inspectable through append-only events and SQLite facts. A worker is still required to drain queued input; schema v46 will define pending-only cancellation and explicit idle/paused wake behavior without relaxing these invariants.
