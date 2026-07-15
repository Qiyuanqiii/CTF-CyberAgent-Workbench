# ADR 0016: Recoverable Docker Rehearsal Attempts

Status: accepted

## Context

Schema v55 creates, inspects, and removes one never-started container, but its durable
result is written only after all daemon operations finish. A process exit after create
and before that commit can therefore leave an exact rehearsal container behind without
a durable recovery cursor. Retrying only from a caller-held operation key is not enough:
the operator may no longer have that key, and a second process must not create twice or
delete an unrelated same-name container.

## Decision

1. Schema v56 persists `sandbox_docker_container_rehearsal_attempt.v1` before the first
   daemon mutation. The immutable intent binds the complete v48-v54 authority chain,
   requester, deterministic operation digest, endpoint class, plan, and request
   fingerprints without storing the raw operation key or execution material.
2. Each attempt has one SQLite-owned lease with a monotonically increasing generation,
   bounded expiry, and explicit release. Only the current unexpired owner and generation
   may append a stage, cleanup, failure, or completion checkpoint. An expired or released
   lease may be acquired by a new generation; stale generations fail closed, including
   otherwise idempotent checkpoint replay.
3. The transport is split into `Stage` and `Cleanup`. `Stage` verifies the already-present
   image, then creates at most one deterministic-name container or adopts an exact
   never-started authority match left by an uncertain response or prior generation. It
   never starts the container and leaves the exact stopped result available for recovery.
4. A durable stage records a 19-item immutable control matrix. The controls describe the
   inspected image/configuration, empty environment, disabled network, mount configuration,
   resource limits, absent ports/devices/attachments, exact authority labels, and
   never-started state. Every item has `execution_evidence=false`; neither Go nor SQLite
   permits this matrix to be interpreted as process-isolation proof.
5. `Cleanup` re-inspects the deterministic name and deletes only an exact authority,
   configuration, request-fingerprint, and container-ID-fingerprint match. An already
   absent container is an idempotent success. A mismatched same-name container is never
   removed.
6. Bounded failure codes are append-only. A failure releases the lease so an operator can
   resume by durable attempt ID while resubmitting the complete Manifest and explicit
   daemon-write confirmation. Application revalidates all v48-v54 authority before any
   resumed daemon access and requires the recomputed intent to match exactly.
7. The legacy v55 rehearsal, its operation row, the v56 completion, and final lease release
   commit atomically. A legacy rehearsal operation without the matching v56 completion is
   treated as a conflict rather than silently replayed.
8. Image and container inspection must both prove an empty environment. An empty Manifest
   environment alone is insufficient because an image may provide inherited variables.
9. The v55 endpoint and capability boundary is unchanged: fixed Linux local socket, Docker
   API `1.40`, disabled proxy/redirect behavior, no network or secrets, and no start, exec,
   attach, pull, logs, export, volume management, or generic request method.
10. Persistence remains metadata-only. The private lease table retains one opaque generated
    owner identity for fencing, but events and CLI never expose it. Raw container IDs, host
    paths, socket paths, commands, arguments, environment values, secrets, complete
    specifications, and operation keys are not persisted or exposed.

## Verification Boundary

Tests cover uncertain create followed by attempt-ID recovery, exact adoption without a
second create, already-absent cleanup, unrelated same-name protection, current-generation
fencing, released and expired takeover, two-Store lease contention, immutable checkpoints,
schema upgrade, CLI recovery projections, and persistence/event privacy. SQLite separately
fixes the 19 control ordinals and names and requires `execution_evidence=0`.

The stage proves only inspected configuration and never-started state. It does not prove
runtime isolation or close host-mount time-of-check/time-of-use replacement. Descriptor-
pinned Linux resolution or daemon-side immutable staging is required before any future
container start boundary.

## Consequences

- A crash around create, stage commit, delete, or final commit has a durable recovery path.
- Replay and takeover converge on one exact authority and never deliberately create twice.
- Operators can inspect and resume by attempt ID without retaining a raw operation key.
- Container execution, output export, production Artifact commit, and broader Docker access
  remain unreachable.

## 中文摘要

schema v56 在第一次 Docker daemon 变更之前先持久化恢复意图，并用带过期时间、递增
generation 的 SQLite 租约隔离并发操作者。`Stage` 只会创建一个确定性名称的未启动容器，
或者收养上一代因响应不确定、进程退出而留下的精确 authority 匹配；它不会重复创建，也
不会启动容器。随后落库的 19 项不可变矩阵只证明镜像与容器配置、空环境、禁用网络、资源
限制、mount 配置、authority 标签和 never-started 状态，每一项都固定
`execution_evidence=false`，不能当作进程隔离或生产执行证据。

`Cleanup` 会重新按确定性名称 inspect，只删除 authority、配置、请求指纹和容器 ID 指纹
全部匹配的对象；容器已经不存在时按幂等成功处理，同名但不匹配的容器绝不删除。失败只
追加有界错误代码并释放租约，操作者可以使用持久化 attempt ID、完整 Manifest 和再次显式
确认恢复。旧 v55 rehearsal、operation、v56 completion 与最终租约释放在同一事务提交。
原始容器 ID、宿主路径、socket、命令、环境值、密钥、完整规格和 operation key 仍不落库。

v56 没有扩大 Docker 权限：固定本机 socket 与 API `1.40`，仍然没有 start、exec、attach、
pull、logs、export、网络或密钥能力。宿主 mount 的 TOCTOU 也尚未被这份配置证据解决；任何
未来 start 边界之前，必须先完成文件描述符固定或 daemon 侧不可变 staging 的独立审计。
