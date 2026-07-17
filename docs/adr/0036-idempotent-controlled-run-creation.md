# ADR 0036: Idempotent Controlled Run Creation

## Status

Accepted as schema v72 and Desktop D1-R1. This is the first product-facing
Desktop business mutation. It creates only a closed Mission/Run/Session graph;
it does not send a Session message, call a model, acquire an execution lease,
run a tool, install a Skill, contact a network, or start a process.

## 中文摘要

schema v72 新增不可变 `run_creation.v1` operation 账本。Go 在一个 SQLite
事务中创建 Mission、交互式 Run、活动 Session、初始 Run Mode、
`preview/noop` 执行档位、root Agent 和精确初始事件，再写入只含幂等键摘要、请求
指纹和对象身份的 operation。Workspace 必须已经注册；目标最多 4096 UTF-8
字节并在持久化前脱敏；网络固定关闭、目标列表为空、预算固定为默认值，模型路由
固定为 Profile。

HTTP 新增 `POST /api/v1/runs` 与只读 `GET /api/v1/workspaces`。写路由只接受
独立 control bearer、恰好一个 `Idempotency-Key`、严格 JSON 和关闭的字段集合。
相同 key/语义跨重启与跨连接收敛到原对象，不同语义冲突。Workspace 投影不公开
宿主路径。Desktop 以单独 `--enable-run-creation` capability 开放创建，不会因此
开放现有取消/档位控制；Wails 原生 bridge 仍只有原来的三个方法。

React 只在 capability 为真时显示 New Run 对话框。令牌与不确定失败后的重试 key
仅驻留内存；同一表单重试复用 key，字段变化生成新 key。前端按请求重新复核响应
中的 Workspace、Profile、Surface、Phase、默认预算和关闭权限，不接受伪造的扩权
或串绑响应。

## Context

D0-A and D0-B established a recoverable read-first Windows shell. The next
useful product step is creating a Run without copying identifiers through the
CLI, but a broad Desktop mutation surface would mix Run creation, chat,
steering, approval, execution, and Skill installation before any one path had
an audited persistence and capability contract.

Run creation already existed in internal CLI services, but it lacked a durable
request-level operation ledger suitable for uncertain HTTP retries. A Desktop
renderer can lose a response after SQLite commits, so retry behavior must be
defined by Go and SQLite rather than by UI timing or generated object IDs.

## Decision

`run_creation.v1` is a semantic operation over goal, Workspace, Profile,
surface, phase, and requester. The raw key is normalized to 16-256 bytes and is
persisted only as a domain-separated SHA-256 digest. The request fingerprint is
also digest-only. Generated Mission, Run, Session, mode, profile, Agent, and
event identities are results, not replay inputs.

The controlled Application service accepts only an existing registered
Workspace. Defaults are `code` Profile, `code` surface, and `deliver` phase.
The resulting scope always binds that Workspace, disables networking, and has
no targets. The Run is interactive, uses the selected Profile as model route,
uses `domain.DefaultBudget()`, starts in `created`, and receives a fresh active
Session. The initial execution profile remains `preview/noop` with process,
execution, and capability authority false.

Store creation and operation insertion share one immediate SQLite transaction.
Schema triggers rebind the operation to the registered Workspace, exact
Mission/Run/Session/mode/profile/root/event graph, default budget, disabled
network, empty targets, initial timestamps, and closed execution profile. The
ledger rejects update and delete. Existing databases receive no fabricated
operation for historical Runs.

`POST /api/v1/runs` is available only when the Run-creation capability is
enabled and only to the distinct control bearer. It accepts one JSON object,
one `Idempotency-Key`, no query, an exact content type, and a bounded body.
Unknown fields, duplicate top-level fields, trailing JSON, invalid UTF-8,
noncanonical enum values, unregistered Workspaces, and goals outside 1-4096
bytes fail closed. First commit and exact replay both return `202 Accepted`;
the response marks replay explicitly.

`GET /api/v1/workspaces` uses the ordinary read bearer and bounded pagination.
Its DTO contains only ID, name, and creation time. Root paths and other host
filesystem details stay inside Go. The OpenAPI document remains generated from
Go DTOs and assigns control security to the POST while retaining read security
for the GET on the same `/runs` path.

Desktop bootstrap carries independent `control_enabled` and
`run_creation_enabled` bits. A creation-only launch receives a control token
but old cancellation/profile routes remain unavailable. The same token may
serve both capabilities only when both command-line flags are explicit. Tokens
never enter URLs, browser storage, SQLite, logs, or the registry.

## Capability Boundary

Schema v72 and D1-R1 add no:

- Session message, steering, Plan selection, Delivery checkpoint, approval,
  Diff application, Skill installation, or external-Skill selection mutation;
- Provider/model call, model stream, automatic Run start, execution lease, or
  Agent scheduling;
- Shell, LocalRunner, Docker start/exec, host process, network target, secret
  access, file write, or tool capability;
- new Wails native method, renderer path/byte input, browser-persisted token,
  installer, registry value, updater, service, or startup entry; or
- permission inference in TypeScript. Go and SQLite remain the authority.

## Validation

Domain, Store, Application, HTTP, Desktop, OpenAPI, and React regressions cover
strict validation, immutable operations, exact replay and changed-intent
conflict, cross-connection convergence, unregistered/cross-Workspace rejection,
default budget and disabled-network SQL binding, no-path Workspace projection,
unknown/duplicate/trailing/oversized HTTP input, capability separation, bearer
privacy, uncertain-response key reuse, forged-response rejection, UTF-8 byte
bounds, browser-storage absence, and Run/Session selection after creation.

The final release gate passed full ordinary/race Go suites in 271.5s/257.9s,
ordinary and secure-Desktop tests plus vet/staticcheck/govulncheck,
deterministic OpenAPI/TypeScript generation, strict TypeScript, 45 frontend
tests across 14 files, production builds, zero-finding dependency audits,
63-file/86-relative-link Markdown validation, repository privacy/forbidden-
entry/diff scans, and an isolated schema-v72 CLI smoke. The audit also fixed
historical-migration trigger teardown order, capability coupling, initial-state
replay validation, invalid UTF-8 JSON, exact root/timestamp/event-count binding,
an unnecessarily broad Store interface, forged Goal/Workspace/mode response
acceptance, and character-versus-UTF-8-byte limits. No unresolved
high/medium issue is known. No real Provider, Agent-controlled Shell/host
process, Docker operation, Skill installer hook, or external network request
is part of this validation.

## Follow-Up

The next independently audited D1 slice may add Go-owned Session message and
steering submission. It must reuse the existing RunSupervisor, durable queue,
Policy, budgets, operation keys, and event stream; it must not create a
Desktop-only execution path. Skill installation, approval/Diff mutations,
real process execution, and distribution remain separate release gates.
