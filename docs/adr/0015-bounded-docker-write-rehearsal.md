# ADR 0015: Bounded Docker Create-Inspect-Remove Rehearsals

Status: accepted

## Context

Schema v54 freezes a deterministic container specification and proves fake transaction ordering, but it never exercises a Docker Engine mutation. Jumping directly from that fake writer to process execution would combine transport security, daemon reconciliation, cancellation cleanup, container start, output capture, and Artifact commit in one release boundary.

The first real write boundary must therefore remain non-executing. It should prove that a narrowly scoped Go transport can create one stopped container from an exact v54 plan, inspect the daemon result, and remove it without exposing a general Docker client or granting production authority.

## Decision

1. Schema v55 introduces `sandbox_docker_container_rehearsal.v1` and a separate `sandbox_docker_write_transport.v1`. The transport is disabled by default and is never part of the v53 read-only observer.
2. A rehearsal requires an explicit operator confirmation plus an exact, complete, and still-current v54 plan. Application and SQLite independently revalidate the v48-v54 Manifest, identity, Policy, approval, budget, lease, input-Artifact, cancellation, cleanup, observation, evidence, simulation, and plan chain.
3. The first profile is Linux-only, network-disabled, environment-free, and secret-free. It accepts only the fixed local `/var/run/docker.sock`; `DOCKER_HOST`, TCP, caller-selected sockets, proxies, redirects, and daemon API negotiation are not supported.
4. The HTTP core is pinned to Docker Engine API `1.40` and exposes a closed method/path allowlist: exact read-only image inspection, one exact create request, inspect by deterministic name or returned 64-hex ID, and non-forced delete by returned ID with the fixed `v=1` anonymous-volume cleanup flag. It has no start, exec, attach, pull, logs, archive, export, network, volume-management, image-build, or general request method.
5. Before create, the already-present image must expose a RepoDigest matching the plan and declare no image `VOLUME`, preventing create-time anonymous-volume side effects. The create body must exactly match the v54 plan: digest-pinned image, non-root user, read-only root, no-new-privileges, all capabilities dropped, init enabled, network mode `none`, bounded resources, private bind propagation, no restart, and no logging driver.
6. Host mount sources are resolved beneath the trusted workspace root. Every existing path component must be non-symlink, and the final source must be a regular file or directory. Resolved paths and the full request remain transient.
7. A deterministic name collision fails closed unless the existing stopped container exactly matches the expected configuration and authority labels. Only such an exact stale rehearsal may be removed before create.
8. After create, the transport inspects and exactly revalidates the returned configuration, attachment flags, capabilities, device/port settings, and mounts before cleanup. The container is never started. A bounded independent cleanup context re-inspects by returned ID and deterministic name, then removes only an exact, never-started authority match after cancellation or request failure. An uncertain create response is reconciled by name rather than left for a later retry.
9. Durable rows, steps, operations, events, and CLI output retain metadata and domain-separated fingerprints only. They omit raw container IDs, host paths, commands, arguments, environment values, secrets, socket paths, and complete specifications.
10. Digest-keyed operation replay returns the original immutable rehearsal without contacting Docker again. Concurrent SQLite callers converge on one durable result.
11. Go types and SQLite constraints fix `container_never_started=true`, `process_never_executed=true`, `image_never_pulled=true`, `output_never_exported=true`, and every production execution, verification, backend-enable, execution-authority, and Artifact-authority flag to false.

## Verification Boundary

Unit tests use a strict fake Engine server to cover the allowed request sequence, image-declared-volume rejection, stale exact-orphan reconciliation, unsafe name collisions, cancellation after a known or uncertain create response, no-blind-delete cleanup, symlink rejection, and endpoint closure. An opt-in Linux integration test requires `CYBERAGENT_DOCKER_WRITE_TEST_IMAGE_DIGEST` to name an already-present immutable volume-free image digest. It must not pull an image and is skipped by default.

The rehearsal proves only that create, exact inspect, and remove can be bounded. It does not prove process isolation, runtime resource enforcement, termination behavior, output collection, or atomic Artifact export. A future start boundary also needs stronger protection against host-path time-of-check/time-of-use replacement, such as daemon-side immutable staging or Linux descriptor-based path pinning.

## Consequences

- v55 performs two real daemon writes in the normal path, or three when removing one exact stale rehearsal container. This is deliberately narrower than execution.
- Container process start, command execution, image pull, network access, secrets, output export, and production Artifact commit remain unreachable.
- HTTP, React, models, child Agents, Rust analyzers, and ordinary tools gain no Docker mutation capability.
- Production execution requires a later, separately reviewed protocol and cannot reinterpret a v55 rehearsal as authorization or proof.

## 中文摘要

schema v55 新增独立且默认关闭的 Docker 写 transport，只允许在 Linux 固定 `/var/run/docker.sock` 上，对完整且仍有效的 v54 计划执行一次“创建未启动容器、精确核验、删除”演练。它不接受 `DOCKER_HOST`、TCP、任意 socket、代理或重定向；API 固定为 `1.40`，方法和路径白名单只增加精确镜像 inspect、create、容器 inspect 与携带固定 `v=1` 的非强制 delete，不包含 start、exec、attach、pull、日志、导出、卷管理或通用请求。create 前必须确认本地镜像 RepoDigest 与计划一致且没有声明 `VOLUME`。首个 profile 强制无网络、无环境变量、无密钥；容器必须使用摘要镜像、非 root、只读根、drop-all capabilities、资源上限和 private mount。名称冲突只有在旧容器完全匹配且从未启动时才允许回收；取消、失败或 create 响应不确定时，独立有界 context 会按 ID/确定性名称重新 inspect，只有精确 authority match 才删除，绝不盲删。

持久化层只记录元数据和指纹，不保存原始容器 ID、宿主路径、命令、环境值、密钥、socket 或完整规格；同操作键重放不会再次访问 daemon，并发调用收敛。正常演练会产生两次真实 daemon 写入，精确回收旧演练容器时为三次，但容器进程从不启动，镜像不拉取，输出不导出，生产执行、验证、后端启用、执行授权和 Artifact 授权全部固定为 false。未来若开放 start，还必须单独解决宿主路径 TOCTOU、终止、输出和原子 Artifact 提交，不得把 v55 解释为生产执行许可。
