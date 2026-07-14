# ADR 0013: Read-Only Docker Production Observation

Status: accepted

## Context

Schema v52 proves the shape of backend evidence and output transactions with in-memory fakes, but it deliberately says nothing about a real Docker daemon. The next boundary must distinguish a narrowly observed production fact from verified isolation and from authority to execute. Using the full Docker client or CLI at this stage would make create, pull, start, exec, and remove operations reachable before mount, network, secret, cancellation, orphan, and Artifact-export controls have independent evidence.

Daemon responses can also contain host names, daemon IDs, storage roots, image repository names, and other machine-specific data. A production observation therefore needs a fixed local endpoint, an allowlisted read protocol, bounded parsing, privacy-preserving persistence, and complete revalidation of the existing authority chain.

## Decision

1. Schema v53 introduces immutable `sandbox_docker_observation.v1` roots, six ordered observation items, and digest-idempotent operation facts. A single v52 output simulation may have at most eight observations.
2. `DockerReadOnlyTransport` exposes only `Endpoint`, `Ping`, `Version`, `Info`, and digest-bound `InspectImage`. It has no create, start, run, exec, pull, stop, or remove method.
3. Linux uses only `/var/run/docker.sock` with proxying disabled. The transport issues GET requests only to `/_ping`, `/version`, `/info`, and `/images/<canonical-sha256-digest>/json`, rejects redirects and changed final methods, hosts, paths, or queries, bounds responses to 2 MiB, requires an exact JSON media type when present, and rejects duplicate JSON fields. It ignores `DOCKER_HOST` and accepts no caller-selected socket or TCP endpoint.
4. Windows records the bounded `transport_unsupported` unavailable result in this slice. Named-pipe support requires a later independently audited transport; it is not emulated with the Docker CLI.
5. CLI probing requires `--confirm-readonly-probe`. Before any transport call, the Application resupplies the complete Manifest and revalidates the exact v48-v52 preparation, approval, candidate, lifecycle, preflight, simulation, Scope, Policy, budgets, Run/Sandbox leases, input Artifacts, cancellation, and cleanup state. SQLite repeats current-state checks before commit.
6. Results are limited to `observation_complete`, `daemon_unavailable`, or `image_unavailable`. A complete result may set `production_observed=true` only for daemon and image metadata. `production_verified`, backend availability/enabling, execution authority, and Artifact-commit authority remain false in Go and SQLite.
7. Read-only daemon metadata cannot prove private mount propagation. That item is explicitly `not_observable_read_only`, remains unverified, and prevents v53 from satisfying the v51 production-control gate.
8. Raw daemon ID, host name, Docker root, socket path, security-option text, image ID, RepoDigests, GraphDriver details, Manifest, command, and private lease data remain transient. SQLite, events, and CLI retain bounded facts and domain-separated fingerprints only.
9. Deterministic fake-transport tests are mandatory. A real-daemon integration test is opt-in, requires an already-present exact image digest, and must never pull an image or create a container.

## Consequences

- The workbench can safely distinguish daemon unavailable, image unavailable, and bounded metadata observation without making a write-capable Docker API reachable.
- `production_observed=true` is not production verification, backend readiness, execution permission, or proof of any mount/network/resource control.
- The Docker CLI and official client remain absent from this boundary. A later execution adapter must use a new protocol and release gate rather than extending or reinterpreting v53 rows.
- HTTP, React, models, child Agents, Rust analyzers, approvals, and ordinary tools gain no Docker or process capability.

## 中文摘要

schema v53 只允许通过固定本机端点读取 Docker daemon 和指定镜像的有界元数据。Linux transport 只连接 `/var/run/docker.sock`，只发四类 GET；Windows 当前明确记录 `transport_unsupported`。接口没有 create/start/run/exec/pull/remove，也不读取 `DOCKER_HOST` 或调用 Docker CLI。每次观测前后都重新核验完整 v48-v52 权限链，结果只能是完整观测、daemon 不可用或镜像不可用。原始 daemon 身份、主机名、Docker 根目录、socket 和 RepoDigest 不持久化。即使 `production_observed=true`，也只说明元数据被读取；私有 mount 仍然不可由只读 API 证明，生产验证、后端启用、执行和 Artifact 提交授权始终为 false。真实容器执行必须另立协议和发布门禁。
