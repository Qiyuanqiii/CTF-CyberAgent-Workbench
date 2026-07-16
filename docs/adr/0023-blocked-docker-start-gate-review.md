# ADR 0023: Blocked Docker Process Start-Gate Review

## Status

Accepted for schema v63 as a design-only, non-authorizing review.

## 中文摘要

schema v63 只回答“为什么现在仍不能启动容器进程”。它把 v51 的 16 项威胁检查逐项绑定到 v52-v62 已有证据、明确的生产证据缺口和未来独立门禁，并冻结一份 11 状态转换的进程生命周期蓝图。所有检查仍不充分，所有转换仍未实现且未授权；成功创建审查记录只会得到 `blocked/deny_start`，不会访问 Docker、重捕获输入或创建进程。

## Context

Schemas v52-v62 progressively added simulation, read-only Docker observation,
container specification compilation, never-started daemon rehearsals,
descriptor-sealed input capture, daemon-owned input handoff, deterministic input
projection, never-started target preparation, and exact-owned cleanup. Those
facts reduce uncertainty around a future container path, but none observes or
controls a running process.

The sixteen requirements frozen by schema v51 therefore remain the governing
threat model. A start implementation must not infer production verification
from a simulation, a compiled control, a stopped-container inspection, or a
successful cleanup. It also needs an explicit ownership and recovery protocol
before a daemon mutation surface can be considered.

## Decision

Schema v63 introduces immutable
`sandbox_docker_start_gate_review.v1` records. Creation requires a completed
schema-v62 cleanup, a resupplied complete Manifest, a stable digest-only
operation identity, and explicit operator design-review confirmation. Go
revalidates the complete v48-v62 authority chain and reconstructs the current
runtime-input descriptor without recapturing input or contacting Docker.

Each of the sixteen v51 checks is mapped to one fixed evidence class and source,
one bounded blocker code, and one future independent gate. Existing evidence is
classified as `never_started_daemon_evidence`, `compiled_only`,
`requirement_only`, or `simulation_only`. Every item is fixed to
`production_verified=false` and `sufficient_for_start=false`; the aggregate
result is always `blocked/deny_start` with zero verified or sufficient checks.

The same transaction stores a fixed eleven-transition
`sandbox_docker_process_lifecycle_blueprint.v1`. It describes absent-to-intent,
fixed-endpoint start submission, running inspection, natural/graceful/forced
exit, cancellation fan-out, uncertain-start handling, lease-loss orphaning,
and generation-fenced reconciliation. The blueprint requires one per-Run
generation-fenced owner, write-ahead state, bounded logs, wait semantics,
TERM-before-KILL escalation, and orphan recovery. Every transition remains
`implemented=false` and `authorized=false`.

## Security Boundary

The v63 Application path has no daemon transport, process runner, start, wait,
signal, log, attach, exec, export, or Artifact writer. It cannot capture input
and it does not accept an endpoint or resource identity from the caller.
Persistence and CLI projections retain only bounded identities, fingerprints,
counts, evidence/source codes, blocker/future-gate codes, blueprint metadata,
and false authority bits. They omit Manifest bodies, resource names, raw
container IDs, host paths, sockets, raw operation keys, and private lease
identities.

SQLite constraints and immutable triggers independently fix the sixteen
mappings, eleven transitions, review conclusion, and every authority bit.
Migration creates no review for older Runs because an operator review and
current authority revalidation cannot be invented.

## Required Evidence Before Start

- Execute the opt-in v59/v61/v62 real-daemon chain on Linux with an
  already-present exact image; it must still perform no pull or start.
- Provide independent production evidence for every v51 runtime-isolation,
  network/secret, termination/recovery, and output/export requirement.
- Implement and audit the lifecycle state machine as a later schema with a
  narrow fixed-endpoint transport and generation-fenced ownership.
- Keep output export and atomic Artifact commit behind a separate audited gate.

The design review itself cannot satisfy any of these requirements.

## Recovery And Validation

Operation replay is semantic and metadata-only. Concurrent Store instances
converge on one review under the Run write lock. Reads revalidate review,
binding, check, transition, lifecycle, evidence, and aggregate fingerprints.
Tests cover exact fixed blockers, tamper rejection, confirmation before work,
full-chain revalidation, no input/daemon calls, replay, cross-store
convergence, migration without fabricated history, SQL immutability, CLI
privacy, and false authority projection.

## Consequences

- The repository now has a durable, inspectable answer for every remaining
  start blocker instead of an informal TODO list.
- The future process protocol is reviewable before any mutation capability is
  exposed.
- Architecture completion improves without increasing execution authority or
  end-user process usability.
- Container start remains absent until a later independently reviewed release
  gate supplies real production evidence.
