# ADR 0049: Deadlock And Livelock Runtime Guards

- Status: Accepted
- Date: 2026-07-19
- Scope: H1 Tool termination boundary, H2 synchronous wait graph, H3 schema v79 Run progress guard

## Context

The runtime already bounded root and Specialist turns, child count, token/model-time
budgets, execution leases, and cancellation fan-out. Three different blocking classes
still needed explicit contracts:

1. an in-process Tool could ignore cancellation, panic, or block while reading a FIFO;
2. an Agent, Tool, Retriever, Store, or Runner could synchronously call back through a
   dependency already waiting for it; and
3. a healthy process could repeatedly complete turns without changing durable work,
   consuming budget forever without a thread-level deadlock.

Real Local and Docker process execution remains disabled. This decision therefore
hardens the enabled Go control plane without claiming that a future OS process tree or
container lifecycle is already protected.

## Decision

### H1: every in-process Tool call has a termination boundary

`tools.Registry` applies a 15-second deadline by default and rejects configured
deadlines above five minutes. Caller cancellation wins over the internal timeout,
timeouts return exit 124, cancellations return exit 130, and panics become bounded
errors. A buffered completion channel lets the caller return even if Tool code does not
cooperate.

Built-in workspace reads check context while walking or reading. `max_bytes` cannot
exceed the configured filesystem limit or the platform integer boundary. Unix opens
use nonblocking, close-on-exec, no-follow semantics and verify the descriptor is a
regular file; other platforms compare pre-open and opened-file identity. FIFOs,
devices, sockets, and other special files fail instead of blocking.

The deadline does not kill an arbitrary goroutine. Built-in Tools honor context, and
future external Tools must do the same. Real Runner implementations must use a
separate process-tree start/wait/TERM/KILL/orphan state machine.

### H2: synchronous dependencies are explicit and acyclic

`waitgraph.Graph` records reference-counted synchronous edges between bounded Agent,
Tool, Retriever, Store, Runner, Model, and external identities. It allows at most 4,096
active nodes and 8,192 active edges. Before inserting an edge, a bounded graph search
rejects self, direct, and indirect cycles. Release is idempotent.

Tool, Retriever, Store, and Runner nodes may never synchronously wait on an Agent,
even before a full cycle is visible. The root Supervisor injects its Agent identity,
the Specialist scheduler records parent-to-child waits, and Tool Gateway invocation
records a unique Tool edge and propagates that Tool identity. Future RAG, Store
callback, Model adapter, and Runner code must enter the same graph before making a
synchronous dependency.

### H3: no-progress loops pause durably

Schema v79 adds one `run_progress_guard.v1` row per observed Run. It persists only
SHA-256 fingerprints of normalized `continue` actions and selected structured state,
bounded counters, fixed thresholds, reason code, turn, and timestamps. Model text,
tool output, file content, credentials, and prompts are not stored in the guard.

Three identical `continue` actions detect `repeated_action`. Six `continue` turns with
no selected structured-state change detect `no_observable_progress`. Counters saturate
at the fixed thresholds. A detected turn atomically commits its Session messages,
model/tool facts, progress events, waiting checkpoint, and `running -> paused` Run
transition. The projected action becomes `wait` with a stable recoverable reason.

Completion replay using the Provider's original `continue` recognizes that it was
already converted to a guarded wait and commits nothing twice. A detected guard can
return to observing only after a later durable `paused -> running` event proves an
explicit operator resume. Migration from v78 creates the table and triggers but no
synthetic guard, event, action, or progress claim.

## Consequences

- Ordinary Tool callers no longer wait forever on enabled built-ins, and panic cannot
  tear down the Agent process.
- Parent/child and Tool callback cycles fail before blocking and leave no stale edge.
- Repeated model turns become a recoverable operator-visible pause rather than an
  unlimited budget drain.
- v79 changes no Policy, approval, Scope, network, Shell, LocalRunner, Docker, or child
  admission authority.
- A third-party goroutine that ignores context may remain alive after its caller
  returns. Future plugin isolation should prefer a cancellable process boundary.
- Real process handle/port deadlocks remain out of scope while real Local/Docker
  execution is disabled; their independent lifecycle gate remains mandatory.

## Verification

The final uncached Go suite passed in 312 seconds, including the Store package in
304.6 seconds. The final-code full race suite passed in 358 seconds. Tool and wait-graph
tests passed 20 repetitions; v79 progress-guard Store tests passed ten repetitions.
Ordinary and secure-Desktop tests/vet, zero-warning staticcheck, module verification
and tidy diff, and govulncheck with no reachable finding passed. The only retained
module-level advisory is `GO-2026-5932`; the application neither imports nor calls the
affected package.

All 120 React tests across 35 files, strict TypeScript, deterministic API generation,
the Vite production build, and zero-vulnerability npm audit passed. A reproducible
unsigned Windows Desktop build produced SHA-256
`31e0df63d3fbbccac6728ad2322196bee55d57e775a15cc34f752c0632bdc699` and correctly
remains `release_ready=false`. Read-only production-bundle browser checks found no
horizontal overflow at desktop or 390 x 844 mobile widths and no console error.

The audit hardened the platform-integer/OOM read limit, Go and SQL observing-state
threshold constraints, explicit resume proof, fail-closed corrupt-record reads, and
saturating counters. No unresolved high- or medium-severity issue is known on an
enabled path. No real key, Provider request, Shell, LocalRunner, Docker, attack
traffic, or external network request was used.

## 中文结论

本决策把三类“看起来都像卡住”的问题分开处理：Tool 调用用硬超时、取消、panic
恢复和普通文件校验兜底；Agent/Tool/RAG/Store/Runner 的同步依赖用有界等待图在阻塞前
拒绝环路和低层反向回调；模型仍能正常返回但连续没有结构化进展时，由 schema v79
在同一事务内暂停 Run，并要求操作者显式恢复。v79 不开放任何 Shell、网络、Local 或
Docker 权限，也不声称已经解决尚未启用的真实进程级死锁。
