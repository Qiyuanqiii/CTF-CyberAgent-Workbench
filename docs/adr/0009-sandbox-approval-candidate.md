# ADR 0009: Sandbox Approval And Disabled Candidate

Status: accepted

## Context

Schema v48 can persist a strict Sandbox Manifest preparation, but an immutable description is not enough to authorize a later action. Approval may change, the Run can consume its budget or acquire a lease, Workspace links can drift, and Policy or Mission Scope can change between preparation and use.

## Decision

1. Schema v49 creates Sandbox approval requests in the shared `tool_approvals` ledger. The request is bound to the preparation ID, Run Session, Workspace, `sandbox.manifest/sandbox_execute`, and exact authorization fingerprint.
2. Only an operator CLI path can review this approval. Policy denial remains permanent and approval cannot widen Scope or enable a backend.
3. Every candidate validation resupplies the complete Manifest. Go normalizes it again and requires exact preparation, Workspace-root, Mission-Scope, Policy, and approval bindings.
4. Mount sources are opened through Go `os.Root`; links escaping the persisted Workspace root and non-file/non-directory objects fail closed. The raw root and paths are not persisted.
5. Candidate creation takes the Run write lock and rechecks aggregate token/model-time usage, tool-call usage, and execution-lease quiescence in the same SQLite transaction. SQL triggers independently enforce the immutable binding and disabled flags.
6. `sandbox_execution_candidate.v1` is metadata only. It always stores `backend_enabled=false` and `execution_authorized=false`; it cannot launch Local or Docker.

## Consequences

- Approval and revalidation are recoverable and auditable without storing commands, argv, paths, environment values, secrets, network targets, or Manifest JSON.
- Identical operation-key retries converge across processes; changed intent conflicts.
- A successful candidate proves only that the current metadata checks passed at one instant. Future execution must perform another check and remains blocked until cancellation, cleanup, network enforcement, secret materialization, host-path isolation, and Artifact export receive separate implementations and audits.

## 中文摘要

schema v49 复用统一审批账本，并要求每次候选校验重新提交 Manifest。Go 再次核对 Run、Workspace、Scope、Policy、批准、预算和租约，挂载源通过 `os.Root` 限制在工作区内。候选只保存指纹、计数和状态，始终固定 `backend_enabled=false`、`execution_authorized=false`，不启动 Local 或 Docker。
