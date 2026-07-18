# ADR 0039: Model, Plan, And Approval Controls

- Status: Accepted
- Date: 2026-07-18
- Scope: non-schema Desktop/API D1-M1, D1-P1, and D1-A1

## Context

The Windows and Web workbenches could already create Runs, queue/cancel Session input,
control Run lifecycle, and hand a bounded queue to the Go Supervisor. Three ordinary
operator workflows were still missing:

1. inspect which Go-owned Providers and model routes are configured;
2. choose one persisted Plan direction and explicitly enter Deliver;
3. inspect and decide durable approvals without using the CLI/TUI.

These surfaces must not move API keys, Policy, process control, or file-write authority
into TypeScript. They also must not turn a UI approval into unrestricted execution.

## Decision

### D1-M1: one Provider Registry

CLI, ordinary API, and Desktop construct one `modelregistry.Registry`. The Registry
registers `mock` plus valid environment-backed Anthropic-compatible Providers and then
loads the five persisted routes. `model_availability.v1` is a deterministic read
projection. It performs no Provider request and structurally omits keys, Base URLs,
environment-variable names, clients, and raw errors. Model and route identifiers are
bounded; malformed or secret-like values fail closed or are redacted.

### D1-P1: two explicit Plan operations

`plan_delivery_control.v1` deliberately separates:

- direction selection, which binds Run, proposal, and ordinal `1..3` and reuses the
  existing atomic selection/WorkItem/handoff-Note transaction; and
- Deliver transition, which requires that immutable selection and reuses the existing
  digest-idempotent Run-mode transition.

Selection cannot change phase. Deliver cannot start or resume a Run, acquire an
execution lease, call a model/tool, or grant capability.

### D1-A1: constrained durable approvals

`approval_queue.v1` returns at most 100 pending metadata records. Commands, arguments,
paths, file content, fingerprints, reasons, operation identities, and private authority
are excluded. `approval_control.v1` supports only `approve_once` and `deny`:

- Shell approval is the existing dry-run completion only;
- ScriptProcess must remain `process-disabled`;
- replace-file can only be denied;
- current Policy is rechecked before approve-once;
- permanent Policy denial cannot be overridden; and
- no Session Grant, workspace write, Shell/Local/Docker process, or capability is
  created.

A pending approval cannot be changed after its Run becomes terminal. An already
committed same-key decision remains replayable after terminal transition so a lost HTTP
response does not break idempotency.

## Desktop And HTTP Boundary

Desktop adds independent `--enable-plan-delivery` and `--enable-approvals` flags. Model
availability remains read-only. All enabled control capabilities share one ephemeral
control token distinct from the read token, while route-specific capability bits keep
them isolated. The Wails native binding allowlist remains exactly `Bootstrap`,
`SelectSkillPackage`, and `PreviewSkillPackage`.

React keeps tokens and uncertain-failure operation keys in memory, validates exact
Run/proposal/selection/approval/Session/Workspace bindings, and rejects any response
claiming process, Shell, Docker, file-write, Session-Grant, or capability authority.

## 中文结论

本批只把已有 Go 事实和受控操作接到桌面/Web，不把安全判断搬到前端。模型可用性
不读取或回显密钥，不做在线探测；Plan 必须“先三选一、再显式进入 Deliver”；审批
只能得到 dry-run 或 process-disabled 结果，永久拒绝、文件写入、Session Grant 和真实
进程均不可通过该入口获得。

## Consequences

- SQLite remains schema v73; existing Plan, Run-mode, approval, ToolRun, and
  ScriptProcess ledgers stay authoritative.
- OpenAPI contains 36 paths, 84 schemas, 27 GET operations, and 11 control POST
  operations.
- Provider connectivity diagnostics, route mutation, Diff apply, background wake/retry,
  Skill installation, and real process execution remain separate later decisions.
