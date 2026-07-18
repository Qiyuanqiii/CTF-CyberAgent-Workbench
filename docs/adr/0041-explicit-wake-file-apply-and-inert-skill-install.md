# ADR 0041: Explicit Wake, File Apply, And Inert Skill Installation

- Status: Accepted
- Date: 2026-07-18
- Scope: schema-v75 D1-Q2, schema-v76 D1-D2, and non-schema D1-B1

## Context

Schema v74 could persist bounded wake intent, the FileEdit surface could record an
approval intent, and Desktop could preview a local Skill package without exposing its
path. None of those preparation steps performed the corresponding action. Operators
still needed three independently authorized, recoverable controls:

1. consume one due wake through the existing RunSupervisor;
2. apply one already approved FileEdit after fresh safety checks; and
3. register one explicitly confirmed package in the inert user Skill Registry.

The controls must remain explicit and Go-owned. They must not create a background
worker, renderer-controlled path, installer hook, or general host-process capability.

## Decision

### D1-Q2: one explicit foreground wake

Schema v75 adds `run_wake_consumer.v1`. A CLI command or control request may claim one
due generation-fenced wake intent and hand at most eight steps to the existing
`run_execution_handoff.v1`. RunSupervisor, Policy, cumulative budgets, cancellation,
checkpoints, model/tool ledgers, and the private execution lease remain authoritative.

The consumer writes preparation before execution and binds completion or failure to
the exact wake generation and handoff operation. Restart replay returns the persisted
result without repeating a completed model call. If a handoff operation exists but no
result is durable, the consumption remains `prepared` and a competing request fails
closed instead of inventing a failure. Lease expiry does not reclaim that generation,
and cancellation is rejected until the exact handoff settles. A failed consumption
with a handoff ID derives model/tool-call facts from its durable result; an unknown
result stays prepared. There is no goroutine, service, startup task,
timer loop, or hidden polling path.

### D1-D2: review and apply are separate capabilities

Schema v76 adds `file_edit_apply.v1`. Apply accepts only an exact Run and Edit plus a
memory-held idempotency key. Go reloads the Run, Mission, Session, Workspace, proposal,
and durable approval. One Edit admits one apply operation. Immediately before writing,
Go rechecks the running Run, active Session, current Policy, authoritative Workspace
path, and original SHA-256. Content is staged in the target directory, synced, then
atomically replaces the target after one final path/hash check; the proposed SHA-256 is
verified after replacement.

The browser supplies neither path nor file body. A successful write is followed by a
durable result, while replay converges on the exact prior result and reports that it did
not write again. Run-bound proposals cannot use legacy `edit approve`; they require
`review-approve` and then the separately enabled `apply`. Legacy approval remains only
for non-Run compatibility proposals.

### D1-B1: confirmed installation remains inert

Desktop adds a fourth narrow Wails method, `InstallSkillPackage`. It accepts only the
short-lived one-time confirmation handle created by native selection and preview; the
renderer still cannot submit a path or archive bytes. The HTTP control accepts strict,
bounded canonical base64 under its own capability and control bearer. Both routes call
the existing content-addressed Registry service and require explicit confirmation of
the `operator_installed_untrusted` trust class.

Import validates and stores the exact archive and metadata. It does not execute package
content, scripts, hooks, commands, tools, Provider calls, network requests, or a package
manager. Installation neither selects the package for a Run nor authorizes context
delivery or declared tools. Those remain separate operator protocols.

## Protocol And Desktop Boundary

`--enable-run-wake-execution`, `--enable-file-edit-apply`, and
`--enable-skill-installation` are independent and default off. Existing wake-intent,
FileEdit-review, and Skill-preview capabilities do not imply them, and enabling any one
does not unlock its siblings. Ordinary `api serve` exposes the same Go handlers only
when a distinct control token is configured.

Public DTOs omit file bodies, source paths, archive bytes, private lease/owner identity,
raw operation keys, model text, and tool arguments. TypeScript remains a client of the
Go HTTP/native contracts and is not a security boundary.

## 中文结论

本批把“准备好了”与“真正执行一次”拆成独立权限：wake 只有用户显式触发时才通过
既有 RunSupervisor 前台消费；Diff 必须先审阅批准，再由另一能力复核当前文件和
Policy 后写入；Skill 安装只登记为不可信惰性包，不运行包内任何内容。三个能力默认
关闭、彼此不能串权，也都不会开启后台服务、通用 Shell、LocalRunner 或 Docker。

## Verification

Focused Store, Application, CLI, HTTP, Desktop, OpenAPI, and React tests cover exact
binding, replay, crash-uncertain wake state, stale hashes, Policy rechecks, independent
capabilities, path/body privacy, and inert package authority. The final ordinary Go
suite passed in 333.1s, with focused race, Windows Desktop tags, 85 React tests, strict
TypeScript, deterministic contracts, production builds, vet/module/npm checks, isolated
CLI smoke, and repository hygiene. The next batch owns the full six-slice robustness
gate and the low-risk receipt for a redacted staging file left by forced termination
before atomic replacement.

## Consequences

- SQLite advances from v74 to v76 for wake-consumption and file-apply ledgers.
- A foreground wake may incur normal model/tool budget only through the existing
  RunSupervisor path; merely scheduling a wake still executes nothing.
- File writing is now available through a narrow operator control, not as general
  renderer or model filesystem authority.
- Local untrusted Skill packages can be installed from HTTP/Desktop, but install-time
  execution, remote download, signatures, Marketplace distribution, and automatic Run
  selection remain future, separately gated work.
