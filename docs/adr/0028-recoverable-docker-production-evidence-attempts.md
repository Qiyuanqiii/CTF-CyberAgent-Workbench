# ADR 0028: Recoverable Docker Production-Evidence Attempts

## Status

Accepted for schema v66. This decision adds durable ownership and recovery
before a production-evidence collector is invoked. It does not enable Docker
daemon contact, container start, host execution, or evidence acceptance.

## 中文摘要

schema v66 为 v65 生产证据捕获增加写前 `attempt -> generation lease ->
reconciliation -> failure/result` 链路。Go 必须先原子提交不可变 attempt、摘要化
operation 和第一代租约，再为当前 generation 写入 quiescent reconciliation
checkpoint，随后才能调用产品 collector。失败只写有界类型码并释放租约；重试或
崩溃恢复必须获得下一 generation，旧 worker 不能再提交 checkpoint、失败或证据。

当前 checkpoint 只记录零 daemon 读取、零已知资源和固定 Linux 本机 Unix endpoint，
用于证明调用顺序和恢复所有权，不声称已经检查真实 Docker 资源。当前 collector 仍
只返回 `unsupported_platform`、`opt_in_required` 或 `harness_pending`，并且 Application
继续拒绝 `capture_complete` 和 `real_daemon_contacted=true`。所有 daemon、start、process、
output 和 Artifact 权限位固定为 `false`。

## Context

Schema v65 made evidence immutable and non-authorizing, but its original flow
invoked the collector before the evidence transaction. That was acceptable for
the built-in inert collector, yet it was not a sufficient boundary for a future
daemon-contacting Linux harness: a process crash could lose ownership and a
retry could duplicate an external effect.

The existing v56, v61, and v62 Sandbox paths already establish the repository's
generation-fenced recovery pattern. Production evidence needs the same durable
ordering before its collector can ever gain a real transport.

## Decision

Schema v66 stores:

- one immutable capture attempt per digest-only operation identity;
- one replaceable lease row with monotonically increasing generation;
- one immutable quiescent reconciliation checkpoint per executed generation;
- up to sixteen append-only typed failures per attempt;
- one immutable result binding the attempt, current reconciliation, and v65
  evidence capture fingerprint.

The attempt binds the exact blocked v63 review, Run, Mission, Workspace,
operator, authority, threat model, fixed probe suite, local-Unix endpoint,
bounded capture timeout, and all false authority bits. A Run may hold at most
32 production-evidence attempts.

The Application sequence is:

1. Validate the same operator and blocked v63 review.
2. Commit the attempt, operation identity, and generation-one lease.
3. Commit the current-generation quiescent reconciliation checkpoint.
4. Derive a deadline bounded by both the attempt timeout and lease expiry.
5. Invoke the collector with attempt ID, generation, fixed endpoint identity,
   and deadline.
6. On failure, append a typed code and release the current lease atomically.
7. On success, atomically store v65 evidence, attempt result, v65 operation,
   lease release, and metadata-only events.

A released or expired lease can be replaced only by generation `N+1`. An
active lease conflicts. Every terminal write compares the full private lease
identity and generation; stale workers fail closed. Resume requires fresh
explicit operator confirmation. CLI and events do not expose lease IDs or
owners.

## SQL Enforcement And Compatibility

A v66 trigger rejects every new v65 evidence operation unless an earlier
attempt operation and matching attempt result already exist in the same
transaction. This prevents the legacy Store creation method from creating a
new receipt without write-ahead ownership, while preserving idempotent replay
of a completed receipt.

Migration does not fabricate attempts for existing v65 evidence. Legacy
receipts remain readable and replayable. New attempt and reconciliation rows,
operations, failures, and results are immutable; only the constrained lease
transition may update. Lease rows cannot be deleted. A release must precede
expiry, and generation `N+1` cannot acquire before generation `N` released or
expired.

## Failure And Deadline Boundary

Persisted failure codes are bounded to collector failure, invalid observation,
unsafe daemon contact, cancellation, deadline, and persistence failure. Raw
errors, endpoints, sockets, paths, resource names, container IDs, lease
identities, and daemon payloads are not stored.

The default capture timeout is 30 seconds. A collector context ends no later
than the lease expiry minus a safety margin. The built-in collector also
rejects an already expired request before collecting.

## Security Consequences

- A product collector call cannot precede durable attempt ownership and a
  current-generation checkpoint.
- A stale generation cannot record a failure, result, or evidence operation.
- Same-key completion replays without calling the collector.
- A malicious or future collector result claiming real daemon contact is
  recorded as `unsafe_daemon_contact` and rejected; no evidence row is written.
- The quiescent checkpoint is not daemon resource reconciliation. It records
  that the current product path has no daemon capability and knows of no
  resources. A future real harness needs a separately audited daemon-aware
  reconciliation protocol before this claim can change.
- Installing arbitrary Go code could still replace or subvert an in-process
  collector. This ADR secures the product control flow and persistence
  boundary; it is not a sandbox for compromised application code.

## Validation

Focused tests cover domain invariants, bounded deadlines, write-ahead ordering
observable from inside the collector, SQL bypass rejection, operation binding,
active-lease conflict, released retry, expired takeover, stale-generation
fencing, typed unsafe-contact failure, generation-two recovery, immutable
rows, metadata-only CLI list/show/resume, completed replay without recollection,
v64-to-latest migration, and v65 legacy evidence preservation without attempt
backfill. Direct-SQL regressions also cover lease deletion, post-expiry release,
and operation creation without an attempt result.

## Follow-Up

The next Docker slice may implement a Linux-only sixteen-probe harness, but
only through an exact pre-existing digest image with no pull, a fixed local
endpoint, bounded transport operations, and daemon-aware restart
reconciliation. Its receipt must remain non-authorizing. Evidence acceptance,
the v63 start/wait/TERM/KILL/orphan lifecycle, output export, atomic Artifact
commit, and Local OS sandboxing remain separate release gates.
