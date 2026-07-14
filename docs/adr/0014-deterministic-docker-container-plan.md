# ADR 0014: Deterministic Docker Container Plans and Fake Write Transactions

Status: accepted

## Context

Schema v53 can read bounded daemon and image metadata, but it cannot prove that an eventual container request is deterministic, privacy-preserving, or constrained by the v51 threat model. Connecting a write-capable Docker transport before freezing the exact create/start/stop/export/remove intent would make review and rollback ambiguous. It would also risk treating a read-only observation as permission to execute.

The next boundary therefore needs to compile one exact, complete v53 authority chain into a reviewable container specification without contacting Docker. It must exercise transaction ordering and rollback against a fake writer while keeping every real daemon mutation unreachable.

## Decision

1. Schema v54 introduces `sandbox_docker_container_spec.v1`, immutable `sandbox_docker_container_plan.v1`, and in-memory-only `sandbox_docker_write_transaction.v1`.
2. Compilation requires a complete v53 observation and resubmitted full Manifest. Application and SQLite independently revalidate v48-v53 identity, fingerprints, Policy, approval, cumulative budgets, Run and Sandbox leases, input Artifacts, cancellation, and cleanup state.
3. The compiler fixes user `65532:65532`, a read-only root, no-new-privileges, all capabilities dropped, init enabled, read-only inputs, exactly one writable output mount, and `rprivate` propagation.
4. Networking is either `none` or a dedicated managed-egress network with default deny, an exact canonical allowlist, and a required enforcement guard. Wildcards and caller-selected daemon endpoints remain invalid.
5. Secret values never enter the specification. Referenced secrets use a private ephemeral mount under `/run/cyberagent/secrets`, are excluded from Docker metadata, and must be destroyed during cleanup.
6. CPU, memory, PID, aggregate output, timeout, cancellation grace, SIGTERM, and SIGKILL behavior are fixed by the Manifest and bounded by observed capacity where available.
7. Container names and labels are derived from bounded authority fingerprints for orphan reconciliation. Reconciliation precedes create, export occurs only after stop, and removal follows export; rollback always removes a staged container identity.
8. The full container specification remains in memory. SQLite, events, and CLI retain only bounded counts, booleans, and domain-separated fingerprints. They omit commands, arguments, paths, targets, environment values, secret references, labels, and container names.
9. The fake write harness stages exactly seven steps. Failure, simulated crash, or cancellation commits no transaction. A successful fake transaction still reports zero daemon writes, no backend contact, and no production submission.
10. Go types and SQLite checks fix production verification, backend availability/enabling, execution authorization, export authorization, and Artifact-commit authorization to false. There is no real Docker writer in schema v54.

## Consequences

- Reviewers can inspect deterministic security properties and transaction order before a write-capable transport exists.
- v54 plans are evidence about compilation and fake rollback only. They are not execution permits and do not prove that Docker enforces any control.
- A future real transport must be a separate minimal interface and release gate. It must not extend or reinterpret the v53 read-only transport or v54 fake harness.
- HTTP, React, models, child Agents, Rust analyzers, approvals, and ordinary tools gain no Docker mutation capability.

## 中文摘要

schema v54 把完整且仍有效的 v53 观测编译为确定性的内存容器规格，并用纯内存假写事务验证顺序与回滚。规格固定非 root、只读根与输入、唯一可写输出、`rprivate` mount、网络默认拒绝与精确白名单、临时密钥、资源和终止上限、orphan 身份以及停止后导出顺序。Application 与 SQLite 在边界上重新核验 v48-v53 权限链。数据库、事件和 CLI 只保存有界元数据与指纹，不保存命令、路径、网络目标、密钥引用或容器身份。七步事务在失败、崩溃或取消时提交为零；即使成功也不会接触 Docker daemon、写入生产 Artifact 或获得执行授权。真实 Docker 写 transport 必须作为后续独立协议接受审计，不能扩展 v53 的只读接口或把 v54 假写结果解释为生产验证。
