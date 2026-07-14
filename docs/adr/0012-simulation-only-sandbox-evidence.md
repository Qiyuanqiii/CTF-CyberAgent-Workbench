# ADR 0012: Simulation-Only Sandbox Evidence And Output Transaction

Status: accepted

## Context

Schema v51 freezes the controls that a future Docker backend must prove, but it intentionally records no positive evidence. Before a daemon client or production Artifact writer is reachable, the control plane needs to exercise evidence binding, replay, crash recovery, output validation, redaction, and all-or-nothing commit behavior without confusing simulated success with production verification.

A single positive "backend available" bit would erase the distinction between daemon capabilities, image identity, mount and network policy, secret handling, resource and termination settings, orphan reconciliation, and output capture. A fake commit must also be unable to create a production Artifact or leak fixture content through events, CLI output, or SQLite metadata.

## Decision

1. Schema v52 introduces immutable `sandbox_backend_evidence.v1` facts produced only by `SimulationBackendClient`. The client derives evidence in memory and never contacts a Docker daemon.
2. Evidence is accepted only for a Docker Manifest and a canonical OCI image digest. It binds the complete v48-v51 authority chain plus separate fingerprints for daemon capabilities, mounts, network, secrets, container configuration, resources, termination, orphan reconciliation, and the v51 output plan.
3. Exactly 16 ordered evidence items mirror the v51 threat model. Each item is `simulated_pass` and satisfied for harness testing, but remains `verified=false`. The root fixes `trust_class=simulation_only`, `production_verified=false`, and every backend, execution, and Artifact authorization flag to false.
4. Every evidence and output boundary resupplies the full Manifest and revalidates current Scope, Policy, exact approval, mount binding, cumulative budgets, Run and Sandbox leases, input Artifact integrity, and all immutable v48-v51 identities. Historical facts never become reusable authorization.
5. `sandbox_output_fixture.v1` is strict, duplicate-aware, bounded UTF-8 test input. `InMemoryOutputHarness` requires exact slot order and kind, regular files for file slots, aggregate byte limits, MIME detection, secret redaction, and rejection of symlinks or special files.
6. The fake Artifact sink stages every redacted output before one atomic in-memory commit. Injected failure or cancellation leaves zero committed fake outputs. Production `run_artifacts` remain untouched and production Artifact count is fixed to zero.
7. SQLite stores metadata, redacted-content digests, and digest-only operation identities. It stores no fixture body, raw locator or host path, command, Manifest, secret, container ID, or private lease identity. Root, item, and operation rows are immutable; bounded same-intent retries converge across processes.
8. CLI and events label the result as simulation-only and omit fixture content, locator fingerprints, operation digests, internal leases, and container identities. Neither surface exposes a command that can create or start a container.

## Consequences

- Evidence persistence and output transaction semantics can be audited before production authority exists.
- `simulated_pass` is test-harness evidence only. It cannot satisfy a v51 production check, prove Docker availability, or authorize execution or Artifact creation.
- A future production adapter requires a new protocol and release gate with independently verified daemon observations. It may not mutate or reinterpret v52 simulation rows.
- HTTP, React, models, child Agents, Rust analyzers, approvals, and ordinary tools gain no Docker, process, network, file-write, or Artifact capability.

## 中文摘要

schema v52 在完全不连接 Docker daemon、不创建容器、不写入生产 Artifact 的前提下，验证后端证据协议和输出事务本身是否健壮。16 项证据逐项绑定镜像摘要、daemon 能力、挂载、网络、密钥、容器配置、资源、终止、孤儿恢复和输出计划，但只标记为 `simulation_only/simulated_pass`，生产验证、后端可用、执行授权和 Artifact 提交授权始终为 false。输出夹具必须严格匹配槽位，经过总字节、MIME、普通文件、symlink/特殊文件拒绝和脱敏检查后，才能一次性提交到内存假账本；失败或取消必须回滚为零。数据库、事件和 CLI 不保存或显示夹具正文、原始路径、命令、Manifest、密钥、容器身份或私有 lease。任何 v52 模拟结果都不能被解释为 Docker 可用证明或真实执行许可。
