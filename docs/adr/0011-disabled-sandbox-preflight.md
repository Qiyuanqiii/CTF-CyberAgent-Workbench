# ADR 0011: Disabled Sandbox Backend And Output Preflight

Status: accepted

## Context

Schema v50 provides durable Sandbox lifecycle ownership, cancellation, input integrity, and cleanup without starting a process. Before a Docker implementation can be connected, the control plane needs an explicit checklist for backend isolation and output capture. A generic "Docker is available" result would be unsafe: daemon reachability says nothing about mount propagation, network isolation, container identity, kill behavior, orphan recovery, or atomic Artifact export.

The preflight must also resist stale authority. A v48 preparation, v49 approval/candidate, or v50 lifecycle is only historical evidence; none may be treated as a reusable execution permit after Policy, budgets, artifacts, mounts, or Run ownership change.

## Decision

1. Schema v51 introduces immutable, one-to-one `sandbox_preflight.v1` facts for eligible v50 executions. Creation resupplies the complete Manifest and revalidates the v48-v50 chain, Run/Mission/Workspace/Scope, current Policy and exact approval, `os.Root` mount bindings, cumulative token/model/tool budgets, Run-lease quiescence, and every input Artifact.
2. The production service uses only `DisabledBackendInspector`. Its handshake reports `backend_disabled`, `available=false`, and an unbound `sandbox_container_identity.v1` with runtime `none` and no fingerprint.
3. The threat model has exactly 16 ordered checks: host-path isolation, private mount propagation, read-only rootfs, read-only inputs, dedicated writable output, network default deny, exact network allowlist, ephemeral secret materialization, non-root identity, CPU/memory/PID limits, wall-clock timeout, graceful then forced kill, orphan reconciliation, regular-file-only output, symlink/special-file rejection, and atomic output Artifact commit.
4. Every check is fixed to `required=true`, `verified=false`, and `evidence_state=not_probed`. Neither Go nor SQLite accepts a positive claim while the backend is disabled.
5. `sandbox_output_export_plan.v1` stores only ordered output kinds and domain-separated locator fingerprints, never raw paths. File slots require regular files and reject symlinks and special files. Every slot requires MIME detection and redaction.
6. Output behavior is fixed to all-or-nothing commit, one aggregate hard byte limit, and reconciliation before retry. Export and Artifact commit remain unauthorized.
7. The preflight root, checks, output slots, and digest-only operation ledger are immutable. Same-key retries converge across processes; a changed intent or second key for the same execution conflicts.
8. Events and CLI expose bounded status and policy metadata only. They omit raw paths, locator and output-plan fingerprints, commands, Manifest content, container identity fingerprints, operation digests, and private lease/owner identities.

## Consequences

- The Docker/backend review surface is explicit and testable before a client can create a container.
- A successful v51 command proves that the authority chain still matches and that the required checks are frozen. It does not prove Docker availability, verify any isolation control, or authorize execution.
- Future backend evidence must use a new audited protocol and migration. It cannot mutate v51 rows or reinterpret `not_probed` as success.
- Future output capture must stage and validate every slot before one atomic Artifact commit. Partial export, raw host paths, and pre-reconciliation retries remain forbidden.
- HTTP, React, models, child Agents, Rust analyzers, ordinary tools, and approvals gain no new process, Docker, network, file, or Artifact capability.

## 中文摘要

schema v51 在真实 Docker 接线前固定一份禁用态后端与输出预检。每次预检都重新提交完整 Manifest，并复核 v48-v50 权限链、Scope、Policy、审批、挂载、累计预算、Run lease 和输入 Artifact。16 项威胁检查全部保持“必需但未验证、未探测”；后端不可用、容器身份未绑定，输出只保存不透明定位指纹和类型，并固定原子提交、总字节上限、MIME/普通文件/symlink/特殊文件/脱敏/重启协调策略。所有执行、导出和 Artifact 提交能力仍为 false；该记录是后续实现清单，不是 Docker 可用证明或执行许可。
