# ADR 0027: Non-Authorizing Docker Production Evidence Ledger

## Status

Accepted for schema v65. This decision defines a durable machine-capture
protocol and records only zero-side-effect platform receipts in the current
product path. It does not add a Docker start or host-process capability.

## 中文摘要

schema v65 建立不可变的 `sandbox_docker_production_evidence.v1` 账本。Go
固定 v51 的 16 项检查、probe code、suite 指纹和平台分类；操作者只能绑定
同一身份创建的 v63 阻塞审查、提供摘要化操作键并显式确认机器采集。调用方
不能上传检查结论、JSON 报告、socket、路径、镜像、容器 ID 或 daemon 原始响应。

当前 Windows 只记录 `unsupported_platform`；Linux 未显式 opt-in 时记录
`opt_in_required`，设置 `CYBERAGENT_DOCKER_PRODUCTION_EVIDENCE=1` 后也只记录
`harness_pending`。三种状态都不打开 socket、不调用 Docker CLI、不发网络请求、
不创建进程。Application 还会拒绝任何声称 `capture_complete` 或
`real_daemon_contacted=true` 的采集结果，直到后续切片先实现写前 attempt、租约和
generation fencing。

该账本不是启动许可证。每个 item 的 `sufficient_for_start` 和聚合的 start、
process、output export、Artifact commit 权限都固定为 `false`，即使未来能够记录
机器验证结果，也必须再经过独立证据接受审查和进程生命周期门禁。

## Context

The v63 start-gate review maps all sixteen v51 threat-model checks to explicit
production blockers. A future Linux harness needs a stable protocol for
machine observations, crash recovery, replay, and audit without accepting an
operator-authored assertion as proof. Running the daemon before a durable
write-ahead ownership record would recreate the crash and duplicate-side-effect
window already closed in earlier Sandbox slices.

This schema therefore separates the representation and persistence boundary
from the future daemon-contacting implementation.

## Decision

`sandbox_docker_production_evidence.v1` contains a fixed suite of sixteen
ordered items. Each item has a Go-owned probe code, blocker code, bounded state,
and evidence digest. The aggregate binds the exact v63 review, Run, Mission,
Workspace, authority and threat-model fingerprints, requester, platform class,
environment fingerprint, and suite fingerprint.

The current product collector has three fail-closed outcomes:

| Platform state | Receipt | Daemon contact |
| --- | --- | --- |
| Non-Linux | `unsupported_platform` | false |
| Linux without explicit opt-in | `opt_in_required` | false |
| Linux with explicit opt-in | `harness_pending` | false |

The domain can validate a future normalized `capture_complete` observation so
the schema does not need to change merely to represent machine evidence.
However, `SandboxManifestService` rejects that outcome and any
`real_daemon_contacted=true` result in v65. Product code cannot persist real
daemon evidence until a separate write-ahead harness gate exists.

All persisted outcomes fix these fields to false:

- every item `sufficient_for_start`;
- aggregate `start_gate_passed`;
- container-start and process-execution authority;
- output-export and Artifact-commit authority.

## Input And Privacy Boundary

The capture request accepts only a v63 review ID, a bounded operation key, the
same operator identity, and explicit confirmation. Evidence conclusions,
reports, endpoints, sockets, paths, image references, resource names, container
IDs, and raw daemon responses are not request fields.

SQLite and events retain only bounded codes, counts, booleans, timestamps, and
digests. The environment fingerprint includes platform, architecture, fixed
endpoint class, opt-in state, and suite identity; it contains no hostname,
filesystem path, or secret. The audit event is metadata-only.

## Recovery And Idempotency

The operation key is reduced to a digest before persistence. A completed
same-key/same-intent replay returns the original evidence without invoking the
collector. A changed intent conflicts. One transaction writes the aggregate,
all sixteen items, operation binding, and audit event. Immutable triggers reject
updates and deletes, and each Run is capped at 32 captures.

Migration v65 does not invent historical evidence. Existing v63 reviews remain
blocked until an operator explicitly requests a new capture.

## Security Consequences

- No v65 product path opens the Docker socket, invokes a Docker executable,
  launches a process, or performs a network request.
- A custom or compromised collector cannot use the Application seam to persist
  a real-daemon receipt before the future write-ahead gate.
- Explicit opt-in and a machine-produced receipt still do not authorize start.
- Approval, model output, TypeScript, child Agents, and document content cannot
  submit or widen evidence fields.
- The schema may represent future verified observations, but it deliberately
  cannot represent a sufficient or authorizing conclusion.

## Validation

Tests cover fixed suite identity, platform and opt-in behavior, cancellation,
future positive item representation without authority, migration without
backfill, SQL immutability, exact v63/operator binding, the 32-row cap,
transactional event creation, idempotent replay, conflicting operation keys,
direct SQL operation-key-to-evidence binding, CLI metadata privacy, and rejection
of an injected real-daemon collector before any ledger row is written.

## Follow-Up

Schema v66 should add a durable write-ahead evidence-capture attempt with an
expiring generation-fenced lease, fixed Linux local endpoint, bounded deadlines,
typed failures, and restart reconciliation before any real daemon call. Only
then may a Linux harness run the sixteen probes against an exact pre-existing
image with no pull and still produce non-authorizing evidence.

Evidence acceptance must remain a later independent review. Docker
start/wait/TERM/KILL/orphan handling, output export, and atomic Artifact commit
remain separate release gates. Local execution remains disabled until an
OS-level sandbox is implemented and audited.
