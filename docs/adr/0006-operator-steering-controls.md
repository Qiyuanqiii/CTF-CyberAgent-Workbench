# ADR 0006: Operator Steering Controls

- Status: Accepted
- Date: 2026-07-14

## Context

Schema v45 can durably queue new operator guidance but intentionally has no cancellation or explicit wake path. Operators need to withdraw stale pending input, retry a Session submission across process failure, and deliberately drain a paused or idle queue. These controls must not race a prepared delivery, steal another worker's Run, replay unrelated failed input, or become a new model/HTTP/child execution surface.

## Decision

Go owns three narrow queue controls in schema v46.

- Pending cancellation creates one immutable `operator_steering_cancellations` fact and, for operator requests, one immutable digest-keyed operation. Exact retries return the same fact; changed intent conflicts.
- An operator cancellation is valid only for a pending message with no prepared delivery while its Run is running or paused. Queue content and order remain immutable.
- Failed or cancelled Runs create bounded `run_terminal` cancellation facts and close prepared deliveries/messages in the same lifecycle transaction. System failure text is redacted, repaired to valid UTF-8, control-normalized, and bounded before persistence so audit text cannot block terminal state.
- A caller-supplied Session operation key always creates or replays steering for a Run-bound Session. This path never performs a synchronous Provider call and does not apply to slash commands or unbound Sessions.
- Explicit drain is bounded to 1-64 queued turns. It acquires the Run execution lease before waking a paused Run, then uses a steering-only Supervisor begin operation. An empty queue does not wake; a conflicting lease does not change Run state.
- The steering-only begin operation may recover an exact prepared steering attempt or prepare the oldest pending message. It cannot generate default Mission input or recover an unrelated failed ordinary turn.
- CLI is the only control surface. HTTP/OpenAPI, React, TUI, models, Tool Gateway calls, and child Agents remain unable to cancel, wake, or drain steering.

## Invariants

1. One message has at most one cancellation fact. Operator cancellation also requires its exact operation ledger before the message may enter `cancelled`.
2. Raw operation keys never enter SQLite or events. Cancellation reason and requester identity remain out of Run event payloads.
3. A prepared message is non-cancellable by an operator. Delivery commit/recovery remains owned by the existing Supervisor transaction.
4. Drain uses the same execution lease, turn/token/time budget, Policy, Tool Gateway, and lifecycle checks as ordinary root execution.
5. Drain cannot create a turn when no queue-backed input exists and cannot recover a non-steering checkpoint.
6. Queue controls grant no tool, Shell, process, network, file-write, approval, Sandbox, delegation, or child-Agent capability.

## Consequences

Operators can safely withdraw stale pending guidance, deliberately wake and process queued work, and retry ordinary Session submissions after an uncertain client/process outcome. The stronger audit ledger adds schema state but no public control API. A drain invocation is an explicit local execution command rather than a background worker; real Sandbox execution and CTF automation remain separately gated.
