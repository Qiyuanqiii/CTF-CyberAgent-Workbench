# ADR 0010: Disabled Sandbox Lifecycle And Fencing

Status: accepted

## Context

Schema v49 records a fully revalidated Sandbox candidate, but that fact is only a point-in-time observation. A safe future backend also needs durable ownership, crash recovery, cancellation, input integrity, bounded output planning, and cleanup that can continue after its parent Run becomes terminal. Treating the Run execution lease as Sandbox ownership would prevent terminal cleanup and would mix model-turn fencing with a longer-lived external resource.

## Decision

1. Schema v50 introduces immutable `sandbox_execution.v1` roots. Creation resupplies the complete Manifest and rechecks the v49 candidate, v48 preparation, Run/Mission/Workspace/Scope, current Policy and approval, `os.Root` mount binding, aggregate budgets, and Run-lease quiescence.
2. A root always fixes `backend_enabled=false`, `execution_authorized=false`, and `backend_started=false`. It is a lifecycle record, not an execution permit, and no Runner is called.
3. Input Artifacts are loaded and content-hash verified, then pinned by exact Run, Session, Workspace, order, ID, SHA-256, size, MIME, stream, source, and redaction state. Their aggregate size is capped at 16 MiB.
4. Output planning persists only capture flags, output-path count, maximum bytes, and a digest. Raw output paths and Artifact bodies are not persisted in the lifecycle ledger or events.
5. Sandbox ownership uses a separate lease with monotonically increasing generations. The initial generation prepares only the disabled record and is released immediately. An expired or released lease may be replaced; stale generations cannot release or commit cleanup.
6. Cancellation and cleanup are immutable facts with digest-only idempotency operations. Cleanup can run after the parent Run becomes terminal and revalidates every input Artifact under the active Sandbox lease.
7. The only accepted cleanup outcome in v50 is `backend_disabled`: no backend started, no orphan was detected or reaped, all inputs were verified, no output Artifact exists, and cleanup completed.

## Consequences

- Crash recovery, cancellation intent, terminal-Run cleanup, and stale-worker fencing can be tested before introducing process authority.
- Private SQLite lease and cleanup rows retain the opaque lease ID and worker owner required for generation fencing. Run events and CLI projections omit both. Lifecycle and audit records retain no commands, argv, Manifest JSON, host/output paths, environment values, secret references, network targets, or Artifact content.
- Future Docker work must reuse these roots and leases, but it must add separately audited container identity, mount/network/secret controls, timeout/kill behavior, orphan reconciliation, and atomic output Artifact capture before any backend flag can change.
- Preparation, approval, candidate, and lifecycle records remain evidence. None grants a model, TypeScript client, Rust analyzer, or ordinary tool permission to execute.

## 中文摘要

schema v50 在不启动任何进程的前提下补齐 Sandbox 生命周期。`begin` 再次提交并复核完整 Manifest、候选、审批、Scope、Policy、挂载、预算和 Run lease；输入 Artifact 按 Run/Session/Workspace、哈希、大小、来源和顺序冻结，总计最多 16 MiB。Sandbox 使用独立 generation fencing，取消与清理事实不可变且可重放，Run 终止后仍可清理，旧 generation 无法提交。当前唯一清理结果为 `backend_disabled`，Local/Docker 继续关闭。
